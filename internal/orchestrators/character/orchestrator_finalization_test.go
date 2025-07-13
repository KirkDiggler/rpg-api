package character_test

import (
	"context"
	"errors"

	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/services/character"
)

// Validation and finalization tests

func (s *OrchestratorTestSuite) TestValidateDraft() {
	testCases := []struct {
		name      string
		input     *character.ValidateDraftInput
		setupMock func()
		wantErr   bool
		validate  func(*character.ValidateDraftOutput)
	}{
		{
			name: "complete and valid draft",
			input: &character.ValidateDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				completeDraft := &dnd5e.CharacterDraft{
					ID:           s.testDraftID,
					PlayerID:     s.testPlayerID,
					Name:         "Complete Character",
					RaceID:       dnd5e.RaceHuman,
					ClassID:      dnd5e.ClassFighter,
					BackgroundID: dnd5e.BackgroundSoldier,
					AbilityScores: &dnd5e.AbilityScores{
						Strength:     16,
						Dexterity:    14,
						Constitution: 15,
						Intelligence: 10,
						Wisdom:       12,
						Charisma:     8,
					},
					StartingSkillIDs: []string{dnd5e.SkillAthletics, dnd5e.SkillIntimidation},
				}

				s.mockDraftRepo.EXPECT().
					Get(s.ctx, s.testDraftID).
					Return(completeDraft, nil)

				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, &engine.ValidateCharacterDraftInput{
						Draft: completeDraft,
					}).
					Return(&engine.ValidateCharacterDraftOutput{
						IsComplete:   true,
						IsValid:      true,
						Errors:       []engine.ValidationError{},
						Warnings:     []engine.ValidationWarning{},
						MissingSteps: []string{},
					}, nil)
			},
			wantErr: false,
			validate: func(output *character.ValidateDraftOutput) {
				s.True(output.IsComplete)
				s.True(output.IsValid)
				s.Empty(output.Errors)
				s.Empty(output.Warnings)
				s.Empty(output.MissingSteps)
			},
		},
		{
			name: "incomplete draft",
			input: &character.ValidateDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, s.testDraftID).
					Return(s.testDraft, nil)

				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterDraftOutput{
						IsComplete: false,
						IsValid:    false,
						Errors: []engine.ValidationError{
							{
								Field:   "ability_scores",
								Message: "Ability scores not set",
								Code:    "MISSING_ABILITY_SCORES",
							},
						},
						MissingSteps: []string{
							"ability_scores",
							"skills",
							"background",
						},
					}, nil)
			},
			wantErr: false,
			validate: func(output *character.ValidateDraftOutput) {
				s.False(output.IsComplete)
				s.False(output.IsValid)
				s.Len(output.Errors, 1)
				s.Len(output.MissingSteps, 3)
			},
		},
		{
			name: "validation with warnings",
			input: &character.ValidateDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, s.testDraftID).
					Return(s.testDraft, nil)

				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterDraftOutput{
						IsComplete: true,
						IsValid:    true,
						Warnings: []engine.ValidationWarning{
							{
								Field:   "class_ability_scores",
								Message: "Wizard with Intelligence below 16 is suboptimal",
								Code:    "RECOMMENDATION",
							},
						},
					}, nil)
			},
			wantErr: false,
			validate: func(output *character.ValidateDraftOutput) {
				s.True(output.IsComplete)
				s.True(output.IsValid)
				s.Len(output.Warnings, 1)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.ValidateDraft(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
				s.NotNil(output)
				if tc.validate != nil {
					tc.validate(output)
				}
			}
		})
	}
}

