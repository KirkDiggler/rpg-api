package lobby

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// redisKeyPrefix namespaces every lobby key. Separate from the v2 encounter
// repo's "enc:v2:" prefix so the two stores cannot collide.
const redisKeyPrefix = "lobby:"

// redisJoinRefPrefix namespaces the join_ref -> lobby id secondary index.
// JoinLobby is the only RPC that looks a lobby up by join_ref rather than id
// (lobby-surface.md's CreateLobby -> {lobby_id, join_ref} split), so this
// index is the only way to serve it in O(1).
const redisJoinRefPrefix = "lobby:joinref:"

// NewRedis returns a Redis-backed Repository.
//
// ttl is refreshed on every Save (both the primary record and the join_ref
// index), per lobby-surface.md "Abandonment": an abandoned WAITING lobby
// expires with no reaper process, mirroring the v2 encounter repo's pattern
// (internal/repositories/encounters/v2/redis.go). Pass 0 to disable
// expiration.
func NewRedis(client redisclient.Client, ttl time.Duration) Repository {
	return &redisRepo{client: client, ttl: ttl}
}

type redisRepo struct {
	client redisclient.Client
	ttl    time.Duration
}

func (r *redisRepo) Get(ctx context.Context, id string) (*Data, error) {
	b, err := r.client.Get(ctx, redisKey(id)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get lobby %q: %w", id, err)
	}
	return decode(b, id)
}

func (r *redisRepo) GetByJoinRef(ctx context.Context, joinRef string) (*Data, error) {
	id, err := r.client.Get(ctx, redisJoinRefKey(joinRef)).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get lobby by join_ref %q: %w", joinRef, err)
	}
	return r.Get(ctx, id)
}

// Save writes the primary record and the join_ref index in one MULTI/EXEC
// pipeline, so the two keys never observe a partially-updated state (a crash
// or network failure between two independent SETs could otherwise leave a
// record with no working join_ref index, or vice versa).
func (r *redisRepo) Save(ctx context.Context, data *Data) error {
	if data == nil {
		return errors.New("data is required")
	}
	if data.ID == "" {
		return errors.New("data.ID is required")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal lobby %q: %w", data.ID, err)
	}
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, redisKey(data.ID), b, r.ttl)
	if data.JoinRef != "" {
		pipe.Set(ctx, redisJoinRefKey(data.JoinRef), data.ID, r.ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("save lobby %q: %w", data.ID, err)
	}
	return nil
}

func redisKey(id string) string {
	return redisKeyPrefix + id
}

func redisJoinRefKey(joinRef string) string {
	return redisJoinRefPrefix + joinRef
}
