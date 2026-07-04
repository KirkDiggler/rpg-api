package encounter

// Dnd5eCombatResolver phased combat methods.
//
// Implements the encounter SDK's optional PhasedCombatResolver interface:
//   - ResolveAttackHit runs phase 1 (combat.ResolveAttackHit) and returns the
//     opaque PhasedAttackContext.
//   - ApplyAttackOutcome runs phase 2 (combat.ApplyAttackOutcome) with the
//     reactor-chosen ReactionModifiers baked in.
//
// The encounter SDK type-asserts CombatResolver → PhasedCombatResolver and
// dispatches through these methods when available.
//
// #689: both phases build their context from the SDK-held attacker/defender
// (AttackInput.Attacker/Defender) — no character re-load.
//
// Same-RPC path (the common attack path — no player reaction, or all reactions
// resolved inline): the SDK calls ResolveAttackHit then ApplyAttackOutcome on
// the SAME resolver instance. We cache the phase-1 heldAttackPrep and reuse it
// for phase 2. This is NOT a re-load — it reuses the same already-hydrated,
// already-subscribed entities the SDK handed us in phase 1, so the #684
// double-subscribe class cannot recur. The cache replaces the old
// pendingPhasedAttack cache, which existed to avoid a re-LOAD; here there is no
// re-load to avoid, only a prep (combatant lookup + gamectx) to reuse.
//
// Cross-RPC resume (CompleteTakeAction in a new RPC after a player reaction
// prompt): a fresh resolver with an empty cache. combat.ApplyAttackOutcome
// needs the held attacker via GetCombatantFromContext, but the SDK's
// ApplyAttackOutcome(attackCtx, modifiers) does not pass the held entities and
// exposes no accessor for e.combatants. This path therefore cannot be served
// without re-loading (the very thing #689 removes). See errPhase2NeedsHeldEntity
// and resolveResumePrep. Tracked as a toolkit gap (the design, plan/10 §92/§97,
// intends the SDK to plumb e.combatants[phasedCtx.AttackerID] into
// ApplyAttackOutcome; v0.17.0's ApplyAttackOutcome(ctx, modifiers) still plumbs
// held entities only for phase 1 — verified against the released tag). The
// in-scope playtest (rage + combat) does not exercise the cross-RPC resume.
// Note: toolkit#662 (NPC-attacker resume host lookup) is now closed, but it did
// not add a held-entity accessor to ApplyAttackOutcome, so this gap stands.

import (
	"errors"
	"fmt"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	dnd5eEvents "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/events"
)

// Compile-time witness: Dnd5eCombatResolver implements the encounter SDK's
// optional PhasedCombatResolver interface, enabling two-phase reaction-prompt
// flows.
var _ tkenc.PhasedCombatResolver = (*Dnd5eCombatResolver)(nil)

// errPhase2NeedsHeldEntity is returned by ApplyAttackOutcome on the cross-RPC
// resume path when the phase-1 prep is not cached (fresh resolver) and the SDK
// did not hand the held entities. Surfacing the gap loudly beats silently
// re-loading (which would reintroduce the #684 double-subscribe).
var errPhase2NeedsHeldEntity = errors.New(
	"phase-2 resume needs the SDK to supply the held attacker/defender to ApplyAttackOutcome " +
		"(toolkit gap: e.combatants not plumbed into the resume path)")

// ResolveAttackHit implements tkenc.PhasedCombatResolver. Runs the dnd5e combat
// phase-1 verb against the held entities + encounter-scoped bus, then translates
// the rulebook AttackContext into the SDK's opaque PhasedAttackContext shape.
//
// Falls back to the stand-in path (single-phase, no chain) when the held
// attacker is absent — keeps fixtures without a character store green. The
// fallback stuffs the stand-in's AttackOutcome into the phased context's
// Rulebook slot; ApplyAttackOutcome unwraps and returns it directly.
func (r *Dnd5eCombatResolver) ResolveAttackHit(
	input tkenc.AttackInput,
) (*tkenc.PhasedAttackContext, []tkenc.ReactionTrigger, error) {
	prep, ok, err := r.prepareHeldAttack(input)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		// Held attacker unavailable — fall back to single-phase stand-in path.
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
		AttackerID: prep.attackerID,
		TargetID:   prep.targetID,
		Weapon:     prep.weapon,
		EventBus:   prep.bus,
		Roller:     r.cfg.Roller,
		AttackHand: prep.attackHand,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("dnd5e resolve attack hit: %w", err)
	}

	// Cache the phase-1 prep for the same-RPC ApplyAttackOutcome to reuse — it
	// reuses the already-held entities, not a re-load. Keyed by IDs so a
	// cross-RPC fresh resolver (empty cache) does not silently mis-serve.
	r.phasedPrep = prep

	return &tkenc.PhasedAttackContext{
		Rulebook:   hitResult,
		AttackerID: input.AttackerID,
		TargetID:   input.TargetID,
	}, nil, nil
}

