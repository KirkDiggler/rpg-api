package encounters

import (
	"context"
	"sync"

	"github.com/KirkDiggler/rpg-api/internal/errors"
)

// InMemoryRepository implements Repository using in-memory storage
type InMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*EncounterData
}

// NewInMemory creates a new in-memory repository
func NewInMemory() *InMemoryRepository {
	return &InMemoryRepository{
		store: make(map[string]*EncounterData),
	}
}

// Save stores an encounter
func (r *InMemoryRepository) Save(ctx context.Context, input *SaveInput) (*SaveOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	if input.EncounterID == "" {
		return nil, errors.InvalidArgument("encounter ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.store[input.EncounterID] = &EncounterData{
		ID:             input.EncounterID,
		RoomData:       input.RoomData,
		InitiativeData: input.InitiativeData,
	}

	return &SaveOutput{Success: true}, nil
}

// Get retrieves an encounter by ID
func (r *InMemoryRepository) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	if input.EncounterID == "" {
		return nil, errors.InvalidArgument("encounter ID is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	data, exists := r.store[input.EncounterID]
	if !exists {
		return nil, errors.NotFound("encounter not found")
	}

	// Return a copy to prevent external modification
	return &GetOutput{
		Data: &EncounterData{
			ID:             data.ID,
			RoomData:       data.RoomData,
			InitiativeData: data.InitiativeData,
		},
	}, nil
}

// Update modifies an existing encounter
func (r *InMemoryRepository) Update(ctx context.Context, input *UpdateInput) (*UpdateOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	if input.EncounterID == "" {
		return nil, errors.InvalidArgument("encounter ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	data, exists := r.store[input.EncounterID]
	if !exists {
		return nil, errors.NotFound("encounter not found")
	}

	// Update only what's provided
	if input.InitiativeData != nil {
		data.InitiativeData = input.InitiativeData
	}

	return &UpdateOutput{Success: true}, nil
}

// Delete removes an encounter
func (r *InMemoryRepository) Delete(ctx context.Context, input *DeleteInput) (*DeleteOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	if input.EncounterID == "" {
		return nil, errors.InvalidArgument("encounter ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.store[input.EncounterID]; !exists {
		return nil, errors.NotFound("encounter not found")
	}

	delete(r.store, input.EncounterID)

	return &DeleteOutput{Success: true}, nil
}
