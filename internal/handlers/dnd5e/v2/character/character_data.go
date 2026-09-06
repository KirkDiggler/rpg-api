package character

// character_data.go is a pure field-for-field wire mapper for the detached
// owner-private character View. Rules and display composition remain in the
// toolkit's EquipmentView and StatusView.

import (
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/core"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/currency"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/equipment"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	orchcharacter "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
)

const (
	refModuleDnd5e = "dnd5e"
	refTypeClass   = "class"
	refTypeRace    = "race"
	refTypeItem    = "item"
)

// BuildCharacterData maps the complete detached owner-private View onto the
// shared CharacterData wire message. Spell slots, legacy class resources, and
// magic status have no source in View and therefore cannot be emitted here.
//
// Fallible only because mapEquipment is (see its own doc): a real toolkit
// lookup (equipment.PriceOf) can fail on a data defect, and design rule 8
// asks that this handler name that failure rather than silently emit a
// zero price.
func BuildCharacterData(view *orchcharacter.View) (*encounterv2pb.CharacterData, error) {
	cd := &encounterv2pb.CharacterData{}
	if view == nil {
		return cd, nil
	}

	mapIdentity(cd, view)
	if err := mapEquipment(cd, view); err != nil {
		return nil, err
	}
	mapStatus(cd, view)
	cd.Wallet = moneyToProto(view.Wallet)
	return cd, nil
}

func mapIdentity(cd *encounterv2pb.CharacterData, view *orchcharacter.View) {
	cd.PlayerId = view.Identity.PlayerID
	cd.ClassRef = &encounterv2pb.Ref{
		Module: refModuleDnd5e,
		Type:   refTypeClass,
		Id:     view.Identity.ClassID,
	}
	cd.RaceRef = &encounterv2pb.Ref{
		Module: refModuleDnd5e,
		Type:   refTypeRace,
		Id:     string(view.Identity.RaceID),
	}
}

// mapEquipment carries the equipment projection across, including each
// inventory item's unit price (rpg-api-protos#298) and equipment_type
// (rpg-api-protos#301): the price is a preview so a client can
// pre-populate a Sell's Receive.Currency before confirming, the
// inventory-side mirror of vendorStockEntryToProto's own Price field in the
// session handler -- never the price authority, the server always
// recomputes at trade time. equipment_type is the same open-vocabulary
// mirror of vendorStockEntryToProto's EquipmentType -- a coarser family
// than Kind (which collapses tool/pack/item/ammunition into one "gear"
// bucket), for a Sell UI grouping by category or reading Unpack's own pack
// contents. Fallible only for price's reason: equipment.PriceOf can fail
// on a catalog Cost string that doesn't parse (an ErrBadCost-class data
// defect), never on an unknown id, because every item here already
// resolved a Name/StatLine from that same catalog when the toolkit's own
// EquipmentView was built -- which is also why the second lookup for
// equipment_type (ResolveEquipmentDetail) needs no error path of its own.
func mapEquipment(cd *encounterv2pb.CharacterData, view *orchcharacter.View) error {
	if view.Equipment == nil {
		return nil
	}

	equipped := make(map[string]*encounterv2pb.Ref, len(view.Equipment.Equipped))
	for slot, itemID := range view.Equipment.Equipped {
		equipped[string(slot)] = &encounterv2pb.Ref{
			Module: refModuleDnd5e,
			Type:   refTypeItem,
			Id:     itemID,
		}
	}

	inventory := make([]*encounterv2pb.Item, 0, len(view.Equipment.Items))
	for _, item := range view.Equipment.Items {
		price, err := equipment.PriceOf(item.ItemID)
		if err != nil {
			return fmt.Errorf("inventory item %q: %w", item.ItemID, err)
		}
		// ResolveEquipmentDetail cannot return nil here: PriceOf just
		// resolved the same id against the same catalog and succeeded.
		detail := equipment.ResolveEquipmentDetail(item.ItemID)
		inventory = append(inventory, &encounterv2pb.Item{
			Ref:           &encounterv2pb.Ref{Module: refModuleDnd5e, Type: refTypeItem, Id: item.ItemID},
			Name:          item.Name,
			StatLine:      item.StatLine,
			Kind:          item.Kind,
			SlotKeys:      item.SlotKeys,
			Quantity:      int32(item.Quantity),
			Price:         moneyToProto(price),
			EquipmentType: string(detail.Type),
		})
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
	return nil
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
	cd.LifeState = ownerLifeStateToProto(view.Status.LifeState)
	cd.DeathSaves = ownerDeathSaveProgressToProto(view.Status.DeathSaves)

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

func ownerLifeStateToProto(state combat.LifeState) sessionpb.LifeState {
	switch state {
	case combat.LifeStateConscious:
		return sessionpb.LifeState_LIFE_STATE_CONSCIOUS
	case combat.LifeStateDying:
		return sessionpb.LifeState_LIFE_STATE_DYING
	case combat.LifeStateStabilized:
		return sessionpb.LifeState_LIFE_STATE_STABILIZED
	case combat.LifeStateDead:
		return sessionpb.LifeState_LIFE_STATE_DEAD
	case combat.LifeStateDefeated:
		return sessionpb.LifeState_LIFE_STATE_DEFEATED
	default:
		return sessionpb.LifeState_LIFE_STATE_UNSPECIFIED
	}
}

func ownerDeathSaveProgressToProto(progress *tkcharacter.DeathSaveProgress) *sessionpb.DeathSaveProgress {
	if progress == nil {
		return nil
	}
	return &sessionpb.DeathSaveProgress{
		Successes:         int32(progress.Successes),
		Failures:          int32(progress.Failures),
		SuccessesNeeded:   int32(progress.SuccessesNeeded),
		FailuresRemaining: int32(progress.FailuresRemaining),
		Stabilized:        progress.Stabilized,
		Dead:              progress.Dead,
	}
}

// moneyToProto mirrors a currency.Money amount onto the wire Money -- the
// same shape session.Trade prices with (dnd5e.api.session.v1alpha1.Money),
// reused here rather than a second Money type (both the wallet and item
// price wire fields' own doc comments say so directly). Used for the
// character's persistent purse (rpg-toolkit#1533) and each inventory item's
// unit price (rpg-api-protos#298).
func moneyToProto(m currency.Money) *sessionpb.Money {
	return &sessionpb.Money{Copper: int32(m.Copper)}
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
