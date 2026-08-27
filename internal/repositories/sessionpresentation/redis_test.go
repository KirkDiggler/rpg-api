package sessionpresentation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
)

const receiveTimeout = 2 * time.Second

type RedisSuite struct {
	suite.Suite
	ctx     context.Context
	server  *miniredis.Miniredis
	client1 *goredis.Client
	client2 *goredis.Client
	repo1   Repository
	repo2   Repository
}

func (s *RedisSuite) SetupTest() {
	s.ctx = context.Background()
	s.server = miniredis.RunT(s.T())
	s.client1 = goredis.NewClient(&goredis.Options{Addr: s.server.Addr()})
	s.client2 = goredis.NewClient(&goredis.Options{Addr: s.server.Addr()})
	s.repo1 = NewRedis(s.client1)
	s.repo2 = NewRedis(s.client2)
}

func (s *RedisSuite) TearDownTest() {
	if s.client1 != nil {
		_ = s.client1.Close()
	}
	if s.client2 != nil {
		_ = s.client2.Close()
	}
	if s.server != nil {
		s.server.Close()
	}
}

func (s *RedisSuite) TestPublish_FirstAcceptPublishesToEverySubscriberAndStoresTTL() {
	sub1, err := s.repo1.Subscribe(s.ctx, &SubscribeInput{Session: unsafeSessionID()})
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = sub1.Close() })

	sub2, err := s.repo2.Subscribe(s.ctx, &SubscribeInput{Session: unsafeSessionID()})
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = sub2.Close() })

	payload := []byte(`{"presentation_id":"present-1","attempt":1}`)
	out, err := s.repo1.Publish(s.ctx, &PublishInput{
		Session:        unsafeSessionID(),
		PresentationID: "present-1",
		Attempt:        1,
		Payload:        payload,
	})
	s.Require().NoError(err)
	s.Require().Equal(payload, out.Payload)

	s.Require().Equal(payload, receivePayload(s.T(), sub1.Payloads()))
	s.Require().Equal(payload, receivePayload(s.T(), sub2.Payloads()))
	s.Require().Equal(2*time.Minute, s.server.TTL(redisPlanKey(unsafeSessionID(), "present-1", 1)))
	stored, getErr := s.server.Get(redisPlanKey(unsafeSessionID(), "present-1", 1))
	s.Require().NoError(getErr)
	s.Require().Equal(payload, []byte(stored))
	hash := sha256.Sum256([]byte(unsafeSessionID()))
	s.Require().Contains(redisPlanKey(unsafeSessionID(), "present-1", 1), hex.EncodeToString(hash[:]))
	s.Require().NotContains(redisPlanKey(unsafeSessionID(), "present-1", 1), unsafeSessionID())
}

func (s *RedisSuite) TestPublish_EqualRetryReturnsStoredBytesWithoutRepublish() {
	sub, err := s.repo1.Subscribe(s.ctx, &SubscribeInput{Session: "session-1"})
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = sub.Close() })

	payload := []byte(`{"presentation_id":"present-1","attempt":1}`)
	first, err := s.repo1.Publish(s.ctx, &PublishInput{
		Session:        "session-1",
		PresentationID: "present-1",
		Attempt:        1,
		Payload:        payload,
	})
	s.Require().NoError(err)
	s.Require().Equal(payload, first.Payload)
	s.Require().Equal(payload, receivePayload(s.T(), sub.Payloads()))

	second, err := s.repo2.Publish(s.ctx, &PublishInput{
		Session:        "session-1",
		PresentationID: "present-1",
		Attempt:        1,
		Payload:        payload,
	})
	s.Require().NoError(err)
	s.Require().Equal(payload, second.Payload)
	assertNoPayload(s.T(), sub.Payloads())
}

func (s *RedisSuite) TestPublish_ConflictReturnsErrConflictWithoutRepublish() {
	sub, err := s.repo1.Subscribe(s.ctx, &SubscribeInput{Session: "session-1"})
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = sub.Close() })

	first := []byte(`{"presentation_id":"present-1","attempt":1,"value":"one"}`)
	_, err = s.repo1.Publish(s.ctx, &PublishInput{
		Session:        "session-1",
		PresentationID: "present-1",
		Attempt:        1,
		Payload:        first,
	})
	s.Require().NoError(err)
	s.Require().Equal(first, receivePayload(s.T(), sub.Payloads()))

	_, err = s.repo2.Publish(s.ctx, &PublishInput{
		Session:        "session-1",
		PresentationID: "present-1",
		Attempt:        1,
		Payload:        []byte(`{"presentation_id":"present-1","attempt":1,"value":"two"}`),
	})
	s.Require().Error(err)
	s.Require().True(errors.Is(err, ErrConflict))
	assertNoPayload(s.T(), sub.Payloads())
}

func (s *RedisSuite) TestPublish_DifferentAttemptPublishesIndependently() {
	sub, err := s.repo1.Subscribe(s.ctx, &SubscribeInput{Session: "session-1"})
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = sub.Close() })

	first := []byte(`{"presentation_id":"present-1","attempt":1}`)
	second := []byte(`{"presentation_id":"present-1","attempt":2}`)

	_, err = s.repo1.Publish(s.ctx, &PublishInput{
		Session:        "session-1",
		PresentationID: "present-1",
		Attempt:        1,
		Payload:        first,
	})
	s.Require().NoError(err)
	_, err = s.repo2.Publish(s.ctx, &PublishInput{
		Session:        "session-1",
		PresentationID: "present-1",
		Attempt:        2,
		Payload:        second,
	})
	s.Require().NoError(err)

	s.Require().Equal(first, receivePayload(s.T(), sub.Payloads()))
	s.Require().Equal(second, receivePayload(s.T(), sub.Payloads()))
}

func (s *RedisSuite) TestSubscribe_ContextCancellationClosesPlans() {
	ctx, cancel := context.WithCancel(s.ctx)
	sub, err := s.repo1.Subscribe(ctx, &SubscribeInput{Session: "session-cancel"})
	s.Require().NoError(err)

	cancel()
	waitForClosedChannel(s.T(), sub.Payloads())
}

func (s *RedisSuite) TestSubscribe_CloseClosesPlansAndReturnsErrClosedWhenRepeated() {
	sub, err := s.repo1.Subscribe(s.ctx, &SubscribeInput{Session: "session-close"})
	s.Require().NoError(err)

	s.Require().NoError(sub.Close())
	waitForClosedChannel(s.T(), sub.Payloads())
	s.Require().ErrorIs(sub.Close(), ErrClosed)
}

func TestRedisSuite(t *testing.T) {
	suite.Run(t, new(RedisSuite))
}

func unsafeSessionID() string {
	return "session/with spaces?#fragment"
}

func receivePayload(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	select {
	case payload, ok := <-ch:
		if !ok {
			t.Fatal("expected payload, channel closed")
		}
		return payload
	case <-time.After(receiveTimeout):
		t.Fatal("timed out waiting for payload")
		return nil
	}
}

func assertNoPayload(t *testing.T, ch <-chan []byte) {
	t.Helper()
	select {
	case payload, ok := <-ch:
		if !ok {
			t.Fatal("expected open channel with no payload, channel closed")
		}
		t.Fatalf("expected no payload, got %s", string(payload))
	case <-time.After(200 * time.Millisecond):
	}
}

func waitForClosedChannel(t *testing.T, ch <-chan []byte) {
	t.Helper()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(receiveTimeout):
		t.Fatal("timed out waiting for closed channel")
	}
}
