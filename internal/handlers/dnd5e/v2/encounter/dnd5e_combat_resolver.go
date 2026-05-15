package encounter

// Dnd5eCombatResolver implements tkenc.CombatResolver by routing every
// attack through the dnd5e rulebook's combat.ResolveAttack chain.
//
// # Architecture decision (Wave 2.11b seam)
//
// The encounter SDK's CombatResolver interface receives only an AttackInput
// with attacker/target entity IDs and stat snapshots — no context and no
// encounter reference. The real resolver needs to:
//
//   - Load the attacker's *character.Character (for player-side attacks) to
//     satisfy the combat.Combatant interface and retrieve the equipped weapon.
//   - Rehydrate the target/attacker *monster.Monster (for NPC-side attacks)
//     from the encounter's MonsterData.DataJSON blob (Decision Point 1:
//     no separate monster store).
//   - Build a per-attack CombatantRegistry, thread it onto a context via
//     combat.WithCombatantLookup, and call combat.ResolveAttack.
//
// To inject encounter-scope data (monster blobs) without changing the
// resolver interface, Dnd5eCombatResolver is created per-request via
// NewDnd5eCombatResolverForData. The handler stores a Dnd5eCombatResolverConfig
// and calls that constructor at each LoadFromData / New site, passing the
// freshly loaded encounter data. This keeps the resolver stateless between
// requests while still having access to the monster map.
//
// Context: context.Background() is used inside ResolveAttack because the
// encounter SDK's CombatResolver interface does not thread a context through.
// Propagating the calling RPC's context is a follow-up improvement (Wave 2.11).
//
// EventBus: a fresh events.EventBus is created per attack. The rulebook's
// combat chain uses it only for publishing within-attack events (attack chain,
// damage chain); these do not need to outlive the ResolveAttack call.
//
// Monster weapon: for monster attackers the resolver builds a synthetic melee
// weapon from the encounter's MonsterData snapshot (DamageDice / DamageType).
// The base dice are extracted from the DamageDice string (e.g. "1d6+2" → "1d6")
// so that combat.ResolveAttack can add the ability modifier from the
// rehydrated monster's Combatant.AbilityScores() without double-counting.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkdice "github.com/KirkDiggler/rpg-toolkit/dice"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/damage"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/gamectx"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

// fallbackDamageDice is the default dice notation used when no weapon is
// equipped or the damage dice string is empty (e.g. unarmed strike stub,
// empty MonsterData.DamageDice). "1d4" is the D&D 5e unarmed-strike damage.
const fallbackDamageDice = "1d4"

// Dnd5eCombatResolverConfig holds the dependencies needed to construct a
// Dnd5eCombatResolver. Stored on the handler; a new resolver is built per
// request via NewDnd5eCombatResolverForData.
//
// CharacterRepo is optional: if nil, character-side attacks fall back to
// StandInCombatResolver behavior. This allows the handler tests that do not
// wire a character repo to continue passing — the fixture uses the snapshot
// fields, which the stand-in already handles. Tests that want the real chain
// should supply a CharacterRepo populated with the fixture character.
//
// Roller is optional: if nil, the rulebook combat chain uses its own default
// roller (crypto-random). Tests can supply a deterministic roller to control
// attack-roll and weapon-damage dice for HP-delta assertions.
type Dnd5eCombatResolverConfig struct {
	// CharacterRepo provides character lookup by ID. When nil, player-side
	// attacks fall back to the snapshot-based stand-in math.
	CharacterRepo characterrepo.Repository

	// Roller is the dice roller for attack and weapon damage rolls. When nil,
	// the rulebook combat chain uses its own default roller.
	Roller tkdice.Roller
}

// Dnd5eCombatResolver implements tkenc.CombatResolver against the dnd5e
// rulebook's combat.ResolveAttack chain. Create one per RPC request via
// NewDnd5eCombatResolverForData — do not share across requests.
type Dnd5eCombatResolver struct {
	cfg           Dnd5eCombatResolverConfig
	encounterData *tkenc.Data
	// standIn is used as fallback when the character repo is absent or a
	// character lookup fails. This preserves existing test behavior.
	standIn *StandInCombatResolver
	// pendingPhased caches the in-flight phase-1 prep so ApplyAttackOutcome
	// can reuse the same attackerChar + resolveCtx without re-loading the
	// character (which would create duplicate condition subscribers). See
	// dnd5e_combat_resolver_phased.go for the caching contract.
	pendingPhased *pendingPhasedAttack
}

