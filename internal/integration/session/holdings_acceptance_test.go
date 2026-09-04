package session_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// holdings_acceptance_test.go is wave 2's proof (rpg-project#368,
// rpg-api#913): the recover-the-artifact scenario driven the way two players
// drive it, through the real handlers, on a dungeon compiled from authored
// YAML.
//
// Everything here is DRIVEN, never asserted about a struct: the scenario's
// ending exists because the file bound it and sessionworld constructed it;
// the prop leaves the map because somebody held it; the run ends because a
// carrier declared Exit standing on the bound exit.

// heirloomRun is one session on the compact heirloom fixture, with alice and
// bob joined and the door still hidden from both.
type heirloomRun struct {
	h     *acceptanceHarness
	alice context.Context
	bob   context.Context
}

const (
	heirloomSession = "heirloom-run"

	// The two seats, in authored [col,row] offsets. Bob stands on the
	// side-door cell so an ordinary departure through an unbound exit is one
	// verb away.
	aliceSeatCol, aliceSeatRow = 0, 1
	bobSeatCol, bobSeatRow     = 0, 2

	// holdReach is the range every Hold below passes. RANGE IS THE HOST'S
	// TRUTH -- the seam takes it and forwards it, and what counts as reach
	// is the rulebook's rule, not this test's. It is generous on purpose:
	// offset-to-axial is a sheared conversion, so two cells that look
	// adjacent on the authored page are not reliably neighbors on the hex
	// grid, and a scene about holdings should not fail on that arithmetic.
	holdReach = 4
)

func startHeirloomRun(t *testing.T) *heirloomRun {
	t.Helper()
	return startHeirloomRunWith(t, true)
}

// startHeirloomRunWith starts the run, optionally with the intel record the
// captain was authored holding left off, which is the ONE thing the two loot
// scenes vary.
func startHeirloomRunWith(t *testing.T, captainHolds bool) *heirloomRun {
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

	// The dungeon is COMPILED FROM AUTHORED YAML, which is the point: the
	// scenario binding in that file is what puts the exited-holding ending
	// on this world, through the rulebook's own scenario package. A world
	// built by hand here would prove the verbs and skip the wiring.
	dungeon, err := sessionworld.Compile([]byte(dungeonstest.HeirloomVaultYAML))
	require.NoError(t, err, "the heirloom fixture must compile")

	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: heirloomSession, Encounter: "heirloom-encounter", World: dungeon.World,
	})
	require.NoError(t, err)

	// THE GARRISON ARRIVES THE WAY THE LOBBY BRINGS IT: one Spawn per
	// authored monster, carrying the intel records the author placed in it.
	// That forwarding is the whole of rpg-api's part in path 2, and this is
	// the call that exercises it -- the same three-plus-one fields
	// StartEncounter builds, with the COMPILED record ids Compiled carries.
	for _, m := range dungeon.Monsters {
		holds := m.Holds
		if !captainHolds {
			holds = nil
		}
		_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
			Session: heirloomSession, ID: m.MemberID, Ref: m.Ref, Position: m.At, Holds: holds,
		})
		require.NoError(t, err, "spawning %s", m.MemberID)
	}

	// THE BODY IS A BODY BECAUSE ITS SHEET SAYS SO. The session's standing
	// seam reads the record Spawn just wrote, so a captain at full hit
	// points is reported UP -- Loot would refuse with ErrNotDown, and every
	// other scene here would run inside a fight with the verbs on the turn
	// clock. Zeroed through the repository the orchestrator itself runs on
	// rather than by reaching into Redis keys, and BEFORE anybody joins, so
	// no bubble ever forms.
	//
	// This is fixture state standing in for a fight the scenes are not
	// about. The fight itself is proven in acceptance_test.go.
	downTheGarrison(t, h)

	run := &heirloomRun{
		h:     h,
		alice: auth.WithPlayerID(context.Background(), "player-alice"),
		bob:   auth.WithPlayerID(context.Background(), "player-bob"),
	}
	_, err = h.handler.Join(run.alice, &sessionpb.JoinRequest{
		Session: heirloomSession, Member: "alice", Position: pbAt(aliceSeatCol, aliceSeatRow),
	})
	require.NoError(t, err)
	_, err = h.handler.Join(run.bob, &sessionpb.JoinRequest{
		Session: heirloomSession, Member: "bob", Position: pbAt(bobSeatCol, bobSeatRow),
	})
	require.NoError(t, err)

	return run
}

