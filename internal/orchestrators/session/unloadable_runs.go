package session

import (
	"context"
	"errors"
	"fmt"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// unloadableKeyPrefix keys the mark that a run's stored record cannot be
// loaded (rpg-project#942).
const unloadableKeyPrefix = "session-unloadable:v1alpha1:"

// UnloadableRuns marks runs whose stored record the session SDK refuses to
// load as invalid encounter data (sdk.ErrInvalidWorld), so that no seat is
// held by a run nobody can play (rpg-project#942).
//
// A MARK, NEVER A REPAIR. The dead run's stored data is left exactly as it
// was, for inspection; only the seat repository reads the mark, and it
// treats a seat naming a marked run as lapsed, as it already treats a seat
// naming a run whose record has expired. The SDK still owns every seat
// write.
//
// Only a caller that saw the SDK refuse the stored record as invalid data
// marks a run. A lock timeout, a cancelled context or a store outage is not
// that refusal, and never marks anything.
type UnloadableRuns struct {
	client redisclient.Client
}

// NewUnloadableRuns returns the marks kept in Redis beside the sessions.
func NewUnloadableRuns(client redisclient.Client) *UnloadableRuns {
	return &UnloadableRuns{client: client}
}

// Mark records that the run's stored record cannot be loaded, with the
// refusal it was seen with. Marking twice keeps the latest reason.
//
// Errors: an empty session id, or the store's own.
func (u *UnloadableRuns) Mark(ctx context.Context, session, reason string) error {
	if session == "" {
		return errors.New("session: UnloadableRuns.Mark session is required")
	}
	if err := u.client.Set(ctx, unloadableKeyPrefix+session, reason, 0).Err(); err != nil {
		return fmt.Errorf("mark run %q unloadable: %w", session, err)
	}
	return nil
}

// isUnloadable reports whether the run is marked unloadable.
func isUnloadable(ctx context.Context, client redisclient.Client, session string) (bool, error) {
	n, err := client.Exists(ctx, unloadableKeyPrefix+session).Result()
	if err != nil {
		return false, fmt.Errorf("check run %q unloadable: %w", session, err)
	}
	return n > 0, nil
}
