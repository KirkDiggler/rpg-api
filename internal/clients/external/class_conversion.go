package external

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/fadedpez/dnd5e-api/entities"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
)

// convertClassToHybrid converts API class data to both toolkit format and UI data
func (c *client) convertClassToHybrid(apiClass *entities.Class) (*class.Data, *ClassUIData) {
	if apiClass == nil {
		return nil, nil
	}

	// Convert API key to toolkit constant, validating it exists
	classID, err := convertKeyToClassID(apiClass.Key)
	if err != nil {
		// Log warning but continue with the raw key
		// This allows us to handle new classes from the API that we don't have constants for yet
		slog.Warn("Unknown class key from API, using raw key",
			"key", apiClass.Key,
			"name", apiClass.Name,
			"error", err)
		classID = constants.Class(apiClass.Key)
	}

	// Convert to toolkit format
	toolkitData := &class.Data{
		ID:                classID,
		Name:              apiClass.Name,
		Description:       "", // Will be in UI data
		HitDice:           apiClass.HitDie,
		HitPointsPerLevel: (apiClass.HitDie + 1) / 2, // Average roll
	}

	// Convert saving throws
	toolkitData.SavingThrows = make([]constants.Ability, 0, len(apiClass.SavingThrows))
	for _, st := range apiClass.SavingThrows {
		if ability := convertToAbilityConstant(st.Key); ability != "" {
			toolkitData.SavingThrows = append(toolkitData.SavingThrows, ability)
		}
	}

	// Convert armor proficiencies
	toolkitData.ArmorProficiencies = make([]string, len(apiClass.ArmorProficiencies))
	for i, armor := range apiClass.ArmorProficiencies {
		toolkitData.ArmorProficiencies[i] = armor.Name
	}

	// Convert weapon proficiencies
	toolkitData.WeaponProficiencies = make([]string, len(apiClass.WeaponProficiencies))
	for i, weapon := range apiClass.WeaponProficiencies {
		toolkitData.WeaponProficiencies[i] = weapon.Name
	}

	// Convert tool proficiencies
	toolkitData.ToolProficiencies = make([]string, len(apiClass.ToolProficiencies))
	for i, tool := range apiClass.ToolProficiencies {
		toolkitData.ToolProficiencies[i] = tool.Name
	}

	// Extract skill options from proficiency choices
	for _, choice := range apiClass.ProficiencyChoices {
		if choice != nil && choice.ChoiceType == "proficiencies" {
			// Check if this is a skill proficiency choice by looking at the options
			isSkillChoice := false
			if choice.OptionList != nil && len(choice.OptionList.Options) > 0 {
				// Check the first option to see if it's a skill
				if refOpt, ok := choice.OptionList.Options[0].(*entities.ReferenceOption); ok && refOpt.Reference != nil {
					if strings.HasPrefix(refOpt.Reference.Name, "Skill: ") {
						isSkillChoice = true
					}
				}
			}

			if isSkillChoice {
				toolkitData.SkillProficiencyCount = choice.ChoiceCount
				for _, option := range choice.OptionList.Options {
					if refOpt, ok := option.(*entities.ReferenceOption); ok && refOpt.Reference != nil {
						skillName := strings.TrimPrefix(refOpt.Reference.Name, "Skill: ")
						if skill := convertToSkillConstant(skillName); skill != "" {
							toolkitData.SkillOptions = append(toolkitData.SkillOptions, skill)
						}
					}
				}
				break // Found the skill choice
			}
		}
	}

	// Convert starting equipment
	toolkitData.StartingEquipment = make([]class.EquipmentData, len(apiClass.StartingEquipment))
	for i, eq := range apiClass.StartingEquipment {
		toolkitData.StartingEquipment[i] = class.EquipmentData{
			ItemID:   eq.Equipment.Key,
			Quantity: eq.Quantity,
		}
	}

	// Convert equipment choices
	toolkitData.EquipmentChoices = make([]class.EquipmentChoiceData, len(apiClass.StartingEquipmentOptions))
	for i, choice := range apiClass.StartingEquipmentOptions {
		choiceData := class.EquipmentChoiceData{
			ID:          generateSlug(choice.Description),
			Description: choice.Description,
			Choose:      choice.ChoiceCount,
		}

		// Extract options
		if choice.OptionList != nil {
			optionIndex := 0
			for _, option := range choice.OptionList.Options {
				// Handle different option types
				switch opt := option.(type) {
				case *entities.CountedReferenceOption:
					// Equipment with count
					if opt.Reference != nil {
						choiceData.Options = append(choiceData.Options, class.EquipmentOption{
							ID: generateSlug(opt.Reference.Name),
							Items: []class.EquipmentData{
								{
									ItemID:   opt.Reference.Key,
									Quantity: opt.Count,
								},
							},
						})
					}
				case *entities.ReferenceOption:
					// Single equipment item
					if opt.Reference != nil {
						choiceData.Options = append(choiceData.Options, class.EquipmentOption{
							ID: generateSlug(opt.Reference.Name),
							Items: []class.EquipmentData{
								{
									ItemID:   opt.Reference.Key,
									Quantity: 1,
								},
							},
						})
					}
				case *entities.ChoiceOption:
					// Handle nested equipment choices (like "choose a martial weapon")
					// The external client should expand these into individual options
					categoryID := detectEquipmentCategory(opt.Description)
					if categoryID != "" {
						// Fetch the equipment category and expand all options
						equipmentCategory, err := c.dnd5eClient.GetEquipmentCategory(categoryID)
						if err != nil {
							slog.Warn("Failed to fetch equipment category",
								"category", categoryID,
								"description", opt.Description,
								"error", err)
							// Fall back to placeholder
							choiceData.Options = append(choiceData.Options, class.EquipmentOption{
								ID: fmt.Sprintf("choose-%s", categoryID),
								Items: []class.EquipmentData{
									{
										ItemID:   fmt.Sprintf("choose-%s", categoryID),
										Quantity: opt.ChoiceCount,
									},
								},
							})
						} else {
							// Expand each item in the category as a separate option
							for _, eq := range equipmentCategory.Equipment {
								if eq != nil {
									choiceData.Options = append(choiceData.Options, class.EquipmentOption{
										ID: eq.Key,
										Items: []class.EquipmentData{
											{
												ItemID:   eq.Key,
												Quantity: 1,
											},
										},
									})
								}
							}
						}
					} else {
						// Not an equipment category - use placeholder
						choiceData.Options = append(choiceData.Options, class.EquipmentOption{
							ID: generateSlug(opt.Description),
							Items: []class.EquipmentData{
								{
									ItemID:   fmt.Sprintf("choice-%s", generateSlug(opt.Description)),
									Quantity: opt.ChoiceCount,
								},
							},
						})
					}
				case *entities.MultipleOption:
					// Multiple items together (bundles)
					var bundleItems []class.EquipmentData

					// Process each item in the bundle
					for _, item := range opt.Items {
						switch bundleItem := item.(type) {
						case *entities.CountedReferenceOption:
							// Concrete item (like shield)
							if bundleItem.Reference != nil {
								bundleItems = append(bundleItems, class.EquipmentData{
									ItemID:   bundleItem.Reference.Key,
									Quantity: bundleItem.Count,
								})
							}
						case *entities.ChoiceOption:
							// Nested choice in bundle (like "choose a martial weapon")
							// For bundles with choices, we use a placeholder
							categoryID := detectEquipmentCategory(bundleItem.Description)
							if categoryID != "" {
								// Use placeholder for equipment category choice in bundle
								bundleItems = append(bundleItems, class.EquipmentData{
									ItemID:   fmt.Sprintf("choose-%s", categoryID),
									Quantity: bundleItem.ChoiceCount,
								})
							} else {
								// Not a category - add placeholder
								bundleItems = append(bundleItems, class.EquipmentData{
									ItemID:   fmt.Sprintf("choice-%s", generateSlug(bundleItem.Description)),
									Quantity: bundleItem.ChoiceCount,
								})
							}
						}
					}

					if len(bundleItems) > 0 {
						choiceData.Options = append(choiceData.Options, class.EquipmentOption{
							ID:    fmt.Sprintf("bundle-%d", optionIndex),
							Items: bundleItems,
						})
					}
				}
				optionIndex++
			}
		}

		toolkitData.EquipmentChoices[i] = choiceData
	}

	// Features would need to be fetched separately for each level
	// TODO: Implement feature fetching from class levels API
	toolkitData.Features = make(map[int][]class.FeatureData)

	// Check for spellcasting
	if apiClass.Spellcasting != nil && apiClass.Spellcasting.SpellcastingAbility != nil {
		toolkitData.Spellcasting = &class.SpellcastingData{
			Ability: convertToAbilityConstant(apiClass.Spellcasting.SpellcastingAbility.Key),
		}
		// Info keys would need more complex parsing for full spellcasting data
	}

	// Extract UI data
	uiData := &ClassUIData{
		Description: "", // TODO: API doesn't provide class description
	}

	// Build primary abilities description from the proficiency list
	if len(toolkitData.SavingThrows) > 0 {
		abilities := make([]string, len(toolkitData.SavingThrows))
		for i, ability := range toolkitData.SavingThrows {
			abilities[i] = string(ability)
		}
		uiData.PrimaryAbilitiesDescription = "Primary abilities: " + strings.Join(abilities, " and ")
	}

	return toolkitData, uiData
}

