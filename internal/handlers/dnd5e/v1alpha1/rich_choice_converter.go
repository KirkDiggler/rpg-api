package v1alpha1

import (
	"fmt"

	"github.com/fadedpez/dnd5e-api/entities"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
)

// convertRichChoiceToProto converts entities.ChoiceOption directly to proto Choice
// This bypasses the intermediate internal entity conversion for better performance
func convertRichChoiceToProto(choice *entities.ChoiceOption, baseID string, index int) *dnd5ev1alpha1.Choice {
	if choice == nil {
		return nil
	}

	choiceID := fmt.Sprintf("%s_choice_%d", baseID, index)
	protoChoice := &dnd5ev1alpha1.Choice{
		Id:          choiceID,
		Description: choice.Description,
		ChooseCount: int32(choice.ChoiceCount),
		ChoiceType:  mapExternalChoiceTypeToProto(choice.ChoiceType),
	}

	// Convert the option list directly
	if choice.OptionList != nil {
		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
			ExplicitOptions: convertRichOptionListToProto(choice.OptionList),
		}
	}

	return protoChoice
}

// convertRichOptionListToProto converts entities.OptionList directly to proto ExplicitOptions
func convertRichOptionListToProto(optionList *entities.OptionList) *dnd5ev1alpha1.ExplicitOptions {
	if optionList == nil || len(optionList.Options) == 0 {
		return &dnd5ev1alpha1.ExplicitOptions{Options: []*dnd5ev1alpha1.ChoiceOption{}}
	}

	protoOptions := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(optionList.Options))

	for i, option := range optionList.Options {
		if protoOption := convertRichOptionToProto(option, i); protoOption != nil {
			protoOptions = append(protoOptions, protoOption)
		}
	}

	return &dnd5ev1alpha1.ExplicitOptions{
		Options: protoOptions,
	}
}

// convertRichOptionToProto converts entities.Option directly to proto ChoiceOption
func convertRichOptionToProto(option entities.Option, index int) *dnd5ev1alpha1.ChoiceOption {
	if option == nil {
		return nil
	}

	switch opt := option.(type) {
	case *entities.ReferenceOption:
		if opt.Reference != nil {
			return &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
					Item: &dnd5ev1alpha1.ItemReference{
						ItemId: opt.Reference.Key,
						Name:   opt.Reference.Name,
					},
				},
			}
		}

	case *entities.CountedReferenceOption:
		if opt.Reference != nil {
			return &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_CountedItem{
					CountedItem: &dnd5ev1alpha1.CountedItemReference{
						ItemId:   opt.Reference.Key,
						Name:     opt.Reference.Name,
						Quantity: int32(opt.Count),
					},
				},
			}
		}

	case *entities.MultipleOption:
		// Convert bundle of items
		items := make([]*dnd5ev1alpha1.BundleItem, 0, len(opt.Items))
		for i, item := range opt.Items {
			if bundleItem := convertEntityToBundleItem(item, i); bundleItem != nil {
				items = append(items, bundleItem)
			}
		}
		return &dnd5ev1alpha1.ChoiceOption{
			OptionType: &dnd5ev1alpha1.ChoiceOption_Bundle{
				Bundle: &dnd5ev1alpha1.ItemBundle{
					Items: items,
				},
			},
		}

	case *entities.ChoiceOption:
		// Handle nested choices - create category reference based on description
		categoryID := extractCategoryFromDescription(opt.Description)
		if categoryID == "" {
			categoryID = "equipment" // Fallback
		}

		nestedChoiceID := fmt.Sprintf("nested_%s_%d", categoryID, index)

		nestedChoice := &dnd5ev1alpha1.Choice{
			Id:          nestedChoiceID,
			Description: opt.Description,
			ChooseCount: int32(opt.ChoiceCount),
			ChoiceType:  dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT,
			OptionSet: &dnd5ev1alpha1.Choice_CategoryReference{
				CategoryReference: &dnd5ev1alpha1.CategoryReference{
					CategoryId: categoryID,
					ExcludeIds: []string{},
				},
			},
		}

		return &dnd5ev1alpha1.ChoiceOption{
			OptionType: &dnd5ev1alpha1.ChoiceOption_NestedChoice{
				NestedChoice: &dnd5ev1alpha1.NestedChoice{
					Choice: nestedChoice,
				},
			},
		}
	}

	return nil
}

