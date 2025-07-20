package v1alpha1

import (
	"fmt"
	"strconv"
	"strings"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// Equipment type mappings from text patterns to proto enum values
var equipmentTypePatterns = map[string]dnd5ev1alpha1.EquipmentType{
	"simple weapon":   dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_MELEE_WEAPON,
	"simple weapons":  dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_MELEE_WEAPON,
	"martial weapon":  dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_MELEE_WEAPON,
	"martial weapons": dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_MELEE_WEAPON,
	"light armor":     dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_LIGHT_ARMOR,
	"medium armor":    dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MEDIUM_ARMOR,
	"heavy armor":     dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_HEAVY_ARMOR,
	"shield":          dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SHIELD,
	"shields":         dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SHIELD,
	"artisan's tools": dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_ARTISAN_TOOLS,
	"gaming set":      dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_GAMING_SET,
	"gaming sets":     dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_GAMING_SET,
	"musical":         dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MUSICAL_INSTRUMENT,
	"pack":            dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_ADVENTURING_GEAR,
	"holy symbol":     dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_ADVENTURING_GEAR,
}

// convertEquipmentChoiceToProto converts external equipment choice data to proto Choice
func convertEquipmentChoiceToProto(choice *external.EquipmentChoiceData, choiceID string) *dnd5ev1alpha1.Choice {
	if choice == nil {
		return nil
	}

	protoChoice := &dnd5ev1alpha1.Choice{
		Id:          choiceID,
		Description: choice.Description,
		ChooseCount: int32(choice.ChooseCount),
		ChoiceType:  dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT,
	}

	// Determine if this is a category reference or explicit options
	if equipmentType := detectEquipmentType(choice.Description); equipmentType != dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_UNSPECIFIED {
		// This is a category reference (e.g., "any simple weapon")
		categoryID := equipmentTypeToCategoryID(equipmentType)
		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_CategoryReference{
			CategoryReference: &dnd5ev1alpha1.CategoryReference{
				CategoryId: categoryID,
			},
		}
	} else {
		// These are explicit options
		options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(choice.Options))
		for _, opt := range choice.Options {
			options = append(options, parseEquipmentOption(opt))
		}

		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
			ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
				Options: options,
			},
		}
	}

	return protoChoice
}

// convertEntityChoiceToProto converts entity choice to proto Choice
func convertEntityChoiceToProto(choice *dnd5e.Choice, choiceID string) *dnd5ev1alpha1.Choice {
	if choice == nil {
		return nil
	}

	// Determine choice type based on the type string
	choiceType := dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_UNSPECIFIED
	switch strings.ToLower(choice.Type) {
	case "skill", "skills":
		choiceType = dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SKILL
	case "tool", "tools", "tool_proficiency":
		choiceType = dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_TOOL
	case "language", "languages":
		choiceType = dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_LANGUAGE
	case "weapon", "weapon_proficiency":
		choiceType = dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_WEAPON_PROFICIENCY
	case "armor", "armor_proficiency":
		choiceType = dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_ARMOR_PROFICIENCY
	}

	protoChoice := &dnd5ev1alpha1.Choice{
		Id:          choiceID,
		Description: fmt.Sprintf("Choose %d %s", choice.Choose, choice.Type),
		ChooseCount: choice.Choose,
		ChoiceType:  choiceType,
	}

	// Check if this references a category (e.g., "from artisan's tools")
	if choice.From != "" && len(choice.Options) == 0 {
		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_CategoryReference{
			CategoryReference: &dnd5ev1alpha1.CategoryReference{
				CategoryId: strings.ToLower(strings.ReplaceAll(choice.From, " ", "-")),
			},
		}
	} else {
		// Explicit options
		options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(choice.Options))
		for _, opt := range choice.Options {
			options = append(options, &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
					Item: &dnd5ev1alpha1.ItemReference{
						ItemId: strings.ToLower(strings.ReplaceAll(opt, " ", "-")),
						Name:   opt,
					},
				},
			})
		}

		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
			ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
				Options: options,
			},
		}
	}

	return protoChoice
}

