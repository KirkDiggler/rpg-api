// Package encounterlog provides storage for encounter events (event sourcing)
package encounterlog

//go:generate mockgen -destination=mock/mock_repository.go -package=encounterlogmock github.com/KirkDiggler/rpg-api/internal/repositories/encounterlog Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// Repository defines the storage interface for encounter events
// Append-only by design (event sourcing)
type Repository interface {
	// Append stores a new event (events are immutable)
	Append(ctx context.Context, input *AppendInput) (*AppendOutput, error)

	// GetByEncounter retrieves events for an encounter
	GetByEncounter(ctx context.Context, input *GetByEncounterInput) (*GetByEncounterOutput, error)
}

// AppendInput contains parameters for appending an encounter event
type AppendInput struct {
	Event *entities.EncounterEvent
}

// AppendOutput contains the result of appending an encounter event
type AppendOutput struct {
	EventID string // The event's ID (from the event itself)
}

// GetByEncounterInput contains parameters for retrieving events by encounter
type GetByEncounterInput struct {
	EncounterID string
	UpToEventID string // Get events up to and including this ID (for late join; empty means from the beginning)
	Limit       int    // Max events to return (0 = no limit)
}

// GetByEncounterOutput contains the result of retrieving events
type GetByEncounterOutput struct {
	Events      []*entities.EncounterEvent
	HasMore     bool
	LastEventID string
}
