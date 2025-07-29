package equipment

import (
	"context"
	"encoding/json"
	"fmt"

	redis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

const (
	equipmentKeyPrefix = "equipment:character:"

	// Error messages
	errCharacterIDEmpty = "character ID cannot be empty"
)

type redisRepository struct {
	client redisclient.Client
}

// RedisConfig contains configuration for the Redis equipment repository.
type RedisConfig struct {
	Client redisclient.Client
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

// NewRedis creates a new Redis-backed equipment repository
func NewRedis(cfg *RedisConfig) (Repository, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &redisRepository{
		client: cfg.Client,
	}, nil
}

// equipmentData is the storage structure for equipment
// This is what gets serialized to Redis
type equipmentData struct {
	CharacterID    string                 `json:"character_id"`
	EquipmentSlots *dnd5e.EquipmentSlots  `json:"equipment_slots,omitempty"`
	Inventory      []dnd5e.InventoryItem  `json:"inventory,omitempty"`
	Encumbrance    *dnd5e.EncumbranceInfo `json:"encumbrance,omitempty"`
}

func (r *redisRepository) Get(ctx context.Context, input GetInput) (*GetOutput, error) {
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}

	key := equipmentKeyPrefix + input.CharacterID
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, errors.NotFoundf("equipment for character %s not found", input.CharacterID)
		}
		return nil, errors.Wrapf(err, "failed to get equipment for character %s", input.CharacterID)
	}

	var data equipmentData
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal equipment data")
	}

	return &GetOutput{
		CharacterID:    data.CharacterID,
		EquipmentSlots: data.EquipmentSlots,
		Inventory:      data.Inventory,
		Encumbrance:    data.Encumbrance,
	}, nil
}

func (r *redisRepository) Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}

	data := equipmentData{
		CharacterID:    input.CharacterID,
		EquipmentSlots: input.EquipmentSlots,
		Inventory:      input.Inventory,
		Encumbrance:    input.Encumbrance,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal equipment data")
	}

	key := equipmentKeyPrefix + input.CharacterID
	if err := r.client.Set(ctx, key, jsonData, 0).Err(); err != nil {
		return nil, errors.Wrapf(err, "failed to update equipment for character %s", input.CharacterID)
	}

	return &UpdateOutput{
		CharacterID:    input.CharacterID,
		EquipmentSlots: input.EquipmentSlots,
		Inventory:      input.Inventory,
		Encumbrance:    input.Encumbrance,
	}, nil
}

func (r *redisRepository) Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error) {
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}

	key := equipmentKeyPrefix + input.CharacterID

	// Check if exists first to return proper error
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check equipment existence")
	}

	if exists == 0 {
		return nil, errors.NotFoundf("equipment for character %s not found", input.CharacterID)
	}

	// Delete the equipment data
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return nil, errors.Wrapf(err, "failed to delete equipment for character %s", input.CharacterID)
	}

	return &DeleteOutput{}, nil
}

// GetKey returns the Redis key for a character's equipment
// Exposed for testing purposes
func GetKey(characterID string) string {
	return fmt.Sprintf("%s%s", equipmentKeyPrefix, characterID)
}
