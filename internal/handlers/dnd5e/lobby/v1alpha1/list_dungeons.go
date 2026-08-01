package lobby

import (
	"context"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// ListDungeons handles the ListDungeons RPC — every dungeon key + display
// name the server can StartEncounter from. NOT gated behind authoring
// (design.md's post-approval correction, plan.md S0/S1): it reads content
// and mutates nothing, so it's wired here on LobbyService rather than the
// new AuthoringService, and works with the authoring gate off.
func (h *Handler) ListDungeons(
	ctx context.Context,
	_ *lobbyv1alpha1.ListDungeonsRequest,
) (*lobbyv1alpha1.ListDungeonsResponse, error) {
	out, err := h.orch.ListDungeons(ctx, &lobbyorch.ListDungeonsInput{})
	if err != nil {
		return nil, lobbyStatusError(err)
	}

	dungeons := make([]*lobbyv1alpha1.DungeonSummary, len(out.Dungeons))
	for i, d := range out.Dungeons {
		dungeons[i] = &lobbyv1alpha1.DungeonSummary{Key: d.Key, Name: d.Name}
	}
	return &lobbyv1alpha1.ListDungeonsResponse{Dungeons: dungeons}, nil
}