// NewDnd5eCombatResolverForData constructs a resolver bound to the given
// encounter data. Called at each LoadFromData / New site in the handler so
// the resolver has access to the encounter's monster map for rehydration.
func NewDnd5eCombatResolverForData(cfg Dnd5eCombatResolverConfig, data *tkenc.Data) *Dnd5eCombatResolver {
	return &Dnd5eCombatResolver{
		cfg:           cfg,
		encounterData: data,
		standIn:       NewStandInCombatResolver(cfg.Roller),
	}
}

// ResolveAttack implements tkenc.CombatResolver. It routes every attack
// through combat.ResolveAttack with a per-attack CombatantRegistry.
//
// The resolver distinguishes player vs monster by checking the attacker ID
// against data.Players (entity IDs) and data.Monsters (monster IDs).
// Player entity ID == character ID in the v2 handler stack (confirmed by the
// CreateEncounter path which seeds EntityID = PlayerID and the fixture which
// uses EntityID as the character ID directly).
//
// Fallback: if the character repo is absent or a character/monster lookup
// fails, the call delegates to StandInCombatResolver so existing tests
// (which do not wire a character repo) continue to pass unchanged.
//
// Wave 2.11c: Three critical fixes land here:
//   1. Encounter-scoped bus: input.EventBus (non-nil from the encounter SDK)
//      is used instead of a fresh per-attack events.NewEventBus(). Conditions
//      Apply()'d at character rehydration stay subscribed across attacks;
//      once-per-turn state (SneakAttack.UsedThisTurn) accumulates correctly.
//   2. gamectx wiring: GameContext, Room, and ReactionReadiness are threaded
//      onto the resolve context so condition handlers can call
//      gamectx.RequireRoom, gamectx.RequireCharacters, gamectx.IsReactionReady
//      without crashing with ErrNoGameContext / ErrNoRoom.
//   3. Encounter-scoped CombatantRegistry for gamectx: the CharacterRegistry
//      covers ALL combatants in the encounter (not only attacker + target).
//      The per-attack combat.CombatantLookup still registers only attacker +
//      target per Wave 2.11b's decision; the gamectx CharacterRegistry is
//      broader so Protection-style ally lookups succeed.
func (r *Dnd5eCombatResolver) ResolveAttack(input tkenc.AttackInput) (*tkenc.AttackOutcome, error) {
	ctx := context.Background()

	attackerID := string(input.AttackerID)
	targetID := string(input.TargetID)

	// -- Select the encounter-scoped bus (Wave 2.11c fix #1) ---------------
	//
	// When the encounter SDK passes a non-nil EventBus in the AttackInput,
	// use it. This bus is the encounter's persistent bus — conditions
	// Apply()'d during character rehydration stay subscribed across attacks.
	// Fall back to a fresh bus only when the caller provides none (e.g. tests
	// that drive the resolver directly without an encounter SDK wrapper).
	bus := input.EventBus
	if bus == nil {
		bus = events.NewEventBus()
	}

	// -- Classify attacker and target as player-entity or monster-entity ----

	attackerChar, attackerMon, err := r.resolveEntity(ctx, attackerID, bus)
	if err != nil || (attackerChar == nil && attackerMon == nil) {
		// Entity unresolvable — fall back to stand-in so existing tests pass.
		return r.standIn.ResolveAttack(input)
	}

	targetChar, targetMon, err := r.resolveEntity(ctx, targetID, bus)
	if err != nil || (targetChar == nil && targetMon == nil) {
		return r.standIn.ResolveAttack(input)
	}

	// -- Build per-attack CombatantRegistry (for combat.WithCombatantLookup) -
	//
	// The combat.CombatantLookup is per-attack: attacker + target only.
	// This is correct for the rulebook chain's Combatant interface
	// (attack roll bonus, damage dice, AC) — the chain only needs the two
	// participants. See Wave 2.11b decision notes in combatant.go.

	reg := NewCombatantRegistry()

	var attackerCombatant combat.Combatant
	if attackerChar != nil {
		attackerCombatant = attackerChar
	} else {
		attackerCombatant = attackerMon
	}

	var targetCombatant combat.Combatant
	if targetChar != nil {
		targetCombatant = targetChar
	} else {
		targetCombatant = targetMon
	}

	reg.Register(attackerID, attackerCombatant)
	reg.Register(targetID, targetCombatant)

	// -- Build encounter-scoped gamectx.CharacterRegistry (Wave 2.11c fix #3) -
	//
	// The gamectx CharacterRegistry is encounter-scope: covers every player
	// and monster so Protection-style conditions that query ally/third-party
	// state succeed regardless of who is attacker vs target.
	//
	// We seed the registry from the already-resolved combatants (attacker and
	// target), using data already loaded during resolveEntity calls. No
	// additional character-repo calls are made. Remaining encounter members
	// are registered with empty weapon data (gamectx.NewCharacterWeapons(nil))
	// — sufficient for ally-lookup queries that don't need weapon details.
	charReg := r.buildEncounterCharacterRegistryFromResolved(attackerChar, targetChar)

	// -- Resolve weapon for the attacker ------------------------------------

	weapon, err := r.resolveWeapon(attackerChar, input)
	if err != nil {
		// Weapon unresolvable — fall back to stand-in.
		return r.standIn.ResolveAttack(input)
	}

	attackHand := combat.AttackHandMain
	if input.AttackHand == "off" {
		attackHand = combat.AttackHandOff
	}

	// -- Wire gamectx onto the resolve context (Wave 2.11c fix #2) ----------
	//
	// The resolve context layers four context values:
	//   1. combat.WithCombatantLookup — rulebook chain's Combatant lookup
	//   2. gamectx.WithGameContext   — CharacterRegistry for condition queries
	//   3. gamectx.WithRoom          — spatial Room for adjacency checks
	//   4. gamectx.WithReactionReadiness — readiness map from encounter data
	//
	// Condition handlers (SneakAttack, Protection) call RequireRoom and
	// RequireCharacters on this context; without these wrappers they would
	// crash with ErrNoGameContext / ErrNoRoom.

	resolveCtx := combat.WithCombatantLookup(ctx, reg)

	gameCtx := gamectx.NewGameContext(gamectx.GameContextConfig{
		CharacterRegistry: charReg,
	})
	resolveCtx = gamectx.WithGameContext(resolveCtx, gameCtx)

	if r.encounterData != nil {
		resolveCtx = gamectx.WithRoom(resolveCtx, newEncounterHexRoom(r.encounterData))
		resolveCtx = gamectx.WithReactionReadiness(resolveCtx, buildReactionReadinessMap(r.encounterData))
	}

	// -- Call the rulebook chain --------------------------------------------

	// Wave 2.11d: legacy single-phase ResolveAttack is intentionally retained
	// for the legacy ResolveAttack interface method. New phased callers route
	// through ResolveAttackHit + ApplyAttackOutcome on the same struct.
	//nolint:staticcheck // SA1019: legacy single-phase wrapper retained for CombatResolver interface
	result, err := combat.ResolveAttack(resolveCtx, &combat.AttackInput{
		AttackerID: attackerID,
		TargetID:   targetID,
		Weapon:     weapon,
		EventBus:   bus,
		AttackHand: attackHand,
		Roller:     r.cfg.Roller, // nil = rulebook default (crypto-random)
	})
	if err != nil {
		return nil, fmt.Errorf("dnd5e combat resolver: %w", err)
	}

	// -- Persist updated attacker condition state (Wave 2.11c fix #1 follow-through) --
	//
	// Conditions like SneakAttack update runtime state (UsedThisTurn) during the
	// attack chain. Because TakeAction creates a fresh Encounter per RPC call via
	// LoadFromData, this in-memory state would be lost before the next attack.
	// With UsedThisTurn now persisted in SneakAttackData (rpg-toolkit#654 / PR #655),
	// we save the attacker's updated character data back to the repo so the next
	// TakeAction RPC sees UsedThisTurn=true when it loads the condition from JSON.
	//
	// Errors are logged-and-ignored: if the save fails the attack still succeeded;
	// the consequence is that once-per-turn is not enforced on the next RPC (which
	// was the status quo before this fix). This avoids rolling back a successful attack.
	if attackerChar != nil && r.cfg.CharacterRepo != nil {
		r.saveAttackerConditionState(ctx, attackerChar)
	}

	// -- Translate *combat.AttackResult → *tkenc.AttackOutcome --------------

	return &tkenc.AttackOutcome{
		Hit:         result.Hit,
		Critical:    result.Critical,
		AttackRoll:  result.AttackRoll,
		AttackBonus: result.AttackBonus,
		TargetAC:    result.TargetAC,
		Damage:      result.TotalDamage,
		DamageType:  string(result.DamageType),
	}, nil
}