// downTheGarrison puts every spawned monster's sheet at zero hit points.
func downTheGarrison(t *testing.T, h *acceptanceHarness) {
	t.Helper()

	sessions := sessionorch.NewSessionRepository(h.redis, time.Hour)
	stored, err := sessions.GetSession(context.Background(), heirloomSession)
	require.NoError(t, err)
	require.NotEmpty(t, stored.NPCs, "Spawn must have written a sheet for every authored monster")
	for i := range stored.NPCs {
		stored.NPCs[i].HitPoints = 0
	}
	require.NoError(t, sessions.SaveSession(context.Background(), stored))
}

func (r *heirloomRun) walkWithin(t *testing.T, member string, region [][2]int, col, row int) {
	t.Helper()

	from := r.whereIs(t, member)
	goal := at(col, row)
	if from == goal {
		return
	}

	floor := map[spatial.Position]bool{}
	for _, c := range region {
		floor[at(c[0], c[1])] = true
	}
	grid := spatial.NewAxialHexGrid(spatial.AxialHexGridConfig{SpanWidth: 1e6, SpanHeight: 1e6})

	prev := map[spatial.Position]spatial.Position{}
	seen := map[spatial.Position]bool{from: true}
	queue := []spatial.Position{from}
	for len(queue) > 0 && !seen[goal] {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range grid.GetNeighbors(cur) {
			if !floor[n] || seen[n] {
				continue
			}
			seen[n], prev[n] = true, cur
			queue = append(queue, n)
		}
	}
	require.True(t, seen[goal], "no walk inside this region reaches [%d,%d]", col, row)

	var reversed []spatial.Position
	for c := goal; c != from; c = prev[c] {
		reversed = append(reversed, c)
	}
	path := make([]*sessionpb.Position, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		path = append(path, &sessionpb.Position{X: reversed[i].X, Y: reversed[i].Y})
	}

	_, err := r.h.handler.Move(r.ctxOf(member), &sessionpb.MoveRequest{
		Session: heirloomSession, Member: member, Path: path,
	})
	require.NoError(t, err)
}

// step takes exactly one move, to the authored cell [col,row] — what a scene
// uses to cross the vault door, which walkWithin deliberately will not do.
func (r *heirloomRun) step(t *testing.T, member string, col, row int) {
	t.Helper()
	_, err := r.h.handler.Move(r.ctxOf(member), &sessionpb.MoveRequest{
		Session: heirloomSession, Member: member,
		Path: []*sessionpb.Position{pbAt(col, row)},
	})
	require.NoError(t, err)
}

func (r *heirloomRun) whereIs(t *testing.T, member string) spatial.Position {
	t.Helper()
	out, err := r.h.handler.GetWhere(r.ctxOf(member), &sessionpb.GetWhereRequest{
		Session: heirloomSession, Member: member,
	})
	require.NoError(t, err)
	return spatial.Position{X: out.GetPosition().GetX(), Y: out.GetPosition().GetY()}
}

func (r *heirloomRun) ctxOf(member string) context.Context {
	if member == "alice" {
		return r.alice
	}
	return r.bob
}

func (r *heirloomRun) atlas(t *testing.T, member string) *sessionpb.GetAtlasResponse {
	t.Helper()
	out, err := r.h.handler.GetAtlas(r.ctxOf(member), &sessionpb.GetAtlasRequest{
		Session: heirloomSession, Member: member,
	})
	require.NoError(t, err)
	return out
}

// story is one member's own beats, from their story rather than a live
// stream: catch-up and live are byte-equal for the same seq, so reading the
// story keeps these scenes free of subscription timing.
func (r *heirloomRun) story(t *testing.T, member string) []*sessionpb.Event {
	t.Helper()
	out, err := r.h.handler.GetStory(r.ctxOf(member), &sessionpb.GetStoryRequest{
		Session: heirloomSession, Member: member,
	})
	require.NoError(t, err)
	return out.GetEntries()
}