// convertKeyToClassID validates and converts an API key to a toolkit class constant
func convertKeyToClassID(key string) (constants.Class, error) {
	// Map of known API keys to toolkit constants
	knownClasses := map[string]constants.Class{
		"barbarian": constants.ClassBarbarian,
		"bard":      constants.ClassBard,
		"cleric":    constants.ClassCleric,
		"druid":     constants.ClassDruid,
		"fighter":   constants.ClassFighter,
		"monk":      constants.ClassMonk,
		"paladin":   constants.ClassPaladin,
		"ranger":    constants.ClassRanger,
		"rogue":     constants.ClassRogue,
		"sorcerer":  constants.ClassSorcerer,
		"warlock":   constants.ClassWarlock,
		"wizard":    constants.ClassWizard,
	}

	if classID, ok := knownClasses[key]; ok {
		return classID, nil
	}

	return "", fmt.Errorf("unknown class key: %s", key)
}

const (
	categoryMartialWeapons = "martial-weapons"
	categorySimpleWeapons  = "simple-weapons"
)

// detectEquipmentCategory tries to detect equipment category from description
func detectEquipmentCategory(description string) string {
	desc := strings.ToLower(description)

	// Map common descriptions to category IDs
	categoryMap := map[string]string{
		"martial weapon":     categoryMartialWeapons,
		"simple weapon":      categorySimpleWeapons,
		"artisan's tools":    "artisans-tools",
		"musical instrument": "musical-instruments",
		"holy symbol":        "holy-symbols",
		"druidic focus":      "druidic-foci",
		"arcane focus":       "arcane-foci",
	}

	// Check for exact matches or contains
	for key, categoryID := range categoryMap {
		if strings.Contains(desc, key) {
			return categoryID
		}
	}

	// Check for plurals
	if strings.Contains(desc, "martial weapons") {
		return categoryMartialWeapons
	}
	if strings.Contains(desc, "simple weapons") {
		return categorySimpleWeapons
	}

	return ""
}
