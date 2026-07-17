package lobby

import (
	"context"
	"errors"
	"fmt"

	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
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
// encounter has been verified still live (exists, Mode != ModeEnded). A
// STARTED lobby whose encounter is not live has nothing left for a client
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

	if data.Status != lobbyrepo.StatusStarted {
		return &GetMyActiveLobbyOutput{LobbyID: data.ID, Status: data.Status}, nil
	}

	// STARTED: the lobby record never learns when its encounter later ends
	// (see Repository's doc comment — StartEncounter's Save is the last
	// write a STARTED lobby ever gets), so liveness has to be checked
	// against the encounter's own current state before trusting
	// encounter_id. A STARTED lobby with no encounter_id at all would be a
	// StartEncounter bug (it always sets both together) — treated the same
	// as "not live" rather than as a hard error, since the caller-visible
	// outcome (nothing to resume) is identical either way.
	if data.EncounterID == "" {
		return &GetMyActiveLobbyOutput{}, nil
	}
	encData, err := o.encounterRepo.Get(ctx, data.EncounterID)
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return &GetMyActiveLobbyOutput{}, nil
		}
		return nil, fmt.Errorf("load encounter %q for liveness check: %w", data.EncounterID, err)
	}
	if encData.Mode == core.ModeEnded {
		return &GetMyActiveLobbyOutput{}, nil
	}

	return &GetMyActiveLobbyOutput{
		LobbyID:     data.ID,
		EncounterID: data.EncounterID,
		Status:      data.Status,
	}, nil
}
