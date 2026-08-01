package encounter

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	"github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// d20 roll bounds — defense-in-depth alongside the toolkit's ErrInvalidRoll, so
// we surface InvalidArgument before reaching the verb.
const (
	minD20Roll = 1
	maxD20Roll = 20
)

// SubmitCheck handles the SubmitCheck RPC. It validates the request envelope,
// delegates skill-check resolution (or the take_reaction branch) to the v2
// orchestrator, and translates the result (and sentinel errors) back to proto.
//
// The orchestrator owns the load → SubmitCheck verb → persist flow (#582
// carve-out). The toolkit owns modifier resolution (via the wired
// CharacterResolver) and any success side-effect dispatch — for the locked-door
// case the wired action is "open", which the toolkit dispatches to OpenDoor
// internally (publishing DoorOpened + GeometryRevealed through the broker). This
// handler stays thin: proto↔input mapping plus submitCheckStatusError.
//
// Behavior contract (preserved across the carve):
//   - missing auth → Unauthenticated
//   - empty encounter_id / entity_id → InvalidArgument
//   - take_reaction set → routes to the reaction branch (the roll is ignored;
//     reaction prompts don't carry a d20 roll)
//   - roll outside [1, 20] → InvalidArgument (defense in depth before the
//     toolkit also checks; clearer error to the client)
//   - encounter not in repo → NotFound
//   - caller not a member / entity_id mismatch → PermissionDenied
//   - no pending prompt → FailedPrecondition
//   - prompt is not a skill check → FailedPrecondition
//   - prompt's TriggeredAction not wired → Unimplemented
//   - encounter has no character resolver → Internal (orchestrator
//     misconfigured itself; the resolver is wired by HandlerConfig)
//   - roll rejected by toolkit (range disagreement) → InvalidArgument
//   - dispatch / save failure → Internal
//
// On success the response carries the resolved total (roll + ability modifier +
// tool-proficiency bonus) and a boolean for total ≥ DC. World effects flow as
// broker events on the existing StreamEncounter session.
func (h *Handler) SubmitCheck(ctx context.Context, req *encounterv2pb.SubmitCheckRequest) (*encounterv2pb.SubmitCheckResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetEntityId() == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}
	// take_reaction is the reaction-prompt response semantic. When set, route to
	// the reaction branch; the roll field is ignored (reaction prompts don't
	// carry a d20 roll).
	if req.TakeReaction != nil {
		return h.submitReactionCheck(ctx, req, playerID)
	}
	if req.GetRoll() < minD20Roll || req.GetRoll() > maxD20Roll {
		return nil, status.Errorf(codes.InvalidArgument,
			"roll %d is outside the d20 range [%d, %d]",
			req.GetRoll(), minD20Roll, maxD20Roll)
	}

	out, err := h.orch.SubmitCheck(ctx, &encounterorch.SubmitCheckInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    core.PlayerID(playerID),
		EntityID:    req.GetEntityId(),
		Roll:        int(req.GetRoll()),
	})
	if err != nil {
		return nil, submitCheckStatusError(err)
	}

	return &encounterv2pb.SubmitCheckResponse{
		Success: out.Success,
		Total:   int32(out.Total), //nolint:gosec // total is roll(1-20)+small mods, fits int32
	}, nil
}

// submitCheckStatusError maps the orchestrator's SubmitCheck errors onto gRPC
// status codes per pat-v2-status-code-mapping. Orchestrator load sentinels and
// the toolkit verb sentinels (surfaced unwrapped by the orchestrator) carry the
// classification; this proto-layer mapper assigns the code.
//
//   - ErrEncounterNotFound → NotFound
//   - ErrPlayerNotInEncounter / ErrEntityOwnershipMismatch → PermissionDenied
//   - ErrNoPendingPrompt / ErrPromptKindMismatch → FailedPrecondition
//   - ErrInvalidRoll → InvalidArgument (range disagreement vs the handler bound)
//   - ErrUnsupportedPromptAction → Unimplemented
//   - ErrOutOfRange → FailedPrecondition (rpg-api#747: toolkit#864's gate-review
//     blocker 2 — the actor walked out of reach between AttemptUnlock and
//     SubmitCheck, so the successful check's dispatch was refused; state-
//     dependent, not the system-shaped ErrSubmitCheckDispatch bucket below)
//   - ErrNoCharacterResolver / ErrSubmitCheckDispatch / load+save failures →
//     Internal
func submitCheckStatusError(err error) error {
	switch {
	case errors.Is(err, encounterorch.ErrEncounterNotFound):
		return status.Error(codes.NotFound, "encounter not found")
	case errors.Is(err, encounterorch.ErrPlayerNotInEncounter):
		return status.Error(codes.PermissionDenied, "player is not in this encounter")
	case errors.Is(err, encounterorch.ErrEntityOwnershipMismatch):
		return status.Error(codes.PermissionDenied,
			"entity_id does not match player's controlled entity")
	case errors.Is(err, encounter.ErrNoPendingPrompt):
		return status.Error(codes.FailedPrecondition,
			"no pending prompt to resolve for caller")
	case errors.Is(err, encounter.ErrPromptKindMismatch):
		return status.Error(codes.FailedPrecondition,
			"pending prompt is not a skill check")
	case errors.Is(err, encounter.ErrInvalidRoll):
		// Defense in depth: the handler-side bounds check should have caught
		// this. Reaching it means the toolkit's range disagrees with ours —
		// still InvalidArgument from the client's perspective.
		return status.Errorf(codes.InvalidArgument, "roll rejected by toolkit: %v", err)
	case errors.Is(err, encounter.ErrUnsupportedPromptAction):
		return status.Errorf(codes.Unimplemented,
			"prompt action not supported in this server version: %v", err)
	case errors.Is(err, encounter.ErrOutOfRange):
		return status.Errorf(codes.FailedPrecondition, "target out of reach: %v", err)
	case errors.Is(err, encounter.ErrNoCharacterResolver):
		// The handler always wires a resolver (StubCharacterResolver default).
		// Reaching this means the orchestrator rebuilt the encounter without the
		// resolver — surface as Internal.
		return status.Errorf(codes.Internal,
			"encounter has no character resolver wired: %v", err)
	}
	// ErrSubmitCheckDispatch (resolution succeeded, side effect failed) + load /
	// save / unclassified failures.
	return status.Errorf(codes.Internal, "submit check: %v", err)
}
