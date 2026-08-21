package lobby

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// AbandonEncounterInput carries the entity-typed AbandonEncounter request.
type AbandonEncounterInput struct {
	// PlayerID is the authenticated caller. Must be the lobby's host.
	PlayerID string
	LobbyID  string
}

// AbandonEncounterOutput is the lean result of an abandon. Clients learn the
// terminal transition from the session's own ended broadcast, not from this
// response — mirrors every other state-changing lobby RPC (SetReady,
// LeaveLobby).
type AbandonEncounterOutput struct{}

// AbandonEncounter administratively ends the lobby's STARTED session —
// host-only, rpg-api#663. The escape hatch for a stuck or unwanted session
// with no other way to end.
//
// Ends the session through the toolkit's declared "withdrawn" ending
// (sessionworld.EndingWithdrawn — the same ending the reference tomb's own
// world declares, session.Manager.End's "declared external ending"
// contract): the party administratively left, rather than being defeated or
// completing the dungeon. Manager.End is load-act-save-return in one call,
// so this method (unlike the old encounter stack's own AbandonEncounter)
// does its own load/mutate/persist internally.
//
// Unlike every other mutating RPC in this package (CreateLobby, LeaveLobby,
// StartEncounter), AbandonEncounter never writes the lobby record — it only
// needs to READ it (host check, resolve EncounterID) and end the session.
// This is deliberate, not an oversight:
//
//   - GetMyActiveLobby's liveness check (get_my_active_lobby.go) already
//     treats a STARTED lobby whose session is no longer Open identically to
//     "no active lobby" — the WHOLE Output is zeroed, not just EncounterID.
//   - CreateLobby's Save() unconditionally refreshes the caller's
//     player->lobby index for the member set of the NEW lobby it just
//     built, regardless of what a stale index entry previously pointed at
//     — so a fresh CreateLobby -> StartEncounter after an abandon needs no
//     cleanup here either.
//
// No new lobby Status value, no reverse encounter->lobby index: both were
// considered and rejected as infrastructure the actual flow doesn't need.
//
// Locking: unlike StartEncounter/LeaveLobby, this method does not take the
// per-lobby lock (o.locks) — it never mutates the lobby record, only reads
// it. It also does not take any per-session lock: none exists in this
// codebase today, so AbandonEncounter racing a concurrent session verb on
// the same session carries the same pre-existing risk profile as any two
// session verbs racing each other — not a new gap introduced here.
func (o *Orchestrator) AbandonEncounter(ctx context.Context, in *AbandonEncounterInput) (*AbandonEncounterOutput, error) {
	if in == nil {
		return nil, errors.New("lobby orchestrator: AbandonEncounterInput is required")
	}

	data, err := o.lobbyRepo.Get(ctx, in.LobbyID)
	if err != nil {
		if errors.Is(err, lobbyrepo.ErrNotFound) {
			return nil, ErrLobbyNotFound
		}
		return nil, fmt.Errorf("load lobby %q: %w", in.LobbyID, err)
	}
	if data.HostPlayerID != in.PlayerID {
		return nil, ErrNotHost
	}
	if data.Status != lobbyrepo.StatusStarted || data.EncounterID == "" {
		return nil, ErrLobbyNotStarted
	}

	if _, err := o.sessionManager.End(ctx, &sdk.EndInput{
		Session: data.EncounterID, Ending: sessionworld.EndingWithdrawn,
	}); err != nil {
		if errors.Is(err, sdk.ErrNoSession) || errors.Is(err, sdk.ErrNoEncounter) || errors.Is(err, sdk.ErrClosed) {
			// Nothing left to abandon — same caller-visible outcome whether
			// the session is simply gone or already ended.
			return nil, ErrEncounterAlreadyEnded
		}
		return nil, fmt.Errorf("end session %q: %w", data.EncounterID, err)
	}

	return &AbandonEncounterOutput{}, nil
}
