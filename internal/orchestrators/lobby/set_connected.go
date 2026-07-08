package lobby

import (
	"context"
	"errors"
	"fmt"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// SetConnectedInput carries a StreamLobby subscribe/disconnect presence
// flip. Not an RPC of its own — the handler's StreamLobby calls this at
// subscribe (Connected: true) and again via defer at stream teardown
// (Connected: false).
type SetConnectedInput struct {
	PlayerID  string
	LobbyID   string
	Connected bool
}

// SetConnectedOutput carries the lobby's full roster, so StreamLobby's
// subscribe call can build its initial Snapshot event from the same load
// this method already did (no second Get needed).
type SetConnectedOutput struct {
	Members []*lobbyrepo.Member
}

// SetConnected flips PlayerID's presence flag and broadcasts
// MemberConnectionChanged. This is presence, NOT membership
// (lobby-surface.md "Presence") — disconnect never removes a seat, only
// explicit LeaveLobby does.
func (o *Orchestrator) SetConnected(ctx context.Context, in *SetConnectedInput) (*SetConnectedOutput, error) {
	if in == nil {
		return nil, errors.New("lobby orchestrator: SetConnectedInput is required")
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
	member, ok := data.Members[in.PlayerID]
	if !ok {
		return nil, ErrPlayerNotInLobby
	}

	member.IsConnected = in.Connected

	if err := o.lobbyRepo.Save(ctx, data); err != nil {
		return nil, fmt.Errorf("save lobby %q: %w", in.LobbyID, err)
	}

	o.lobbyBroker.Publish(in.LobbyID, &Event{
		Kind: EventKindMemberConnectionChanged,
		MemberConnectionChanged: &MemberConnectionChangedPayload{
			PlayerID: in.PlayerID, Connected: in.Connected,
		},
	})

	return &SetConnectedOutput{Members: orderedMembers(data)}, nil
}
