package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

type BrokerTestSuite struct {
	suite.Suite
	ctx    context.Context
	broker *sessionorch.Broker
}

func (s *BrokerTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = sessionorch.NewBroker()
}

func TestBrokerSuite(t *testing.T) {
	suite.Run(t, new(BrokerTestSuite))
}

func (s *BrokerTestSuite) TestPublish_DeliversOnlyToMatchingSessionAndRecipient() {
	alice, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	defer func() { _ = alice.Close() }()

	bob, err := s.broker.Subscribe("sess-1", "bob")
	s.Require().NoError(err)
	defer func() { _ = bob.Close() }()

	otherSession, err := s.broker.Subscribe("sess-2", "alice")
	s.Require().NoError(err)
	defer func() { _ = otherSession.Close() }()

	err = s.broker.Publish(s.ctx, []sdk.Event{
		{Session: "sess-1", Recipient: "alice", Kind: sdk.EventMoved, Seq: 1},
	})
	s.Require().NoError(err)

	select {
	case evt := <-alice.Events():
		s.Equal(uint64(1), evt.Seq)
	default:
		s.Fail("alice should have received the event addressed to her")
	}

	select {
	case <-bob.Events():
		s.Fail("bob must not receive an event addressed to alice")
	default:
	}

	select {
	case <-otherSession.Events():
		s.Fail("a subscriber on a different session must not receive the event")
	default:
	}
}

// TestPublish_DropsPastHeadroomAndReportsFailure is rpg-api#819's own RED
// test: the Broker's 32-slot per-(session,recipient) channel used to drop a
// subscriber's overflow with `select { default: }` and Publish returned nil
// regardless -- silently, with the SDK's own DeliveryReport.Failed unable to
// ever become true (issue rpg-api#819, "Defect 1"). RULED (Kirk,
// 2026-08-23): drop, never silently -- the acting player's verb must never
// block on a lagging viewer, but the drop itself must be visible.
//
// bob shares the same Publish call as alice's overflow on purpose: the
// ruling is not just "report the drop," it is "one lagging viewer never
// starves the rest of the batch" -- bob must receive his own event in the
// very call that drops alice's.
func (s *BrokerTestSuite) TestPublish_DropsPastHeadroomAndReportsFailure() {
	alice, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	defer func() { _ = alice.Close() }()

	bob, err := s.broker.Subscribe("sess-1", "bob")
	s.Require().NoError(err)
	defer func() { _ = bob.Close() }()

	// Fill the 32-slot channel to exactly headroom without draining it --
	// this alone must never be reported as a drop.
	fill := make([]sdk.Event, 0, 32)
	for i := 0; i < 32; i++ {
		fill = append(fill, sdk.Event{Session: "sess-1", Recipient: "alice", Kind: sdk.EventMoved, Seq: uint64(i)})
	}
	s.Require().NoError(s.broker.Publish(s.ctx, fill), "filling exactly to headroom is not itself a drop")
	s.Equal(uint64(0), s.broker.Dropped("sess-1", "alice"))

	// One more batch: bob's own event (a different recipient, must be
	// wholly unaffected) and, second, a marked struck for alice landing
	// past her already-full headroom.
	err = s.broker.Publish(s.ctx, []sdk.Event{
		{Session: "sess-1", Recipient: "bob", Kind: sdk.EventMoved, Seq: 500},
		{Session: "sess-1", Recipient: "alice", Kind: sdk.EventStruck, Seq: 999},
	})

	s.Require().Error(err, "a drop past headroom must be reported, never silent")
	s.ErrorIs(err, sessionorch.ErrLagged)
	var lagged *sessionorch.ErrSubscriberLagged
	s.Require().ErrorAs(err, &lagged)
	s.Equal("sess-1", lagged.Session)
	s.Equal("alice", lagged.Recipient)
	s.Equal(uint64(999), lagged.Seq)
	s.Equal(sdk.EventStruck, lagged.Kind)
	s.Equal(uint64(1), lagged.Total)
	s.Equal(uint64(1), s.broker.Dropped("sess-1", "alice"), "the drop must be counted per (session, recipient)")

	// bob is the OTHER subscriber in the very batch that dropped alice's
	// event -- he must still have received his.
	select {
	case evt := <-bob.Events():
		s.Equal(uint64(500), evt.Seq)
	default:
		s.Fail("bob must still receive his own event even though alice's dropped in the same batch")
	}

	// Drain alice's whole buffer: it must be exactly the 32 fill events, in
	// order, and the struck event that arrived past headroom must never
	// appear anywhere in it.
	for i := 0; i < 32; i++ {
		select {
		case evt := <-alice.Events():
			s.Equal(uint64(i), evt.Seq, "alice's buffer must hold exactly the fill events, unperturbed by the drop")
		default:
			s.Fail("alice's buffer must be full of the 32 fill events")
		}
	}
	select {
	case evt := <-alice.Events():
		s.Fail("the struck event must never reach alice", "got seq %d kind %s", evt.Seq, evt.Kind)
	default:
	}
}