// ApplyAttackOutcome implements tkenc.PhasedCombatResolver. Translates the SDK's
// ReactionModifiers into rulebook combat.ReactionModifier values and runs phase
// 2 against the dnd5e combat verbs.
//
// #689: phase 2 reuses the held-entity prep cached in phase 1 (same-RPC path).
// No re-load. The cross-RPC resume path is a known toolkit gap — see the file
// header and errPhase2NeedsHeldEntity.
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

	prep, err := r.resolveResumePrep(phasedCtx)
	if err != nil {
		return nil, err
	}

	// Translate SDK modifiers → rulebook modifiers.
	rulebookMods := make([]combat.ReactionModifier, 0, len(modifiers))
	for _, m := range modifiers {
		rulebookMods = append(rulebookMods, combat.ReactionModifier{
			ConditionRef: m.ConditionRef,
			ACBonus:      m.ACBonus,
		})
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

	// No write-back here: the SDK's ToData cascade (#689) re-serializes the held
	// attacker's mutated condition state (SneakAttack.UsedThisTurn) when the host
	// persists the encounter. The resolver no longer touches the character store.

	return &tkenc.AttackOutcome{
		Hit:                 result.Hit,
		Critical:            result.Critical,
		AttackRoll:          result.AttackRoll,
		AttackBonus:         result.AttackBonus,
		TargetAC:            result.TargetAC,
		Damage:              result.TotalDamage,
		DamageType:          string(result.DamageType),
		Components:          translateDamageComponents(result.Breakdown),
		HasAdvantage:        result.HasAdvantage,
		HasDisadvantage:     result.HasDisadvantage,
		AdvantageSources:    result.AdvantageSources,
		DisadvantageSources: result.DisadvantageSources,
	}, nil
}

// resolveResumePrep returns the phase-2 prep. On the same-RPC path it reuses the
// prep cached by ResolveAttackHit (held entities, no re-load). On the cross-RPC
// resume path the cache is empty and the SDK does not supply the held entities,
// so we surface errPhase2NeedsHeldEntity rather than re-loading (which would
// reintroduce #684). See the file header.
func (r *Dnd5eCombatResolver) resolveResumePrep(phasedCtx *tkenc.PhasedAttackContext) (*heldAttackPrep, error) {
	if r.phasedPrep != nil &&
		r.phasedPrep.attackerID == string(phasedCtx.AttackerID) &&
		r.phasedPrep.targetID == string(phasedCtx.TargetID) {
		prep := r.phasedPrep
		r.phasedPrep = nil // single-call cache; clear before phase 2 runs
		return prep, nil
	}
	return nil, errPhase2NeedsHeldEntity
}

// translateDamageComponents converts a dnd5e DamageBreakdown into the
// encounter SDK's generic []core.DamageComponent slice. Returns nil when
// breakdown is nil or empty (stand-in resolver, NPC attack).
func translateDamageComponents(breakdown *combat.DamageBreakdown) []encountercore.DamageComponent {
	if breakdown == nil || len(breakdown.Components) == 0 {
		return nil
	}
	out := make([]encountercore.DamageComponent, 0, len(breakdown.Components))
	for _, c := range breakdown.Components {
		out = append(out, encountercore.DamageComponent{
			Source:     damageComponentSource(c),
			Amount:     c.Total(),
			DamageType: string(c.DamageType),
			IsCritical: c.IsCritical,
		})
	}
	return out
}

// damageComponentSource derives the opaque source string for a DamageComponent.
// Uses SourceRef.String() when a ref is available (e.g. "dnd5e:features:sneak_attack"),
// falls back to the category string (e.g. "weapon", "ability") otherwise.
func damageComponentSource(c dnd5eEvents.DamageComponent) string {
	if c.SourceRef != nil {
		return c.SourceRef.String()
	}
	return string(c.Source)
}
