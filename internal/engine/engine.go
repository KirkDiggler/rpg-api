package engine

import "context"

type engine struct {
}

type Config struct {
}

func (cfg *Config) Validate() error {
	return nil
}

func New(cfg *Config) (Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &engine{}, nil
}

func (e *engine) CalculateAbilityModifier(score int32) int32 {
	return -1
}

func (e *engine) CalculateProficiencyBonus(level int32) int32 {
	return -1
}

func (e *engine) CalculateCharacterStats(
	ctx context.Context,
	input *CalculateCharacterStatsInput,
) (*CalculateCharacterStatsOutput, error) {
	return nil, nil
}

func (e *engine) ValidateCharacterDraft(
	ctx context.Context,
	input *ValidateCharacterDraftInput,
) (*ValidateCharacterDraftOutput, error) {
	return nil, nil
}

func (e *engine) ValidateRaceChoice(
	ctx context.Context,
	input *ValidateRaceChoiceInput,
) (*ValidateRaceChoiceOutput, error) {
	return nil, nil
}

func (e *engine) ValidateClassChoice(
	ctx context.Context,
	input *ValidateClassChoiceInput,
) (*ValidateClassChoiceOutput, error) {
	return nil, nil
}

func (e *engine) ValidateAbilityScores(
	ctx context.Context,
	input *ValidateAbilityScoresInput,
) (*ValidateAbilityScoresOutput, error) {
	return nil, nil
}

func (e *engine) ValidateSkillChoices(
	ctx context.Context,
	input *ValidateSkillChoicesInput,
) (*ValidateSkillChoicesOutput, error) {
	return nil, nil
}

func (e *engine) GetAvailableSkills(
	ctx context.Context,
	input *GetAvailableSkillsInput,
) (*GetAvailableSkillsOutput, error) {
	return nil, nil
}

func (e *engine) ValidateBackgroundChoice(
	ctx context.Context,
	input *ValidateBackgroundChoiceInput,
) (*ValidateBackgroundChoiceOutput, error) {
	return nil, nil
}
