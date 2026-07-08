package lobby

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// CreateLobby handles the CreateLobby RPC: validates the request envelope,
// delegates lobby creation to the orchestrator, and maps sentinel errors to
// gRPC status codes.
func (h *Handler) CreateLobby(
	ctx context.Context,
	req *lobbyv1alpha1.CreateLobbyRequest,
) (*lobbyv1alpha1.CreateLobbyResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetCampaignId() == "" {
		return nil, status.Error(codes.InvalidArgument, "campaign_id is required")
	}
	if req.GetCharacterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}

	out, err := h.orch.CreateLobby(ctx, &lobbyorch.CreateLobbyInput{
		PlayerID:    playerID,
		CampaignID:  req.GetCampaignId(),
		CharacterID: req.GetCharacterId(),
	})
	if err != nil {
		return nil, lobbyStatusError(err)
	}

	return &lobbyv1alpha1.CreateLobbyResponse{
		LobbyId:      out.LobbyID,
		JoinRef:      out.JoinRef,
		HostPlayerId: out.HostPlayerID,
	}, nil
}
