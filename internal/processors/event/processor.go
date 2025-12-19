// Package event provides event processing for encounter events
package event

//go:generate mockgen -destination=mock/mock_processor.go -package=eventmock github.com/KirkDiggler/rpg-api/internal/processors/event Processor

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/publishers/encounter"
	"github.com/KirkDiggler/rpg-api/internal/repositories/encounterlog"
)

// Processor handles event persistence and publication
type Processor interface {
	// Process persists the event and publishes it to subscribers
	Process(ctx context.Context, input *ProcessInput) (*ProcessOutput, error)
}

// ProcessInput contains parameters for processing an event
type ProcessInput struct {
	EncounterID string
	Event       *entities.EncounterEvent
}

// ProcessOutput contains the result of processing an event
type ProcessOutput struct {
	EventID string
}

// Config holds dependencies for the processor
type Config struct {
	EncounterLogRepo encounterlog.Repository
	Publisher        encounter.Publisher
}

type processor struct {
	encounterLogRepo encounterlog.Repository
	publisher        encounter.Publisher
}

// New creates a new event processor
func New(cfg *Config) (Processor, error) {
	if cfg == nil {
		return nil, apierr.InvalidArgument("config is required")
	}
	if cfg.EncounterLogRepo == nil {
		return nil, apierr.InvalidArgument("encounter log repository is required")
	}
	if cfg.Publisher == nil {
		return nil, apierr.InvalidArgument("publisher is required")
	}

	return &processor{
		encounterLogRepo: cfg.EncounterLogRepo,
		publisher:        cfg.Publisher,
	}, nil
}

// Process persists the event and publishes it to subscribers
func (p *processor) Process(ctx context.Context, input *ProcessInput) (*ProcessOutput, error) {
	if input == nil || input.Event == nil {
		return nil, apierr.InvalidArgument("event is required")
	}

	// 1. Persist to encounter log (source of truth)
	appendOutput, err := p.encounterLogRepo.Append(ctx, &encounterlog.AppendInput{
		Event: input.Event,
	})
	if err != nil {
		return nil, apierr.Wrapf(err, "failed to persist event")
	}

	// 2. Publish to subscribers (for real-time updates)
	// Publish failures don't fail the operation - persistence is source of truth
	// In production, might want to log, retry, or queue failed publishes
	_, _ = p.publisher.Publish(ctx, &encounter.PublishInput{
		EncounterID: input.EncounterID,
		Event:       input.Event,
	})

	// Use the EventID from the repository response - repo is source of truth
	return &ProcessOutput{EventID: appendOutput.EventID}, nil
}
