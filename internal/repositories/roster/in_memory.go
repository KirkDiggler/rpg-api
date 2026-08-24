package roster

import (
	"context"
	"errors"
	"sync"
)

// NewInMemory returns a map-backed Repository for tests and local harnesses.
func NewInMemory() Repository {
	return &inMemoryRepo{rows: make(map[string]*Data)}
}

type inMemoryRepo struct {
	mu   sync.RWMutex
	rows map[string]*Data
}

func (r *inMemoryRepo) Get(_ context.Context, encounterID string) (*Data, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	data, ok := r.rows[encounterID]
	if !ok {
		return nil, ErrNotFound
	}
	// Copy so callers cannot mutate the stored row.
	out := *data
	out.Members = append([]Member(nil), data.Members...)
	return &out, nil
}

func (r *inMemoryRepo) Save(_ context.Context, data *Data) error {
	if data == nil || data.EncounterID == "" {
		return errors.New("save roster: missing encounter id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := *data
	stored.Members = append([]Member(nil), data.Members...)
	r.rows[data.EncounterID] = &stored
	return nil
}
