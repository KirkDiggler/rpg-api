// Package dungeons provides repository interfaces and implementations for dungeon data storage.
package dungeons

//go:generate mockgen -destination=mock/mock_repository.go -package=dungeonmock github.com/KirkDiggler/rpg-api/internal/repositories/dungeons Repository

import (
	"context"
	"time"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// Repository defines the storage interface for dungeons
type Repository interface {
	// Save stores a dungeon
	Save(ctx context.Context, input *SaveInput) (*SaveOutput, error)

	// Get retrieves a dungeon by ID
	Get(ctx context.Context, input *GetInput) (*GetOutput, error)

	// GetByEncounterID retrieves a dungeon by its associated encounter ID
	GetByEncounterID(ctx context.Context, input *GetByEncounterIDInput) (*GetOutput, error)

	// Update modifies an existing dungeon
	Update(ctx context.Context, input *UpdateInput) (*UpdateOutput, error)

	// Delete removes a dungeon
	Delete(ctx context.Context, input *DeleteInput) (*DeleteOutput, error)
}

// SaveInput defines the request for saving a dungeon
type SaveInput struct {
	Dungeon *entities.Dungeon
}

// SaveOutput defines the response for saving a dungeon
type SaveOutput struct {
	Success bool
}

// GetInput defines the request for retrieving a dungeon
type GetInput struct {
	DungeonID string
}

// GetOutput defines the response for retrieving a dungeon
type GetOutput struct {
	Dungeon *entities.Dungeon
}

// GetByEncounterIDInput defines the request for retrieving a dungeon by encounter ID
type GetByEncounterIDInput struct {
	EncounterID string
}

// UpdateInput defines the request for updating a dungeon
type UpdateInput struct {
	DungeonID string

	// State changes
	State *entities.DungeonState

	// Exploration updates
	RevealedRooms map[string]bool // Additional rooms revealed
	OpenDoors     map[string]bool // Additional doors opened
	CurrentRoomID *string         // New current room

	// Metrics updates
	RoomsCleared   *int
	MonstersKilled *int

	// Completion
	CompletedAt *time.Time
}

// UpdateOutput defines the response for updating a dungeon
type UpdateOutput struct {
	Success bool
}

// DeleteInput defines the request for deleting a dungeon
type DeleteInput struct {
	DungeonID string
}

// DeleteOutput defines the response for deleting a dungeon
type DeleteOutput struct {
	Success bool
}