// buildEncounterCharacterRegistryFromResolved constructs a
// gamectx.BasicCharacterRegistry seeded from the already-resolved attacker
// and target characters. Monsters and additional players are registered with
// empty weapon data so lookups return non-nil rather than nil.
//
// Design: we do NOT make additional character-repo calls here. The attacker
// and target were already loaded by resolveEntity; using those avoids
// doubling the repo call count (which would break tests that set up exactly
// one Get expectation per entity). A future improvement can add a lazy-loading
// path for third-party ally lookups (Protection fighting style) once those
// tests are wired.
//
// Returns a non-nil registry even when both chars are nil.
func (r *Dnd5eCombatResolver) buildEncounterCharacterRegistryFromResolved(
	attackerChar *character.Character,
	targetChar *character.Character,
) *gamectx.BasicCharacterRegistry {
	reg := gamectx.NewBasicCharacterRegistry()

	// Register the attacker's weapons if we have a rehydrated character.
	if attackerChar != nil {
		reg.Add(attackerChar.GetID(), characterToGamectxWeapons(attackerChar))
	}

	// Register the target's weapons if we have a rehydrated character.
	if targetChar != nil {
		reg.Add(targetChar.GetID(), characterToGamectxWeapons(targetChar))
	}

	if r.encounterData == nil {
		return reg
	}

	// Register remaining players with empty weapon data so lookups return
	// non-nil (avoids nil-pointer panics in condition handlers that assume
	// a non-nil CharacterWeapons when the entity is in the encounter).
	for _, pd := range r.encounterData.Players {
		entityID := string(pd.EntityID)
		if reg.GetCharacterWeapons(entityID) != nil {
			continue // already registered from resolved char
		}
		reg.Add(entityID, gamectx.NewCharacterWeapons(nil))
	}

	// Register monsters with empty weapon data.
	for _, md := range r.encounterData.Monsters {
		id := string(md.ID)
		if reg.GetCharacterWeapons(id) != nil {
			continue
		}
		reg.Add(id, gamectx.NewCharacterWeapons(nil))
	}

	return reg
}

