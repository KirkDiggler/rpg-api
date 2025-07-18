package character_test

import (
	"context"
	"errors"

	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	characterorchestrator "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
)

// Section update tests

func (s *OrchestratorTestSuite) TestUpdateName() {
	testCases := []struct {
		name      string
		input     *characterorchestrator.UpdateNameInput
		setupMock func()
		wantErr   bool
		errMsg    string
		validate  func(*characterorchestrator.UpdateNameOutput)
	}{
		{
			name: "successful name update",
			input: &characterorchestrator.UpdateNameInput{
				DraftID: s.testDraftID,
				Name:    "Gandalf the White",
			},
			setupMock: func() {
				// Get existing draft
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: s.testDraft}, nil)

				// Update draft
				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						s.Equal("Gandalf the White", input.Draft.Name)
						s.True(input.Draft.Progress.HasName())
						// Repository returns the updated draft
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
			wantErr: false,
			validate: func(output *characterorchestrator.UpdateNameOutput) {
				s.Equal("Gandalf the White", output.Draft.Name)
				s.Empty(output.Warnings)
			},
		},
		{
			name:      "nil input",
			input:     nil,
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "input is required",
		},
		{
			name: "empty name",
			input: &characterorchestrator.UpdateNameInput{
				DraftID: s.testDraftID,
				Name:    "",
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "validation failed: name: is required",
		},
		{
			name: "draft not found",
			input: &characterorchestrator.UpdateNameInput{
				DraftID: "nonexistent",
				Name:    "Test",
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: "nonexistent"}).
					Return(nil, errors.New("not found"))
			},
			wantErr: true,
			errMsg:  "failed to get draft",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.UpdateName(s.ctx, tc.input)

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

func (s *OrchestratorTestSuite) TestUpdateRace() {
	testCases := []struct {
		name      string
		input     *characterorchestrator.UpdateRaceInput
		setupMock func()
		wantErr   bool
		errMsg    string
		validate  func(*characterorchestrator.UpdateRaceOutput)
	}{
		{
			name: "successful race update",
			input: &characterorchestrator.UpdateRaceInput{
				DraftID: s.testDraftID,
				RaceID:  dnd5e.RaceElf,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: s.testDraft}, nil)

				// Engine validates race choice
				s.mockEngine.EXPECT().
					ValidateRaceChoice(s.ctx, &engine.ValidateRaceChoiceInput{
						RaceID:    dnd5e.RaceElf,
						SubraceID: "",
					}).
					Return(&engine.ValidateRaceChoiceOutput{
						IsValid: true,
						AbilityMods: map[string]int32{
							"dexterity": 2,
						},
					}, nil)

				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
			wantErr: false,
			validate: func(output *characterorchestrator.UpdateRaceOutput) {
				s.Equal(dnd5e.RaceElf, output.Draft.RaceID)
				s.Empty(output.Warnings)
			},
		},
		{
			name: "invalid race choice",
			input: &characterorchestrator.UpdateRaceInput{
				DraftID:   s.testDraftID,
				RaceID:    "invalid-race",
				SubraceID: "invalid-subrace",
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: s.testDraft}, nil)

				s.mockEngine.EXPECT().
					ValidateRaceChoice(s.ctx, gomock.Any()).
					Return(&engine.ValidateRaceChoiceOutput{
						IsValid: false,
						Errors: []engine.ValidationError{
							{
								Field:   "race",
								Message: "Invalid race selection",
								Code:    "INVALID_RACE",
							},
						},
					}, nil)

				// Still expect update even with validation errors (converted to warnings)
				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
			wantErr: false, // Returns warnings, not error
			validate: func(output *characterorchestrator.UpdateRaceOutput) {
				s.Len(output.Warnings, 1)
				s.Equal("Invalid race selection", output.Warnings[0].Message)
			},
		},
		{
			name: "engine error",
			input: &characterorchestrator.UpdateRaceInput{
				DraftID: s.testDraftID,
				RaceID:  dnd5e.RaceElf,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: s.testDraft}, nil)

				s.mockEngine.EXPECT().
					ValidateRaceChoice(s.ctx, gomock.Any()).
					Return(nil, errors.New("engine error"))
			},
			wantErr: true,
			errMsg:  "failed to validate race",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.UpdateRace(s.ctx, tc.input)

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

