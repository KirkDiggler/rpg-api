package sessionpresentation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

const (
	redisPlanPrefix    = "sessionpresentation:plan:"
	redisChannelPrefix = "sessionpresentation:channel:"
	publishTTL         = 2 * time.Minute

	publishEqual    = int64(1)
	publishConflict = int64(-1)
	publishFirst    = int64(2)
)

var acceptAndPublishScript = goredis.NewScript(`
local existing = redis.call('GET', KEYS[1])
if existing then
  if existing == ARGV[1] then return 1 end
  return -1
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
redis.call('PUBLISH', KEYS[2], ARGV[1])
return 2
`)

func NewRedis(client redisclient.Client) Repository {
	return &redisRepository{client: client}
}

type redisRepository struct {
	client redisclient.Client
}

func (r *redisRepository) Publish(ctx context.Context, input *PublishInput) (*PublishOutput, error) {
	if err := validatePublishInput(input); err != nil {
		return nil, err
	}

	result, err := acceptAndPublishScript.Run(
		ctx,
		r.client,
		[]string{
			redisPlanKey(input.Session, input.PresentationID, input.Attempt),
			redisChannelKey(input.Session),
		},
		input.Payload,
		publishTTL.Milliseconds(),
	).Int64()
	if err != nil {
		return nil, fmt.Errorf("publish session presentation %q/%q/%d: %w", input.Session, input.PresentationID, input.Attempt, err)
	}

	switch result {
	case publishEqual, publishFirst:
		return &PublishOutput{Payload: bytes.Clone(input.Payload)}, nil
	case publishConflict:
		return nil, ErrConflict
	default:
		return nil, fmt.Errorf("publish session presentation %q/%q/%d: unexpected script result %d", input.Session, input.PresentationID, input.Attempt, result)
	}
}

func (r *redisRepository) Subscribe(ctx context.Context, input *SubscribeInput) (Subscription, error) {
	if err := validateSubscribeInput(input); err != nil {
		return nil, err
	}

	pubsub := r.client.Subscribe(ctx, redisChannelKey(input.Session))
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, fmt.Errorf("subscribe session presentation %q: %w", input.Session, err)
	}

	subscription := &redisSubscription{
		pubsub:   pubsub,
		payloads: make(chan []byte, 1),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go subscription.run(ctx)
	go subscription.closeOnContext(ctx)
	return subscription, nil
}

type redisSubscription struct {
	pubsub   *goredis.PubSub
	payloads chan []byte
	stop     chan struct{}
	done     chan struct{}

	closeMu sync.Mutex
	closed  bool
}

func (s *redisSubscription) Payloads() <-chan []byte {
	return s.payloads
}

func (s *redisSubscription) Close() error {
	if !s.initiateClose() {
		return ErrClosed
	}

	err := s.pubsub.Close()
	<-s.done
	if err != nil && !errors.Is(err, goredis.ErrClosed) {
		return fmt.Errorf("close session presentation subscription: %w", err)
	}
	return nil
}

func (s *redisSubscription) closeOnContext(ctx context.Context) {
	<-ctx.Done()
	if !s.initiateClose() {
		return
	}
	_ = s.pubsub.Close()
}

func (s *redisSubscription) initiateClose() bool {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return false
	}
	s.closed = true
	close(s.stop)
	return true
}

func (s *redisSubscription) run(ctx context.Context) {
	defer close(s.done)
	defer close(s.payloads)

	for {
		message, err := s.pubsub.ReceiveMessage(ctx)
		if err != nil {
			return
		}

		payload := bytes.Clone([]byte(message.Payload))
		select {
		case s.payloads <- payload:
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		}
	}
}

func redisPlanKey(sessionID, presentationID string, attempt uint32) string {
	return fmt.Sprintf("%s%s:%s:%d", redisPlanPrefix, redisSafeSuffix(sessionID), presentationID, attempt)
}

func redisChannelKey(sessionID string) string {
	return redisChannelPrefix + redisSafeSuffix(sessionID)
}

func redisSafeSuffix(sessionID string) string {
	hash := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(hash[:])
}
