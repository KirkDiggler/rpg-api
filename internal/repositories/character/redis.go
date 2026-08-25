package character

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"

	redis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

const (
	characterKeyPrefix = "character:"
	playerIndexPrefix  = "character:player:"
	sessionIndexPrefix = "character:session:"

	// Error messages
	errCharacterNil         = "character cannot be nil"
	errCharacterDataNil     = "character data cannot be nil"
	errCharacterIDEmpty     = "character ID cannot be empty"
	errPlayerIDEmpty        = "player ID cannot be empty"
	errSessionIDEmpty       = "session ID cannot be empty"
	errExpectedVersionEmpty = "expected character version cannot be empty"
	errEquipmentConflict    = "character equipment changed concurrently"

	maxEquipmentPatchWatchAttempts = 8
)

type redisRepository struct {
	client redisclient.Client
	clock  clock.Clock
}

// RedisConfig contains configuration for the Redis character repository.
type RedisConfig struct {
	Client redisclient.Client
	Clock  clock.Clock
}

// Validate validates the RedisConfig.
func (cfg *RedisConfig) Validate() error {
	if cfg == nil {
		return apierr.InvalidArgument("config cannot be nil")
	}
	if cfg.Client == nil {
		return apierr.InvalidArgument("client cannot be nil")
	}
	return nil
}

// NewRedis creates a new Redis-backed character repository
func NewRedis(cfg *RedisConfig) (Repository, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Use real clock if none provided
	c := cfg.Clock
	if c == nil {
		c = clock.New()
	}

	return &redisRepository{
		client: cfg.Client,
		clock:  c,
	}, nil
}

func (r *redisRepository) Create(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	if input.Character == nil {
		return nil, apierr.InvalidArgument(errCharacterNil)
	}
	if input.Character.Data == nil {
		return nil, apierr.InvalidArgument(errCharacterDataNil)
	}
	if input.Character.Data.ID == "" {
		return nil, apierr.InvalidArgument(errCharacterIDEmpty)
	}

	key := characterKeyPrefix + input.Character.Data.ID

	// Check if already exists
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to check existence")
	}

	if exists > 0 {
		return nil, apierr.AlreadyExistsf("character with ID %s already exists", input.Character.Data.ID)
	}

	// Marshal character (includes appearance)
	data, err := json.Marshal(input.Character)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal character")
	}

	// Start transaction
	pipe := r.client.TxPipeline()

	// Set character data
	pipe.Set(ctx, key, data, 0) // No TTL for characters

	// Add to player index
	if input.Character.Data.PlayerID != "" {
		playerKey := playerIndexPrefix + input.Character.Data.PlayerID
		pipe.SAdd(ctx, playerKey, input.Character.Data.ID)
	}

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to create character")
	}

	return &CreateOutput{Character: input.Character}, nil
}

func (r *redisRepository) Get(ctx context.Context, input GetInput) (*GetOutput, error) {
	if input.ID == "" {
		return nil, apierr.InvalidArgument(errCharacterIDEmpty)
	}

	key := characterKeyPrefix + input.ID
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, apierr.NotFoundf("character with ID %s not found", input.ID)
		}
		return nil, apierr.Wrapf(err, "failed to get character")
	}

	var char entities.Character
	if unmarshalErr := json.Unmarshal([]byte(result), &char); unmarshalErr != nil {
		return nil, apierr.Wrapf(unmarshalErr, "failed to unmarshal character")
	}

	return &GetOutput{
		Character: &char,
		Version:   characterVersion([]byte(result)),
	}, nil
}

