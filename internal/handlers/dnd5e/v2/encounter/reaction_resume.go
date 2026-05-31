package encounter

// reaction_resume.go — the rulebook-touching adapter seam for the two-phase
// attack (TakeAction phase-1 pause + SubmitCheck take_reaction phase-2 resume).
//
// The v2 orchestrator runs both halves but stays free of the dnd5e combat
// rulebook import. The rule-ish pieces it needs are supplied here as funcs and
// wired into encounterorch.Config:
//
//   - marshalReactionAttackContext: serializes the in-flight phase-1
//     PhasedAttackContext (Rulebook slot = the resolver's native
//     *combat.AttackContext) into the opaque AttackContextJSON persisted on the
//     pending prompt. The phase-1 counterpart to decodeReactionAttackContext.
//   - decodeReactionAttackContext: unmarshals the persisted opaque
//     AttackContextJSON back into the toolkit's PhasedAttackContext, whose
//     Rulebook slot holds the resolver's native *combat.AttackContext.
//   - buildReactionModifiers: maps a pending prompt + take/skip choice to the
//     ReactionModifier set, carrying the rule-ish Shield +5 AC magnitude.
//
// This file is intentionally OUTSIDE the depguard no-rulebook-internals set: it
// is a translation adapter (like the combat / movement resolvers), not an
// orchestrator method. It imports rulebooks/dnd5e/combat to interpret the opaque
// attack-context blob the resolver wrote in phase 1.

import (
	"encoding/json"
	"fmt"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
)

// shieldACBonus is the AC bonus the Shield spell applies in phase 2 when taken.
// Mirrors the rulebook's Shield magnitude (combat does not export a named const;
// see combat/attack_phases.go "typically +5 for Shield"). Lives in this adapter
// seam, NOT in the orchestrator — it is a rule magnitude.
const shieldACBonus = 5

// reactionShieldRef is the canonical Shield spell condition ref used to select
// the Shield modifier band.
const reactionShieldRef = "dnd5e:spells:shield"

// marshalReactionAttackContext serializes the in-flight phase-1
// PhasedAttackContext returned by TakeActionPhased into the opaque
// AttackContextJSON the orchestrator persists on the pending reaction prompt.
// The Rulebook slot holds the resolver's native *combat.AttackContext (made pure
// data in Wave 2.11d so it round-trips through JSON); this adapter type-asserts
// it so the orchestrator never touches the rulebook shape. The phase-1
// counterpart to decodeReactionAttackContext.
func marshalReactionAttackContext(ctx *tkenc.PhasedAttackContext) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil attack context")
	}
	rulebookCtx, ok := ctx.Rulebook.(*combat.AttackContext)
	if !ok {
		return nil, fmt.Errorf("AttackContext.Rulebook is %T, expected *combat.AttackContext", ctx.Rulebook)
	}
	raw, err := json.Marshal(rulebookCtx)
	if err != nil {
		return nil, fmt.Errorf("marshal attack context: %w", err)
	}
	return raw, nil
}

// decodeReactionAttackContext unmarshals the persisted opaque AttackContextJSON
// (written by the phased combat resolver in phase 1) into the toolkit's
// PhasedAttackContext. The Rulebook slot carries the resolver's native
// *combat.AttackContext, which CompleteTakeAction passes back to
// ApplyAttackOutcome. AttackerID / TargetID are mirrored so the SDK can route
// phase 2 without inspecting the opaque payload.
func decodeReactionAttackContext(raw []byte) (*tkenc.PhasedAttackContext, error) {
	rulebookCtx := &combat.AttackContext{}
	if err := json.Unmarshal(raw, rulebookCtx); err != nil {
		return nil, fmt.Errorf("unmarshal attack context: %w", err)
	}
	return &tkenc.PhasedAttackContext{
		Rulebook:   rulebookCtx,
		AttackerID: core.EntityID(rulebookCtx.AttackerID),
		TargetID:   core.EntityID(rulebookCtx.TargetID),
	}, nil
}

// buildReactionModifiers maps a pending reaction prompt + take/skip choice to
// the ReactionModifier slice for phase 2. When take is false (skip), or the
// prompt's condition is not a modifier-style reaction, it returns no modifiers.
//
// Today only Shield contributes a modifier (OA is a separate attack, not a
// modifier on the in-flight attack). When the second modifier-style reaction
// lands, add a case here.
func buildReactionModifiers(prompt *tkenc.PendingReactionPrompt, take bool) []tkenc.ReactionModifier {
	if !take || prompt == nil {
		return nil
	}
	if prompt.ConditionRef == reactionShieldRef {
		return []tkenc.ReactionModifier{
			{ConditionRef: prompt.ConditionRef, ACBonus: shieldACBonus},
		}
	}
	return nil
}

// isOneShotReaction decides whether a taken reaction consumes its readiness (the
// reactor must re-ready before the next window) vs. stays ready. This is a
// rulebook decision — Shield is a spell that costs a reaction + slot, so a taken
// Shield is one-shot; an opportunity attack is a free reaction that stays ready.
// Kept handler-side so the orchestrator stays free of per-condition ref
// knowledge (the orchestrator only performs the toolkit readiness-reset call).
func isOneShotReaction(conditionRef string) bool {
	return conditionRef == reactionShieldRef
}
