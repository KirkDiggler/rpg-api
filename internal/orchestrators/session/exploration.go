package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

const explorationPrefix = "character-exploration:v1:"

// Exploration follows the canonical character identity/authority, not a player
// or a session TTL. The host stores opaque provider data and decides no rules.
type redisExplorationRepository struct{ client redisclient.Client }

func (r *redisExplorationRepository) GetExploration(ctx context.Context, id string) (*sdk.ExplorationData, error) {
	if id == "" {
		return nil, errors.New("exploration character id is required")
	}
	raw, err := r.client.Get(ctx, explorationPrefix+id).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, sdk.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var data sdk.ExplorationData
	if err = json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("decode exploration: %w", err)
	}
	return &data, nil
}
func (r *redisExplorationRepository) SaveExploration(ctx context.Context, data *sdk.ExplorationData) error {
	if data == nil || data.Character == "" {
		return errors.New("exploration character id is required")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, explorationPrefix+data.Character, raw, 0).Err()
}

// Discovery operations share character preferences across session IDs, never
// encounter knowledge or attempt history. This host
// therefore uses one store-wide coordinator for SDK operations when that
// capability is enabled. It trades parallelism for correct shared-record writes
// without teaching the host how counters merge. It is still process-local;
// distributed hosts must inject a coordinator covering their shared store.
type sharedStoreLocker struct{ inner *InProcessSessionLocker }

func (l sharedStoreLocker) LockSession(ctx context.Context, in *sdk.LockSessionInput) (*sdk.LockSessionOutput, error) {
	if in == nil {
		return nil, sdk.ErrNilInput
	}
	if in.Session == "" {
		return nil, sdk.ErrNoSessionID
	}
	return l.inner.LockSession(ctx, &sdk.LockSessionInput{Session: "shared-sdk-store"})
}