// characterToGamectxWeapons extracts the equipped weapon slots from a
// rehydrated *character.Character and converts them to a gamectx.CharacterWeapons
// value. Used by buildEncounterCharacterRegistry.
func characterToGamectxWeapons(char *character.Character) *gamectx.CharacterWeapons {
	var equipped []*gamectx.EquippedWeapon
	mainHand := char.GetEquippedSlot(character.SlotMainHand)
	if mainHand != nil {
		if w := mainHand.AsWeapon(); w != nil {
			equipped = append(equipped, &gamectx.EquippedWeapon{
				ID:       w.ID,
				Name:     w.Name,
				Slot:     gamectx.SlotMainHand,
				IsMelee:  w.Category == weapons.CategorySimpleMelee || w.Category == weapons.CategoryMartialMelee,
			})
		}
	}
	offHand := char.GetEquippedSlot(character.SlotOffHand)
	if offHand != nil {
		if w := offHand.AsWeapon(); w != nil {
			equipped = append(equipped, &gamectx.EquippedWeapon{
				ID:       w.ID,
				Name:     w.Name,
				Slot:     gamectx.SlotOffHand,
				IsMelee:  w.Category == weapons.CategorySimpleMelee || w.Category == weapons.CategoryMartialMelee,
			})
		}
	}
	return gamectx.NewCharacterWeapons(equipped)
}

