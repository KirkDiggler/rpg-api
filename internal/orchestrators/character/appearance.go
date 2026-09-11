package character

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
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

// equipmentWriteInput is one attempt at writing a character's equipment slots.
type equipmentWriteInput struct {
	CharacterID string
	// Slot is carried for the log alone — it says which slot moved when a
	// notification fails, and decides nothing.
	Slot       character.InventorySlot
	Current    *characterrepo.GetOutput
	Slots      character.EquipmentSlots
	ArmorClass int
}

// writeEquipment persists one equipment change and, WHEN AND ONLY WHEN the
// write lands, tells whoever can see this character that their view is stale.
//
// # Why the notification lives here and not in the verbs
//
// It used to live in EquipItem, and UnequipItem did not have it — so putting a
// weapon away was invisible to a watching peer until somebody took a step,
// while drawing one appeared instantly. Kirk found that in the first walk.
//
// The bug was not the missing line. It was that "the sheet changed" and
// "watchers are told" were two facts kept in step by hand, in two verbs that
// are otherwise byte-identical here. A third writer — a swap, a disarm, loot
// landing in a hand — would have been the same mistake a third time. So the
// two facts are now one: this is the only place equipment is written, and
// nothing can write it without telling.
//
// # Returns
//
//   - (patch, nil, nil)   the write landed, and watchers have been told
//   - (nil, current, nil) a version race; caller should re-read and retry.
//     NOTHING was written, so nobody is told.
//   - (nil, nil, err)     the write failed
//
// A FAILING NOTIFICATION DOES NOT FAIL THE WRITE. The sheet is durable by
// then; returning an error would tell the client its equip failed and invite a
// retry that writes again. The cost instead is that watchers keep the picture
// they had until the next sight refresh — which is where this whole thing
// started, so it degrades to yesterday rather than to broken. Logged, never
// swallowed.
func (o *Orchestrator) writeEquipment(
	ctx context.Context, in *equipmentWriteInput,
) (*characterrepo.PatchEquipmentOutput, *characterrepo.GetOutput, error) {
	patch, err := o.characterRepo.PatchEquipment(ctx, characterrepo.PatchEquipmentInput{
		CharacterID:            in.CharacterID,
		ExpectedVersion:        in.Current.Version,
		ExpectedEquipmentSlots: maps.Clone(in.Current.Character.Data.EquipmentSlots),
		EquipmentSlots:         in.Slots,
		ArmorClass:             in.ArmorClass,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to patch character equipment: %w", err)
	}
	if patch == nil || patch.Character == nil || patch.Character.Data == nil {
		return nil, nil, fmt.Errorf(
			"failed to patch character equipment: repository returned no character data")
	}
	if !patch.Applied {
		return nil, &characterrepo.GetOutput{Character: patch.Character, Version: patch.Version}, nil
	}

	if nerr := o.notifyAppearance(ctx, patch.Character.Data, in.CharacterID); nerr != nil {
		slog.WarnContext(ctx, "character: watchers not told of an equipment change",
			"character_id", in.CharacterID, "slot", in.Slot, "error", nerr)
	}

	return patch, nil, nil
}
