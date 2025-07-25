// Package rpgtoolkit provides the concrete implementation of the engine interface using rpg-toolkit modules.
package rpgtoolkit

import (
	"context"
	"fmt"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/dice"
	"github.com/KirkDiggler/rpg-toolkit/events"
)

// Adapter implements the engine.Engine interface using rpg-toolkit
type Adapter struct {
	eventBus       events.EventBus
	diceRoller     dice.Roller
	externalClient external.Client
}

// Skill source constants
const (
	skillSourceClass      = "class"
	skillSourceBackground = "background"
)

// Ability constants
const (
	abilityStrength     = "strength"
	abilityDexterity    = "dexterity"
	abilityConstitution = "constitution"
	abilityIntelligence = "intelligence"
	abilityWisdom       = "wisdom"
	abilityCharisma     = "charisma"
)

// AdapterConfig contains configuration for creating a new Adapter
type AdapterConfig struct {
	EventBus       events.EventBus
	DiceRoller     dice.Roller
	ExternalClient external.Client
}

// Validate checks that all required dependencies are provided
func (c *AdapterConfig) Validate() error {
	if c.EventBus == nil {
		return errors.InvalidArgument("event bus is required")
	}
	if c.DiceRoller == nil {
		return errors.InvalidArgument("dice roller is required")
	}
	if c.ExternalClient == nil {
		return errors.InvalidArgument("external client is required")
	}
	return nil
}

// NewAdapter creates a new rpg-toolkit engine adapter
func NewAdapter(cfg *AdapterConfig) (*Adapter, error) {
	if cfg == nil {
		return nil, errors.InvalidArgument("config is required")
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Adapter{
		eventBus:       cfg.EventBus,
		diceRoller:     cfg.DiceRoller,
		externalClient: cfg.ExternalClient,
	}, nil
}

// Verify that Adapter implements engine.Engine interface
var _ engine.Engine = (*Adapter)(nil)

// ValidateCharacter validates a complete character against D&D 5e rules
func (a *Adapter) ValidateCharacter(
	_ context.Context,
	input *engine.ValidateCharacterInput,
) (*engine.ValidateCharacterOutput, error) {
	if input == nil || input.Character == nil {
		return &engine.ValidateCharacterOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{Field: "character", Message: "Character is required", Code: "REQUIRED"},
			},
			Warnings: []engine.ValidationWarning{},
		}, nil
	}

	character := input.Character
	var errors []engine.ValidationError
	var warnings []engine.ValidationWarning

	// Validate ability scores
	if character.AbilityScores.Strength == 0 || character.AbilityScores.Dexterity == 0 ||
		character.AbilityScores.Constitution == 0 || character.AbilityScores.Intelligence == 0 ||
		character.AbilityScores.Wisdom == 0 || character.AbilityScores.Charisma == 0 {
		errors = append(errors, engine.ValidationError{
			Field:   "ability_scores",
			Message: "All ability scores must be set",
			Code:    "MISSING_ABILITY_SCORES",
		})
	}

	// Validate ability score ranges (3-20 for finalized characters, considering racial bonuses)
	validateAbilityScore := func(score int32, name string) {
		if score < 3 || score > 20 {
			errors = append(errors, engine.ValidationError{
				Field:   name,
				Message: fmt.Sprintf("Ability score must be between 3 and 20, got %d", score),
				Code:    "INVALID_ABILITY_SCORE_RANGE",
			})
		}
	}

	validateAbilityScore(character.AbilityScores.Strength, "strength")
	validateAbilityScore(character.AbilityScores.Dexterity, "dexterity")
	validateAbilityScore(character.AbilityScores.Constitution, "constitution")
	validateAbilityScore(character.AbilityScores.Intelligence, "intelligence")
	validateAbilityScore(character.AbilityScores.Wisdom, "wisdom")
	validateAbilityScore(character.AbilityScores.Charisma, "charisma")

	// Class-specific validation
	if input.Class != nil {
		// Check for suboptimal ability scores based on primary abilities
		for _, primaryAbility := range input.Class.PrimaryAbilities {
			var score int32
			switch strings.ToLower(primaryAbility) {
			case "strength":
				score = character.AbilityScores.Strength
			case "dexterity":
				score = character.AbilityScores.Dexterity
			case "constitution":
				score = character.AbilityScores.Constitution
			case "intelligence":
				score = character.AbilityScores.Intelligence
			case "wisdom":
				score = character.AbilityScores.Wisdom
			case "charisma":
				score = character.AbilityScores.Charisma
			}

			if score < 15 {
				warnings = append(warnings, engine.ValidationWarning{
					Field:   "ability_scores",
					Message: fmt.Sprintf("%s with %s below 15 is suboptimal", input.Class.Name, primaryAbility),
					Code:    "SUBOPTIMAL_PRIMARY_ABILITY",
				})
			}
		}
	}

	// Validate level
	if character.Level < 1 || character.Level > 20 {
		errors = append(errors, engine.ValidationError{
			Field:   "level",
			Message: "Character level must be between 1 and 20",
			Code:    "INVALID_LEVEL",
		})
	}

	// Validate hit points
	if character.CurrentHP < 0 {
		errors = append(errors, engine.ValidationError{
			Field:   "current_hp",
			Message: "Current hit points cannot be negative",
			Code:    "INVALID_CURRENT_HP",
		})
	}

	// Note: We can't validate CurrentHP against MaxHP because Character entity
	// doesn't store MaxHP - it's calculated by the engine when needed

	// Character is valid if there are no errors
	isValid := len(errors) == 0

	return &engine.ValidateCharacterOutput{
		IsValid:  isValid,
		Errors:   errors,
		Warnings: warnings,
	}, nil
}