func (r *redisRepository) Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	if input.Character == nil {
		return nil, apierr.InvalidArgument(errCharacterNil)
	}
	if input.Character.Data == nil {
		return nil, apierr.InvalidArgument(errCharacterDataNil)
	}
	if input.Character.Data.ID == "" {
		return nil, apierr.InvalidArgument(errCharacterIDEmpty)
	}

	key := characterKeyPrefix + input.Character.Data.ID

	// Get existing character to check indexes
	existingOutput, err := r.Get(ctx, GetInput{ID: input.Character.Data.ID})
	if err != nil {
		return nil, err
	}
	existing := existingOutput.Character

	// Marshal updated character (includes appearance)
	data, err := json.Marshal(input.Character)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal character")
	}

	// Start transaction
	pipe := r.client.TxPipeline()

	// Update character data
	pipe.Set(ctx, key, data, 0)

	// Update player index if changed
	if existing.Data.PlayerID != input.Character.Data.PlayerID {
		if existing.Data.PlayerID != "" {
			oldPlayerKey := playerIndexPrefix + existing.Data.PlayerID
			pipe.SRem(ctx, oldPlayerKey, input.Character.Data.ID)
		}
		if input.Character.Data.PlayerID != "" {
			newPlayerKey := playerIndexPrefix + input.Character.Data.PlayerID
			pipe.SAdd(ctx, newPlayerKey, input.Character.Data.ID)
		}
	}

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to update character")
	}

	return &UpdateOutput{Character: input.Character}, nil
}

func (r *redisRepository) PatchEquipment(
	ctx context.Context,
	input PatchEquipmentInput,
) (*PatchEquipmentOutput, error) {
	if input.CharacterID == "" {
		return nil, apierr.InvalidArgument(errCharacterIDEmpty)
	}
	if input.ExpectedVersion == "" {
		return nil, apierr.InvalidArgument(errExpectedVersionEmpty)
	}

	key := characterKeyPrefix + input.CharacterID
	for range maxEquipmentPatchWatchAttempts {
		var output *PatchEquipmentOutput
		err := r.client.Watch(ctx, func(tx *redis.Tx) error {
			stored, getErr := tx.Get(ctx, key).Bytes()
			if getErr != nil {
				if errors.Is(getErr, redis.Nil) {
					return apierr.NotFoundf("character with ID %s not found", input.CharacterID)
				}
				return apierr.Wrapf(getErr, "failed to get character for equipment patch")
			}

			var current entities.Character
			if unmarshalErr := json.Unmarshal(stored, &current); unmarshalErr != nil {
				return apierr.Wrapf(unmarshalErr, "failed to unmarshal character for equipment patch")
			}
			if current.Data == nil {
				return apierr.Internal(errCharacterDataNil)
			}

			if !maps.Equal(current.Data.EquipmentSlots, input.ExpectedEquipmentSlots) {
				return apierr.Aborted(errEquipmentConflict)
			}

			version := characterVersion(stored)
			if version != input.ExpectedVersion {
				output = &PatchEquipmentOutput{
					Character: &current,
					Version:   version,
					Applied:   false,
				}
				return nil
			}

			current.Data.EquipmentSlots = maps.Clone(input.EquipmentSlots)
			current.Data.ArmorClass = input.ArmorClass
			patched, marshalErr := json.Marshal(&current)
			if marshalErr != nil {
				return apierr.Wrapf(marshalErr, "failed to marshal character equipment patch")
			}

			if _, txErr := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, patched, 0)
				return nil
			}); txErr != nil {
				return txErr
			}

			output = &PatchEquipmentOutput{
				Character: &current,
				Version:   characterVersion(patched),
				Applied:   true,
			}
			return nil
		}, key)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if output == nil {
			return nil, apierr.Internal("equipment patch returned no result")
		}
		return output, nil
	}

	return nil, apierr.Aborted(errEquipmentConflict)
}

