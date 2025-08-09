package character

import (
	"context"
	"encoding/json"
	"log/slog"

	redis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

const (
	characterKeyPrefix     = "character:"
	playerIndexPrefix      = "character:player:"
	sessionIndexPrefix     = "character:session:"
	equipmentSlotsPrefix   = "character:equipment:"

	// Error messages
	errCharacterNil     = "character cannot be nil"
	errCharacterIDEmpty = "character ID cannot be empty"
	errPlayerIDEmpty    = "player ID cannot be empty"
	errSessionIDEmpty   = "session ID cannot be empty"
	errSlotEmpty        = "slot cannot be empty"
	errItemIDEmpty      = "item ID cannot be empty"
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
		return errors.InvalidArgument("config cannot be nil")
	}
	if cfg.Client == nil {
		return errors.InvalidArgument("client cannot be nil")
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
	if input.CharacterData == nil {
		return nil, errors.InvalidArgument(errCharacterNil)
	}
	if input.CharacterData.ID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}

	key := characterKeyPrefix + input.CharacterData.ID

	// Check if already exists
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check existence")
	}

	if exists > 0 {
		return nil, errors.AlreadyExistsf("character with ID %s already exists", input.CharacterData.ID)
	}

	// Marshal character data
	data, err := json.Marshal(input.CharacterData)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal character data")
	}

	// Start transaction
	pipe := r.client.TxPipeline()

	// Set character data
	pipe.Set(ctx, key, data, 0) // No TTL for characters

	// Add to player index
	if input.CharacterData.PlayerID != "" {
		playerKey := playerIndexPrefix + input.CharacterData.PlayerID
		pipe.SAdd(ctx, playerKey, input.CharacterData.ID)
	}

	// Note: character.Data doesn't have SessionID directly, that would need to be handled at orchestrator level

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create character")
	}

	return &CreateOutput{CharacterData: input.CharacterData}, nil
}

func (r *redisRepository) Get(ctx context.Context, input GetInput) (*GetOutput, error) {
	if input.ID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}

	key := characterKeyPrefix + input.ID
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, errors.NotFoundf("character with ID %s not found", input.ID)
		}
		return nil, errors.Wrapf(err, "failed to get character")
	}

	var charData toolkitchar.Data
	if err := json.Unmarshal([]byte(result), &charData); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal character data")
	}

	return &GetOutput{CharacterData: &charData}, nil
}

func (r *redisRepository) Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	if input.CharacterData == nil {
		return nil, errors.InvalidArgument(errCharacterNil)
	}
	if input.CharacterData.ID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}

	key := characterKeyPrefix + input.CharacterData.ID

	// Get existing character to check indexes
	existingOutput, err := r.Get(ctx, GetInput{ID: input.CharacterData.ID})
	if err != nil {
		return nil, err
	}
	existing := existingOutput.CharacterData

	// Marshal updated character data
	data, err := json.Marshal(input.CharacterData)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal character data")
	}

	// Start transaction
	pipe := r.client.TxPipeline()

	// Update character data
	pipe.Set(ctx, key, data, 0)

	// Update player index if changed
	if existing.PlayerID != input.CharacterData.PlayerID {
		if existing.PlayerID != "" {
			oldPlayerKey := playerIndexPrefix + existing.PlayerID
			pipe.SRem(ctx, oldPlayerKey, input.CharacterData.ID)
		}
		if input.CharacterData.PlayerID != "" {
			newPlayerKey := playerIndexPrefix + input.CharacterData.PlayerID
			pipe.SAdd(ctx, newPlayerKey, input.CharacterData.ID)
		}
	}

	// Note: Session management would need to be handled at orchestrator level
	// since character.Data doesn't include SessionID

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update character")
	}

	return &UpdateOutput{CharacterData: input.CharacterData}, nil
}

func (r *redisRepository) Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	if input.ID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}

	// Get character to find indexes
	getOutput, err := r.Get(ctx, GetInput(input))
	if err != nil {
		return nil, err
	}
	charData := getOutput.CharacterData

	// Start transaction
	pipe := r.client.TxPipeline()

	// Delete character
	key := characterKeyPrefix + input.ID
	pipe.Del(ctx, key)

	// Remove from player index
	if charData.PlayerID != "" {
		playerKey := playerIndexPrefix + charData.PlayerID
		pipe.SRem(ctx, playerKey, input.ID)
	}

	// Note: Session index cleanup would need to be handled at orchestrator level

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to delete character")
	}

	return &DeleteOutput{}, nil
}

