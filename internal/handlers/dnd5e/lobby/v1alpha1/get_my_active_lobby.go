package lobby

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// GetMyActiveLobby handles the GetMyActiveLobby RPC: resolves the
// authenticated caller's own active lobby (resume-after-refresh,
// rpg-dnd5e-web#444). No request fields — identity comes entirely from the
// authenticated context, matching StreamLobby's pattern.
func (h *Handler) GetMyActiveLobby(
	ctx context.Context,
	_ *lobbyv1alpha1.GetMyActiveLobbyRequest,
) (*lobbyv1alpha1.GetMyActiveLobbyResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}

	out, err := h.orch.GetMyActiveLobby(ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: playerID})
	if err != nil {
		return nil, lobbyStatusError(err)
	}

	return &lobbyv1alpha1.GetMyActiveLobbyResponse{
		LobbyId:     out.LobbyID,
		EncounterId: out.EncounterID,
		LobbyStatus: translateLobbyStatus(out.Status),
	}, nil
}
