package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

func TestAcceptance_WorldAssetSceneryRefsReachStartedSession(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	dungeon, err := sessionworld.Compile([]byte(dungeonstest.WorldAssetSceneryYAML))
	require.NoError(t, err, "a dungeon placing all four scenery namespaces must compile")

	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "world-asset-scenery-run", Encounter: "world-asset-scenery", World: dungeon.World,
	})
	require.NoError(t, err)

	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "world-asset-scenery-run", Member: "alice", Position: pbAt(0, 0),
	})
	require.NoError(t, err)

	atlas, err := h.handler.GetAtlas(ctx, &sessionpb.GetAtlasRequest{
		Session: "world-asset-scenery-run", Member: "alice",
	})
	require.NoError(t, err)

	refs := make([]string, 0, len(atlas.GetProps()))
	propsByRef := make(map[string]*sessionpb.AtlasProp, len(atlas.GetProps()))
	for _, prop := range atlas.GetProps() {
		refs = append(refs, prop.GetRef())
		propsByRef[prop.GetRef()] = prop
	}
	require.ElementsMatch(t, dungeonstest.WorldAssetSceneryRefs, refs)

	potion := propsByRef[dungeonstest.WorldAssetSceneryRefs[1]]
	require.NotNil(t, potion)
	require.False(t, potion.GetBlocksMovement())
	require.False(t, potion.GetBlocksLineOfSight())

	gate := propsByRef[dungeonstest.WorldAssetSceneryRefs[3]]
	require.NotNil(t, gate)
	require.True(t, gate.GetBlocksMovement())
	require.True(t, gate.GetBlocksLineOfSight())
}
