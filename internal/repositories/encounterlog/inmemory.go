package encounterlog

import (
	"context"
	"sync"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// InMemoryRepository stores encounter events in memory
type InMemoryRepository struct {
	mu     sync.RWMutex
	events map[string][]*entities.EncounterEvent // encounterID -> events (ordered by insertion)
}

// NewInMemory creates a new in-memory repository
func NewInMemory() *InMemoryRepository {
	return &InMemoryRepository{
		events: make(map[string][]*entities.EncounterEvent),
	}
}

// Append stores a new event (events are immutable)
func (r *InMemoryRepository) Append(_ context.Context, input *AppendInput) (*AppendOutput, error) {
	if input == nil || input.Event == nil {
		return nil, apierr.InvalidArgument("event is required")
	}
	if input.Event.EncounterID == "" {
		return nil, apierr.InvalidArgument("encounter ID is required")
	}
	if input.Event.ID == "" {
		return nil, apierr.InvalidArgument("event ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.events[input.Event.EncounterID] = append(r.events[input.Event.EncounterID], input.Event)

	return &AppendOutput{EventID: input.Event.ID}, nil
}

// GetByEncounter retrieves events for an encounter
func (r *InMemoryRepository) GetByEncounter(_ context.Context, input *GetByEncounterInput) (*GetByEncounterOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}
	if input.EncounterID == "" {
		return nil, apierr.InvalidArgument("encounter ID is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	allEvents := r.events[input.EncounterID]
	if len(allEvents) == 0 {
		return &GetByEncounterOutput{Events: []*entities.EncounterEvent{}}, nil
	}

	// Filter by UpToEventID if provided (returns events from beginning up to and including target)
	filtered := make([]*entities.EncounterEvent, 0, len(allEvents))
	for _, e := range allEvents {
		filtered = append(filtered, e)
		if input.UpToEventID != "" && e.ID == input.UpToEventID {
			break // Stop after the target event (inclusive)
		}
	}

	// Apply limit
	hasMore := false
	if input.Limit > 0 && len(filtered) > input.Limit {
		filtered = filtered[:input.Limit]
		hasMore = true
	}

	lastID := ""
	if len(filtered) > 0 {
		lastID = filtered[len(filtered)-1].ID
	}

	return &GetByEncounterOutput{
		Events:      filtered,
		HasMore:     hasMore,
		LastEventID: lastID,
	}, nil
}
