package character

// character_data.go composes the equipment-facing CharacterData fields
// (equipped/inventory/slots/armor_class_detail/main_hand_damage) from the
// toolkit's EquipmentView display projection (rpg-toolkit#812) — the
// rpg-api#680 fix for both of board #11's named equipment sins: the equip
// path bypassing the rules engine (fixed in the orchestrator, see
// internal/orchestrators/character/orchestrator.go) and AC on the wire
// being a straight copy of a stored int instead of EffectiveAC. Every
// field composed here is a pass-through or Ref-translation of a
// toolkit-owned value — no rules are computed in rpg-api.
//
// Moved from the now-deleted internal/handlers/dnd5e/v2/encounter package
// (the old EncounterService, rpg-project#227) into this, its one remaining
// caller: recomputedCharacterData in handler.go.

import (
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// refModuleDnd5e is the canonical module string for the dnd5e rulebook,
// mirroring the (now-deleted) v2 encounter handler's own constant of the
// same name.
const refModuleDnd5e = "dnd5e"

// BuildEquipmentCharacterData composes the equipment-facing fields of
// CharacterData from a toolkit EquipmentView. nil view returns a zero-value
// CharacterData (callers merge onto identity fields already populated, e.g.
// by recomputedCharacterData).
func BuildEquipmentCharacterData(view *tkcharacter.EquipmentView) *encounterv2pb.CharacterData {
	cd := &encounterv2pb.CharacterData{}
	if view == nil {
		return cd
	}

	equipped := make(map[string]*encounterv2pb.Ref, len(view.Items))
	inventory := make([]*encounterv2pb.Item, 0, len(view.Items))
	for _, item := range view.Items {
		ref := &encounterv2pb.Ref{Module: refModuleDnd5e, Type: "item", Id: item.ItemID}
		inventory = append(inventory, &encounterv2pb.Item{
			Ref:      ref,
			Name:     item.Name,
			StatLine: item.StatLine,
			Kind:     item.Kind,
			SlotKeys: item.SlotKeys,
			// IconKey intentionally left empty (rpg-api#680 Scope-decision):
			// no toolkit/asset-manifest source exists yet for a bare sprite
			// key — the fixture data rpg-dnd5e-web#557 ships is a full sprite
			// path, not a key. Composing one in rpg-api would be exactly the
			// kind of display-field invention this slice exists to stop.
		})
		if item.Slot != "" {
			equipped[string(item.Slot)] = ref
		}
	}
	cd.Equipped = equipped
	cd.Inventory = inventory

	slots := make([]*encounterv2pb.SlotDef, 0, len(view.Slots))
	for _, s := range view.Slots {
		slots = append(slots, &encounterv2pb.SlotDef{
			Key:          s.Key,
			DisplayLabel: s.DisplayLabel,
			Accepts:      s.Accepts,
		})
	}
	cd.Slots = slots

	cd.ArmorClassDetail = &encounterv2pb.ArmorClassDisplay{
		Total: int32(view.ACTotal),
		Note:  view.ACNote,
	}
	cd.MainHandDamage = view.MainHandDamage

	return cd
}