// CalculateAbilityModifier calculates the D&D 5e ability modifier for a given score
func (a *Adapter) CalculateAbilityModifier(score int32) int32 {
	// D&D 5e formula: floor((score - 10) / 2)
	// In Go, integer division already floors for positive results
	// For negative results, we need to adjust
	modifier := (score - 10) / 2
	if score < 10 && (score-10)%2 != 0 {
		modifier-- // Adjust for proper floor behavior with negative odd numbers
	}
	return modifier
}

// CalculateProficiencyBonus calculates the D&D 5e proficiency bonus for a given level
func (a *Adapter) CalculateProficiencyBonus(level int32) int32 {
	if level <= 0 {
		return 0
	}
	// D&D 5e proficiency bonus: +2 at level 1-4, +3 at 5-8, +4 at 9-12, +5 at 13-16, +6 at 17-20
	return 2 + ((level - 1) / 4)
}

// extractMaxHitDie extracts the maximum value from a hit die string (e.g., "1d8" -> 8)
func extractMaxHitDie(hitDice string) int32 {
	// Expected format: "1d8", "1d10", etc.
	// Extract the number after 'd'
	if len(hitDice) < 3 || hitDice[0] != '1' || hitDice[1] != 'd' {
		return 6 // Default to d6 if invalid format
	}

	// Parse the die value
	switch hitDice[2:] {
	case "6":
		return 6
	case "8":
		return 8
	case "10":
		return 10
	case "12":
		return 12
	default:
		return 6 // Default to d6
	}
}

// calculateSavingThrows calculates all saving throw bonuses
func (a *Adapter) calculateSavingThrows(
	abilityScores *dnd5e.AbilityScores,
	proficientSaves []string,
	proficiencyBonus int32,
) map[string]int32 {
	if abilityScores == nil {
		return map[string]int32{}
	}

	// Create a map for proficient saves for quick lookup
	proficientMap := make(map[string]bool)
	for _, save := range proficientSaves {
		proficientMap[save] = true
	}

	// Calculate all saving throws
	savingThrows := map[string]int32{
		abilityStrength:     a.CalculateAbilityModifier(abilityScores.Strength),
		abilityDexterity:    a.CalculateAbilityModifier(abilityScores.Dexterity),
		abilityConstitution: a.CalculateAbilityModifier(abilityScores.Constitution),
		abilityIntelligence: a.CalculateAbilityModifier(abilityScores.Intelligence),
		abilityWisdom:       a.CalculateAbilityModifier(abilityScores.Wisdom),
		abilityCharisma:     a.CalculateAbilityModifier(abilityScores.Charisma),
	}

	// Add proficiency bonus to proficient saves
	for save := range savingThrows {
		if proficientMap[save] {
			savingThrows[save] += proficiencyBonus
		}
	}

	return savingThrows
}

// calculateSkillBonuses calculates all skill bonuses based on proficiencies
//
//nolint:unused // May be used when draft calculation is needed
func (a *Adapter) calculateSkillBonuses(
	ctx context.Context,
	draft *dnd5e.CharacterDraft,
	proficiencyBonus int32,
) map[string]int32 {
	if draft == nil || draft.AbilityScores == nil {
		return map[string]int32{}
	}

	// Get background data to determine background skills
	var backgroundSkills []string
	if draft.Background != nil {
		// Use hydrated background data if available
		backgroundSkills = draft.Background.SkillProficiencies
	} else if draft.BackgroundID != "" {
		// Fallback to fetching if not hydrated
		backgroundData, err := a.externalClient.GetBackgroundData(ctx, draft.BackgroundID)
		if err == nil && backgroundData != nil {
			backgroundSkills = backgroundData.SkillProficiencies
		}
	}

	// Create proficiency map from selected skills and background skills
	proficientSkills := make(map[string]bool)
	// TODO(#46): Extract skill selections from draft.ChoiceSelections
	// Add background skills (they're automatic proficiencies)
	for _, skillID := range backgroundSkills {
		proficientSkills[skillID] = true
	}

	// Calculate all skill bonuses
	allSkills := []string{
		"acrobatics", "animal_handling", "arcana", "athletics",
		"deception", "history", "insight", "intimidation",
		"investigation", "medicine", "nature", "perception",
		"performance", "persuasion", "religion", "sleight_of_hand",
		"stealth", "survival",
	}

	skillBonuses := make(map[string]int32)
	for _, skill := range allSkills {
		// Get ability for this skill
		ability := getSkillAbility(skill)

		// Calculate base modifier
		modifier := a.getAbilityModifier(draft.AbilityScores, ability)

		// Add proficiency if proficient
		if proficientSkills[skill] {
			modifier += proficiencyBonus
		}

		skillBonuses[skill] = modifier
	}

	return skillBonuses
}

