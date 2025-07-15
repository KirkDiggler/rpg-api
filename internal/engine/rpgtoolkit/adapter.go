// Package rpgtoolkit provides the concrete implementation of the engine interface using rpg-toolkit modules.
package rpgtoolkit

import (
	"context"

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

// ValidateCharacterDraft validates a character draft for completeness and rule compliance
func (a *Adapter) ValidateCharacterDraft(
	_ context.Context,
	input *engine.ValidateCharacterDraftInput,
) (*engine.ValidateCharacterDraftOutput, error) {
	// TODO(#39): Implement comprehensive character draft validation using input.Draft
	// For now, return placeholder validation
	_ = input // Will be used in future implementation

	return &engine.ValidateCharacterDraftOutput{
		IsComplete:   false, // TODO: Check draft completeness
		IsValid:      true,  // TODO: Validate all rules
		Errors:       []engine.ValidationError{},
		Warnings:     []engine.ValidationWarning{},
		MissingSteps: []string{}, // TODO: Return missing creation steps
	}, nil
}

// CalculateCharacterStats calculates derived character statistics
func (a *Adapter) CalculateCharacterStats(
	_ context.Context,
	input *engine.CalculateCharacterStatsInput,
) (*engine.CalculateCharacterStatsOutput, error) {
	// TODO(#38): Implement character stat calculations using input.Draft
	// For now, return placeholder calculations
	_ = input // Will be used in future implementation

	return &engine.CalculateCharacterStatsOutput{
		MaxHP:            10,                             // TODO: Calculate based on class hit die + CON modifier
		ArmorClass:       10,                             // TODO: Calculate based on armor + DEX modifier
		Initiative:       0,                              // TODO: Calculate DEX modifier
		Speed:            30,                             // TODO: Get from race data
		ProficiencyBonus: a.CalculateProficiencyBonus(1), // TODO: Use actual level
		SavingThrows:     map[string]int32{},             // TODO: Calculate with proficiencies
		Skills:           map[string]int32{},             // TODO: Calculate with proficiencies
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
	copy(traits, raceData.Traits)

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
				traits = append(traits, subrace.Traits...)

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
		HitDice:           classData.HitDice,
		PrimaryAbility:    classData.PrimaryAbility,
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
	_ context.Context,
	input *engine.ValidateSkillChoicesInput,
) (*engine.ValidateSkillChoicesOutput, error) {
	// TODO(#37): Implement skill validation using input.SelectedSkillIDs
	// Need proficiency system integration
	_ = input // Will be used in future implementation

	return &engine.ValidateSkillChoicesOutput{
		IsValid:  true, // TODO: Validate skill choices
		Errors:   []engine.ValidationError{},
		Warnings: []engine.ValidationWarning{},
	}, nil
}

// GetAvailableSkills returns available skill choices for class and background
func (a *Adapter) GetAvailableSkills(
	_ context.Context,
	input *engine.GetAvailableSkillsInput,
) (*engine.GetAvailableSkillsOutput, error) {
	// TODO(#37): Implement skill availability using input.ClassID and input.BackgroundID
	// Need D&D 5e skill data and class/background integration
	_ = input // Will be used in future implementation

	return &engine.GetAvailableSkillsOutput{
		ClassSkills:      []engine.SkillChoice{}, // TODO: Return class skills
		BackgroundSkills: []engine.SkillChoice{}, // TODO: Return background skills
	}, nil
}

// ValidateBackgroundChoice validates a background selection
func (a *Adapter) ValidateBackgroundChoice(
	_ context.Context,
	input *engine.ValidateBackgroundChoiceInput,
) (*engine.ValidateBackgroundChoiceOutput, error) {
	// TODO(#36): Implement background validation using input.BackgroundID
	// Need D&D 5e background data in rpg-toolkit
	_ = input // Will be used in future implementation

	return &engine.ValidateBackgroundChoiceOutput{
		IsValid:            true, // TODO: Real validation
		Errors:             []engine.ValidationError{},
		SkillProficiencies: []string{}, // TODO: Return background skills
		Languages:          0,          // TODO: Return language choices
		Equipment:          []string{}, // TODO: Return starting equipment
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
