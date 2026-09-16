package authoringv1alpha1

import (
	"context"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// ListWeapons reports every weapon the server's rulebook can arm a placed
// monster with, for the builder's action palette (rpg-project#448).
//
// # Verbatim, and that is the whole contract
//
// The catalog is CONTENT. The rulebook owns the names, the category words
// and the refs; a web client holding its own list of weapon names would need
// a release before an author could reach a weapon the rulebook already has,
// and the day the two lists disagreed the palette would offer a chip that
// writes a file the compiler refuses. So this file copies four values and
// maps nothing, and there is no second copy of a weapon's name, its category
// or its ref anywhere in rpg-api or in the web.
//
// It carries no dice, no range and no properties, because the palette does
// not compute an attack: the toolkit assembles one at spawn from the
// wielding monster's own ability scores and proficiency, which is why a
// skeleton shortbow and a goblin shortbow are the same weapon.
//
// UNGATED. Reading a catalog mutates nothing -- GetDungeon's precedent,
// carried through ListScenarios -- so unlike PutDungeon it makes no
// authoring-enabled refusal of its own. It is still a signed-in verb, like
// every RPC on this service.
func (h *Handler) ListWeapons(
	ctx context.Context, _ *authoringpb.ListWeaponsRequest,
) (*authoringpb.ListWeaponsResponse, error) {
	if err := requireAuthenticated(ctx); err != nil {
		return nil, err
	}

	out, err := h.orch.ListWeapons(ctx, &authoringorch.ListWeaponsInput{})
	if err != nil {
		return nil, statusError(err)
	}

	descriptors := make([]*authoringpb.WeaponDescriptor, len(out.Weapons))
	for i, w := range out.Weapons {
		descriptors[i] = weaponDescriptorToProto(w)
	}

	return &authoringpb.ListWeaponsResponse{Weapons: descriptors}, nil
}

// weaponDescriptorToProto mirrors one weapon onto the wire, field for field.
//
// `category` crosses as the rulebook's own word and `ranged` as the
// rulebook's own predicate. Neither is derived from the other here, and the
// bool is not read out of the word: the two are separate on the wire on
// purpose (see WeaponDescriptor in the proto), so a rulebook that spells a
// ranged category some other way keeps the bool true.
func weaponDescriptorToProto(w authoringorch.WeaponDescriptor) *authoringpb.WeaponDescriptor {
	return &authoringpb.WeaponDescriptor{
		Ref:      w.Ref,
		Name:     w.Name,
		Ranged:   w.Ranged,
		Category: w.Category,
	}
}
