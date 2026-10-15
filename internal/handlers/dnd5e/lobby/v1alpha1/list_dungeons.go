package lobby

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

// ListDungeons handles the ListDungeons RPC: for the caller, every encounter
// in their world (source WORLD, owner set), then the single-room references
// (source SHIPPED). NOT behind the authoring gate -- it reads content and
// mutates nothing, and the lobby's picker needs it with authoring off
// (rpg-api#806, rpg-project#131, rpg-project#585).
func (h *Handler) ListDungeons(
	ctx context.Context,
	_ *lobbyv1alpha1.ListDungeonsRequest,
) (*lobbyv1alpha1.ListDungeonsResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	worldID, err := trustedWorld(ctx)
	if err != nil {
		return nil, err
	}

	out, err := h.orch.ListDungeons(ctx, &lobbyorch.ListDungeonsInput{
		Caller: dungeons.Caller{WorldID: worldID, PlayerID: playerID},
	})
	if err != nil {
		return nil, lobbyStatusError(err)
	}

	resp := &lobbyv1alpha1.ListDungeonsResponse{
		Dungeons: make([]*lobbyv1alpha1.DungeonSummary, 0, len(out.Dungeons)),
	}
	for _, d := range out.Dungeons {
		resp.Dungeons = append(resp.Dungeons, DungeonSummaryToProto(d))
	}

	return resp, nil
}

// DungeonSummaryToProto is the one wire form of a dungeon summary, shared by
// the lobby and authoring lists. Ownership rides on owner, not on source.
func DungeonSummaryToProto(d dungeons.Summary) *lobbyv1alpha1.DungeonSummary {
	return &lobbyv1alpha1.DungeonSummary{
		Key:    d.Key,
		Name:   d.Name,
		Source: dungeonSourceToProto(d.Source),
		Owner:  d.Owner,
	}
}

func dungeonSourceToProto(s dungeons.Source) lobbyv1alpha1.DungeonSource {
	switch s {
	case dungeons.SourceWorld:
		return lobbyv1alpha1.DungeonSource_DUNGEON_SOURCE_WORLD
	case dungeons.SourceShipped:
		return lobbyv1alpha1.DungeonSource_DUNGEON_SOURCE_SHIPPED
	default:
		return lobbyv1alpha1.DungeonSource_DUNGEON_SOURCE_UNSPECIFIED
	}
}

// trustedWorld is the world the auth boundary installed; its absence is
// FailedPrecondition, never "shipped only" (design R21).
func trustedWorld(ctx context.Context) (string, error) {
	world, ok := worldcontext.Get(ctx)
	if !ok || world.WorldID == "" {
		return "", status.Error(codes.FailedPrecondition, worldcontext.RequiredMessage)
	}
	return world.WorldID, nil
}
