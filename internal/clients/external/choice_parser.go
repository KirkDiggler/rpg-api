package external

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// parseEquipmentChoices converts external equipment choice data to rich entity choices
func parseEquipmentChoices(choices []*EquipmentChoiceData, classID string) []dnd5e.Choice {
	result := make([]dnd5e.Choice, 0, len(choices))

	for i, choice := range choices {
		if choice == nil {
			continue
		}

		parsed := dnd5e.Choice{
			ID:          fmt.Sprintf("%s_equipment_%d", classID, i+1),
			Description: choice.Description,
			Type:        dnd5e.ChoiceTypeEquipment,
			ChooseCount: int32(choice.ChooseCount),
		}

		// Parse the description to determine option type
		if equipmentType := detectEquipmentType(choice.Description); equipmentType != "" {
			// This is a category reference (e.g., "any simple weapon")
			parsed.OptionSet = &dnd5e.CategoryReference{
				CategoryID: equipmentType,
			}
		} else if strings.Contains(strings.ToLower(choice.Description), " or ") {
			// This is a nested choice
			parsed.OptionSet = parseNestedEquipmentChoice(choice.Description, choice.Options)
		} else {
			// These are explicit options
			parsed.OptionSet = &dnd5e.ExplicitOptions{
				Options: parseEquipmentOptions(choice.Options),
			}
		}

		result = append(result, parsed)
	}

	return result
}

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

// Equipment type mappings from text patterns
var equipmentTypePatterns = map[string]string{
	"simple weapon":   "simple-weapons",
	"simple weapons":  "simple-weapons",
	"martial weapon":  "martial-weapons",
	"martial weapons": "martial-weapons",
	"light armor":     "light-armor",
	"medium armor":    "medium-armor",
	"heavy armor":     "heavy-armor",
	"shield":          "shields",
	"shields":         "shields",
	"artisan's tools": "artisan-tools",
	"gaming set":      "gaming-sets",
	"gaming sets":     "gaming-sets",
	"musical":         "musical-instruments",
	"pack":            "packs",
	"holy symbol":     "holy-symbols",
}

// detectEquipmentType analyzes the description to determine equipment category
func detectEquipmentType(description string) string {
	lowerDesc := strings.ToLower(description)

	for pattern, categoryID := range equipmentTypePatterns {
		if strings.Contains(lowerDesc, pattern) {
			return categoryID
		}
	}

	return ""
}

// parseNestedEquipmentChoice handles options like "(a) a shortsword or (b) any simple weapon"
func parseNestedEquipmentChoice(description string, options []string) dnd5e.ChoiceOptionSet {
	// First try to parse from the description
	parts := strings.Split(description, " or ")

	parsedOptions := make([]dnd5e.ChoiceOption, 0)

	for _, part := range parts {
		// Clean up the option (remove (a), (b) prefixes)
		cleanPart := strings.TrimSpace(part)
		if len(cleanPart) > 3 && cleanPart[0] == '(' && cleanPart[2] == ')' {
			cleanPart = strings.TrimSpace(cleanPart[3:])
		}

		// Check if this is a category reference
		if categoryID := detectEquipmentType(cleanPart); categoryID != "" {
			// For now, add as an item reference with the category ID
			// In the future, this could be a more complex nested choice
			parsedOptions = append(parsedOptions, &dnd5e.ItemReference{
				ItemID: categoryID,
				Name:   cleanPart,
			})
		} else {
			// Regular item
			parsedOptions = append(parsedOptions, parseEquipmentOption(cleanPart))
		}
	}

	// If we couldn't parse from description, fall back to options list
	if len(parsedOptions) == 0 && len(options) > 0 {
		for _, opt := range options {
			parsedOptions = append(parsedOptions, parseEquipmentOption(opt))
		}
	}

	// Create a nested choice
	nestedChoice := &dnd5e.Choice{
		ID:          "nested",
		Description: description,
		ChooseCount: 1,
		Type:        dnd5e.ChoiceTypeEquipment,
		OptionSet: &dnd5e.ExplicitOptions{
			Options: parsedOptions,
		},
	}

	return &dnd5e.ExplicitOptions{
		Options: []dnd5e.ChoiceOption{
			&dnd5e.NestedChoice{
				Choice: nestedChoice,
			},
		},
	}
}

// parseEquipmentOptions converts string options to choice options
func parseEquipmentOptions(options []string) []dnd5e.ChoiceOption {
	result := make([]dnd5e.ChoiceOption, 0, len(options))

	for _, opt := range options {
		result = append(result, parseEquipmentOption(opt))
	}

	return result
}

// parseEquipmentOption parses a single equipment option string
func parseEquipmentOption(optionStr string) dnd5e.ChoiceOption {
	// Check if it starts with a quantity
	parts := strings.Fields(optionStr)
	if len(parts) > 1 {
		// Try to parse first part as number
		if quantity, err := strconv.Atoi(parts[0]); err == nil {
			// This has a quantity
			itemName := strings.Join(parts[1:], " ")
			return &dnd5e.CountedItemReference{
				ItemID:   strings.ToLower(strings.ReplaceAll(itemName, " ", "-")),
				Name:     itemName,
				Quantity: int32(quantity),
			}
		}
	}

	// Default: single item reference
	cleanOption := strings.TrimSpace(optionStr)
	// Remove (a), (b) prefixes if present
	if len(cleanOption) > 3 && cleanOption[0] == '(' && cleanOption[2] == ')' {
		cleanOption = strings.TrimSpace(cleanOption[3:])
	}

	return &dnd5e.ItemReference{
		ItemID: strings.ToLower(strings.ReplaceAll(cleanOption, " ", "-")),
		Name:   cleanOption,
	}
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