// calculateSkillBonusesForCharacter calculates all skill bonuses for a finalized character
func (a *Adapter) calculateSkillBonusesForCharacter(
	_ context.Context,
	character *dnd5e.Character,
	input *engine.CalculateCharacterStatsInput,
	proficiencyBonus int32,
) map[string]int32 {
	// Create proficiency map from selected skills and background skills
	proficientSkills := make(map[string]bool)

	// Add background skills (they're automatic proficiencies)
	if input.Background != nil {
		for _, skillID := range input.Background.SkillProficiencies {
			proficientSkills[skillID] = true
		}
	}

	// TODO(#46): Extract skill selections from character's finalized choices
	// For now, we'll use a placeholder that assumes athletics and intimidation for fighters
	// This should be replaced with actual skill proficiencies stored on the character
	if character.ClassID == dnd5e.ClassFighter {
		proficientSkills["athletics"] = true
		proficientSkills["intimidation"] = true
	}

	// Calculate all skill bonuses
	allSkills := []string{
		"acrobatics", "animal_handling", "arcana", "athletics",
		"deception", "history", "insight", "intimidation",
		"investigation", "medicine", "nature", "perception",
		"performance", "persuasion", "religion", "sleight_of_hand",
		"stealth", "survival",
	}

	skillBonuses := make(map[string]int32)
	for _, skill := range allSkills {
		// Get ability for this skill
		ability := getSkillAbility(skill)

		// Calculate base modifier
		modifier := a.getAbilityModifier(&character.AbilityScores, ability)

		// Add proficiency if proficient
		if proficientSkills[skill] {
			modifier += proficiencyBonus
		}

		skillBonuses[skill] = modifier
	}

	return skillBonuses
}

// getAbilityModifier gets the modifier for a specific ability
func (a *Adapter) getAbilityModifier(scores *dnd5e.AbilityScores, ability string) int32 {
	switch ability {
	case abilityStrength:
		return a.CalculateAbilityModifier(scores.Strength)
	case abilityDexterity:
		return a.CalculateAbilityModifier(scores.Dexterity)
	case abilityConstitution:
		return a.CalculateAbilityModifier(scores.Constitution)
	case abilityIntelligence:
		return a.CalculateAbilityModifier(scores.Intelligence)
	case abilityWisdom:
		return a.CalculateAbilityModifier(scores.Wisdom)
	case abilityCharisma:
		return a.CalculateAbilityModifier(scores.Charisma)
	default:
		return 0
	}
}

// ValidateCharacterDraft validates draft choices are appropriate for selected race/class/background
func (a *Adapter) ValidateCharacterDraft(
	ctx context.Context,
	input *engine.ValidateCharacterDraftInput,
) (*engine.ValidateCharacterDraftOutput, error) {
	if input == nil || input.Draft == nil {
		return &engine.ValidateCharacterDraftOutput{
			IsValid:      false,
			IsComplete:   false,
			Errors:       []engine.ValidationError{{Field: "draft", Message: "Draft is required", Code: "REQUIRED"}},
			Warnings:     []engine.ValidationWarning{},
			MissingSteps: []string{},
		}, nil
	}

	draft := input.Draft
	var errors []engine.ValidationError
	var warnings []engine.ValidationWarning

	// Check completeness and get missing steps
	missingSteps := a.checkDraftCompleteness(draft)

	// Validate selected IDs exist
	idErrors := a.validateDraftIDs(ctx, draft)
	errors = append(errors, idErrors...)

	// Validate choices are appropriate for selected race/class/background
	if len(draft.ChoiceSelections) > 0 {
		choiceErrors := a.validateDraftChoices(ctx, draft)
		errors = append(errors, choiceErrors...)
	}

	// Check if draft is complete
	isComplete := len(missingSteps) == 0

	// Draft is valid if there are no errors
	isValid := len(errors) == 0

	return &engine.ValidateCharacterDraftOutput{
		IsComplete:   isComplete,
		IsValid:      isValid,
		Errors:       errors,
		Warnings:     warnings,
		MissingSteps: missingSteps,
	}, nil
}

// checkDraftCompleteness checks which required fields are missing from the draft
func (a *Adapter) checkDraftCompleteness(draft *dnd5e.CharacterDraft) []string {
	var missingSteps []string

	if draft.Name == "" {
		missingSteps = append(missingSteps, "name")
	}

	if draft.RaceID == "" {
		missingSteps = append(missingSteps, "race")
	}

	if draft.ClassID == "" {
		missingSteps = append(missingSteps, "class")
	}

	if draft.AbilityScores == nil {
		missingSteps = append(missingSteps, "ability_scores")
	}

	if draft.BackgroundID == "" {
		missingSteps = append(missingSteps, "background")
	}

	// TODO(#46): Check if skills have been selected from draft.ChoiceSelections
	// For now, assume skills are missing until we implement choice extraction
	missingSteps = append(missingSteps, "skills")

	return missingSteps
}

// validateDraftIDs validates that the selected race, class, and background IDs exist
func (a *Adapter) validateDraftIDs(ctx context.Context, draft *dnd5e.CharacterDraft) []engine.ValidationError {
	var errors []engine.ValidationError

	// Validate race ID
	if draft.RaceID != "" {
		_, err := a.externalClient.GetRaceData(ctx, draft.RaceID)
		if err != nil {
			errors = append(errors, engine.ValidationError{
				Field:   "race_id",
				Message: "Invalid race ID: " + draft.RaceID,
				Code:    "INVALID_RACE_ID",
			})
		}

		// If subrace is specified, validate it exists for this race
		if draft.SubraceID != "" {
			if err := a.validateSubraceForRace(ctx, draft.RaceID, draft.SubraceID); err != nil {
				errors = append(errors, engine.ValidationError{
					Field:   "subrace_id",
					Message: "Invalid subrace for selected race",
					Code:    "INVALID_SUBRACE_FOR_RACE",
				})
			}
		}
	}

	// Validate class ID
	if draft.ClassID != "" {
		_, err := a.externalClient.GetClassData(ctx, draft.ClassID)
		if err != nil {
			errors = append(errors, engine.ValidationError{
				Field:   "class_id",
				Message: "Invalid class ID: " + draft.ClassID,
				Code:    "INVALID_CLASS_ID",
			})
		}
	}

	// Validate background ID
	if draft.BackgroundID != "" {
		_, err := a.externalClient.GetBackgroundData(ctx, draft.BackgroundID)
		if err != nil {
			errors = append(errors, engine.ValidationError{
				Field:   "background_id",
				Message: "Invalid background ID: " + draft.BackgroundID,
				Code:    "INVALID_BACKGROUND_ID",
			})
		}
	}

	return errors
}

