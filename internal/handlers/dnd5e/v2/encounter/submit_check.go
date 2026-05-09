package encounter

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// d20 roll bounds — defense-in-depth alongside the toolkit's
// ErrInvalidRoll, so we surface InvalidArgument before reaching the verb.
const (
	minD20Roll = 1
	maxD20Roll = 20
)

// SubmitCheck handles the SubmitCheck RPC: it resolves the caller's currently
// pending InputRequired prompt against a supplied d20 roll. The toolkit owns
// modifier resolution (via the wired CharacterResolver) and any side-effect
// dispatch on success — for Wave 2.9 the only wired action is "open" on a
// locked door, which the toolkit dispatches to OpenDoor internally; that
// publishes DoorOpened + GeometryRevealed through the broker the same way
// Wave 2.7 did.
//
// Behavior contract:
//   - missing auth → Unauthenticated
//   - empty encounter_id / entity_id → InvalidArgument
//   - roll outside [1, 20] → InvalidArgument (defense in depth before
//     the toolkit also checks; surfaces a clearer error to the client)
//   - encounter not in repo → NotFound
//   - caller is not a member of the encounter → PermissionDenied
//   - entity_id does not match caller's controlled entity →
//     PermissionDenied (mirrors MoveEntity / EndTurn / TakeAction)
//   - no pending prompt → FailedPrecondition
//   - prompt is not a skill check → FailedPrecondition (other prompt
//     kinds resolve via different verbs in future waves)
//   - prompt's TriggeredAction is not wired in this toolkit version →
//     Unimplemented
//   - encounter has no character resolver → Internal (the orchestrator
//     misconfigured itself; the resolver is wired by HandlerConfig)
//   - save failure → Internal
//
// On success the response carries the resolved total (roll + ability
// modifier + tool-proficiency bonus) and a boolean indicating whether
// total >= prompt DC. World effects flow as broker events on the existing
// StreamEncounter session — the response itself does not include them.
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
	if req.GetRoll() < minD20Roll || req.GetRoll() > maxD20Roll {
		return nil, status.Errorf(codes.InvalidArgument,
			"roll %d is outside the d20 range [%d, %d]",
			req.GetRoll(), minD20Roll, maxD20Roll)
	}

	data, err := h.encRepo.Get(ctx, req.GetEncounterId())
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal,
			"load encounter %q: %v", req.GetEncounterId(), err)
	}

	// Authorization: the caller must be a member of this encounter and the
	// supplied entity_id must match their controlled entity. Mirrors the
	// pattern used by MoveEntity / EndTurn / TakeAction so a caller cannot
	// resolve a prompt that doesn't belong to them (defense in depth — the
	// toolkit already keys PendingPrompts by playerID, so a non-member would
	// hit ErrNoPendingPrompt anyway, but PermissionDenied is the clearer
	// signal and prevents leaking encounter membership through the FailedPrecondition
	// vs PermissionDenied distinction).
	pd, ok := data.Players[core.PlayerID(playerID)]
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "player is not in this encounter")
	}
	if string(pd.EntityID) != req.GetEntityId() {
		return nil, status.Error(codes.PermissionDenied,
			"entity_id does not match player's controlled entity")
	}

	enc, err := tkenc.LoadFromData(data, h.broker, tkenc.WithCharacterResolver(h.resolver))
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"load from data %q: %v", req.GetEncounterId(), err)
	}

	result, submitErr := enc.SubmitCheck(core.PlayerID(playerID), int(req.GetRoll()))
	if submitErr != nil {
		switch {
		case errors.Is(submitErr, tkenc.ErrNoPendingPrompt):
			return nil, status.Error(codes.FailedPrecondition,
				"no pending prompt to resolve for caller")
		case errors.Is(submitErr, tkenc.ErrPromptKindMismatch):
			return nil, status.Error(codes.FailedPrecondition,
				"pending prompt is not a skill check")
		case errors.Is(submitErr, tkenc.ErrInvalidRoll):
			// Defense in depth: handler-side bounds check above should have
			// caught this. If it didn't, the toolkit's range disagrees with
			// ours — still InvalidArgument from the client's perspective.
			return nil, status.Errorf(codes.InvalidArgument,
				"roll rejected by toolkit: %v", submitErr)
		case errors.Is(submitErr, tkenc.ErrUnsupportedPromptAction):
			return nil, status.Errorf(codes.Unimplemented,
				"prompt action not supported in this server version: %v", submitErr)
		case errors.Is(submitErr, tkenc.ErrNoCharacterResolver):
			// The handler always wires a resolver (StubCharacterResolver as
			// default). Reaching this means the orchestrator rebuilt the
			// encounter without going through HandlerConfig — surface as
			// Internal.
			return nil, status.Errorf(codes.Internal,
				"encounter has no character resolver wired: %v", submitErr)
		default:
			// Resolution-proceeded-but-dispatch-failed cases (downstream
			// OpenDoor failures) wrap with fmt.Errorf rather than sentinels.
			// The check itself succeeded (Success=true is reported in result)
			// but the side effect failed; the prompt has been cleared by the
			// toolkit. Surface as Internal so the client knows the world
			// state may be inconsistent.
			return nil, status.Errorf(codes.Internal,
				"submit check dispatch: %v", submitErr)
		}
	}

	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal,
			"save encounter %q after submit check: %v", req.GetEncounterId(), err)
	}

	return &encounterv2pb.SubmitCheckResponse{
		Success: result.Success,
		Total:   int32(result.Total), //nolint:gosec // total is roll(1-20)+small mods, fits int32
	}, nil
}
