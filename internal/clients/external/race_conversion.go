package external

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/fadedpez/dnd5e-api/entities"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/languages"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
)

// convertRaceToHybrid converts API race data to both toolkit format and UI data
func convertRaceToHybrid(apiRace *entities.Race) (*race.Data, *RaceUIData) {
	if apiRace == nil {
		return nil, nil
	}

	// Convert API key to toolkit constant, validating it exists
	raceID, err := convertKeyToRaceID(apiRace.Key)
	if err != nil {
		// Log warning but continue with the raw key
		// This allows us to handle new races from the API that we don't have constants for yet
		slog.Warn("Unknown race key from API, using raw key",
			"key", apiRace.Key,
			"name", apiRace.Name,
			"error", err)
		raceID = races.Race(apiRace.Key)
	}

	// Convert to toolkit format
	toolkitData := &race.Data{
		ID:          raceID,
		Name:        apiRace.Name,
		Description: "", // API doesn't provide a general description
		Size:        apiRace.Size,
		Speed:       apiRace.Speed,
	}

	// Convert ability score increases
	toolkitData.AbilityScoreIncreases = make(map[abilities.Ability]int)
	for _, bonus := range apiRace.AbilityBonuses {
		if bonus.AbilityScore != nil {
			// Convert ability name to constant
			ability := convertToAbilityConstant(bonus.AbilityScore.Key)
			if ability != "" {
				toolkitData.AbilityScoreIncreases[ability] = bonus.Bonus
			}
		}
	}

	// Convert traits
	toolkitData.Traits = make([]race.TraitData, len(apiRace.Traits))
	for i, trait := range apiRace.Traits {
		toolkitData.Traits[i] = race.TraitData{
			ID:          generateSlug(trait.Name),
			Name:        trait.Name,
			Description: "", // Would need to fetch full trait details
		}
	}

	// Convert languages
	toolkitData.Languages = make([]languages.Language, 0, len(apiRace.Languages))
	for _, lang := range apiRace.Languages {
		if langConst := convertToLanguageConstant(lang.Key); langConst != "" {
			toolkitData.Languages = append(toolkitData.Languages, langConst)
		}
	}

	// Convert proficiencies
	for _, prof := range apiRace.StartingProficiencies {
		// Determine proficiency type from name
		profName := prof.Name
		if strings.Contains(strings.ToLower(profName), "skill:") {
			// Handle skill proficiencies
			skillName := strings.TrimSpace(strings.TrimPrefix(profName, "Skill:"))
			if skill := convertToSkillConstant(skillName); skill != "" {
				toolkitData.SkillProficiencies = append(toolkitData.SkillProficiencies, skill)
			}
		} else if isWeaponProficiency(profName) {
			toolkitData.WeaponProficiencies = append(toolkitData.WeaponProficiencies, profName)
		} else if isToolProficiency(profName) {
			toolkitData.ToolProficiencies = append(toolkitData.ToolProficiencies, profName)
		}
	}

	// Convert language options
	if apiRace.LanguageOptions != nil {
		toolkitData.LanguageChoice = &race.ChoiceData{
			ID:          "language_choice",
			Type:        "language",
			Choose:      apiRace.LanguageOptions.ChoiceCount,
			Description: apiRace.LanguageOptions.Description,
		}
		// Extract options
		if apiRace.LanguageOptions.OptionList != nil {
			for _, option := range apiRace.LanguageOptions.OptionList.Options {
				if refOpt, ok := option.(*entities.ReferenceOption); ok && refOpt.Reference != nil {
					toolkitData.LanguageChoice.From = append(toolkitData.LanguageChoice.From, refOpt.Reference.Key)
				}
			}
		}
	}

	// Convert proficiency options
	if apiRace.StartingProficiencyOptions != nil {
		// Determine the choice type from description
		choiceType := "proficiency"
		desc := strings.ToLower(apiRace.StartingProficiencyOptions.Description)
		if strings.Contains(desc, "skill") {
			choiceType = "skill"
			toolkitData.SkillChoice = &race.ChoiceData{
				ID:          "skill_choice",
				Type:        choiceType,
				Choose:      apiRace.StartingProficiencyOptions.ChoiceCount,
				Description: apiRace.StartingProficiencyOptions.Description,
			}
			if apiRace.StartingProficiencyOptions.OptionList != nil {
				for _, option := range apiRace.StartingProficiencyOptions.OptionList.Options {
					if refOpt, ok := option.(*entities.ReferenceOption); ok && refOpt.Reference != nil {
						toolkitData.SkillChoice.From = append(toolkitData.SkillChoice.From, refOpt.Reference.Key)
					}
				}
			}
		} else if strings.Contains(desc, "tool") {
			choiceType = "tool"
			toolkitData.ToolChoice = &race.ChoiceData{
				ID:          "tool_choice",
				Type:        choiceType,
				Choose:      apiRace.StartingProficiencyOptions.ChoiceCount,
				Description: apiRace.StartingProficiencyOptions.Description,
			}
			if apiRace.StartingProficiencyOptions.OptionList != nil {
				for _, option := range apiRace.StartingProficiencyOptions.OptionList.Options {
					if refOpt, ok := option.(*entities.ReferenceOption); ok && refOpt.Reference != nil {
						toolkitData.ToolChoice.From = append(toolkitData.ToolChoice.From, refOpt.Reference.Key)
					}
				}
			}
		}
	}

	// Convert subraces
	toolkitData.Subraces = make([]race.SubraceData, len(apiRace.SubRaces))
	for i, subrace := range apiRace.SubRaces {
		subraceID := fromAPIFormat(subrace.Key, "SUBRACE")
		toolkitData.Subraces[i] = race.SubraceData{
			ID:          races.Subrace(subraceID),
			Name:        subrace.Name,
			Description: "", // Would need to fetch full subrace details
		}
	}

	// Extract UI data
	uiData := &RaceUIData{
		SizeDescription:      apiRace.SizeDescription,
		AgeDescription:       "", // TODO: API doesn't provide this field
		AlignmentDescription: "", // TODO: API doesn't provide this field
	}

	return toolkitData, uiData
}

