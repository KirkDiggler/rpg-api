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

// placed_atlas_acceptance_test.go is the wire half of rpg-api-protos#351,
// driven: a v4 room with a holdable footprint, read through the real
// GetAtlas handler, and the placement taken through the real Hold handler.
//
// Everything here is DRIVEN rather than asserted about a struct. The
// rectangle is on the map because the authored document declared it and
// `propBindings` gave it its one order; it leaves the map because somebody
// picked it up.
//
// WHY THE SCENE HAS TO EXIST ALONGSIDE THE CONVERTER TEST. AtlasToProto's own
// tests prove the fields cross a seam. They cannot prove the list a client
// reads is the list the engine answers Hold from, because both sides of that
// claim are the SDK's. Here the second member stands on a cell THE ATLAS
// ITSELF NAMED and picks the thing up — an offer a client would make off the
// wire, and the engine granting it.

const placedTableSession = "placed-table-run"

// placedRun is one session on the placed-table room, with alice on the
// authored party start and bob standing wherever the scene puts him.
type placedRun struct {
	h     *acceptanceHarness
	alice context.Context
	bob   context.Context
	start spatial.Position
}

// startPlacedTableRun compiles the v4 room, starts the session, and seats
// alice on the authored start. Bob joins later, because where he stands is
// read off the atlas rather than chosen here.
func startPlacedTableRun(t *testing.T) *placedRun {
	t.Helper()

	h := newAcceptanceHarness(t)
	for _, who := range []struct{ id, player string }{
		{"alice", "player-alice"}, {"bob", "player-bob"},
	} {
		_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(who.id, who.player)},
		})
		require.NoError(t, err)
	}

	dungeon, err := sessionworld.Compile(dungeonstest.PlacedTableRoomYAML(t))
	require.NoError(t, err, "the placed-table room must compile")
	require.NotEmpty(t, dungeon.PartySeats)

	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: placedTableSession, Encounter: "placed-table-encounter", World: dungeon.World,
	})
	require.NoError(t, err)

	run := &placedRun{
		h:     h,
		alice: auth.WithPlayerID(context.Background(), "player-alice"),
		bob:   auth.WithPlayerID(context.Background(), "player-bob"),
		start: dungeon.PartySeats[0],
	}
	_, err = h.handler.Join(run.alice, &sessionpb.JoinRequest{
		Session: placedTableSession, Member: "alice",
		Position: &sessionpb.Position{X: run.start.X, Y: run.start.Y},
	})
	require.NoError(t, err)

	return run
}

func (r *placedRun) ctxOf(member string) context.Context {
	if member == "alice" {
		return r.alice
	}
	return r.bob
}

func (r *placedRun) placedAtlas(t *testing.T, member string) *sessionpb.GetAtlasResponse {
	t.Helper()
	out, err := r.h.handler.GetAtlas(r.ctxOf(member), &sessionpb.GetAtlasRequest{
		Session: placedTableSession, Member: member,
	})
	require.NoError(t, err)
	return out
}

// placedByID keys one member's placements by the author's id -- the only
// handle a placement has, and what Hold names.
func placedByID(atlas *sessionpb.GetAtlasResponse) map[string]*sessionpb.AtlasPlacedProp {
	out := map[string]*sessionpb.AtlasPlacedProp{}
	for _, p := range atlas.GetPlaced() {
		out[p.GetId()] = p
	}
	return out
}

