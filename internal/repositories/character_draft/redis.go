package characterdraft

import (
	"context"
	"encoding/json"
	"time"

	redis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
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
	errDraftIDEmpty  = "draft ID cannot be empty"
	errPlayerIDEmpty = "player ID cannot be empty"
	errDraftExpired  = "draft has already expired"
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
		return errors.InvalidArgument("redis client is required")
	}
	if c.Clock == nil {
		return errors.InvalidArgument("clock is required")
	}
	if c.IDGenerator == nil {
		return errors.InvalidArgument("ID generator is required")
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
		return nil, errors.InvalidArgument(errDraftNil)
	}
	if input.Draft.PlayerID == "" {
		return nil, errors.InvalidArgument(errPlayerIDEmpty)
	}

	// Make a copy to avoid modifying input
	draft := *input.Draft

	// Repository generates ID if not provided
	if draft.ID == "" {
		draft.ID = r.idGen.Generate()
	}

	// Repository sets timestamps
	now := r.clock.Now()
	draft.CreatedAt = now.Unix()
	draft.UpdatedAt = now.Unix()

	// Set expiration if not provided
	if draft.ExpiresAt == 0 {
		draft.ExpiresAt = now.Add(defaultTTL).Unix()
	}

	// Validate expiration
	if draft.ExpiresAt > 0 {
		expiresAt := time.Unix(draft.ExpiresAt, 0)
		ttl := time.Until(expiresAt)
		if ttl <= 0 {
			return nil, errors.InvalidArgument(errDraftExpired)
		}
	}

	isNew := true
	// Check for existing draft for this player
	playerKey := playerMappingPrefix + draft.PlayerID
	existingDraftID, err := r.client.Get(ctx, playerKey).Result()
	if err != nil {
		if err != redis.Nil {
			return nil, errors.Wrapf(err, "failed to check existing draft")
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

	// Marshal new draft
	data, err := json.Marshal(&draft)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal draft")
	}

	// Calculate TTL
	ttl := defaultTTL
	if draft.ExpiresAt > 0 {
		expiresAt := time.Unix(draft.ExpiresAt, 0)
		ttl = time.Until(expiresAt)
		// Already validated above, so ttl should be positive
	}

	// Set draft data
	draftKey := draftKeyPrefix + draft.ID
	pipe.Set(ctx, draftKey, data, ttl)

	// Set player mapping (no TTL on this key)
	pipe.Set(ctx, playerKey, draft.ID, 0)

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create draft")
	}

	return &CreateOutput{Draft: &draft}, nil
}

func (r *redisRepository) Get(ctx context.Context, input GetInput) (*GetOutput, error) {
	if input.ID == "" {
		return nil, errors.InvalidArgument(errDraftIDEmpty)
	}

	key := draftKeyPrefix + input.ID
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, errors.NotFoundf("draft with ID %s not found", input.ID)
		}
		return nil, errors.Wrapf(err, "failed to get draft")
	}

	data := []byte(result)

	var draft dnd5e.CharacterDraft
	if err := json.Unmarshal(data, &draft); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal draft")
	}

	return &GetOutput{Draft: &draft}, nil
}

func (r *redisRepository) GetByPlayerID(ctx context.Context, input GetByPlayerIDInput) (*GetByPlayerIDOutput, error) {
	if input.PlayerID == "" {
		return nil, errors.InvalidArgument(errPlayerIDEmpty)
	}

	// Get draft ID from player mapping
	playerKey := playerMappingPrefix + input.PlayerID
	draftID, err := r.client.Get(ctx, playerKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, errors.NotFoundf("no draft found for player %s", input.PlayerID)
		}
		return nil, errors.Wrapf(err, "failed to get player draft mapping")
	}

	// Get the actual draft
	getOutput, err := r.Get(ctx, GetInput{ID: draftID})
	if err != nil {
		// If draft doesn't exist, clean up the mapping
		if errors.IsNotFound(err) {
			r.client.Del(ctx, playerKey)
		}
		return nil, err
	}

	return &GetByPlayerIDOutput{Draft: getOutput.Draft}, nil
}

func (r *redisRepository) Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	if input.Draft == nil {
		return nil, errors.InvalidArgument(errDraftNil)
	}
	if input.Draft.ID == "" {
		return nil, errors.InvalidArgument(errDraftIDEmpty)
	}

	key := draftKeyPrefix + input.Draft.ID

	// Check if exists
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check existence")
	}
	if exists == 0 {
		return nil, errors.NotFoundf("draft with ID %s not found", input.Draft.ID)
	}

	// Make a copy to avoid modifying input
	draft := *input.Draft

	// Repository updates timestamp
	draft.UpdatedAt = r.clock.Now().Unix()

	// Marshal draft
	data, err := json.Marshal(&draft)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal draft")
	}

	// Calculate TTL
	ttl := defaultTTL
	if draft.ExpiresAt > 0 {
		expiresAt := time.Unix(draft.ExpiresAt, 0)
		ttl = time.Until(expiresAt)
		if ttl <= 0 {
			return nil, errors.InvalidArgument(errDraftExpired)
		}
	}

	// Update with TTL
	if err := r.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return nil, errors.Wrapf(err, "failed to update draft")
	}

	return &UpdateOutput{Draft: &draft}, nil
}

func (r *redisRepository) Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	if input.ID == "" {
		return nil, errors.InvalidArgument(errDraftIDEmpty)
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
	if getOutput.Draft.PlayerID != "" {
		playerKey := playerMappingPrefix + getOutput.Draft.PlayerID
		pipe.Del(ctx, playerKey)
	}

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to delete draft")
	}

	return &DeleteOutput{}, nil
}