func (r *heirloomRun) kinds(t *testing.T, member string) []sessionpb.EventKind {
	t.Helper()
	entries := r.story(t, member)
	out := make([]sessionpb.EventKind, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.GetKind())
	}
	return out
}

// propIDs names the props on one member's map. A held prop is absent from
// everybody's, so this is how "it left the map" is asked.
func propIDs(atlas *sessionpb.GetAtlasResponse) []string {
	out := make([]string, 0, len(atlas.GetProps()))
	for _, p := range atlas.GetProps() {
		out = append(out, p.GetId())
	}
	return out
}

func exitIDs(atlas *sessionpb.GetAtlasResponse) []string {
	out := make([]string, 0, len(atlas.GetExits()))
	for _, e := range atlas.GetExits() {
		out = append(out, e.GetId())
	}
	return out
}

// findTheVault is path 1: search the hall, find the concealed door, open it.
// The harness's roller answers the top face, so the perception check clears
// DC 15 every run rather than one in three.
func (r *heirloomRun) findTheVault(t *testing.T, member string) {
	t.Helper()
	_, err := r.h.handler.Search(r.ctxOf(member), &sessionpb.SearchRequest{
		Session: heirloomSession, Member: member, Region: "hall",
	})
	require.NoError(t, err)
	_, err = r.h.handler.OpenDoor(r.ctxOf(member), &sessionpb.OpenDoorRequest{
		Session: heirloomSession, Member: member, Door: dungeonstest.HeirloomVaultDoorID,
	})
	require.NoError(t, err)
}

// TestAcceptance_TheAtlasCarriesIdsHoldabilityAndTheWaysOut is the wire
// half of design §5: a client offers Hold only where `holdable` is true and
// never guesses from an id, and it can DRAW the way out because the atlas
// says where the exits are.
func TestAcceptance_TheAtlasCarriesIdsHoldabilityAndTheWaysOut(t *testing.T) {
	run := startHeirloomRun(t)

	for _, member := range []string{"alice", "bob"} {
		atlas := run.atlas(t, member)

		require.ElementsMatch(t,
			[]string{dungeonstest.HeirloomBoundExitID, dungeonstest.HeirloomOtherExitID},
			exitIDs(atlas),
			"%s is served both authored ways out -- exits are structure, the same for everyone", member)

		byID := map[string]*sessionpb.AtlasProp{}
		for _, p := range atlas.GetProps() {
			byID[p.GetId()] = p
		}
		require.Contains(t, byID, dungeonstest.ChalicePropID)
		require.True(t, byID[dungeonstest.ChalicePropID].GetHoldable(),
			"the chalice is authored holdable, and the wire says so")
		require.Contains(t, byID, dungeonstest.PillarPropID)
		require.False(t, byID[dungeonstest.PillarPropID].GetHoldable(),
			"a thing nobody declared holdable stays scenery")
		require.NotContains(t, byID, dungeonstest.HeirloomPropID,
			"the heirloom stands in a room %s has not found", member)
	}
}

// TestAcceptance_HoldingRemovesThePropForEveryone is design §8's row: hold
// removes the prop for everyone and the holder holds it.
//
// The chalice rather than the heirloom on purpose -- it stands in the open
// hall, so "gone for EVERYONE" is observable without a reveal in the way of
// it.
func TestAcceptance_HoldingRemovesThePropForEveryone(t *testing.T) {
	run := startHeirloomRun(t)

	require.Contains(t, propIDs(run.atlas(t, "alice")), dungeonstest.ChalicePropID)
	require.Contains(t, propIDs(run.atlas(t, "bob")), dungeonstest.ChalicePropID)

	_, err := run.h.handler.Hold(run.alice, &sessionpb.HoldRequest{
		Session: heirloomSession, Member: "alice",
		Target: dungeonstest.ChalicePropID, Range: holdReach,
	})
	require.NoError(t, err)

	require.NotContains(t, propIDs(run.atlas(t, "alice")), dungeonstest.ChalicePropID,
		"the holder's own map loses it too -- she is carrying it, not standing next to it")
	require.NotContains(t, propIDs(run.atlas(t, "bob")), dungeonstest.ChalicePropID,
		"physical state folds on the truth grain: it left the floor for everybody")

	for _, member := range []string{"alice", "bob"} {
		var held *sessionpb.Held
		for _, e := range run.story(t, member) {
			if e.GetKind() == sessionpb.EventKind_EVENT_KIND_HELD {
				held = e.GetHeld()
			}
		}
		require.NotNil(t, held, "%s was present and hears the held beat", member)
		require.Equal(t, "alice", held.GetHolder())
		require.Equal(t, dungeonstest.ChalicePropID, held.GetProp())
	}
}

