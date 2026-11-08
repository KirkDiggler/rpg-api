package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

// ActListenersTestSuite pins the broker's answer to who is listening
// (rpg-project#879 S7): the recipients of one session with a live
// subscription, and nobody else.
type ActListenersTestSuite struct {
	suite.Suite
	ctx    context.Context
	broker *sessionorch.ActBroker
}

func TestActListenersSuite(t *testing.T) {
	suite.Run(t, new(ActListenersTestSuite))
}

func (s *ActListenersTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = sessionorch.NewActBroker(8)
}

func (s *ActListenersTestSuite) listening(session string) []string {
	got, err := s.broker.Listening(s.ctx, session)
	s.Require().NoError(err)
	return got
}

func (s *ActListenersTestSuite) TestNobodySubscribedIsAnEmptyList() {
	got := s.listening("sess-1")
	s.NotNil(got)
	s.Empty(got)
}

func (s *ActListenersTestSuite) TestNamesASubscribedRecipientAndNotAnother() {
	_, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)

	s.Equal([]string{"alice"}, s.listening("sess-1"), "bob never subscribed")
}

func (s *ActListenersTestSuite) TestTwoSubscriptionsOfOneRecipientNameItOnce() {
	first, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	_, err = s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	_, err = s.broker.Subscribe("sess-1", "bob")
	s.Require().NoError(err)

	s.Equal([]string{"alice", "bob"}, s.listening("sess-1"))

	s.Require().NoError(first.Close())
	s.Equal([]string{"alice", "bob"}, s.listening("sess-1"), "alice's second tab still listens")
}

func (s *ActListenersTestSuite) TestForgetsARecipientWhenItsSubscriptionCloses() {
	sub, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	s.Require().Equal([]string{"alice"}, s.listening("sess-1"))

	s.Require().NoError(sub.Close())
	s.Empty(s.listening("sess-1"))
}

func (s *ActListenersTestSuite) TestForgetsARecipientClosedAsLagged() {
	broker := sessionorch.NewActBroker(1)
	sub, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)

	// The buffer counts commits: two undrained commits into a buffer of one,
	// and the second closes her as lagged.
	s.Require().NoError(broker.PublishActs(s.ctx, []sdk.Act{
		{Session: "sess-1", ID: "a", Recipient: "alice", Family: sdk.FamilyAct},
	}), "the first commit fills the buffer")
	err = broker.PublishActs(s.ctx, []sdk.Act{
		{Session: "sess-1", ID: "b", Recipient: "alice", Family: sdk.FamilyAct},
	})
	s.Require().ErrorIs(err, sessionorch.ErrActsLagged)
	lagged, _ := sub.Lagged()
	s.Require().True(lagged)

	got, err := broker.Listening(s.ctx, "sess-1")
	s.Require().NoError(err)
	s.Empty(got, "a lagged subscriber is no longer listening")
}

func (s *ActListenersTestSuite) TestForgetsEveryoneWhenTheBrokerCloses() {
	_, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)

	s.Require().NoError(s.broker.Close())
	s.Empty(s.listening("sess-1"))
}

func (s *ActListenersTestSuite) TestIsScopedPerSession() {
	_, err := s.broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	_, err = s.broker.Subscribe("sess-2", "bob")
	s.Require().NoError(err)

	s.Equal([]string{"alice"}, s.listening("sess-1"))
	s.Equal([]string{"bob"}, s.listening("sess-2"))
	s.Empty(s.listening("sess-3"))
}
