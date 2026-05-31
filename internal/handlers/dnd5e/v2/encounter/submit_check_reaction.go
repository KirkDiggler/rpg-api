package encounter

// submit_check_reaction.go — the SubmitCheck{take_reaction} branch.
//
// When the caller's pending prompt is a reaction prompt (set by TakeAction's
// phased path / an NPCAct pause), SubmitCheck routes here. This handler is thin:
// validate the take/skip choice, delegate the phase-2 reaction completion to the
// v2 orchestrator (#582 carve-out), and map the result + sentinel errors back to
// proto. The rule-ish pieces (the opaque AttackContext decode, the per-condition
// ReactionModifier magnitudes such as Shield +5 AC) live in the injected
// ReactionResume funcs wired in handler.New — see reaction_resume.go.
//
// INTERNAL single-RPC: this resolves a reaction prompt already persisted on the
// snapshot; there is no cross-RPC wire-pause here (that is the deferred EndTurn
// concern).

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// submitReactionCheck handles the SubmitCheck{take_reaction} branch. The roll
// is ignored on this path. On success the response carries Success=true (phase 2
// ran) and Total = the reactor's choice (1 = took, 0 = skipped) for client
// telemetry; the attack outcome flows as broker events.
func (h *Handler) submitReactionCheck(
	ctx context.Context,
	req *encounterv2pb.SubmitCheckRequest,
	playerID string,
) (*encounterv2pb.SubmitCheckResponse, error) {
	out, err := h.orch.SubmitReactionCheck(ctx, &encounterorch.SubmitReactionCheckInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    core.PlayerID(playerID),
		EntityID:    req.GetEntityId(),
		Take:        req.GetTakeReaction(),
	})
	if err != nil {
		return nil, submitReactionCheckStatusError(err)
	}

	return &encounterv2pb.SubmitCheckResponse{
		Success: true,
		Total:   reactionResponseTotal(out.TookReaction),
	}, nil
}

// submitReactionCheckStatusError maps the orchestrator's SubmitReactionCheck
// errors onto gRPC status codes per pat-v2-status-code-mapping.
//
//   - ErrEncounterNotFound → NotFound
//   - ErrPlayerNotInEncounter / ErrEntityOwnershipMismatch → PermissionDenied
//   - ErrNoPendingReactionPrompt → FailedPrecondition
//   - decode / CompleteTakeAction / cascade / save failures → Internal
func submitReactionCheckStatusError(err error) error {
	switch {
	case errors.Is(err, encounterorch.ErrEncounterNotFound):
		return status.Error(codes.NotFound, "encounter not found")
	case errors.Is(err, encounterorch.ErrPlayerNotInEncounter):
		return status.Error(codes.PermissionDenied, "player is not in this encounter")
	case errors.Is(err, encounterorch.ErrEntityOwnershipMismatch):
		return status.Error(codes.PermissionDenied,
			"entity_id does not match player's controlled entity")
	case errors.Is(err, encounterorch.ErrNoPendingReactionPrompt):
		return status.Error(codes.FailedPrecondition,
			"no pending reaction prompt to resolve for caller")
	}
	return status.Errorf(codes.Internal, "submit reaction check: %v", err)
}

// reactionResponseTotal maps a take/skip choice to a small int the client can
// read for telemetry. take=1, skip=0.
func reactionResponseTotal(took bool) int32 {
	if took {
		return 1
	}
	return 0
}
