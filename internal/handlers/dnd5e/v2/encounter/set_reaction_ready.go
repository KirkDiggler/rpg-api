package encounter

// SetReactionReady RPC handler.
//
// Toggles per-character readiness for a specific reaction. Players opt in
// to spell-cost reactions (Shield) by setting ready=true; conditions only
// publish reaction triggers when the readiness flag is set, so without
// this RPC no Shield prompts ever fire.
//
// Free-cost reactions (OA) are seeded ready=true at AddPlayer time
// (encounter SDK Wave 2.11c); players use this RPC to override the default
// (e.g. "hold OA this turn").
//
// The v2 orchestrator owns the load → SetReactionReady verb → persist flow
// (#582 step 3). This handler stays thin: proto↔input mapping plus
// setReactionReadyStatusError for the sentinel→gRPC code mapping.

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// SetReactionReady handles the SetReactionReady RPC: validates the request
// envelope, delegates the readiness toggle to the v2 orchestrator, and maps
// the orchestrator's sentinel errors back to gRPC status codes.
//
// Behavior contract (preserved across the carve):
//   - missing auth → Unauthenticated
//   - empty encounter_id / character_id / reaction_ref → InvalidArgument
//   - encounter not in repo → NotFound
//   - caller is not a member of the encounter → PermissionDenied
//   - character_id does not match caller's controlled entity → PermissionDenied
//   - SDK rejects the entity (not in encounter) → FailedPrecondition
//   - save failure → Internal
//
// On success returns an empty SetReactionReadyResponse — clients update
// their readiness panel from the next encounter snapshot or readiness-changed
// event (future).
func (h *Handler) SetReactionReady(
	ctx context.Context,
	req *encounterv2pb.SetReactionReadyRequest,
) (*encounterv2pb.SetReactionReadyResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetCharacterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}
	if req.GetReactionRef() == nil ||
		req.GetReactionRef().GetModule() == "" ||
		req.GetReactionRef().GetType() == "" ||
		req.GetReactionRef().GetId() == "" {
		return nil, status.Error(codes.InvalidArgument,
			"reaction_ref must have module, type, and id set")
	}

	_, err := h.orch.SetReactionReady(ctx, &encounterorch.SetReactionReadyInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    core.PlayerID(playerID),
		EntityID:    core.EntityID(req.GetCharacterId()),
		ReactionRef: refProtoToString(req.GetReactionRef()),
		Ready:       req.GetReady(),
	})
	if err != nil {
		return nil, setReactionReadyStatusError(err)
	}

	return &encounterv2pb.SetReactionReadyResponse{}, nil
}

// setReactionReadyStatusError maps the orchestrator's SetReactionReady errors
// onto gRPC status codes per pat-v2-status-code-mapping. Orchestrator load
// sentinels carry the auth classification; the ErrSetReactionReadyRefused
// wrapper carries the state-dependent verb refusal.
//
//   - ErrEncounterNotFound → NotFound
//   - ErrPlayerNotInEncounter / ErrEntityOwnershipMismatch → PermissionDenied
//   - ErrSetReactionReadyRefused (SDK rejects the entity) → FailedPrecondition
//   - load / save / unclassified failures → Internal
func setReactionReadyStatusError(err error) error {
	switch {
	case errors.Is(err, encounterorch.ErrEncounterNotFound):
		return status.Error(codes.NotFound, "encounter not found")
	case errors.Is(err, encounterorch.ErrPlayerNotInEncounter):
		return status.Error(codes.PermissionDenied, "player is not in this encounter")
	case errors.Is(err, encounterorch.ErrEntityOwnershipMismatch):
		return status.Error(codes.PermissionDenied,
			"character_id does not match player's controlled entity")
	case errors.Is(err, encounterorch.ErrSetReactionReadyRefused):
		return status.Errorf(codes.FailedPrecondition, "%v", err)
	}
	return status.Errorf(codes.Internal, "set reaction ready: %v", err)
}

// refProtoToString joins a proto Ref into the canonical "module:type:id"
// string the toolkit uses as the readiness-map key.
func refProtoToString(r *encounterv2pb.Ref) string {
	return r.GetModule() + ":" + r.GetType() + ":" + r.GetId()
}
