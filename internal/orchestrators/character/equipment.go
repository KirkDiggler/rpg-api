package character

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// Equipment is the session SDK's equip verbs, the one equip path for every
// host surface (rpg-project#542). *sdk.Manager satisfies it.
//
// The verb takes the guard the character's seat decides, applies the
// rulebook's change, prices it on the member's turn in a fight, saves the
// record, tells the beat and rechecks sight. That is why this package keeps no
// equipment patch, no version check, no retry loop and no appearance
// notifier: each of those was a copy of something the verb now owns.
type Equipment interface {
	Equip(ctx context.Context, in *sdk.EquipInput) (*sdk.EquipOutput, error)
	Unequip(ctx context.Context, in *sdk.UnequipInput) (*sdk.UnequipOutput, error)
}

var _ Equipment = (*sdk.Manager)(nil)
