package session_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// actKeys is each act as "<id> <family> <body>", in order: enough to say
// two reads of the story are the same acts in the same order.
func actKeys(acts []*storypb.Act) []string {
	out := make([]string, len(acts))
	for i, a := range acts {
		out[i] = fmt.Sprintf("%s %s %T", a.GetId(), a.GetFamily(), a.GetBody())
	}
	return out
}

// struckBy reports whether acts hold an attack by attacker.
func struckBy(acts []*storypb.Act, attacker string) bool {
	for _, a := range acts {
		if a.GetAttack().GetBy().GetId() == attacker {
			return true
		}
	}
	return false
}

// TestTwoStoryStreamSubscribers is rpg-project#257 slice 3 / rpg-api#819's
// acceptance proof, carried onto the act stream when the v1 event stream was
// deleted (rpg-project#631): two joined characters, each with its own live
// StoryService StreamEvents call through the real handler and the real
// ActBroker, receive exactly the acts their own GetStory reads, across a
// driven monster turn. Then one of them stops being drained, the NEXT driven
// turn closes that subscriber as lagged without failing the acting player's
// verb, and the other subscriber keeps receiving everything.
func TestTwoStoryStreamSubscribers(t *testing.T) {
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

	// Both players seat on the tomb's clear sight row (matching
	// buildThreeRoomTomb's own pillar-gap geometry) and the fight forms at
	// launch, one formation holding all three.
	launched := h.launch(t, session,
		withMonsters(buildThreeRoomTomb(t), monsterAt("skel-1", refs.Monsters.Skeleton().String(), 20, 3)),
		seatAt("alice", 16, 3), seatAt("bob", 18, 3))
	require.Len(t, launched.Formed, 1, "both players are in sight of skel-1 along the gap row")
	// testDice{}'s flat rolls tie every initiative roll, so the order falls
	// to the ID tie-break (alphabetical): "alice" < "bob" < "skel-1".
	require.Equal(t, []string{"alice", "bob", "skel-1"}, launched.Formed[0].Order)

	// -- both subscribe for real, through the actual StoryService handler.
	// The handler subscribes BEFORE it replays, so once the replay of the
	// launch has arrived the subscription is live.
	aliceCtx, cancelAlice := context.WithCancel(ctxAlice)
	defer cancelAlice()
	aliceStream := newActStream(aliceCtx)
	go func() {
		if streamErr := h.story.StreamEvents(&storypb.StreamEventsRequest{Session: session, Member: "alice"}, aliceStream); streamErr != nil {
			t.Logf("alice's StreamEvents call returned an error: %v", streamErr)
		}
	}()
	require.NotEmpty(t, aliceStream.settle(t, 2*time.Second), "alice's replay of the launch")

	bobCtx, cancelBob := context.WithCancel(ctxBob)
	bobDone := make(chan error, 1)
	bobStream := newActStream(bobCtx)
	go func() {
		bobDone <- h.story.StreamEvents(&storypb.StreamEventsRequest{Session: session, Member: "bob"}, bobStream)
	}()
	require.NotEmpty(t, bobStream.settle(t, 2*time.Second), "bob's replay of the launch")
	bobBaseline := len(bobStream.snapshot())

	// --- round 1: both real subscribers, a driven monster turn ---
	//
	// alice's EndTurn hands straight to bob; bob's EndTurn drives skel-1's
	// WHOLE turn, which closes and strikes, then wraps the round to alice.
	out1, err := h.handler.EndTurn(ctxAlice, &sessionpb.EndTurnRequest{
		Session: session, Member: "alice",
		DeclarationId: currentDeclarationID(ctxAlice, t, h.handler, session, "alice", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err)
	require.Equal(t, "bob", out1.GetNext())

	out2, err := h.handler.EndTurn(ctxBob, &sessionpb.EndTurnRequest{
		Session: session, Member: "bob",
		DeclarationId: currentDeclarationID(ctxBob, t, h.handler, session, "bob", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err)
	require.Equal(t, "alice", out2.GetNext())
	require.True(t, out2.GetRoundWrapped())

	// Each live stream holds exactly the acts its own GetStory reads: no gap,
	// no duplicate, in order.
	aliceRound1 := aliceStream.settle(t, 2*time.Second)
	bobRound1 := bobStream.settle(t, 2*time.Second)
	require.Equal(t, actKeys(storyOf(ctxAlice, t, h.story, session, "alice")), actKeys(aliceRound1))
	require.Equal(t, actKeys(storyOf(ctxBob, t, h.story, session, "bob")), actKeys(bobRound1))
	require.True(t, struckBy(bobRound1[bobBaseline:], "skel-1"), "bob must see skel-1's round-1 attack")
	require.True(t, struckBy(aliceRound1, "skel-1"), "alice must see skel-1's round-1 attack")
	require.Nil(t, aliceStream.laggedAt(), "nobody has lagged yet")
	require.Nil(t, bobStream.laggedAt(), "nobody has lagged yet")

	// --- bob stops draining ---
	//
	// End bob's real StreamEvents call, then hold a raw ActBroker
	// subscription in its place that nobody reads. The broker cannot tell
	// it from a stalled client: both are an unread channel under the same
	// (session, "bob") key.
	cancelBob()
	select {
	case doneErr := <-bobDone:
		require.NoError(t, doneErr)
	case <-time.After(time.Second):
		t.Fatal("bob's StreamEvents call did not return after cancellation")
	}
	bobLag, err := h.manager.ActBroker.Subscribe(session, "bob")
	require.NoError(t, err)
	defer func() { _ = bobLag.Close() }()

	// Prime his buffer to exactly its capacity: filling it is not itself a
	// lag, so this must close nothing.
	capacity := cap(bobLag.Acts())
	fill := make([]sdk.Act, 0, capacity)
	for i := 0; i < capacity; i++ {
		fill = append(fill, sdk.Act{Session: session, Recipient: "bob", ID: fmt.Sprintf("fill-%04d", i)})
	}
	require.NoError(t, h.manager.ActBroker.PublishActs(context.Background(), fill))
	lagged, _ := bobLag.Lagged()
	require.False(t, lagged, "a full buffer is not a lag")

	// --- round 2: the next driven turn, with bob now lagging ---
	beforeRound2 := len(aliceStream.snapshot())
	_, err = h.handler.EndTurn(ctxAlice, &sessionpb.EndTurnRequest{
		Session: session, Member: "alice",
		DeclarationId: currentDeclarationID(ctxAlice, t, h.handler, session, "alice", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err, "a lagging subscriber must never fail the acting player's own verb")
	out3, err := h.handler.EndTurn(ctxBob, &sessionpb.EndTurnRequest{
		Session: session, Member: "bob",
		DeclarationId: currentDeclarationID(ctxBob, t, h.handler, session, "bob", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err, "a lagging subscriber must never fail the acting player's own verb")
	require.Equal(t, "alice", out3.GetNext())

	lagged, _ = bobLag.Lagged()
	require.True(t, lagged, "round 2's acts could not fit bob's full buffer: the broker closes him as lagged")

	// The OTHER subscriber -- alice, still live and draining -- gets every
	// act of this same turn, with no gap carried over from round 1.
	allAlice := aliceStream.settle(t, 2*time.Second)
	require.Greater(t, len(allAlice), beforeRound2, "alice must have received round 2's acts too")
	require.Equal(t, actKeys(storyOf(ctxAlice, t, h.story, session, "alice")), actKeys(allAlice))
	require.True(t, struckBy(allAlice[beforeRound2:], "skel-1"),
		"alice must see skel-1's round-2 attack even though bob's subscriber is the one lagging")
	require.Nil(t, aliceStream.laggedAt())
}
