package external

import (
	"fmt"
	"math"
	"strings"

	"github.com/fadedpez/dnd5e-api/entities"

	"github.com/KirkDiggler/rpg-api/internal/types/choices"
)

// safeIntToInt32 safely converts int to int32, clamping to max/min int32 values
func safeIntToInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < math.MinInt32 {
		return math.MinInt32
	}
	return int32(n)
}

// parseProficiencyChoices converts external choice data to rich entity choices
func parseProficiencyChoices(choiceData []*ChoiceData, baseID string) []choices.Choice {
	result := make([]choices.Choice, 0, len(choiceData))

	for i, choice := range choiceData {
		if choice == nil {
			continue
		}

		choiceType := mapExternalChoiceType(choice.Type)
		parsed := choices.Choice{
			ID:          fmt.Sprintf("%s_%s_%d", baseID, choice.Type, i+1),
			Description: fmt.Sprintf("Choose %d %s", choice.Choose, choice.Type),
			Type:        choiceType,
			ChooseCount: safeIntToInt32(choice.Choose),
		}

		// Check if this references a category
		if choice.From != "" && len(choice.Options) == 0 {
			parsed.OptionSet = &choices.CategoryReference{
				CategoryID: generateSlug(choice.From),
			}
		} else {
			// Explicit options
			options := make([]choices.ChoiceOption, 0, len(choice.Options))
			for _, opt := range choice.Options {
				options = append(options, &choices.ItemReference{
					ItemID: generateSlug(opt),
					Name:   opt,
				})
			}
			parsed.OptionSet = &choices.ExplicitOptions{
				Options: options,
			}
		}

		result = append(result, parsed)
	}

	return result
}

// mapExternalChoiceType maps external choice type strings to entity choice types
// Using centralized choice type mapper for consistency
func mapExternalChoiceType(externalType string) choices.ChoiceType {
	// Import the centralized mapper (we'll need to add the import)
	// For now, using the logic directly until we can import across packages
	normalized := strings.ToLower(strings.TrimSpace(externalType))

	switch normalized {
	case "skill", "skills", "proficiencies", "skill_proficiency":
		return choices.ChoiceTypeSkill
	case "tool", "tools", "tool_proficiency", "tool_proficiencies":
		return choices.ChoiceTypeTool
	case "language", "languages", "language_choice":
		return choices.ChoiceTypeLanguage
	case "weapon", "weapons", "weapon_proficiency", "weapon_proficiencies":
		return choices.ChoiceTypeWeaponProficiency
	case "armor", "armors", "armor_proficiency", "armor_proficiencies":
		return choices.ChoiceTypeArmorProficiency
	case "spell", "spells", "spell_choice":
		return choices.ChoiceTypeSpell
	case "feat", "feats", "feature", "features", "feat_choice":
		return choices.ChoiceTypeFeat
	default:
		return choices.ChoiceTypeEquipment
	}
}

// Equipment categories
const (
	equipmentCategoryDefault = "equipment"
)

// extractCategoryFromChoice attempts to extract the equipment category from a choice option
func extractCategoryFromChoice(choice *entities.ChoiceOption) string {
	if choice == nil {
		return equipmentCategoryDefault
	}

	// Use description-based matching for common equipment categories
	desc := strings.ToLower(choice.Description)

	// Martial weapons
	if strings.Contains(desc, "martial weapon") || strings.Contains(desc, "martial melee weapon") {
		return "martial-weapons"
	}

	// Simple weapons
	if strings.Contains(desc, "simple weapon") || strings.Contains(desc, "simple melee weapon") {
		return "simple-weapons"
	}

	// Ranged weapons
	if strings.Contains(desc, "ranged weapon") {
		if strings.Contains(desc, "martial") {
			return "martial-ranged-weapons"
		}
		return "simple-ranged-weapons"
	}

	// Armor categories
	if strings.Contains(desc, "light armor") {
		return "light-armor"
	}
	if strings.Contains(desc, "medium armor") {
		return "medium-armor"
	}
	if strings.Contains(desc, "heavy armor") {
		return "heavy-armor"
	}
	if strings.Contains(desc, "shield") {
		return "shields"
	}

	// Tools and instruments
	if strings.Contains(desc, "artisan") && strings.Contains(desc, "tool") {
		return "artisan-tools"
	}
	if strings.Contains(desc, "musical instrument") {
		return "musical-instruments"
	}
	if strings.Contains(desc, "gaming set") {
		return "gaming-sets"
	}

	// Adventuring gear
	if strings.Contains(desc, "adventuring gear") || strings.Contains(desc, "gear") {
		return "adventuring-gear"
	}

	// Default fallback
	return equipmentCategoryDefault
}