// validateSubraceForRace checks if a subrace is valid for the given race
func (a *Adapter) validateSubraceForRace(ctx context.Context, raceID, subraceID string) error {
	raceData, err := a.externalClient.GetRaceData(ctx, raceID)
	if err != nil {
		return err
	}

	for _, subrace := range raceData.Subraces {
		if subrace.ID == subraceID {
			return nil
		}
	}

	return errors.InvalidArgument("subrace not found for race")
}

// validateDraftChoices validates that choices are appropriate for the selected race/class/background
func (a *Adapter) validateDraftChoices(ctx context.Context, draft *dnd5e.CharacterDraft) []engine.ValidationError {
	var errors []engine.ValidationError

	// Group choices by source
	choicesBySource := make(map[string][]dnd5e.ChoiceSelection)
	for _, choice := range draft.ChoiceSelections {
		choicesBySource[string(choice.Source)] = append(choicesBySource[string(choice.Source)], choice)
	}

	// Validate race choices
	if draft.RaceID != "" && len(choicesBySource[string(dnd5e.ChoiceSourceRace)]) > 0 {
		raceErrors := a.validateRaceChoices(ctx, draft.RaceID, choicesBySource[string(dnd5e.ChoiceSourceRace)])
		errors = append(errors, raceErrors...)
	}

	// Validate class choices
	if draft.ClassID != "" && len(choicesBySource[string(dnd5e.ChoiceSourceClass)]) > 0 {
		classErrors := a.validateClassChoices(ctx, draft.ClassID, choicesBySource[string(dnd5e.ChoiceSourceClass)])
		errors = append(errors, classErrors...)
	}

	// Background choices validation would go here if backgrounds had choices
	// Currently backgrounds only provide automatic proficiencies

	return errors
}

// validateRaceChoices validates that race choices are valid for the given race
func (a *Adapter) validateRaceChoices(
	ctx context.Context,
	raceID string,
	choices []dnd5e.ChoiceSelection,
) []engine.ValidationError {
	var errors []engine.ValidationError

	raceData, err := a.externalClient.GetRaceData(ctx, raceID)
	if err != nil || raceData == nil {
		return errors
	}

	// Check each race choice is valid for this race
	for _, choice := range choices {
		validChoice := false
		for _, raceChoice := range raceData.Choices {
			if raceChoice.ID == choice.ChoiceID {
				validChoice = true
				break
			}
		}
		if !validChoice {
			errors = append(errors, engine.ValidationError{
				Field:   "choices",
				Message: fmt.Sprintf("Choice '%s' is not valid for race '%s'", choice.ChoiceID, raceID),
				Code:    "INVALID_RACE_CHOICE",
			})
		}
	}

	return errors
}

// validateClassChoices validates that class choices are valid for the given class
func (a *Adapter) validateClassChoices(
	ctx context.Context,
	classID string,
	choices []dnd5e.ChoiceSelection,
) []engine.ValidationError {
	var errors []engine.ValidationError

	classData, err := a.externalClient.GetClassData(ctx, classID)
	if err != nil || classData == nil {
		return errors
	}

	// Check each class choice is valid for this class
	for _, choice := range choices {
		validChoice := false
		for _, classChoice := range classData.Choices {
			if classChoice.ID == choice.ChoiceID {
				validChoice = true
				break
			}
		}
		if !validChoice {
			errors = append(errors, engine.ValidationError{
				Field:   "choices",
				Message: fmt.Sprintf("Choice '%s' is not valid for class '%s'", choice.ChoiceID, classID),
				Code:    "INVALID_CLASS_CHOICE",
			})
		}
	}

	return errors
}