// Helper functions to convert to constants
func convertToAbilityConstant(key string) abilities.Ability {
	switch strings.ToLower(key) {
	case "str":
		return abilities.STR
	case "dex":
		return abilities.DEX
	case "con":
		return abilities.CON
	case "int":
		return abilities.INT
	case "wis":
		return abilities.WIS
	case "cha":
		return abilities.CHA
	default:
		return ""
	}
}

func convertToLanguageConstant(key string) languages.Language {
	// Map API language keys to constants
	switch strings.ToLower(key) {
	case "common":
		return languages.Common
	case "dwarvish":
		return languages.Dwarvish
	case "elvish":
		return languages.Elvish
	case "giant":
		return languages.Giant
	case "gnomish":
		return languages.Gnomish
	case "goblin":
		return languages.Goblin
	case "halfling":
		return languages.Halfling
	case "orc":
		return languages.Orc
	// Add more mappings as needed
	default:
		return ""
	}
}

func convertToSkillConstant(name string) skills.Skill {
	// Map skill names to constants
	skillName := strings.ToLower(strings.TrimSpace(name))
	switch skillName {
	case "acrobatics":
		return skills.Acrobatics
	case "animal handling":
		return skills.AnimalHandling
	case "arcana":
		return skills.Arcana
	case "athletics":
		return skills.Athletics
	case "deception":
		return skills.Deception
	case "history":
		return skills.History
	case "insight":
		return skills.Insight
	case "intimidation":
		return skills.Intimidation
	case "investigation":
		return skills.Investigation
	case "medicine":
		return skills.Medicine
	case "nature":
		return skills.Nature
	case "perception":
		return skills.Perception
	case "performance":
		return skills.Performance
	case "persuasion":
		return skills.Persuasion
	case "religion":
		return skills.Religion
	case "sleight of hand":
		return skills.SleightOfHand
	case "stealth":
		return skills.Stealth
	case "survival":
		return skills.Survival
	default:
		return ""
	}
}

func isWeaponProficiency(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "sword") ||
		strings.Contains(lower, "axe") ||
		strings.Contains(lower, "hammer") ||
		strings.Contains(lower, "bow") ||
		strings.Contains(lower, "crossbow") ||
		strings.Contains(lower, "dagger") ||
		strings.Contains(lower, "mace") ||
		strings.Contains(lower, "spear") ||
		strings.Contains(lower, "weapon")
}

func isToolProficiency(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "tools") ||
		strings.Contains(lower, "supplies") ||
		strings.Contains(lower, "kit") ||
		strings.Contains(lower, "instruments")
}

// convertKeyToRaceID validates and converts an API key to a toolkit race constant
func convertKeyToRaceID(key string) (races.Race, error) {
	// Use the toolkit's All map to validate races
	if raceID, ok := races.All[key]; ok {
		return raceID, nil
	}

	return "", fmt.Errorf("unknown race key: %s", key)
}