// TestAcceptance_TheAtlasCarriesAPlacedFootprintAndItsCells is the claim a
// client acts on: a v4 room's authored rectangles reach GetAtlas with their
// pose, their holdable flag and the cells they stand on -- and the flag says
// which one a client may offer Hold on.
func TestAcceptance_TheAtlasCarriesAPlacedFootprintAndItsCells(t *testing.T) {
	run := startPlacedTableRun(t)

	atlas := run.placedAtlas(t, "alice")
	byID := placedByID(atlas)

	require.Contains(t, byID, dungeonstest.PlacedTableID,
		"the authored footprint is on the map from the first frame")
	table := byID[dungeonstest.PlacedTableID]
	require.True(t, table.GetHoldable(),
		"`propBindings` made this one holdable, and the wire says so")

	require.Contains(t, byID, dungeonstest.PlacedBenchID)
	require.False(t, byID[dungeonstest.PlacedBenchID].GetHoldable(),
		"a rectangle nobody declared holdable stays scenery")

	// THE POSE, which is what a client draws it as. The document gives the
	// table a rectangle longer than it is wide, and the two numbers must not
	// arrive swapped -- a client that drew them the other way round would lay
	// the table across the room instead of along it.
	pose := table.GetPlacement()
	require.NotNil(t, pose, "a placement always carries its rectangle")
	require.Greater(t, pose.GetDepth(), pose.GetWidth(),
		"the authored table is longer along its facing than across it")
	require.NotNil(t, pose.GetOrigin())
	require.NotNil(t, pose.GetLocalOffset())

	// THE CELLS, which is what the wire exists for. A footprint has no anchor
	// cell, so a client that wanted to know what the table is next to would
	// otherwise have to run the engine's geometry a second time.
	require.NotEmpty(t, table.GetCells(),
		"a placement on this list always says where it stands")

	// AND IT IS NOT A PROP. The two lists are different kinds of thing and
	// never share an id: a client reading `props` for this rectangle finds
	// nothing, which is exactly why `placed` was added.
	for _, p := range atlas.GetProps() {
		require.NotEqual(t, dungeonstest.PlacedTableID, p.GetId(),
			"a footprint is never folded into the cell props")
	}
}

// TestAcceptance_HoldingAPlacedFootprintRemovesItForEveryone is the absence
// law's second clause, driven: a placement somebody picked up is gone from
// EVERY member's map, not flagged on anyone's.
//
// Bob stands on a cell THE ATLAS NAMED for the table, which is the whole
// point of carrying `cells`: the offer a client makes from that list and the
// reach the engine judges are the same set, so an offer cannot be refused for
// standing in the wrong place.
func TestAcceptance_HoldingAPlacedFootprintRemovesItForEveryone(t *testing.T) {
	run := startPlacedTableRun(t)

	// The floor, as the atlas paints it -- so the cell bob is seated on is
	// one the map actually has.
	floor := map[spatial.Position]bool{}
	for _, c := range run.placedAtlas(t, "alice").GetCells() {
		floor[spatial.Position{X: c.GetX(), Y: c.GetY()}] = true
	}

	table := placedByID(run.placedAtlas(t, "alice"))[dungeonstest.PlacedTableID]
	require.NotNil(t, table)

	var seat *spatial.Position
	for _, c := range table.GetCells() {
		at := spatial.Position{X: c.GetX(), Y: c.GetY()}
		if floor[at] && at != run.start {
			seat = &at
			break
		}
	}
	require.NotNil(t, seat, "the table stands on floor somebody else can be seated on")

	_, err := run.h.handler.Join(run.bob, &sessionpb.JoinRequest{
		Session: placedTableSession, Member: "bob",
		Position: &sessionpb.Position{X: seat.X, Y: seat.Y},
	})
	require.NoError(t, err)

	for _, member := range []string{"alice", "bob"} {
		require.Contains(t, placedByID(run.placedAtlas(t, member)), dungeonstest.PlacedTableID,
			"%s can see the table before anybody takes it", member)
	}

	// RANGE 0 IS ADJACENT, and standing on a cell the placement covers is
	// distance zero. Nothing generous is passed here on purpose: this is the
	// list and the reach agreeing, and a range that reached across the room
	// would prove neither.
	_, err = run.h.handler.Hold(run.bob, &sessionpb.HoldRequest{
		Session: placedTableSession, Member: "bob",
		Target: dungeonstest.PlacedTableID, Range: 0,
	})
	require.NoError(t, err, "a member standing on a cell the atlas named can take the thing")

	for _, member := range []string{"alice", "bob"} {
		after := placedByID(run.placedAtlas(t, member))
		require.NotContains(t, after, dungeonstest.PlacedTableID,
			"%s's map loses it -- a thing leaving the floor is not a secret", member)
		require.Contains(t, after, dungeonstest.PlacedBenchID,
			"and the rectangle beside it is untouched")
	}
}