// CalculateCharacterStats calculates derived character statistics
func (a *Adapter) CalculateCharacterStats(
	ctx context.Context,
	input *engine.CalculateCharacterStatsInput,
) (*engine.CalculateCharacterStatsOutput, error) {
	if input == nil || input.Character == nil {
		return nil, errors.InvalidArgument("character is required")
	}

	character := input.Character

	// Validate required fields
	if character.ClassID == "" {
		return nil, errors.InvalidArgument("class ID is required for stat calculation")
	}
	if character.RaceID == "" {
		return nil, errors.InvalidArgument("race ID is required for stat calculation")
	}

	// Validate required hydrated data
	if input.Class == nil {
		return nil, errors.InvalidArgument("class info is required for stat calculation")
	}
	if input.Race == nil {
		return nil, errors.InvalidArgument("race info is required for stat calculation")
	}

	// Calculate ability modifiers
	conModifier := a.CalculateAbilityModifier(character.AbilityScores.Constitution)
	dexModifier := a.CalculateAbilityModifier(character.AbilityScores.Dexterity)

	// Calculate Max HP (hit die max value + CON modifier at character level)
	maxHP := extractMaxHitDie(input.Class.HitDie) + conModifier
	if character.Level > 1 {
		// Additional HP for levels beyond 1st
		// TODO(#46): Implement proper HP calculation for higher levels
		maxHP += (character.Level - 1) * (extractMaxHitDie(input.Class.HitDie)/2 + 1 + conModifier)
	}

	// Calculate Armor Class (10 + DEX modifier, no armor)
	armorClass := 10 + dexModifier

	// Calculate Initiative (DEX modifier)
	initiative := dexModifier

	// Calculate Proficiency Bonus based on character level
	proficiencyBonus := a.CalculateProficiencyBonus(character.Level)

	// Calculate Saving Throws
	savingThrows := a.calculateSavingThrows(
		&character.AbilityScores,
		input.Class.SavingThrowProficiencies,
		proficiencyBonus)

	// Calculate Skill Bonuses
	skills := a.calculateSkillBonusesForCharacter(ctx, character, input, proficiencyBonus)

	return &engine.CalculateCharacterStatsOutput{
		MaxHP:            maxHP,
		ArmorClass:       armorClass,
		Initiative:       initiative,
		Speed:            input.Race.Speed,
		ProficiencyBonus: proficiencyBonus,
		SavingThrows:     savingThrows,
		Skills:           skills,
	}, nil
}

// ValidateRaceChoice validates a race selection
func (a *Adapter) ValidateRaceChoice(
	ctx context.Context,
	input *engine.ValidateRaceChoiceInput,
) (*engine.ValidateRaceChoiceOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	if input.RaceID == "" {
		return &engine.ValidateRaceChoiceOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{
					Field:   "race_id",
					Message: "Race ID is required",
					Code:    "REQUIRED",
				},
			},
		}, nil
	}

	// Fetch race data from external source
	raceData, err := a.externalClient.GetRaceData(ctx, input.RaceID)
	if err != nil {
		return &engine.ValidateRaceChoiceOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{
					Field:   "race_id",
					Message: "Invalid race ID or external data unavailable",
					Code:    "INVALID_RACE",
				},
			},
		}, nil
	}

	// Start with race traits and ability modifiers
	traits := make([]string, len(raceData.Traits))
	for i, trait := range raceData.Traits {
		traits[i] = trait.Name
	}

	abilityMods := make(map[string]int32)
	for ability, bonus := range raceData.AbilityBonuses {
		abilityMods[ability] = bonus
	}

	// If subrace is specified, validate it and add subrace bonuses
	if input.SubraceID != "" {
		subraceFound := false
		for _, subrace := range raceData.Subraces {
			if subrace.ID == input.SubraceID {
				subraceFound = true

				// Add subrace traits
				for _, trait := range subrace.Traits {
					traits = append(traits, trait.Name)
				}

				// Add subrace ability bonuses
				for ability, bonus := range subrace.AbilityBonuses {
					abilityMods[ability] += bonus
				}
				break
			}
		}

		if !subraceFound {
			return &engine.ValidateRaceChoiceOutput{
				IsValid: false,
				Errors: []engine.ValidationError{
					{
						Field:   "subrace_id",
						Message: "Invalid subrace for selected race",
						Code:    "INVALID_SUBRACE",
					},
				},
			}, nil
		}
	}

	return &engine.ValidateRaceChoiceOutput{
		IsValid:     true,
		Errors:      []engine.ValidationError{},
		RaceTraits:  traits,
		AbilityMods: abilityMods,
	}, nil
}

// ValidateClassChoice validates a class selection
func (a *Adapter) ValidateClassChoice(
	ctx context.Context,
	input *engine.ValidateClassChoiceInput,
) (*engine.ValidateClassChoiceOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	if input.ClassID == "" {
		return &engine.ValidateClassChoiceOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{
					Field:   "class_id",
					Message: "Class ID is required",
					Code:    "REQUIRED",
				},
			},
		}, nil
	}

	// Fetch class data from external source
	classData, err := a.externalClient.GetClassData(ctx, input.ClassID)
	if err != nil {
		return &engine.ValidateClassChoiceOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{
					Field:   "class_id",
					Message: "Invalid class ID or external data unavailable",
					Code:    "INVALID_CLASS",
				},
			},
		}, nil
	}

	var validationErrors []engine.ValidationError
	var warnings []engine.ValidationWarning

	// TODO(#36): Add specific multiclassing prerequisite validation
	// For now, we'll assume single-class character creation without prerequisites
	// Multiclassing prerequisites will be added when that feature is implemented
	//
	// Example logic for when multiclassing is implemented:
	// if input.AbilityScores != nil && classData.PrimaryAbility != "" {
	//     score := getAbilityScore(input.AbilityScores, classData.PrimaryAbility)
	//     if score < 13 { // Standard multiclassing requirement
	//         validationErrors = append(validationErrors, engine.ValidationError{
	//             Field:   "ability_scores",
	//             Message: fmt.Sprintf("Class requires %s 13+ for multiclassing", classData.PrimaryAbility),
	//             Code:    "INSUFFICIENT_ABILITY_SCORE",
	//         })
	//     }
	// }

	return &engine.ValidateClassChoiceOutput{
		IsValid:           len(validationErrors) == 0,
		Errors:            validationErrors,
		Warnings:          warnings,
		HitDice:           fmt.Sprintf("1d%d", classData.HitDice),
		PrimaryAbility:    strings.Join(classData.PrimaryAbilities, ", "),
		SavingThrows:      classData.SavingThrows,
		SkillChoicesCount: classData.SkillsCount,
		AvailableSkills:   classData.AvailableSkills,
	}, nil
}

