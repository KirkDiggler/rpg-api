package lobby

import (
	"context"
	"errors"
	"fmt"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// CreateLobbyInput carries the entity-typed CreateLobby request. The handler
// builds it after envelope validation (auth, non-empty campaign_id /
// character_id).
type CreateLobbyInput struct {
	// PlayerID is the authenticated player creating the lobby. Becomes the
	// lobby's host.
	PlayerID string

	// CampaignID is a validated-but-otherwise-opaque reference, carried
	// through for future campaign-scoping (mirrors CreateEncounterRequest's
	// campaign_id, unused today).
	CampaignID string

	// CharacterID is the character the host binds to their seat. Ownership
	// is validated against PlayerID.
	CharacterID string
}

// CreateLobbyOutput is the newly created lobby's identity: the lobby_id
// every other RPC addresses it by, and the join_ref other players supply to
// JoinLobby.
type CreateLobbyOutput struct {
	LobbyID      string
	JoinRef      string
	HostPlayerID string
}

// CreateLobby creates a new WAITING lobby with the caller as its sole member
// and host. No event is published — StreamLobby is snapshot-first, so no
// subscriber can exist yet for a lobby that didn't exist a moment ago.
func (o *Orchestrator) CreateLobby(ctx context.Context, in *CreateLobbyInput) (*CreateLobbyOutput, error) {
	if in == nil {
		return nil, errors.New("lobby orchestrator: CreateLobbyInput is required")
	}

	characterName, err := o.resolveCharacter(ctx, in.PlayerID, in.CharacterID)
	if err != nil {
		return nil, err
	}

	lobbyID := o.lobbyIDGen.Generate()
	joinRef := o.joinRefGen.Generate()

	data := &lobbyrepo.Data{
		ID:           lobbyID,
		JoinRef:      joinRef,
		CampaignID:   in.CampaignID,
		HostPlayerID: in.PlayerID,
		Status:       lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			in.PlayerID: {
				PlayerID:      in.PlayerID,
				CharacterID:   in.CharacterID,
				CharacterName: characterName,
				IsHost:        true,
			},
		},
		MemberOrder: []string{in.PlayerID},
	}

	if err := o.lobbyRepo.Save(ctx, data); err != nil {
		return nil, fmt.Errorf("save lobby %q: %w", lobbyID, err)
	}

	return &CreateLobbyOutput{
		LobbyID:      lobbyID,
		JoinRef:      joinRef,
		HostPlayerID: in.PlayerID,
	}, nil
}