func (s *OrchestratorTestSuite) TestUpdateClass() {
	draftWithAbilityScores := &dnd5e.CharacterDraft{
		ID:        s.testDraftID,
		PlayerID:  s.testPlayerID,
		SessionID: s.testSessionID,
		Name:      "Test Character",
		AbilityScores: &dnd5e.AbilityScores{
			Strength:     8,
			Dexterity:    14,
			Constitution: 12,
			Intelligence: 16,
			Wisdom:       13,
			Charisma:     10,
		},
		StartingSkillIDs: []string{dnd5e.SkillArcana}, // Has existing skills
	}

	testCases := []struct {
		name      string
		input     *characterorchestrator.UpdateClassInput
		draft     *dnd5e.CharacterDraft
		setupMock func()
		wantErr   bool
		validate  func(*characterorchestrator.UpdateClassOutput)
	}{
		{
			name: "successful class update",
			input: &characterorchestrator.UpdateClassInput{
				DraftID: s.testDraftID,
				ClassID: dnd5e.ClassWizard,
			},
			draft: draftWithAbilityScores,
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: draftWithAbilityScores}, nil)

				s.mockEngine.EXPECT().
					ValidateClassChoice(s.ctx, &engine.ValidateClassChoiceInput{
						ClassID:       dnd5e.ClassWizard,
						AbilityScores: draftWithAbilityScores.AbilityScores,
					}).
					Return(&engine.ValidateClassChoiceOutput{
						IsValid:           true,
						HitDice:           "1d6",
						PrimaryAbility:    "intelligence",
						SavingThrows:      []string{"intelligence", "wisdom"},
						SkillChoicesCount: 2,
					}, nil)

				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						// Verify skills were cleared
						s.Empty(input.Draft.StartingSkillIDs)
						s.False(input.Draft.Progress.HasSkills())
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
			wantErr: false,
			validate: func(output *characterorchestrator.UpdateClassOutput) {
				s.Equal(dnd5e.ClassWizard, output.Draft.ClassID)
				s.Empty(output.Warnings)
			},
		},
		{
			name: "class with ability score warnings",
			input: &characterorchestrator.UpdateClassInput{
				DraftID: s.testDraftID,
				ClassID: dnd5e.ClassBarbarian,
			},
			draft: draftWithAbilityScores,
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: draftWithAbilityScores}, nil)

				s.mockEngine.EXPECT().
					ValidateClassChoice(s.ctx, gomock.Any()).
					Return(&engine.ValidateClassChoiceOutput{
						IsValid: false,
						Errors: []engine.ValidationError{
							{
								Field:   "strength",
								Message: "Barbarians require 13+ Strength",
								Code:    "ABILITY_REQUIREMENT",
							},
						},
						Warnings: []engine.ValidationWarning{
							{
								Field:   "constitution",
								Message: "Barbarians benefit from high Constitution",
								Code:    "RECOMMENDATION",
							},
						},
					}, nil)

				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
			wantErr: false,
			validate: func(output *characterorchestrator.UpdateClassOutput) {
				s.Len(output.Warnings, 2)
				// Check both errors converted to warnings and actual warnings
				foundReq := false
				foundRec := false
				for _, w := range output.Warnings {
					if w.Message == "Barbarians require 13+ Strength" {
						foundReq = true
					}
					if w.Message == "Barbarians benefit from high Constitution" {
						foundRec = true
					}
				}
				s.True(foundReq, "Should have requirement warning")
				s.True(foundRec, "Should have recommendation warning")
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.UpdateClass(s.ctx, tc.input)

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

func (s *OrchestratorTestSuite) TestUpdateAbilityScores() {
	draftWithClass := &dnd5e.CharacterDraft{
		ID:        s.testDraftID,
		PlayerID:  s.testPlayerID,
		SessionID: s.testSessionID,
		ClassID:   dnd5e.ClassWizard,
	}

	testCases := []struct {
		name      string
		input     *characterorchestrator.UpdateAbilityScoresInput
		draft     *dnd5e.CharacterDraft
		setupMock func()
		wantErr   bool
		validate  func(*characterorchestrator.UpdateAbilityScoresOutput)
	}{
		{
			name: "valid ability scores",
			input: &characterorchestrator.UpdateAbilityScoresInput{
				DraftID: s.testDraftID,
				AbilityScores: dnd5e.AbilityScores{
					Strength:     15,
					Dexterity:    14,
					Constitution: 13,
					Intelligence: 12,
					Wisdom:       10,
					Charisma:     8,
				},
			},
			draft: s.testDraft, // Has ClassID = wizard
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: s.testDraft}, nil)

				s.mockEngine.EXPECT().
					ValidateAbilityScores(s.ctx, gomock.Any()).
					Return(&engine.ValidateAbilityScoresOutput{
						IsValid: true,
					}, nil)

				// Since draft has ClassID, it will revalidate class requirements
				s.mockEngine.EXPECT().
					ValidateClassChoice(s.ctx, &engine.ValidateClassChoiceInput{
						ClassID: dnd5e.ClassWizard,
						AbilityScores: &dnd5e.AbilityScores{
							Strength:     15,
							Dexterity:    14,
							Constitution: 13,
							Intelligence: 12,
							Wisdom:       10,
							Charisma:     8,
						},
					}).
					Return(&engine.ValidateClassChoiceOutput{
						IsValid: true,
					}, nil)

				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
			wantErr: false,
			validate: func(output *characterorchestrator.UpdateAbilityScoresOutput) {
				s.Equal(int32(15), output.Draft.AbilityScores.Strength)
				s.Empty(output.Warnings)
			},
		},
		{
			name: "revalidates class requirements",
			input: &characterorchestrator.UpdateAbilityScoresInput{
				DraftID: s.testDraftID,
				AbilityScores: dnd5e.AbilityScores{
					Strength:     10,
					Dexterity:    10,
					Constitution: 10,
					Intelligence: 8, // Too low for wizard
					Wisdom:       10,
					Charisma:     10,
				},
			},
			draft: draftWithClass,
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: draftWithClass}, nil)

				s.mockEngine.EXPECT().
					ValidateAbilityScores(s.ctx, gomock.Any()).
					Return(&engine.ValidateAbilityScoresOutput{
						IsValid: true,
					}, nil)

				// Revalidates class with new scores
				s.mockEngine.EXPECT().
					ValidateClassChoice(s.ctx, gomock.Any()).
					Return(&engine.ValidateClassChoiceOutput{
						IsValid: false,
						Errors: []engine.ValidationError{
							{
								Field:   "intelligence",
								Message: "Wizards require 13+ Intelligence",
								Code:    "ABILITY_REQUIREMENT",
							},
						},
					}, nil)

				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
			wantErr: false,
			validate: func(output *characterorchestrator.UpdateAbilityScoresOutput) {
				s.Len(output.Warnings, 1)
				s.Equal("class_requirements", output.Warnings[0].Field)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.UpdateAbilityScores(s.ctx, tc.input)

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

func (s *OrchestratorTestSuite) TestUpdateSkills() {
	completePrereqsDraft := &dnd5e.CharacterDraft{
		ID:           s.testDraftID,
		PlayerID:     s.testPlayerID,
		SessionID:    s.testSessionID,
		ClassID:      dnd5e.ClassRogue,
		BackgroundID: dnd5e.BackgroundCriminal,
	}

	testCases := []struct {
		name      string
		input     *characterorchestrator.UpdateSkillsInput
		draft     *dnd5e.CharacterDraft
		setupMock func()
		wantErr   bool
		validate  func(*characterorchestrator.UpdateSkillsOutput)
	}{
		{
			name: "successful skill selection",
			input: &characterorchestrator.UpdateSkillsInput{
				DraftID: s.testDraftID,
				SkillIDs: []string{
					dnd5e.SkillStealth,
					dnd5e.SkillDeception,
					dnd5e.SkillInvestigation,
					dnd5e.SkillPerception,
				},
			},
			draft: completePrereqsDraft,
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: completePrereqsDraft}, nil)

				s.mockEngine.EXPECT().
					ValidateSkillChoices(s.ctx, &engine.ValidateSkillChoicesInput{
						ClassID:      dnd5e.ClassRogue,
						BackgroundID: dnd5e.BackgroundCriminal,
						SelectedSkillIDs: []string{
							dnd5e.SkillStealth,
							dnd5e.SkillDeception,
							dnd5e.SkillInvestigation,
							dnd5e.SkillPerception,
						},
					}).
					Return(&engine.ValidateSkillChoicesOutput{
						IsValid: true,
					}, nil)

				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
			wantErr: false,
			validate: func(output *characterorchestrator.UpdateSkillsOutput) {
				s.Len(output.Draft.StartingSkillIDs, 4)
				s.Empty(output.Warnings)
			},
		},
		{
			name: "missing prerequisites",
			input: &characterorchestrator.UpdateSkillsInput{
				DraftID:  s.testDraftID,
				SkillIDs: []string{dnd5e.SkillStealth},
			},
			draft: s.testDraft, // No background set
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: s.testDraft}, nil)

				// Still expect update even with missing prerequisites (converted to warnings)
				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
			wantErr: false,
			validate: func(output *characterorchestrator.UpdateSkillsOutput) {
				s.Len(output.Warnings, 1)
				s.Equal("MISSING_PREREQUISITES", output.Warnings[0].Type)
			},
		},
		{
			name: "invalid skill choices",
			input: &characterorchestrator.UpdateSkillsInput{
				DraftID: s.testDraftID,
				SkillIDs: []string{
					dnd5e.SkillArcana, // Not available to rogues
					dnd5e.SkillReligion,
				},
			},
			draft: completePrereqsDraft,
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: completePrereqsDraft}, nil)

				s.mockEngine.EXPECT().
					ValidateSkillChoices(s.ctx, gomock.Any()).
					Return(&engine.ValidateSkillChoicesOutput{
						IsValid: false,
						Errors: []engine.ValidationError{
							{
								Field:   "skills",
								Message: "Invalid skill selections for class",
								Code:    "INVALID_SKILLS",
							},
						},
					}, nil)

				// Still expect update even with invalid skills (converted to warnings)
				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
			wantErr: false,
			validate: func(output *characterorchestrator.UpdateSkillsOutput) {
				s.Len(output.Warnings, 1)
				s.Equal("INVALID_SKILLS", output.Warnings[0].Type)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.UpdateSkills(s.ctx, tc.input)

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
