package lobby_test

import (
	"context"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// observedLobbyRepository wraps a REAL lobby repository to observe — and, in
// the refusal and failure cases, inject — behavior at Save, the one write
// StartEncounter makes on the lobby record. It is the smallest wrapper that can
// prove ordering (save before publish) and count writes without a second
// repository implementation.
//
// beforeSave runs BEFORE the underlying write and before any injected error, so
// a test can assert the exact record the launch was about to persist and read
// the world at that moment. It is declared here because Task 3's contract case
// uses it first and the later failure cases need the same type.
type observedLobbyRepository struct {
	lobbyrepo.Repository

	// saveCalls counts Save invocations through this wrapper.
	saveCalls int

	// beforeSave, when set, observes data immediately before the write.
	beforeSave func(*lobbyrepo.Data)

	// saveErr, when set, fails the write with this error instead of persisting.
	saveErr error
}

func (r *observedLobbyRepository) Save(ctx context.Context, data *lobbyrepo.Data) error {
	r.saveCalls++
	if r.beforeSave != nil {
		r.beforeSave(data)
	}
	if r.saveErr != nil {
		return r.saveErr
	}
	return r.Repository.Save(ctx, data)
}