// ValidateAbilityScores validates ability score generation
func (a *Adapter) ValidateAbilityScores(
	ctx context.Context,
	input *engine.ValidateAbilityScoresInput,
) (*engine.ValidateAbilityScoresOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	if input.AbilityScores == nil {
		return &engine.ValidateAbilityScoresOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{
					Field:   "ability_scores",
					Message: "Ability scores are required",
					Code:    "REQUIRED",
				},
			},
		}, nil
	}

	// Validate based on generation method
	switch input.Method {
	case engine.AbilityScoreMethodStandardArray:
		return a.validateStandardArray(ctx, input.AbilityScores)
	case engine.AbilityScoreMethodPointBuy:
		return a.validatePointBuy(ctx, input.AbilityScores)
	case engine.AbilityScoreMethodManual:
		return a.validateManualScores(ctx, input.AbilityScores)
	default:
		return &engine.ValidateAbilityScoresOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{
					Field:   "method",
					Message: "Invalid ability score generation method",
					Code:    "INVALID_METHOD",
				},
			},
		}, nil
	}
}

// ValidateSkillChoices validates skill selections
func (a *Adapter) ValidateSkillChoices(
	ctx context.Context,
	input *engine.ValidateSkillChoicesInput,
) (*engine.ValidateSkillChoicesOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	var validationErrors []engine.ValidationError

	// Validate class ID is provided
	if input.ClassID == "" {
		validationErrors = append(validationErrors, engine.ValidationError{
			Field:   "class_id",
			Message: "Class ID is required for skill validation",
			Code:    "REQUIRED",
		})
	}

	// If we have validation errors already, return early
	if len(validationErrors) > 0 {
		return &engine.ValidateSkillChoicesOutput{
			IsValid: false,
			Errors:  validationErrors,
		}, nil
	}

	// Fetch class data to get available skills and skill count
	classData, err := a.externalClient.GetClassData(ctx, input.ClassID)
	if err != nil {
		return &engine.ValidateSkillChoicesOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{
					Field:   "class_id",
					Message: "Invalid class ID or external data unavailable",
					Code:    "INVALID_CLASS",
				},
			},
		}, nil
	}

	// Track all available skills and their sources
	availableSkills := make(map[string]string) // skill -> source
	requiredSkillCount := classData.SkillsCount

	// Add class skills to available skills
	for _, skill := range classData.AvailableSkills {
		availableSkills[skill] = skillSourceClass
	}

	// If background is provided, fetch its data
	var backgroundSkills []string
	if input.BackgroundID != "" {
		backgroundData, err := a.externalClient.GetBackgroundData(ctx, input.BackgroundID)
		if err != nil {
			// Background fetch error is a warning, not a hard failure
			validationErrors = append(validationErrors, engine.ValidationError{
				Field:   "background_id",
				Message: "Invalid background ID or external data unavailable",
				Code:    "INVALID_BACKGROUND",
			})
		} else {
			// Background skills are automatic proficiencies, not choices
			backgroundSkills = backgroundData.SkillProficiencies
		}
	}

	// Validate selected skills
	selectedFromClass := int32(0)
	duplicateCheck := make(map[string]bool)

	for _, skillID := range input.SelectedSkillIDs {
		// Check for duplicates
		if duplicateCheck[skillID] {
			validationErrors = append(validationErrors, engine.ValidationError{
				Field:   "selected_skills",
				Message: "Duplicate skill selection: " + skillID,
				Code:    "DUPLICATE_SKILL",
			})
			continue
		}
		duplicateCheck[skillID] = true

		// Check if skill is available from class
		if source, ok := availableSkills[skillID]; ok && source == skillSourceClass {
			selectedFromClass++
		} else {
			// Check if it's a background skill (which would be automatic, not a choice)
			isBackgroundSkill := false
			for _, bgSkill := range backgroundSkills {
				if bgSkill == skillID {
					isBackgroundSkill = true
					break
				}
			}

			if isBackgroundSkill {
				validationErrors = append(validationErrors, engine.ValidationError{
					Field:   "selected_skills",
					Message: "Skill " + skillID + " is automatically granted by background, not a choice",
					Code:    "BACKGROUND_SKILL_NOT_CHOICE",
				})
			} else {
				validationErrors = append(validationErrors, engine.ValidationError{
					Field:   "selected_skills",
					Message: "Skill " + skillID + " is not available for this class",
					Code:    "INVALID_SKILL_CHOICE",
				})
			}
		}
	}

	// Validate skill count
	if selectedFromClass != requiredSkillCount {
		validationErrors = append(validationErrors, engine.ValidationError{
			Field: "selected_skills",
			Message: fmt.Sprintf("Must select exactly %d skills from class list, selected %d",
				requiredSkillCount, selectedFromClass),
			Code: "INCORRECT_SKILL_COUNT",
		})
	}

	// Generate warnings for optimization hints
	var warnings []engine.ValidationWarning
	if len(backgroundSkills) > 0 && len(validationErrors) == 0 {
		// Check if any selected skills overlap with background skills
		for _, selected := range input.SelectedSkillIDs {
			for _, bgSkill := range backgroundSkills {
				if selected == bgSkill {
					warnings = append(warnings, engine.ValidationWarning{
						Field: "selected_skills",
						Message: fmt.Sprintf("Skill %s is also provided by background - "+
							"consider choosing a different skill to maximize proficiencies", selected),
						Code: "SKILL_OVERLAP",
					})
				}
			}
		}
	}

	return &engine.ValidateSkillChoicesOutput{
		IsValid:  len(validationErrors) == 0,
		Errors:   validationErrors,
		Warnings: warnings,
	}, nil
}

