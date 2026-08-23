package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// TestGetStoryMatchesLiveEvents is rpg-api-protos#239's own acceptance
// criterion made executable, entirely through the real, wired stack (real
// Manager, real Broker, real Handler.StreamEvents and Handler.GetStory) --
// the same shape TestTwoStreamEventsSubscribers proves the stream with.
//
// Alice subscribes live BEFORE the skeleton's turn is driven, so her live
// stream captures the whole driven turn (moved..., a swing, turn-ended)
// exactly as StreamEvents would deliver it in production. Then GetStory{
// from_seq: 1} catches up from the very start of the session and, for every
// seq the live stream actually delivered, the caught-up Event must be
// proto.Equal to the live one: byte-equal for the same seq, not merely the
// same fields by inspection, since session/v0.23.0 (rpg-toolkit#1213) makes
// GetStory and StreamEvents share one projection (projectEntry) all the way
// from the toolkit through to eventToProto here.
func TestGetStoryMatchesLiveEvents(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	const session = "story-events-run"

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	world := buildThreeRoomTomb(t)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: session, Encounter: "tomb-encounter", World: world,
	})
	require.NoError(t, err)
	// Spawned before Join, on the tomb's pillar-gap sight row (the same
	// geometry TestSkeletonsDrivenTurnMovesAndStrikes uses), so the fight
	// forms the moment alice joins in sight and range of it -- a turn order
	// this test can predict rather than one shaped by join-then-spawn timing.
	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: session, ID: "skel-1", Ref: refs.Monsters.Skeleton().String(),
		Position: at(19, 3),
	})
	require.NoError(t, err)

	joinResp, err := h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: session, Member: "alice",
		Position: pbAt(16, 3), // inside the tomb, on the pillar gap row, three cells out
	})
	require.NoError(t, err)
	require.NotNil(t, joinResp.GetFormed(), "in sight and in range at first light: the fight forms on Join")
	require.Equal(t, []string{"alice", "skel-1"}, joinResp.GetFormed().GetOrder(),
		"the tied roll breaks to alice -- it must be her turn first, not the skeleton's")

	// Subscribe live, for real, through the actual handler and broker --
	// AFTER Join (design rule 6: StreamEvents carries no replay obligation,
	// so the join beat itself is deliberately not part of what live captures).
	aliceCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newRecordingStream(aliceCtx)
	go func() {
		if streamErr := h.handler.StreamEvents(&sessionpb.StreamEventsRequest{Session: session, Member: "alice"}, stream); streamErr != nil {
			t.Logf("alice's StreamEvents call returned an error: %v", streamErr)
		}
	}()
	waitForLive(t, h.manager.Broker, session, "alice", stream)
	baseline := len(stream.snapshot())

	// Alice has nothing to do this turn -- three cells out with no movement
	// spent -- so ending immediately is the honest move, and the skeleton's
	// whole turn (approach, swing, its own turn-ended) drives inside this
	// one call.
	endResp, err := h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: session, Member: "alice"})
	require.NoError(t, err, "the skeleton's whole turn drives inside this one call")
	require.Equal(t, "alice", endResp.GetNext(), "a two-member fight wraps straight back to whoever led it")

	waitForQuiescence(t, stream, 2*time.Second)
	live := stream.snapshot()[baseline:]
	require.NotEmpty(t, live, "the driven turn must have produced at least one live event")
	require.True(t, containsKind(live, sessionpb.EventKind_EVENT_KIND_TURN_ENDED),
		"alice's own end-turn beat and the skeleton's own must both be turn-ended events")
	require.True(t, containsKind(live, sessionpb.EventKind_EVENT_KIND_MOVED) ||
		containsKind(live, sessionpb.EventKind_EVENT_KIND_STRUCK) ||
		containsKind(live, sessionpb.EventKind_EVENT_KIND_MISSED),
		"the skeleton had to close the gap and then swing")

	// Catch up from the very start of the session and prove every seq the
	// live stream delivered comes back byte-equal.
	storyResp, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: session, Member: "alice", FromSeq: 1})
	require.NoError(t, err)

	bySeq := make(map[uint64]*sessionpb.Event, len(storyResp.GetEntries()))
	for _, e := range storyResp.GetEntries() {
		bySeq[e.GetSeq()] = e
	}
	for _, liveEvt := range live {
		storyEvt, ok := bySeq[liveEvt.GetSeq()]
		require.True(t, ok, "GetStory must carry every seq the live stream delivered; missing seq %d", liveEvt.GetSeq())
		require.True(t, proto.Equal(liveEvt, storyEvt),
			"seq %d: live and catch-up must be byte-equal (rpg-api-protos#239)\n  live:  %v\n  story: %v",
			liveEvt.GetSeq(), liveEvt, storyEvt)
	}

	// FromSeq:1 catches up from before alice's own Join beat -- the join
	// beat's audience is "all current members including the joiner", so
	// alice's own arrival (unlike skel-1's earlier spawn, which predates her
	// membership and so never reaches her story) is hers to see. This is
	// the shared-converter path proving the typed Joined body
	// (session/v0.24.0, rpg-toolkit#1217) reaches GetStory, not only
	// StreamEvents (which -- design rule 6 -- never carries the join beat
	// live, so `live` above cannot exercise this on its own).
	var sawAliceJoined bool
	for _, e := range storyResp.GetEntries() {
		if e.GetKind() == sessionpb.EventKind_EVENT_KIND_JOINED && e.GetJoined().GetMember() == "alice" {
			sawAliceJoined = true
		}
	}
	require.True(t, sawAliceJoined, "alice's own join beat must carry a typed Joined body")
}

// TestGetStoryAgedOutReturnsOutOfRange proves the OTHER half of GetStory's
// documented contract (read.go's own doc on Manager.Story): a resume point
// that has aged out of the retention window must come back as the
// documented error, not a short or empty answer a client could mistake for
// "you are already caught up."
//
// buildThreeRoomTomb's own world carries RetentionUnbounded (every other
// acceptance test in this package wants the whole story kept), so this test
// builds its own copy with a small window instead of touching the shared
// fixture -- world.Retention is exported (encounter.EncounterData) precisely
// so a caller can do this without a second builder.
func TestGetStoryAgedOutReturnsOutOfRange(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	const session = "story-trimmed-run"

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	world := buildThreeRoomTomb(t)
	world.Retention = 2 // small on purpose: this test wants trimming, not the whole story
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: session, Encounter: "tomb-encounter", World: world,
	})
	require.NoError(t, err)

	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: session, Member: "alice", Position: pbAt(1, 2),
	})
	require.NoError(t, err)

	// Outside a fight, world-clock movement is unpriced (session's own
	// priceWalk returns a free cost for anything but the turn clock), so
	// alice can walk back and forth freely -- each Move call appends at
	// least one "moved" beat, well past the two-entry retention window in
	// only a few calls.
	waypoints := []*sessionpb.Position{pbAt(2, 2), pbAt(1, 2)}
	for i := 0; i < 6; i++ {
		_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
			Session: session, Member: "alice",
			Path: []*sessionpb.Position{waypoints[i%2]},
		})
		require.NoError(t, err)
	}

	_, err = h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: session, Member: "alice", FromSeq: 1})
	requireGRPCCode(t, err, codes.OutOfRange)
}
