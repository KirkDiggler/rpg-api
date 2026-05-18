package encounter

// Wave 2.11d — apply universal reaction conditions on rehydrated entities.
//
// The encounter SDK is rulebook-agnostic (per the boundary rule); rpg-api owns
// the dnd5e-specific decision of which reaction conditions to apply by default.
// Per Director B5, this code lives here (not in the encounter SDK) so the SDK
// stays free of dnd5e references for reactions.
//
// What gets applied:
//   - OpportunityAttackCondition on every melee combatant. Default-on readiness
//     is seeded by the encounter SDK at AddPlayer/AddMonster time (Wave 2.11c).
//   - ShieldSpellCondition on characters that could plausibly cast Shield
//     (have at least one 1st-level spell slot — heuristic stand-in for a real
//     "spell prepared" check). Default-off readiness; player toggles via
//     SetReactionReady.

import (
	"context"
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
)

// applyReactionConditions applies the OA and Shield conditions to a freshly-
// rehydrated character. Idempotent — Apply() returns AlreadyExists which we
// swallow (the canonical conditions library uses fresh instances per Apply
// call, so subsequent calls Apply fresh subscribers; we only Apply once per
// rehydration here).
//
// Side effect: character.Data.Conditions is NOT updated — these conditions
// are applied programmatically every rehydration, not persisted on the
// character blob. The encounter's reaction-readiness map (already persisted)
// carries the per-encounter readiness state.
func applyReactionConditions(ctx context.Context, char *character.Character, bus events.EventBus) error {
	if char == nil || bus == nil {
		return nil
	}

	// OpportunityAttackCondition is universal for melee combatants. We apply
	// it to every character; the OA condition's predicate (gamectx.IsReactionReady
	// + leaving threat range) gates whether it actually publishes a trigger.
	// The encounter SDK seeds default-on OA readiness at AddPlayer time when
	// DamageDice is set (Wave 2.11c). Non-melee characters with no melee weapon
	// will have predicates that never match (no threatened reach to leave) so
	// the condition is a silent no-op.
	oa := conditions.NewOpportunityAttackCondition(char.GetID())
	if err := oa.Apply(ctx, bus); err != nil {
		return fmt.Errorf("apply OA condition for %q: %w", char.GetID(), err)
	}

	// ShieldSpellCondition only on characters with first-level spell slots
	// (heuristic: spellcaster who plausibly has Shield prepared). Wave 2.11d
	// keeps this simple — a future wave can add proper "spells prepared"
	// tracking and tighten the gate. Default readiness is OFF so a non-wizard
	// who happens to have spell slots won't see Shield prompts they can't
	// actually take.
	if hasFirstLevelSpellSlot(char) {
		shield := conditions.NewShieldSpellCondition(char.GetID())
		if err := shield.Apply(ctx, bus); err != nil {
			return fmt.Errorf("apply Shield condition for %q: %w", char.GetID(), err)
		}
	}

	return nil
}

// hasFirstLevelSpellSlot reports whether the character has at least one
// 1st-level spell slot total (max > 0). Used as the eligibility gate for
// applying ShieldSpellCondition.
func hasFirstLevelSpellSlot(char *character.Character) bool {
	if char == nil {
		return false
	}
	data := char.ToData()
	if data == nil || data.SpellSlots == nil {
		return false
	}
	slot, ok := data.SpellSlots[1]
	if !ok {
		return false
	}
	return slot.Max > 0
}
