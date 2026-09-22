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

// wireSegment is one atlas segment as the proto carries it, comparable so a
// test can ask set questions of a repeated field.
type wireSegment struct {
	fromQ, fromR, toQ, toR float64
	height                 float32
}

func segmentSet(ss []*sessionpb.AtlasSegment) map[wireSegment]bool {
	out := make(map[wireSegment]bool, len(ss))
	for _, s := range ss {
		out[wireSegment{
			fromQ: s.GetFrom().GetQ(), fromR: s.GetFrom().GetR(),
			toQ: s.GetTo().GetQ(), toR: s.GetTo().GetR(),
			height: s.GetHeight(),
		}] = true
	}

	return out
}

func unionOfSegments(a, b map[wireSegment]bool) map[wireSegment]bool {
	out := make(map[wireSegment]bool, len(a)+len(b))
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}

	return out
}

// without returns the cells of a that are not in b.
func (c cellSet) without(b cellSet) cellSet {
	out := make(cellSet, len(c))
	for k := range c {
		if !b[k] {
			out[k] = true
		}
	}

	return out
}

func (c cellSet) union(b cellSet) cellSet {
	out := make(cellSet, len(c)+len(b))
	for k := range c {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}

	return out
}

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
// OPENING IS THE CAUSE, not crossing. The composition reveals a concealment
// to whoever perceives one of its doors open, which is one of the causes it
// recognizes (the others being standing inside its cells, walking through its
// door, and intel that names it), and it is the one that fires first here.
// Measured rather than assumed: with the open removed and only the search
// left, no reveal arrives at all.
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

	// ONE BEAT FOR ONE SECRET (design rpg-project#490, E4). What arrived as a
	// region reveal, with the door's own reveal beside it, is one
	// concealment_revealed now: the room, its floor, its walls and the door
	// that guarded it, all on the beat that names the secret.
	var revealed *sessionpb.ConcealmentRevealed
	for _, e := range story.GetEntries() {
		if e.GetKind() == sessionpb.EventKind_EVENT_KIND_CONCEALMENT_REVEALED {
			revealed = e.GetConcealmentRevealed()
		}
	}
	require.NotNil(t, revealed, "opening the hidden door reveals the secret it guarded")
	require.Equal(t, "concealed-vault/vault", revealed.GetConcealment(),
		"the compiled id of the concealment the v2 lowering built from the concealed region")

	// The room arrives WHOLE, as the region entry a refetch would carry --
	// and the door arrives with it, which is the whole difference from the
	// two beats this replaced.
	var vaultRegion *sessionpb.AtlasRegion
	for _, r := range revealed.GetRegions() {
		if r.GetId() == "vault" {
			vaultRegion = r
		}
	}
	require.NotNil(t, vaultRegion, "the touched region rides the reveal")
	require.Equal(t, []string{"concealed-vault/vault-door"}, doorIDsOf(revealed.GetDoors()),
		"the door it hid, on the same beat, in the shape GetDoors answers in")
	require.Equal(t, []string{"concealed-vault/vault-door"}, doorwayConnectionsOf(revealed.GetDoorways()),
		"and its crossing, for the cached atlas")

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
		setOfPositions(vaultRegion.GetCells()),
		"and the beat said the same twelve, which is what lets a client patch instead of refetch")
	require.Equal(t, offsetCells(dungeonstest.ConcealedVaultVaultCells),
		setOfPositions(revealed.GetCells()),
		"the concealment's own floor list says the same twelve, so the patch cannot disagree with itself")

	// SEGMENTS ADD. The beat hands over the walls she did not have, nothing
	// ever leaves, and a client that appends them to its cache lands exactly
	// where a refetch would have put it. The beat must carry at least one, or
	// a revealed room draws with no walls at all -- which is precisely what
	// happens to a client that has deleted its boundary fitter.
	beforeSegments := segmentSet(before.GetSegments())
	eventSegments := segmentSet(revealed.GetSegments())
	require.NotEmpty(t, eventSegments, "the beat has to hand over the room's walls")
	require.Equal(t, unionOfSegments(beforeSegments, eventSegments),
		segmentSet(after.GetSegments()),
		"segments after a reveal are segments before it plus the beat's, exactly")

	// SEALED REPLACES, WITHIN THE ROOM, and cells LEAVE. Before the reveal her
	// sealed list holds the footing the projection gave that wall: floor it
	// stands on that belongs to a room she cannot see reaches her as ownerless,
	// and ownerless floor is floor nobody stands on. The moment the vault is
	// hers those same cells are ordinary vault floor. A client that appended
	// the beat's sealed list instead of swapping the room's would leave a room
	// it can see permanently unwalkable at its edges.
	vault := offsetCells(dungeonstest.ConcealedVaultVaultCells)
	beforeSealed := setOfPositions(before.GetSealed())
	afterSealed := setOfPositions(after.GetSealed())
	require.Equal(t, beforeSealed.without(vault).union(setOfPositions(revealed.GetSealed())),
		afterSealed,
		"sealed after a reveal is sealed before it, less the revealed room's cells, plus the beat's")
	require.NotEmpty(t, beforeSealed.without(afterSealed),
		"cells have to LEAVE the sealed list, or this assertion would pass on an append too")
}

// doorIDsOf and doorwayConnectionsOf read a reveal's door and doorway lists by
// the name each entry carries, so the assertion pins WHICH doors arrived and
// in what order rather than how many -- a count would pass on the wrong door.
func doorIDsOf(ds []*sessionpb.DoorInfo) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.GetDoor()
	}

	return out
}

func doorwayConnectionsOf(ds []*sessionpb.AtlasDoorway) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.GetConnection()
	}

	return out
}