// convertEntityEquipmentChoiceToProto converts entity equipment choice to proto Choice
func convertEntityEquipmentChoiceToProto(choice *dnd5e.EquipmentChoice, choiceID string) *dnd5ev1alpha1.Choice {
	if choice == nil {
		return nil
	}

	protoChoice := &dnd5ev1alpha1.Choice{
		Id:          choiceID,
		Description: choice.Description,
		ChooseCount: choice.ChooseCount,
		ChoiceType:  dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT,
	}

	// Determine if this is a category reference or explicit options
	if equipmentType := detectEquipmentType(choice.Description); equipmentType != dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_UNSPECIFIED {
		// This is a category reference (e.g., "any simple weapon")
		categoryID := equipmentTypeToCategoryID(equipmentType)
		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_CategoryReference{
			CategoryReference: &dnd5ev1alpha1.CategoryReference{
				CategoryId: categoryID,
			},
		}
	} else {
		// These are explicit options
		options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(choice.Options))
		for _, opt := range choice.Options {
			options = append(options, parseEquipmentOption(opt))
		}

		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
			ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
				Options: options,
			},
		}
	}

	return protoChoice
}

// convertProficiencyChoiceToProto converts external proficiency choice data to proto Choice
func convertProficiencyChoiceToProto(choice *external.ChoiceData, choiceID string) *dnd5ev1alpha1.Choice {
	if choice == nil {
		return nil
	}

	// Determine choice type based on the type string
	choiceType := dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_UNSPECIFIED
	switch strings.ToLower(choice.Type) {
	case "skill", "skills":
		choiceType = dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SKILL
	case "tool", "tools", "tool_proficiency":
		choiceType = dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_TOOL
	case "language", "languages":
		choiceType = dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_LANGUAGE
	case "weapon", "weapon_proficiency":
		choiceType = dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_WEAPON_PROFICIENCY
	case "armor", "armor_proficiency":
		choiceType = dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_ARMOR_PROFICIENCY
	}

	protoChoice := &dnd5ev1alpha1.Choice{
		Id:          choiceID,
		Description: fmt.Sprintf("Choose %d %s", choice.Choose, choice.Type),
		ChooseCount: int32(choice.Choose),
		ChoiceType:  choiceType,
	}

	// Check if this references a category (e.g., "from artisan's tools")
	if choice.From != "" && len(choice.Options) == 0 {
		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_CategoryReference{
			CategoryReference: &dnd5ev1alpha1.CategoryReference{
				CategoryId: strings.ToLower(strings.ReplaceAll(choice.From, " ", "-")),
			},
		}
	} else {
		// Explicit options
		options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(choice.Options))
		for _, opt := range choice.Options {
			options = append(options, &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
					Item: &dnd5ev1alpha1.ItemReference{
						ItemId: strings.ToLower(strings.ReplaceAll(opt, " ", "-")),
						Name:   opt,
					},
				},
			})
		}

		protoChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
			ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
				Options: options,
			},
		}
	}

	return protoChoice
}

// detectEquipmentType analyzes the description to determine equipment type
func detectEquipmentType(description string) dnd5ev1alpha1.EquipmentType {
	lowerDesc := strings.ToLower(description)

	for pattern, equipType := range equipmentTypePatterns {
		if strings.Contains(lowerDesc, pattern) {
			return equipType
		}
	}

	return dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_UNSPECIFIED
}

// equipmentTypeToCategoryID converts equipment type to category ID
func equipmentTypeToCategoryID(equipType dnd5ev1alpha1.EquipmentType) string {
	switch equipType {
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_MELEE_WEAPON,
		dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_RANGED_WEAPON:
		return "simple-weapons"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_MELEE_WEAPON,
		dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_RANGED_WEAPON:
		return "martial-weapons"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_LIGHT_ARMOR:
		return "light-armor"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MEDIUM_ARMOR:
		return "medium-armor"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_HEAVY_ARMOR:
		return "heavy-armor"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SHIELD:
		return "shields"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_ARTISAN_TOOLS:
		return "artisan-tools"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_GAMING_SET:
		return "gaming-sets"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MUSICAL_INSTRUMENT:
		return "musical-instruments"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_ADVENTURING_GEAR:
		return "adventuring-gear"
	case dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_TOOLS:
		return "tools"
	default:
		return ""
	}
}