// generateNestedChoiceID creates a unique ID for nested choices
func generateNestedChoiceID(description string, categoryID string) string {
	// Clean description for ID generation
	cleanDesc := strings.ToLower(description)
	cleanDesc = strings.ReplaceAll(cleanDesc, " ", "_")
	cleanDesc = strings.ReplaceAll(cleanDesc, ",", "")
	cleanDesc = strings.ReplaceAll(cleanDesc, "(", "")
	cleanDesc = strings.ReplaceAll(cleanDesc, ")", "")

	// Limit length to avoid overly long IDs
	if len(cleanDesc) > 30 {
		cleanDesc = cleanDesc[:30]
	}

	return fmt.Sprintf("nested_%s_%s", categoryID, cleanDesc)
}

// parseEquipmentChoicesFromEntities converts rich entity equipment choices directly to Choice structures
func parseEquipmentChoicesFromEntities(choiceOptions []*entities.ChoiceOption, classID string) []choices.Choice {
	result := make([]choices.Choice, 0, len(choiceOptions))

	for i, choice := range choiceOptions {
		if choice == nil {
			continue
		}

		parsed := choices.Choice{
			ID:          fmt.Sprintf("%s_equipment_%d", classID, i+1),
			Description: choice.Description,
			Type:        choices.ChoiceTypeEquipment,
			ChooseCount: safeIntToInt32(choice.ChoiceCount),
		}

		// Convert the rich option list
		if choice.OptionList != nil {
			parsed.OptionSet = convertEntityOptionList(choice.OptionList)
		} else {
			// Fallback to empty explicit options
			parsed.OptionSet = &choices.ExplicitOptions{
				Options: []choices.ChoiceOption{},
			}
		}

		result = append(result, parsed)
	}

	return result
}

// convertEntityOptionList converts entity OptionList to dnd5e ChoiceOptionSet
func convertEntityOptionList(optionList *entities.OptionList) choices.ChoiceOptionSet {
	options := make([]choices.ChoiceOption, 0, len(optionList.Options))

	for _, option := range optionList.Options {
		convertedOption := convertEntityOption(option)
		if convertedOption != nil {
			options = append(options, convertedOption)
		}
	}

	return &choices.ExplicitOptions{
		Options: options,
	}
}

// convertEntityOption converts a single entity option to dnd5e ChoiceOption
func convertEntityOption(option entities.Option) choices.ChoiceOption {
	switch opt := option.(type) {
	case *entities.ReferenceOption:
		if opt.Reference != nil {
			return &choices.ItemReference{
				ItemID: opt.Reference.Key,
				Name:   opt.Reference.Name,
			}
		}

	case *entities.CountedReferenceOption:
		if opt.Reference != nil {
			return &choices.CountedItemReference{
				ItemID:   opt.Reference.Key,
				Name:     opt.Reference.Name,
				Quantity: safeIntToInt32(opt.Count),
			}
		}

	case *entities.MultipleOption:
		// Handle bundle of items like "a martial weapon and a shield"
		items := make([]choices.BundleItem, 0, len(opt.Items))
		for _, item := range opt.Items {
			switch itemOpt := item.(type) {
			case *entities.CountedReferenceOption:
				if itemOpt.Reference != nil {
					items = append(items, choices.BundleItem{
						ItemType: &choices.BundleItemConcreteItem{
							ConcreteItem: &choices.CountedItemReference{
								ItemID:   itemOpt.Reference.Key,
								Name:     itemOpt.Reference.Name,
								Quantity: safeIntToInt32(itemOpt.Count),
							},
						},
					})
				}
			case *entities.ChoiceOption:
				// This is a nested choice (like "a martial weapon")
				categoryID := extractCategoryFromChoice(itemOpt)
				if categoryID == "" {
					categoryID = "martial-weapons" // Default fallback
				}

				// Generate a proper nested choice ID
				nestedID := generateNestedChoiceID(itemOpt.Description, categoryID)

				// Create a proper nested choice
				nestedChoice := &choices.Choice{
					ID:          nestedID,
					Description: itemOpt.Description,
					Type:        choices.ChoiceTypeEquipment,
					ChooseCount: safeIntToInt32(itemOpt.ChoiceCount),
					OptionSet: &choices.CategoryReference{
						CategoryID: categoryID,
					},
				}

				items = append(items, choices.BundleItem{
					ItemType: &choices.BundleItemChoiceItem{
						ChoiceItem: &choices.NestedChoice{
							Choice: nestedChoice,
						},
					},
				})
			}
		}
		return &choices.ItemBundle{
			Items: items,
		}

	case *entities.ChoiceOption:
		// Handle nested choices like "two martial weapons"
		// Extract the category
		categoryID := extractCategoryFromChoice(opt)
		if categoryID == "" {
			categoryID = equipmentCategoryDefault // Generic fallback
		}

		// Generate a proper nested choice ID based on description and category
		nestedID := generateNestedChoiceID(opt.Description, categoryID)

		nestedChoice := &choices.Choice{
			ID:          nestedID,
			Description: opt.Description,
			Type:        choices.ChoiceTypeEquipment,
			ChooseCount: safeIntToInt32(opt.ChoiceCount),
			OptionSet: &choices.CategoryReference{
				CategoryID: categoryID,
			},
		}
		return &choices.NestedChoice{
			Choice: nestedChoice,
		}
	}

	return nil
}
