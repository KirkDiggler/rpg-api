package session_test

import (
	"context"
	"testing"

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

func (s *BrokerTestSuite) TestPublish_NeverBlocksOnASlowSubscriber() {
	sub, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	// Fill the subscriber's buffer, then publish one more: Publish must
	// return promptly (best-effort delivery, EventStream contract) rather
	// than block on a full channel.
	events := make([]sdk.Event, 0, 64)
	for i := 0; i < 64; i++ {
		events = append(events, sdk.Event{Session: "sess-1", Recipient: "alice", Seq: uint64(i)})
	}
	err = s.broker.Publish(s.ctx, events)
	s.Require().NoError(err, "publish is best-effort and must never fail or block on a full subscriber buffer")
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
