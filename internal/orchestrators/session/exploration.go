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
