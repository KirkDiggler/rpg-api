package lobby

import (
	"context"
	"errors"
	"fmt"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// SetReadyInput carries the entity-typed SetReady request.
type SetReadyInput struct {
	PlayerID string
	LobbyID  string
	Ready    bool
}

// SetReadyOutput is the lean result of a readiness toggle. Clients update
// from the broadcast MemberReady event, not this response.
type SetReadyOutput struct{}

// SetReady toggles PlayerID's readiness flag and broadcasts the change.
func (o *Orchestrator) SetReady(ctx context.Context, in *SetReadyInput) (*SetReadyOutput, error) {
	if in == nil {
		return nil, errors.New("lobby orchestrator: SetReadyInput is required")
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
	member, ok := data.Members[in.PlayerID]
	if !ok {
		return nil, ErrPlayerNotInLobby
	}

	member.IsReady = in.Ready

	if err := o.lobbyRepo.Save(ctx, data); err != nil {
		return nil, fmt.Errorf("save lobby %q: %w", in.LobbyID, err)
	}

	o.lobbyBroker.Publish(in.LobbyID, &Event{
		Kind:        EventKindMemberReady,
		MemberReady: &MemberReadyPayload{PlayerID: in.PlayerID, Ready: in.Ready},
	})

	return &SetReadyOutput{}, nil
}
