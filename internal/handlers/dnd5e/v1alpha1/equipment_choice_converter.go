package v1alpha1

import (
	"fmt"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
)

// convertEquipmentChoices converts toolkit equipment choices to proto choices
func convertEquipmentChoices(classData *class.Data) []*dnd5ev1alpha1.Choice {
	var choices []*dnd5ev1alpha1.Choice

	for i, equipChoice := range classData.EquipmentChoices {
		protoChoice := &dnd5ev1alpha1.Choice{
			Id:          fmt.Sprintf("%s_equipment_%d", classData.ID, i+1),
			Description: equipChoice.Description,
			ChooseCount: int32(equipChoice.Choose),
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
		}

		// Convert options
		options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(equipChoice.Options))
		for _, opt := range equipChoice.Options {
			protoOption := convertEquipmentOption(opt)
			if protoOption != nil {
				options = append(options, protoOption)
			}
		}

		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
			ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
				Options: options,
			},
		}

		choices = append(choices, protoChoice)
	}

	return choices
}

// convertEquipmentOption converts a single equipment option to proto
func convertEquipmentOption(opt class.EquipmentOption) *dnd5ev1alpha1.ChoiceOption {
	// Handle simple cases first
	if len(opt.Items) == 1 && opt.Items[0].ConcreteItem != nil {
		return convertSingleItem(opt.Items[0].ConcreteItem)
	}

	// Handle bundles
	return convertBundle(opt.Items)
}

// convertSingleItem converts a single concrete item to a choice option
func convertSingleItem(item *class.EquipmentData) *dnd5ev1alpha1.ChoiceOption {
	if item.Quantity == 1 {
		return &dnd5ev1alpha1.ChoiceOption{
			OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
				Item: &dnd5ev1alpha1.ItemReference{
					ItemId: item.ItemID,
					Name:   item.ItemID, // TODO: Get actual item name
				},
			},
		}
	}

	return &dnd5ev1alpha1.ChoiceOption{
		OptionType: &dnd5ev1alpha1.ChoiceOption_CountedItem{
			CountedItem: &dnd5ev1alpha1.CountedItemReference{
				ItemId:   item.ItemID,
				Name:     item.ItemID, // TODO: Get actual item name
				Quantity: int32(item.Quantity),
			},
		},
	}
}

// convertBundle converts a bundle of items to a choice option
func convertBundle(items []class.EquipmentBundleItem) *dnd5ev1alpha1.ChoiceOption {
	bundleItems := make([]*dnd5ev1alpha1.BundleItem, 0, len(items))

	for _, item := range items {
		bundleItem := convertBundleItem(item)
		if bundleItem != nil {
			bundleItems = append(bundleItems, bundleItem)
		}
	}

	return &dnd5ev1alpha1.ChoiceOption{
		OptionType: &dnd5ev1alpha1.ChoiceOption_Bundle{
			Bundle: &dnd5ev1alpha1.ItemBundle{
				Items: bundleItems,
			},
		},
	}
}

// convertBundleItem converts a single bundle item to proto
func convertBundleItem(item class.EquipmentBundleItem) *dnd5ev1alpha1.BundleItem {
	// Handle concrete items
	if item.ConcreteItem != nil {
		return &dnd5ev1alpha1.BundleItem{
			ItemType: &dnd5ev1alpha1.BundleItem_ConcreteItem{
				ConcreteItem: &dnd5ev1alpha1.CountedItemReference{
					ItemId:   item.ConcreteItem.ItemID,
					Name:     item.ConcreteItem.ItemID, // TODO: Get actual item name
					Quantity: int32(item.ConcreteItem.Quantity),
				},
			},
		}
	}

	// Handle nested choices
	if item.NestedChoice != nil {
		return &dnd5ev1alpha1.BundleItem{
			ItemType: &dnd5ev1alpha1.BundleItem_ChoiceItem{
				ChoiceItem: &dnd5ev1alpha1.NestedChoice{
					Choice: convertNestedChoice(item.NestedChoice),
				},
			},
		}
	}

	return nil
}

// convertNestedChoice converts a nested equipment choice to proto
func convertNestedChoice(choice *class.EquipmentChoiceData) *dnd5ev1alpha1.Choice {
	protoChoice := &dnd5ev1alpha1.Choice{
		Id:          choice.ID,
		Description: choice.Description,
		ChooseCount: int32(choice.Choose),
		ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
	}

	// Convert nested options
	options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(choice.Options))
	for _, opt := range choice.Options {
		// Nested choices should have simple items
		if len(opt.Items) > 0 && opt.Items[0].ConcreteItem != nil {
			options = append(options, &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
					Item: &dnd5ev1alpha1.ItemReference{
						ItemId: opt.Items[0].ConcreteItem.ItemID,
						Name:   opt.Items[0].ConcreteItem.ItemID, // TODO: Get actual item name
					},
				},
			})
		}
	}

	protoChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
		ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
			Options: options,
		},
	}

	return protoChoice
}
