package lobby

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// ListDungeons handles the ListDungeons RPC: every dungeon key + display
// name StartEncounter can build, from the content registry. NOT behind the
// authoring gate -- it reads content and mutates nothing, and the lobby's
// picker needs it with authoring off (rpg-api#806, rpg-project#131).
func (h *Handler) ListDungeons(
	ctx context.Context,
	_ *lobbyv1alpha1.ListDungeonsRequest,
) (*lobbyv1alpha1.ListDungeonsResponse, error) {
	if auth.GetPlayerID(ctx) == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}

	out, err := h.orch.ListDungeons(ctx, &lobbyorch.ListDungeonsInput{})
	if err != nil {
		return nil, lobbyStatusError(err)
	}

	resp := &lobbyv1alpha1.ListDungeonsResponse{
		Dungeons: make([]*lobbyv1alpha1.DungeonSummary, 0, len(out.Dungeons)),
	}
	for _, d := range out.Dungeons {
		resp.Dungeons = append(resp.Dungeons, &lobbyv1alpha1.DungeonSummary{Key: d.Key, Name: d.Name})
	}

	return resp, nil
}