// TestAcceptance_LeavingThroughTheBoundExitEndsTheRun is the whole win, end
// to end: find the vault, pick up the heirloom, walk out the front gate.
//
// The ENDING KEY IS THE SCENARIO'S OWN ID. Nobody in rpg-api chose it: the
// file bound recover-the-artifact, the rulebook's scenario package declared
// an ending under that key, sessionworld put it on the world beside
// `withdrawn`, and the beat carries it to both players.
func TestAcceptance_LeavingThroughTheBoundExitEndsTheRun(t *testing.T) {
	run := startHeirloomRun(t)
	run.findTheVault(t, "alice")

	hallSide := dungeonstest.HeirloomVaultDoorCrossing[0]
	vaultSide := dungeonstest.HeirloomVaultDoorCrossing[1]
	run.walkWithin(t, "alice", dungeonstest.HeirloomHallCells, hallSide[0], hallSide[1])
	run.step(t, "alice", vaultSide[0], vaultSide[1])
	run.walkWithin(t, "alice", dungeonstest.HeirloomVaultCells, 5, 1)

	_, err := run.h.handler.Hold(run.alice, &sessionpb.HoldRequest{
		Session: heirloomSession, Member: "alice",
		Target: dungeonstest.HeirloomPropID, Range: holdReach,
	})
	require.NoError(t, err)

	// Back to the cell the scenario binds as the way out.
	run.walkWithin(t, "alice", dungeonstest.HeirloomVaultCells, vaultSide[0], vaultSide[1])
	run.step(t, "alice", hallSide[0], hallSide[1])
	run.walkWithin(t, "alice", dungeonstest.HeirloomHallCells, aliceSeatCol, aliceSeatRow)

	exited, err := run.h.handler.Exit(run.alice, &sessionpb.ExitRequest{
		Session: heirloomSession, Member: "alice",
	})
	require.NoError(t, err)
	require.NotNil(t, exited.GetClosed(), "leaving the bound exit while holding ends the run")

	var ended *sessionpb.Ended
	var departure *sessionpb.Exited
	for _, e := range run.story(t, "bob") {
		switch e.GetKind() {
		case sessionpb.EventKind_EVENT_KIND_ENDED:
			ended = e.GetEnded()
		case sessionpb.EventKind_EVENT_KIND_EXITED:
			departure = e.GetExited()
		default:
		}
	}
	require.NotNil(t, ended, "the run ends for everyone still in it, not only for the carrier")
	require.Equal(t, "recover-the-artifact", ended.GetEnding())

	require.NotNil(t, departure, "and the departure that ended it is narratable")
	require.Equal(t, "alice", departure.GetMember())
	require.Equal(t, []string{dungeonstest.HeirloomPropID}, departure.GetHolding(),
		"the record says what she carried out")
	require.Equal(t, dungeonstest.HeirloomBoundExitID, departure.GetExit(),
		"and which way out she used")
}

