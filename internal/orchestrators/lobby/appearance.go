package lobby

import (
	"context"
	"fmt"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	apierr "github.com/KirkDiggler/rpg-api/internal/apierr"
	charorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// AppearanceNotifier tells a player's live encounter that one of its members
// now looks different, so everyone watching them is told to look again.
//
// It lives beside the lobby because the lobby IS the index this needs and
// nothing else has one: a player is seated in at most one lobby
// (GetByPlayerID), and that lobby carries the EncounterID of the stack it
// started. The character orchestrator states the need as a capability and is
// handed this; it never learns that encounters exist.
//
// EQUIPITEM STAYS THE SINGLE WRITER. Nothing here writes equipment. The sheet
// is changed in one place and this tells the session afterwards.
type AppearanceNotifier struct {
	lobbies lobbyrepo.Repository
	session *sdk.Manager
}

// NewAppearanceNotifier builds the notifier from the two halves it needs.
func NewAppearanceNotifier(lobbies lobbyrepo.Repository, session *sdk.Manager) *AppearanceNotifier {
	return &AppearanceNotifier{lobbies: lobbies, session: session}
}

// AppearanceChanged finds the player's live encounter, if any, and tells it.
//
// NO LIVE ENCOUNTER IS THE ORDINARY CASE, and it returns nil. Most gear
// changes happen with nobody watching — in the builder, between runs, in a
// lobby that has not started yet. There is nothing to tell and nothing went
// wrong, so this must not be reported as a failure.
func (n *AppearanceNotifier) AppearanceChanged(
	ctx context.Context, in *charorch.AppearanceChangedInput,
) error {
	if in == nil || in.PlayerID == "" || in.CharacterID == "" {
		return apierr.InvalidArgument("player ID and character ID are required")
	}

	data, err := n.lobbies.GetByPlayerID(ctx, in.PlayerID)
	if err != nil {
		if apierr.IsNotFound(err) {
			return nil // Not in a lobby. Nobody is watching.
		}
		return fmt.Errorf("find lobby for player %q: %w", in.PlayerID, err)
	}
	if data == nil || data.EncounterID == "" {
		return nil // In a lobby that has not started. Still nobody watching.
	}

	// The lobby starts its stack with Session and Encounter under ONE id and
	// seats each member under their CHARACTER id, so both arguments here are
	// read off what is already stored rather than derived.
	if _, err := n.session.Recheck(ctx, &sdk.RecheckInput{
		Session: data.EncounterID,
		Members: []string{in.CharacterID},
	}); err != nil {
		return fmt.Errorf("recheck %q in session %q: %w", in.CharacterID, data.EncounterID, err)
	}

	return nil
}
