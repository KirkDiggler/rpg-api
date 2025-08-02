package v1alpha1

import (
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// convertPresentationClassToProto converts orchestrator presentation class to proto ClassInfo
// This is a simple mapping function with NO business logic
func convertPresentationClassToProto(pc *character.PresentationClass) *dnd5ev1alpha1.ClassInfo {
	if pc == nil {
		return nil
	}

	info := &dnd5ev1alpha1.ClassInfo{
		Id:          pc.ID,
		Name:        pc.Name,
		Description: pc.Description,
		HitDie:      pc.HitDie,
	}

	// Map simple string arrays
	info.SavingThrowProficiencies = pc.SavingThrows
	info.PrimaryAbilities = pc.SavingThrows // TODO: Get actual primary abilities when available
	info.WeaponProficiencies = pc.WeaponProficiencies
	info.ArmorProficiencies = pc.ArmorProficiencies
	info.AvailableSkills = pc.AvailableSkills
	info.SkillChoicesCount = int32(pc.SkillChoicesCount)

	// Convert choices
	info.Choices = make([]*dnd5ev1alpha1.Choice, 0, len(pc.Choices))
	for _, choice := range pc.Choices {
		info.Choices = append(info.Choices, convertPresentationChoiceToProto(choice))
	}

	return info
}

// convertPresentationChoiceToProto converts presentation choice to proto Choice
func convertPresentationChoiceToProto(pc character.PresentationChoice) *dnd5ev1alpha1.Choice {
	protoChoice := &dnd5ev1alpha1.Choice{
		Id:          pc.ID,
		Description: pc.Description,
		ChooseCount: int32(pc.ChooseCount),
		ChoiceType:  convertChoiceCategoryToProto(pc.Category),
	}

	// Convert options
	options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(pc.Options))
	for _, opt := range pc.Options {
		options = append(options, convertPresentationOptionToProto(opt))
	}

	protoChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
		ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
			Options: options,
		},
	}

	return protoChoice
}

// convertPresentationOptionToProto converts presentation option to proto ChoiceOption
func convertPresentationOptionToProto(po character.PresentationOption) *dnd5ev1alpha1.ChoiceOption {
	switch po.Type {
	case character.OptionTypeSingleItem:
		if po.Item != nil {
			return &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
					Item: &dnd5ev1alpha1.ItemReference{
						ItemId: po.Item.ItemID,
						Name:   po.Item.Name,
					},
				},
			}
		}
	case character.OptionTypeCountedItem:
		if po.Item != nil {
			return &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_CountedItem{
					CountedItem: &dnd5ev1alpha1.CountedItemReference{
						ItemId:   po.Item.ItemID,
						Name:     po.Item.Name,
						Quantity: int32(po.Item.Quantity),
					},
				},
			}
		}
	case character.OptionTypeBundle:
		if po.Bundle != nil {
			return &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_Bundle{
					Bundle: convertPresentationBundleToProto(po.Bundle),
				},
			}
		}
	}
	return nil
}

// convertPresentationBundleToProto converts presentation bundle to proto ItemBundle
func convertPresentationBundleToProto(pb *character.PresentationBundle) *dnd5ev1alpha1.ItemBundle {
	if pb == nil {
		return nil
	}

	bundle := &dnd5ev1alpha1.ItemBundle{
		Items: make([]*dnd5ev1alpha1.BundleItem, 0, len(pb.Items)),
	}

	for _, item := range pb.Items {
		bundleItem := convertPresentationBundleItemToProto(item)
		if bundleItem != nil {
			bundle.Items = append(bundle.Items, bundleItem)
		}
	}

	return bundle
}

// convertPresentationBundleItemToProto converts presentation bundle item to proto BundleItem
func convertPresentationBundleItemToProto(pbi character.PresentationBundleItem) *dnd5ev1alpha1.BundleItem {
	switch pbi.Type {
	case character.BundleItemTypeItem:
		if pbi.Item != nil {
			return &dnd5ev1alpha1.BundleItem{
				ItemType: &dnd5ev1alpha1.BundleItem_ConcreteItem{
					ConcreteItem: &dnd5ev1alpha1.CountedItemReference{
						ItemId:   pbi.Item.ItemID,
						Name:     pbi.Item.Name,
						Quantity: int32(pbi.Item.Quantity),
					},
				},
			}
		}
	case character.BundleItemTypeChoice:
		if pbi.NestedChoice != nil {
			return &dnd5ev1alpha1.BundleItem{
				ItemType: &dnd5ev1alpha1.BundleItem_ChoiceItem{
					ChoiceItem: &dnd5ev1alpha1.NestedChoice{
						Choice: convertPresentationChoiceToProto(*pbi.NestedChoice),
					},
				},
			}
		}
	}
	return nil
}

// convertChoiceCategoryToProto converts toolkit ChoiceCategory to proto
func convertChoiceCategoryToProto(category shared.ChoiceCategory) dnd5ev1alpha1.ChoiceCategory {
	switch category {
	case shared.ChoiceEquipment:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT
	case shared.ChoiceSkills:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS
	case shared.ChoiceLanguages:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES
	case shared.ChoiceSpells:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS
	case shared.ChoiceAbilityScores:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_ABILITY_SCORES
	case shared.ChoiceName:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_NAME
	case shared.ChoiceFightingStyle:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE
	case shared.ChoiceRace:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_RACE
	case shared.ChoiceClass:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CLASS
	case shared.ChoiceBackground:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_BACKGROUND
	case shared.ChoiceCantrips:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS
	default:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_UNSPECIFIED
	}
}