// GetAvailableSkills returns available skill choices for class and background
func (a *Adapter) GetAvailableSkills(
	ctx context.Context,
	input *engine.GetAvailableSkillsInput,
) (*engine.GetAvailableSkillsOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	output := &engine.GetAvailableSkillsOutput{
		ClassSkills:      []engine.SkillChoice{},
		BackgroundSkills: []engine.SkillChoice{},
	}

	// Fetch class skills if class ID is provided
	if input.ClassID != "" {
		classData, err := a.externalClient.GetClassData(ctx, input.ClassID)
		if err != nil {
			// Return empty skills rather than error - let validation handle invalid IDs
			return output, nil
		}

		// Convert class available skills to SkillChoice structs
		for _, skillID := range classData.AvailableSkills {
			// For now, we'll use the skill ID as the name until we have a proper skill data source
			// In a real implementation, we'd fetch skill details from a skill data source
			skillChoice := engine.SkillChoice{
				SkillID:     skillID,
				SkillName:   formatSkillName(skillID),
				Description: fmt.Sprintf("Proficiency in %s", formatSkillName(skillID)),
				Ability:     getSkillAbility(skillID),
			}
			output.ClassSkills = append(output.ClassSkills, skillChoice)
		}
	}

	// Fetch background skills if background ID is provided
	if input.BackgroundID != "" {
		backgroundData, err := a.externalClient.GetBackgroundData(ctx, input.BackgroundID)
		if err != nil {
			// Return what we have rather than error
			return output, nil
		}

		// Convert background skill proficiencies to SkillChoice structs
		for _, skillID := range backgroundData.SkillProficiencies {
			skillChoice := engine.SkillChoice{
				SkillID:     skillID,
				SkillName:   formatSkillName(skillID),
				Description: fmt.Sprintf("Proficiency in %s (from background)", formatSkillName(skillID)),
				Ability:     getSkillAbility(skillID),
			}
			output.BackgroundSkills = append(output.BackgroundSkills, skillChoice)
		}
	}

	return output, nil
}

// formatSkillName converts a skill ID to a human-readable name
func formatSkillName(skillID string) string {
	// This is a simple implementation - in production this would come from skill data
	// Convert snake_case to Title Case
	formatted := ""
	capitalize := true
	for _, r := range skillID {
		switch {
		case r == '_' || r == '-':
			formatted += " "
			capitalize = true
		case capitalize && r >= 'a' && r <= 'z':
			formatted += string(r - ('a' - 'A'))
			capitalize = false
		default:
			formatted += string(r)
			capitalize = r == ' '
		}
	}
	return formatted
}

// getSkillAbility returns the associated ability for a skill
func getSkillAbility(skillID string) string {
	// D&D 5e skill to ability mapping
	// In production, this would come from skill data
	skillAbilityMap := map[string]string{
		"athletics":       "strength",
		"acrobatics":      "dexterity",
		"sleight_of_hand": "dexterity",
		"stealth":         "dexterity",
		"arcana":          "intelligence",
		"history":         "intelligence",
		"investigation":   "intelligence",
		"nature":          "intelligence",
		"religion":        "intelligence",
		"animal_handling": "wisdom",
		"insight":         "wisdom",
		"medicine":        "wisdom",
		"perception":      "wisdom",
		"survival":        "wisdom",
		"deception":       "charisma",
		"intimidation":    "charisma",
		"performance":     "charisma",
		"persuasion":      "charisma",
	}

	if ability, ok := skillAbilityMap[skillID]; ok {
		return ability
	}
	return "unknown"
}

// ValidateBackgroundChoice validates a background selection
func (a *Adapter) ValidateBackgroundChoice(
	ctx context.Context,
	input *engine.ValidateBackgroundChoiceInput,
) (*engine.ValidateBackgroundChoiceOutput, error) {
	if input == nil {
		return &engine.ValidateBackgroundChoiceOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{
					Field:   "input",
					Message: "Input is required",
					Code:    "REQUIRED",
				},
			},
		}, nil
	}

	if input.BackgroundID == "" {
		return &engine.ValidateBackgroundChoiceOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{
					Field:   "background_id",
					Message: "Background ID is required",
					Code:    "REQUIRED",
				},
			},
		}, nil
	}

	// Fetch background data from external source
	backgroundData, err := a.externalClient.GetBackgroundData(ctx, input.BackgroundID)
	if err != nil {
		return &engine.ValidateBackgroundChoiceOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{
					Field:   "background_id",
					Message: "Invalid background ID or external data unavailable",
					Code:    "INVALID_BACKGROUND",
				},
			},
		}, nil
	}

	if backgroundData == nil {
		return &engine.ValidateBackgroundChoiceOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{
					Field:   "background_id",
					Message: "Background not found",
					Code:    "NOT_FOUND",
				},
			},
		}, nil
	}

	// Convert background data to output format
	skillProficiencies := make([]string, len(backgroundData.SkillProficiencies))
	copy(skillProficiencies, backgroundData.SkillProficiencies)

	equipment := make([]string, len(backgroundData.Equipment))
	copy(equipment, backgroundData.Equipment)

	return &engine.ValidateBackgroundChoiceOutput{
		IsValid:            true,
		Errors:             []engine.ValidationError{},
		SkillProficiencies: skillProficiencies,
		Languages:          backgroundData.Languages,
		Equipment:          equipment,
	}, nil
}

