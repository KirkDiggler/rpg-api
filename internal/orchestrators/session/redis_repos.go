package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// Key prefixes are distinct from the old encounter path's ("enc:v2:" in
// internal/repositories/encounters/v2/redis.go) and from each other (design
// rule S13: one repository per data type), so the two stacks' Redis
// footprints never collide even though they run in the same process and
// against the same Redis instance during coexistence.
const (
	sessionKeyPrefix    = "session:v1alpha1:"
	sessionEncKeyPrefix = "session-enc:v1alpha1:"
)

// SessionData and EncounterData are opaque blobs the SDK builds: this
// repository round-trips their bytes and never constructs or inspects them
// (design §3, toolkit S12/S13 -- key-value, one repository per data type).

// redisSessionRepository is the Redis-backed sdk.SessionRepository.
type redisSessionRepository struct {
	client redisclient.Client
	ttl    time.Duration
}

// NewSessionRepository returns a Redis-backed sdk.SessionRepository.
// ttl is the per-key expiration applied on save; pass 0 to disable expiration.
func NewSessionRepository(client redisclient.Client, ttl time.Duration) sdk.SessionRepository {
	return &redisSessionRepository{client: client, ttl: ttl}
}

// GetSession implements sdk.SessionRepository.
func (r *redisSessionRepository) GetSession(ctx context.Context, id string) (*sdk.SessionData, error) {
	b, err := r.client.Get(ctx, sessionKeyPrefix+id).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, fmt.Errorf("session %q: %w", id, sdk.ErrNotFound)
		}
		return nil, fmt.Errorf("get session %q: %w", id, err)
	}
	var out sdk.SessionData
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("unmarshal stored session %q: %w", id, err)
	}
	return &out, nil
}

// SaveSession implements sdk.SessionRepository.
func (r *redisSessionRepository) SaveSession(ctx context.Context, data *sdk.SessionData) error {
	if data == nil {
		return errors.New("session: SaveSession data is required")
	}
	if data.ID == "" {
		return errors.New("session: SaveSession data.ID is required")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal session %q: %w", data.ID, err)
	}
	if err := r.client.Set(ctx, sessionKeyPrefix+data.ID, b, r.ttl).Err(); err != nil {
		return fmt.Errorf("set session %q: %w", data.ID, err)
	}
	return nil
}

// redisEncounterRepository is the Redis-backed sdk.EncounterRepository.
type redisEncounterRepository struct {
	client redisclient.Client
	ttl    time.Duration
}

// NewEncounterRepository returns a Redis-backed sdk.EncounterRepository.
// ttl is the per-key expiration applied on save; pass 0 to disable expiration.
func NewEncounterRepository(client redisclient.Client, ttl time.Duration) sdk.EncounterRepository {
	return &redisEncounterRepository{client: client, ttl: ttl}
}

// GetEncounter implements sdk.EncounterRepository.
func (r *redisEncounterRepository) GetEncounter(ctx context.Context, id string) (*tkencounter.EncounterData, error) {
	b, err := r.client.Get(ctx, sessionEncKeyPrefix+id).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, fmt.Errorf("encounter %q: %w", id, sdk.ErrNotFound)
		}
		return nil, fmt.Errorf("get encounter %q: %w", id, err)
	}
	var out tkencounter.EncounterData
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("unmarshal stored encounter %q: %w", id, err)
	}
	return &out, nil
}

// SaveEncounter implements sdk.EncounterRepository.
func (r *redisEncounterRepository) SaveEncounter(ctx context.Context, id string, data *tkencounter.EncounterData) error {
	if id == "" {
		return errors.New("session: SaveEncounter id is required")
	}
	if data == nil {
		return errors.New("session: SaveEncounter data is required")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal encounter %q: %w", id, err)
	}
	if err := r.client.Set(ctx, sessionEncKeyPrefix+id, b, r.ttl).Err(); err != nil {
		return fmt.Errorf("set encounter %q: %w", id, err)
	}
	return nil
}
