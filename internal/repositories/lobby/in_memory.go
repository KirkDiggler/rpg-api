package lobby

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// NewInMemory returns a thread-safe in-memory Repository, for tests and the
// bufconn integration harness.
//
// Save and Get round-trip through JSON serialization on every call, matching
// the encountersv2 in-memory repo's contract: callers cannot mutate stored
// state after Save (no aliasing), and the JSON tags are exercised on every
// write.
func NewInMemory() Repository {
	return &inMemory{
		byID:       make(map[string][]byte),
		idByRef:    make(map[string]string),
		idByPlayer: make(map[string]string),
	}
}

type inMemory struct {
	mu   sync.Mutex
	byID map[string][]byte
	// idByRef maps join_ref -> lobby id, the secondary index GetByJoinRef reads.
	idByRef map[string]string
	// idByPlayer maps player id -> lobby id, the secondary index
	// GetByPlayerID reads. Save keeps this in lockstep with data.Members on
	// every write; ClearPlayerIndex is the only way an entry is removed.
	idByPlayer map[string]string
}

func (r *inMemory) Get(_ context.Context, id string) (*Data, error) {
	r.mu.Lock()
	b, ok := r.byID[id]
	r.mu.Unlock()
	if !ok {
		return nil, ErrNotFound
	}
	return decode(b, id)
}

func (r *inMemory) GetByJoinRef(_ context.Context, joinRef string) (*Data, error) {
	r.mu.Lock()
	id, ok := r.idByRef[joinRef]
	var b []byte
	if ok {
		b, ok = r.byID[id]
	}
	r.mu.Unlock()
	if !ok {
		return nil, ErrNotFound
	}
	return decode(b, id)
}

func (r *inMemory) GetByPlayerID(_ context.Context, playerID string) (*Data, error) {
	r.mu.Lock()
	id, ok := r.idByPlayer[playerID]
	var b []byte
	if ok {
		b, ok = r.byID[id]
	}
	r.mu.Unlock()
	if !ok {
		return nil, ErrNotFound
	}
	return decode(b, id)
}

func (r *inMemory) Save(_ context.Context, data *Data) error {
	if data == nil {
		return errors.New("data is required")
	}
	if data.ID == "" {
		return errors.New("data.ID is required")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal lobby %q: %w", data.ID, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[data.ID] = b
	if data.JoinRef != "" {
		r.idByRef[data.JoinRef] = data.ID
	}
	for playerID := range data.Members {
		r.idByPlayer[playerID] = data.ID
	}
	return nil
}

func (r *inMemory) ClearPlayerIndex(_ context.Context, playerID string) error {
	r.mu.Lock()
	delete(r.idByPlayer, playerID)
	r.mu.Unlock()
	return nil
}

func decode(b []byte, id string) (*Data, error) {
	var out Data
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("unmarshal stored lobby %q: %w", id, err)
	}
	return &out, nil
}
