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
	errWorldIDEmpty         = "world ID cannot be empty"
	errCharacterNil         = "character cannot be nil"
	errCharacterDataNil     = "character data cannot be nil"
	errCharacterIDEmpty     = "character ID cannot be empty"
	errPlayerIDEmpty        = "player ID cannot be empty"
	errSessionIDEmpty       = "session ID cannot be empty"
	errExpectedVersionEmpty = "expected character version cannot be empty"
	errEquipmentConflict    = "character equipment changed concurrently"
	errStoredWorldMismatch  = "stored character world does not match requested world"
	errOwnershipImmutable   = "character player ownership cannot change"
	errIndexPlayerMismatch  = "character index entry resolves to a different player"

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

// characterKey is the world-scoped record key. World IDs are canonical decimal
// identifiers; record IDs never contain the colon delimiter, so the
// (world, id) tuple is unambiguous.
func characterKey(worldID, id string) string {
	return characterKeyPrefix + worldID + ":" + id
}

// playerIndexKey is the world-scoped player membership set.
func playerIndexKey(worldID, playerID string) string {
	return playerIndexPrefix + worldID + ":" + playerID
}

// sessionIndexKey is the world-scoped session membership set.
func sessionIndexKey(worldID, sessionID string) string {
	return sessionIndexPrefix + worldID + ":" + sessionID
}

func (r *redisRepository) Create(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	if input.Character == nil {
		return nil, apierr.InvalidArgument(errCharacterNil)
	}
	if input.Character.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.Character.Data == nil {
		return nil, apierr.InvalidArgument(errCharacterDataNil)
	}
	if input.Character.Data.ID == "" {
		return nil, apierr.InvalidArgument(errCharacterIDEmpty)
	}

	key := characterKey(input.Character.WorldID, input.Character.Data.ID)

	// Check if already exists in this world
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to check existence")
	}

	if exists > 0 {
		return nil, apierr.AlreadyExistsf(
			"character with ID %s already exists in world %s",
			input.Character.Data.ID, input.Character.WorldID)
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

	// Add to world-scoped player index
	if input.Character.Data.PlayerID != "" {
		pipe.SAdd(ctx, playerIndexKey(input.Character.WorldID, input.Character.Data.PlayerID), input.Character.Data.ID)
	}

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to create character")
	}

	return &CreateOutput{Character: input.Character}, nil
}

func (r *redisRepository) Get(ctx context.Context, input GetInput) (*GetOutput, error) {
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.ID == "" {
		return nil, apierr.InvalidArgument(errCharacterIDEmpty)
	}

	key := characterKey(input.WorldID, input.ID)
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, apierr.NotFoundf("character with ID %s not found in world %s", input.ID, input.WorldID)
		}
		return nil, apierr.Wrapf(err, "failed to get character")
	}

	char, err := decodeCharacter([]byte(result), input.WorldID)
	if err != nil {
		return nil, err
	}

	return &GetOutput{
		Character: char,
		Version:   characterVersion([]byte(result)),
	}, nil
}

// decodeCharacter validates a stored envelope before it is projected. A nil
// toolkit payload or ownership metadata that contradicts the requested world
// is reported as storage corruption, never as an empty success.
func decodeCharacter(raw []byte, worldID string) (*entities.Character, error) {
	var char entities.Character
	if err := json.Unmarshal(raw, &char); err != nil {
		return nil, apierr.Wrapf(err, "failed to unmarshal character")
	}
	if char.Data == nil {
		return nil, apierr.Internal(errCharacterDataNil)
	}
	if char.WorldID != worldID {
		return nil, apierr.Internalf("%s: requested %q, stored %q", errStoredWorldMismatch, worldID, char.WorldID)
	}
	return &char, nil
}

