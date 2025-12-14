package encounter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/apierrors"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

const (
	// encounterChannelPattern is the pattern for encounter event channels
	// Format: encounter:{encounterID}:events
	encounterChannelPattern = "encounter:%s:events"

	// Error messages
	errEventNil            = "event cannot be nil"
	errEncounterIDEmpty    = "encounter ID cannot be empty"
	errSubscriptionIDEmpty = "subscription ID cannot be empty"
)

// redisPublisher implements Publisher using Redis pub/sub
type redisPublisher struct {
	client        redisclient.Client
	idgen         idgen.Generator
	subscriptions map[string]*subscription // subscriptionID -> subscription
	mu            sync.RWMutex
}

// subscription tracks a single subscription to encounter events
type subscription struct {
	encounterID string
	events      chan *entities.EncounterEvent
	errors      chan error
	cancel      context.CancelFunc
	done        chan struct{}
}

// RedisConfig contains configuration for the Redis publisher
type RedisConfig struct {
	Client redisclient.Client
	IDGen  idgen.Generator
}

// Validate validates the RedisConfig
func (cfg *RedisConfig) Validate() error {
	if cfg == nil {
		return apierrors.InvalidArgument("config cannot be nil")
	}
	if cfg.Client == nil {
		return apierrors.InvalidArgument("client cannot be nil")
	}
	return nil
}

// NewRedis creates a new Redis-backed encounter event publisher
func NewRedis(cfg *RedisConfig) (Publisher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Use default IDGen if none provided
	gen := cfg.IDGen
	if gen == nil {
		gen = idgen.NewSequential("sub")
	}

	return &redisPublisher{
		client:        cfg.Client,
		idgen:         gen,
		subscriptions: make(map[string]*subscription),
	}, nil
}

// Publish publishes an event to all subscribers of the encounter
func (p *redisPublisher) Publish(ctx context.Context, input *PublishInput) (*PublishOutput, error) {
	if input == nil {
		return nil, apierrors.InvalidArgument("input cannot be nil")
	}
	if input.Event == nil {
		return nil, apierrors.InvalidArgument(errEventNil)
	}
	if input.EncounterID == "" {
		return nil, apierrors.InvalidArgument(errEncounterIDEmpty)
	}

	// Serialize event to JSON
	data, err := json.Marshal(input.Event)
	if err != nil {
		return nil, apierrors.Wrapf(err, "failed to marshal event")
	}

	// Publish to Redis channel
	channel := fmt.Sprintf(encounterChannelPattern, input.EncounterID)
	if err := p.client.Publish(ctx, channel, data).Err(); err != nil {
		return nil, apierrors.Wrapf(err, "failed to publish event to channel %s", channel)
	}

	return &PublishOutput{}, nil
}

// Subscribe subscribes to events for a specific encounter
func (p *redisPublisher) Subscribe(ctx context.Context, input *SubscribeInput) (*SubscribeOutput, error) {
	if input == nil {
		return nil, apierrors.InvalidArgument("input cannot be nil")
	}
	if input.EncounterID == "" {
		return nil, apierrors.InvalidArgument(errEncounterIDEmpty)
	}

	// Generate unique subscription ID
	subscriptionID := p.idgen.Generate()

	// Create channels for events and errors
	events := make(chan *entities.EncounterEvent, 100) // Buffered to prevent blocking
	errs := make(chan error, 10)
	done := make(chan struct{})

	// Create cancellable context for the subscription
	subCtx, cancel := context.WithCancel(context.Background())

	// Create subscription record
	sub := &subscription{
		encounterID: input.EncounterID,
		events:      events,
		errors:      errs,
		cancel:      cancel,
		done:        done,
	}

	// Store subscription
	p.mu.Lock()
	p.subscriptions[subscriptionID] = sub
	p.mu.Unlock()

	// Start listening for events in a goroutine
	go p.listen(subCtx, sub)

	return &SubscribeOutput{
		SubscriptionID: subscriptionID,
		Events:         events,
		Errors:         errs,
	}, nil
}

// Unsubscribe unsubscribes from encounter events
func (p *redisPublisher) Unsubscribe(ctx context.Context, input *UnsubscribeInput) (*UnsubscribeOutput, error) {
	if input == nil {
		return nil, apierrors.InvalidArgument("input cannot be nil")
	}
	if input.SubscriptionID == "" {
		return nil, apierrors.InvalidArgument(errSubscriptionIDEmpty)
	}

	p.mu.Lock()
	sub, exists := p.subscriptions[input.SubscriptionID]
	if !exists {
		p.mu.Unlock()
		return nil, apierrors.NotFoundf("subscription %s not found", input.SubscriptionID)
	}
	delete(p.subscriptions, input.SubscriptionID)
	p.mu.Unlock()

	// Cancel the subscription context
	sub.cancel()

	// Wait for listener to finish
	<-sub.done

	// Close channels
	close(sub.events)
	close(sub.errors)

	return &UnsubscribeOutput{}, nil
}

// listen listens for Redis pub/sub messages and forwards them to the subscription
func (p *redisPublisher) listen(ctx context.Context, sub *subscription) {
	defer close(sub.done)

	channel := fmt.Sprintf(encounterChannelPattern, sub.encounterID)
	pubsub := p.client.Subscribe(ctx, channel)
	defer func() {
		_ = pubsub.Close()
	}()

	// Wait for confirmation that subscription is active
	_, err := pubsub.Receive(ctx)
	if err != nil {
		select {
		case sub.errors <- apierrors.Wrapf(err, "failed to confirm subscription to %s", channel):
		case <-ctx.Done():
		}
		return
	}

	// Listen for messages
	msgChan := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgChan:
			if !ok {
				// Channel closed
				return
			}

			// Deserialize event
			var event entities.EncounterEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				select {
				case sub.errors <- apierrors.Wrapf(err, "failed to unmarshal event"):
				case <-ctx.Done():
					return
				}
				continue
			}

			// Send event to subscriber
			select {
			case sub.events <- &event:
			case <-ctx.Done():
				return
			}
		}
	}
}
