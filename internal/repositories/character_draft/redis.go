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
	errWorldIDEmpty             = "world ID cannot be empty"
	errDraftNil                 = "draft cannot be nil"
	errDraftDataNil             = "draft data cannot be nil"
	errDraftIDEmpty             = "draft ID cannot be empty"
	errPlayerIDEmpty            = "player ID cannot be empty"
	errStoredWorldMismatch      = "stored draft world does not match requested world"
	errOwnershipImmutable       = "draft world and player ownership cannot change"
	errMappingPlayerMismatchFmt = "draft %q mapping resolves to a different player"
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

// draftKey is the world-scoped draft record key. World IDs are canonical
// decimal identifiers; draft IDs never contain the colon delimiter.
func draftKey(worldID, id string) string {
	return draftKeyPrefix + worldID + ":" + id
}

// playerMappingKey is the world-scoped single-draft mapping for a player.
func playerMappingKey(worldID, playerID string) string {
	return playerMappingPrefix + worldID + ":" + playerID
}

func (r *redisRepository) Create(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	if input.Draft == nil {
		return nil, apierr.InvalidArgument(errDraftNil)
	}
	if input.Draft.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
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
	// Check for existing draft for this player in this world
	playerKey := playerMappingKey(draft.WorldID, draft.Data.PlayerID)
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

	// Delete existing draft if any. The mapping is world-scoped, so this only
	// ever removes this player's draft in this world.
	if !isNew {
		pipe.Del(ctx, draftKey(draft.WorldID, existingDraftID))
	}

	data, err := json.Marshal(draft)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal draft")
	}

	// Set draft data
	pipe.Set(ctx, draftKey(draft.WorldID, draft.Data.ID), data, defaultTTL)

	// Set world-scoped player mapping (no TTL on this key)
	pipe.Set(ctx, playerKey, draft.Data.ID, 0)

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to create draft")
	}

	return &CreateOutput{Draft: draft}, nil
}

func (r *redisRepository) Get(ctx context.Context, input GetInput) (*GetOutput, error) {
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.ID == "" {
		return nil, apierr.InvalidArgument(errDraftIDEmpty)
	}

	key := draftKey(input.WorldID, input.ID)
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, apierr.NotFoundf("draft with ID %s not found in world %s", input.ID, input.WorldID)
		}
		return nil, apierr.Wrapf(err, "failed to get draft")
	}

	draft, err := decodeDraft([]byte(result), input.WorldID)
	if err != nil {
		return nil, err
	}

	return &GetOutput{Draft: draft}, nil
}

// decodeDraft validates a stored envelope before it is projected. A nil
// toolkit payload or ownership metadata contradicting the requested world is
// reported as storage corruption, never as a missing draft.
func decodeDraft(raw []byte, worldID string) (*entities.CharacterDraft, error) {
	var draft entities.CharacterDraft
	if err := json.Unmarshal(raw, &draft); err != nil {
		return nil, apierr.Wrapf(err, "failed to unmarshal draft")
	}
	if draft.Data == nil {
		return nil, apierr.Internal(errDraftDataNil)
	}
	if draft.WorldID != worldID {
		return nil, apierr.Internalf("%s: requested %q, stored %q", errStoredWorldMismatch, worldID, draft.WorldID)
	}
	return &draft, nil
}

func (r *redisRepository) GetByPlayerID(ctx context.Context, input GetByPlayerIDInput) (*GetByPlayerIDOutput, error) {
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.PlayerID == "" {
		return nil, apierr.InvalidArgument(errPlayerIDEmpty)
	}

	// Get draft ID from the world-scoped player mapping
	playerKey := playerMappingKey(input.WorldID, input.PlayerID)
	draftID, err := r.client.Get(ctx, playerKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, apierr.NotFoundf("no draft found for player %s in world %s", input.PlayerID, input.WorldID)
		}
		return nil, apierr.Wrapf(err, "failed to get player draft mapping")
	}

	// Get the actual draft
	getOutput, err := r.Get(ctx, GetInput{WorldID: input.WorldID, ID: draftID})
	if err != nil {
		// If draft doesn't exist, clean up the mapping
		if apierr.IsNotFound(err) {
			r.client.Del(ctx, playerKey)
		}
		return nil, err
	}

	// A mapping must resolve to a draft owned by the mapped player. A
	// contradictory record is corruption, never projected.
	if getOutput.Draft.Data.PlayerID != input.PlayerID {
		return nil, apierr.Internalf(errMappingPlayerMismatchFmt, draftID)
	}

	return &GetByPlayerIDOutput{Draft: getOutput.Draft}, nil
}

func (r *redisRepository) Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	if input.Draft == nil {
		return nil, apierr.InvalidArgument(errDraftNil)
	}
	if input.Draft.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.Draft.Data == nil {
		return nil, apierr.InvalidArgument(errDraftDataNil)
	}
	if input.Draft.Data.ID == "" {
		return nil, apierr.InvalidArgument(errDraftIDEmpty)
	}

	// Load the owned record first. This proves the draft exists in this world
	// and verifies the stored world metadata agrees with the key.
	existingOutput, err := r.Get(ctx, GetInput{WorldID: input.Draft.WorldID, ID: input.Draft.Data.ID})
	if err != nil {
		return nil, err
	}
	existing := existingOutput.Draft

	// World and player ownership are immutable: an update may replace toolkit
	// data but must never rehome a draft.
	if existing.WorldID != input.Draft.WorldID || existing.Data.PlayerID != input.Draft.Data.PlayerID {
		return nil, apierr.InvalidArgument(errOwnershipImmutable)
	}

	draft := input.Draft

	// Marshal draft
	data, err := json.Marshal(draft)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal draft")
	}

	// Update with TTL. Ownership cannot move, so the world-scoped player
	// mapping is deliberately not migrated.
	if err := r.client.Set(ctx, draftKey(draft.WorldID, draft.Data.ID), data, defaultTTL).Err(); err != nil {
		return nil, apierr.Wrapf(err, "failed to update draft")
	}

	return &UpdateOutput{Draft: draft}, nil
}

func (r *redisRepository) Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.ID == "" {
		return nil, apierr.InvalidArgument(errDraftIDEmpty)
	}

	// Get draft to find the player mapping; this also verifies stored world
	// ownership and prevents a corrupt envelope from driving a deletion.
	getOutput, err := r.Get(ctx, GetInput{WorldID: input.WorldID, ID: input.ID})
	if err != nil {
		return nil, err
	}

	pipe := r.client.TxPipeline()

	// Delete draft
	pipe.Del(ctx, draftKey(input.WorldID, input.ID))

	// Delete the world-scoped player mapping
	if getOutput.Draft.Data != nil && getOutput.Draft.Data.PlayerID != "" {
		pipe.Del(ctx, playerMappingKey(input.WorldID, getOutput.Draft.Data.PlayerID))
	}

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to delete draft")
	}

	return &DeleteOutput{}, nil
}
