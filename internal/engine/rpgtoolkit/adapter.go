// Package rpgtoolkit provides the concrete implementation of the engine interface using rpg-toolkit modules.
package rpgtoolkit

import (
	"context"
	"errors"

	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/dice"
	"github.com/KirkDiggler/rpg-toolkit/events"
)

// Adapter implements the engine.Engine interface using rpg-toolkit
type Adapter struct {
	eventBus   events.EventBus
	diceRoller dice.Roller
}

// AdapterConfig contains configuration for creating a new Adapter
type AdapterConfig struct {
	EventBus   events.EventBus
	DiceRoller dice.Roller
}

// Validate checks that all required dependencies are provided
func (c *AdapterConfig) Validate() error {
	if c.EventBus == nil {
		return errors.New("event bus is required")
	}
	if c.DiceRoller == nil {
		return errors.New("dice roller is required")
	}
	return nil
}

// NewAdapter creates a new rpg-toolkit engine adapter
func NewAdapter(cfg *AdapterConfig) (*Adapter, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Adapter{
		eventBus:   cfg.EventBus,
		diceRoller: cfg.DiceRoller,
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
	_ context.Context,
	input *engine.ValidateRaceChoiceInput,
) (*engine.ValidateRaceChoiceOutput, error) {
	// TODO(#36): Implement race validation using input.RaceID and input.SubraceID
	// Need D&D 5e race data in rpg-toolkit
	_ = input // Will be used in future implementation

	return &engine.ValidateRaceChoiceOutput{
		IsValid:     true, // TODO: Real validation
		Errors:      []engine.ValidationError{},
		RaceTraits:  []string{},         // TODO: Return actual race traits
		AbilityMods: map[string]int32{}, // TODO: Return ability score modifiers
	}, nil
}

// ValidateClassChoice validates a class selection
func (a *Adapter) ValidateClassChoice(
	_ context.Context,
	input *engine.ValidateClassChoiceInput,
) (*engine.ValidateClassChoiceOutput, error) {
	// TODO(#36): Implement class validation using input.ClassID and input.AbilityScores
	// Need D&D 5e class data in rpg-toolkit
	_ = input // Will be used in future implementation

	return &engine.ValidateClassChoiceOutput{
		IsValid:           true, // TODO: Real validation with ability score prerequisites
		Errors:            []engine.ValidationError{},
		Warnings:          []engine.ValidationWarning{},
		HitDice:           "1d8",      // TODO: Get from class data
		PrimaryAbility:    "strength", // TODO: Get from class data
		SavingThrows:      []string{}, // TODO: Get from class data
		SkillChoicesCount: 2,          // TODO: Get from class data
		AvailableSkills:   []string{}, // TODO: Get from class data
	}, nil
}

// ValidateAbilityScores validates ability score generation
func (a *Adapter) ValidateAbilityScores(
	_ context.Context,
	input *engine.ValidateAbilityScoresInput,
) (*engine.ValidateAbilityScoresOutput, error) {
	// TODO(#35): Implement ability score validation using input.AbilityScores and input.Method
	// Support standard array, point buy, and manual methods
	_ = input // Will be used in future implementation

	return &engine.ValidateAbilityScoresOutput{
		IsValid:  true, // TODO: Validate based on method
		Errors:   []engine.ValidationError{},
		Warnings: []engine.ValidationWarning{},
	}, nil
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
