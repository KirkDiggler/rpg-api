package lobby

import (
	"context"
	"errors"
	"log/slog"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// unloadable.go is A RUN THAT CANNOT BE LOADED HOLDS NO SEAT (rpg-project#942).
//
// A run whose stored record the SDK refuses as invalid encounter data
// (sdk.ErrInvalidWorld) fails every verb, so its members could never leave
// it, and the seats it holds would keep them from starting or joining any
// other run. When the lobby sees that refusal it logs it at warning level
// with the session id, marks the run (Config.UnloadableRuns), and treats it
// as ended: the seat repository then answers its seats as lapsed. The run's
// stored data is left in place for inspection, never deleted or repaired.
//
// NARROW ON PURPOSE. Only sdk.ErrInvalidWorld from reading the run marks
// it. A lock timeout, a cancelled context or a store outage is a transient
// failure of the read, not a verdict on the record, and never releases a
// seat: the caller sees the error, exactly as before.

// unloadableRun reports whether err is the SDK refusing a run's stored
// record, and if so logs it and marks the run. A failed mark is logged and
// does not change the answer: the run is still unloadable, its seats simply
// stay held until a later read marks it.
func (o *Orchestrator) unloadableRun(ctx context.Context, session string, err error) bool {
	if !errors.Is(err, sdk.ErrInvalidWorld) {
		return false
	}
	slog.WarnContext(ctx, "lobby: a run's stored record cannot be loaded; treating it as ended and releasing its seats",
		"session", session, "error", err)
	if o.unloadable == nil {
		return true
	}
	if markErr := o.unloadable.Mark(ctx, session, err.Error()); markErr != nil {
		slog.WarnContext(ctx, "lobby: could not mark an unloadable run", "session", session, "error", markErr)
	}
	return true
}

// releaseUnloadableSeats looks at where each party character is seated and
// marks every run among them whose stored record cannot be loaded; asked
// only when the lobby has somewhere to mark (Config.UnloadableRuns). It
// reports whether any seat was released that way, so StartEncounter may try
// its launch once more. Any other error on the way (a seat or a status that
// could not be read) releases nothing and is returned.
func (o *Orchestrator) releaseUnloadableSeats(ctx context.Context, launching string, party []string) (bool, error) {
	released := false
	seen := map[string]bool{}
	for _, character := range party {
		seat, err := o.sessionManager.Seat(ctx, &sdk.SeatInput{Character: character})
		if err != nil {
			if errors.Is(err, sdk.ErrNoSeat) {
				continue
			}
			return false, err
		}
		run := seat.Seat.Session
		if run == launching || seen[run] {
			continue
		}
		seen[run] = true
		if _, err := o.sessionManager.Status(ctx, &sdk.StatusInput{Session: run}); err != nil {
			if o.unloadableRun(ctx, run, err) {
				released = true
				continue
			}
			return false, err
		}
	}
	return released, nil
}
