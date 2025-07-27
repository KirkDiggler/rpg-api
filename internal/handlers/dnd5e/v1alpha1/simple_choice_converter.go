package v1alpha1

import (
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
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
	return ConvertEntityChoiceTypeToProto(choiceType)
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
		items := make([]*dnd5ev1alpha1.BundleItem, len(opt.Items))
		for i, bundleItem := range opt.Items {
			items[i] = convertBundleItemToProto(bundleItem)
		}
		return &dnd5ev1alpha1.ChoiceOption{
			OptionType: &dnd5ev1alpha1.ChoiceOption_Bundle{
				Bundle: &dnd5ev1alpha1.ItemBundle{
					Items: items,
				},
			},
		}
	case *dnd5e.NestedChoice:
		// Optimize nested choice category references
		protoNestedChoice := convertChoiceToProto(opt.Choice)
		if protoNestedChoice != nil {
			// Ensure proper category ID for nested choices based on description
			if catRef, ok := protoNestedChoice.OptionSet.(*dnd5ev1alpha1.Choice_CategoryReference); ok {
				if catRef.CategoryReference.CategoryId == EquipmentCategoryEquipment {
					// Try to extract a more specific category from the description
					if specificCategory := extractSpecificCategory(protoNestedChoice.Description); specificCategory != "" {
						catRef.CategoryReference.CategoryId = specificCategory
					}
				}
			}
		}

		return &dnd5ev1alpha1.ChoiceOption{
			OptionType: &dnd5ev1alpha1.ChoiceOption_NestedChoice{
				NestedChoice: &dnd5ev1alpha1.NestedChoice{
					Choice: protoNestedChoice,
				},
			},
		}
	default:
		return nil
	}
}

// extractSpecificCategory extracts specific equipment categories from choice descriptions
func extractSpecificCategory(description string) string {
	return GetEquipmentCategoryFromDescription(description)
}

// convertBundleItemToProto converts entity bundle item to proto bundle item
func convertBundleItemToProto(item dnd5e.BundleItem) *dnd5ev1alpha1.BundleItem {
	switch itemType := item.ItemType.(type) {
	case *dnd5e.BundleItemConcreteItem:
		if itemType.ConcreteItem != nil {
			return &dnd5ev1alpha1.BundleItem{
				ItemType: &dnd5ev1alpha1.BundleItem_ConcreteItem{
					ConcreteItem: &dnd5ev1alpha1.CountedItemReference{
						ItemId:   itemType.ConcreteItem.ItemID,
						Name:     itemType.ConcreteItem.Name,
						Quantity: itemType.ConcreteItem.Quantity,
					},
				},
			}
		}
	case *dnd5e.BundleItemChoiceItem:
		if itemType.ChoiceItem != nil && itemType.ChoiceItem.Choice != nil {
			return &dnd5ev1alpha1.BundleItem{
				ItemType: &dnd5ev1alpha1.BundleItem_ChoiceItem{
					ChoiceItem: &dnd5ev1alpha1.NestedChoice{
						Choice: convertChoiceToProto(itemType.ChoiceItem.Choice),
					},
				},
			}
		}
	}
	return nil
}
