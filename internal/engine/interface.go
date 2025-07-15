// Package engine wraps the rpg toolkit
package engine

//go:generate mockgen -destination=mock/mock_engine.go -package=enginemock github.com/KirkDiggler/rpg-api/internal/engine Engine

import (
	"context"
)

// Engine provides game mechanics and rules calculations
type Engine interface {
	// Character validation and calculations
	ValidateCharacterDraft(ctx context.Context, input *ValidateCharacterDraftInput) (*ValidateCharacterDraftOutput, error)
	CalculateCharacterStats(
		ctx context.Context,
		input *CalculateCharacterStatsInput,
	) (*CalculateCharacterStatsOutput, error)

	// Race and class validation
	ValidateRaceChoice(ctx context.Context, input *ValidateRaceChoiceInput) (*ValidateRaceChoiceOutput, error)
	ValidateClassChoice(ctx context.Context, input *ValidateClassChoiceInput) (*ValidateClassChoiceOutput, error)

	// Ability score validation
	ValidateAbilityScores(ctx context.Context, input *ValidateAbilityScoresInput) (*ValidateAbilityScoresOutput, error)

	// Skill validation
	ValidateSkillChoices(ctx context.Context, input *ValidateSkillChoicesInput) (*ValidateSkillChoicesOutput, error)
	GetAvailableSkills(ctx context.Context, input *GetAvailableSkillsInput) (*GetAvailableSkillsOutput, error)

	// Background validation
	ValidateBackgroundChoice(
		ctx context.Context,
		input *ValidateBackgroundChoiceInput,
	) (*ValidateBackgroundChoiceOutput, error)

	// Utility methods
	CalculateProficiencyBonus(level int32) int32
	CalculateAbilityModifier(score int32) int32
}