// Compile-time check that our entity wrappers implement core.Entity
var (
	_ core.Entity = (*CharacterEntity)(nil)
	_ core.Entity = (*CharacterDraftEntity)(nil)
)

// validateStandardArray validates ability scores against the D&D 5e standard array
func (a *Adapter) validateStandardArray(
	_ context.Context,
	scores *dnd5e.AbilityScores,
) (*engine.ValidateAbilityScoresOutput, error) {
	// Standard array values: 15, 14, 13, 12, 10, 8
	standardArray := []int32{15, 14, 13, 12, 10, 8}

	// Get all ability scores
	actualScores := []int32{
		scores.Strength,
		scores.Dexterity,
		scores.Constitution,
		scores.Intelligence,
		scores.Wisdom,
		scores.Charisma,
	}

	// Sort both arrays for comparison
	sortedStandard := make([]int32, len(standardArray))
	copy(sortedStandard, standardArray)
	sortInt32Slice(sortedStandard)

	sortedActual := make([]int32, len(actualScores))
	copy(sortedActual, actualScores)
	sortInt32Slice(sortedActual)

	// Compare sorted arrays
	for i := range sortedStandard {
		if sortedStandard[i] != sortedActual[i] {
			return &engine.ValidateAbilityScoresOutput{
				IsValid: false,
				Errors: []engine.ValidationError{
					{
						Field:   "ability_scores",
						Message: "Ability scores must match the standard array: 15, 14, 13, 12, 10, 8",
						Code:    "INVALID_STANDARD_ARRAY",
					},
				},
			}, nil
		}
	}

	return &engine.ValidateAbilityScoresOutput{
		IsValid: true,
	}, nil
}

// validatePointBuy validates ability scores against D&D 5e point buy rules
func (a *Adapter) validatePointBuy(
	_ context.Context,
	scores *dnd5e.AbilityScores,
) (*engine.ValidateAbilityScoresOutput, error) {
	// Point buy: 27 points to spend, scores must be between 8-15
	// Cost: 8=0, 9=1, 10=2, 11=3, 12=4, 13=5, 14=7, 15=9
	pointCosts := map[int32]int32{
		8:  0,
		9:  1,
		10: 2,
		11: 3,
		12: 4,
		13: 5,
		14: 7,
		15: 9,
	}

	allScores := []int32{
		scores.Strength,
		scores.Dexterity,
		scores.Constitution,
		scores.Intelligence,
		scores.Wisdom,
		scores.Charisma,
	}

	totalCost := int32(0)
	errors := []engine.ValidationError{}

	// Validate each score and calculate total cost
	abilityNames := []string{"strength", "dexterity", "constitution", "intelligence", "wisdom", "charisma"}
	for i, score := range allScores {
		if score < 8 || score > 15 {
			errors = append(errors, engine.ValidationError{
				Field:   abilityNames[i],
				Message: "Point buy scores must be between 8 and 15",
				Code:    "INVALID_POINT_BUY_RANGE",
			})
			continue
		}

		cost, ok := pointCosts[score]
		if !ok {
			// Should not happen due to range check above
			continue
		}
		totalCost += cost
	}

	// Check total points spent
	if totalCost > 27 {
		errors = append(errors, engine.ValidationError{
			Field:   "ability_scores",
			Message: "Point buy total exceeds 27 points",
			Code:    "POINT_BUY_EXCEEDED",
		})
	}

	// Add warning if points are unspent
	warnings := []engine.ValidationWarning{}
	if totalCost < 27 && len(errors) == 0 {
		warnings = append(warnings, engine.ValidationWarning{
			Field:   "ability_scores",
			Message: "You have unspent point buy points",
			Code:    "UNSPENT_POINTS",
		})
	}

	return &engine.ValidateAbilityScoresOutput{
		IsValid:  len(errors) == 0,
		Errors:   errors,
		Warnings: warnings,
	}, nil
}

// validateManualScores validates manually entered ability scores
func (a *Adapter) validateManualScores(
	_ context.Context,
	scores *dnd5e.AbilityScores,
) (*engine.ValidateAbilityScoresOutput, error) {
	// Manual scores: must be between 3-18
	errors := []engine.ValidationError{}

	// Check each ability score
	validateScore := func(score int32, abilityName string) {
		if score < 3 || score > 18 {
			errors = append(errors, engine.ValidationError{
				Field:   abilityName,
				Message: "Ability scores must be between 3 and 18",
				Code:    "INVALID_ABILITY_SCORE_RANGE",
			})
		}
	}

	validateScore(scores.Strength, "strength")
	validateScore(scores.Dexterity, "dexterity")
	validateScore(scores.Constitution, "constitution")
	validateScore(scores.Intelligence, "intelligence")
	validateScore(scores.Wisdom, "wisdom")
	validateScore(scores.Charisma, "charisma")

	return &engine.ValidateAbilityScoresOutput{
		IsValid: len(errors) == 0,
		Errors:  errors,
	}, nil
}

// sortInt32Slice sorts a slice of int32 values in ascending order
func sortInt32Slice(slice []int32) {
	for i := 0; i < len(slice); i++ {
		for j := i + 1; j < len(slice); j++ {
			if slice[i] > slice[j] {
				slice[i], slice[j] = slice[j], slice[i]
			}
		}
	}
}
