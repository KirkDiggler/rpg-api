package v1alpha1

import (
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// convertChoiceToProto converts entity choice to proto choice with simple field mapping
func convertChoiceToProto(choice *dnd5e.Choice) *dnd5ev1alpha1.Choice {
	if choice == nil {
		return nil
	}

	protoChoice := &dnd5ev1alpha1.Choice{
		Id:          choice.ID,
		Description: choice.Description,
		ChooseCount: choice.ChooseCount,
		ChoiceType:  convertChoiceTypeToProto(choice.Type),
	}

	// Convert option set
	switch optSet := choice.OptionSet.(type) {
	case *dnd5e.ExplicitOptions:
		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
			ExplicitOptions: convertExplicitOptionsToProto(optSet),
		}
	case *dnd5e.CategoryReference:
		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_CategoryReference{
			CategoryReference: &dnd5ev1alpha1.CategoryReference{
				CategoryId: optSet.CategoryID,
				ExcludeIds: optSet.ExcludeIDs,
			},
		}
	}

	return protoChoice
}

// convertChoiceTypeToProto maps entity choice type to proto choice type
func convertChoiceTypeToProto(choiceType dnd5e.ChoiceType) dnd5ev1alpha1.ChoiceType {
	switch choiceType {
	case dnd5e.ChoiceTypeEquipment:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT
	case dnd5e.ChoiceTypeSkill:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SKILL
	case dnd5e.ChoiceTypeTool:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_TOOL
	case dnd5e.ChoiceTypeLanguage:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_LANGUAGE
	case dnd5e.ChoiceTypeWeaponProficiency:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_WEAPON_PROFICIENCY
	case dnd5e.ChoiceTypeArmorProficiency:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_ARMOR_PROFICIENCY
	case dnd5e.ChoiceTypeSpell:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SPELL
	case dnd5e.ChoiceTypeFeat:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_FEAT
	default:
		return dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_UNSPECIFIED
	}
}

// convertExplicitOptionsToProto converts entity explicit options to proto
func convertExplicitOptionsToProto(options *dnd5e.ExplicitOptions) *dnd5ev1alpha1.ExplicitOptions {
	if options == nil {
		return nil
	}

	protoOptions := make([]*dnd5ev1alpha1.ChoiceOption, len(options.Options))
	for i, opt := range options.Options {
		protoOptions[i] = convertChoiceOptionToProto(opt)
	}

	return &dnd5ev1alpha1.ExplicitOptions{
		Options: protoOptions,
	}
}

// convertChoiceOptionToProto converts entity choice option to proto
func convertChoiceOptionToProto(option dnd5e.ChoiceOption) *dnd5ev1alpha1.ChoiceOption {
	if option == nil {
		return nil
	}

	switch opt := option.(type) {
	case *dnd5e.ItemReference:
		return &dnd5ev1alpha1.ChoiceOption{
			OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
				Item: &dnd5ev1alpha1.ItemReference{
					ItemId: opt.ItemID,
					Name:   opt.Name,
				},
			},
		}
	case *dnd5e.CountedItemReference:
		return &dnd5ev1alpha1.ChoiceOption{
			OptionType: &dnd5ev1alpha1.ChoiceOption_CountedItem{
				CountedItem: &dnd5ev1alpha1.CountedItemReference{
					ItemId:   opt.ItemID,
					Name:     opt.Name,
					Quantity: opt.Quantity,
				},
			},
		}
	case *dnd5e.ItemBundle:
		items := make([]*dnd5ev1alpha1.CountedItemReference, len(opt.Items))
		for i, item := range opt.Items {
			items[i] = &dnd5ev1alpha1.CountedItemReference{
				ItemId:   item.ItemID,
				Name:     item.Name,
				Quantity: item.Quantity,
			}
		}
		return &dnd5ev1alpha1.ChoiceOption{
			OptionType: &dnd5ev1alpha1.ChoiceOption_Bundle{
				Bundle: &dnd5ev1alpha1.ItemBundle{
					Items: items,
				},
			},
		}
	case *dnd5e.NestedChoice:
		return &dnd5ev1alpha1.ChoiceOption{
			OptionType: &dnd5ev1alpha1.ChoiceOption_NestedChoice{
				NestedChoice: &dnd5ev1alpha1.NestedChoice{
					Choice: convertChoiceToProto(opt.Choice),
				},
			},
		}
	default:
		return nil
	}
}
