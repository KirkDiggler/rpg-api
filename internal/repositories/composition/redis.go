package composition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	goredis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	worldcomposition "github.com/KirkDiggler/rpg-toolkit/world/composition"
)

const compositionKeyPrefix = "composition:"

type redisRepository struct {
	client redisclient.Client
}

// RedisConfig contains the Redis dependency for the repository.
type RedisConfig struct {
	Client redisclient.Client
}

// Validate verifies that the repository has its required dependency.
func (cfg *RedisConfig) Validate() error {
	if cfg == nil {
		return apierr.InvalidArgument("config cannot be nil")
	}
	if cfg.Client == nil {
		return apierr.InvalidArgument("redis client is required")
	}
	return nil
}

// NewRedis creates a Redis-backed composition repository.
func NewRedis(cfg *RedisConfig) (Repository, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &redisRepository{client: cfg.Client}, nil
}

func (r *redisRepository) Create(ctx context.Context, input *CreateInput) (*CreateOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("create input is required")
	}
	if input.Composition == nil {
		return nil, apierr.InvalidArgument("composition is required")
	}
	if input.Composition.WorldID == "" {
		return nil, apierr.InvalidArgument("composition world ID is required")
	}
	if input.Composition.ID == "" {
		return nil, apierr.InvalidArgument("composition ID is required")
	}

	stored, err := json.Marshal(input.Composition)
	if err != nil {
		return nil, apierr.Wrapf(err, "marshal composition %q", input.Composition.ID)
	}
	created, err := r.client.HSetNX(
		ctx,
		compositionKey(input.Composition.WorldID),
		input.Composition.ID,
		stored,
	).Result()
	if err != nil {
		return nil, apierr.Wrapf(err, "create composition %q", input.Composition.ID)
	}
	if !created {
		return nil, apierr.AlreadyExistsf("composition %s already exists", input.Composition.ID)
	}

	composition, err := decodeComposition(stored, input.Composition.WorldID, input.Composition.ID)
	if err != nil {
		return nil, err
	}
	return &CreateOutput{Composition: composition}, nil
}

func (r *redisRepository) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("get input is required")
	}
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument("world ID is required")
	}
	if input.ID == "" {
		return nil, apierr.InvalidArgument("composition ID is required")
	}

	stored, err := r.client.HGet(ctx, compositionKey(input.WorldID), input.ID).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return nil, apierr.NotFoundf("composition %s not found", input.ID)
		}
		return nil, apierr.Wrapf(err, "get composition %q", input.ID)
	}
	composition, err := decodeComposition(stored, input.WorldID, input.ID)
	if err != nil {
		return nil, err
	}
	return &GetOutput{Composition: composition}, nil
}

func (r *redisRepository) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("list input is required")
	}
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument("world ID is required")
	}

	stored, err := r.client.HGetAll(ctx, compositionKey(input.WorldID)).Result()
	if err != nil {
		return nil, apierr.Wrapf(err, "list compositions for world %q", input.WorldID)
	}
	ids := make([]string, 0, len(stored))
	for id := range stored {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	output := &ListOutput{Compositions: make([]*worldcomposition.Data, 0, len(ids))}
	for _, id := range ids {
		composition, decodeErr := decodeComposition([]byte(stored[id]), input.WorldID, id)
		if decodeErr != nil {
			return nil, decodeErr
		}
		output.Compositions = append(output.Compositions, composition)
	}
	return output, nil
}

func decodeComposition(stored []byte, worldID, id string) (*worldcomposition.Data, error) {
	var composition worldcomposition.Data
	if err := json.Unmarshal(stored, &composition); err != nil {
		return nil, apierr.Wrapf(err, "decode composition %q", id)
	}
	if composition.ID != id || composition.WorldID != worldID {
		return nil, apierr.Internalf("composition %q stored envelope does not match its world and ID", id)
	}
	return &composition, nil
}

func compositionKey(worldID string) string {
	digest := sha256.Sum256([]byte(worldID))
	return compositionKeyPrefix + hex.EncodeToString(digest[:])
}
