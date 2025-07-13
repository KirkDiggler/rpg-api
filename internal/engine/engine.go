package engine

//go:generate mockgen -destination=mock/mock_engine.go -package=enginemock github.com/KirkDiggler/rpg-api/internal/engine Engine

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// Engine provides game mechanics and rules calculations
type Engine interface {
	// Character validation and calculations
	ValidateCharacterDraft(ctx context.Context, input *ValidateCharacterDraftInput) (*ValidateCharacterDraftOutput, error)
	CalculateCharacterStats(ctx context.Context, input *CalculateCharacterStatsInput) (*CalculateCharacterStatsOutput, error)

	// Race and class validation
	ValidateRaceChoice(ctx context.Context, input *ValidateRaceChoiceInput) (*ValidateRaceChoiceOutput, error)
	ValidateClassChoice(ctx context.Context, input *ValidateClassChoiceInput) (*ValidateClassChoiceOutput, error)

	// Ability score validation
	ValidateAbilityScores(ctx context.Context, input *ValidateAbilityScoresInput) (*ValidateAbilityScoresOutput, error)

	// Skill validation
	ValidateSkillChoices(ctx context.Context, input *ValidateSkillChoicesInput) (*ValidateSkillChoicesOutput, error)
	GetAvailableSkills(ctx context.Context, input *GetAvailableSkillsInput) (*GetAvailableSkillsOutput, error)

	// Background validation
	ValidateBackgroundChoice(ctx context.Context, input *ValidateBackgroundChoiceInput) (*ValidateBackgroundChoiceOutput, error)

	// Utility methods
	CalculateProficiencyBonus(level int32) int32
	CalculateAbilityModifier(score int32) int32
}

// ValidateCharacterDraftInput contains the draft to validate
type ValidateCharacterDraftInput struct {
	Draft *dnd5e.CharacterDraft
}

// ValidateCharacterDraftOutput contains validation results
type ValidateCharacterDraftOutput struct {
	IsComplete   bool
	IsValid      bool
	Errors       []ValidationError
	Warnings     []ValidationWarning
	MissingSteps []string
}

// CalculateCharacterStatsInput contains character data for stat calculation
type CalculateCharacterStatsInput struct {
	Draft *dnd5e.CharacterDraft
}

// CalculateCharacterStatsOutput contains calculated character stats
type CalculateCharacterStatsOutput struct {
	MaxHP            int32
	ArmorClass       int32
	Initiative       int32
	Speed            int32
	ProficiencyBonus int32
	SavingThrows     map[string]int32
	Skills           map[string]int32
}

// ValidateRaceChoiceInput contains race validation data
type ValidateRaceChoiceInput struct {
	RaceID    string
	SubraceID string
}

// ValidateRaceChoiceOutput contains race validation results
type ValidateRaceChoiceOutput struct {
	IsValid     bool
	Errors      []ValidationError
	RaceTraits  []string
	AbilityMods map[string]int32
}

// ValidateClassChoiceInput contains class validation data
type ValidateClassChoiceInput struct {
	ClassID       string
	AbilityScores *dnd5e.AbilityScores
}

// ValidateClassChoiceOutput contains class validation results
type ValidateClassChoiceOutput struct {
	IsValid           bool
	Errors            []ValidationError
	Warnings          []ValidationWarning
	HitDice           string
	PrimaryAbility    string
	SavingThrows      []string
	SkillChoicesCount int32
	AvailableSkills   []string
}

// ValidateAbilityScoresInput contains ability scores to validate
type ValidateAbilityScoresInput struct {
	AbilityScores *dnd5e.AbilityScores
	Method        string // "standard_array", "point_buy", "manual"
}

// ValidateAbilityScoresOutput contains ability score validation results
type ValidateAbilityScoresOutput struct {
	IsValid  bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// ValidateSkillChoicesInput contains skill choices to validate
type ValidateSkillChoicesInput struct {
	ClassID          string
	BackgroundID     string
	SelectedSkillIDs []string
}

// ValidateSkillChoicesOutput contains skill validation results
type ValidateSkillChoicesOutput struct {
	IsValid  bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// GetAvailableSkillsInput contains data to determine available skills
type GetAvailableSkillsInput struct {
	ClassID      string
	BackgroundID string
}

// GetAvailableSkillsOutput contains available skill choices
type GetAvailableSkillsOutput struct {
	ClassSkills      []SkillChoice
	BackgroundSkills []SkillChoice
}

// ValidateBackgroundChoiceInput contains background validation data
type ValidateBackgroundChoiceInput struct {
	BackgroundID string
}

// ValidateBackgroundChoiceOutput contains background validation results
type ValidateBackgroundChoiceOutput struct {
	IsValid            bool
	Errors             []ValidationError
	SkillProficiencies []string
	Languages          int32
	Equipment          []string
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
	Code    string
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	Field   string
	Message string
	Code    string
}

// SkillChoice represents an available skill choice
type SkillChoice struct {
	SkillID     string
	SkillName   string
	Description string
	Ability     string
}
