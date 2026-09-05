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
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// holdout_acceptance_test.go is the hold-out's scene A2 (rpg-project#375,
// design §9) at this repo's own level: the whole thing through the launch
// path and the gRPC surface, on the SHIPPED raider camp, with nothing
// spawned or seeded by the test itself.
//
// The party comes in at the gate. The Wiseman's letter lies at their feet;
// they Hold it. They walk east through the yard into the chief's hut; the
// chief sees them and a fight forms; in the same pass, the letter's holder
// standing in the mind's region teaches the mind the fact the letter
// reveals, the camp's stance toward the party folds to neutral, the fight
// between them dissolves because its sides stopped being sides, and the
// hold-out ends the run. On the wire that is, in order: FIGHT_STARTED,
// STANCE_CHANGED, FIGHT_ENDED carrying BY_STANCE, ENDED naming hold-out.

// raiderCampKey is the shipped fixture's key.
const raiderCampKey = "reference-raider-camp"

// alwaysTheLowestFace answers 1 to every roll, so the camp never lands a
// blow while the party walks. This scene is about whether a stance change
// reaches the wire, and who wins a fight is not its question (the lobby's
// loot scene made the same call, for the same reason).
type alwaysTheLowestFace struct{}

func (alwaysTheLowestFace) Roll(_ context.Context, _ int) (int, error) { return 1, nil }

// campRoute is the walk from the party's seat at the gate to the inside of
// the hut, in authored [col,row] offsets: around the letter's cell, through
// the palisade's doorway ([2,3] to [3,4]), east along the yard, and through
// the hut's doorway ([9,5] to [10,4]). Every step is a hex neighbor and
// every wall crossing is a door; the composition refuses anything else, so
// a wrong step here fails the walk rather than teleporting.
var campRoute = [][2]int{
	{1, 4}, {2, 4}, {2, 3}, {3, 4}, {4, 4}, {5, 4}, {6, 4}, {7, 4}, {8, 4}, {8, 5}, {9, 5}, {10, 4}, {11, 4},
}

