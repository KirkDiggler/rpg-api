package engine

import "github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"

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

// AbilityScoreMethod represents the method used to generate ability scores
type AbilityScoreMethod string

// Ability score generation methods
const (
	AbilityScoreMethodStandardArray AbilityScoreMethod = "standard_array"
	AbilityScoreMethodPointBuy      AbilityScoreMethod = "point_buy"
	AbilityScoreMethodManual        AbilityScoreMethod = "manual"
)

// ValidateAbilityScoresInput contains ability scores to validate
type ValidateAbilityScoresInput struct {
	AbilityScores *dnd5e.AbilityScores
	Method        AbilityScoreMethod
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
