package session_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// holdout_acceptance_test.go is the hold-out's acceptance at this repo's own
// level (rpg-project#375, design §9): the SHIPPED raider camp, launched
// through the lobby's own StartEncounter and played through the gRPC
// handler, with nothing spawned or seeded by the scenes themselves.
//
// The camp, step B: the party comes in at the gate; the scout in the yard
// forms a fight on sight; the Wiseman's letter is in RESERVE and appears at
// the gate at round 6 (A5); carried into the chief's region mid-fight it
// teaches the chief, the camp's stance toward the party folds to neutral,
// the fight dissolves because its sides stopped being sides, and the
// hold-out ends the run (A2). Kill the chief instead and three zombies pour
// through the gate (A4).

// raiderCampKey is the shipped fixture's key.
const raiderCampKey = "reference-raider-camp"

// alwaysTheLowestFace answers 1 to every roll, so the camp never lands a
// blow while the party holds out. These scenes are about what reaches the
// wire, and who wins a fight is not their question (the lobby's loot scene
// made the same call, for the same reason).
type alwaysTheLowestFace struct{}

func (alwaysTheLowestFace) Roll(_ context.Context, _ int) (int, error) { return 1, nil }

// switchableDice answers the highest face while high is set and the lowest
// otherwise. A scene flips it around the one verb whose rolls it wants to
// land -- alice's own Attack -- so her blows always hit for full damage and
// the camp's never do. Every verb runs synchronously under the test, so the
// flip is scoped exactly; atomic anyway, because -race is watching.
type switchableDice struct{ high *atomic.Bool }

func (d switchableDice) Roll(_ context.Context, size int) (int, error) {
	if d.high.Load() {
		return size, nil
	}
	return 1, nil
}

// The camp's geometry the scenes lean on, in authored [col,row] offsets.
var (
	// gateLeg walks from the party's seat to the yard: around the letter's
	// cell, through the palisade's doorway ([2,3] to [3,4]). The scout sees
	// alice at the doorway and the fight forms there.
	gateLeg = [][2]int{{1, 4}, {2, 4}, {2, 3}, {3, 4}}

	// yardFloor is every cell of the yard, for the walk to the hut to
	// route across -- around whatever stands in the way.
	yardFloor = rectOffsets(3, 0, 7, 8)

	// hutDoorFromYard and hutDoorCell are the two sides of the hut's
	// doorway: [9,5] in the yard, [10,4] in the hut.
	hutDoorFromYard = [2]int{9, 5}
	hutDoorCell     = [2]int{10, 4}

	// letterCell is where the letter appears; arrivalCells are where the
	// reinforcements do.
	letterCell   = [2]int{1, 3}
	arrivalCells = [][2]int{{1, 4}, {2, 4}, {1, 5}}
)

func rectOffsets(col0, row0, width, height int) [][2]int {
	out := make([][2]int, 0, width*height)
	for row := row0; row < row0+height; row++ {
		for col := col0; col < col0+width; col++ {
			out = append(out, [2]int{col, row})
		}
	}
	return out
}

// campRun is one launched camp with alice as the whole party.
type campRun struct {
	h     *acceptanceHarness
	sess  string
	alice context.Context
}

// launchTheCamp starts the shipped raider camp through the lobby's own
// StartEncounter -- the production launch, so the forwarding under test is
// the launch's, not the scene's -- and returns the run.
func launchTheCamp(t *testing.T) *campRun {
	t.Helper()
	return launchTheCampWith(t, alwaysTheLowestFace{})
}

func launchTheCampWith(t *testing.T, roller sdk.Roller) *campRun {
	t.Helper()

	h := newAcceptanceHarnessWithDice(t, roller)
	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	lobbies := lobbyrepo.NewInMemory()
	require.NoError(t, lobbies.Save(context.Background(), &lobbyrepo.Data{
		ID: "lobby-1", HostPlayerID: "player-alice", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{"player-alice": {
			PlayerID: "player-alice", CharacterID: "alice", IsHost: true, IsReady: true,
		}},
		MemberOrder: []string{"player-alice"},
	}))
	lobby, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            lobbies,
		LobbyBroker:          lobbyorch.NewBroker(),
		CharacterRepo:        h.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		SessionManager:       h.manager.Manager,
		Dungeons:             dungeonstest.Shipped(t),
	})
	require.NoError(t, err)

	out, err := lobby.StartEncounter(context.Background(), &lobbyorch.StartEncounterInput{
		PlayerID: "player-alice", LobbyID: "lobby-1", DungeonKey: raiderCampKey,
	})
	require.NoError(t, err, "the camp launches: its chief entered the raiders as their mind")

	return &campRun{h: h, sess: out.EncounterID, alice: auth.WithPlayerID(context.Background(), "player-alice")}
}