// convertItemToCountedReference converts various item types to CountedItemReference
func convertItemToCountedReference(item entities.Option) *dnd5ev1alpha1.CountedItemReference {
	switch itemOpt := item.(type) {
	case *entities.ReferenceOption:
		if itemOpt.Reference != nil {
			return &dnd5ev1alpha1.CountedItemReference{
				ItemId:   itemOpt.Reference.Key,
				Name:     itemOpt.Reference.Name,
				Quantity: 1, // Default quantity for simple references
			}
		}
	case *entities.CountedReferenceOption:
		if itemOpt.Reference != nil {
			return &dnd5ev1alpha1.CountedItemReference{
				ItemId:   itemOpt.Reference.Key,
				Name:     itemOpt.Reference.Name,
				Quantity: int32(itemOpt.Count),
			}
		}
	case *entities.ChoiceOption:
		// For choice items in bundles, create a reference to the category
		categoryID := extractCategoryFromDescription(itemOpt.Description)
		if categoryID == "" {
			categoryID = "equipment"
		}
		return &dnd5ev1alpha1.CountedItemReference{
			ItemId:   categoryID,
			Name:     itemOpt.Description,
			Quantity: int32(itemOpt.ChoiceCount),
		}
	}
	return nil
}

// extractCategoryFromDescription extracts equipment category from choice description
func extractCategoryFromDescription(description string) string {
	return GetEquipmentCategoryFromDescription(description)
}

// convertEntityToBundleItem converts an entity option to a bundle item
func convertEntityToBundleItem(item entities.Option, index int) *dnd5ev1alpha1.BundleItem {
	switch itemOpt := item.(type) {
	case *entities.CountedReferenceOption:
		if itemOpt.Reference != nil {
			return &dnd5ev1alpha1.BundleItem{
				ItemType: &dnd5ev1alpha1.BundleItem_ConcreteItem{
					ConcreteItem: &dnd5ev1alpha1.CountedItemReference{
						ItemId:   itemOpt.Reference.Key,
						Name:     itemOpt.Reference.Name,
						Quantity: int32(itemOpt.Count),
					},
				},
			}
		}
	case *entities.ChoiceOption:
		// Create a nested choice for this item
		categoryID := extractCategoryFromDescription(itemOpt.Description)
		if categoryID == "" {
			categoryID = "equipment"
		}
		
		nestedChoiceID := fmt.Sprintf("bundle_nested_%s_%d", categoryID, index)
		
		return &dnd5ev1alpha1.BundleItem{
			ItemType: &dnd5ev1alpha1.BundleItem_ChoiceItem{
				ChoiceItem: &dnd5ev1alpha1.NestedChoice{
					Choice: &dnd5ev1alpha1.Choice{
						Id:          nestedChoiceID,
						Description: itemOpt.Description,
						ChooseCount: int32(itemOpt.ChoiceCount),
						ChoiceType:  dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT,
						OptionSet: &dnd5ev1alpha1.Choice_CategoryReference{
							CategoryReference: &dnd5ev1alpha1.CategoryReference{
								CategoryId: categoryID,
							},
						},
					},
				},
			},
		}
	}
	return nil
}

// mapExternalChoiceTypeToProto maps string choice types to proto enums
func mapExternalChoiceTypeToProto(choiceType string) dnd5ev1alpha1.ChoiceType {
	return ConvertExternalChoiceTypeToProto(choiceType)
}

// convertRichChoicesToProto converts multiple rich entity choices to proto choices
func convertRichChoicesToProto(choices []*entities.ChoiceOption, baseID string) []*dnd5ev1alpha1.Choice {
	if len(choices) == 0 {
		return nil
	}

	protoChoices := make([]*dnd5ev1alpha1.Choice, 0, len(choices))
	for i, choice := range choices {
		if protoChoice := convertRichChoiceToProto(choice, baseID, i+1); protoChoice != nil {
			protoChoices = append(protoChoices, protoChoice)
		}
	}

	return protoChoices
}