func (s *OrchestratorTestSuite) TestFinalizeDraft() {
	completeDraft := &dnd5e.CharacterDraft{
		ID:           s.testDraftID,
		PlayerID:     s.testPlayerID,
		SessionID:    s.testSessionID,
		Name:         "Aragorn",
		RaceID:       dnd5e.RaceHuman,
		ClassID:      dnd5e.ClassRanger,
		BackgroundID: dnd5e.BackgroundOutlander,
		Alignment:    dnd5e.AlignmentNeutralGood,
		AbilityScores: &dnd5e.AbilityScores{
			Strength:     16,
			Dexterity:    14,
			Constitution: 15,
			Intelligence: 10,
			Wisdom:       13,
			Charisma:     12,
		},
		StartingSkillIDs: []string{dnd5e.SkillSurvival, dnd5e.SkillAnimalHandling},
	}

	testCases := []struct {
		name      string
		input     *character.FinalizeDraftInput
		setupMock func()
		wantErr   bool
		errMsg    string
		validate  func(*character.FinalizeDraftOutput)
	}{
		{
			name: "successful finalization",
			input: &character.FinalizeDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, s.testDraftID).
					Return(completeDraft, nil)

				// Validate draft
				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterDraftOutput{
						IsComplete: true,
						IsValid:    true,
					}, nil)

				// Calculate stats
				s.mockEngine.EXPECT().
					CalculateCharacterStats(s.ctx, &engine.CalculateCharacterStatsInput{
						Draft: completeDraft,
					}).
					Return(&engine.CalculateCharacterStatsOutput{
						MaxHP:            12, // 10 + CON mod
						ArmorClass:       11, // 10 + DEX mod
						Initiative:       2,
						Speed:            30,
						ProficiencyBonus: 2,
					}, nil)

				// Create character
				s.mockCharRepo.EXPECT().
					Create(s.ctx, gomock.Any()).
					DoAndReturn(func(ctx context.Context, char *dnd5e.Character) error {
						s.Equal("Aragorn", char.Name)
						s.Equal(int32(1), char.Level)
						s.Equal(int32(12), char.CurrentHP)
						s.Equal(dnd5e.ClassRanger, char.ClassID)
						return nil
					})

				// Delete draft
				s.mockDraftRepo.EXPECT().
					Delete(s.ctx, s.testDraftID).
					Return(nil)
			},
			wantErr: false,
			validate: func(output *character.FinalizeDraftOutput) {
				s.NotNil(output.Character)
				s.True(output.DraftDeleted)
				s.Equal("Aragorn", output.Character.Name)
				s.Equal(int32(1), output.Character.Level)
			},
		},
		{
			name: "incomplete draft",
			input: &character.FinalizeDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, s.testDraftID).
					Return(s.testDraft, nil) // Missing ability scores

				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterDraftOutput{
						IsComplete:   false,
						MissingSteps: []string{"ability_scores", "background"},
					}, nil)
			},
			wantErr: true,
			errMsg:  "cannot finalize incomplete draft",
		},
		{
			name: "invalid draft",
			input: &character.FinalizeDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, s.testDraftID).
					Return(completeDraft, nil)

				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterDraftOutput{
						IsComplete: true,
						IsValid:    false,
						Errors: []engine.ValidationError{
							{Message: "Invalid skill selection"},
						},
					}, nil)
			},
			wantErr: true,
			errMsg:  "cannot finalize invalid draft",
		},
		{
			name: "draft deletion fails",
			input: &character.FinalizeDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, s.testDraftID).
					Return(completeDraft, nil)

				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterDraftOutput{
						IsComplete: true,
						IsValid:    true,
					}, nil)

				s.mockEngine.EXPECT().
					CalculateCharacterStats(s.ctx, gomock.Any()).
					Return(&engine.CalculateCharacterStatsOutput{
						MaxHP: 12,
					}, nil)

				s.mockCharRepo.EXPECT().
					Create(s.ctx, gomock.Any()).
					Return(nil)

				// Draft deletion fails
				s.mockDraftRepo.EXPECT().
					Delete(s.ctx, s.testDraftID).
					Return(errors.New("delete failed"))
			},
			wantErr: false, // Character creation succeeded, so we don't fail
			validate: func(output *character.FinalizeDraftOutput) {
				s.NotNil(output.Character)
				s.False(output.DraftDeleted) // Draft not deleted
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.FinalizeDraft(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
			} else {
				s.NoError(err)
				s.NotNil(output)
				if tc.validate != nil {
					tc.validate(output)
				}
			}
		})
	}
}

// Character operation tests

func (s *OrchestratorTestSuite) TestGetCharacter() {
	testCases := []struct {
		name      string
		input     *character.GetCharacterInput
		setupMock func()
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful retrieval",
			input: &character.GetCharacterInput{
				CharacterID: s.testCharacterID,
			},
			setupMock: func() {
				s.mockCharRepo.EXPECT().
					Get(s.ctx, s.testCharacterID).
					Return(s.testCharacter, nil)
			},
			wantErr: false,
		},
		{
			name:      "nil input",
			input:     nil,
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "input is required",
		},
		{
			name: "character not found",
			input: &character.GetCharacterInput{
				CharacterID: "nonexistent",
			},
			setupMock: func() {
				s.mockCharRepo.EXPECT().
					Get(s.ctx, "nonexistent").
					Return(nil, errors.New("not found"))
			},
			wantErr: true,
			errMsg:  "failed to get character",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.GetCharacter(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
			} else {
				s.NoError(err)
				s.NotNil(output)
				s.Equal(s.testCharacter, output.Character)
			}
		})
	}
}

func (s *OrchestratorTestSuite) TestDeleteCharacter() {
	testCases := []struct {
		name      string
		input     *character.DeleteCharacterInput
		setupMock func()
		wantErr   bool
	}{
		{
			name: "successful deletion",
			input: &character.DeleteCharacterInput{
				CharacterID: s.testCharacterID,
			},
			setupMock: func() {
				s.mockCharRepo.EXPECT().
					Delete(s.ctx, s.testCharacterID).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name: "repository error",
			input: &character.DeleteCharacterInput{
				CharacterID: s.testCharacterID,
			},
			setupMock: func() {
				s.mockCharRepo.EXPECT().
					Delete(s.ctx, s.testCharacterID).
					Return(errors.New("database error"))
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.DeleteCharacter(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
				s.NotNil(output)
				s.Contains(output.Message, "deleted successfully")
			}
		})
	}
}
