package session_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// storyEventsJSON spells acts for a failure message: what a reader needs to
// see when an assertion about the act stream does not hold.
func storyEventsJSON(acts []*storypb.Act) string {
	var b strings.Builder
	for _, act := range acts {
		b.WriteString(protojson.Format(act))
		b.WriteString("\n")
	}
	return b.String()
}

// clockActsOf is every clock act in acts carrying mark.
func clockActsOf(acts []*storypb.Act, mark storypb.ClockMark) []*storypb.ClockEntry {
	var out []*storypb.ClockEntry
	for _, act := range acts {
		if clock := act.GetClock(); clock != nil && clock.GetMark() == mark {
			out = append(out, clock)
		}
	}
	return out
}

// TestGetStoryMatchesLiveEvents is rpg-api-protos#239's acceptance criterion
// restated on the act stream (rpg-project#631): through the real, wired stack
// (real Manager, real act broker, real StoryService StreamEvents and
// GetStory), what a member receives LIVE over a driven turn is, act for act,
// what GetStory answers afterwards -- one fold serves both.
//
// Alice subscribes live BEFORE the skeleton's turn is driven, so her live
// stream captures the whole driven turn (her own turn passing, the
// skeleton's pick and swing, its own turn passing) exactly as StreamEvents
// delivers it in production.
func TestGetStoryMatchesLiveEvents(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	const session = "story-events-run"

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	// The skeleton stands on the tomb's pillar-gap sight row and alice seats
	// inside the tomb on that row, three cells out, so the fight forms at
	// launch with a turn order this test can predict.
	launched := h.launch(t, session,
		withMonsters(buildThreeRoomTomb(t), monsterAt("skel-1", refs.Monsters.Skeleton().String(), 19, 3)),
		seatAt("alice", 16, 3))
	require.Len(t, launched.Formed, 1, "in sight and in range at first light: the fight forms at launch")
	require.Equal(t, []string{"alice", "skel-1"}, launched.Formed[0].Order,
		"the tied roll breaks to alice -- it must be her turn first, not the skeleton's")

	// Alice has nothing to do this turn, so ending immediately is the honest
	// move, and the skeleton's whole turn drives inside this one call.
	var endResp *sessionpb.EndTurnResponse
	live := watchActs(ctx, t, h, session, "alice", func() {
		endResp, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{
			Session: session, Member: "alice",
			DeclarationId: currentDeclarationID(ctx, t, h.handler, session, "alice", sessionpb.Verb_VERB_END_TURN),
		})
		require.NoError(t, err, "the skeleton's whole turn drives inside this one call")
	})
	require.Equal(t, "alice", endResp.GetNext(), "a two-member fight wraps straight back to whoever led it")

	// Alice's own turn passing and the skeleton's own both reach her live.
	passed := map[string]bool{}
	for _, clock := range clockActsOf(live, storypb.ClockMark_CLOCK_MARK_TURN_ENDED) {
		passed[clock.GetTurn().GetMember().GetId()] = true
	}
	require.True(t, passed["alice"] && passed["skel-1"],
		"alice's own end-turn and the skeleton's own must both arrive as turn-ended clock acts:\n%s", storyEventsJSON(live))

	var acted bool
	for _, act := range live {
		if act.GetWalk().GetMover().GetId() == "skel-1" || act.GetAttack().GetBy().GetId() == "skel-1" {
			acted = true
		}
	}
	require.True(t, acted, "the skeleton had to close the gap or swing:\n%s", storyEventsJSON(live))

	// Catch up from the very start: the live acts are exactly GetStory's tail.
	requireLiveIsStory(ctx, t, h, session, "alice", live)

	// GetStory from the start carries alice's own arrival -- the join's
	// audience is every current member including the joiner, so her own
	// joining is hers to read back even though no live subscriber of hers
	// existed yet to receive it.
	var sawAliceJoined bool
	for _, clock := range clockActsOf(storyOf(ctx, t, h.story, session, "alice"), storypb.ClockMark_CLOCK_MARK_JOINED) {
		if clock.GetMember().GetId() == "alice" {
			sawAliceJoined = true
		}
	}
	require.True(t, sawAliceJoined, "alice's own join must reach her story as a joined clock act naming her")
}

// storyRestartedStream records a StoryService StreamEvents call's messages
// in order, including the StoryRestarted marker actStream does not keep.
type storyRestartedStream struct {
	*actStream
	restartedFirst bool
	sawRestarted   bool
}

