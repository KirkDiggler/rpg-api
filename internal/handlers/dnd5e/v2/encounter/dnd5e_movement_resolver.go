package encounter

// Dnd5eMovementResolver implements tkenc.MovementResolver by routing every
// per-step movement through the dnd5e rulebook's combat.MoveEntity chain.
//
// # Architecture (Wave 2.11e #539 seam)
//
// The encounter SDK's MovementResolver interface (added in #667 / shipped
// player-direction first, NPC-direction mirror in #672, EventBus field in
// #674) receives a MovementStepInput per atomic hex step: EntityID,
// FromHex, ToHex, and the encounter's bus (so combat.MoveEntity can
// publish MovementChain). The resolver:
//
//   - Builds gamectx context: WithRoom (so OA/Disengaging predicates can
//     query spatial state) + WithCharacters (for any condition that needs
//     character lookup) + WithReactionReadiness (so OA predicate gates on
//     per-character readiness).
//   - Calls combat.MoveEntity with a single-hex path (the SDK iterates;
//     each ResolveStep is one step). combat.MoveEntity publishes
//     MovementChain — chain subscribers (DisengagingCondition adds
//     OAPreventionSources; OpportunityAttackCondition publishes a
//     ReactionTriggerEvent + the inline triggerOpportunityAttack fires
//     the attack against the mover).
//   - Translates MoveEntityResult → MovementStepResult. MovementStopped =
//     Prevented; StopReason = PreventReason.
//
// Per-request constructor: built per LoadFromData site via
// NewDnd5eMovementResolverForData(cfg, data). The resolver holds the
// encounter Data pointer so it can construct the spatial Room from
// current positions + reaction-readiness state at each step. Same
// per-request lifetime as Dnd5eCombatResolver.
//
// # NPC-OA-only scope (per director signoff on #658 Q1=(b))
//
// Player-pause branch is deferred to toolkit#665 (future Sentinel /
// spell-during-movement reactions). For Wave 2.11e, all OA conditions
// auto-fire inline via combat.MoveEntity → triggerOpportunityAttack →
// combat.ResolveAttack. The encounter SDK's trigger buffer catches the
// ReactionTriggerEvents for observability but does not act on them.

