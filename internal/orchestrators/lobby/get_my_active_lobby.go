package lobby

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// GetMyActiveLobbyInput carries the entity-typed GetMyActiveLobby request.
type GetMyActiveLobbyInput struct {
	// PlayerID is the authenticated caller. Required.
	PlayerID string
}

// GetMyActiveLobbyOutput reports the caller's active lobby, if any.
//
// A zero-value Output (LobbyID == "") means no active lobby — the normal
// response for the common case of a player who isn't currently in one, not
// an error.
//
// EncounterID is set if and only if Status is StatusStarted AND the
// session has been verified still open (Manager.Status reports Open). A
// STARTED lobby whose session is not open has nothing left for a client
// to resume into — the lobby record itself is a terminal, dead husk at that
// point (WAITING -> STARTED never reverses; see lobbyrepo.Status) — so this
// method reports that case identically to "no active lobby": the whole
// Output is zeroed, not just EncounterID. That keeps callers to one branch
// ("is LobbyID empty?") instead of a second STARTED-but-unusable case they
// would otherwise have to special-case.
type GetMyActiveLobbyOutput struct {
	LobbyID     string
	EncounterID string
	Status      lobbyrepo.Status
}

// GetMyActiveLobby resolves the authenticated player's own active lobby —
// resume-after-refresh support (rpg-dnd5e-web#444: a client that has lost
// all in-memory session state calls this once to learn whether it should
// skip straight back into a lobby or a running encounter instead of landing
// on Home). Pure lookup: no lock, no mutation, no broadcast.
func (o *Orchestrator) GetMyActiveLobby(ctx context.Context, in *GetMyActiveLobbyInput) (*GetMyActiveLobbyOutput, error) {
	if in == nil {
		return nil, errors.New("lobby orchestrator: GetMyActiveLobbyInput is required")
	}
	if in.PlayerID == "" {
		return nil, errors.New("lobby orchestrator: GetMyActiveLobbyInput.PlayerID is required")
	}

	data, err := o.lobbyRepo.GetByPlayerID(ctx, in.PlayerID)
	if err != nil {
		if errors.Is(err, lobbyrepo.ErrNotFound) {
			return &GetMyActiveLobbyOutput{}, nil
		}
		return nil, fmt.Errorf("load active lobby for player %q: %w", in.PlayerID, err)
	}

	// The index is a pointer, not a source of truth — verify the player is
	// actually still in the lobby it points at before trusting it. Save only
	// adds/refreshes an index entry for players CURRENTLY in data.Members
	// (see Repository.Save's doc comment) and never removes a departed
	// player's entry itself; LeaveLobby's ClearPlayerIndex call is what's
	// supposed to do that, and it's deliberately best-effort (leave_lobby.go)
	// so an infra hiccup there can leave a stale entry pointing at a lobby
	// the player already left. Catching that here, not just at write time,
	// means the discarded-error path in LeaveLobby is fully safe: the write
	// side stays best-effort, this read side is authoritative. Self-heals
	// the stale entry so it doesn't keep costing a lookup until its TTL
	// expires.
	if _, isMember := data.Members[in.PlayerID]; !isMember {
		_ = o.lobbyRepo.ClearPlayerIndex(ctx, in.PlayerID)
		return &GetMyActiveLobbyOutput{}, nil
	}

	if data.Status != lobbyrepo.StatusStarted {
		return &GetMyActiveLobbyOutput{LobbyID: data.ID, Status: data.Status}, nil
	}

	// STARTED: the lobby record never learns when its session later ends
	// (StartEncounter's Save is the last write a STARTED lobby ever gets),
	// so liveness has to be checked against the session's own current state
	// before trusting encounter_id. A STARTED lobby with no encounter_id at
	// all would be a StartEncounter bug (it always sets both together) —
	// treated the same as "not live" rather than as a hard error, since the
	// caller-visible outcome (nothing to resume) is identical either way.
	if data.EncounterID == "" {
		return &GetMyActiveLobbyOutput{}, nil
	}
	status, err := o.sessionManager.Status(ctx, &sdk.StatusInput{Session: data.EncounterID})
	if err != nil {
		if errors.Is(err, sdk.ErrNoSession) || errors.Is(err, sdk.ErrNoEncounter) {
			return &GetMyActiveLobbyOutput{}, nil
		}
		return nil, fmt.Errorf("check session %q liveness: %w", data.EncounterID, err)
	}
	if !status.Open {
		return &GetMyActiveLobbyOutput{}, nil
	}

	return &GetMyActiveLobbyOutput{
		LobbyID:     data.ID,
		EncounterID: data.EncounterID,
		Status:      data.Status,
	}, nil
}