func (s *storyRestartedStream) Send(msg *storypb.StreamEventsResponse) error {
	if msg.GetRestarted() != nil {
		s.mu.Lock()
		s.sawRestarted = true
		s.restartedFirst = s.messages == 0
		s.mu.Unlock()
	}
	return s.actStream.Send(msg)
}

// TestGetStoryAgedOutReturnsOutOfRange proves what a resume point that aged
// out of the retention window answers: never a short or empty page a client
// could mistake for "you are already caught up."
//
// THE ANSWER CHANGED WITH THE ACT STREAM (rpg-project#631, R25). The v1
// GetStory refused an aged-out from_seq with OUT_OF_RANGE. The v2
// StoryService answers it instead: GetStory returns restarted=true with the
// acts from the oldest retained beat, and StreamEvents sends StoryRestarted
// FIRST and then that same replay -- the client replaces what it holds
// rather than appending. The name is kept so the suite's history still reads
// as one test; the claim is the v2 one.
//
// buildThreeRoomTomb's own world carries RetentionUnbounded, so this test
// narrows its own stored run to a small window after the launch.
func TestGetStoryAgedOutReturnsOutOfRange(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	const session = "story-trimmed-run"

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	h.launch(t, session, buildThreeRoomTomb(t), seatAt("alice", 1, 2))

	// The resume point a client would hold: the first act alice has.
	first := storyOf(ctx, t, h.story, session, "alice")
	require.NotEmpty(t, first, "the launch gives alice a story to resume from")
	resumeFrom := first[0].GetId()

	repo := sessionorch.NewEncounterRepository(h.redis, 0)
	stored, err := repo.GetEncounter(ctx, session)
	require.NoError(t, err)
	stored.Retention = 2
	require.NoError(t, repo.SaveEncounter(ctx, session, stored))

	// Outside a fight, world-clock movement is unpriced, so alice can walk
	// back and forth freely -- each Move appends at least one beat, well past
	// the two-entry retention window in only a few calls.
	waypoints := []*sessionpb.Position{pbAt(2, 2), pbAt(1, 2)}
	for i := 0; i < 6; i++ {
		_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
			Session: session, Member: "alice",
			Path: []*sessionpb.Position{waypoints[i%2]},
		})
		require.NoError(t, err)
	}

	// GetStory: told, and the replay starts past the aged-out id.
	page, err := h.story.GetStory(ctx, &storypb.GetStoryRequest{Session: session, Member: "alice", AfterId: resumeFrom})
	require.NoError(t, err)
	require.True(t, page.GetRestarted(), "an aged-out resume point is TOLD, not answered as if caught up")
	require.NotEmpty(t, page.GetActs(), "a restart replays from the oldest retained beat")
	require.Greater(t, page.GetActs()[0].GetId(), resumeFrom, "the aged-out act is no longer retained")
	retained := storyOf(ctx, t, h.story, session, "alice")
	require.True(t, proto.Equal(retained[0], page.GetActs()[0]),
		"the restart's replay is the retained story from its oldest act")

	// A resume point still inside the window is not a restart.
	inside, err := h.story.GetStory(ctx, &storypb.GetStoryRequest{Session: session, Member: "alice", AfterId: retained[0].GetId()})
	require.NoError(t, err)
	require.False(t, inside.GetRestarted(), "a retained resume point appends, it does not restart")

	// StreamEvents: StoryRestarted goes out FIRST, then the same replay.
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := &storyRestartedStream{actStream: newActStream(streamCtx)}
	done := make(chan error, 1)
	go func() {
		done <- h.story.StreamEvents(&storypb.StreamEventsRequest{Session: session, Member: "alice", AfterId: resumeFrom}, stream)
	}()
	replayed := stream.settle(t, len(retained), 2*time.Second)
	require.True(t, stream.sawRestarted, "a stale stream resume sends StoryRestarted")
	require.True(t, stream.restartedFirst, "StoryRestarted precedes every act of the replay")
	require.Len(t, replayed, len(retained), "the stream's replay is the whole retained story")
	for i := range retained {
		require.True(t, proto.Equal(retained[i], replayed[i]), "replayed act %d differs from GetStory's", i)
	}
	cancel()
	require.NoError(t, <-done)
}