// parseEquipmentOption parses an equipment option string into a ChoiceOption
func parseEquipmentOption(optionStr string) *dnd5ev1alpha1.ChoiceOption {
	// Handle options like "(a) a shortsword or (b) any simple weapon"
	// or "2 handaxes"

	// Check if it starts with a quantity
	parts := strings.Fields(optionStr)
	if len(parts) > 1 {
		// Try to parse first part as number
		var quantity int32 = 1
		if q, err := strconv.Atoi(parts[0]); err == nil {
			quantity = int32(q)
			// This has a quantity
			itemName := strings.Join(parts[1:], " ")
			return &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_CountedItem{
					CountedItem: &dnd5ev1alpha1.CountedItemReference{
						ItemId:   strings.ToLower(strings.ReplaceAll(itemName, " ", "-")),
						Name:     itemName,
						Quantity: quantity,
					},
				},
			}
		}
	}

	// Check for nested choices (e.g., "a shortsword or any simple weapon")
	if strings.Contains(strings.ToLower(optionStr), " or ") {
		return parseNestedEquipmentChoice(optionStr)
	}

	// Default: single item reference
	cleanOption := strings.TrimSpace(optionStr)
	// Remove (a), (b) prefixes if present
	if len(cleanOption) > 3 && cleanOption[0] == '(' && cleanOption[2] == ')' {
		cleanOption = strings.TrimSpace(cleanOption[3:])
	}

	return &dnd5ev1alpha1.ChoiceOption{
		OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
			Item: &dnd5ev1alpha1.ItemReference{
				ItemId: strings.ToLower(strings.ReplaceAll(cleanOption, " ", "-")),
				Name:   cleanOption,
			},
		},
	}
}

// parseNestedEquipmentChoice handles options like "(a) a shortsword or (b) any simple weapon"
func parseNestedEquipmentChoice(optionStr string) *dnd5ev1alpha1.ChoiceOption {
	// Split by " or " to get individual options
	parts := strings.Split(optionStr, " or ")
	if len(parts) < 2 {
		// Not a valid nested choice, treat as single item
		return parseEquipmentOption(optionStr)
	}

	// Create a nested choice
	nestedChoice := &dnd5ev1alpha1.Choice{
		Id:          "nested_choice",
		Description: optionStr,
		ChooseCount: 1,
		ChoiceType:  dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT,
	}

	// Parse each option
	options := make([]*dnd5ev1alpha1.ChoiceOption, 0, len(parts))
	for _, part := range parts {
		// Clean up the option (remove (a), (b) prefixes)
		cleanPart := strings.TrimSpace(part)
		if len(cleanPart) > 3 && cleanPart[0] == '(' && cleanPart[2] == ')' {
			cleanPart = strings.TrimSpace(cleanPart[3:])
		}
		
		// Check if this is a category reference (e.g., "any simple weapon")
		if equipmentType := detectEquipmentType(cleanPart); equipmentType != dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_UNSPECIFIED {
			// This option is a category reference
			options = append(options, &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
					Item: &dnd5ev1alpha1.ItemReference{
						ItemId: equipmentTypeToCategoryID(equipmentType),
						Name:   cleanPart,
					},
				},
			})
		} else {
			// Regular item
			options = append(options, &dnd5ev1alpha1.ChoiceOption{
				OptionType: &dnd5ev1alpha1.ChoiceOption_Item{
					Item: &dnd5ev1alpha1.ItemReference{
						ItemId: strings.ToLower(strings.ReplaceAll(cleanPart, " ", "-")),
						Name:   cleanPart,
					},
				},
			})
		}
	}

	nestedChoice.OptionSet = &dnd5ev1alpha1.Choice_ExplicitOptions{
		ExplicitOptions: &dnd5ev1alpha1.ExplicitOptions{
			Options: options,
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