// TestAcceptance_LeavingFromAnywhereElseDropsWhatYouCarry is design R9: a
// departure that did not end the run drops what the member carried, where
// they left from. Without it a carrier who walks out through the lobby -- or
// disconnects -- takes the only win in the run with them.
//
// Bob does the leaving, from a hall cell that is not either authored exit,
// so this is also the "the other member leaves first and the run continues"
// half of design §8's row.
func TestAcceptance_LeavingFromAnywhereElseDropsWhatYouCarry(t *testing.T) {
	run := startHeirloomRun(t)

	run.walkWithin(t, "bob", dungeonstest.HeirloomHallCells, 2, 2)

	_, err := run.h.handler.Hold(run.bob, &sessionpb.HoldRequest{
		Session: heirloomSession, Member: "bob",
		Target: dungeonstest.ChalicePropID, Range: holdReach,
	})
	require.NoError(t, err)
	require.NotContains(t, propIDs(run.atlas(t, "alice")), dungeonstest.ChalicePropID)

	exited, err := run.h.handler.Exit(run.bob, &sessionpb.ExitRequest{
		Session: heirloomSession, Member: "bob",
	})
	require.NoError(t, err)
	require.Nil(t, exited.GetClosed(),
		"leaving from a cell nobody authored as a way out is an ordinary departure")

	var dropped *sessionpb.Dropped
	var departure *sessionpb.Exited
	for _, e := range run.story(t, "alice") {
		switch e.GetKind() {
		case sessionpb.EventKind_EVENT_KIND_DROPPED:
			dropped = e.GetDropped()
		case sessionpb.EventKind_EVENT_KIND_EXITED:
			departure = e.GetExited()
		default:
		}
	}
	require.NotNil(t, dropped, "the chalice lands back on the map, and everyone present is told")
	require.Equal(t, "bob", dropped.GetMember())
	require.Equal(t, dungeonstest.ChalicePropID, dropped.GetProp())
	require.Equal(t, pbAt(2, 2).GetX(), dropped.GetAt().GetX(), "where he stood on the way out")
	require.Equal(t, pbAt(2, 2).GetY(), dropped.GetAt().GetY())

	require.NotNil(t, departure)
	require.Empty(t, departure.GetExit(),
		"he used no authored exit -- empty is the truth, not 'unknown'")

	// THE TWO BEATS NEVER DISAGREE (rpg-toolkit#1507, found by this scene and
	// fixed in encounter v0.57.1). `holding` is what actually LEFT THE RUN
	// with the member, so a departure that dropped everything carries
	// nothing out and the DROPPED beat below is the whole story of where it
	// went. The composition used to write this beat with what the member
	// held before deciding whether an ending had fired, which said "he left
	// holding the chalice" one line above "the chalice is on the floor".
	//
	// A holding is therefore either carried out through the bound exit or
	// left on the floor, and never silently deleted -- which is the wire
	// contract Exited.holding states, now true of the beat as well.
	require.Empty(t, departure.GetHolding(),
		"a departure that dropped what it held carried nothing out")

	require.Contains(t, propIDs(run.atlas(t, "alice")), dungeonstest.ChalicePropID,
		"a refetch agrees with the beat: the chalice is on the map again")
	require.NotContains(t, run.kinds(t, "alice"), sessionpb.EventKind_EVENT_KIND_ENDED,
		"and the run goes on for the member still in it")
}

// TestAcceptance_ThePartyThatNeverSearchesFinishesBlind is design §8's
// secrecy row, and the probe law at the wire.
//
// Nobody searches, so the vault door is on nobody's map and the heirloom
// stands in a room nobody has found. A client that guessed its id learns
// NOTHING: the refusal is byte-identical to the one a completely invented id
// gets. If it were not -- if "out of range" or "not holdable" came back for
// the real id -- guessing would be a way to map the dungeon.
func TestAcceptance_ThePartyThatNeverSearchesFinishesBlind(t *testing.T) {
	run := startHeirloomRun(t)

	for _, member := range []string{"alice", "bob"} {
		atlas := run.atlas(t, member)
		require.Empty(t, atlas.GetDoorways(),
			"%s never searched, so the hidden door is not on their map", member)
		require.NotContains(t, propIDs(atlas), dungeonstest.HeirloomPropID)
	}

	guessed, guessedErr := run.h.handler.Hold(run.alice, &sessionpb.HoldRequest{
		Session: heirloomSession, Member: "alice",
		Target: dungeonstest.HeirloomPropID, Range: holdReach,
	})
	require.Nil(t, guessed)
	require.Error(t, guessedErr)

	invented, inventedErr := run.h.handler.Hold(run.alice, &sessionpb.HoldRequest{
		Session: heirloomSession, Member: "alice",
		Target: "no-such-thing-at-all", Range: holdReach,
	})
	require.Nil(t, invented)
	require.Error(t, inventedErr)

	require.Equal(t, inventedErr.Error(), guessedErr.Error(),
		"a real prop the member cannot see and an id nothing has answer with the same bytes")
	require.NotContains(t, guessedErr.Error(), dungeonstest.HeirloomPropID,
		"and the refusal does not echo the guessed id back")
}

