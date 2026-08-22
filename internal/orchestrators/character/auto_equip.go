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
// line added here. Filed as rpg-toolkit#1164 (an exported Role/Kind on
// EquipmentRequirement, or an exported per-class accessor, would let this
// map be derived instead of hand-copied); acceptable for this interim fix
// in the meantime. See rpg-api#746.
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
// has no separate armor pick to auto-equip from. See rpg-toolkit#1164.
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
// the class's declared PRIMARY weapon choice (main hand, plus off hand for
// a second one-handed weapon or a bundled shield -- see equipChosenWeapons)
// and its distinct ARMOR choice (if any) are auto-equipped. The fuller
// design -- gearing up in the lobby, writing these same equipment_slots
// from a player's own choice of what to carry -- is separate, deferred
// work (see the issue).
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

// equipChosenWeapons equips gear resolved from a primary-weapon choice: the
// first weapon into the main hand, then -- ONLY if that weapon is one-
// handed (character.CompatibleSlots says whether it also fits the off
// hand) -- whatever else the SAME choice granted to fill the off hand with:
// a second weapon (dual wield) if the choice resolved one, else a bundled
// shield (Fighter's "a martial weapon and a shield" option, where the
// shield rides in the same EquipmentSelection as the category-resolved
// weapon). Each hand is attempted AT MOST ONCE -- unlike an unbounded loop
// over every remaining candidate, which would let EquipItem's own
// swap-on-occupied rule silently move a THIRD candidate into the off hand
// and overwrite the second (caught in review on this PR).
//
// A two-handed main-hand pick claims the hand alone: CompatibleSlots
// excludes the off hand for it, and the off hand is never attempted in that
// case -- attempting it anyway would hit EquipItem's "main hand holding a
// two-handed weapon blocks the off hand until something is equipped there,
// which frees the main hand" rule and CLEAR the two-hander, exactly
// backwards from what should happen here.
func equipChosenWeapons(itemIDs []shared.SelectionID, char *character.Character) {
	var weaponIDs []shared.SelectionID
	var shieldID shared.SelectionID
	for _, id := range itemIDs {
		item, err := equipment.GetByID(id)
		if err != nil {
			continue
		}
		slots := character.CompatibleSlots(item)
		switch {
		case slotsInclude(slots, character.SlotMainHand):
			weaponIDs = append(weaponIDs, id)
		case shieldID == "" && slotsInclude(slots, character.SlotOffHand):
			shieldID = id
		}
	}
	if len(weaponIDs) == 0 {
		return
	}

	mainID := weaponIDs[0]
	if err := char.EquipItem(character.SlotMainHand, mainID); err != nil {
		return
	}

	mainItem, err := equipment.GetByID(mainID)
	if err != nil || !slotsInclude(character.CompatibleSlots(mainItem), character.SlotOffHand) {
		return // two-handed (or unresolvable): this choice has nothing more to place
	}

	if len(weaponIDs) > 1 {
		_ = char.EquipItem(character.SlotOffHand, weaponIDs[1])
		return
	}
	if shieldID != "" {
		_ = char.EquipItem(character.SlotOffHand, shieldID)
	}
}

// equipChosenArmor equips the first body-armor item (never a shield) among
// an armor choice's resolved selection into the armor slot.
func equipChosenArmor(itemIDs []shared.SelectionID, char *character.Character) {
	for _, id := range itemIDs {
		item, err := equipment.GetByID(id)
		if err != nil || !slotsInclude(character.CompatibleSlots(item), character.SlotArmor) {
			continue
		}
		if err := char.EquipItem(character.SlotArmor, id); err == nil {
			return
		}
	}
}

func slotsInclude(slots []character.InventorySlot, target character.InventorySlot) bool {
	for _, s := range slots {
		if s == target {
			return true
		}
	}
	return false
}
