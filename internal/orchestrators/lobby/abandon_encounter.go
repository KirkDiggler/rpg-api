package lobby

import (
	"context"
	"errors"
	"fmt"

	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
)

// AbandonEncounterInput carries the entity-typed AbandonEncounter request.
type AbandonEncounterInput struct {
	// PlayerID is the authenticated caller. Must be the lobby's host.
	PlayerID string
	LobbyID  string
}

// AbandonEncounterOutput is the lean result of an abandon. Clients learn the
// terminal transition from the encounter's own EncounterEnded broadcast on
// StreamEncounter, not from this response — mirrors every other
// state-changing lobby RPC (SetReady, LeaveLobby).
type AbandonEncounterOutput struct{}

// AbandonEncounter administratively ends the lobby's STARTED encounter —
// host-only, rpg-api#663. The escape hatch for a stuck or unwanted encounter
// (e.g. a FREE_ROAM deadlock, rpg-toolkit#483) with no other way to end.
//
// Unlike every other mutating RPC in this package (CreateLobby, LeaveLobby,
// StartEncounter), AbandonEncounter never writes the lobby record — it only
// needs to READ it (host check, resolve EncounterID) and WRITE the
// encounter. This is deliberate, not an oversight:
//
//   - GetMyActiveLobby's liveness check (get_my_active_lobby.go) already
//     treats a STARTED lobby whose encounter carries Mode == ModeEnded
//     identically to "no active lobby" — the WHOLE Output is zeroed, not
//     just EncounterID. Once rpg-toolkit#797's End(reason) persists that
//     Mode, GetMyActiveLobby reports correctly with no cooperating write
//     from this method.
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
// it. It also does not take any per-encounter lock: none exists in this
// codebase today (the v2 encounter orchestrator's own combat verbs share
// the same unguarded load-mutate-save exposure against each other), so
// AbandonEncounter racing a concurrent combat verb on the same encounter
// carries the same pre-existing risk profile as any two combat verbs
// racing each other — not a new gap introduced here.
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

	encData, err := o.encounterRepo.Get(ctx, data.EncounterID)
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			// Nothing left to abandon — same caller-visible outcome as an
			// encounter that ended naturally.
			return nil, ErrEncounterAlreadyEnded
		}
		return nil, fmt.Errorf("load encounter %q: %w", data.EncounterID, err)
	}

	enc, err := tkenc.LoadFromData(ctx, encData, o.encounterBroker,
		tkenc.WithCharacterResolver(o.buildCharacterResolver(encData)),
		tkenc.WithCombatResolver(o.buildCombatResolver(encData)),
		tkenc.WithMovementResolver(o.buildMovementResolver(encData)),
	)
	if err != nil {
		return nil, fmt.Errorf("load encounter from data %q: %w", data.EncounterID, err)
	}

	if endErr := enc.End(tkenc.EncounterEndedReasonAbandoned); endErr != nil {
		if errors.Is(endErr, tkenc.ErrEncounterEnded) {
			return nil, ErrEncounterAlreadyEnded
		}
		return nil, fmt.Errorf("end encounter %q: %w", data.EncounterID, endErr)
	}

	if err := o.encounterRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, fmt.Errorf("save encounter %q: %w", data.EncounterID, err)
	}

	return &AbandonEncounterOutput{}, nil
}
