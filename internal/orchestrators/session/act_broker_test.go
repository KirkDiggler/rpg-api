package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

// ActBrokerTestSuite pins the act stream's transport (rpg-project#630,
// event-stream R28 and the same-id rule on sdk.Act.ID): a subscriber is keyed
// by session AND recipient, a lagging one is closed as lagged and told its
// last id, and a same-id run is enqueued whole or not at all.
type ActBrokerTestSuite struct {
	suite.Suite
	ctx context.Context
}

func TestActBrokerSuite(t *testing.T) {
	suite.Run(t, new(ActBrokerTestSuite))
}

func (s *ActBrokerTestSuite) SetupTest() {
	s.ctx = context.Background()
}

// act is one act for recipient with the given record id.
func act(recipient, id string) sdk.Act {
	return sdk.Act{Session: "sess-1", ID: id, Recipient: recipient, Family: sdk.FamilyAct}
}

// acts is recipient's acts with the given ids, in order; repeat an id for a
// same-id run.
func acts(recipient string, ids ...string) []sdk.Act {
	out := make([]sdk.Act, len(ids))
	for i, id := range ids {
		out[i] = act(recipient, id)
	}
	return out
}

func (s *ActBrokerTestSuite) drain(sub *sessionorch.ActSubscription) []string {
	var got []string
	for {
		select {
		case a, ok := <-sub.Acts():
			if !ok {
				return got
			}
			got = append(got, a.ID)
		case <-time.After(time.Second):
			s.FailNow("the subscription never closed")
			return got
		}
	}
}

// TestActBrokerLagClosesBetweenIDs: with a buffer of 3, publishing the runs
// [A] and [B, B, B] gives the subscriber A, then a close marked lagged with
// LastID A, and NO B. Three B's do not fit beside A, and enqueueing two of
// them would split the run: the client would resume after B and lose the
// third with no signal.
func (s *ActBrokerTestSuite) TestActBrokerLagClosesBetweenIDs() {
	broker := sessionorch.NewActBroker(3)
	sub, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)

	err = broker.PublishActs(s.ctx, acts("alice", "A", "B", "B", "B"))
	s.Require().ErrorIs(err, sessionorch.ErrActsLagged)

	s.Equal([]string{"A"}, s.drain(sub), "no part of B's run is handed over")
	lagged, last := sub.Lagged()
	s.True(lagged)
	s.Equal("A", last, "the last id whose whole run was enqueued")
}

// TestActBrokerWholeRunFits: a same-id run that fits the free buffer is
// enqueued whole, and the subscriber stays live.
func (s *ActBrokerTestSuite) TestActBrokerWholeRunFits() {
	broker := sessionorch.NewActBroker(4)
	sub, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)

	s.Require().NoError(broker.PublishActs(s.ctx, acts("alice", "A", "B", "B", "B")))
	s.Require().NoError(sub.Close())
	s.Equal([]string{"A", "B", "B", "B"}, s.drain(sub))
	lagged, _ := sub.Lagged()
	s.False(lagged)
}

// TestActBrokerRoutesByRecipient: alice's acts never reach bob's
// subscription, and bob's never reach alice's, in the same session and the
// same publish.
func (s *ActBrokerTestSuite) TestActBrokerRoutesByRecipient() {
	broker := sessionorch.NewActBroker(8)
	alice, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	bob, err := broker.Subscribe("sess-1", "bob")
	s.Require().NoError(err)
	other, err := broker.Subscribe("sess-2", "alice")
	s.Require().NoError(err)

	batch := append(acts("alice", "A1", "A2"), acts("bob", "B1")...)
	s.Require().NoError(broker.PublishActs(s.ctx, batch))
	s.Require().NoError(alice.Close())
	s.Require().NoError(bob.Close())
	s.Require().NoError(other.Close())

	s.Equal([]string{"A1", "A2"}, s.drain(alice))
	s.Equal([]string{"B1"}, s.drain(bob))
	s.Empty(s.drain(other), "same member, another session")
}

// TestActBrokerLagLeavesOthersWhole: one recipient's lag does not cost a
// second subscriber of the same recipient, or another recipient, any act.
func (s *ActBrokerTestSuite) TestActBrokerLagLeavesOthersWhole() {
	broker := sessionorch.NewActBroker(2)
	slow, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	s.Require().NoError(broker.PublishActs(s.ctx, acts("alice", "A")))

	fast, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	err = broker.PublishActs(s.ctx, acts("alice", "B", "B"))
	s.Require().ErrorIs(err, sessionorch.ErrActsLagged)

	s.Require().NoError(fast.Close())
	s.Equal([]string{"B", "B"}, s.drain(fast))
	s.Equal([]string{"A"}, s.drain(slow))
	lagged, last := slow.Lagged()
	s.True(lagged)
	s.Equal("A", last)
}

// TestActBrokerClosedRefusesSubscribe: after Close, every subscription is
// ended unlagged and a new one is refused.
func (s *ActBrokerTestSuite) TestActBrokerClosedRefusesSubscribe() {
	broker := sessionorch.NewActBroker(2)
	sub, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	s.Require().NoError(broker.Close())
	s.Empty(s.drain(sub))
	lagged, _ := sub.Lagged()
	s.False(lagged)
	_, err = broker.Subscribe("sess-1", "alice")
	s.ErrorIs(err, sessionorch.ErrBrokerClosed)
}
