// Package combatlog provides storage for combat events (event sourcing)
package combatlog

//go:generate mockgen -destination=mock/mock_repository.go -package=combatlogmock github.com/KirkDiggler/rpg-api/internal/repositories/combatlog Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// Repository defines the storage interface for combat events
// Append-only by design (event sourcing)
type Repository interface {
	// Append stores a new event (events are immutable)
	Append(ctx context.Context, input *AppendInput) (*AppendOutput, error)

	// GetByEncounter retrieves events for an encounter
	GetByEncounter(ctx context.Context, input *GetByEncounterInput) (*GetByEncounterOutput, error)
}

// AppendInput contains parameters for appending a combat event
type AppendInput struct {
	Event *entities.EncounterEvent
}

// AppendOutput contains the result of appending a combat event
type AppendOutput struct {
	EventID string // The event's ID (from the event itself)
}

// GetByEncounterInput contains parameters for retrieving events by encounter
type GetByEncounterInput struct {
	EncounterID string
	UpToEventID string // Get events up to this ID (for late join)
	Limit       int    // Max events to return (0 = no limit)
}

// GetByEncounterOutput contains the result of retrieving events
type GetByEncounterOutput struct {
	Events      []*entities.EncounterEvent
	HasMore     bool
	LastEventID string
}
