package encounters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/KirkDiggler/rpg-toolkit/encounter"
)

// NewInMemory returns a thread-safe in-memory Repository.
//
// Save and Get round-trip the data through JSON serialization on every
// call. This costs a marshal/unmarshal cycle per operation but gives two
// real properties: callers cannot mutate stored state after Save (no
// aliasing); and the toolkit's JSON tags are exercised on every write,
// catching serialization regressions in tests rather than production.
func NewInMemory() Repository {
	return &inMemory{data: make(map[string][]byte)}
}

type inMemory struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (r *inMemory) Get(ctx context.Context, id string) (*encounter.Data, error) {
	r.mu.Lock()
	b, ok := r.data[id]
	r.mu.Unlock()
	if !ok {
		return nil, ErrNotFound
	}
	var out encounter.Data
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("unmarshal stored encounter %q: %w", id, err)
	}
	return &out, nil
}

func (r *inMemory) Save(ctx context.Context, data *encounter.Data) error {
	if data == nil {
		return errors.New("data is required")
	}
	if data.ID == "" {
		return errors.New("data.ID is required")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal encounter %q: %w", data.ID, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[string(data.ID)] = b
	return nil
}