// launchTheCamp starts the shipped raider camp through the lobby's own
// StartEncounter -- the production launch, so the forwarding under test is
// the launch's, not the scene's -- with alice as the whole party, and
// returns the session id.
func launchTheCamp(t *testing.T, h *acceptanceHarness) string {
	t.Helper()

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

	return out.EncounterID
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

func TestAcceptance_TheLetterCarriedToTheChiefTurnsTheCamp(t *testing.T) {
	h := newAcceptanceHarnessWithDice(t, alwaysTheLowestFace{})
	sess := launchTheCamp(t, h)
	alice := auth.WithPlayerID(context.Background(), "player-alice")

	// The roster says which side everyone is on, in the file's own words.
	roster, err := h.handler.GetRoster(alice, &sessionpb.GetRosterRequest{Session: sess})
	require.NoError(t, err)
	factions := map[string]string{}
	for _, m := range roster.GetMembers() {
		factions[m.GetId()] = m.GetFaction()
	}
	require.Equal(t, "raiders", factions["chief"], "the chief's row carries his faction")
	require.Equal(t, "raiders", factions["scout"])
	require.Equal(t, "party", factions["alice"], "and the party's carries the reserved word")

	// Hold the letter at the gate. Nobody has learned anything: holding is
	// not knowing, and a fighter is not the camp's mind.
	_, err = h.handler.Hold(alice, &sessionpb.HoldRequest{
		Session: sess, Member: "alice", Target: "letter", Range: holdReach,
	})
	require.NoError(t, err)
	before, err := h.handler.GetStory(alice, &sessionpb.GetStoryRequest{Session: sess, Member: "alice"})
	require.NoError(t, err)
	require.NotEqual(t, -1, beatIndex(before.GetEntries(), sessionpb.EventKind_EVENT_KIND_HELD))
	require.Equal(t, -1, beatIndex(before.GetEntries(), sessionpb.EventKind_EVENT_KIND_STANCE_CHANGED),
		"the camp has not turned: the letter is in the wrong hands")

	// Walk into the hut. On the world clock the walk runs until the scout
	// in the yard sees alice and a fight forms -- the camp is hostile, and
	// attacks on sight; from then on each turn is one declared move of up
	// to a fighter's 30 ft, and the camp's own turns run inside EndTurn.
	// The walk stops the moment the run ends.
	//
	// The fight is the SCOUT'S, and it has to be: inside the hut the
	// letter's holder standing in the mind's region teaches the mind in
	// the same pass that would have let the chief see her, and predicates
	// fold BEFORE that pass's sight refresh (design §3.8) -- so by the
	// time the chief could form a fight, the camp is already neutral. A2's
	// "mid-fight" is a fight the camp started before the letter arrived.
	route := make([]*sessionpb.Position, 0, len(campRoute))
	for _, c := range campRoute {
		route = append(route, pbAt(c[0], c[1]))
	}
	walked := 0
	inFight := false
	ended := false
	for turn := 0; turn < 8 && walked < len(route) && !ended; turn++ {
		req := &sessionpb.MoveRequest{Session: sess, Member: "alice", Path: route[walked:]}
		if inFight {
			req.Path = route[walked:min(walked+6, len(route))]
			req.DeclarationId = currentDeclarationID(alice, t, h.handler, sess, "alice", sessionpb.Verb_VERB_MOVE)
		}
		moved, moveErr := h.handler.Move(alice, req)
		require.NoError(t, moveErr, "turn %d: walking from step %d", turn, walked)
		walked += len(moved.GetSteps())
		if moved.GetFormed() != nil {
			inFight = true
			require.Contains(t, moved.GetFormed().GetOrder(), "scout", "the fight that forms is the yard's")
			t.Logf("turn %d: the fight formed after step %d, order %v", turn, walked, moved.GetFormed().GetOrder())
		}
		story, storyErr := h.handler.GetStory(alice, &sessionpb.GetStoryRequest{Session: sess, Member: "alice"})
		require.NoError(t, storyErr)
		if beatIndex(story.GetEntries(), sessionpb.EventKind_EVENT_KIND_ENDED) != -1 {
			ended = true
			break
		}
		if inFight {
			_, endErr := h.handler.EndTurn(alice, &sessionpb.EndTurnRequest{
				Session: sess, Member: "alice",
				DeclarationId: currentDeclarationID(alice, t, h.handler, sess, "alice", sessionpb.Verb_VERB_END_TURN),
			})
			require.NoError(t, endErr, "turn %d: ending alice's turn", turn)
		}
	}
	{
		story, storyErr := h.handler.GetStory(alice, &sessionpb.GetStoryRequest{Session: sess, Member: "alice"})
		require.NoError(t, storyErr)
		kinds := make([]string, 0, len(story.GetEntries()))
		for _, e := range story.GetEntries() {
			kinds = append(kinds, e.GetKind().String())
		}
		t.Logf("walked %d/%d steps; alice's story: %v", walked, len(route), kinds)
	}
	require.True(t, inFight, "a fight formed on the way in: the camp was hostile until the chief learned better")
	require.True(t, ended, "the run ended before the walk did (alice walked %d of %d steps)", walked, len(route))

	// The order of things on the wire, in alice's story.
	story, err := h.handler.GetStory(alice, &sessionpb.GetStoryRequest{Session: sess, Member: "alice"})
	require.NoError(t, err)
	entries := story.GetEntries()
	fightStarted := beatIndex(entries, sessionpb.EventKind_EVENT_KIND_FIGHT_STARTED)
	stanceChanged := beatIndex(entries, sessionpb.EventKind_EVENT_KIND_STANCE_CHANGED)
	fightEnded := beatIndex(entries, sessionpb.EventKind_EVENT_KIND_FIGHT_ENDED)
	endedAt := beatIndex(entries, sessionpb.EventKind_EVENT_KIND_ENDED)
	require.NotEqual(t, -1, stanceChanged, "the camp turned")
	require.Less(t, fightStarted, stanceChanged, "mid-fight: the fight was on before the camp turned")
	require.Less(t, stanceChanged, fightEnded, "the stance turns, THEN the fight dissolves")
	require.Less(t, fightEnded, endedAt, "and then the hold-out ends the run")

	stance := entries[stanceChanged].GetStanceChanged()
	require.Equal(t, []string{"party", "raiders"}, stance.GetBetween(), "the pair, sorted, in the file's words")
	require.Equal(t, "neutral", stance.GetStance(), "not hostile -- design R2")
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_STANCE, entries[fightEnded].GetFightEnded().GetCause(),
		"the fight ended because its sides stopped being sides, and the wire says so")
	require.Equal(t, "hold-out", entries[endedAt].GetEnded().GetEnding(),
		"the scenario's own key, translated by nobody")

	// Every recipient hears the stance turn, monsters included (design §6:
	// a stance is truth grain, like a door's state). The chief's story is
	// read through the Manager because no player owns a monster's view.
	chiefsStory, err := h.manager.Manager.Story(context.Background(), &sdk.StoryInput{Session: sess, Member: "chief"})
	require.NoError(t, err)
	chiefHeard := false
	for _, e := range chiefsStory {
		if e.Kind == sdk.EventStanceChanged {
			chiefHeard = true
		}
	}
	require.True(t, chiefHeard, "the chief was told the camp turned -- nothing on this side narrows the audience")

	status, err := h.handler.GetStatus(alice, &sessionpb.GetStatusRequest{Session: sess})
	require.NoError(t, err)
	require.False(t, status.GetOpen(), "the hold-out is over")
}
