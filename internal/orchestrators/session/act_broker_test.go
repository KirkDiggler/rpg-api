package session_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

// ActBrokerTestSuite pins the act stream's transport (rpg-project#630,
// event-stream R28 and the same-id rule on sdk.Act.ID): a subscriber is keyed
// by session AND recipient, one commit's acts for it are one unit and the
// buffer counts units (rpg-project#894), a lagging one is closed as lagged and
// told its last id, and a same-id run is never split.
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

// brokered is every act the subscription is handed until it closes, units
// flattened in order.
func (s *ActBrokerTestSuite) brokered(sub *sessionorch.ActSubscription) []sessionorch.BrokeredAct {
	var got []sessionorch.BrokeredAct
	for {
		select {
		case unit, ok := <-sub.Units():
			if !ok {
				return got
			}
			got = append(got, unit...)
		case <-time.After(time.Second):
			s.FailNow("the subscription never closed")
			return got
		}
	}
}

func (s *ActBrokerTestSuite) drain(sub *sessionorch.ActSubscription) []string {
	brokered := s.brokered(sub)
	got := make([]string, 0, len(brokered))
	for _, a := range brokered {
		got = append(got, a.Act.ID)
	}
	return got
}

// garrisonVerb is one commit of n acts for recipient, the shape of a patrol
// verb on the garrison keep: mostly lone steps, with a same-id run of three
// every tenth id.
func garrisonVerb(recipient, prefix string, n int) []sdk.Act {
	out := make([]sdk.Act, 0, n)
	for i := 0; len(out) < n; i++ {
		id := fmt.Sprintf("%s-%04d", prefix, i)
		out = append(out, act(recipient, id))
		if i%10 == 0 {
			for j := 0; j < 2 && len(out) < n; j++ {
				out = append(out, act(recipient, id))
			}
		}
	}
	return out
}

// TestActBrokerGarrisonVerbFitsTwoCommits: a 320-act commit is one unit, so a
// buffer of 2 holds it with room to spare. Nothing lags, every act arrives in
// order, in one receive.
func (s *ActBrokerTestSuite) TestActBrokerGarrisonVerbFitsTwoCommits() {
	broker := sessionorch.NewActBroker(2)
	sub, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)

	verb := garrisonVerb("alice", "v1", 320)
	s.Require().NoError(broker.PublishActs(s.ctx, verb))

	want := make([]string, 0, len(verb))
	for _, a := range verb {
		want = append(want, a.ID)
	}
	unit := <-sub.Units()
	got := make([]string, 0, len(unit))
	for _, a := range unit {
		got = append(got, a.Act.ID)
	}
	s.Equal(want, got, "the whole commit is one unit, in order")
	s.Require().NoError(sub.Close())
	lagged, _ := sub.Lagged()
	s.False(lagged)
}

// TestActBrokerLagClosesBetweenCommits: with a buffer of 2 and nobody
// reading, the third commit does not fit. The subscriber is handed the first
// two commits whole, then a close marked lagged with LastID the last id of
// the second, and NO act of the third: the cut is between two ids, and the
// same-id runs on both sides of it arrive whole or not at all.
func (s *ActBrokerTestSuite) TestActBrokerLagClosesBetweenCommits() {
	broker := sessionorch.NewActBroker(2)
	sub, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)

	s.Require().NoError(broker.PublishActs(s.ctx, acts("alice", "A", "B", "B", "B")), "commit 1 fits")
	s.Require().NoError(broker.PublishActs(s.ctx, acts("alice", "C", "C", "D")), "commit 2 fills the buffer: not a lag")
	lagged, _ := sub.Lagged()
	s.Require().False(lagged, "a full buffer is not a lag")

	err = broker.PublishActs(s.ctx, acts("alice", "E", "E", "E", "F"))
	s.Require().ErrorIs(err, sessionorch.ErrActsLagged)
	var lag *sessionorch.ActsLaggedError
	s.Require().ErrorAs(err, &lag)
	s.Equal("D", lag.LastID)

	got := s.brokered(sub)
	ids := make([]string, len(got))
	for i, a := range got {
		ids[i] = a.Act.ID
	}
	s.Equal([]string{"A", "B", "B", "B", "C", "C", "D"}, ids, "no part of the third commit is handed over")
	s.True(got[len(got)-1].RunEnd, "the last act handed over ends its run: the close is between ids")
	lagged, last := sub.Lagged()
	s.True(lagged)
	s.Equal("D", last, "the last id of the last unit enqueued")

	// A lagged subscriber is gone: later commits are not queued for it.
	s.Require().NoError(broker.PublishActs(s.ctx, acts("alice", "G")))
}

