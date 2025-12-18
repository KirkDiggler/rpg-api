// Package encounters provides repository interfaces and implementations for encounter data storage.
package encounters

import (
	"context"
	"sync"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
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
func (r *InMemoryRepository) Save(_ context.Context, input *SaveInput) (*SaveOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	if input.EncounterID == "" {
		return nil, apierr.InvalidArgument("encounter ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.store[input.EncounterID] = &EncounterData{
		ID:                input.EncounterID,
		RoomData:          input.RoomData,
		InitiativeData:    input.InitiativeData,
		InitiativeRolls:   input.InitiativeRolls,
		MovementRemaining: input.MovementRemaining,
		ActionEconomy:     input.ActionEconomy,
		Monsters:          input.Monsters,
		BossMonsterIDs:    input.BossMonsterIDs,
		CharacterHP:       input.CharacterHP,
		State:             input.State,
		JoinCode:          input.JoinCode,
		HostID:            input.HostID,
		Players:           input.Players,
		CreatedAt:         input.CreatedAt,
	}

	return &SaveOutput{Success: true}, nil
}

// Get retrieves an encounter by ID
func (r *InMemoryRepository) Get(_ context.Context, input *GetInput) (*GetOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	if input.EncounterID == "" {
		return nil, apierr.InvalidArgument("encounter ID is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	data, exists := r.store[input.EncounterID]
	if !exists {
		return nil, apierr.NotFound("encounter not found")
	}

	// Return a copy to prevent external modification
	return &GetOutput{
		Data: &EncounterData{
			ID:                data.ID,
			RoomData:          data.RoomData,
			InitiativeData:    data.InitiativeData,
			InitiativeRolls:   data.InitiativeRolls,
			MovementRemaining: data.MovementRemaining,
			ActionEconomy:     data.ActionEconomy,
			Monsters:          data.Monsters,
			BossMonsterIDs:    data.BossMonsterIDs,
			CharacterHP:       data.CharacterHP,
			State:             data.State,
			JoinCode:          data.JoinCode,
			HostID:            data.HostID,
			Players:           data.Players,
			CreatedAt:         data.CreatedAt,
		},
	}, nil
}

// GetByJoinCode retrieves an encounter by its join code
func (r *InMemoryRepository) GetByJoinCode(_ context.Context, input *GetByJoinCodeInput) (*GetOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	if input.JoinCode == "" {
		return nil, apierr.InvalidArgument("join code is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Search for encounter with matching join code
	for _, data := range r.store {
		if data.JoinCode == input.JoinCode {
			// Return a copy to prevent external modification
			return &GetOutput{
				Data: &EncounterData{
					ID:                data.ID,
					RoomData:          data.RoomData,
					InitiativeData:    data.InitiativeData,
					InitiativeRolls:   data.InitiativeRolls,
					MovementRemaining: data.MovementRemaining,
					ActionEconomy:     data.ActionEconomy,
					Monsters:          data.Monsters,
					BossMonsterIDs:    data.BossMonsterIDs,
					CharacterHP:       data.CharacterHP,
					State:             data.State,
					JoinCode:          data.JoinCode,
					HostID:            data.HostID,
					Players:           data.Players,
					CreatedAt:         data.CreatedAt,
				},
			}, nil
		}
	}

	return nil, apierr.NotFound("encounter not found")
}

// Update modifies an existing encounter
func (r *InMemoryRepository) Update(_ context.Context, input *UpdateInput) (*UpdateOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	if input.EncounterID == "" {
		return nil, apierr.InvalidArgument("encounter ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	data, exists := r.store[input.EncounterID]
	if !exists {
		return nil, apierr.NotFound("encounter not found")
	}

	// Update only what's provided
	if input.InitiativeData != nil {
		data.InitiativeData = input.InitiativeData
	}
	if input.RoomData != nil {
		data.RoomData = input.RoomData
	}
	if input.MovementRemaining != nil {
		data.MovementRemaining = *input.MovementRemaining
	}
	if input.ActionEconomy != nil {
		data.ActionEconomy = input.ActionEconomy
	}
	if input.Monsters != nil {
		data.Monsters = input.Monsters
	}
	if input.BossMonsterIDs != nil {
		data.BossMonsterIDs = input.BossMonsterIDs
	}
	if input.CharacterHP != nil {
		data.CharacterHP = input.CharacterHP
	}

	// Multiplayer fields
	if input.State != nil {
		data.State = *input.State
	}
	if input.HostID != nil {
		data.HostID = *input.HostID
	}
	if input.Players != nil {
		data.Players = input.Players
	}

	return &UpdateOutput{Success: true}, nil
}

// Delete removes an encounter
func (r *InMemoryRepository) Delete(_ context.Context, input *DeleteInput) (*DeleteOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	if input.EncounterID == "" {
		return nil, apierr.InvalidArgument("encounter ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.store[input.EncounterID]; !exists {
		return nil, apierr.NotFound("encounter not found")
	}

	delete(r.store, input.EncounterID)

	return &DeleteOutput{Success: true}, nil
}
