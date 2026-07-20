package lobby

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// AbandonEncounter handles the AbandonEncounter RPC: host-only,
// administratively ends the lobby's STARTED encounter. Clients learn the
// terminal transition from the encounter's own EncounterEnded broadcast on
// StreamEncounter, not from this response — the RPC itself is a lean ack,
// matching SetReady/LeaveLobby.
func (h *Handler) AbandonEncounter(
	ctx context.Context,
	req *lobbyv1alpha1.AbandonEncounterRequest,
) (*lobbyv1alpha1.AbandonEncounterResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetLobbyId() == "" {
		return nil, status.Error(codes.InvalidArgument, "lobby_id is required")
	}

	if _, err := h.orch.AbandonEncounter(ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: playerID,
		LobbyID:  req.GetLobbyId(),
	}); err != nil {
		return nil, lobbyStatusError(err)
	}

	return &lobbyv1alpha1.AbandonEncounterResponse{}, nil
}
