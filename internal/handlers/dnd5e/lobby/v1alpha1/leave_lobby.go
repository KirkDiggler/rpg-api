package lobby

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// LeaveLobby handles the LeaveLobby RPC. Pre-start only — see
// lobbyorch.LeaveLobby's doc for the host-migration and lifecycle rules.
func (h *Handler) LeaveLobby(
	ctx context.Context,
	req *lobbyv1alpha1.LeaveLobbyRequest,
) (*lobbyv1alpha1.LeaveLobbyResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetLobbyId() == "" {
		return nil, status.Error(codes.InvalidArgument, "lobby_id is required")
	}

	_, err := h.orch.LeaveLobby(ctx, &lobbyorch.LeaveLobbyInput{
		PlayerID: playerID,
		LobbyID:  req.GetLobbyId(),
	})
	if err != nil {
		return nil, lobbyStatusError(err)
	}

	return &lobbyv1alpha1.LeaveLobbyResponse{}, nil
}
