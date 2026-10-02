package dicesession

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

const (
	// Key pattern: dice_session:{world_id}:{entity_id}:{context}
	sessionKeyPrefix = "dice_session:"
	defaultTTL       = 15 * time.Minute

	// Error messages
	errWorldIDEmpty        = "world ID cannot be empty"
	errSessionNil          = "session cannot be nil"
	errEntityIDEmpty       = "entity ID cannot be empty"
	errContextEmpty        = "context cannot be empty"
	errSessionExpired      = "session has already expired"
	errStoredWorldMismatch = "stored dice session world does not match requested world"
)

// Config holds the configuration for the Redis repository
type Config struct {
	Client redisclient.Client
	Clock  clock.Clock
}

// Validate ensures all required dependencies are provided
func (c *Config) Validate() error {
	if c.Client == nil {
		return apierr.InvalidArgument("redis client is required")
	}
	if c.Clock == nil {
		return apierr.InvalidArgument("clock is required")
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
		return nil, apierr.Wrap(err, "invalid config")
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
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.EntityID == "" {
		return nil, apierr.InvalidArgument(errEntityIDEmpty)
	}
	if input.Context == "" {
		return nil, apierr.InvalidArgument(errContextEmpty)
	}

	now := r.clock.Now()
	ttl := input.TTL
	if ttl == 0 {
		ttl = defaultTTL
	}

	session := &DiceSession{
		WorldID:   input.WorldID,
		EntityID:  input.EntityID,
		Context:   input.Context,
		Rolls:     input.Rolls,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	// Serialize the session
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal session")
	}

	// Store in Redis with TTL under the world-scoped key
	key := r.buildKey(input.WorldID, input.EntityID, input.Context)
	err = r.client.Set(ctx, key, sessionJSON, ttl).Err()
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to store session in Redis")
	}

	return &CreateOutput{
		Session: session,
	}, nil
}

// Get retrieves a dice session by world, entity ID and context
func (r *redisRepository) Get(ctx context.Context, input GetInput) (*GetOutput, error) {
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.EntityID == "" {
		return nil, apierr.InvalidArgument(errEntityIDEmpty)
	}
	if input.Context == "" {
		return nil, apierr.InvalidArgument(errContextEmpty)
	}

	key := r.buildKey(input.WorldID, input.EntityID, input.Context)

	// Get from Redis
	sessionJSON, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, apierr.NotFound("dice session not found")
		}
		return nil, apierr.Wrapf(err, "failed to get session from Redis")
	}

	// Deserialize the session
	var session DiceSession
	if err := json.Unmarshal([]byte(sessionJSON), &session); err != nil {
		return nil, apierr.Wrapf(err, "failed to unmarshal session")
	}

	// A stored session whose ownership metadata contradicts the requested
	// world is corruption, never a valid read.
	if session.WorldID != input.WorldID {
		return nil, apierr.Internalf("%s: requested %q, stored %q",
			errStoredWorldMismatch, input.WorldID, session.WorldID)
	}

	// Check if session has expired
	if r.clock.Now().After(session.ExpiresAt) {
		// Session has expired, clean it up
		_ = r.client.Del(ctx, key)
		return nil, apierr.NotFound("dice session has expired")
	}

	return &GetOutput{
		Session: &session,
	}, nil
}

// Delete removes a dice session
func (r *redisRepository) Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.EntityID == "" {
		return nil, apierr.InvalidArgument(errEntityIDEmpty)
	}
	if input.Context == "" {
		return nil, apierr.InvalidArgument(errContextEmpty)
	}

	key := r.buildKey(input.WorldID, input.EntityID, input.Context)

	// Get the session first to count rolls. Delete keeps its documented
	// best-effort count: a failed pre-count read (including a corrupt stored
	// envelope) still deletes the world-scoped key and reports zero, matching
	// the #1047 contract. Only the delete's own error propagates.
	getOutput, err := r.Get(ctx, GetInput(input))

	var rollsDeleted int
	if err == nil && getOutput.Session != nil {
		// nolint:gosec // roll count is always small
		rollsDeleted = len(getOutput.Session.Rolls)
	}

	// Delete from Redis
	result := r.client.Del(ctx, key)
	if result.Err() != nil {
		return nil, apierr.Wrapf(result.Err(), "failed to delete session from Redis")
	}

	return &DeleteOutput{
		RollsDeleted: rollsDeleted,
	}, nil
}

// Update replaces an existing dice session (used for adding rolls)
func (r *redisRepository) Update(ctx context.Context, session *DiceSession) error {
	if session == nil {
		return apierr.InvalidArgument(errSessionNil)
	}
	if session.WorldID == "" {
		return apierr.InvalidArgument(errWorldIDEmpty)
	}
	if session.EntityID == "" {
		return apierr.InvalidArgument(errEntityIDEmpty)
	}
	if session.Context == "" {
		return apierr.InvalidArgument(errContextEmpty)
	}

	// Calculate remaining TTL
	now := r.clock.Now()
	if now.After(session.ExpiresAt) {
		return apierr.InvalidArgument(errSessionExpired)
	}

	remainingTTL := session.ExpiresAt.Sub(now)

	key := r.buildKey(session.WorldID, session.EntityID, session.Context)

	// If a record already exists at this world-scoped key, its stored
	// ownership must agree. A contradictory envelope is corruption and is not
	// overwritten. A missing key keeps the existing last-writer behavior.
	stored, readErr := r.client.Get(ctx, key).Bytes()
	switch {
	case readErr == nil:
		var existing DiceSession
		if unmarshalErr := json.Unmarshal(stored, &existing); unmarshalErr != nil {
			return apierr.Wrapf(unmarshalErr, "failed to unmarshal stored session")
		}
		if existing.WorldID != session.WorldID {
			return apierr.Internalf("%s: requested %q, stored %q",
				errStoredWorldMismatch, session.WorldID, existing.WorldID)
		}
	case readErr != redis.Nil:
		return apierr.Wrapf(readErr, "failed to read existing session")
	}

	// Serialize the session
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return apierr.Wrapf(err, "failed to marshal session")
	}

	// Update in Redis with remaining TTL
	err = r.client.Set(ctx, key, sessionJSON, remainingTTL).Err()
	if err != nil {
		return apierr.Wrapf(err, "failed to update session in Redis")
	}

	return nil
}

// buildKey creates the world-scoped Redis key for a dice session. World IDs
// are canonical decimal identifiers; entity and context IDs never contain the
// colon delimiter, so the (world, entity, context) tuple is unambiguous.
func (r *redisRepository) buildKey(worldID, entityID, context string) string {
	return fmt.Sprintf("%s%s:%s:%s", sessionKeyPrefix, worldID, entityID, context)
}
