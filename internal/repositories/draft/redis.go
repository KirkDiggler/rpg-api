package draft

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// RedisRepository implements the Repository interface using Redis as storage
type RedisRepository struct {
	client redis.UniversalClient
	prefix string
}

// RedisConfig holds configuration for the Redis repository
type RedisConfig struct {
	Client redis.UniversalClient
	Prefix string // Key prefix for namespacing
}

// NewRedisRepository creates a new Redis-backed draft repository
func NewRedisRepository(config RedisConfig) *RedisRepository {
	prefix := config.Prefix
	if prefix == "" {
		prefix = "draft"
	}
	
	return &RedisRepository{
		client: config.Client,
		prefix: prefix,
	}
}

// Save creates or updates a draft
func (r *RedisRepository) Save(ctx context.Context, draft *character.Draft) error {
	if draft == nil {
		return fmt.Errorf("draft is required")
	}
	
	// Convert to data for serialization
	data := draft.ToData()
	if data == nil {
		return fmt.Errorf("failed to convert draft to data")
	}
	
	// Serialize to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal draft: %w", err)
	}
	
	// Store in Redis
	key := r.key(draft.ID())
	if err := r.client.Set(ctx, key, jsonData, 0).Err(); err != nil {
		return fmt.Errorf("failed to save draft: %w", err)
	}
	
	// Add to player's draft list
	playerKey := r.playerKey(draft.PlayerID())
	if err := r.client.SAdd(ctx, playerKey, draft.ID()).Err(); err != nil {
		return fmt.Errorf("failed to add draft to player list: %w", err)
	}
	
	return nil
}

// Get retrieves a draft by ID
func (r *RedisRepository) Get(ctx context.Context, id string) (*character.Draft, error) {
	key := r.key(id)
	
	// Get from Redis
	jsonData, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("draft not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}
	
	// Deserialize from JSON
	var data character.DraftData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal draft: %w", err)
	}
	
	// Convert from data
	draft := character.LoadDraftFromData(&data)
	if draft == nil {
		return nil, fmt.Errorf("failed to load draft from data")
	}
	
	return draft, nil
}

// Delete removes a draft
func (r *RedisRepository) Delete(ctx context.Context, id string) error {
	// Get the draft first to find the player ID
	draft, err := r.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get draft for deletion: %w", err)
	}
	
	// Remove from Redis
	key := r.key(id)
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete draft: %w", err)
	}
	
	// Remove from player's draft list
	playerKey := r.playerKey(draft.PlayerID())
	if err := r.client.SRem(ctx, playerKey, id).Err(); err != nil {
		return fmt.Errorf("failed to remove draft from player list: %w", err)
	}
	
	return nil
}

// List retrieves drafts for a player
func (r *RedisRepository) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	
	if input.PlayerID == "" {
		return nil, fmt.Errorf("player ID is required")
	}
	
	// Get all draft IDs for the player
	playerKey := r.playerKey(input.PlayerID)
	draftIDs, err := r.client.SMembers(ctx, playerKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list draft IDs: %w", err)
	}
	
	// Retrieve all drafts
	drafts := make([]*character.Draft, 0, len(draftIDs))
	for _, id := range draftIDs {
		draft, err := r.Get(ctx, id)
		if err != nil {
			// Log error but continue - draft might have been deleted
			continue
		}
		drafts = append(drafts, draft)
	}
	
	// Apply pagination
	total := len(drafts)
	start := input.Offset
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	
	end := start + input.Limit
	if input.Limit <= 0 || end > total {
		end = total
	}
	
	return &ListOutput{
		Drafts: drafts[start:end],
		Total:  total,
	}, nil
}

// key returns the Redis key for a draft
func (r *RedisRepository) key(id string) string {
	return fmt.Sprintf("%s:%s", r.prefix, id)
}

// playerKey returns the Redis key for a player's draft list
func (r *RedisRepository) playerKey(playerID string) string {
	return fmt.Sprintf("%s:player:%s", r.prefix, playerID)
}

// Helper function to check if the repository is healthy
func (r *RedisRepository) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Helper function to clear all drafts (useful for testing)
func (r *RedisRepository) Clear(ctx context.Context) error {
	// Get all keys with our prefix
	pattern := fmt.Sprintf("%s:*", r.prefix)
	iter := r.client.Scan(ctx, 0, pattern, 0).Iterator()
	
	keys := []string{}
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	
	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to scan keys: %w", err)
	}
	
	if len(keys) == 0 {
		return nil
	}
	
	// Delete all keys
	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("failed to delete keys: %w", err)
	}
	
	return nil
}