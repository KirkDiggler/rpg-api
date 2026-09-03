package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// cellSet is a set of wire cells, so a test can ask set questions of a
// repeated Position field.
type cellSet map[spatial.Position]bool

func setOfPositions(ps []*sessionpb.Position) cellSet {
	out := make(cellSet, len(ps))
	for _, p := range ps {
		out[spatial.Position{X: p.GetX(), Y: p.GetY()}] = true
	}

	return out
}

func offsetCells(pairs [][2]int) cellSet {
	out := make(cellSet, len(pairs))
	for _, p := range pairs {
		out[at(p[0], p[1])] = true
	}

	return out
}

// TestAcceptance_OpeningAConcealedDoorRevealsTheRoomOnTheWire is wall
// geometry's reveal half (rpg-project#360, rpg-api#899) driven the way a
// player drives it: a real session on an authored dungeon, a hall, a vault
// nobody can see, one line between them with a hidden door standing in it.
//
// Alice searches the hall, finds the door, and opens it. The room arrives as a
// beat on her own stream, and her next GetAtlas agrees with what the beat
// said. That agreement is the whole point: the event and the atlas are two
// views of one projection, and a client that patches its cache from the beat
// must end up where a refetch would have put it.
//
// OPENING IS THE CAUSE, not crossing. The composition reveals a region to
// whoever perceives its door open, which is one of three causes it recognizes
// (the others being standing inside the room and walking through its door),
// and it is the one that fires first here. Measured rather than assumed: with
// the open removed and only the search left, no reveal arrives at all.
func TestAcceptance_OpeningAConcealedDoorRevealsTheRoomOnTheWire(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	// The dungeon is compiled from authored YAML rather than built by hand,
	// because the authored LINE is what produces the segments under test. A
	// fixture that declared crossings directly would carry no line at all.
	dungeon, err := sessionworld.Compile([]byte(dungeonstest.ConcealedVaultYAML))
	require.NoError(t, err, "the concealed-vault fixture must compile")

	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "reveal-run", Encounter: "vault-encounter", World: dungeon.World,
	})
	require.NoError(t, err)

	// Alice stands in the hall, on the cell the door's crossing touches.
	hallSide := dungeonstest.ConcealedVaultDoorCrossing[0]
	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "reveal-run", Member: "alice", Position: pbAt(hallSide[0], hallSide[1]),
	})
	require.NoError(t, err)

	before, err := h.handler.GetAtlas(ctx, &sessionpb.GetAtlasRequest{
		Session: "reveal-run", Member: "alice",
	})
	require.NoError(t, err)

	// A non-knower is served the wall and NOT the doorway: the map has to look
	// like an honest dead end, or its shape answers the question a search asks.
	require.Empty(t, before.GetDoorways(), "a hidden door is not on a non-knower's map")
	beforeFloor := setOfPositions(before.GetCells())
	for _, c := range dungeonstest.ConcealedVaultHallCells {
		require.Truef(t, beforeFloor[at(c[0], c[1])], "the hall cell %v is floor from the start", c)
	}

	// Search the hall. The roller this harness supplies answers the top face,
	// so the perception check clears DC 15 every run rather than one in three.
	_, err = h.handler.Search(ctx, &sessionpb.SearchRequest{
		Session: "reveal-run", Member: "alice", Region: "hall",
	})
	require.NoError(t, err)

	found, err := h.handler.GetAtlas(ctx, &sessionpb.GetAtlasRequest{
		Session: "reveal-run", Member: "alice",
	})
	require.NoError(t, err)
	require.Len(t, found.GetDoorways(), 1, "finding the door puts its gap on her map")

	// Open it. A hidden door standing open is a room seen into, and that is
	// the cause the reveal rides on.
	_, err = h.handler.OpenDoor(ctx, &sessionpb.OpenDoorRequest{
		Session: "reveal-run", Member: "alice", Door: "concealed-vault/vault-door",
	})
	require.NoError(t, err)

	// The beat, from her own story rather than a live stream: catch-up and
	// live are byte-equal for the same seq, and reading the story keeps this
	// test free of subscription timing.
	story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{
		Session: "reveal-run", Member: "alice",
	})
	require.NoError(t, err)

	var revealed *sessionpb.RegionRevealed
	for _, e := range story.GetEntries() {
		if e.GetKind() == sessionpb.EventKind_EVENT_KIND_REGION_REVEALED {
			revealed = e.GetRegionRevealed()
		}
	}
	require.NotNil(t, revealed, "opening the hidden door reveals the room it guards")
	require.Equal(t, "vault", revealed.GetRegion().GetId())

	after, err := h.handler.GetAtlas(ctx, &sessionpb.GetAtlasRequest{
		Session: "reveal-run", Member: "alice",
	})
	require.NoError(t, err)

	// The room is hers now, cell for cell.
	afterOwned := make(cellSet)
	for _, r := range after.GetRegions() {
		if r.GetId() == "vault" {
			afterOwned = setOfPositions(r.GetCells())
		}
	}
	require.Equal(t, offsetCells(dungeonstest.ConcealedVaultVaultCells), afterOwned,
		"the vault's own cells, all twelve of them")
	require.Equal(t, offsetCells(dungeonstest.ConcealedVaultVaultCells),
		setOfPositions(revealed.GetRegion().GetCells()),
		"and the beat said the same twelve, which is what lets a client patch instead of refetch")
}
