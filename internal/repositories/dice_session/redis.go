package dicesession

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

const (
	// Key pattern: dice_session:{entity_id}:{context}
	sessionKeyPrefix = "dice_session:"
	defaultTTL       = 15 * time.Minute

	// Error messages
	errSessionNil     = "session cannot be nil"
	errEntityIDEmpty  = "entity ID cannot be empty"
	errContextEmpty   = "context cannot be empty"
	errSessionExpired = "session has already expired"
)

// Config holds the configuration for the Redis repository
type Config struct {
	Client redisclient.Client
	Clock  clock.Clock
}

// Validate ensures all required dependencies are provided
func (c *Config) Validate() error {
	if c.Client == nil {
		return errors.InvalidArgument("redis client is required")
	}
	if c.Clock == nil {
		return errors.InvalidArgument("clock is required")
	}
	return nil
}

type redisRepository struct {
	client redisclient.Client
	clock  clock.Clock
}

// NewRedisRepository creates a new Redis repository for dice sessions
func NewRedisRepository(cfg *Config) (Repository, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	return &redisRepository{
		client: cfg.Client,
		clock:  cfg.Clock,
	}, nil
}

// Ensure redisRepository implements Repository
var _ Repository = (*redisRepository)(nil)

// Create stores a new dice session with the specified TTL
func (r *redisRepository) Create(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	if input.EntityID == "" {
		return nil, errors.InvalidArgument(errEntityIDEmpty)
	}
	if input.Context == "" {
		return nil, errors.InvalidArgument(errContextEmpty)
	}

	now := r.clock.Now()
	ttl := input.TTL
	if ttl == 0 {
		ttl = defaultTTL
	}

	session := &DiceSession{
		EntityID:  input.EntityID,
		Context:   input.Context,
		Rolls:     input.Rolls,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	// Serialize the session
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal session")
	}

	// Store in Redis with TTL
	key := r.buildKey(input.EntityID, input.Context)
	err = r.client.Set(ctx, key, sessionJSON, ttl).Err()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to store session in Redis")
	}

	return &CreateOutput{
		Session: session,
	}, nil
}

// Get retrieves a dice session by entity ID and context
func (r *redisRepository) Get(ctx context.Context, input GetInput) (*GetOutput, error) {
	if input.EntityID == "" {
		return nil, errors.InvalidArgument(errEntityIDEmpty)
	}
	if input.Context == "" {
		return nil, errors.InvalidArgument(errContextEmpty)
	}

	key := r.buildKey(input.EntityID, input.Context)

	// Get from Redis
	sessionJSON, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, errors.NotFound("dice session not found")
		}
		return nil, errors.Wrapf(err, "failed to get session from Redis")
	}

	// Deserialize the session
	var session DiceSession
	if err := json.Unmarshal([]byte(sessionJSON), &session); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal session")
	}

	// Check if session has expired
	if r.clock.Now().After(session.ExpiresAt) {
		// Session has expired, clean it up
		_ = r.client.Del(ctx, key)
		return nil, errors.NotFound("dice session has expired")
	}

	return &GetOutput{
		Session: &session,
	}, nil
}

// Delete removes a dice session
func (r *redisRepository) Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	if input.EntityID == "" {
		return nil, errors.InvalidArgument(errEntityIDEmpty)
	}
	if input.Context == "" {
		return nil, errors.InvalidArgument(errContextEmpty)
	}

	key := r.buildKey(input.EntityID, input.Context)

	// Get the session first to count rolls
	getOutput, err := r.Get(ctx, GetInput(input))

	var rollsDeleted int32
	if err == nil && getOutput.Session != nil {
		// nolint:gosec // roll count is always small
		rollsDeleted = int32(len(getOutput.Session.Rolls))
	}

	// Delete from Redis
	result := r.client.Del(ctx, key)
	if result.Err() != nil {
		return nil, errors.Wrapf(result.Err(), "failed to delete session from Redis")
	}

	return &DeleteOutput{
		RollsDeleted: rollsDeleted,
	}, nil
}

// Update replaces an existing dice session (used for adding rolls)
func (r *redisRepository) Update(ctx context.Context, session *DiceSession) error {
	if session == nil {
		return errors.InvalidArgument(errSessionNil)
	}
	if session.EntityID == "" {
		return errors.InvalidArgument(errEntityIDEmpty)
	}
	if session.Context == "" {
		return errors.InvalidArgument(errContextEmpty)
	}

	// Calculate remaining TTL
	now := r.clock.Now()
	if now.After(session.ExpiresAt) {
		return errors.InvalidArgument(errSessionExpired)
	}

	remainingTTL := session.ExpiresAt.Sub(now)

	// Serialize the session
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal session")
	}

	// Update in Redis with remaining TTL
	key := r.buildKey(session.EntityID, session.Context)
	err = r.client.Set(ctx, key, sessionJSON, remainingTTL).Err()
	if err != nil {
		return errors.Wrapf(err, "failed to update session in Redis")
	}

	return nil
}

// buildKey creates the Redis key for a dice session
func (r *redisRepository) buildKey(entityID, context string) string {
	return fmt.Sprintf("%s%s:%s", sessionKeyPrefix, entityID, context)
}
