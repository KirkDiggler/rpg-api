package encounters

import (
	"context"
	"errors"
	"sync"

	"github.com/KirkDiggler/rpg-toolkit/encounter"
)

// NewInMemory returns a thread-safe in-memory Repository.
func NewInMemory() Repository {
	return &inMemory{data: make(map[string]*encounter.Data)}
}

type inMemory struct {
	mu   sync.Mutex
	data map[string]*encounter.Data
}

func (r *inMemory) Get(ctx context.Context, id string) (*encounter.Data, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	return d, nil
}

func (r *inMemory) Save(ctx context.Context, data *encounter.Data) error {
	if data == nil {
		return errors.New("data is required")
	}
	if data.ID == "" {
		return errors.New("data.ID is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[string(data.ID)] = data
	return nil
}
