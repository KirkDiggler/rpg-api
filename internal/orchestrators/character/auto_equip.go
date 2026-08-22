package character

import (
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/equipment"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// primaryWeaponChoiceIDs names every class's "which weapon(s) do you carry"
// equipment choice, spelled out from the toolkit's own exported ChoiceID
// constants (character/choices/choice_ids.go) rather than pattern-matched:
// 11 of 12 classes name theirs "<Class>WeaponsPrimary", but Cleric's is
// "cleric-weapons" (no Primary suffix), so a suffix match would silently
// skip Clerics. If the toolkit adds a 13th class this map does not follow
// automatically -- there is no exported way to enumerate it from outside
// the choices package (getXEquipmentRequirements is unexported) -- so a new
// class's fighter-shaped bug (chosen weapon never equipped) would need a
// line added here. Acceptable for an interim fix; see rpg-api#746.
var primaryWeaponChoiceIDs = map[choices.ChoiceID]bool{
	choices.FighterWeaponsPrimary:   true,
	choices.BarbarianWeaponsPrimary: true,
	choices.RogueWeaponsPrimary:     true,
	choices.WizardWeaponsPrimary:    true,
	choices.ClericWeapons:           true,
	choices.BardWeaponsPrimary:      true,
	choices.DruidWeaponsPrimary:     true,
	choices.MonkWeaponsPrimary:      true,
	choices.PaladinWeaponsPrimary:   true,
	choices.RangerWeaponsPrimary:    true,
	choices.SorcererWeaponsPrimary:  true,
	choices.WarlockWeaponsPrimary:   true,
}

// armorChoiceIDs names the classes that offer a distinct body-armor
// equipment choice today -- only Fighter, Cleric and Ranger
// (character/choices/choice_ids.go); every other class either wears no
// armor by class design (casters, Monk, Barbarian's Unarmored Defense) or
// has no separate armor pick to auto-equip from.
var armorChoiceIDs = map[choices.ChoiceID]bool{
	choices.FighterArmor: true,
	choices.ClericArmor:  true,
	choices.RangerArmor:  true,
}

// autoEquipChosenGear puts a freshly finalized character's primary-weapon
// and (where the class has one) armor picks into their slots -- see
// rpg-api#746. Before this, FinalizeDraft never called EquipItem at all:
// only cmd/devseed's hand-written fixtures set EquipmentSlots directly in
// Go, so every character created through the real API (draft -> finalize,
// the only path a player uses) finalized with nothing equipped and fought
// unarmed regardless of what was chosen.
//
// Deliberately narrow, matching the issue's own interim-fix framing: only
// the class's declared PRIMARY weapon choice and its distinct ARMOR choice
// (if any) are auto-equipped. A shield bundled into the same weapon choice
// (Fighter's "martial weapon + shield" option) is left alone on purpose --
// EquipItem is still the one path that equips a shield (see
// cmd/sandboxseed), and this function is idempotent with it: a shield left
// unequipped here still equips cleanly into the off hand afterward. The
// fuller design -- gearing up in the lobby, writing these same
// equipment_slots from a player's own choice of what to carry -- is
// separate, deferred work (see the issue).
//
// No slot legality is decided here. Every placement goes through
// char.EquipItem, the exact call EquipItem (the RPC) and the toolkit's own
// occupancy rules (two-handed clears/blocks the off hand, an incompatible
// slot is refused) already run through -- see character.CompatibleSlots's
// doc: "rpg-api must not reconstruct this rule itself." This function only
// decides WHICH choice's items are candidates and which slot to try first;
// a refusal from EquipItem (item doesn't fit, e.g. a resolved id that turns
// out to not be equipment) is swallowed rather than failing finalize --
// leaving that one slot unequipped is a strictly better failure mode than
// blocking character creation over a gearing detail.
func autoEquipChosenGear(draftChoices []choices.ChoiceData, char *character.Character) {
	for _, choice := range draftChoices {
		if choice.Category != shared.ChoiceEquipment {
			continue
		}
		switch {
		case primaryWeaponChoiceIDs[choice.ChoiceID]:
			equipChosenWeapons(choice.EquipmentSelection, char)
		case armorChoiceIDs[choice.ChoiceID]:
			equipChosenArmor(choice.EquipmentSelection, char)
		}
	}
}

// equipChosenWeapons equips the weapon-typed items among a primary-weapon
// choice's resolved selection: the first into the main hand (a two-handed
// weapon claims it alone -- EquipItem's own rule), a second ONE-HANDED
// weapon into the off hand if nothing has claimed it yet. Non-weapon items
// resolved from the same choice (a bundled shield) are skipped -- see this
// file's package doc comment for why.
func equipChosenWeapons(itemIDs []shared.SelectionID, char *character.Character) {
	mainHandSet := false
	for _, id := range itemIDs {
		item, err := equipment.GetByID(id)
		if err != nil || !isWeaponCandidate(item) {
			continue
		}
		if !mainHandSet {
			if err := char.EquipItem(character.SlotMainHand, id); err == nil {
				mainHandSet = true
			}
			continue
		}
		// A second weapon: try the off hand. EquipItem refuses this on its
		// own if the main hand ended up holding a two-handed weapon, or if
		// this item cannot go there (equipmentFitsSlot) -- nothing here
		// re-decides that.
		_ = char.EquipItem(character.SlotOffHand, id)
	}
}

// equipChosenArmor equips the first body-armor item (never a shield) among
// an armor choice's resolved selection into the armor slot.
func equipChosenArmor(itemIDs []shared.SelectionID, char *character.Character) {
	for _, id := range itemIDs {
		item, err := equipment.GetByID(id)
		if err != nil || !isArmorCandidate(item) {
			continue
		}
		if err := char.EquipItem(character.SlotArmor, id); err == nil {
			return
		}
	}
}

// isWeaponCandidate reports whether item is a weapon (fits the main hand) --
// true for both one- and two-handed weapons, false for a shield (off hand
// only) or body armor (armor only). Reads the answer off
// character.CompatibleSlots, the toolkit's single source of truth for slot
// fit, rather than inspecting item properties directly.
func isWeaponCandidate(item equipment.Equipment) bool {
	return slotsInclude(character.CompatibleSlots(item), character.SlotMainHand)
}

// isArmorCandidate reports whether item is body armor (fits the armor
// slot) -- false for a shield, which fits the off hand instead. See
// isWeaponCandidate.
func isArmorCandidate(item equipment.Equipment) bool {
	return slotsInclude(character.CompatibleSlots(item), character.SlotArmor)
}

func slotsInclude(slots []character.InventorySlot, target character.InventorySlot) bool {
	for _, s := range slots {
		if s == target {
			return true
		}
	}
	return false
}
