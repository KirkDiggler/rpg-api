package session

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// sharedStoreLocker routes every session verb onto one store-wide key. It
// dates from when a session verb wrote the character's discovery preference,
// a record shared across sessions. That preference is character
// configuration now (rpg-project#753 R2), read by sessions and written only
// under the character's guard, so this widening has no remaining reason; the
// locker slice (plan T4) removes it. It is still process-local.
type sharedStoreLocker struct{ inner *InProcessSessionLocker }

func (l sharedStoreLocker) LockSession(ctx context.Context, in *sdk.LockSessionInput) (*sdk.LockSessionOutput, error) {
	if in == nil {
		return nil, sdk.ErrNilInput
	}
	if in.Session == "" {
		return nil, sdk.ErrNoSessionID
	}
	return l.inner.LockSession(ctx, &sdk.LockSessionInput{Session: "shared-sdk-store"})
}

// LockCharacter takes the character's own guard, per character id. It never
// shares the store-wide session key: Launch, Join and Exit hold that key and
// then take character guards, so routing a character onto it would deadlock
// the verb on its own guard.
func (l sharedStoreLocker) LockCharacter(ctx context.Context, in *sdk.LockCharacterInput) (*sdk.LockCharacterOutput, error) {
	return l.inner.LockCharacter(ctx, in)
}