// -----------------------------------------------------------------------
// Path 2 — loot the captain
// -----------------------------------------------------------------------

// TestAcceptance_LootingTheCaptainRevealsTheDoorToTheLooterAlone is design
// §8's second row and design P4: loot is a second writer of the fact search
// writes, so the reveal it produces is byte-identical to a successful
// search's -- one DOOR_REVEALED, to the looter, and nobody else's map moves.
//
// THE WHOLE CHAIN IS DRIVEN, file to beat. The author DECLARES A RECORD --
// `intel: [{id: vault-map, reveals: {door: vault-door}}]` -- and places it in
// a monster with `holds: [vault-map]`; dungeonspec mints the compiled record
// id and the compiled door id it reveals; sessionworld carries the record id
// onto the monster and the intel TABLE onto the field; the launch forwards
// the record into Spawn; the composition seeds it as a holding when the
// monster enters the world; Loot copies it to the looter and reads the
// table to learn what it reveals. Nothing in that list is asserted about a
// struct -- the door appears on one player's map because somebody looted a
// body.
//
// THE INDIRECTION IS THE POINT (rpg-project#372). The member holds a RECORD,
// not a door, so the same fact kind serves a region or a treasure the day a
// use case asks for one, and none of that touches who holds it.
func TestAcceptance_LootingTheCaptainRevealsTheDoorToTheLooterAlone(t *testing.T) {
	t.Run("the captain held the record", func(t *testing.T) { lootTheCaptain(t, true) })

	// THE CONTROL, and this file needs it: with the ONE difference removed
	// -- the captain holding the record -- the identical scene must reveal
	// nothing. Without it, "alice's map gained a doorway" could have been
	// the join, the spawn, or anything else in the run, and the assertion
	// above would pass on a version of Loot that revealed the door to
	// whoever asked. The record stays DECLARED in both, which is the
	// sharper control: authored knowledge nobody carries reveals nothing.
	t.Run("a captain holding nothing gives nothing", func(t *testing.T) { lootTheCaptain(t, false) })
}

func lootTheCaptain(t *testing.T, captainHolds bool) {
	t.Helper()

	run := startHeirloomRunWith(t, captainHolds)

	// Nobody has searched. The way in is on nobody's map.
	require.Empty(t, run.atlas(t, "alice").GetDoorways())
	require.Empty(t, run.atlas(t, "bob").GetDoorways())

	_, err := run.h.handler.Loot(run.alice, &sessionpb.LootRequest{
		Session: heirloomSession, Member: "alice",
		Target: dungeonstest.HeirloomCaptainMemberID, Range: holdReach,
	})
	require.NoError(t, err, "loot is offered on every downed body and never refuses for having nothing")

	if captainHolds {
		require.Len(t, run.atlas(t, "alice").GetDoorways(), 1,
			"looting the body that held the record puts what it reveals on the looter's map")
		require.Contains(t, run.kinds(t, "alice"), sessionpb.EventKind_EVENT_KIND_DOOR_REVEALED,
			"the looter hears the same beat a successful search produces")
	} else {
		require.Empty(t, run.atlas(t, "alice").GetDoorways(),
			"a body holding nothing transfers nothing")
		require.NotContains(t, run.kinds(t, "alice"), sessionpb.EventKind_EVENT_KIND_DOOR_REVEALED)
	}
	require.Empty(t, run.atlas(t, "bob").GetDoorways(),
		"and nobody else's map moves either way -- knowledge is audience-scoped, "+
			"unlike where things physically are")
	require.NotContains(t, run.kinds(t, "bob"), sessionpb.EventKind_EVENT_KIND_DOOR_REVEALED)

	// EVERYONE PRESENT HEARS THE LOOT ITSELF, and it names looter and body
	// and nothing of what moved (design P3): a beat that varied with what
	// the body held would tell the party which corpse was worth looting.
	// This runs in BOTH cases above, which is what makes it a claim.
	for _, member := range []string{"alice", "bob"} {
		var looted *sessionpb.Looted
		for _, e := range run.story(t, member) {
			if e.GetKind() == sessionpb.EventKind_EVENT_KIND_LOOTED {
				looted = e.GetLooted()
			}
		}
		require.NotNil(t, looted, "%s was present", member)
		require.Equal(t, "alice", looted.GetLooter())
		require.Equal(t, dungeonstest.HeirloomCaptainMemberID, looted.GetBody())
	}
}

