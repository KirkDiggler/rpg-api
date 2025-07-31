package external

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/fadedpez/dnd5e-api/entities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
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
		raceID = constants.Race(apiRace.Key)
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
	toolkitData.AbilityScoreIncreases = make(map[constants.Ability]int)
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
	toolkitData.Languages = make([]constants.Language, 0, len(apiRace.Languages))
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
			ID:          constants.Subrace(subraceID),
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
func convertToAbilityConstant(key string) constants.Ability {
	switch strings.ToLower(key) {
	case "str":
		return constants.STR
	case "dex":
		return constants.DEX
	case "con":
		return constants.CON
	case "int":
		return constants.INT
	case "wis":
		return constants.WIS
	case "cha":
		return constants.CHA
	default:
		return ""
	}
}

func convertToLanguageConstant(key string) constants.Language {
	// Map API language keys to constants
	switch strings.ToLower(key) {
	case "common":
		return constants.LanguageCommon
	case "dwarvish":
		return constants.LanguageDwarvish
	case "elvish":
		return constants.LanguageElvish
	case "giant":
		return constants.LanguageGiant
	case "gnomish":
		return constants.LanguageGnomish
	case "goblin":
		return constants.LanguageGoblin
	case "halfling":
		return constants.LanguageHalfling
	case "orc":
		return constants.LanguageOrc
	// Add more mappings as needed
	default:
		return ""
	}
}

func convertToSkillConstant(name string) constants.Skill {
	// Map skill names to constants
	skillName := strings.ToLower(strings.TrimSpace(name))
	switch skillName {
	case "acrobatics":
		return constants.SkillAcrobatics
	case "animal handling":
		return constants.SkillAnimalHandling
	case "arcana":
		return constants.SkillArcana
	case "athletics":
		return constants.SkillAthletics
	case "deception":
		return constants.SkillDeception
	case "history":
		return constants.SkillHistory
	case "insight":
		return constants.SkillInsight
	case "intimidation":
		return constants.SkillIntimidation
	case "investigation":
		return constants.SkillInvestigation
	case "medicine":
		return constants.SkillMedicine
	case "nature":
		return constants.SkillNature
	case "perception":
		return constants.SkillPerception
	case "performance":
		return constants.SkillPerformance
	case "persuasion":
		return constants.SkillPersuasion
	case "religion":
		return constants.SkillReligion
	case "sleight of hand":
		return constants.SkillSleightOfHand
	case "stealth":
		return constants.SkillStealth
	case "survival":
		return constants.SkillSurvival
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
func convertKeyToRaceID(key string) (constants.Race, error) {
	// Map of known API keys to toolkit constants
	knownRaces := map[string]constants.Race{
		"dragonborn": constants.RaceDragonborn,
		"dwarf":      constants.RaceDwarf,
		"elf":        constants.RaceElf,
		"gnome":      constants.RaceGnome,
		"half-elf":   constants.RaceHalfElf,
		"halfling":   constants.RaceHalfling,
		"half-orc":   constants.RaceHalfOrc,
		"human":      constants.RaceHuman,
		"tiefling":   constants.RaceTiefling,
	}

	if raceID, ok := knownRaces[key]; ok {
		return raceID, nil
	}

	return "", fmt.Errorf("unknown race key: %s", key)
}