// TestActBrokerMarksRunEnd: the broker knows the unit, so it marks it: the
// last act of every same-id run, and every run of one, carries RunEnd.
func (s *ActBrokerTestSuite) TestActBrokerMarksRunEnd() {
	broker := sessionorch.NewActBroker(8)
	sub, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)

	s.Require().NoError(broker.PublishActs(s.ctx, acts("alice", "A", "B", "B", "B", "C")))
	s.Require().NoError(sub.Close())
	brokered := s.brokered(sub)
	ends := make([]bool, 0, len(brokered))
	for _, a := range brokered {
		ends = append(ends, a.RunEnd)
	}
	s.Equal([]bool{true, false, false, true, true}, ends)
}

// TestActBrokerRoutesByRecipient: alice's acts never reach bob's
// subscription, and bob's never reach alice's, in the same session and the
// same publish. A commit that interleaves the two still gives each recipient
// one unit, in its own order.
func (s *ActBrokerTestSuite) TestActBrokerRoutesByRecipient() {
	broker := sessionorch.NewActBroker(8)
	alice, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	bob, err := broker.Subscribe("sess-1", "bob")
	s.Require().NoError(err)
	other, err := broker.Subscribe("sess-2", "alice")
	s.Require().NoError(err)

	batch := []sdk.Act{act("alice", "A1"), act("bob", "B1"), act("alice", "A2")}
	s.Require().NoError(broker.PublishActs(s.ctx, batch))
	first := <-alice.Units()
	firstIDs := make([]string, 0, len(first))
	for _, a := range first {
		firstIDs = append(firstIDs, a.Act.ID)
	}
	s.Equal([]string{"A1", "A2"}, firstIDs, "alice's acts of one commit are one unit")
	s.Require().NoError(broker.PublishActs(s.ctx, batch))
	s.Require().NoError(alice.Close())
	s.Require().NoError(bob.Close())
	s.Require().NoError(other.Close())

	s.Equal([]string{"A1", "A2"}, s.drain(alice))
	s.Equal([]string{"B1", "B1"}, s.drain(bob))
	s.Empty(s.drain(other), "same member, another session")
}

// TestActBrokerLagLeavesOthersWhole: one recipient's lag does not cost a
// second subscriber of the same recipient, or another recipient, any act.
func (s *ActBrokerTestSuite) TestActBrokerLagLeavesOthersWhole() {
	broker := sessionorch.NewActBroker(1)
	slow, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	s.Require().NoError(broker.PublishActs(s.ctx, acts("alice", "A")))

	fast, err := broker.Subscribe("sess-1", "alice")
	s.Require().NoError(err)
	bob, err := broker.Subscribe("sess-1", "bob")
	s.Require().NoError(err)
	batch := append(acts("alice", "B", "B"), act("bob", "B"))
	err = broker.PublishActs(s.ctx, batch)
	s.Require().ErrorIs(err, sessionorch.ErrActsLagged)

	s.Require().NoError(fast.Close())
	s.Require().NoError(bob.Close())
	s.Equal([]string{"B", "B"}, s.drain(fast))
	s.Equal([]string{"B"}, s.drain(bob))
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