// TestAcceptance_TheLootedWayInOpensTheSameVault closes path 2 to where path
// 1 already runs: what the record reveals is not a private note about a door,
// it is the door. The looter opens it and the vault is hers, exactly as if
// she had found it by searching.
func TestAcceptance_TheLootedWayInOpensTheSameVault(t *testing.T) {
	run := startHeirloomRun(t)

	_, err := run.h.handler.Loot(run.alice, &sessionpb.LootRequest{
		Session: heirloomSession, Member: "alice",
		Target: dungeonstest.HeirloomCaptainMemberID, Range: holdReach,
	})
	require.NoError(t, err)

	_, err = run.h.handler.OpenDoor(run.alice, &sessionpb.OpenDoorRequest{
		Session: heirloomSession, Member: "alice", Door: dungeonstest.HeirloomVaultDoorID,
	})
	require.NoError(t, err, "the door the record revealed is one she can open")

	hallSide := dungeonstest.HeirloomVaultDoorCrossing[0]
	vaultSide := dungeonstest.HeirloomVaultDoorCrossing[1]
	run.walkWithin(t, "alice", dungeonstest.HeirloomHallCells, hallSide[0], hallSide[1])
	run.step(t, "alice", vaultSide[0], vaultSide[1])
	run.walkWithin(t, "alice", dungeonstest.HeirloomVaultCells, 5, 1)

	_, err = run.h.handler.Hold(run.alice, &sessionpb.HoldRequest{
		Session: heirloomSession, Member: "alice",
		Target: dungeonstest.HeirloomPropID, Range: holdReach,
	})
	require.NoError(t, err, "and the heirloom behind it is hers to pick up")

	run.walkWithin(t, "alice", dungeonstest.HeirloomVaultCells, vaultSide[0], vaultSide[1])
	run.step(t, "alice", hallSide[0], hallSide[1])
	run.walkWithin(t, "alice", dungeonstest.HeirloomHallCells, aliceSeatCol, aliceSeatRow)

	exited, err := run.h.handler.Exit(run.alice, &sessionpb.ExitRequest{
		Session: heirloomSession, Member: "alice",
	})
	require.NoError(t, err)
	require.NotNil(t, exited.GetClosed(),
		"path 2 ends where path 1 ends, and the ending cannot tell them apart")
	require.Equal(t, "recover-the-artifact", exited.GetClosed().GetEnding())
}

// TestAcceptance_AHolderAndANonHolderAreIdenticalUntilSomebodyLoots is design
// §7's secrecy row, at the seam where it can actually be broken
// (rpg-project#372, and slice 2's design P3 before it).
//
// Loot is offered on EVERY downed body. If anything a client can read
// differed between a captain carrying the way into the vault and one
// carrying nothing, the affordance would answer the question it exists to
// ask, and a party would know which corpse was worth its time before
// touching it.
//
// So this runs the identical scene twice, varying ONLY whether the captain
// was authored holding the record, and compares the two runs BYTE FOR BYTE:
// every member's atlas and every member's whole story, as the wire serializes
// them. Not field-by-field — a field-by-field check only covers the fields
// somebody thought to list, and the leak this guards against is precisely a
// field nobody thought about.
func TestAcceptance_AHolderAndANonHolderAreIdenticalUntilSomebodyLoots(t *testing.T) {
	holding := heirloomWireBytes(t, true)
	empty := heirloomWireBytes(t, false)

	// Non-vacuity, said out loud: an assertion that two empty maps match
	// would pass forever and mean nothing.
	require.NotEmpty(t, holding["alice/atlas"], "there is an atlas to compare")
	require.NotEmpty(t, holding["alice/story"], "and beats to compare")
	require.Equal(t, empty, holding,
		"a captain holding the way into the vault and a captain holding nothing "+
			"are the same bytes to every observer, until somebody loots one")
}

