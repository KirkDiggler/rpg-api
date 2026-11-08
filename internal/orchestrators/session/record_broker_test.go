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

// RecordBrokerTestSuite pins the record stream's transport (rpg-project#628,
// event-stream R28, rpg-api#1122 scoped to the new stream): a subscriber that
// falls behind is CLOSED AS LAGGED and told its last id, never skipped.
type RecordBrokerTestSuite struct {
	suite.Suite
	ctx context.Context
}

func TestRecordBrokerSuite(t *testing.T) {
	suite.Run(t, new(RecordBrokerTestSuite))
}

func (s *RecordBrokerTestSuite) SetupTest() {
	s.ctx = context.Background()
}

// beats is n record events of one session with ids id-01..id-n.
func beats(session string, from, n int) []sdk.RecordEvent {
	out := make([]sdk.RecordEvent, n)
	for i := range out {
		out[i] = sdk.RecordEvent{Session: session, ID: fmt.Sprintf("id-%02d", from+i), Beat: "moved"}
	}
	return out
}

func (s *RecordBrokerTestSuite) drain(sub *sessionorch.RecordSubscription) []string {
	var got []string
	for {
		select {
		case unit, ok := <-sub.Units():
			if !ok {
				return got
			}
			for _, evt := range unit {
				got = append(got, evt.ID)
			}
		case <-time.After(time.Second):
			s.FailNow("the subscription never closed")
			return got
		}
	}
}

// TestRecordBrokerLongCommitIsOneUnit: a commit of 320 beats, the record's
// growth for one garrison verb, is one unit, so a buffer of 2 holds it and
// nothing lags.
func (s *RecordBrokerTestSuite) TestRecordBrokerLongCommitIsOneUnit() {
	broker := sessionorch.NewRecordBroker(2)
	sub, err := broker.Subscribe("sess-1")
	s.Require().NoError(err)

	commit := beats("sess-1", 1, 320)
	s.Require().NoError(broker.PublishRecord(s.ctx, commit))
	want := make([]string, 0, len(commit))
	for _, evt := range commit {
		want = append(want, evt.ID)
	}
	unit := <-sub.Units()
	got := make([]string, 0, len(unit))
	for _, evt := range unit {
		got = append(got, evt.ID)
	}
	s.Equal(want, got, "the whole commit is one unit, in order")
	s.Require().NoError(sub.Close())
	lagged, _ := sub.Lagged()
	s.False(lagged)
}

// TestRecordBrokerLaggedSubscriberIsTold: with a buffer of 4, five commits
// published unread give the subscriber the first four, then a closed channel
// marked lagged with the fourth commit's last id. Nothing is skipped
// silently: the fifth is never handed over as if the stream were whole.
func (s *RecordBrokerTestSuite) TestRecordBrokerLaggedSubscriberIsTold() {
	broker := sessionorch.NewRecordBroker(4)
	sub, err := broker.Subscribe("sess-1")
	s.Require().NoError(err)

	for i := 0; i < 4; i++ {
		s.Require().NoError(broker.PublishRecord(s.ctx, beats("sess-1", 2*i+1, 2)))
	}
	err = broker.PublishRecord(s.ctx, beats("sess-1", 9, 2))
	s.Require().ErrorIs(err, sessionorch.ErrRecordLagged)

	s.Equal([]string{"id-01", "id-02", "id-03", "id-04", "id-05", "id-06", "id-07", "id-08"}, s.drain(sub))
	lagged, last := sub.Lagged()
	s.True(lagged)
	s.Equal("id-08", last)

	// A lagged subscriber is gone: later beats are not queued for it.
	s.Require().NoError(broker.PublishRecord(s.ctx, beats("sess-1", 11, 1)))
}

// TestRecordBrokerRoutesBySession: a subscriber receives its session's
// beats, every one of them, and none of another session's.
func (s *RecordBrokerTestSuite) TestRecordBrokerRoutesBySession() {
	broker := sessionorch.NewRecordBroker(8)
	sub, err := broker.Subscribe("sess-1")
	s.Require().NoError(err)
	other, err := broker.Subscribe("sess-1")
	s.Require().NoError(err)

	s.Require().NoError(broker.PublishRecord(s.ctx, beats("sess-2", 1, 2)))
	s.Require().NoError(broker.PublishRecord(s.ctx, beats("sess-1", 3, 2)))
	s.Require().NoError(sub.Close())
	s.Require().NoError(other.Close())

	s.Equal([]string{"id-03", "id-04"}, s.drain(sub))
	s.Equal([]string{"id-03", "id-04"}, s.drain(other))
	lagged, _ := sub.Lagged()
	s.False(lagged, "an ordinary close is not a lag")
}

// TestRecordBrokerOneLaggardStarvesNobody: one full subscriber is closed;
// the other on the same session keeps every beat.
func (s *RecordBrokerTestSuite) TestRecordBrokerOneLaggardStarvesNobody() {
	broker := sessionorch.NewRecordBroker(2)
	slow, err := broker.Subscribe("sess-1")
	s.Require().NoError(err)
	fast, err := broker.Subscribe("sess-1")
	s.Require().NoError(err)

	s.Require().NoError(broker.PublishRecord(s.ctx, beats("sess-1", 1, 2)))
	s.Require().NoError(broker.PublishRecord(s.ctx, beats("sess-1", 3, 1)))
	var got []string
	for i := 0; i < 2; i++ {
		for _, evt := range <-fast.Units() {
			got = append(got, evt.ID)
		}
	}
	s.Require().ErrorIs(broker.PublishRecord(s.ctx, beats("sess-1", 4, 1)), sessionorch.ErrRecordLagged)
	for _, evt := range <-fast.Units() {
		got = append(got, evt.ID)
	}

	s.Equal([]string{"id-01", "id-02", "id-03", "id-04"}, got)
	s.Equal([]string{"id-01", "id-02", "id-03"}, s.drain(slow))
	lagged, last := slow.Lagged()
	s.True(lagged)
	s.Equal("id-03", last)
	s.Require().NoError(fast.Close())
}

// TestRecordBrokerClosed: Close ends every subscription, unlagged, and
// refuses new ones.
func (s *RecordBrokerTestSuite) TestRecordBrokerClosed() {
	broker := sessionorch.NewRecordBroker(0)
	sub, err := broker.Subscribe("sess-1")
	s.Require().NoError(err)
	s.Require().NoError(broker.Close())
	s.Empty(s.drain(sub))
	lagged, _ := sub.Lagged()
	s.False(lagged)
	_, err = broker.Subscribe("sess-1")
	s.ErrorIs(err, sessionorch.ErrBrokerClosed)
	s.Require().NoError(sub.Close(), "closing after the broker closed is safe")
}