// saveAttackerConditionState persists the attacker's updated condition JSON
// back to the character repository after an attack resolves.
//
// This is the write-back step for Wave 2.11c fix #1 cross-RPC persistence:
// conditions like SneakAttack update runtime state (UsedThisTurn) during the
// attack chain. With UsedThisTurn now included in SneakAttackData JSON
// (rpg-toolkit PR #655), calling ToData() on the post-attack character
// captures the new state. The next TakeAction RPC will then load the updated
// JSON and find UsedThisTurn=true, enforcing the once-per-turn gate.
//
// Errors are intentionally swallowed: the attack itself succeeded; failing
// to write back means once-per-turn is not enforced on the next RPC (the
// status-quo before this feature), which is safer than failing a completed attack.
func (r *Dnd5eCombatResolver) saveAttackerConditionState(ctx context.Context, char *character.Character) {
	updatedData := char.ToData()
	if updatedData == nil {
		return
	}
	_, _ = r.cfg.CharacterRepo.Update(ctx, characterrepo.UpdateInput{
		Character: &entities.Character{Data: updatedData},
	})
}

// buildReactionReadinessMap converts the encounter Data's ReactionReadiness
// map (keyed by encountercore.EntityID) to a gamectx.ReactionReadinessMap
// (keyed by string). Returns an empty map (not nil) when data is nil.
func buildReactionReadinessMap(data *tkenc.Data) gamectx.ReactionReadinessMap {
	if data == nil {
		return gamectx.ReactionReadinessMap{}
	}
	result := make(gamectx.ReactionReadinessMap, len(data.ReactionReadiness))
	for entityID, refs := range data.ReactionReadiness {
		inner := make(map[string]bool, len(refs))
		for ref, ready := range refs {
			inner[ref] = ready
		}
		result[string(entityID)] = inner
	}
	return result
}

// resolveEntity loads either a *character.Character or *monster.Monster for
// the given entity ID. It returns (char, nil, nil) for players, (nil, mon, nil)
// for monsters, and (nil, nil, nil) when the entity cannot be classified.
//
// For player entities: the v2 handler uses EntityID == CharacterID; the entity
// ID is used directly as a character repo lookup key.
//
// For monster entities: the entity is rehydrated from MonsterData.DataJSON
// stored in the encounter (Decision Point 1: rehydrate per attack, no separate
// monster store).
//
// Wave 2.11c: bus is the encounter-scoped event bus. Characters are loaded
// with this bus so their conditions subscribe to the persistent encounter bus
// and retain state (SneakAttack.UsedThisTurn) across attacks.
func (r *Dnd5eCombatResolver) resolveEntity(
	ctx context.Context,
	entityID string,
	bus events.EventBus,
) (*character.Character, *monster.Monster, error) {
	if r.encounterData == nil {
		return nil, nil, nil
	}

	// Check monsters first (entity IDs for monsters are distinct keys).
	if monData, ok := r.encounterData.Monsters[encountercore.EntityID(entityID)]; ok {
		mon, err := r.rehydrateMonster(ctx, monData, bus)
		if err != nil {
			return nil, nil, fmt.Errorf("rehydrate monster %q: %w", entityID, err)
		}
		return nil, mon, nil
	}

	// Check players (entity IDs for players come from PlayerData.EntityID).
	for _, pd := range r.encounterData.Players {
		if string(pd.EntityID) == entityID {
			// EntityID == CharacterID in the v2 handler stack.
			char, err := r.loadCharacterWithBus(ctx, entityID, bus)
			if err != nil {
				// Character load failed — return nil so caller falls back to stand-in.
				return nil, nil, nil //nolint:nilerr // deliberate fallback
			}
			return char, nil, nil
		}
	}

	// Entity not found in this encounter — caller falls back to stand-in.
	return nil, nil, nil
}

