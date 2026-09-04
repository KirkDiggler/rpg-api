package session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
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

// walkWithin moves member from where they stand to the authored cell
// [col,row], one adjacent step at a time, staying inside the region whose
// cells are given.
//
// A PATH IS COMPUTED, NEVER SPELLED. Offset-to-axial is a sheared conversion
// (rpg-toolkit#1141, #1150), so two cells that look adjacent on the authored
// page are not reliably neighbors on the hex grid the game runs — and a
// hand-written route is exactly the kind of arithmetic this workspace has
// already paid for twice. This asks the grid what a neighbor is and
// breadth-firsts over the region's own floor, which also keeps the walk
// inside one room, so no step ever tries to cross the seam except where a
// scene deliberately steps through the door.
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

	// KNOWN DIVERGENCE, filed as rpg-toolkit#1507. The design and the proto
	// both say a departure that dropped what it held carries nothing out --
	// "a holding is either carried out through the exit or left on the
	// floor, never silently deleted" -- but the composition writes the
	// `exited` beat with what the member held BEFORE it decides whether an
	// ending fired, so today it says `holding: [chalice]` and the `dropped`
	// beat below says the chalice is on the floor. Two beats about one
	// departure that disagree.
	//
	// Pinned as it actually behaves, loudly, rather than asserted as it
	// should: rpg-api translates verbatim and has nothing to fix here, and a
	// test that quietly skipped the field would let the disagreement reach
	// the client unrecorded. This line flips to require.Empty when #1507
	// lands.
	require.Equal(t, []string{dungeonstest.ChalicePropID}, departure.GetHolding(),
		"today the exited beat still claims he carried it out -- rpg-toolkit#1507")

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
// # Why this scene builds its world by hand
//
// Every other scene in this file compiles authored YAML, which is the right
// way round. This one cannot, and the reason is a hole worth naming rather
// than working around quietly: an authored `knows:` CANNOT REACH A MONSTER
// THE HOST SPAWNS (rpg-toolkit#1506). The toolkit seeds knowledge links at
// construction only, rpg-api builds its worlds empty of members on purpose,
// and neither `session.SpawnInput` nor `encounter.JoinInput` carries the
// field -- so the shipped heirloom fixture's `knows: [vault]` on the captain
// is dropped at the seam.
//
// The world below is therefore the world a launch WILL build once that
// lands: the same compiled field, with the captain standing in it already
// knowing the way in. What this scene proves is rpg-api's half -- that the
// reveal loot produces crosses the wire to one member and not the other --
// and that half is finished and does not change when #1506 does.
func TestAcceptance_LootingTheCaptainRevealsTheDoorToTheLooterAlone(t *testing.T) {
	t.Run("the captain carried the way in", func(t *testing.T) { lootTheCaptain(t, true) })

	// THE CONTROL, and this file needs it: with the ONE difference removed
	// -- the captain knowing the door -- the identical scene must reveal
	// nothing. Without it, "alice's map gained a doorway" could have been
	// the join, the loot beat, or anything else in the run, and the
	// assertion above would pass on a version of Loot that revealed the
	// door to whoever asked.
	t.Run("a captain who knew nothing gives nothing", func(t *testing.T) { lootTheCaptain(t, false) })
}

func lootTheCaptain(t *testing.T, captainKnows bool) {
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

	compiled, err := tkdungeonspec.Load([]byte(dungeonstest.HeirloomVaultYAML))
	require.NoError(t, err, "the same authored field every other scene here compiles")

	world := worldWithACaptain(t, compiled.Field, captainKnows)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: heirloomSession, Encounter: "heirloom-encounter", World: world,
	})
	require.NoError(t, err)

	// THE BODY IS A BODY BECAUSE ITS SHEET SAYS SO. The session's standing
	// seam reads this record, so a captain with no sheet is reported UP and
	// Loot refuses with ErrNotDown before the scene starts. Seeded through
	// the repository the orchestrator itself runs on, not by reaching into
	// Redis keys.
	sessions := sessionorch.NewSessionRepository(h.redis, time.Hour)
	stored, err := sessions.GetSession(context.Background(), heirloomSession)
	require.NoError(t, err)
	stored.NPCs = append(stored.NPCs, monster.Data{
		ID: captainID, Name: "Skeleton Captain", HitPoints: 0, MaxHitPoints: 22, ArmorClass: 15,
	})
	require.NoError(t, sessions.SaveSession(context.Background(), stored))

	run := &heirloomRun{
		h:     h,
		alice: auth.WithPlayerID(context.Background(), "player-alice"),
		bob:   auth.WithPlayerID(context.Background(), "player-bob"),
	}
	for _, who := range []struct {
		id       string
		col, row int
	}{{"alice", aliceSeatCol, aliceSeatRow}, {"bob", bobSeatCol, bobSeatRow}} {
		_, joinErr := h.handler.Join(run.ctxOf(who.id), &sessionpb.JoinRequest{
			Session: heirloomSession, Member: who.id, Position: pbAt(who.col, who.row),
		})
		require.NoError(t, joinErr)
	}

	// Nobody has searched. The way in is on nobody's map.
	require.Empty(t, run.atlas(t, "alice").GetDoorways())
	require.Empty(t, run.atlas(t, "bob").GetDoorways())

	_, err = h.handler.Loot(run.alice, &sessionpb.LootRequest{
		Session: heirloomSession, Member: "alice", Target: captainID, Range: holdReach,
	})
	require.NoError(t, err, "loot is offered on every downed body and never refuses for having nothing")

	if captainKnows {
		require.Len(t, run.atlas(t, "alice").GetDoorways(), 1,
			"looting the body that carried the way in puts the door on the looter's map")
		require.Contains(t, run.kinds(t, "alice"), sessionpb.EventKind_EVENT_KIND_DOOR_REVEALED,
			"the looter hears the same beat a successful search produces")
	} else {
		require.Empty(t, run.atlas(t, "alice").GetDoorways(),
			"a body with nothing to give transfers nothing")
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
		require.Equal(t, captainID, looted.GetBody())
	}
}

