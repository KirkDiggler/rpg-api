// Package dungeons provides repository interfaces and implementations for dungeon data storage.
package dungeons

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// InMemoryRepository implements Repository using in-memory storage
type InMemoryRepository struct {
	mu               sync.RWMutex
	store            map[string]*entities.Dungeon // dungeonID -> Dungeon
	encounterIDIndex map[string]string            // encounterID -> dungeonID
}

// NewInMemory creates a new in-memory repository
func NewInMemory() *InMemoryRepository {
	return &InMemoryRepository{
		store:            make(map[string]*entities.Dungeon),
		encounterIDIndex: make(map[string]string),
	}
}

// Save stores a dungeon. This is an upsert operation - existing dungeons will be overwritten.
func (r *InMemoryRepository) Save(_ context.Context, input *SaveInput) (*SaveOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	if input.Dungeon == nil {
		return nil, apierr.InvalidArgument("dungeon is required")
	}

	if input.Dungeon.ID == "" {
		return nil, apierr.InvalidArgument("dungeon ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Store the dungeon (deep copy to prevent external mutation)
	copied, err := copyDungeon(input.Dungeon)
	if err != nil {
		return nil, apierr.Internal("failed to copy dungeon: " + err.Error())
	}
	r.store[input.Dungeon.ID] = copied

	// Update encounter ID index if set
	if input.Dungeon.EncounterID != "" {
		r.encounterIDIndex[input.Dungeon.EncounterID] = input.Dungeon.ID
	}

	return &SaveOutput{Success: true}, nil
}

// Get retrieves a dungeon by ID
func (r *InMemoryRepository) Get(_ context.Context, input *GetInput) (*GetOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	if input.DungeonID == "" {
		return nil, apierr.InvalidArgument("dungeon ID is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	dungeon, exists := r.store[input.DungeonID]
	if !exists {
		return nil, apierr.NotFound("dungeon not found")
	}

	// Return a copy to prevent external modification
	copied, err := copyDungeon(dungeon)
	if err != nil {
		return nil, apierr.Internal("failed to copy dungeon: " + err.Error())
	}

	return &GetOutput{
		Dungeon: copied,
	}, nil
}

// GetByEncounterID retrieves a dungeon by its associated encounter ID
func (r *InMemoryRepository) GetByEncounterID(_ context.Context, input *GetByEncounterIDInput) (*GetOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	if input.EncounterID == "" {
		return nil, apierr.InvalidArgument("encounter ID is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Use the index for O(1) lookup
	dungeonID, exists := r.encounterIDIndex[input.EncounterID]
	if !exists {
		return nil, apierr.NotFound("dungeon not found")
	}

	dungeon, exists := r.store[dungeonID]
	if !exists {
		return nil, apierr.NotFound("dungeon not found")
	}

	// Return a copy to prevent external modification
	copied, err := copyDungeon(dungeon)
	if err != nil {
		return nil, apierr.Internal("failed to copy dungeon: " + err.Error())
	}

	return &GetOutput{
		Dungeon: copied,
	}, nil
}

// Update modifies an existing dungeon
func (r *InMemoryRepository) Update(_ context.Context, input *UpdateInput) (*UpdateOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	if input.DungeonID == "" {
		return nil, apierr.InvalidArgument("dungeon ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	dungeon, exists := r.store[input.DungeonID]
	if !exists {
		return nil, apierr.NotFound("dungeon not found")
	}

	// Update only what's provided
	if input.State != nil {
		dungeon.State = *input.State
	}

	if input.CurrentRoomID != nil {
		dungeon.CurrentRoomID = *input.CurrentRoomID
	}

	// Merge revealed rooms - only adds true values since rooms cannot be "unexplored"
	if input.RevealedRooms != nil {
		if dungeon.RevealedRooms == nil {
			dungeon.RevealedRooms = make(map[string]bool)
		}
		for roomID, revealed := range input.RevealedRooms {
			if revealed {
				dungeon.RevealedRooms[roomID] = true
			}
		}
	}

	// Merge open doors - only adds true values since doors cannot be "closed" once opened
	if input.OpenDoors != nil {
		if dungeon.OpenDoors == nil {
			dungeon.OpenDoors = make(map[string]bool)
		}
		for connID, open := range input.OpenDoors {
			if open {
				dungeon.OpenDoors[connID] = true
			}
		}
	}

	// Update metrics
	if input.RoomsCleared != nil {
		dungeon.RoomsCleared = *input.RoomsCleared
	}

	if input.MonstersKilled != nil {
		dungeon.MonstersKilled = *input.MonstersKilled
	}

	// Update completion timestamp
	if input.CompletedAt != nil {
		dungeon.CompletedAt = input.CompletedAt
	}

	return &UpdateOutput{Success: true}, nil
}

// Delete removes a dungeon
func (r *InMemoryRepository) Delete(_ context.Context, input *DeleteInput) (*DeleteOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	if input.DungeonID == "" {
		return nil, apierr.InvalidArgument("dungeon ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	dungeon, exists := r.store[input.DungeonID]
	if !exists {
		return nil, apierr.NotFound("dungeon not found")
	}

	// Remove from encounter ID index
	if dungeon.EncounterID != "" {
		delete(r.encounterIDIndex, dungeon.EncounterID)
	}

	// Remove from store
	delete(r.store, input.DungeonID)

	return &DeleteOutput{Success: true}, nil
}

// copyDungeon creates a deep copy of a dungeon using JSON serialization.
// This handles toolkit types (ConnectionEdge) and component types (Room) generically.
func copyDungeon(d *entities.Dungeon) (*entities.Dungeon, error) {
	if d == nil {
		return nil, nil
	}

	// Use JSON marshal/unmarshal for deep copy
	// This works because toolkit and component types are JSON-serializable
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}

	var result entities.Dungeon
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