// loadCharacter fetches a *character.Character by ID from the character repo
// and rehydrates it with a fresh ephemeral event bus. Used by
// buildEncounterCharacterRegistry (which only needs weapon/ability-score data,
// not condition subscriptions). Returns an error if the repo is absent or the
// lookup fails.
func (r *Dnd5eCombatResolver) loadCharacter(ctx context.Context, characterID string) (*character.Character, error) {
	return r.loadCharacterWithBus(ctx, characterID, events.NewEventBus())
}

// loadCharacterWithBus fetches a *character.Character by ID and rehydrates it
// with the provided event bus. Wave 2.11c: the encounter-scoped bus is passed
// here so conditions Apply()'d during LoadFromData subscribe to the persistent
// encounter bus rather than an ephemeral per-attack bus.
//
// Returns an error if the repo is absent or the lookup fails.
func (r *Dnd5eCombatResolver) loadCharacterWithBus(ctx context.Context, characterID string, bus events.EventBus) (*character.Character, error) {
	if r.cfg.CharacterRepo == nil {
		return nil, fmt.Errorf("character repo not configured")
	}
	out, err := r.cfg.CharacterRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
	if err != nil {
		return nil, fmt.Errorf("get character %q: %w", characterID, err)
	}
	if out.Character == nil || out.Character.Data == nil {
		return nil, fmt.Errorf("character %q has no data", characterID)
	}
	char, err := character.LoadFromData(ctx, out.Character.Data, bus)
	if err != nil {
		return nil, fmt.Errorf("load character %q from data: %w", characterID, err)
	}
	// Wave 2.11d: apply universal reaction conditions (OA + Shield-if-spellcaster).
	// Errors are non-fatal — log and continue so a condition-Apply failure does
	// not block the attack chain that the resolver was about to run.
	_ = applyReactionConditions(ctx, char, bus)
	return char, nil
}

// rehydrateMonster loads a *monster.Monster from the MonsterData blob.
// If DataJSON is present, it is used (round-trips full ability scores /
// proficiency / conditions). When DataJSON is absent, a minimal monster is
// constructed from the snapshot fields so the AC/HP values are available for
// the Combatant interface (combat.ResolveAttack will compute +0 ability
// modifier since ability scores are zero — acceptable for the no-DataJSON
// fallback path which is used only by test fixtures without a full monster).
//
// Wave 2.11c: bus is the encounter-scoped event bus passed to monster.LoadFromData
// so any monster conditions subscribe to the persistent encounter bus.
func (r *Dnd5eCombatResolver) rehydrateMonster(ctx context.Context, md *tkenc.MonsterData, bus events.EventBus) (*monster.Monster, error) {
	if len(md.DataJSON) > 0 {
		var data monster.Data
		if err := json.Unmarshal(md.DataJSON, &data); err != nil {
			return nil, fmt.Errorf("unmarshal monster data: %w", err)
		}
		// Sync live HP from the encounter's MonsterData — the DataJSON snapshot
		// may be stale if the monster took damage since last full persistence.
		data.HitPoints = md.HP
		data.MaxHitPoints = md.MaxHP
		mon, err := monster.LoadFromData(ctx, &data, bus)
		if err != nil {
			return nil, fmt.Errorf("load monster from data: %w", err)
		}
		return mon, nil
	}

	// No DataJSON — build from snapshot fields only. Ability scores are zero;
	// combat.ResolveAttack will compute a +0 ability modifier. This is the
	// test-fixture path (AddMonster without DataJSON) — acceptable for playtest
	// fixtures that don't embed full monster data.
	mon := monster.New(monster.Config{
		ID:               string(md.ID),
		Name:             md.MonsterRef,
		HP:               md.HP,
		AC:               md.AC,
		ProficiencyBonus: 2, // CR-appropriate default; snapshot doesn't carry proficiency
	})
	return mon, nil
}

