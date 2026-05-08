package encounters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-toolkit/encounter"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// redisKeyPrefix is the prefix used for all v2 encounter keys in Redis.
// Separate from v1 (`encounter:`) so the two repos cannot collide.
const redisKeyPrefix = "enc:v2:"

// NewRedis returns a Redis-backed Repository.
//
// Save and Get round-trip the data through JSON serialization on every call,
// matching NewInMemory's contract: callers cannot mutate stored state after
// Save (no aliasing) and the toolkit's JSON tags are exercised on every write.
//
// ttl is the per-key expiration applied on Save. Pass 0 to disable expiration.
func NewRedis(client redisclient.Client, ttl time.Duration) Repository {
	return &redisRepo{client: client, ttl: ttl}
}

type redisRepo struct {
	client redisclient.Client
	ttl    time.Duration
}

func (r *redisRepo) Get(ctx context.Context, id string) (*encounter.Data, error) {
	b, err := r.client.Get(ctx, redisKey(id)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get encounter %q: %w", id, err)
	}
	var out encounter.Data
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("unmarshal stored encounter %q: %w", id, err)
	}
	return &out, nil
}

func (r *redisRepo) Save(ctx context.Context, data *encounter.Data) error {
	if data == nil {
		return errors.New("data is required")
	}
	if data.ID == "" {
		return errors.New("data.ID is required")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal encounter %q: %w", data.ID, err)
	}
	if err := r.client.Set(ctx, redisKey(string(data.ID)), b, r.ttl).Err(); err != nil {
		return fmt.Errorf("set encounter %q: %w", data.ID, err)
	}
	return nil
}

func redisKey(id string) string {
	return redisKeyPrefix + id
}
