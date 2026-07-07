package lobby

import (
	"context"
	"errors"
	"fmt"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// JoinLobbyInput carries the entity-typed JoinLobby request.
type JoinLobbyInput struct {
	// PlayerID is the authenticated player joining.
	PlayerID string

	// JoinRef is the opaque ref minted by CreateLobby. JoinLobby is the only
	// RPC that addresses a lobby by ref instead of lobby_id.
	JoinRef string

	// CharacterID is the character to bind. Ownership is validated against
	// PlayerID.
	CharacterID string
}

// JoinLobbyOutput carries the lobby's identity and full roster after the
// join (or rebind) lands.
type JoinLobbyOutput struct {
	LobbyID string
	Members []*lobbyrepo.Member
}

// JoinLobby adds PlayerID to the lobby identified by JoinRef, or — if
// PlayerID is already a member — rebinds their CharacterID in place.
//
// JoinLobby is deliberately idempotent (lobby-surface.md "Contract edge
// cases"): a reconnect retry, a second tab, or picking a different character
// pre-start all land here rather than erroring. This is also the only
// character re-select path — there is no separate RPC for it.
func (o *Orchestrator) JoinLobby(ctx context.Context, in *JoinLobbyInput) (*JoinLobbyOutput, error) {
	if in == nil {
		return nil, errors.New("lobby orchestrator: JoinLobbyInput is required")
	}

	data, err := o.lobbyRepo.GetByJoinRef(ctx, in.JoinRef)
	if err != nil {
		if errors.Is(err, lobbyrepo.ErrNotFound) {
			return nil, ErrLobbyNotFound
		}
		return nil, fmt.Errorf("load lobby by join_ref %q: %w", in.JoinRef, err)
	}

	unlock := o.locks.Lock(data.ID)
	defer unlock()

	// Re-Get under the lock: the join_ref lookup above ran unlocked, so a
	// concurrent mutation could have landed between it and here.
	data, err = o.lobbyRepo.Get(ctx, data.ID)
	if err != nil {
		if errors.Is(err, lobbyrepo.ErrNotFound) {
			return nil, ErrLobbyNotFound
		}
		return nil, fmt.Errorf("load lobby %q: %w", data.ID, err)
	}

	if data.Status == lobbyrepo.StatusStarted {
		// Late join (lobby-surface.md "Late join"): mid-encounter join is a
		// future encounter-side concern, not a lobby one.
		return nil, ErrLobbyAlreadyStarted
	}

	characterName, err := o.resolveCharacter(ctx, in.PlayerID, in.CharacterID)
	if err != nil {
		return nil, err
	}

	existing, isRebind := data.Members[in.PlayerID]
	if isRebind {
		existing.CharacterID = in.CharacterID
		existing.CharacterName = characterName
	} else {
		if len(data.Members) >= o.partyCap {
			return nil, ErrLobbyFull
		}
		data.Members[in.PlayerID] = &lobbyrepo.Member{
			PlayerID:      in.PlayerID,
			CharacterID:   in.CharacterID,
			CharacterName: characterName,
		}
		data.MemberOrder = append(data.MemberOrder, in.PlayerID)
	}

	if err := o.lobbyRepo.Save(ctx, data); err != nil {
		return nil, fmt.Errorf("save lobby %q: %w", data.ID, err)
	}

	// MemberJoined doubles as the roster-upsert event for both a genuinely
	// new member and a rebind: the payload carries the full current Member
	// state, so a receiving client just upserts its roster by player_id
	// either way. lobby-surface.md's event vocabulary has no separate
	// "rebound" event — this is the only fire-and-forget signal a character
	// re-select needs.
	o.lobbyBroker.Publish(data.ID, &Event{
		Kind:         EventKindMemberJoined,
		MemberJoined: data.Members[in.PlayerID],
	})

	return &JoinLobbyOutput{
		LobbyID: data.ID,
		Members: orderedMembers(data),
	}, nil
}
