package character

import (
	"context"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// AppearanceChangedInput names the character whose look changed, and the
// player who owns them.
//
// BOTH IDS, BECAUSE THE CALLER ALREADY HAS BOTH. A notifier given only the
// character would have to read the character store back to find its owner,
// and the orchestrator calling it has just loaded that record.
type AppearanceChangedInput struct {
	// PlayerID owns the character. It is how a live encounter is found.
	PlayerID string
	// CharacterID is the character, and is also the member id it has inside
	// an encounter (the lobby seats members under their character id).
	CharacterID string
}

// AppearanceNotifier is told that something an observer could SEE about a
// character has changed.
//
// # Why this is a capability and not a call
//
// A character does not know about encounters, and must not learn. Equipment
// is written here because the SHEET lives here; whether that character is
// currently standing in a dungeon with somebody watching them is a different
// concern entirely, owned somewhere else. So this package states what it needs
// — somebody told when a look changes — and is handed an implementation that
// knows where to take it (rpg-toolkit#1033: capabilities are supplied, never
// defaulted; refused at the door, never guarded at the use site).
//
// # It names who, never what
//
// There is deliberately nowhere here to say a longsword was put away. What a
// watcher may now perceive is their own testimony to re-read, which is the
// only shape in which one observer's picture can disagree with the truth —
// and a game whose engine cannot lie can never have an illusion in it.
//
// # A no-op is the ordinary case
//
// Most characters whose gear changes are not in an encounter at all. An
// implementation finding no live encounter for the player has nothing to do
// and says so by returning nil. That is not an error and must not be reported
// as one.
type AppearanceNotifier interface {
	AppearanceChanged(ctx context.Context, in *AppearanceChangedInput) error
}

// notifyAppearance tells the supplied notifier that this character's look
// changed, if the record says who owns them.
//
// A CHARACTER WITH NO OWNER IS NOT AN ERROR HERE. Some stored records carry no
// PlayerID — seeded content, older rows — and the notifier's whole job is to
// find that player's live encounter. There is nobody to look up, so there is
// nothing to tell, and saying so loudly would turn a harmless gap in old data
// into a warning on every equip.
func (o *Orchestrator) notifyAppearance(
	ctx context.Context, data *character.Data, characterID string,
) error {
	if data == nil || data.PlayerID == "" {
		return nil
	}

	return o.appearance.AppearanceChanged(ctx, &AppearanceChangedInput{
		PlayerID:    data.PlayerID,
		CharacterID: characterID,
	})
}

// NoAppearanceNotifier tells nobody, and is the honest answer for a caller
// that has no encounters to tell.
//
// EXPORTED SO IT MUST BE CHOSEN. The capability is required at construction
// precisely so that "nobody is ever told" is a decision somebody wrote down
// rather than a nil nobody noticed (rpg-toolkit#1033). A deployment with no
// live sessions, and a test that is not about who was told, both say so by
// passing this.
type NoAppearanceNotifier struct{}

// AppearanceChanged does nothing and succeeds.
func (NoAppearanceNotifier) AppearanceChanged(_ context.Context, _ *AppearanceChangedInput) error {
	return nil
}
