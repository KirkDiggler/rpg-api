package session

import (
	"context"
	"sync"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// InProcessSessionLocker serializes complete SDK operations by session ID,
// and character verbs by character ID. Waiting is cancellable; acquired guards
// remain held until Release, even if the request is canceled. Idle entries are
// removed after the last owner/waiter.
//
// SESSIONS AND CHARACTERS ARE TWO KEY SPACES. The SDK takes a session's guard
// and then the guards of the characters it seats or releases (Launch, Join,
// Exit), so a character key that could collide with a session key — or a
// store-wide session key (sharedStoreLocker) — would deadlock the verb on its
// own non-reentrant guard. The two maps can never share an entry.
//
// Share one instance among Managers accessing the same sessions. This protects
// only one process, not uncoordinated API replicas. A multi-process host must
// supply a shared sdk.SessionLocker instead. The zero value is usable.
type InProcessSessionLocker struct {
	mu         sync.Mutex
	entries    map[string]*sessionLockEntry // session guards
	characters map[string]*sessionLockEntry // character guards
}

type sessionLockEntry struct {
	token chan struct{}
	refs  int // guarded by InProcessSessionLocker.mu; includes owner and waiters
}

var _ sdk.SessionLocker = (*InProcessSessionLocker)(nil)

// NewInProcessSessionLocker constructs a process-local locker keyed by session
// and, separately, by character.
func NewInProcessSessionLocker() *InProcessSessionLocker {
	return &InProcessSessionLocker{}
}

// LockSession acquires the session guard or returns without one on cancellation
// or invalid input. Release is idempotent and does not use the request context.
func (l *InProcessSessionLocker) LockSession(ctx context.Context, in *sdk.LockSessionInput) (*sdk.LockSessionOutput, error) {
	if in == nil {
		return nil, sdk.ErrNilInput
	}
	if in.Session == "" {
		return nil, sdk.ErrNoSessionID
	}
	release, err := l.acquire(ctx, sessionSpace, in.Session)
	if err != nil {
		return nil, err
	}
	return &sdk.LockSessionOutput{Release: release}, nil
}

// LockCharacter acquires one character's guard or returns without one on
// cancellation or invalid input. Release is idempotent and does not use the
// request context.
func (l *InProcessSessionLocker) LockCharacter(ctx context.Context, in *sdk.LockCharacterInput) (*sdk.LockCharacterOutput, error) {
	if in == nil {
		return nil, sdk.ErrNilInput
	}
	if in.Character == "" {
		return nil, sdk.ErrNoMemberID
	}
	release, err := l.acquire(ctx, characterSpace, in.Character)
	if err != nil {
		return nil, err
	}
	return &sdk.LockCharacterOutput{Release: release}, nil
}

// lockSpace names which of the two key spaces a guard lives in.
type lockSpace int

const (
	sessionSpace lockSpace = iota
	characterSpace
)

// entriesOf returns the map for space, creating it on first use so the zero
// value stays usable. Callers hold l.mu.
func (l *InProcessSessionLocker) entriesOf(space lockSpace) map[string]*sessionLockEntry {
	if space == characterSpace {
		if l.characters == nil {
			l.characters = make(map[string]*sessionLockEntry)
		}
		return l.characters
	}
	if l.entries == nil {
		l.entries = make(map[string]*sessionLockEntry)
	}
	return l.entries
}

// acquire takes the guard for id in the given key space.
func (l *InProcessSessionLocker) acquire(ctx context.Context, space lockSpace, id string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	entries := l.entriesOf(space)
	entry := entries[id]
	if entry == nil {
		entry = &sessionLockEntry{token: make(chan struct{}, 1)}
		entry.token <- struct{}{}
		entries[id] = entry
	}
	entry.refs++
	l.mu.Unlock()

	dropReference := func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		entry.refs--
		if entry.refs == 0 {
			delete(entries, id)
		}
	}
	select {
	case <-ctx.Done():
		dropReference()
		return nil, ctx.Err()
	case <-entry.token:
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			entry.token <- struct{}{}
			dropReference()
		})
	}
	// Both select arms may have become ready together. Do not hand a canceled
	// waiter permission to execute merely because the token arm won the select.
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}
