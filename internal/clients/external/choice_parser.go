package external

import (
	"fmt"
	"strings"

	"github.com/fadedpez/dnd5e-api/entities"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// parseProficiencyChoices converts external choice data to rich entity choices
func parseProficiencyChoices(choices []*ChoiceData, baseID string) []dnd5e.Choice {
	result := make([]dnd5e.Choice, 0, len(choices))

	for i, choice := range choices {
		if choice == nil {
			continue
		}

		choiceType := mapExternalChoiceType(choice.Type)
		parsed := dnd5e.Choice{
			ID:          fmt.Sprintf("%s_%s_%d", baseID, choice.Type, i+1),
			Description: fmt.Sprintf("Choose %d %s", choice.Choose, choice.Type),
			Type:        choiceType,
			ChooseCount: int32(choice.Choose),
		}

		// Check if this references a category
		if choice.From != "" && len(choice.Options) == 0 {
			parsed.OptionSet = &dnd5e.CategoryReference{
				CategoryID: strings.ToLower(strings.ReplaceAll(choice.From, " ", "-")),
			}
		} else {
			// Explicit options
			options := make([]dnd5e.ChoiceOption, 0, len(choice.Options))
			for _, opt := range choice.Options {
				options = append(options, &dnd5e.ItemReference{
					ItemID: strings.ToLower(strings.ReplaceAll(opt, " ", "-")),
					Name:   opt,
				})
			}
			parsed.OptionSet = &dnd5e.ExplicitOptions{
				Options: options,
			}
		}

		result = append(result, parsed)
	}

	return result
}


// mapExternalChoiceType maps external choice type strings to entity choice types
func mapExternalChoiceType(externalType string) dnd5e.ChoiceType {
	switch strings.ToLower(externalType) {
	case "skill", "skills":
		return dnd5e.ChoiceTypeSkill
	case "tool", "tools", "tool_proficiency":
		return dnd5e.ChoiceTypeTool
	case "language", "languages":
		return dnd5e.ChoiceTypeLanguage
	case "weapon", "weapon_proficiency":
		return dnd5e.ChoiceTypeWeaponProficiency
	case "armor", "armor_proficiency":
		return dnd5e.ChoiceTypeArmorProficiency
	default:
		return dnd5e.ChoiceTypeEquipment
	}
}

// extractCategoryFromChoice attempts to extract the equipment category from a choice option
func extractCategoryFromChoice(choice *entities.ChoiceOption) string {
	// This is a simplified version - in reality we'd need to examine the option list
	// to find equipment category references
	if choice.Description == "a martial weapon" || choice.Description == "two martial weapons" {
		return "martial-weapons"
	}
	if choice.Description == "a simple weapon" || choice.Description == "two simple weapons" {
		return "simple-weapons"
	}
	return ""
}

// parseEquipmentChoicesFromEntities converts rich entity equipment choices directly to Choice structures
func parseEquipmentChoicesFromEntities(choices []*entities.ChoiceOption, classID string) []dnd5e.Choice {
	result := make([]dnd5e.Choice, 0, len(choices))

	for i, choice := range choices {
		if choice == nil {
			continue
		}

		parsed := dnd5e.Choice{
			ID:          fmt.Sprintf("%s_equipment_%d", classID, i+1),
			Description: choice.Description,
			Type:        dnd5e.ChoiceTypeEquipment,
			ChooseCount: int32(choice.ChoiceCount),
		}

		// Convert the rich option list
		if choice.OptionList != nil {
			parsed.OptionSet = convertEntityOptionList(choice.OptionList)
		} else {
			// Fallback to empty explicit options
			parsed.OptionSet = &dnd5e.ExplicitOptions{
				Options: []dnd5e.ChoiceOption{},
			}
		}

		result = append(result, parsed)
	}

	return result
}

// convertEntityOptionList converts entity OptionList to dnd5e ChoiceOptionSet
func convertEntityOptionList(optionList *entities.OptionList) dnd5e.ChoiceOptionSet {
	options := make([]dnd5e.ChoiceOption, 0, len(optionList.Options))

	for _, option := range optionList.Options {
		convertedOption := convertEntityOption(option)
		if convertedOption != nil {
			options = append(options, convertedOption)
		}
	}

	return &dnd5e.ExplicitOptions{
		Options: options,
	}
}

// convertEntityOption converts a single entity option to dnd5e ChoiceOption
func convertEntityOption(option entities.Option) dnd5e.ChoiceOption {
	switch opt := option.(type) {
	case *entities.ReferenceOption:
		if opt.Reference != nil {
			return &dnd5e.ItemReference{
				ItemID: opt.Reference.Key,
				Name:   opt.Reference.Name,
			}
		}

	case *entities.CountedReferenceOption:
		if opt.Reference != nil {
			return &dnd5e.CountedItemReference{
				ItemID:   opt.Reference.Key,
				Name:     opt.Reference.Name,
				Quantity: int32(opt.Count),
			}
		}

	case *entities.MultipleOption:
		// Handle bundle of items like "a martial weapon and a shield"
		items := make([]dnd5e.CountedItemReference, 0, len(opt.Items))
		for _, item := range opt.Items {
			switch itemOpt := item.(type) {
			case *entities.CountedReferenceOption:
				if itemOpt.Reference != nil {
					items = append(items, dnd5e.CountedItemReference{
						ItemID:   itemOpt.Reference.Key,
						Name:     itemOpt.Reference.Name,
						Quantity: int32(itemOpt.Count),
					})
				}
			case *entities.ChoiceOption:
				// This is a nested choice (like "a martial weapon")
				// Extract the category from the option list
				categoryID := extractCategoryFromChoice(itemOpt)
				if categoryID == "" {
					categoryID = "martial-weapons" // Default fallback
				}
				items = append(items, dnd5e.CountedItemReference{
					ItemID:   categoryID,
					Name:     itemOpt.Description,
					Quantity: int32(itemOpt.ChoiceCount),
				})
			}
		}
		return &dnd5e.ItemBundle{
			Items: items,
		}

	case *entities.ChoiceOption:
		// Handle nested choices like "two martial weapons"
		// Extract the category
		categoryID := extractCategoryFromChoice(opt)
		if categoryID == "" {
			categoryID = "equipment" // Generic fallback
		}

		nestedChoice := &dnd5e.Choice{
			ID:          "nested", // Will be fixed by caller
			Description: opt.Description,
			Type:        dnd5e.ChoiceTypeEquipment,
			ChooseCount: int32(opt.ChoiceCount),
			OptionSet: &dnd5e.CategoryReference{
				CategoryID: categoryID,
			},
		}
		return &dnd5e.NestedChoice{
			Choice: nestedChoice,
		}
	}

	return nil
}
