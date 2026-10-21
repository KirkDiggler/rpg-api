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
		case evt, ok := <-sub.Events():
			if !ok {
				return got
			}
			got = append(got, evt.ID)
		case <-time.After(time.Second):
			s.FailNow("the subscription never closed")
			return got
		}
	}
}

// TestRecordBrokerLaggedSubscriberIsTold: with a buffer of 4, five beats
// published unread give the subscriber the first four, then a closed channel
// marked lagged with the fourth id. Nothing is skipped silently: the fifth is
// never handed over as if the stream were whole.
func (s *RecordBrokerTestSuite) TestRecordBrokerLaggedSubscriberIsTold() {
	broker := sessionorch.NewRecordBroker(4)
	sub, err := broker.Subscribe("sess-1")
	s.Require().NoError(err)

	err = broker.PublishRecord(s.ctx, beats("sess-1", 1, 5))
	s.Require().ErrorIs(err, sessionorch.ErrRecordLagged)

	s.Equal([]string{"id-01", "id-02", "id-03", "id-04"}, s.drain(sub))
	lagged, last := sub.Lagged()
	s.True(lagged)
	s.Equal("id-04", last)

	// A lagged subscriber is gone: later beats are not queued for it.
	s.Require().NoError(broker.PublishRecord(s.ctx, beats("sess-1", 6, 1)))
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
	got := make([]string, 0, 3)
	got = append(got, (<-fast.Events()).ID, (<-fast.Events()).ID)
	s.Require().ErrorIs(broker.PublishRecord(s.ctx, beats("sess-1", 3, 1)), sessionorch.ErrRecordLagged)
	got = append(got, (<-fast.Events()).ID)

	s.Equal([]string{"id-01", "id-02", "id-03"}, got)
	s.Equal([]string{"id-01", "id-02"}, s.drain(slow))
	lagged, last := slow.Lagged()
	s.True(lagged)
	s.Equal("id-02", last)
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
