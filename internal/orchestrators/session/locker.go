package session

import (
	"context"
	"sync"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// InProcessSessionLocker serializes complete SDK operations by session ID.
// Waiting is cancellable; acquired guards remain held until Release, even if
// the request is canceled. Idle entries are removed after the last owner/waiter.
//
// Share one instance among Managers accessing the same sessions. This protects
// only one process, not uncoordinated API replicas or out-of-session character
// writes. A multi-process host must supply a shared sdk.SessionLocker instead.
// The zero value is usable.
type InProcessSessionLocker struct {
	mu      sync.Mutex
	entries map[string]*sessionLockEntry
}

type sessionLockEntry struct {
	token chan struct{}
	refs  int // guarded by InProcessSessionLocker.mu; includes owner and waiters
}

var _ sdk.SessionLocker = (*InProcessSessionLocker)(nil)

// NewInProcessSessionLocker constructs a process-local, session-keyed locker.
func NewInProcessSessionLocker() *InProcessSessionLocker {
	return &InProcessSessionLocker{}
}

// LockSession acquires the session guard or returns without one on cancellation
// or invalid input. Release is idempotent and does not use the request context.
func (l *InProcessSessionLocker) LockSession(ctx context.Context, in *sdk.LockSessionInput) (*sdk.LockSessionOutput, error) {
	if in == nil {
		return nil, sdk.ErrNilInput
	}
	id := in.Session
	if id == "" {
		return nil, sdk.ErrNoSessionID
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	if l.entries == nil {
		l.entries = make(map[string]*sessionLockEntry)
	}
	entry := l.entries[id]
	if entry == nil {
		entry = &sessionLockEntry{token: make(chan struct{}, 1)}
		entry.token <- struct{}{}
		l.entries[id] = entry
	}
	entry.refs++
	l.mu.Unlock()

	dropReference := func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		entry.refs--
		if entry.refs == 0 {
			delete(l.entries, id)
		}
	}
	select {
	case <-ctx.Done():
		dropReference()
		return nil, ctx.Err()
	case <-entry.token:
	}
	var once sync.Once
	out := &sdk.LockSessionOutput{Release: func() {
		once.Do(func() {
			entry.token <- struct{}{}
			dropReference()
		})
	}}
	// Both select arms may have become ready together. Do not hand a canceled
	// waiter permission to execute merely because the token arm won the select.
	if err := ctx.Err(); err != nil {
		out.Release()
		return nil, err
	}
	return out, nil
}
