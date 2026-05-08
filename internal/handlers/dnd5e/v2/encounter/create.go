package encounter

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// CreateEncounter handles the CreateEncounter RPC. It validates auth and
// request fields, constructs a fresh encounter with the caller as the sole
// player, persists it, and returns the proto Encounter projected for the
// caller's view.
func (h *Handler) CreateEncounter(ctx context.Context, req *encounterv2pb.CreateEncounterRequest) (*encounterv2pb.CreateEncounterResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetCampaignId() == "" {
		return nil, status.Error(codes.InvalidArgument, "campaign_id is required")
	}

	encID := core.EncounterID(uuid.NewString())
	enc := tkenc.New(encID, h.broker)
	// The creator is automatically added as the encounter's first player.
	// Entity ID mirrors the player ID for the initial seat; future PRs can
	// wire in a character-selection step that replaces this with the
	// player's chosen character ID.
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: core.PlayerID(playerID),
		EntityID: core.EntityID(playerID),
		Position: core.Hex{Q: 0, R: 0, S: 0},
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "add creator to encounter: %v", err)
	}

	data := enc.ToData()
	if err := h.encRepo.Save(ctx, data); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter: %v", err)
	}

	pbEncounter, err := ProjectFor(data, core.PlayerID(playerID), h.broker, h.now())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "project encounter: %v", err)
	}

	return &encounterv2pb.CreateEncounterResponse{
		Encounter: pbEncounter,
	}, nil
}