func (s *BrokerTestSuite) TestPublish_NeverBlocksOnASlowSubscriber() {
	sub, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	// Fill well past the subscriber's buffer: Publish must return promptly
	// (best-effort delivery, EventStream contract) rather than block on a
	// full channel -- and, since rpg-api#819, must report the overflow
	// rather than silently succeeding.
	events := make([]sdk.Event, 0, 64)
	for i := 0; i < 64; i++ {
		events = append(events, sdk.Event{Session: "sess-1", Recipient: "alice", Seq: uint64(i)})
	}

	done := make(chan error, 1)
	go func() { done <- s.broker.Publish(s.ctx, events) }()

	select {
	case err := <-done:
		s.Require().Error(err, "publish must report the 32 events it could not deliver, not silently succeed")
		var lagged *sessionorch.ErrSubscriberLagged
		s.Require().ErrorAs(err, &lagged)
		s.Equal(uint64(32), s.broker.Dropped("sess-1", "alice"))
	case <-time.After(time.Second):
		s.Fail("Publish must return promptly (best-effort delivery) rather than block on a full subscriber buffer")
	}
}

// TestDropped_ClearsWhenTheLastSubscriberForAKeyGoes pins Copilot's own
// finding on PR #821: dropped was only ever incremented, so a long-lived
// broker retained one entry per (session, recipient) that ever lagged for
// the life of the process, even long after that session and connection
// were gone -- an unbounded leak. The fix ties the count's lifetime to the
// key's own live subscribers: once the last one for a key unsubscribes, its
// accumulated count goes with it, the same as the channel map entry
// already does.
func (s *BrokerTestSuite) TestDropped_ClearsWhenTheLastSubscriberForAKeyGoes() {
	sub, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)

	fill := make([]sdk.Event, 0, 40)
	for i := 0; i < 40; i++ {
		fill = append(fill, sdk.Event{Session: "sess-1", Recipient: "alice", Seq: uint64(i)})
	}
	s.Require().Error(s.broker.Publish(s.ctx, fill), "40 into a 32-slot buffer must drop 8")
	s.Equal(uint64(8), s.broker.Dropped("sess-1", "alice"))

	s.Require().NoError(sub.Close())
	s.Equal(uint64(0), s.broker.Dropped("sess-1", "alice"),
		"the count must not outlive every subscriber that ever used this key")

	// A fresh reconnect under the very same key starts clean too -- the
	// leaked count would otherwise resurface here.
	fresh, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	defer func() { _ = fresh.Close() }()
	s.Equal(uint64(0), s.broker.Dropped("sess-1", "alice"))
}

func (s *BrokerTestSuite) TestClose_ClosesLiveSubscriptions() {
	sub, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)

	s.Require().NoError(s.broker.Close())

	_, ok := <-sub.Events()
	s.False(ok, "subscription channel must be closed when the broker closes")
}

func (s *BrokerTestSuite) TestSubscribe_AfterClose_Errors() {
	s.Require().NoError(s.broker.Close())
	_, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().ErrorIs(err, sessionorch.ErrBrokerClosed)
}
