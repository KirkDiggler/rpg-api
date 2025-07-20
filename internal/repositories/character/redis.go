package character

import (
	"context"
	"encoding/json"
	"log/slog"

	redis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

const (
	characterKeyPrefix = "character:"
	playerIndexPrefix  = "character:player:"
	sessionIndexPrefix = "character:session:"

	// Error messages
	errCharacterNil     = "character cannot be nil"
	errCharacterIDEmpty = "character ID cannot be empty"
	errPlayerIDEmpty    = "player ID cannot be empty"
	errSessionIDEmpty   = "session ID cannot be empty"
)

type redisRepository struct {
	client redisclient.Client
}

// RedisConfig contains configuration for the Redis character repository.
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

// NewRedis creates a new Redis-backed character repository
func NewRedis(cfg *RedisConfig) (Repository, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &redisRepository{
		client: cfg.Client,
	}, nil
}

func (r *redisRepository) Create(ctx context.Context, input CreateInput) (*CreateOutput, error) {
	if input.Character == nil {
		return nil, errors.InvalidArgument(errCharacterNil)
	}
	if input.Character.ID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}

	key := characterKeyPrefix + input.Character.ID

	// Check if already exists
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check existence")
	}

	if exists > 0 {
		return nil, errors.AlreadyExistsf("character with ID %s already exists", input.Character.ID)
	}

	// Marshal character
	data, err := json.Marshal(input.Character)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal character")
	}

	// Start transaction
	pipe := r.client.TxPipeline()

	// Set character data
	pipe.Set(ctx, key, data, 0) // No TTL for characters

	// Add to player index
	if input.Character.PlayerID != "" {
		playerKey := playerIndexPrefix + input.Character.PlayerID
		pipe.SAdd(ctx, playerKey, input.Character.ID)
	}

	// Add to session index
	if input.Character.SessionID != "" {
		sessionKey := sessionIndexPrefix + input.Character.SessionID
		pipe.SAdd(ctx, sessionKey, input.Character.ID)
	}

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create character")
	}

	return &CreateOutput{}, nil
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

	var character dnd5e.Character
	if err := json.Unmarshal([]byte(result), &character); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal character")
	}

	return &GetOutput{Character: &character}, nil
}

func (r *redisRepository) Update(ctx context.Context, input UpdateInput) (*UpdateOutput, error) {
	if input.Character == nil {
		return nil, errors.InvalidArgument(errCharacterNil)
	}
	if input.Character.ID == "" {
		return nil, errors.InvalidArgument(errCharacterIDEmpty)
	}

	key := characterKeyPrefix + input.Character.ID

	// Get existing character to check indexes
	existingOutput, err := r.Get(ctx, GetInput{ID: input.Character.ID})
	if err != nil {
		return nil, err
	}
	existing := existingOutput.Character

	// Marshal updated character
	data, err := json.Marshal(input.Character)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal character")
	}

	// Start transaction
	pipe := r.client.TxPipeline()

	// Update character data
	pipe.Set(ctx, key, data, 0)

	// Update player index if changed
	if existing.PlayerID != input.Character.PlayerID {
		if existing.PlayerID != "" {
			oldPlayerKey := playerIndexPrefix + existing.PlayerID
			pipe.SRem(ctx, oldPlayerKey, input.Character.ID)
		}
		if input.Character.PlayerID != "" {
			newPlayerKey := playerIndexPrefix + input.Character.PlayerID
			pipe.SAdd(ctx, newPlayerKey, input.Character.ID)
		}
	}

	// Update session index if changed
	if existing.SessionID != input.Character.SessionID {
		if existing.SessionID != "" {
			oldSessionKey := sessionIndexPrefix + existing.SessionID
			pipe.SRem(ctx, oldSessionKey, input.Character.ID)
		}
		if input.Character.SessionID != "" {
			newSessionKey := sessionIndexPrefix + input.Character.SessionID
			pipe.SAdd(ctx, newSessionKey, input.Character.ID)
		}
	}

	// Execute transaction
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update character")
	}

	return &UpdateOutput{}, nil
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
	character := getOutput.Character

	// Start transaction
	pipe := r.client.TxPipeline()

	// Delete character
	key := characterKeyPrefix + input.ID
	pipe.Del(ctx, key)

	// Remove from player index
	if character.PlayerID != "" {
		playerKey := playerIndexPrefix + character.PlayerID
		pipe.SRem(ctx, playerKey, input.ID)
	}

	// Remove from session index
	if character.SessionID != "" {
		sessionKey := sessionIndexPrefix + character.SessionID
		pipe.SRem(ctx, sessionKey, input.ID)
	}

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
func (r *redisRepository) listByIndex(ctx context.Context, indexKey string) ([]*dnd5e.Character, error) {
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
	characters := make([]*dnd5e.Character, 0, len(characterIDs))
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
		characters = append(characters, getOutput.Character)
	}

	slog.DebugContext(ctx, "successfully retrieved all characters from index",
		"index_key", indexKey,
		"total_found", len(characters))

	return characters, nil
}
