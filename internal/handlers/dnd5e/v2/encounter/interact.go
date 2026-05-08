package encounter

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	"github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// Interact handles the Interact RPC. Wave 2.7 wires only door interactions:
// when target_entity_id matches a known door, the toolkit's
// Encounter.OpenDoor() verb is dispatched. The toolkit publishes the cause
// event (DoorOpenedEvent) and the parallel effect event (HexRevealedEvent)
// to the broker for delivery on StreamEncounter.
//
// Future waves extend the dispatch to chests, levers, NPCs, and traps; the
// optional interaction_kind field is plumbed through the proto for that
// future routing but is unused today.
//
// Behavior contract:
//   - missing auth → Unauthenticated
//   - empty encounter_id / target_entity_id → InvalidArgument
//   - encounter not in repo → NotFound
//   - target is not a known door in this encounter → NotFound
//   - toolkit OpenDoor refuses (player not in encounter, door already open,
//     etc.) → FailedPrecondition (per pat-v2-status-code-mapping)
//   - save failure → Internal
//
// The empty InteractResponse is by proto design — door world changes flow as
// events on StreamEncounter, not as response payload. Future locked-door
// flow (Wave 2.10) will populate InputRequired for skill-check prompts.
func (h *Handler) Interact(ctx context.Context, req *encounterv2pb.InteractRequest) (*encounterv2pb.InteractResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetTargetEntityId() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_entity_id is required")
	}

	data, err := h.encRepo.Get(ctx, req.GetEncounterId())
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal, "load encounter %q: %v", req.GetEncounterId(), err)
	}

	// Wave 2.7 dispatch: only door interactions are wired. Future waves add
	// chests, levers, NPCs, traps via additional lookups + dispatch arms.
	targetID := core.EntityID(req.GetTargetEntityId())
	if _, ok := data.Doors[targetID]; !ok {
		return nil, status.Error(codes.NotFound, "target entity is not a door, or door does not exist")
	}

	enc, err := encounter.LoadFromData(data, h.broker)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load from data %q: %v", req.GetEncounterId(), err)
	}

	if err := enc.OpenDoor(core.PlayerID(playerID), targetID); err != nil {
		// Toolkit OpenDoor errors are state-dependent (player not in
		// encounter, door already open) — these are gRPC FailedPrecondition
		// per pat-v2-status-code-mapping. The request is syntactically
		// valid but the world state forbids the action.
		return nil, status.Errorf(codes.FailedPrecondition, "open door: %v", err)
	}

	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter %q: %v", req.GetEncounterId(), err)
	}

	return &encounterv2pb.InteractResponse{}, nil
}