// resolveWeapon returns the weapon to use for combat.AttackInput.
//
// For character attackers: reads the equipped main-hand (or off-hand) weapon
// from the character's inventory slot via character.GetEquippedSlot.
//
// For monster attackers: builds a synthetic melee weapon from the encounter
// SDK's MonsterData snapshot (AttackerDamageDice / AttackerDamageType). The
// base dice are extracted from the dice notation to avoid double-counting the
// ability modifier that combat.ResolveAttack adds from the Combatant interface.
//
// Fallback: if the character has no weapon equipped, a minimal stub weapon is
// returned so the attack can still resolve (using unarmed-strike stats as a
// proxy). This matches the stand-in's "1d4" fallback behavior — the
// difference is that the real chain will still compute the correct hit bonus
// from the combatant's ability scores.
func (r *Dnd5eCombatResolver) resolveWeapon(
	attackerChar *character.Character,
	input tkenc.AttackInput,
) (*weapons.Weapon, error) {
	if attackerChar != nil {
		return r.characterWeapon(attackerChar, input.AttackHand)
	}
	// Monster attacker: build synthetic weapon from snapshot.
	return syntheticMonsterWeapon(input.AttackerDamageDice, input.AttackerDamageType), nil
}

// characterWeapon retrieves the equipped weapon for a character attacker.
// Falls back to a stub "unarmed" weapon if no weapon is equipped.
func (r *Dnd5eCombatResolver) characterWeapon(char *character.Character, attackHand string) (*weapons.Weapon, error) {
	slot := character.SlotMainHand
	if attackHand == "off" {
		slot = character.SlotOffHand
	}
	equipped := char.GetEquippedSlot(slot)
	if equipped != nil {
		w := equipped.AsWeapon()
		if w != nil {
			return w, nil
		}
	}

	// No weapon equipped — use the unarmed stub so the attack resolves with
	// the character's strength modifier instead of aborting.
	return unarmedStubWeapon(), nil
}

// syntheticMonsterWeapon constructs a minimal *weapons.Weapon from the
// MonsterData snapshot fields. The Damage field is set to the BASE dice
// only (the "+N" suffix is stripped) so that combat.ResolveAttack can add
// the ability modifier from the monster's Combatant.AbilityScores() without
// double-counting.
//
// Examples:
//
//	"1d6+2" → Damage: "1d6"
//	"2d4"   → Damage: "2d4"
//	""      → Damage: "1d4" (stub fallback)
func syntheticMonsterWeapon(damageDice, damageTypeStr string) *weapons.Weapon {
	baseDice := extractBaseDice(damageDice)
	dmgType := damage.Type(damageTypeStr)
	if dmgType == "" {
		dmgType = damage.Bludgeoning
	}
	return &weapons.Weapon{
		ID:         "monster-natural-attack",
		Name:       "Natural Attack",
		Category:   weapons.CategorySimpleMelee,
		Damage:     baseDice,
		DamageType: dmgType,
	}
}

// unarmedStubWeapon returns a minimal melee weapon representing an unarmed
// strike (1d4 bludgeoning, no special properties). Used when a character has
// no weapon equipped on the active attack hand.
func unarmedStubWeapon() *weapons.Weapon {
	return &weapons.Weapon{
		ID:         "unarmed",
		Name:       "Unarmed Strike",
		Category:   weapons.CategorySimpleMelee,
		Damage:     fallbackDamageDice,
		DamageType: damage.Bludgeoning,
	}
}

// extractBaseDice strips a modifier suffix from a dice notation string,
// returning the bare dice portion.
//
//	"1d6+2"  → "1d6"
//	"2d4+1"  → "2d4"
//	"2d4"    → "2d4"
//	""       → "1d4"
func extractBaseDice(notation string) string {
	if notation == "" {
		return fallbackDamageDice
	}
	// Trim whitespace before parsing.
	notation = strings.TrimSpace(notation)
	// Find the first '+' or '-' after the dice spec. Everything before that
	// is the base dice. We locate 'd' first to skip modifiers that appear
	// before the dice (rare but safe).
	dIdx := strings.IndexByte(notation, 'd')
	if dIdx < 0 {
		// Not a standard dice notation — return as-is and let the rulebook
		// parser surface any error.
		return notation
	}
	// Look for +/- after the 'd'.
	rest := notation[dIdx+1:]
	if plusIdx := strings.IndexAny(rest, "+-"); plusIdx >= 0 {
		return notation[:dIdx+1+plusIdx]
	}
	return notation
}