func (r *campRun) story(t *testing.T) []*sessionpb.Event {
	t.Helper()
	out, err := r.h.handler.GetStory(r.alice, &sessionpb.GetStoryRequest{Session: r.sess, Member: "alice"})
	require.NoError(t, err)
	return out.GetEntries()
}

// factions is the roster as id -> faction: who is in the run, and on which
// side. A member in reserve has no row.
func (r *campRun) factions(t *testing.T) map[string]string {
	t.Helper()
	roster, err := r.h.handler.GetRoster(r.alice, &sessionpb.GetRosterRequest{Session: r.sess})
	require.NoError(t, err)
	out := map[string]string{}
	for _, m := range roster.GetMembers() {
		out[m.GetId()] = m.GetFaction()
	}
	return out
}

func (r *campRun) props(t *testing.T) []string {
	t.Helper()
	atlas, err := r.h.handler.GetAtlas(r.alice, &sessionpb.GetAtlasRequest{Session: r.sess, Member: "alice"})
	require.NoError(t, err)
	out := make([]string, 0, len(atlas.GetProps()))
	for _, p := range atlas.GetProps() {
		out = append(out, p.GetId())
	}
	return out
}

func (r *campRun) round(t *testing.T) int {
	t.Helper()
	out, err := r.h.handler.Turn(r.alice, &sessionpb.TurnRequest{Session: r.sess, Member: "alice"})
	require.NoError(t, err)
	return int(out.GetRound())
}

func (r *campRun) where(t *testing.T) spatial.Position {
	t.Helper()
	out, err := r.h.handler.GetWhere(r.alice, &sessionpb.GetWhereRequest{Session: r.sess, Member: "alice"})
	require.NoError(t, err)
	return spatial.Position{X: out.GetPosition().GetX(), Y: out.GetPosition().GetY()}
}

// occupied is every cell alice can see somebody standing on.
func (r *campRun) occupied(t *testing.T) map[spatial.Position]bool {
	t.Helper()
	view, err := r.h.handler.GetView(r.alice, &sessionpb.GetViewRequest{Session: r.sess, Member: "alice"})
	require.NoError(t, err)
	out := map[spatial.Position]bool{}
	for _, s := range view.GetSightings() {
		if seen := s.GetSeen(); seen != nil && seen.GetPosition() != nil {
			out[spatial.Position{X: seen.GetPosition().GetX(), Y: seen.GetPosition().GetY()}] = true
		}
	}
	return out
}