// heirloomWireBytes is everything both members can read of a fresh run,
// serialized: the atlas each is served and the story each has been told.
func heirloomWireBytes(t *testing.T, captainHolds bool) map[string]string {
	t.Helper()

	run := startHeirloomRunWith(t, captainHolds)

	out := map[string]string{}
	for _, member := range []string{"alice", "bob"} {
		atlas, err := run.h.handler.GetAtlas(run.ctxOf(member), &sessionpb.GetAtlasRequest{
			Session: heirloomSession, Member: member,
		})
		require.NoError(t, err)
		atlasBytes, err := proto.Marshal(atlas)
		require.NoError(t, err)
		out[member+"/atlas"] = string(atlasBytes)

		story, err := run.h.handler.GetStory(run.ctxOf(member), &sessionpb.GetStoryRequest{
			Session: heirloomSession, Member: member,
		})
		require.NoError(t, err)
		// THE WHOLE BEAT, rendered: session, seq, recipient, kind, the raw
		// payload and the typed body. Measured rather than assumed to be
		// stable — these beats carry no wall-clock time and no correlation
		// id, so two runs of the same scene render identically and any
		// difference is a real one.
		require.NotEmpty(t, story.GetEntries(), "%s has been told something to compare", member)
		var said []string
		for _, e := range story.GetEntries() {
			said = append(said, e.GetKind().String()+"|"+protojson.Format(e))
		}
		out[member+"/story"] = strings.Join(said, "\n")
	}

	return out
}

// TestAcceptance_ForwardingTheAuthorsRawRecordIDIsRefusedByName is the hazard
// the seam documents and the one a host is most likely to walk into
// (rpg-project#372): dungeonspec mints `<key>/<id>`, and a launch that
// forwarded the author's own spelling would name a record the composition
// does not declare.
//
// REFUSED, NOT IGNORED, and that is the whole value of the row. A spawn that
// shrugged would place a captain holding nothing, and the run would play
// perfectly until somebody looted the body for an empty answer — a scenario
// that cannot be won by its second path, with nothing anywhere saying why.
func TestAcceptance_ForwardingTheAuthorsRawRecordIDIsRefusedByName(t *testing.T) {
	h := newAcceptanceHarness(t)
	dungeon, err := sessionworld.Compile([]byte(dungeonstest.HeirloomVaultYAML))
	require.NoError(t, err)

	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: heirloomSession, Encounter: "heirloom-encounter", World: dungeon.World,
	})
	require.NoError(t, err)

	var captain sessionworld.Monster
	for _, m := range dungeon.Monsters {
		if m.PlacementID == dungeonstest.HeirloomCaptainPlacementID {
			captain = m
		}
	}
	require.Equal(t, []string{dungeonstest.HeirloomIntelRecordID}, captain.Holds,
		"the compiled id is what the launch forwards")

	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: heirloomSession, ID: captain.MemberID, Ref: captain.Ref, Position: captain.At,
		// The AUTHOR's spelling — what a host reaching for the file's own
		// word instead of the compiler's would send.
		Holds: []string{dungeonstest.HeirloomIntelAuthoredID},
	})
	require.Error(t, err, "a record this dungeon does not declare is refused at the spawn")
	require.ErrorIs(t, err, sdk.ErrNoIntel,
		"and by a sentinel about intel, not about doorways")

	// The compiled id, on the same call, is accepted — so the refusal above
	// is about the ID and not about anything else in this spawn.
	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: heirloomSession, ID: captain.MemberID, Ref: captain.Ref, Position: captain.At,
		Holds: captain.Holds,
	})
	require.NoError(t, err)
}
