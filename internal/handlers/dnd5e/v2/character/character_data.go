package character

// character_data.go is a pure field-for-field wire mapper for the detached
// owner-private character View. Rules and display composition remain in the
// toolkit's EquipmentView and StatusView.

import (
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	orchcharacter "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-toolkit/core"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
)

const (
	refModuleDnd5e = "dnd5e"
	refTypeItem    = "item"
)

// BuildCharacterData maps the complete detached owner-private View onto the
// shared CharacterData wire message. Spell slots, legacy class resources, and
// magic status have no source in View and therefore cannot be emitted here.
func BuildCharacterData(view *orchcharacter.View) *encounterv2pb.CharacterData {
	cd := &encounterv2pb.CharacterData{}
	if view == nil {
		return cd
	}

	mapEquipment(cd, view)
	mapStatus(cd, view)
	return cd
}

func mapEquipment(cd *encounterv2pb.CharacterData, view *orchcharacter.View) {
	if view.Equipment == nil {
		return
	}

	equipped := make(map[string]*encounterv2pb.Ref, len(view.Equipment.Items))
	inventory := make([]*encounterv2pb.Item, 0, len(view.Equipment.Items))
	for _, item := range view.Equipment.Items {
		ref := &encounterv2pb.Ref{Module: refModuleDnd5e, Type: refTypeItem, Id: item.ItemID}
		inventory = append(inventory, &encounterv2pb.Item{
			Ref:      ref,
			Name:     item.Name,
			StatLine: item.StatLine,
			Kind:     item.Kind,
			SlotKeys: item.SlotKeys,
		})
		if item.Slot != "" {
			equipped[string(item.Slot)] = ref
		}
	}
	cd.Equipped = equipped
	cd.Inventory = inventory

	slots := make([]*encounterv2pb.SlotDef, 0, len(view.Equipment.Slots))
	for _, slot := range view.Equipment.Slots {
		slots = append(slots, &encounterv2pb.SlotDef{
			Key:          slot.Key,
			DisplayLabel: slot.DisplayLabel,
			Accepts:      slot.Accepts,
		})
	}
	cd.Slots = slots
	cd.ArmorClassDetail = &encounterv2pb.ArmorClassDisplay{
		Total: int32(view.Equipment.ACTotal),
		Note:  view.Equipment.ACNote,
	}
	cd.MainHandDamage = view.Equipment.MainHandDamage
}

func mapStatus(cd *encounterv2pb.CharacterData, view *orchcharacter.View) {
	if view.Status == nil {
		return
	}

	cd.Level = int32(view.Status.Level)
	cd.HitPoints = &encounterv2pb.HitPoints{
		Current: int32(view.Status.HitPoints.Current),
		Max:     int32(view.Status.HitPoints.Maximum),
	}
	cd.BaseSpeedFeet = int32(view.Status.BaseSpeedFeet)

	cd.Features = make([]*encounterv2pb.FeatureView, 0, len(view.Status.Features))
	for _, feature := range view.Status.Features {
		cd.Features = append(cd.Features, &encounterv2pb.FeatureView{
			Ref:         refToProto(feature.Ref),
			Name:        feature.Name,
			Detail:      feature.Detail,
			ResourceKey: optionalResourceKey(feature.ResourceKey),
		})
	}

	cd.Conditions = make([]*encounterv2pb.ConditionView, 0, len(view.Status.Conditions))
	for _, condition := range view.Status.Conditions {
		cd.Conditions = append(cd.Conditions, &encounterv2pb.ConditionView{
			Ref:          refToProto(condition.Ref),
			Name:         condition.Name,
			Detail:       condition.Detail,
			SourceMember: cloneString(condition.SourceMember),
		})
	}

	cd.Resources = make([]*encounterv2pb.ResourceView, 0, len(view.Status.Resources))
	for _, resource := range view.Status.Resources {
		cd.Resources = append(cd.Resources, &encounterv2pb.ResourceView{
			Key:     string(resource.Key),
			Name:    resource.Name,
			Current: int32(resource.Current),
			Maximum: int32(resource.Maximum),
		})
	}
}

func refToProto(ref core.Ref) *encounterv2pb.Ref {
	return &encounterv2pb.Ref{Module: ref.Module, Type: ref.Type, Id: ref.ID}
}

func optionalResourceKey(value *coreResources.ResourceKey) *string {
	if value == nil {
		return nil
	}
	result := string(*value)
	return &result
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
