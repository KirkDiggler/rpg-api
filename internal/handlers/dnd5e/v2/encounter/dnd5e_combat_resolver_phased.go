package encounter

// Wave 2.11d — Dnd5eCombatResolver phased combat methods.
//
// Implements the encounter SDK's optional PhasedCombatResolver interface:
//   - ResolveAttackHit runs phase 1 (combat.ResolveAttackHit) and returns the
//     opaque PhasedAttackContext + any ReactionTriggerEvents already emitted.
//   - ApplyAttackOutcome runs phase 2 (combat.ApplyAttackOutcome) with the
//     reactor-chosen ReactionModifiers baked in.
//
// The encounter SDK type-asserts CombatResolver → PhasedCombatResolver and
// dispatches through these methods when available. Tests exercise the path
// end-to-end through Encounter.TakeActionPhased.
//
// AttackContext is pure data (Wave 2.11d B6 refactor): the EventBus + Roller
// the damage chain needs come from ApplyAttackOutcomeInput, supplied by this
// resolver from the encounter-scoped bus + cfg.Roller. This keeps the
// AttackContext serializable across the player-reaction RPC gap (TakeAction →
// SubmitCheck{take_reaction} → CompleteTakeAction).

import (
	"context"
	"errors"
	"fmt"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/gamectx"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

// Compile-time witness: Dnd5eCombatResolver implements the encounter SDK's
// optional PhasedCombatResolver interface, enabling two-phase reaction-prompt
// flows.
var _ tkenc.PhasedCombatResolver = (*Dnd5eCombatResolver)(nil)

// phasedPrep holds the shared state produced by prepareAttack and consumed
// by ResolveAttackHit / ApplyAttackOutcome.
type phasedPrep struct {
	attackerChar *character.Character
	weapon       *weapons.Weapon
	resolveCtx   context.Context
	bus          events.EventBus
	attackHand   combat.AttackHand
}

// pendingPhasedAttack caches the in-flight phase-1 prep so phase 2 can reuse
// the same attackerChar + resolveCtx (avoiding a duplicate character load
// that would re-Apply SneakAttack and similar conditions to the encounter
// bus, creating duplicate subscribers and breaking the once-per-turn
// write-back).
//
// Single-call assumption: each Dnd5eCombatResolver instance handles one
// attack at a time (the encounter SDK calls ResolveAttackHit immediately
// followed by either inline ApplyAttackOutcome or a CompleteTakeAction
// after a player reaction prompt). The cache is cleared at the end of
// ApplyAttackOutcome.
//
// For the player-reactions branch (where phase 2 runs in a separate RPC
// after SubmitCheck), the orchestrator constructs a fresh resolver per
// request and re-runs prepareAttack from the saved AttackContext's IDs.
// In that case the cache is empty and phase 2 falls back to a fresh prep
// (same path as today's no-reactions path with a single character load).
type pendingPhasedAttack struct {
	attackerID string
	targetID   string
	prep       *phasedPrep
}

// ResolveAttackHit implements tkenc.PhasedCombatResolver. Runs the dnd5e
// combat phase-1 verb against the encounter-scoped bus + gamectx context,
// then translates the rulebook AttackContext into the SDK's opaque
// PhasedAttackContext shape.
//
// The encounter SDK installs a buffered subscriber on ReactionTriggerTopic
// before calling this method, so the second return value (resolverTriggers)
// is normally empty — the SDK reads its own buffer. The slot exists so a
// resolver implementation may pre-collect triggers if it wants to.
//
// Falls back to the stand-in path (single-phase, no chain) when the
// attacker or target cannot be resolved as a real character/monster — keeps
// existing tests that don't wire a character repo green. The fallback
// stuffs the stand-in's AttackOutcome into the phased context's Rulebook
// slot; ApplyAttackOutcome unwraps and returns it directly.
func (r *Dnd5eCombatResolver) ResolveAttackHit(
	input tkenc.AttackInput,
) (*tkenc.PhasedAttackContext, []tkenc.ReactionTrigger, error) {
	prep, err := r.prepareAttack(input)
	if err != nil {
		// Entity unresolvable — fall back to single-phase stand-in path.
		// Wrap the stand-in's outcome so phase 2 can return it directly.
		standInOutcome, sErr := r.standIn.ResolveAttack(input)
		if sErr != nil {
			return nil, nil, sErr
		}
		return &tkenc.PhasedAttackContext{
			Rulebook:   standInOutcome,
			AttackerID: input.AttackerID,
			TargetID:   input.TargetID,
		}, nil, nil
	}

	hitResult, err := combat.ResolveAttackHit(prep.resolveCtx, &combat.ResolveAttackHitInput{
		AttackerID: string(input.AttackerID),
		TargetID:   string(input.TargetID),
		Weapon:     prep.weapon,
		EventBus:   prep.bus,
		Roller:     r.cfg.Roller,
		AttackHand: prep.attackHand,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("dnd5e resolve attack hit: %w", err)
	}

	// Cache the prep for ApplyAttackOutcome to reuse — avoids re-loading the
	// character (which would re-Apply SneakAttack to the encounter bus and
	// create duplicate subscribers). See pendingPhasedAttack docs.
	r.pendingPhased = &pendingPhasedAttack{
		attackerID: string(input.AttackerID),
		targetID:   string(input.TargetID),
		prep:       prep,
	}

	// Note: write-back of attacker condition state happens in ApplyAttackOutcome,
	// not here. SneakAttack flips UsedThisTurn during the DAMAGE chain (which
	// runs in ApplyAttackOutcome), not the attack chain. Saving here would
	// persist a stale UsedThisTurn=false snapshot.

	return &tkenc.PhasedAttackContext{
		Rulebook:   hitResult,
		AttackerID: input.AttackerID,
		TargetID:   input.TargetID,
	}, nil, nil
}

// ApplyAttackOutcome implements tkenc.PhasedCombatResolver. Translates the
// SDK's ReactionModifiers into rulebook combat.ReactionModifier values and
// runs phase 2 against the dnd5e combat verbs.
//
// On a hit, publishes DamageReceivedEvent on the encounter bus (handled
// inside combat.ApplyAttackOutcome). On a miss (e.g. Shield deflected),
// returns Hit=false; the encounter SDK's wrapper converts that to a
// no-damage AttackOutcome that publishes only AttackResolvedEvent.
//
// The EventBus + Roller passed to combat.ApplyAttackOutcome come from the
// cached prep when present (same RPC as ResolveAttackHit) or from a fresh
// prepareAttack call (player-reactions resume path). The AttackContext
// itself is pure data and does not carry these references.
func (r *Dnd5eCombatResolver) ApplyAttackOutcome(
	phasedCtx *tkenc.PhasedAttackContext,
	modifiers []tkenc.ReactionModifier,
) (*tkenc.AttackOutcome, error) {
	if phasedCtx == nil {
		return nil, errors.New("nil PhasedAttackContext")
	}
	// Fast path: stand-in fallback from ResolveAttackHit packaged its
	// AttackOutcome directly into Rulebook. Return it unchanged.
	if standInOutcome, ok := phasedCtx.Rulebook.(*tkenc.AttackOutcome); ok {
		return standInOutcome, nil
	}
	hitResult, ok := phasedCtx.Rulebook.(*combat.AttackContext)
	if !ok {
		return nil, fmt.Errorf("PhasedAttackContext.Rulebook is %T, expected *combat.AttackContext or *tkenc.AttackOutcome",
			phasedCtx.Rulebook)
	}

	// Translate SDK modifiers → rulebook modifiers.
	rulebookMods := make([]combat.ReactionModifier, 0, len(modifiers))
	for _, m := range modifiers {
		rulebookMods = append(rulebookMods, combat.ReactionModifier{
			ConditionRef: m.ConditionRef,
			ACBonus:      m.ACBonus,
		})
	}

	// Phase 2 needs the same gamectx (room + character registry + reaction
	// readiness) as phase 1. Prefer the cached prep from ResolveAttackHit if
	// we're in the same TakeAction call window — this avoids re-loading the
	// character, which would re-Apply SneakAttack to the encounter bus and
	// create duplicate subscribers (each instance of SneakAttack tries to
	// add to the damage chain; only one wins, and the loser persists
	// UsedThisTurn=false).
	//
	// On the player-reactions branch (CompleteTakeAction in a separate RPC),
	// the resolver was constructed fresh and the cache is empty. Fall back
	// to a fresh prepareAttack — that path has the same single-character-load
	// shape as today's legacy ResolveAttack and the duplicate-subscriber
	// issue does not apply.
	var prep *phasedPrep
	if r.pendingPhased != nil &&
		r.pendingPhased.attackerID == string(phasedCtx.AttackerID) &&
		r.pendingPhased.targetID == string(phasedCtx.TargetID) {
		prep = r.pendingPhased.prep
		r.pendingPhased = nil // single-call cache; clear before phase 2 runs
	} else {
		freshPrep, err := r.prepareAttack(tkenc.AttackInput{
			AttackerID: phasedCtx.AttackerID,
			TargetID:   phasedCtx.TargetID,
			EventBus:   nil, // re-derived from prep
		})
		if err != nil {
			return nil, fmt.Errorf("rebuild phase-2 context: %w", err)
		}
		prep = freshPrep
	}

	result, err := combat.ApplyAttackOutcome(prep.resolveCtx, &combat.ApplyAttackOutcomeInput{
		HitResult: hitResult,
		Reactions: rulebookMods,
		EventBus:  prep.bus,
		Roller:    r.cfg.Roller,
	})
	if err != nil {
		return nil, fmt.Errorf("dnd5e apply attack outcome: %w", err)
	}

	// Persist updated attacker condition state (Wave 2.11c write-back). The
	// damage chain just ran in combat.ApplyAttackOutcome — SneakAttack's
	// UsedThisTurn flip is captured here. Saving earlier (after phase 1)
	// would persist a stale snapshot from before the damage chain mutation.
	if prep.attackerChar != nil && r.cfg.CharacterRepo != nil {
		r.saveAttackerConditionState(context.Background(), prep.attackerChar)
	}

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

// prepareAttack centralizes the shared "resolve entities + build registry +
// build resolve context" steps used by ResolveAttackHit (and the phase-2
// resume path in ApplyAttackOutcome). The path mirrors the legacy
// ResolveAttack flow in dnd5e_combat_resolver.go.
//
// Returns nil prep + err if the entities cannot be resolved (caller should
// fall back to the stand-in resolver in that case).
//
//nolint:gocyclo // shared setup spans multiple gates, mirroring the legacy resolver
func (r *Dnd5eCombatResolver) prepareAttack(input tkenc.AttackInput) (*phasedPrep, error) {
	ctx := context.Background()
	attackerID := string(input.AttackerID)
	targetID := string(input.TargetID)

	bus := input.EventBus
	if bus == nil {
		bus = events.NewEventBus()
	}

	attackerChar, attackerMon, err := r.resolveEntity(ctx, attackerID, bus)
	if err != nil || (attackerChar == nil && attackerMon == nil) {
		if err != nil {
			return nil, fmt.Errorf("resolve attacker %q: %w", attackerID, err)
		}
		return nil, fmt.Errorf("resolve attacker %q: not found in encounter", attackerID)
	}
	targetChar, targetMon, err := r.resolveEntity(ctx, targetID, bus)
	if err != nil || (targetChar == nil && targetMon == nil) {
		if err != nil {
			return nil, fmt.Errorf("resolve target %q: %w", targetID, err)
		}
		return nil, fmt.Errorf("resolve target %q: not found in encounter", targetID)
	}

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

	charReg := r.buildEncounterCharacterRegistryFromResolved(attackerChar, targetChar)

	weapon, err := r.resolveWeapon(attackerChar, input)
	if err != nil {
		return nil, fmt.Errorf("resolve weapon: %w", err)
	}

	attackHand := combat.AttackHandMain
	if input.AttackHand == "off" {
		attackHand = combat.AttackHandOff
	}

	resolveCtx := combat.WithCombatantLookup(ctx, reg)
	gameCtx := gamectx.NewGameContext(gamectx.GameContextConfig{
		CharacterRegistry: charReg,
	})
	resolveCtx = gamectx.WithGameContext(resolveCtx, gameCtx)
	if r.encounterData != nil {
		resolveCtx = gamectx.WithRoom(resolveCtx, newEncounterHexRoom(r.encounterData))
		resolveCtx = gamectx.WithReactionReadiness(resolveCtx, buildReactionReadinessMap(r.encounterData))
	}

	return &phasedPrep{
		attackerChar: attackerChar,
		weapon:       weapon,
		resolveCtx:   resolveCtx,
		bus:          bus,
		attackHand:   attackHand,
	}, nil
}
