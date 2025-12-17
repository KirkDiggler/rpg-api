// Package dungeons provides repository interfaces and implementations for dungeon data storage.
package dungeons

import (
	"context"
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

// Save stores a dungeon
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
	r.store[input.Dungeon.ID] = copyDungeon(input.Dungeon)

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
	return &GetOutput{
		Dungeon: copyDungeon(dungeon),
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
	return &GetOutput{
		Dungeon: copyDungeon(dungeon),
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

	// Merge revealed rooms
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

	// Merge open doors
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

// copyDungeon creates a deep copy of a dungeon to prevent external mutation
func copyDungeon(d *entities.Dungeon) *entities.Dungeon {
	if d == nil {
		return nil
	}

	result := &entities.Dungeon{
		ID:             d.ID,
		EncounterID:    d.EncounterID,
		Theme:          d.Theme,
		Difficulty:     d.Difficulty,
		Length:         d.Length,
		Seed:           d.Seed,
		State:          d.State,
		StartRoomID:    d.StartRoomID,
		BossRoomID:     d.BossRoomID,
		CurrentRoomID:  d.CurrentRoomID,
		RoomsCleared:   d.RoomsCleared,
		MonstersKilled: d.MonstersKilled,
		CreatedAt:      d.CreatedAt,
		CompletedAt:    d.CompletedAt,
	}

	// Copy rooms map
	if d.Rooms != nil {
		result.Rooms = make(map[string]*entities.DungeonRoom, len(d.Rooms))
		for k, v := range d.Rooms {
			roomCopy := &entities.DungeonRoom{
				ID:     v.ID,
				Type:   v.Type,
				Width:  v.Width,
				Height: v.Height,
				Theme:  v.Theme,
				IsBoss: v.IsBoss,
			}
			if v.Monsters != nil {
				roomCopy.Monsters = make([]string, len(v.Monsters))
				copy(roomCopy.Monsters, v.Monsters)
			}
			result.Rooms[k] = roomCopy
		}
	}

	// Copy connections slice
	if d.Connections != nil {
		result.Connections = make([]*entities.DungeonConnection, len(d.Connections))
		for i, conn := range d.Connections {
			connCopy := &entities.DungeonConnection{
				ID:           conn.ID,
				FromRoomID:   conn.FromRoomID,
				ToRoomID:     conn.ToRoomID,
				PhysicalHint: conn.PhysicalHint,
			}
			if conn.FromPosition != nil {
				connCopy.FromPosition = &entities.Position{
					X: conn.FromPosition.X,
					Y: conn.FromPosition.Y,
					Z: conn.FromPosition.Z,
				}
			}
			if conn.ToPosition != nil {
				connCopy.ToPosition = &entities.Position{
					X: conn.ToPosition.X,
					Y: conn.ToPosition.Y,
					Z: conn.ToPosition.Z,
				}
			}
			result.Connections[i] = connCopy
		}
	}

	// Copy revealed rooms map
	if d.RevealedRooms != nil {
		result.RevealedRooms = make(map[string]bool, len(d.RevealedRooms))
		for k, v := range d.RevealedRooms {
			result.RevealedRooms[k] = v
		}
	}

	// Copy open doors map
	if d.OpenDoors != nil {
		result.OpenDoors = make(map[string]bool, len(d.OpenDoors))
		for k, v := range d.OpenDoors {
			result.OpenDoors[k] = v
		}
	}

	return result
}