func (r *redisRepository) ListByPlayerID(
	ctx context.Context,
	input ListByPlayerIDInput,
) (*ListByPlayerIDOutput, error) {
	if input.PlayerID == "" {
		return nil, errors.InvalidArgument(errPlayerIDEmpty)
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
		return nil, errors.InvalidArgument(errSessionIDEmpty)
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
func (r *redisRepository) listByIndex(ctx context.Context, indexKey string) ([]*toolkitchar.Data, error) {
	// Get character IDs from index
	slog.DebugContext(ctx, "fetching character IDs from index",
		"index_key", indexKey)

	characterIDs, err := r.client.SMembers(ctx, indexKey).Result()
	if err != nil {
		slog.ErrorContext(ctx, "failed to get character IDs from Redis",
			"index_key", indexKey,
			"error", err.Error())
		return nil, errors.Wrapf(err, "failed to get characters from index %s", indexKey)
	}

	slog.DebugContext(ctx, "found character IDs in index",
		"index_key", indexKey,
		"count", len(characterIDs),
		"character_ids", characterIDs)

	// Get all characters
	characters := make([]*toolkitchar.Data, 0, len(characterIDs))
	for _, id := range characterIDs {
		slog.DebugContext(ctx, "fetching character from Redis",
			"character_id", id)

		getOutput, err := r.Get(ctx, GetInput{ID: id})
		if err != nil {
			// If character doesn't exist, clean up the index
			if errors.IsNotFound(err) {
				slog.WarnContext(ctx, "character not found, cleaning up index",
					"character_id", id,
					"index_key", indexKey)
				r.client.SRem(ctx, indexKey, id)
				continue
			}
			slog.ErrorContext(ctx, "failed to get character from Redis",
				"character_id", id,
				"error", err.Error())
			return nil, errors.Wrapf(err, "failed to get character %s", id)
		}
		characters = append(characters, getOutput.CharacterData)
	}

	slog.DebugContext(ctx, "successfully retrieved all characters from index",
		"index_key", indexKey,
		"total_found", len(characters))

	return characters, nil
}

func (r *redisRepository) GetEquipmentSlots(ctx context.Context, input GetEquipmentSlotsInput) (*GetEquipmentSlotsOutput, error) {
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}

	// Check if character exists
	if _, err := r.Get(ctx, GetInput{ID: input.CharacterID}); err != nil {
		return nil, err
	}

	key := equipmentSlotsPrefix + input.CharacterID
	result, err := r.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get equipment slots")
	}
	
	slog.InfoContext(ctx, "Retrieved equipment slots from Redis",
		"key", key,
		"result", result,
		"armor", result["armor"])

	// Convert map to EquipmentSlots struct
	equipmentSlots := &EquipmentSlots{
		MainHand: result["main_hand"],
		OffHand:  result["off_hand"],
		Armor:    result["armor"],
		Shield:   result["shield"],
		Ring1:    result["ring1"],
		Ring2:    result["ring2"],
		Amulet:   result["amulet"],
		Boots:    result["boots"],
		Gloves:   result["gloves"],
		Helmet:   result["helmet"],
		Belt:     result["belt"],
		Cloak:    result["cloak"],
	}

	return &GetEquipmentSlotsOutput{EquipmentSlots: equipmentSlots}, nil
}

func (r *redisRepository) SetEquipmentSlot(ctx context.Context, input SetEquipmentSlotInput) (*SetEquipmentSlotOutput, error) {
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}
	if input.Slot == "" {
		return nil, errors.InvalidArgument(errSlotEmpty)
	}
	if input.ItemID == "" {
		return nil, errors.InvalidArgument(errItemIDEmpty)
	}

	// Check if character exists
	if _, err := r.Get(ctx, GetInput{ID: input.CharacterID}); err != nil {
		return nil, err
	}

	key := equipmentSlotsPrefix + input.CharacterID

	// Get previous item in the slot
	previousItem, err := r.client.HGet(ctx, key, input.Slot).Result()
	if err != nil && err != redis.Nil {
		return nil, errors.Wrapf(err, "failed to get previous item in slot")
	}

	// Set the new item in the slot
	err = r.client.HSet(ctx, key, input.Slot, input.ItemID).Err()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set equipment slot")
	}

	return &SetEquipmentSlotOutput{PreviousItemID: previousItem}, nil
}

func (r *redisRepository) ClearEquipmentSlot(ctx context.Context, input ClearEquipmentSlotInput) (*ClearEquipmentSlotOutput, error) {
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}
	if input.Slot == "" {
		return nil, errors.InvalidArgument(errSlotEmpty)
	}

	key := equipmentSlotsPrefix + input.CharacterID

	// Get current item in the slot
	currentItem, err := r.client.HGet(ctx, key, input.Slot).Result()
	if err != nil && err != redis.Nil {
		return nil, errors.Wrapf(err, "failed to get current item in slot")
	}

	// Clear the slot
	err = r.client.HDel(ctx, key, input.Slot).Err()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to clear equipment slot")
	}

	return &ClearEquipmentSlotOutput{ClearedItemID: currentItem}, nil
}
