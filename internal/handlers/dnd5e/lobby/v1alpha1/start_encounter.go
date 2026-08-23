package lobby

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// StartEncounter handles the StartEncounter RPC: host-only, all-ready gated.
// This is now the SOLE path an encounter comes into existence through — see
// lobbyorch.StartEncounter's doc for the persist-then-emit ordering that
// makes EncounterStarted safe for clients to act on.
func (h *Handler) StartEncounter(
	ctx context.Context,
	req *lobbyv1alpha1.StartEncounterRequest,
) (*lobbyv1alpha1.StartEncounterResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetLobbyId() == "" {
		return nil, status.Error(codes.InvalidArgument, "lobby_id is required")
	}

	out, err := h.orch.StartEncounter(ctx, &lobbyorch.StartEncounterInput{
		PlayerID: playerID,
		LobbyID:  req.GetLobbyId(),
		// DungeonKey selects which registered dungeon to build from
		// (rpg-api#806). Empty means the reference tomb; a key the registry
		// does not have is NotFound, never a silent fallback.
		DungeonKey: lobbyorch.DungeonKey(req.GetDungeonKey()),
	})
	if err != nil {
		return nil, lobbyStatusError(err)
	}

	return &lobbyv1alpha1.StartEncounterResponse{EncounterId: out.EncounterID}, nil
}
