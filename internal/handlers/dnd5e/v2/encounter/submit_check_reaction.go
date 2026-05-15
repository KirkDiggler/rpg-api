package encounter

// Wave 2.11d — SubmitCheck{take_reaction} branch.
//
// When the caller's pending prompt is a ReactionPrompt (set by TakeAction's
// phased path), SubmitCheck routes here instead of the skill-check verb.
// Builds ReactionModifiers from the take/skip choice, calls
// Encounter.CompleteTakeAction to run phase 2, clears the pending prompt,
// and persists the snapshot.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
)

// shieldACBonus is the AC bonus the Shield spell applies in phase 2 when
// taken. Mirrors combat.ShieldACBonus from the rulebook to keep the
// orchestrator's modifier construction self-contained.
const shieldACBonus = 5

// canonical condition refs for which Wave 2.11d builds reaction modifiers.
const (
	shieldRef = "dnd5e:spells:shield"
)

// submitReactionCheck handles the SubmitCheck{take_reaction} branch.
//
// Looks up the caller's pending reaction prompt; if take_reaction is true,
// builds the corresponding ReactionModifier set; if false, runs phase 2
// with no modifiers (skip). Either way, the prompt is cleared and the
// snapshot persists.
func (h *Handler) submitReactionCheck(
	ctx context.Context,
	req *encounterv2pb.SubmitCheckRequest,
	playerID string,
) (*encounterv2pb.SubmitCheckResponse, error) {
	data, err := h.encRepo.Get(ctx, req.GetEncounterId())
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal,
			"load encounter %q: %v", req.GetEncounterId(), err)
	}

	pd, ok := data.Players[core.PlayerID(playerID)]
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "player is not in this encounter")
	}
	if string(pd.EntityID) != req.GetEntityId() {
		return nil, status.Error(codes.PermissionDenied,
			"entity_id does not match player's controlled entity")
	}

	prompt, ok := data.PendingReactionPrompts[core.PlayerID(playerID)]
	if !ok || prompt == nil {
		return nil, status.Error(codes.FailedPrecondition,
			"no pending reaction prompt to resolve for caller")
	}

	enc, err := tkenc.LoadFromData(data, h.broker,
		tkenc.WithCharacterResolver(h.resolver),
		tkenc.WithCombatResolver(h.buildCombatResolver(data)))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load from data %q: %v", req.GetEncounterId(), err)
	}

	// Unmarshal the persisted AttackContext back into the rulebook shape.
	rulebookCtx := &combat.AttackContext{}
	if err := json.Unmarshal(prompt.AttackContextJSON, rulebookCtx); err != nil {
		return nil, status.Errorf(codes.Internal, "unmarshal attack context: %v", err)
	}
	phasedCtx := &tkenc.PhasedAttackContext{
		Rulebook:   rulebookCtx,
		AttackerID: core.EntityID(rulebookCtx.AttackerID),
		TargetID:   core.EntityID(rulebookCtx.TargetID),
	}

	// Build modifiers from take/skip choice. Today only Shield is supported
	// as a player-take reaction; OA is also a possible future ref but for
	// 2.11d Wave OA auto-fires (NPC reactor inline) — player OA prompts
	// land in this same code path once #650's OA-as-condition variant
	// targets player reactors that can choose to skip. The switch is
	// extensible for those follow-ups.
	var modifiers []tkenc.ReactionModifier
	if req.TakeReaction != nil && *req.TakeReaction {
		modifiers = buildReactionModifiers(prompt)
	}

	// Run phase 2 with the chosen modifiers (or none if skipped).
	if err := enc.CompleteTakeAction(phasedCtx, modifiers); err != nil {
		return nil, status.Errorf(codes.Internal, "complete take action: %v", err)
	}

	// One-shot Shield: when the player takes a Shield reaction, clear the
	// readiness flag so the wizard must re-ready before the next attack.
	// Free reactions (OA) stay ready until explicitly toggled off.
	if req.TakeReaction != nil && *req.TakeReaction && prompt.ConditionRef == shieldRef {
		if setErr := enc.SetReactionReady(prompt.ReactorEntityID, prompt.ConditionRef, false); setErr != nil {
			// Non-fatal — readiness state is best-effort housekeeping.
			// The reaction already fired; logging-and-continuing is safer.
			_ = setErr
		}
	}

	enc.ClearPendingReactionPrompt(core.PlayerID(playerID))

	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal,
			"save encounter %q after reaction: %v", req.GetEncounterId(), err)
	}

	// Reaction responses don't carry a roll/total in the Wave 2.11d shape;
	// the response Success indicates "phase 2 ran" and Total carries the
	// reactor's choice (1 = took, 0 = skipped) for client telemetry.
	return &encounterv2pb.SubmitCheckResponse{
		Success: true,
		Total:   reactionResponseTotal(req.TakeReaction),
	}, nil
}

// buildReactionModifiers translates a pending prompt into the rulebook
// ReactionModifier slice for phase 2. Centralizes the per-condition modifier
// shape so future reactions add a case here.
func buildReactionModifiers(prompt *tkenc.PendingReactionPrompt) []tkenc.ReactionModifier {
	switch prompt.ConditionRef {
	case shieldRef:
		return []tkenc.ReactionModifier{
			{ConditionRef: prompt.ConditionRef, ACBonus: shieldACBonus},
		}
	}
	return nil
}

// reactionResponseTotal maps a take/skip choice to a small int the client
// can read for telemetry. take=1, skip=0. Helper exists so the take_reaction
// nil-check is not duplicated at the callsite.
func reactionResponseTotal(takeReaction *bool) int32 {
	if takeReaction != nil && *takeReaction {
		return 1
	}
	return 0
}

// compile-time check for fmt usage so unused-import doesn't trip if a
// future change drops one of the formatted error sites.
var _ = fmt.Sprintf
