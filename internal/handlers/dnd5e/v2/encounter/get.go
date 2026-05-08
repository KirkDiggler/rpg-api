package encounter

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// GetEncounter handles the GetEncounter RPC. It validates auth and request
// fields, loads the encounter, enforces membership, and returns the proto
// Encounter projected through the caller's PerceptionView.
func (h *Handler) GetEncounter(ctx context.Context, req *encounterv2pb.GetEncounterRequest) (*encounterv2pb.GetEncounterResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}

	data, err := h.encRepo.Get(ctx, req.GetEncounterId())
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal, "load encounter %q: %v", req.GetEncounterId(), err)
	}

	// Authority check: caller must be a member of this encounter.
	// Mirrors the MoveEntity pattern exactly: data.Players[core.PlayerID(playerID)].
	if _, ok := data.Players[core.PlayerID(playerID)]; !ok {
		return nil, status.Error(codes.PermissionDenied, "player is not in this encounter")
	}

	pbEncounter, err := ProjectFor(data, core.PlayerID(playerID), h.broker, h.now())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "project encounter: %v", err)
	}

	return &encounterv2pb.GetEncounterResponse{
		Encounter: pbEncounter,
	}, nil
}
