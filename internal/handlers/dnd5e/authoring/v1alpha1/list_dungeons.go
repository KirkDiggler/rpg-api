package authoringv1alpha1

import (
	"context"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	lobbyhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/lobby/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// ListDungeons is the studio's library for the caller: their world's
// encounters (source WORLD, owner set) by key, then the single-room
// references (source SHIPPED) by key, in the lobby list's one wire form.
func (h *Handler) ListDungeons(
	ctx context.Context,
	_ *authoringpb.ListDungeonsRequest,
) (*authoringpb.ListDungeonsResponse, error) {
	caller, err := callerOf(ctx)
	if err != nil {
		return nil, err
	}

	out, err := h.orch.ListDungeons(ctx, &authoringorch.ListDungeonsInput{Caller: caller})
	if err != nil {
		return nil, statusError(err)
	}

	resp := &authoringpb.ListDungeonsResponse{
		Dungeons: make([]*lobbyv1alpha1.DungeonSummary, 0, len(out.Dungeons)),
	}
	for _, d := range out.Dungeons {
		resp.Dungeons = append(resp.Dungeons, lobbyhandler.DungeonSummaryToProto(d))
	}

	return resp, nil
}