func characterVersion(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (r *redisRepository) Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	if input.ID == "" {
		return nil, apierr.InvalidArgument(errCharacterIDEmpty)
	}

	// Get character to find indexes
	getOutput, err := r.Get(ctx, GetInput(input))
	if err != nil {
		return nil, err
	}
	char := getOutput.Character

	// Start transaction
	pipe := r.client.TxPipeline()

	// Delete character
	key := characterKeyPrefix + input.ID
	pipe.Del(ctx, key)

	// Remove from player index
	if char.Data != nil && char.Data.PlayerID != "" {
		playerKey := playerIndexPrefix + char.Data.PlayerID
		pipe.SRem(ctx, playerKey, input.ID)
	}

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to delete character")
	}

	return &DeleteOutput{}, nil
}

func (r *redisRepository) ListByPlayerID(
	ctx context.Context,
	input ListByPlayerIDInput,
) (*ListByPlayerIDOutput, error) {
	if input.PlayerID == "" {
		return nil, apierr.InvalidArgument(errPlayerIDEmpty)
	}

	indexKey := playerIndexPrefix + input.PlayerID
	slog.DebugContext(ctx, "listing characters by player index",
		"player_id", input.PlayerID,
		"index_key", indexKey)

	characters, err := r.listByIndex(ctx, indexKey)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list characters by player index",
			"player_id", input.PlayerID,
			"index_key", indexKey,
			"error", err.Error())
		return nil, err
	}

	slog.DebugContext(ctx, "successfully listed characters by player",
		"player_id", input.PlayerID,
		"count", len(characters))

	return &ListByPlayerIDOutput{Characters: characters}, nil
}

func (r *redisRepository) ListBySessionID(
	ctx context.Context,
	input ListBySessionIDInput,
) (*ListBySessionIDOutput, error) {
	if input.SessionID == "" {
		return nil, apierr.InvalidArgument(errSessionIDEmpty)
	}

	indexKey := sessionIndexPrefix + input.SessionID
	slog.DebugContext(ctx, "listing characters by session index",
		"session_id", input.SessionID,
		"index_key", indexKey)

	characters, err := r.listByIndex(ctx, indexKey)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list characters by session index",
			"session_id", input.SessionID,
			"index_key", indexKey,
			"error", err.Error())
		return nil, err
	}

	slog.DebugContext(ctx, "successfully listed characters by session",
		"session_id", input.SessionID,
		"count", len(characters))

	return &ListBySessionIDOutput{Characters: characters}, nil
}

// listByIndex is a helper function to list characters by any index
func (r *redisRepository) listByIndex(ctx context.Context, indexKey string) ([]*entities.Character, error) {
	// Get character IDs from index
	slog.DebugContext(ctx, "fetching character IDs from index",
		"index_key", indexKey)

	characterIDs, err := r.client.SMembers(ctx, indexKey).Result()
	if err != nil {
		slog.ErrorContext(ctx, "failed to get character IDs from Redis",
			"index_key", indexKey,
			"error", err.Error())
		return nil, apierr.Wrapf(err, "failed to get characters from index %s", indexKey)
	}

	slog.DebugContext(ctx, "found character IDs in index",
		"index_key", indexKey,
		"count", len(characterIDs),
		"character_ids", characterIDs)

	// Get all characters
	characters := make([]*entities.Character, 0, len(characterIDs))
	for _, id := range characterIDs {
		slog.DebugContext(ctx, "fetching character from Redis",
			"character_id", id)

		getOutput, err := r.Get(ctx, GetInput{ID: id})
		if err != nil {
			// If character doesn't exist, clean up the index
			if apierr.IsNotFound(err) {
				slog.WarnContext(ctx, "character not found, cleaning up index",
					"character_id", id,
					"index_key", indexKey)
				r.client.SRem(ctx, indexKey, id)
				continue
			}
			slog.ErrorContext(ctx, "failed to get character from Redis",
				"character_id", id,
				"error", err.Error())
			return nil, apierr.Wrapf(err, "failed to get character %s", id)
		}
		characters = append(characters, getOutput.Character)
	}

	slog.DebugContext(ctx, "successfully retrieved all characters from index",
		"index_key", indexKey,
		"total_found", len(characters))

	return characters, nil
}
