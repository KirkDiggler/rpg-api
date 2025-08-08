package encounters

//go:generate mockgen -destination=mock/mock_repository.go -package=encountermock github.com/KirkDiggler/rpg-api/internal/repositories/encounters Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// Repository defines the storage interface for encounters
type Repository interface {
	// Save stores an encounter
	Save(ctx context.Context, input *SaveInput) (*SaveOutput, error)
	
	// Get retrieves an encounter by ID
	Get(ctx context.Context, input *GetInput) (*GetOutput, error)
	
	// Update modifies an existing encounter
	Update(ctx context.Context, input *UpdateInput) (*UpdateOutput, error)
	
	// Delete removes an encounter
	Delete(ctx context.Context, input *DeleteInput) (*DeleteOutput, error)
}

// EncounterData represents the persistent state of an encounter
type EncounterData struct {
	ID             string
	RoomData       *spatial.RoomData
	InitiativeData *initiative.TrackerData
}

// SaveInput defines the request for saving an encounter
type SaveInput struct {
	EncounterID    string
	RoomData       *spatial.RoomData
	InitiativeData *initiative.TrackerData
}

// SaveOutput defines the response for saving an encounter
type SaveOutput struct {
	Success bool
}

// GetInput defines the request for retrieving an encounter
type GetInput struct {
	EncounterID string
}

// GetOutput defines the response for retrieving an encounter
type GetOutput struct {
	Data *EncounterData
}

// UpdateInput defines the request for updating an encounter
type UpdateInput struct {
	EncounterID    string
	InitiativeData *initiative.TrackerData // Usually what changes during encounter
}

// UpdateOutput defines the response for updating an encounter
type UpdateOutput struct {
	Success bool
}

// DeleteInput defines the request for deleting an encounter
type DeleteInput struct {
	EncounterID string
}

// DeleteOutput defines the response for deleting an encounter
type DeleteOutput struct {
	Success bool
}