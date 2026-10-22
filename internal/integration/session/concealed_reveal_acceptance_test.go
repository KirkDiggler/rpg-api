package session_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
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
// test can ask set questions of a repeated field. The v1 atlas carries the
// height as a float32 and the act stream as a float64; the key holds the
// float32 both agree on.
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

// actSegmentSet is segmentSet over the act stream's segments.
func actSegmentSet(ss []*storypb.AtlasSegment) map[wireSegment]bool {
	out := make(map[wireSegment]bool, len(ss))
	for _, s := range ss {
		out[wireSegment{
			fromQ: s.GetFrom().GetQ(), fromR: s.GetFrom().GetR(),
			toQ: s.GetTo().GetQ(), toR: s.GetTo().GetR(),
			height: float32(s.GetHeight()),
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

// setOfActPositions is setOfPositions over the act stream's cells.
func setOfActPositions(ps []*storypb.Position) cellSet {
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
// Alice walks the hall, finds the door, and opens it. The room arrives as a
// beat on her own stream, and her next GetAtlas agrees with what the beat
// said. That agreement is the whole point: the event and the atlas are two
// views of one projection, and a client that patches its cache from the beat
// must end up where a refetch would have put it.
//
// OPENING IS THE CAUSE, not crossing. The composition reveals a concealment
// to whoever perceives one of its doors open, which is one of the causes it
// recognizes (the others being standing inside its cells, walking through its
// door, and intel that names it), and it is the one that fires first here.
// Measured rather than assumed: with the open removed and only the walk
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

	// Alice starts at the far end of the hall, three cells from the cell the
	// door's crossing touches.
	hallSide := dungeonstest.ConcealedVaultDoorCrossing[0]
	h.launch(t, "reveal-run", dungeon.Spec, seatAt("alice", 0, hallSide[1]))

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

	// Walk the hall to the cell beside the door. Discovery is automatic: the
	// move itself carries her within one hex of the concealed door, and the
	// roller this harness supplies answers the top face, so the perception
	// check clears DC 15 every run rather than one in three.
	var path []*sessionpb.Position
	for col := 1; col <= hallSide[0]; col++ {
		path = append(path, pbAt(col, hallSide[1]))
	}
	_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "reveal-run", Member: "alice", Path: path,
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

	// The reveal, from her own StoryService story rather than a live stream:
	// live is the story's tail act for act, and reading the story keeps this
	// test free of subscription timing.
	story := storyOf(ctx, t, h.story, "reveal-run", "alice")

	// ONE BEAT FOR ONE SECRET (design rpg-project#490, E4). What arrived as a
	// region reveal, with the door's own reveal beside it, is one
	// concealment_revealed now: the room, its floor, its walls and the door
	// that guarded it, all on the beat that names the secret.
	var revealed *storypb.SecretRevealed
	for _, a := range story {
		if a.GetPerception().GetSense() == storypb.Sense_SENSE_REVEAL {
			require.Nil(t, revealed, "one reveal for one secret")
			require.Equal(t, "alice", a.GetPerception().GetObserver().GetId(), "and it is hers")
			revealed = a.GetPerception().GetRevealed()
		}
	}
	require.NotNil(t, revealed, "opening the hidden door reveals the secret it guarded")
	require.Equal(t, "concealed-vault/vault", revealed.GetConcealment(),
		"the compiled id of the concealment the v2 lowering built from the concealed region")

	// The room arrives WHOLE, as the region entry a refetch would carry --
	// and the door arrives with it, which is the whole difference from the
	// two beats this replaced.
	var vaultRegion *storypb.AtlasRegion
	for _, r := range revealed.GetRegions() {
		if r.GetId() == "vault" {
			vaultRegion = r
		}
	}
	require.NotNil(t, vaultRegion, "the touched region rides the reveal")
	require.Equal(t, []string{"concealed-vault/vault-door"}, doorIDsOf(revealed.GetDoors()),
		"the door it hid, on the same beat, in the shape GetDoors answers in")
	require.Len(t, revealed.GetDoors()[0].GetDoorways(), 1,
		"and its crossing rides on the door, for the cached atlas")

	after, err := h.handler.GetAtlas(ctx, &sessionpb.GetAtlasRequest{
		Session: "reveal-run", Member: "alice",
	})
	require.NoError(t, err)

	// The crossing the reveal carried under the door is the crossing a refetch
	// serves for that door, end for end, and the act's nested doorway names
	// its door as the v1 beat's `connection` did (rpg-project#643 E).
	var atlasCrossings []string
	for _, d := range after.GetDoorways() {
		if d.GetConnection() == "concealed-vault/vault-door" {
			atlasCrossings = append(atlasCrossings, crossingOf(d.GetFrom().GetX(), d.GetFrom().GetY(), d.GetTo().GetX(), d.GetTo().GetY()))
		}
	}
	require.NotEmpty(t, atlasCrossings, "the refetched atlas serves the vault door's crossing")
	require.Equal(t, atlasCrossings, revealedCrossingsOf(revealed.GetDoors()),
		"the reveal's crossing is the atlas's crossing")
	for _, door := range revealed.GetDoors() {
		for _, doorway := range door.GetDoorways() {
			require.Equal(t, "concealed-vault/vault-door", doorway.GetDoor(), "the revealed doorway names its door")
		}
	}

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
		setOfActPositions(vaultRegion.GetCells()),
		"and the beat said the same twelve, which is what lets a client patch instead of refetch")
	require.Equal(t, offsetCells(dungeonstest.ConcealedVaultVaultCells),
		setOfActPositions(revealed.GetCells()),
		"the concealment's own floor list says the same twelve, so the patch cannot disagree with itself")

	// SEGMENTS ADD. The beat hands over the walls she did not have, nothing
	// ever leaves, and a client that appends them to its cache lands exactly
	// where a refetch would have put it. The beat must carry at least one, or
	// a revealed room draws with no walls at all -- which is precisely what
	// happens to a client that has deleted its boundary fitter.
	beforeSegments := segmentSet(before.GetSegments())
	eventSegments := actSegmentSet(revealed.GetSegments())
	require.NotEmpty(t, eventSegments, "the beat has to hand over the room's walls")
	require.Equal(t, unionOfSegments(beforeSegments, eventSegments),
		segmentSet(after.GetSegments()),
		"segments after a reveal are segments before it plus the beat's, exactly")

	// SEALED AFTER THE REVEAL. Sealed after it is sealed before it, less the
	// revealed room's cells, plus the beat's. Under fog of war R9 a presented
	// wall teaches no footing hex and footing under a concealed room stays
	// withheld, so before the reveal her sealed list holds none of the vault.
	// With the before-sealed list and the vault disjoint, this room no longer
	// tells a replace from an append: both give the same answer here. No
	// failing case pins replace over append today; follow-up rpg-toolkit#2062
	// adds one in the session module.
	vault := offsetCells(dungeonstest.ConcealedVaultVaultCells)
	beforeSealed := setOfPositions(before.GetSealed())
	afterSealed := setOfPositions(after.GetSealed())
	require.Equal(t, beforeSealed.without(vault).union(setOfActPositions(revealed.GetSealed())),
		afterSealed,
		"sealed after a reveal is sealed before it, less the revealed room's cells, plus the beat's")
	// The vault's footing never arrived before the reveal (fog of war R9).
	require.Equal(t, beforeSealed, beforeSealed.without(vault),
		"the vault's footing is withheld until sight reaches it (fog of war R9)")
}

// doorIDsOf reads a reveal's door list by the name each entry carries, so the
// assertion pins WHICH doors arrived and in what order rather than how many --
// a count would pass on the wrong door.
func doorIDsOf(ds []*storypb.RevealedDoor) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.GetDoor()
	}

	return out
}

// revealedCrossingsOf is every crossing the revealed doors carry, in order:
// the act stream carries each door's doorways on the door.
func revealedCrossingsOf(ds []*storypb.RevealedDoor) []string {
	var out []string
	for _, d := range ds {
		for _, w := range d.GetDoorways() {
			out = append(out, crossingOf(w.GetFrom().GetX(), w.GetFrom().GetY(), w.GetTo().GetX(), w.GetTo().GetY()))
		}
	}

	return out
}

// crossingOf names one doorway by its two cells.
func crossingOf(fromX, fromY, toX, toY float64) string {
	return fmt.Sprintf("(%g,%g)->(%g,%g)", fromX, fromY, toX, toY)
}
