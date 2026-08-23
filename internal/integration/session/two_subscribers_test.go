package session_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// recordingStream captures, in order, every *sessionpb.Event a real
// StreamEvents call sends it -- a genuine subscriber all the way through
// the real handler and the real Broker, not a mock of either. It always
// drains promptly (matching a healthy client's own read loop); THIS suite's
// own lagging half is played by a raw Broker.Subscribe handle instead (see
// TestTwoStreamEventsSubscribers), because Broker.Publish makes no
// distinction between a stalled client connection and a channel nobody
// reads -- both are just an unread chan sdk.Event, keyed the same way.
type recordingStream struct {
	grpc.ServerStream
	ctx context.Context

	mu     sync.Mutex
	events []*sessionpb.Event
}

func newRecordingStream(ctx context.Context) *recordingStream {
	return &recordingStream{ctx: ctx}
}

func (s *recordingStream) Context() context.Context { return s.ctx }

func (s *recordingStream) Send(evt *sessionpb.Event) error {
	s.mu.Lock()
	s.events = append(s.events, evt)
	s.mu.Unlock()
	return nil
}

func (s *recordingStream) snapshot() []*sessionpb.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*sessionpb.Event, len(s.events))
	copy(out, s.events)
	return out
}

// waitForLive republishes a throwaway canary event directly on the broker
// until it shows up on stream -- the same technique the handler's own
// stream_events_test.go uses (waitForPublishedEvent), necessary because the
// Subscribe call happens inside the goroutine StreamEvents runs on, with no
// other signal for exactly when it has registered.
func waitForLive(t *testing.T, broker *sessionorch.Broker, session, recipient string, stream *recordingStream) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if len(stream.snapshot()) > 0 {
			// The subscription is live. A tick that fired before this one
			// was observed may still have a duplicate canary in flight --
			// settle briefly so the caller's own baseline (captured right
			// after this returns) is taken after every straggler has
			// landed, not before.
			time.Sleep(50 * time.Millisecond)
			return
		}
		select {
		case <-ticker.C:
			_ = broker.Publish(context.Background(), []sdk.Event{
				{Session: session, Recipient: recipient, Seq: 0, Kind: sdk.EventUnknown},
			})
		case <-deadline:
			t.Fatalf("stream for %s never subscribed", recipient)
		}
	}
}

// assertContiguousSeqs fails unless the events, in the order received, carry
// strictly increasing, gapless Seq values -- the same "no gap, no
// duplicate" contract the toolkit's own two_players_test.go pins at the SDK
// level, checked here again one layer up, through the real handler and the
// real Broker.
func assertContiguousSeqs(t *testing.T, events []*sessionpb.Event, who string) {
	t.Helper()
	require.NotEmpty(t, events, "%s must have received something", who)
	for i := 1; i < len(events); i++ {
		require.Equal(t, events[i-1].GetSeq()+1, events[i].GetSeq(),
			"%s: seq %d must immediately follow %d -- no gap, no duplicate", who, events[i].GetSeq(), events[i-1].GetSeq())
	}
}

// settleForwarders gives the async StreamEvents forwarding goroutines (one
// per live subscriber, broker channel -> stream.Send) a moment to actually
// drain and append before the test reads a snapshot. Publish itself is
// synchronous -- by the time an EndTurn call returns, every event is
// already sitting in each subscriber's own channel -- but delivery from
// there into a recordingStream's captured slice happens on that
// subscriber's own goroutine, which this test does not otherwise
// synchronize with.
func settleForwarders() { time.Sleep(100 * time.Millisecond) }

func containsKind(events []*sessionpb.Event, kind sessionpb.EventKind) bool {
	for _, e := range events {
		if e.GetKind() == kind {
			return true
		}
	}
	return false
}

