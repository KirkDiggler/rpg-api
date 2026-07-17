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

// redisPlayerPrefix namespaces the player id -> lobby id secondary index
// GetByPlayerID reads (GetMyActiveLobby / resume-after-refresh,
// rpg-dnd5e-web#444). Deliberately its own top-level prefix ("player:...",
// not "lobby:player:...") since this index is conceptually owned by the
// player, not the lobby — a future non-lobby feature keyed by player id
// would want the same namespace root.
const redisPlayerPrefix = "player:"

// redisPlayerSuffix closes the player key: player:{playerID}:lobby.
const redisPlayerSuffix = ":lobby"

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

func (r *redisRepo) GetByPlayerID(ctx context.Context, playerID string) (*Data, error) {
	id, err := r.client.Get(ctx, redisPlayerKey(playerID)).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get lobby by player %q: %w", playerID, err)
	}
	return r.Get(ctx, id)
}

// Save writes the primary record, the join_ref index, and a player index
// entry per current member in one MULTI/EXEC pipeline, so no reader ever
// observes a partially-updated state (a crash or network failure between
// independent SETs could otherwise leave a record with a stale or missing
// secondary index).
//
// Save only ever adds/refreshes player index entries for members present in
// data.Members — see the Repository.Save doc comment for why a removed
// member's stale entry is a caller responsibility (ClearPlayerIndex), not
// something Save can infer from a single Data value.
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
	for playerID := range data.Members {
		pipe.Set(ctx, redisPlayerKey(playerID), data.ID, r.ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("save lobby %q: %w", data.ID, err)
	}
	return nil
}

func (r *redisRepo) ClearPlayerIndex(ctx context.Context, playerID string) error {
	if err := r.client.Del(ctx, redisPlayerKey(playerID)).Err(); err != nil {
		return fmt.Errorf("clear player index %q: %w", playerID, err)
	}
	return nil
}

func redisKey(id string) string {
	return redisKeyPrefix + id
}

func redisJoinRefKey(joinRef string) string {
	return redisJoinRefPrefix + joinRef
}

func redisPlayerKey(playerID string) string {
	return redisPlayerPrefix + playerID + redisPlayerSuffix
}