// captainID is the monster who knows the way into the vault.
const captainID = "captain"

// worldWithACaptain builds the heirloom vault with one member already
// standing in it: a downed skeleton captain, carrying the knowledge link the
// authored file gives them or carrying nothing.
//
// `knows` is the ONE thing the two scenes vary, so everything else about the
// two worlds is identical by construction rather than by inspection.
//
// The capabilities are internal/sessionworld's, restated here because they
// are unexported there and for the same reason they are trivial there: this
// world is EMPTY OF PLAYERS at the moment it is built, so nobody can see,
// roll, strike or take a turn in it. The session package supplies the real
// ones when it loads the world to play it.
func worldWithACaptain(t *testing.T, field tkencounter.FieldInput, knows bool) *tkencounter.EncounterData {
	t.Helper()

	captain := tkencounter.MemberInput{
		ID: captainID, Kind: tkencounter.KindMonster, Name: "Skeleton Captain",
		// Authored offset [1,0], in the hall where the party comes in.
		Position: spatial.Position{X: 1, Y: 0},
	}
	if knows {
		// The knowledge link, by COMPILED door id -- dungeonspec mints
		// `<key>/<id>` so two dungeons in one process cannot collide.
		captain.Knows = []tkencounter.DoorID{dungeonstest.HeirloomVaultDoorID}
	}

	enc, err := tkencounter.NewEncounter(&tkencounter.SetupInput{
		Field:   field,
		Members: []tkencounter.MemberInput{captain},
		Endings: []tkencounter.EndingInput{
			{Key: sessionworld.EndingWithdrawn, Trigger: tkencounter.TriggerExternal{}},
		},
		Retention:     tkencounter.RetentionUnbounded,
		Initiative:    orderAsGiven{},
		Standing:      everybodyIsDown{},
		Sight:         allSeeing{},
		TurnDriver:    tkencounter.PassDriver{},
		Striker:       tkencounter.RefusingStriker{},
		Announcer:     tkencounter.RefusingAnnouncer{},
		CheckResolver: neverResolves{},
		Witness:       nobodyPerceives{},
	})
	require.NoError(t, err)

	data := enc.ToData()
	return &data
}

// everybodyIsDown answers for the FIXTURE BUILD ALONE, and it answers "down"
// so that no fight has already formed by the time a session loads the blob:
// the only member here is the captain, and a captain reported up beside two
// players would put all three in a bubble before the scene ran. Once the
// session owns the world the real standing seam answers out of the stored
// sheet, which is why the scene seeds that sheet at zero.
type everybodyIsDown struct{}

func (everybodyIsDown) Standing(down []tkencounter.MemberID) ([]tkencounter.MemberID, error) {
	return down, nil
}

func (everybodyIsDown) Assess(members []tkencounter.MemberID) (*tkencounter.ParticipationAssessment, error) {
	out := &tkencounter.ParticipationAssessment{
		Members: make([]tkencounter.MemberParticipation, 0, len(members)),
	}
	for _, id := range members {
		out.Members = append(out.Members, tkencounter.MemberParticipation{
			Member: id, Down: true, Conscious: false, Contact: false,
			Turn: tkencounter.TurnParticipationRemove,
		})
	}
	return out, nil
}

type neverResolves struct{}

func (neverResolves) ResolveCheck(*tkencounter.ResolveCheckInput) (*tkencounter.ResolveCheckOutput, error) {
	return nil, errNoCheckAtConstruction
}

var errNoCheckAtConstruction = errors.New("a check was rolled against a world still being built")

type nobodyPerceives struct{}

func (nobodyPerceives) Perceivers(*tkencounter.PerceiversInput) ([]tkencounter.MemberID, error) {
	return []tkencounter.MemberID{}, nil
}