// TestTwoStreamEventsSubscribers is rpg-project#257 slice 3 / rpg-api#819's
// own acceptance proof, entirely through the real, wired stack (real
// Manager, real Broker, real Handler.StreamEvents) rather than the toolkit's
// own fake-EventStream two_players_test.go: two joined characters, each with
// its own live StreamEvents call, prove they receive exactly their own
// addressed copies with contiguous seqs across a driven monster turn --
// then one of them stops being drained and the NEXT driven turn's Publish
// reports the lag (DeliveryReport.Failed, surfaced on the verb's own
// response -- convert.go's deliveryReportToProto, already wired) while the
// other subscriber, still live, keeps receiving everything without a gap.
func TestTwoStreamEventsSubscribers(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctxAlice := auth.WithPlayerID(context.Background(), "player-alice")
	ctxBob := auth.WithPlayerID(context.Background(), "player-bob")
	const session = "two-sub-run"

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)
	_, err = h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("bob", "player-bob")},
	})
	require.NoError(t, err)

	world := buildThreeRoomTomb(t)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: session, Encounter: "tomb-encounter", World: world,
	})
	require.NoError(t, err)

	// Both players join first, on the tomb's clear sight row (matching
	// buildThreeRoomTomb's own pillar-gap geometry), before anything hostile
	// exists to see -- exactly the toolkit's own two_players_test.go order
	// (join, join, THEN spawn), so the fight forms on Spawn with a turn
	// order this test can predict rather than one shaped by which player
	// happened to join within an already-live bubble.
	_, err = h.handler.Join(ctxAlice, &sessionpb.JoinRequest{
		Session: session, Member: "alice", Position: &sessionpb.Position{X: 15, Y: 3},
	})
	require.NoError(t, err)
	_, err = h.handler.Join(ctxBob, &sessionpb.JoinRequest{
		Session: session, Member: "bob", Position: &sessionpb.Position{X: 17, Y: 3},
	})
	require.NoError(t, err)

	spawnResp, err := h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: session, ID: "skel-1", Ref: refs.Monsters.Skeleton().String(),
		Position: spatial.Position{X: 19, Y: 3},
	})
	require.NoError(t, err)
	require.NotNil(t, spawnResp.Formed, "both players are in sight of skel-1's spawn point along the gap row")
	// testDice{}'s flat rolls tie every initiative roll, so the order falls
	// to the ID tie-break (alphabetical): "alice" < "bob" < "skel-1".
	require.Equal(t, []string{"alice", "bob", "skel-1"}, spawnResp.Formed.Order)

	// -- both subscribe for real, through the actual handler --
	aliceCtx, cancelAlice := context.WithCancel(ctxAlice)
	defer cancelAlice()
	aliceStream := newRecordingStream(aliceCtx)
	go func() {
		if streamErr := h.handler.StreamEvents(&sessionpb.StreamEventsRequest{Session: session, Member: "alice"}, aliceStream); streamErr != nil {
			t.Logf("alice's StreamEvents call returned an error: %v", streamErr)
		}
	}()
	waitForLive(t, h.manager.Broker, session, "alice", aliceStream)
	aliceBaseline := len(aliceStream.snapshot())

	bobCtx, cancelBob := context.WithCancel(ctxBob)
	bobDone := make(chan error, 1)
	bobStream := newRecordingStream(bobCtx)
	go func() {
		bobDone <- h.handler.StreamEvents(&sessionpb.StreamEventsRequest{Session: session, Member: "bob"}, bobStream)
	}()
	waitForLive(t, h.manager.Broker, session, "bob", bobStream)
	bobBaseline := len(bobStream.snapshot())

	// --- round 1: both real subscribers, a driven monster turn ---
	//
	// alice's own EndTurn passes (nothing to do yet, hands straight to bob --
	// a real player, no drive). bob's own EndTurn drives skel-1's WHOLE turn:
	// it is closer to bob (distance 2) than to alice (distance 4), so it
	// closes and strikes her, then wraps the round back to alice.
	out1, err := h.handler.EndTurn(ctxAlice, &sessionpb.EndTurnRequest{Session: session, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, "bob", out1.GetNext())

	out2, err := h.handler.EndTurn(ctxBob, &sessionpb.EndTurnRequest{Session: session, Member: "bob"})
	require.NoError(t, err)
	require.Equal(t, "alice", out2.GetNext())
	require.True(t, out2.GetRoundWrapped())
	require.False(t, out2.GetDelivery().GetFailed(), "nobody has lagged yet -- round 1 must deliver cleanly")
	settleForwarders()

	// Both real subscribers must have received their OWN addressed copies of
	// the whole round, with contiguous seqs -- rpg-toolkit#940 today
	// broadcasts every beat to the whole roster regardless of perception, so
	// the two sets are the same length and each contains the strike.
	round1Alice := aliceStream.snapshot()[aliceBaseline:]
	round1Bob := bobStream.snapshot()[bobBaseline:]
	assertContiguousSeqs(t, round1Alice, "alice")
	assertContiguousSeqs(t, round1Bob, "bob")
	require.Equal(t, len(round1Alice), len(round1Bob), "today's broadcast addresses every beat to the whole roster (rpg-toolkit#940)")
	require.True(t, containsKind(round1Alice, sessionpb.EventKind_EVENT_KIND_STRUCK), "alice must see skel-1's round-1 strike")
	require.True(t, containsKind(round1Bob, sessionpb.EventKind_EVENT_KIND_STRUCK), "bob must see skel-1's round-1 strike")

	// --- bob stops draining ---
	//
	// End bob's real StreamEvents call and wait for it to actually return
	// (its defer unsubscribes the old channel) before opening a fresh raw
	// Broker.Subscribe handle in her place -- one nobody ever reads from.
	// The Broker cannot tell this apart from a stalled client connection;
	// both are just an unread chan sdk.Event under the same (session, "bob")
	// key, which is exactly the point.
	cancelBob()
	select {
	case doneErr := <-bobDone:
		require.NoError(t, doneErr)
	case <-time.After(time.Second):
		t.Fatal("bob's StreamEvents call did not return after cancellation")
	}

	bobLag, err := h.manager.Broker.Subscribe(session, "bob")
	require.NoError(t, err)
	defer func() { _ = bobLag.Close() }()

	// Prime her buffer to exactly headroom -- filling it is not itself a
	// drop, so this must report nothing.
	fill := make([]sdk.Event, 0, 32)
	for i := 0; i < 32; i++ {
		fill = append(fill, sdk.Event{Session: session, Recipient: "bob", Seq: uint64(1000 + i), Kind: sdk.EventUnknown})
	}
	require.NoError(t, h.manager.Broker.Publish(context.Background(), fill))
	require.Equal(t, uint64(0), h.manager.Broker.Dropped(session, "bob"))

	// --- round 2: the next driven turn, with bob now lagging ---
	//
	// alice passes again (hands to bob); bob's own EndTurn drives skel-1 a
	// second time. Every beat this produces is still addressed to bob too
	// (the same whole-roster broadcast round 1 relied on) -- and her
	// primed-to-headroom subscriber now drops every one of them.
	_, err = h.handler.EndTurn(ctxAlice, &sessionpb.EndTurnRequest{Session: session, Member: "alice"})
	require.NoError(t, err, "a lagging subscriber must never fail the acting player's own verb")

	beforeRound2 := len(aliceStream.snapshot()[aliceBaseline:])
	out3, err := h.handler.EndTurn(ctxBob, &sessionpb.EndTurnRequest{Session: session, Member: "bob"})
	require.NoError(t, err, "a lagging subscriber must never fail the acting player's own verb")
	require.Equal(t, "alice", out3.GetNext())

	require.True(t, out3.GetDelivery().GetFailed(),
		"bob's own subscriber lagged during this driven turn -- DeliveryReport.Failed must surface it on the caller's own response")
	require.Greater(t, h.manager.Broker.Dropped(session, "bob"), uint64(0),
		"round 2's own beats must have dropped for bob now that her buffer is primed to headroom")
	settleForwarders()

	// The OTHER subscriber -- alice, still live and draining -- must still
	// get everything from this same driven turn, with no gap at all
	// carried over from round 1.
	allAlice := aliceStream.snapshot()[aliceBaseline:]
	require.Greater(t, len(allAlice), beforeRound2, "alice must have received round 2's own beats too")
	assertContiguousSeqs(t, allAlice, "alice")
	require.True(t, containsKind(allAlice[beforeRound2:], sessionpb.EventKind_EVENT_KIND_STRUCK),
		"alice must see skel-1's round-2 strike even though bob's own subscriber is the one lagging")
}