import (
	"context"
	"encoding/json"
	"fmt"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkdice "github.com/KirkDiggler/rpg-toolkit/dice"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/gamectx"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	monsteractions "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/actions"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// Dnd5eMovementResolverConfig holds the dependencies needed to construct a
// Dnd5eMovementResolver. Stored on the handler; a new resolver is built
// per request via NewDnd5eMovementResolverForData.
//
// CharacterRepo is required to load player characters as combat.Combatant
// for the OA path: when an OA fires from combat.MoveEntity's
// triggerOpportunityAttack, the rulebook calls
// combat.GetCombatantFromContext(ctx, attackerID) which needs the attacker
// (player or monster) registered in the CombatantLookup. The resolver
// pre-loads players + rehydrates monsters into a per-step registry.
// When nil, the OA path will panic on GetCombatantFromContext — only
// non-combat encounters / handler tests without a char repo should leave
// this nil.
//
// Roller is optional: if nil, the rulebook movement chain uses its own
// default roller (crypto-random) for any OAs that fire inline.
type Dnd5eMovementResolverConfig struct {
	// CharacterRepo provides character lookup by ID for the OA path.
	CharacterRepo characterrepo.Repository

	// Roller is the dice roller for OA attack rolls when triggerOpportunityAttack
	// fires inline. When nil, combat.MoveEntity uses its own default roller.
	Roller tkdice.Roller
}

// Dnd5eMovementResolver implements tkenc.MovementResolver against the dnd5e
// rulebook's combat.MoveEntity chain. Create one per RPC request via
// NewDnd5eMovementResolverForData — do not share across requests.
type Dnd5eMovementResolver struct {
	cfg           Dnd5eMovementResolverConfig
	encounterData *tkenc.Data
}

// NewDnd5eMovementResolverForData constructs a resolver bound to the given
// encounter data. Called at each LoadFromData / New site in the handler so
// the resolver can construct a spatial.Room from the encounter's current
// entity positions when each step fires.
func NewDnd5eMovementResolverForData(cfg Dnd5eMovementResolverConfig, data *tkenc.Data) *Dnd5eMovementResolver {
	return &Dnd5eMovementResolver{
		cfg:           cfg,
		encounterData: data,
	}
}

// ResolveStep implements tkenc.MovementResolver. Runs the dnd5e rulebook's
// combat.MoveEntity chain for a single hex step.
//
// MovementStepInput.EntityID is the mover (player or monster). Per #658
// Q4 signoff, the SDK does not pass EntityType — the resolver figures
// out direction by checking the entity against data.Players /
// data.Monsters maps; for combat.MoveEntity's required EntityType field,
// we set "character" for players and "monster" for monsters.
//
// MovementStepInput.EventBus (added in #674) is the encounter's rulebook
// event bus. combat.MoveEntity publishes MovementChain on this bus;
// DisengagingCondition + OpportunityAttackCondition (both Apply()'d at
// character load via applyReactionConditions) subscribe to it.
//
// The returned MovementStepResult flags Prevented when chain subscribers
// (Disengaging) blocked the step or movement otherwise stopped.
// PreventReason carries the rulebook-side stop reason for telemetry.
func (r *Dnd5eMovementResolver) ResolveStep(input tkenc.MovementStepInput) (*tkenc.MovementStepResult, error) {
	if input.EventBus == nil {
		return nil, fmt.Errorf("MovementStepInput.EventBus is required")
	}
	if r.encounterData == nil {
		return nil, fmt.Errorf("Dnd5eMovementResolver: encounter data is nil")
	}

	entityType := r.classifyEntityType(string(input.EntityID))

	// Build the spatial.Room from current encounter state. combat.MoveEntity
	// queries the room for current positions, threatening entities, and
	// updates the room when the entity moves. The room view is read+write
	// on positions; the encounter SDK's PlayerData/MonsterData.Position is
	// the source of truth on RPC entry but combat.MoveEntity updates the
	// room mid-iteration so the next ResolveStep sees the updated state.
	room := newEncounterHexRoom(r.encounterData)

	// Wire gamectx onto the resolver context:
	//   - combat.WithRoom — combat.MoveEntity reads room from context
	//   - gamectx.WithGameContext + CharacterRegistry — for any condition
	//     handler that calls RequireCharacters during the chain
	//   - gamectx.WithRoom — duplicate of the combat one, but condition
	//     handlers (e.g., OpportunityAttackCondition.isLeavingMyThreatRange)
	//     call gamectx.RequireRoom not combat.GetRoom
	//   - gamectx.WithReactionReadiness — OA predicate gates on readiness
	ctx := context.Background()
	ctx = combat.WithRoom(ctx, room)

	// Build per-step CombatantRegistry covering all encounter participants.
	// combat.MoveEntity's triggerOpportunityAttack calls
	// combat.GetCombatantFromContext(ctx, attackerID) to resolve the OA
	// attacker — needs every potential threatener registered. The held mover
	// (#689) is registered directly; threateners are loaded lookup-only (see
	// buildCombatantRegistry).
	combReg := r.buildCombatantRegistry(ctx, input)
	ctx = combat.WithCombatantLookup(ctx, combReg)

	// Empty CharacterRegistry — chain subscribers that need character lookup
	// (e.g., gamectx.RequireCharacters) will get a non-nil registry but no
	// specific lookups will succeed. For OA/Disengaging that's fine — their
	// predicates use the spatial Room + ReactionReadiness, not character
	// state. Future condition types that need ability scores or weapon
	// state would tighten this to the populated registry the combat
	// resolver builds (rpg-api#533, out of #539 scope).
	charReg := gamectx.NewBasicCharacterRegistry()
	gameCtx := gamectx.NewGameContext(gamectx.GameContextConfig{
		CharacterRegistry: charReg,
	})
	ctx = gamectx.WithGameContext(ctx, gameCtx)
	ctx = gamectx.WithRoom(ctx, room)
	ctx = gamectx.WithReactionReadiness(ctx, buildReactionReadinessMap(r.encounterData))

	// Build the single-step path. combat.MoveEntity expects []spatial.Position;
	// it will read current position from room.GetEntityPosition(EntityID) and
	// then iterate the path. The toolkit's combat.MoveEntity already trims
	// path entries equal to currentPos, so passing [ToHex] is correct (and
	// passing [FromHex, ToHex] would also work — the leading hex would be
	// trimmed). Single-step keeps the input minimal.
	moveInput := &combat.MoveEntityInput{
		EntityID:   string(input.EntityID),
		EntityType: entityType,
		Path:       []spatial.Position{hexToPos(input.ToHex)},
		EventBus:   input.EventBus,
		Roller:     r.cfg.Roller,
	}

	result, err := combat.MoveEntity(ctx, moveInput)
	if err != nil {
		return nil, fmt.Errorf("dnd5e movement resolver: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("dnd5e movement resolver: nil result with nil error")
	}

	return &tkenc.MovementStepResult{
		Prevented:     result.MovementStopped,
		PreventReason: result.StopReason,
	}, nil
}

// classifyEntityType returns "character" for player entity IDs (matched
// against data.Players[*].EntityID) or "monster" for monster IDs
// (matched against data.Monsters map keys). Falls back to "character" if
// the entity isn't found — combat.MoveEntity will reject the call when
// room.GetEntityPosition fails, so the fallback is safe.
//
// This shape is what combat.MoveEntity expects on MoveEntityInput.EntityType.
// The encounter SDK's MovementStepInput intentionally omits the field
// (per #658 Q4 — the SDK is direction-agnostic); the resolver impl owns
// the classification.
func (r *Dnd5eMovementResolver) classifyEntityType(entityID string) string {
	if r.encounterData == nil {
		return string(entityTypeCharacter)
	}
	for _, pd := range r.encounterData.Players {
		if string(pd.EntityID) == entityID {
			return string(entityTypeCharacter)
		}
	}
	if _, ok := r.encounterData.Monsters[encountercore.EntityID(entityID)]; ok {
		return string(entityTypeMonster)
	}
	return string(entityTypeCharacter)
}

// buildCombatantRegistry constructs a CombatantRegistry seeded with every
// player + monster in the encounter. combat.MoveEntity's
// triggerOpportunityAttack relies on this lookup to fetch the OA attacker (any
// threatening combatant of the mover); registering all participants avoids the
// chicken-and-egg of "who's threatening?" being resolved by combat.MoveEntity
// itself.
//
// #689 + held entities: the SDK hands the mover on input.Mover (already
// hydrated, conditions on the encounter bus) — register it directly, no load.
// The THREATENERS (the other combatants, one of which is the OA attacker) are
// not exposed by the SDK per step (only Mover is), so they are loaded
// lookup-only on a THROWAWAY bus — NOT the encounter bus. The registry
// combatant is consumed only for attack stats/AC/weapon by
// GetCombatantFromContext; the OA condition that actually FIRES is the held
// instance the SDK cascade subscribed to the encounter bus. Loading the
// lookup-only copy on a throwaway bus therefore does NOT re-subscribe conditions
// to the encounter bus and cannot recur the #684 double-subscribe.
//
// Known limitation (toolkit gap, surfaced for the orchestrator-restructure
// chunk): the SDK should expose all held combatants to the movement resolver
// (as it does for the combat resolver via AttackInput.Attacker/Defender) so
// this lookup-only load can be dropped entirely. Until then this is the minimal
// safe path that keeps movement-OA working without re-subscribing.
func (r *Dnd5eMovementResolver) buildCombatantRegistry(ctx context.Context, input tkenc.MovementStepInput) *CombatantRegistry {
	reg := NewCombatantRegistry()

	moverID := string(input.EntityID)
	if input.Mover != nil {
		// Held mover (no load): registered directly.
		reg.Register(moverID, input.Mover)
	}

	// Throwaway bus for lookup-only threatener loads. Conditions these copies
	// Apply() subscribe to THIS bus, which is discarded after the step — they
	// never touch the encounter bus, so no double-subscribe.
	lookupBus := events.NewEventBus()

	for _, pd := range r.encounterData.Players {
		entityID := string(pd.EntityID)
		if entityID == moverID {
			continue // mover already registered from input.Mover
		}
		if char := r.loadCharacterForLookup(ctx, entityID, lookupBus); char != nil {
			reg.Register(entityID, char)
		}
	}

	for _, md := range r.encounterData.Monsters {
		entityID := string(md.ID)
		if entityID == moverID {
			continue
		}
		if mon := r.loadMonsterForLookup(ctx, md, lookupBus); mon != nil {
			reg.Register(entityID, mon)
		}
	}

	return reg
}

// loadCharacterForLookup loads a *character.Character by ID purely so the OA
// chain's GetCombatantFromContext can read its attack stats/weapon. Loaded on a
// throwaway bus (never the encounter bus) — see buildCombatantRegistry. Returns
// nil when the repo is absent or the lookup fails (the OA simply won't resolve
// that threatener, which is the pre-existing behavior for unloadable entities).
func (r *Dnd5eMovementResolver) loadCharacterForLookup(ctx context.Context, characterID string, bus events.EventBus) *character.Character {
	if r.cfg.CharacterRepo == nil {
		return nil
	}
	out, err := r.cfg.CharacterRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
	if err != nil || out == nil || out.Character == nil || out.Character.Data == nil {
		return nil
	}
	char, err := character.LoadFromData(ctx, out.Character.Data, bus)
	if err != nil {
		return nil
	}
	return char
}

// loadMonsterForLookup loads a *monster.Monster from MonsterData.DataJSON purely
// for the OA-chain lookup, on a throwaway bus. Returns nil when the blob is
// absent or unloadable.
func (r *Dnd5eMovementResolver) loadMonsterForLookup(ctx context.Context, md *tkenc.MonsterData, bus events.EventBus) *monster.Monster {
	if md == nil || len(md.DataJSON) == 0 {
		return nil
	}
	var data monster.Data
	if err := json.Unmarshal(md.DataJSON, &data); err != nil {
		return nil
	}
	data.HitPoints = md.HP
	data.MaxHitPoints = md.MaxHP
	mon, err := monster.LoadFromData(ctx, &data, bus)
	if err != nil {
		return nil
	}
	if err := monsteractions.LoadMonsterActions(mon, data.Actions); err != nil {
		return nil
	}
	return mon
}

// Compile-time check that Dnd5eMovementResolver implements the SDK
// interface.
var _ tkenc.MovementResolver = (*Dnd5eMovementResolver)(nil)
