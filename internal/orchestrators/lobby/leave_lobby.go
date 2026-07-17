package lobby

import (
	"context"
	"errors"
	"fmt"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// LeaveLobbyInput carries the entity-typed LeaveLobby request.
type LeaveLobbyInput struct {
	PlayerID string
	LobbyID  string
}

// LeaveLobbyOutput is the lean result of a leave. Clients update from the
// broadcast MemberLeft (+ optional HostChanged) events.
type LeaveLobbyOutput struct{}

// LeaveLobby removes PlayerID's membership. Pre-start only — a lobby that
// has already started is a terminal WAITING -> STARTED transition
// (lobby-surface.md "Lifecycle"), so there is no membership left to leave.
//
// Disconnect is NOT LeaveLobby (lobby-surface.md "Presence"): a dropped
// StreamLobby subscription flips is_connected via SetConnected and keeps the
// seat. Only this explicit call frees it.
//
// If the leaving member is host, the oldest remaining member (by join
// order) becomes host (lobby-surface.md "Host leaves") — dissolving the
// lobby would punish the rest of the party for one drop. If the departing
// member was the last one, the now-empty lobby is simply saved and left to
// expire via TTL (lobby-surface.md "Abandonment") — no active delete.
func (o *Orchestrator) LeaveLobby(ctx context.Context, in *LeaveLobbyInput) (*LeaveLobbyOutput, error) {
	if in == nil {
		return nil, errors.New("lobby orchestrator: LeaveLobbyInput is required")
	}

	unlock := o.locks.Lock(in.LobbyID)
	defer unlock()

	data, err := o.lobbyRepo.Get(ctx, in.LobbyID)
	if err != nil {
		if errors.Is(err, lobbyrepo.ErrNotFound) {
			return nil, ErrLobbyNotFound
		}
		return nil, fmt.Errorf("load lobby %q: %w", in.LobbyID, err)
	}
	if data.Status == lobbyrepo.StatusStarted {
		return nil, ErrLobbyAlreadyStarted
	}
	leaving, ok := data.Members[in.PlayerID]
	if !ok {
		return nil, ErrPlayerNotInLobby
	}

	delete(data.Members, in.PlayerID)
	data.MemberOrder = removeFromOrder(data.MemberOrder, in.PlayerID)

	var newHostID string
	if leaving.IsHost && len(data.MemberOrder) > 0 {
		newHostID = data.MemberOrder[0]
		data.Members[newHostID].IsHost = true
		data.HostPlayerID = newHostID
	}

	if err := o.lobbyRepo.Save(ctx, data); err != nil {
		return nil, fmt.Errorf("save lobby %q: %w", in.LobbyID, err)
	}
	// Save only adds/refreshes index entries for members still present in
	// data.Members — it cannot infer removal from a single Data value (see
	// Repository.Save's doc comment). The departing player's stale entry
	// must be cleared explicitly, or GetMyActiveLobby would keep resolving
	// them to a lobby they already left.
	if err := o.lobbyRepo.ClearPlayerIndex(ctx, in.PlayerID); err != nil {
		return nil, fmt.Errorf("clear player index for %q: %w", in.PlayerID, err)
	}

	o.lobbyBroker.Publish(in.LobbyID, &Event{
		Kind:       EventKindMemberLeft,
		MemberLeft: &MemberLeftPayload{PlayerID: in.PlayerID},
	})
	if newHostID != "" {
		o.lobbyBroker.Publish(in.LobbyID, &Event{
			Kind:        EventKindHostChanged,
			HostChanged: &HostChangedPayload{PlayerID: newHostID},
		})
	}

	return &LeaveLobbyOutput{}, nil
}

// removeFromOrder returns order with playerID removed, preserving the
// relative order of the rest.
func removeFromOrder(order []string, playerID string) []string {
	out := make([]string, 0, len(order))
	for _, id := range order {
		if id != playerID {
			out = append(out, id)
		}
	}
	return out
}
