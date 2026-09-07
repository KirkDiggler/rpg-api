package characterdraft

import (
	"context"
	"encoding/json"
	"time"

	redis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

const (
	draftKeyPrefix      = "draft:"
	playerMappingPrefix = "draft:player:"
	defaultTTL          = 24 * time.Hour

	// Error messages
	errDraftNil      = "draft cannot be nil"
	errDraftDataNil  = "draft data cannot be nil"
	errDraftIDEmpty  = "draft ID cannot be empty"
	errPlayerIDEmpty = "player ID cannot be empty"
)

// Config holds the configuration for the Redis repository
type Config struct {
	Client      redisclient.Client
	Clock       clock.Clock
	IDGenerator idgen.Generator
}

// Validate ensures all required dependencies are provided
func (c *Config) Validate() error {
	if c.Client == nil {
		return apierr.InvalidArgument("redis client is required")
	}
	if c.Clock == nil {
		return apierr.InvalidArgument("clock is required")
	}
	if c.IDGenerator == nil {
		return apierr.InvalidArgument("ID generator is required")
	}
	return nil
}

type redisRepository struct {
	client redisclient.Client
	clock  clock.Clock
	idGen  idgen.Generator
}

// NewRedis creates a new Redis-backed character draft repository
func NewRedis(cfg *Config) (Repository, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &redisRepository{
		client: cfg.Client,
		clock:  cfg.Clock,
		idGen:  cfg.IDGenerator,
	}, nil
}

func (r *redisRepository) Create(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	if input.Draft == nil {
		return nil, apierr.InvalidArgument(errDraftNil)
	}
	if input.Draft.Data == nil {
		return nil, apierr.InvalidArgument(errDraftDataNil)
	}
	if input.Draft.Data.PlayerID == "" {
		return nil, apierr.InvalidArgument(errPlayerIDEmpty)
	}

	draft := input.Draft

	// Repository generates ID if not provided
	if draft.Data.ID == "" {
		draft.Data.ID = r.idGen.Generate()
	}

	isNew := true
	// Check for existing draft for this player
	playerKey := playerMappingPrefix + draft.Data.PlayerID
	existingDraftID, err := r.client.Get(ctx, playerKey).Result()
	if err != nil {
		if err != redis.Nil {
			return nil, apierr.Wrapf(err, "failed to check existing draft")
		}
		// err == redis.Nil means no existing draft, so isNew stays true
	} else {
		// Found existing draft, so this is a replacement
		isNew = false
	}

	// Start transaction
	pipe := r.client.TxPipeline()

	// Delete existing draft if any
	if !isNew {
		oldDraftKey := draftKeyPrefix + existingDraftID
		pipe.Del(ctx, oldDraftKey)
	}

	data, err := json.Marshal(draft)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal draft")
	}

	// Set draft data
	draftKey := draftKeyPrefix + draft.Data.ID
	pipe.Set(ctx, draftKey, data, defaultTTL)

	// Set player mapping (no TTL on this key)
	pipe.Set(ctx, playerKey, draft.Data.ID, 0)

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to create draft")
	}

	return &CreateOutput{Draft: draft}, nil
}

func (r *redisRepository) Get(ctx context.Context, input GetInput) (*GetOutput, error) {
	if input.ID == "" {
		return nil, apierr.InvalidArgument(errDraftIDEmpty)
	}

	key := draftKeyPrefix + input.ID
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, apierr.NotFoundf("draft with ID %s not found", input.ID)
		}
		return nil, apierr.Wrapf(err, "failed to get draft")
	}

	var draft entities.CharacterDraft
	if err := json.Unmarshal([]byte(result), &draft); err != nil {
		return nil, apierr.Wrapf(err, "failed to unmarshal draft")
	}

	return &GetOutput{Draft: &draft}, nil
}

func (r *redisRepository) GetByPlayerID(ctx context.Context, input GetByPlayerIDInput) (*GetByPlayerIDOutput, error) {
	if input.PlayerID == "" {
		return nil, apierr.InvalidArgument(errPlayerIDEmpty)
	}

	// Get draft ID from player mapping
	playerKey := playerMappingPrefix + input.PlayerID
	draftID, err := r.client.Get(ctx, playerKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, apierr.NotFoundf("no draft found for player %s", input.PlayerID)
		}
		return nil, apierr.Wrapf(err, "failed to get player draft mapping")
	}

	// Get the actual draft
	getOutput, err := r.Get(ctx, GetInput{ID: draftID})
	if err != nil {
		// If draft doesn't exist, clean up the mapping
		if apierr.IsNotFound(err) {
			r.client.Del(ctx, playerKey)
		}
		return nil, err
	}

	return &GetByPlayerIDOutput{Draft: getOutput.Draft}, nil
}

func (r *redisRepository) Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	if input.Draft == nil {
		return nil, apierr.InvalidArgument(errDraftNil)
	}
	if input.Draft.Data == nil {
		return nil, apierr.InvalidArgument(errDraftDataNil)
	}
	if input.Draft.Data.ID == "" {
		return nil, apierr.InvalidArgument(errDraftIDEmpty)
	}

	key := draftKeyPrefix + input.Draft.Data.ID

	// Check if exists
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to check existence")
	}
	if exists == 0 {
		return nil, apierr.NotFoundf("draft with ID %s not found", input.Draft.Data.ID)
	}

	draft := input.Draft

	// Marshal draft
	data, err := json.Marshal(draft)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal draft")
	}

	// Update with TTL
	if err := r.client.Set(ctx, key, data, defaultTTL).Err(); err != nil {
		return nil, apierr.Wrapf(err, "failed to update draft")
	}

	return &UpdateOutput{Draft: draft}, nil
}

func (r *redisRepository) Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	if input.ID == "" {
		return nil, apierr.InvalidArgument(errDraftIDEmpty)
	}

	// Get draft to find player ID
	getOutput, err := r.Get(ctx, GetInput(input))
	if err != nil {
		return nil, err
	}

	pipe := r.client.TxPipeline()

	// Delete draft
	draftKey := draftKeyPrefix + input.ID
	pipe.Del(ctx, draftKey)

	// Delete player mapping
	if getOutput.Draft.Data != nil && getOutput.Draft.Data.PlayerID != "" {
		playerKey := playerMappingPrefix + getOutput.Draft.Data.PlayerID
		pipe.Del(ctx, playerKey)
	}

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to delete draft")
	}

	return &DeleteOutput{}, nil
}