func (r *redisRepository) Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	if input.Character == nil {
		return nil, apierr.InvalidArgument(errCharacterNil)
	}
	if input.Character.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.Character.Data == nil {
		return nil, apierr.InvalidArgument(errCharacterDataNil)
	}
	if input.Character.Data.ID == "" {
		return nil, apierr.InvalidArgument(errCharacterIDEmpty)
	}

	// Load the owned record first. This both proves the record exists in this
	// world and verifies the stored world metadata agrees with the key.
	existingOutput, err := r.Get(ctx, GetInput{WorldID: input.Character.WorldID, ID: input.Character.Data.ID})
	if err != nil {
		return nil, err
	}
	existing := existingOutput.Character

	// Player ownership is immutable: an update may replace toolkit data but
	// must never rehome a record to another player.
	if existing.Data.PlayerID != input.Character.Data.PlayerID {
		return nil, apierr.InvalidArgument(errOwnershipImmutable)
	}

	key := characterKey(input.Character.WorldID, input.Character.Data.ID)

	// Marshal updated character (includes appearance)
	data, err := json.Marshal(input.Character)
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to marshal character")
	}

	// Start transaction. Ownership cannot move, so the world-scoped player
	// index is already correct and is deliberately not rewritten.
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, key, data, 0)

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
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.CharacterID == "" {
		return nil, apierr.InvalidArgument(errCharacterIDEmpty)
	}
	if input.ExpectedVersion == "" {
		return nil, apierr.InvalidArgument(errExpectedVersionEmpty)
	}

	key := characterKey(input.WorldID, input.CharacterID)
	for range maxEquipmentPatchWatchAttempts {
		var output *PatchEquipmentOutput
		err := r.client.Watch(ctx, func(tx *redis.Tx) error {
			stored, getErr := tx.Get(ctx, key).Bytes()
			if getErr != nil {
				if errors.Is(getErr, redis.Nil) {
					return apierr.NotFoundf("character with ID %s not found in world %s", input.CharacterID, input.WorldID)
				}
				return apierr.Wrapf(getErr, "failed to get character for equipment patch")
			}

			current, decodeErr := decodeCharacter(stored, input.WorldID)
			if decodeErr != nil {
				return decodeErr
			}

			if !maps.Equal(current.Data.EquipmentSlots, input.ExpectedEquipmentSlots) {
				return apierr.Aborted(errEquipmentConflict)
			}

			version := characterVersion(stored)
			if version != input.ExpectedVersion {
				output = &PatchEquipmentOutput{
					Character: current,
					Version:   version,
					Applied:   false,
				}
				return nil
			}

			current.Data.EquipmentSlots = maps.Clone(input.EquipmentSlots)
			current.Data.ArmorClass = input.ArmorClass
			if input.Conditions != nil {
				current.Data.Conditions = make([]json.RawMessage, len(*input.Conditions))
				for i, condition := range *input.Conditions {
					current.Data.Conditions[i] = append(json.RawMessage(nil), condition...)
				}
			}
			patched, marshalErr := json.Marshal(current)
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
				Character: current,
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
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.ID == "" {
		return nil, apierr.InvalidArgument(errCharacterIDEmpty)
	}

	// Get character to find indexes; this also verifies the stored world and
	// prevents a corrupt envelope from driving a deletion.
	getOutput, err := r.Get(ctx, GetInput{WorldID: input.WorldID, ID: input.ID})
	if err != nil {
		return nil, err
	}
	char := getOutput.Character

	// Start transaction
	pipe := r.client.TxPipeline()

	// Delete character
	pipe.Del(ctx, characterKey(input.WorldID, input.ID))

	// Remove from the world-scoped player index
	if char.Data != nil && char.Data.PlayerID != "" {
		pipe.SRem(ctx, playerIndexKey(input.WorldID, char.Data.PlayerID), input.ID)
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
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.PlayerID == "" {
		return nil, apierr.InvalidArgument(errPlayerIDEmpty)
	}

	indexKey := playerIndexKey(input.WorldID, input.PlayerID)
	slog.DebugContext(ctx, "listing characters by world-scoped player index",
		"world_id", input.WorldID,
		"player_id", input.PlayerID,
		"index_key", indexKey)

	characters, err := r.listByIndex(ctx, indexKey, input.WorldID, func(char *entities.Character) error {
		if char.Data.PlayerID != input.PlayerID {
			return apierr.Internalf("%s: player index %q resolved character %q owned by %q",
				errIndexPlayerMismatch, input.PlayerID, char.Data.ID, char.Data.PlayerID)
		}
		return nil
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to list characters by player index",
			"world_id", input.WorldID,
			"player_id", input.PlayerID,
			"index_key", indexKey,
			"error", err.Error())
		return nil, err
	}

	slog.DebugContext(ctx, "successfully listed characters by player",
		"world_id", input.WorldID,
		"player_id", input.PlayerID,
		"count", len(characters))

	return &ListByPlayerIDOutput{Characters: characters}, nil
}

func (r *redisRepository) ListBySessionID(
	ctx context.Context,
	input ListBySessionIDInput,
) (*ListBySessionIDOutput, error) {
	if input.WorldID == "" {
		return nil, apierr.InvalidArgument(errWorldIDEmpty)
	}
	if input.SessionID == "" {
		return nil, apierr.InvalidArgument(errSessionIDEmpty)
	}

	indexKey := sessionIndexKey(input.WorldID, input.SessionID)
	slog.DebugContext(ctx, "listing characters by world-scoped session index",
		"world_id", input.WorldID,
		"session_id", input.SessionID,
		"index_key", indexKey)

	// Toolkit character.Data carries no session field, so the world-scoped
	// index key plus the record's stored world is the verifiable association.
	characters, err := r.listByIndex(ctx, indexKey, input.WorldID, nil)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list characters by session index",
			"world_id", input.WorldID,
			"session_id", input.SessionID,
			"index_key", indexKey,
			"error", err.Error())
		return nil, err
	}

	slog.DebugContext(ctx, "successfully listed characters by session",
		"world_id", input.WorldID,
		"session_id", input.SessionID,
		"count", len(characters))

	return &ListBySessionIDOutput{Characters: characters}, nil
}

// listByIndex resolves a world-scoped index set to owned records. Missing
// records are stale local members and are lazily cleaned. A resolved record
// whose stored ownership contradicts the index (different world via Get, or a
// different player via check) is reported as corruption instead of projected,
// and the poisoning member is left in place rather than deleting a foreign
// index.
func (r *redisRepository) listByIndex(
	ctx context.Context,
	indexKey string,
	worldID string,
	check func(*entities.Character) error,
) ([]*entities.Character, error) {
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

		getOutput, err := r.Get(ctx, GetInput{WorldID: worldID, ID: id})
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
		if check != nil {
			if err := check(getOutput.Character); err != nil {
				return nil, err
			}
		}
		characters = append(characters, getOutput.Character)
	}

	slog.DebugContext(ctx, "successfully retrieved all characters from index",
		"index_key", indexKey,
		"total_found", len(characters))

	return characters, nil
}
