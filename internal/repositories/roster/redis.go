package roster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// redisKeyPrefix namespaces every roster key. Its own prefix beside
// "lobby:" and "enc:v2:" so the stores cannot collide.
const redisKeyPrefix = "roster:"

// NewRedis returns a Redis-backed Repository.
//
// ttl is refreshed on every Save, mirroring the lobby repo's abandonment
// pattern: an abandoned encounter's roster expires with no reaper process.
// Pass 0 to disable expiration.
func NewRedis(client redisclient.Client, ttl time.Duration) Repository {
	return &redisRepo{client: client, ttl: ttl}
}

type redisRepo struct {
	client redisclient.Client
	ttl    time.Duration
}

func (r *redisRepo) Get(ctx context.Context, encounterID string) (*Data, error) {
	b, err := r.client.Get(ctx, redisKey(encounterID)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get roster %q: %w", encounterID, err)
	}
	var data Data
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("decode roster %q: %w", encounterID, err)
	}
	return &data, nil
}

func (r *redisRepo) Save(ctx context.Context, data *Data) error {
	if data == nil || data.EncounterID == "" {
		return errors.New("save roster: missing encounter id")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode roster %q: %w", data.EncounterID, err)
	}
	if err := r.client.Set(ctx, redisKey(data.EncounterID), b, r.ttl).Err(); err != nil {
		return fmt.Errorf("save roster %q: %w", data.EncounterID, err)
	}
	return nil
}

func redisKey(encounterID string) string {
	return redisKeyPrefix + encounterID
}