func (r *campRun) endTurn(t *testing.T) *sessionpb.EndTurnResponse {
	t.Helper()
	out, err := r.h.handler.EndTurn(r.alice, &sessionpb.EndTurnRequest{
		Session: r.sess, Member: "alice",
		DeclarationId: currentDeclarationID(r.alice, t, r.h.handler, r.sess, "alice", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err)
	return out
}

func (r *campRun) ended(t *testing.T) bool {
	t.Helper()
	return beatIndex(r.story(t), sessionpb.EventKind_EVENT_KIND_ENDED) != -1
}

// intoTheYard walks the gate leg on the world clock. The scout sees alice at
// the palisade's doorway and the camp -- hostile until its chief learns
// better -- forms a fight on sight.
func (r *campRun) intoTheYard(t *testing.T) {
	t.Helper()
	path := make([]*sessionpb.Position, 0, len(gateLeg))
	for _, c := range gateLeg {
		path = append(path, pbAt(c[0], c[1]))
	}
	moved, err := r.h.handler.Move(r.alice, &sessionpb.MoveRequest{Session: r.sess, Member: "alice", Path: path})
	require.NoError(t, err)
	require.NotNil(t, moved.GetFormed(), "the camp attacks on sight: a fight formed on the way in")
	require.Contains(t, moved.GetFormed().GetOrder(), "scout", "and it is the yard's")
	require.Equal(t, at(3, 4), r.where(t), "alice stands in the doorway, in the yard")
}

// holdOutUntil ends alice's turn -- the camp takes its own inside each
// EndTurn -- until the fight has started round n.
func (r *campRun) holdOutUntil(t *testing.T, n int) {
	t.Helper()
	for guard := 0; r.round(t) < n; guard++ {
		require.Less(t, guard, 2*n, "the rounds must advance")
		r.endTurn(t)
	}
}

// walkToTheHut takes alice from wherever she stands in the yard through the
// hut's doorway, one declared move of up to a fighter's 30 ft per turn,
// routing around whoever stands in the way, until she is in the hut or the
// run has ended. The camp's turns run inside each EndTurn.
func (r *campRun) walkToTheHut(t *testing.T) {
	t.Helper()
	grid := spatial.NewAxialHexGrid(spatial.AxialHexGridConfig{SpanWidth: 1e6, SpanHeight: 1e6})
	floor := map[spatial.Position]bool{}
	for _, c := range yardFloor {
		floor[at(c[0], c[1])] = true
	}
	door := at(hutDoorFromYard[0], hutDoorFromYard[1])
	hut := at(hutDoorCell[0], hutDoorCell[1])

	for turn := 0; turn < 8 && !r.ended(t); turn++ {
		from := r.where(t)
		if from == hut {
			return
		}
		var path []spatial.Position
		if from == door {
			path = []spatial.Position{hut}
		} else {
			path = append(shortestPath(t, grid, floor, r.occupied(t), from, door), hut)
		}
		if len(path) > 6 {
			path = path[:6]
		}
		pbPath := make([]*sessionpb.Position, 0, len(path))
		for _, c := range path {
			pbPath = append(pbPath, &sessionpb.Position{X: c.X, Y: c.Y})
		}
		_, err := r.h.handler.Move(r.alice, &sessionpb.MoveRequest{
			Session: r.sess, Member: "alice", Path: pbPath,
			DeclarationId: currentDeclarationID(r.alice, t, r.h.handler, r.sess, "alice", sessionpb.Verb_VERB_MOVE),
		})
		require.NoError(t, err, "turn %d: walking %v", turn, path)
		if r.ended(t) || r.where(t) == hut {
			return
		}
		r.endTurn(t)
	}
}

// shortestPath is a breadth-first walk over floor, avoiding occupied cells,
// from one cell to another; the path excludes from and includes to.
func shortestPath(
	t *testing.T, grid spatial.Grid, floor, occupied map[spatial.Position]bool, from, to spatial.Position,
) []spatial.Position {
	t.Helper()
	prev := map[spatial.Position]spatial.Position{}
	seen := map[spatial.Position]bool{from: true}
	queue := []spatial.Position{from}
	for len(queue) > 0 && !seen[to] {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range grid.GetNeighbors(cur) {
			if !floor[n] || seen[n] || (occupied[n] && n != to) {
				continue
			}
			seen[n], prev[n] = true, cur
			queue = append(queue, n)
		}
	}
	require.True(t, seen[to], "no walk across the yard from %v reaches %v around %v", from, to, occupied)
	var reversed []spatial.Position
	for c := to; c != from; c = prev[c] {
		reversed = append(reversed, c)
	}
	path := make([]spatial.Position, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		path = append(path, reversed[i])
	}
	return path
}

// beatIndex is where a beat of one kind first appears in a story, or -1.
func beatIndex(story []*sessionpb.Event, kind sessionpb.EventKind) int {
	for i, e := range story {
		if e.GetKind() == kind {
			return i
		}
	}
	return -1
}

// arrivals is every ARRIVED beat in a story, by placement id.
func arrivals(story []*sessionpb.Event) map[string]*sessionpb.Arrived {
	out := map[string]*sessionpb.Arrived{}
	for _, e := range story {
		if e.GetKind() == sessionpb.EventKind_EVENT_KIND_ARRIVED {
			out[e.GetArrived().GetId()] = e.GetArrived()
		}
	}
	return out
}

// TestAcceptance_TheLetterArrivesAtRoundSixAndNotBefore is A5 on the wire:
// a placement in reserve is on no map and in no beat until its predicate
// holds, then it is placed where the author drew it with an ARRIVED beat,
// and it can be held.
func TestAcceptance_TheLetterArrivesAtRoundSixAndNotBefore(t *testing.T) {
	r := launchTheCamp(t)
	require.NotContains(t, r.props(t), "letter", "in reserve at frame one: on nobody's map")

	r.intoTheYard(t)
	for round := r.round(t); round < 6; round = r.round(t) {
		require.NotContains(t, r.props(t), "letter", "round %d: not yet", round)
		require.Empty(t, arrivals(r.story(t)), "round %d: nothing has arrived", round)
		r.endTurn(t)
	}
	require.Equal(t, 6, r.round(t))

	// Placed on the first verb after the predicate holds -- the round's own
	// start counts, or the next verb does.
	if len(arrivals(r.story(t))) == 0 {
		r.endTurn(t)
	}
	letter, arrived := arrivals(r.story(t))["letter"]
	require.True(t, arrived, "the letter arrived at round 6")
	require.Equal(t, sessionpb.PlacementKind_PLACEMENT_KIND_PROP, letter.GetKind())
	require.Equal(t, at(letterCell[0], letterCell[1]).X, letter.GetCell().GetX(), "where the author drew it")
	require.Equal(t, at(letterCell[0], letterCell[1]).Y, letter.GetCell().GetY())
	require.Contains(t, r.props(t), "letter", "and it is on the map now")

	_, err := r.h.handler.Hold(r.alice, &sessionpb.HoldRequest{
		Session: r.sess, Member: "alice", Target: "letter", Range: holdReach,
	})
	require.NoError(t, err, "and it can be held, like any prop that was always there")
	require.NotContains(t, r.props(t), "letter")
}

// TestAcceptance_TheLetterCarriedToTheChiefTurnsTheCamp is A2 as the step-B
// walk: hold out to round 6, fetch the letter the Wiseman sent, carry it to
// the chief mid-fight. On the wire that is, in order: FIGHT_STARTED,
// ARRIVED (the letter), HELD, STANCE_CHANGED, FIGHT_ENDED carrying
// BY_STANCE, ENDED naming hold-out.
//
// The fight is the SCOUT'S, and it has to be: inside the hut the letter's
// holder standing in the mind's region teaches the mind in the same pass
// that would have let the chief see her, and predicates fold BEFORE that
// pass's sight refresh (design §3.8) -- so by the time the chief could form
// a fight, the camp is already neutral.
func TestAcceptance_TheLetterCarriedToTheChiefTurnsTheCamp(t *testing.T) {
	r := launchTheCamp(t)

	// The roster says which side everyone is on, in the file's own words --
	// and the three reinforcements in reserve have no row.
	factions := r.factions(t)
	require.Equal(t, map[string]string{"alice": "party", "chief": "raiders", "scout": "raiders"}, factions,
		"the party, the chief and the scout; nobody who is still in reserve")

	r.intoTheYard(t)
	r.holdOutUntil(t, 6)
	if _, arrived := arrivals(r.story(t))["letter"]; !arrived {
		r.endTurn(t)
	}
	require.Contains(t, arrivals(r.story(t)), "letter")

	// The letter lies at the gate, two cells behind alice in the doorway.
	// Holding is not knowing: a fighter is not the camp's mind.
	_, err := r.h.handler.Hold(r.alice, &sessionpb.HoldRequest{
		Session: r.sess, Member: "alice", Target: "letter", Range: holdReach,
	})
	require.NoError(t, err)
	require.Equal(t, -1, beatIndex(r.story(t), sessionpb.EventKind_EVENT_KIND_STANCE_CHANGED),
		"the camp has not turned: the letter is in the wrong hands")

	r.walkToTheHut(t)
	require.True(t, r.ended(t), "the run ended when the letter reached the chief's hut")

	entries := r.story(t)
	fightStarted := beatIndex(entries, sessionpb.EventKind_EVENT_KIND_FIGHT_STARTED)
	arrivedAt := beatIndex(entries, sessionpb.EventKind_EVENT_KIND_ARRIVED)
	held := beatIndex(entries, sessionpb.EventKind_EVENT_KIND_HELD)
	stanceChanged := beatIndex(entries, sessionpb.EventKind_EVENT_KIND_STANCE_CHANGED)
	fightEnded := beatIndex(entries, sessionpb.EventKind_EVENT_KIND_FIGHT_ENDED)
	endedAt := beatIndex(entries, sessionpb.EventKind_EVENT_KIND_ENDED)
	require.NotEqual(t, -1, stanceChanged, "the camp turned")
	require.Less(t, fightStarted, arrivedAt, "mid-fight: the letter arrived while the fight was on")
	require.Less(t, arrivedAt, held, "and was picked up after it arrived")
	require.Less(t, held, stanceChanged, "carried to the chief")
	require.Less(t, stanceChanged, fightEnded, "the stance turns, THEN the fight dissolves")
	require.Less(t, fightEnded, endedAt, "and then the hold-out ends the run")

	stance := entries[stanceChanged].GetStanceChanged()
	require.Equal(t, []string{"party", "raiders"}, stance.GetBetween(), "the pair, sorted, in the file's words")
	require.Equal(t, "neutral", stance.GetStance(), "not hostile -- design R2")
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_STANCE, entries[fightEnded].GetFightEnded().GetCause(),
		"the fight ended because its sides stopped being sides, and the wire says so")
	require.Equal(t, "hold-out", entries[endedAt].GetEnded().GetEnding(),
		"the scenario's own key, translated by nobody")

	// Every recipient hears the stance turn, monsters included (design §6).
	// The chief's story is read through the Manager: no player owns a
	// monster's view.
	chiefsStory, err := r.h.manager.Manager.Story(context.Background(), &sdk.StoryInput{Session: r.sess, Member: "chief"})
	require.NoError(t, err)
	chiefHeard := false
	for _, e := range chiefsStory {
		if e.Kind == sdk.EventStanceChanged {
			chiefHeard = true
		}
	}
	require.True(t, chiefHeard, "the chief was told the camp turned -- nothing on this side narrows the audience")

	status, err := r.h.handler.GetStatus(r.alice, &sessionpb.GetStatusRequest{Session: r.sess})
	require.NoError(t, err)
	require.False(t, status.GetOpen(), "the hold-out is over")
}

// TestAcceptance_TheChiefsFallBringsTheReinforcements is A4 on the wire:
// three placements waiting on `{ down: chief }` are in reserve -- no roster
// row, no cell -- until the chief goes down, then each arrives at the gate
// where the author drew it with an ARRIVED beat, as a raider, and the
// fight already running in the yard keeps running.
func TestAcceptance_TheChiefsFallBringsTheReinforcements(t *testing.T) {
	r := launchTheCamp(t)
	require.Len(t, r.factions(t), 3, "alice, the chief, the scout -- the reinforcements are in reserve")

	r.intoTheYard(t)

	// The chief falls. A body is a body because its sheet says so: the
	// session's standing seam reads it, and the world notices the fall at
	// the next verb.
	sessions := sessionorch.NewSessionRepository(r.h.redis, time.Hour)
	stored, err := sessions.GetSession(context.Background(), r.sess)
	require.NoError(t, err)
	felled := false
	for i := range stored.NPCs {
		if stored.NPCs[i].ID == "chief" {
			stored.NPCs[i].HitPoints = 0
			felled = true
		}
	}
	require.True(t, felled, "the chief's sheet is in the session under his authored id")
	require.NoError(t, sessions.SaveSession(context.Background(), stored))

	// Placed on the first verb after the fall is noticed.
	for verbs := 0; verbs < 2 && len(arrivals(r.story(t))) < 3; verbs++ {
		r.endTurn(t)
	}
	came := arrivals(r.story(t))
	require.Len(t, came, 3, "three reinforcements arrived, each with its own beat")
	wantCells := map[spatial.Position]bool{}
	for _, c := range arrivalCells {
		wantCells[at(c[0], c[1])] = true
	}
	for _, id := range []string{"reinforcement-1", "reinforcement-2", "reinforcement-3"} {
		a, arrived := came[id]
		require.True(t, arrived, "%s arrived", id)
		require.Equal(t, sessionpb.PlacementKind_PLACEMENT_KIND_MONSTER, a.GetKind())
		require.True(t, wantCells[spatial.Position{X: a.GetCell().GetX(), Y: a.GetCell().GetY()}],
			"%s stands where the author drew it, at the gate", id)
	}

	factions := r.factions(t)
	require.Len(t, factions, 6, "the roster grew by three")
	for _, id := range []string{"reinforcement-1", "reinforcement-2", "reinforcement-3"} {
		require.Equal(t, "raiders", factions[id], "%s arrived as a raider", id)
	}

	require.Equal(t, -1, beatIndex(r.story(t), sessionpb.EventKind_EVENT_KIND_FIGHT_ENDED),
		"the yard's fight goes on: a fall on the other side is not a stance")
	status, err := r.h.handler.GetStatus(r.alice, &sessionpb.GetStatusRequest{Session: r.sess})
	require.NoError(t, err)
	require.True(t, status.GetOpen(), "and the run is not over -- the camp never learned anything")
}

// TestAcceptance_TheChiefFelledByThePartyBringsTheReinforcements is A4 by
// the walk's own path: no sheet is edited -- alice walks into the hut and
// cuts the chief down herself through the gRPC surface, and the three
// zombies arrive INSIDE THAT BLOW'S OWN RECORD, in the same verb, before
// anybody takes another turn. This is the path the session's walk-4 fix
// repaired: the seams rebuilt after resolution handed the world back had
// dropped the reserve, so participation was asked about members it had
// never been told of.
func TestAcceptance_TheChiefFelledByThePartyBringsTheReinforcements(t *testing.T) {
	high := &atomic.Bool{}
	r := launchTheCampWith(t, switchableDice{high: high})
	require.Len(t, r.factions(t), 3, "the reinforcements are in reserve")

	r.intoTheYard(t)
	r.walkToTheHut(t)
	require.Equal(t, at(hutDoorCell[0], hutDoorCell[1]), r.where(t), "alice stands in the hut, in the chief's sight")

	// Cut the chief down. He closes in on his own turns; alice swings when
	// he is in reach, and only her rolls land.
	felled := false
	for turn := 0; turn < 12 && !felled; turn++ {
		if r.canAttack(t, "chief") {
			high.Store(true)
			swing, err := r.h.handler.Attack(r.alice, &sessionpb.AttackRequest{
				Session: r.sess, Attacker: "alice", Target: "chief",
				DeclarationId: currentDeclarationID(r.alice, t, r.h.handler, r.sess, "alice", sessionpb.Verb_VERB_ATTACK),
			})
			high.Store(false)
			require.NoError(t, err)
			require.True(t, swing.GetHit(), "the highest face always lands")
			if r.isDown(t, "chief") {
				felled = true
				break
			}
		}
		r.endTurn(t)
	}
	require.True(t, felled, "alice's own blow felled the chief")

	// Inside that blow's record: three ARRIVED beats, as raiders, at the
	// gate where the author drew them -- no further verb was needed.
	came := arrivals(r.story(t))
	require.Len(t, came, 3, "three reinforcements arrived in the same verb that felled the chief")
	wantCells := map[spatial.Position]bool{}
	for _, c := range arrivalCells {
		wantCells[at(c[0], c[1])] = true
	}
	for _, id := range []string{"reinforcement-1", "reinforcement-2", "reinforcement-3"} {
		a, arrived := came[id]
		require.True(t, arrived, "%s arrived", id)
		require.Equal(t, sessionpb.PlacementKind_PLACEMENT_KIND_MONSTER, a.GetKind())
		require.True(t, wantCells[spatial.Position{X: a.GetCell().GetX(), Y: a.GetCell().GetY()}],
			"%s stands where the author drew it", id)
	}
	factions := r.factions(t)
	require.Len(t, factions, 6, "the roster grew by three")
	for _, id := range []string{"reinforcement-1", "reinforcement-2", "reinforcement-3"} {
		require.Equal(t, "raiders", factions[id])
	}

	// And the world goes on: the scout is still up, the run is still open,
	// and the next verb is accepted -- which is where the walk-4 defect
	// used to surface.
	require.Equal(t, -1, beatIndex(r.story(t), sessionpb.EventKind_EVENT_KIND_FIGHT_ENDED))
	status, err := r.h.handler.GetStatus(r.alice, &sessionpb.GetStatusRequest{Session: r.sess})
	require.NoError(t, err)
	require.True(t, status.GetOpen())
	r.endTurn(t)
}

// canAttack reports whether member is an available target of alice's Attack
// right now, by the declaration the session itself offers her.
func (r *campRun) canAttack(t *testing.T, member string) bool {
	t.Helper()
	out, err := r.h.handler.Afford(r.alice, &sessionpb.AffordRequest{Session: r.sess, Member: "alice"})
	require.NoError(t, err)
	for _, d := range out.GetDeclarations() {
		if d.GetVerb() != sessionpb.Verb_VERB_ATTACK || !d.GetAvailable() {
			continue
		}
		for _, c := range d.GetCandidates() {
			if c.GetMember() == member && c.GetAvailable() {
				return true
			}
		}
	}
	return false
}

// isDown reports whether the story has told alice that member went down.
func (r *campRun) isDown(t *testing.T, member string) bool {
	t.Helper()
	for _, e := range r.story(t) {
		if e.GetKind() == sessionpb.EventKind_EVENT_KIND_DOWNED && e.GetDowned().GetMember() == member {
			return true
		}
	}
	return false
}
