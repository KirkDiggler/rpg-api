package character_test

import (
	"context"
	"errors"

	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	characterorchestrator "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
)

// Validation and finalization tests

func (s *OrchestratorTestSuite) TestValidateDraft() {
	testCases := []struct {
		name      string
		input     *characterorchestrator.ValidateDraftInput
		setupMock func()
		wantErr   bool
		validate  func(*characterorchestrator.ValidateDraftOutput)
	}{
		{
			name: "complete and valid draft",
			input: &characterorchestrator.ValidateDraftInput{
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
					// Skills are now handled through ChoiceSelections
				}

				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(completeDraft)}, nil)

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
			validate: func(output *characterorchestrator.ValidateDraftOutput) {
				s.True(output.IsComplete)
				s.True(output.IsValid)
				s.Empty(output.Errors)
				s.Empty(output.Warnings)
				s.Empty(output.MissingSteps)
			},
		},
		{
			name: "incomplete draft",
			input: &characterorchestrator.ValidateDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(s.testDraft)}, nil)

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
			validate: func(output *characterorchestrator.ValidateDraftOutput) {
				s.False(output.IsComplete)
				s.False(output.IsValid)
				s.Len(output.Errors, 1)
				s.Len(output.MissingSteps, 3)
			},
		},
		{
			name: "validation with warnings",
			input: &characterorchestrator.ValidateDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(s.testDraft)}, nil)

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
			validate: func(output *characterorchestrator.ValidateDraftOutput) {
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
		// Skills are now handled through ChoiceSelections
		ChoiceSelections: []dnd5e.ChoiceSelection{
			{
				ChoiceID:     "ranger-skills",
				ChoiceType:   dnd5e.ChoiceTypeSkill,
				Source:       dnd5e.ChoiceSourceClass,
				SelectedKeys: []string{dnd5e.SkillSurvival, dnd5e.SkillPerception, dnd5e.SkillAnimalHandling},
			},
			{
				ChoiceID:     "outlander-language",
				ChoiceType:   dnd5e.ChoiceTypeLanguage,
				Source:       dnd5e.ChoiceSourceBackground,
				SelectedKeys: []string{dnd5e.LanguageElvish},
			},
		},
	}

	testCases := []struct {
		name      string
		input     *characterorchestrator.FinalizeDraftInput
		setupMock func()
		wantErr   bool
		errMsg    string
		validate  func(*characterorchestrator.FinalizeDraftOutput)
	}{
		{
			name: "successful finalization",
			input: &characterorchestrator.FinalizeDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(completeDraft)}, nil)

				// Validate draft
				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterDraftOutput{
						IsComplete: true,
						IsValid:    true,
					}, nil)

				// Mock external client calls for hydrateDraft
				s.mockExternalClient.EXPECT().
					GetRaceData(s.ctx, dnd5e.RaceHuman).
					Return(&external.RaceData{
						ID:    dnd5e.RaceHuman,
						Name:  "Human",
						Speed: 30,
						AbilityBonuses: map[string]int32{
							"strength":     1,
							"dexterity":    1,
							"constitution": 1,
							"intelligence": 1,
							"wisdom":       1,
							"charisma":     1,
						},
						Languages: []string{dnd5e.LanguageCommon},
					}, nil)

				s.mockExternalClient.EXPECT().
					GetClassData(s.ctx, dnd5e.ClassRanger).
					Return(&external.ClassData{
						ID:               dnd5e.ClassRanger,
						Name:             "Ranger",
						HitDice:          "1d10",
						SavingThrows:     []string{"strength", "dexterity"},
						PrimaryAbilities: []string{"dexterity", "wisdom"},
						SkillsCount:      3,
						AvailableSkills: []string{
							"animal_handling", "athletics", "insight", "investigation",
							"nature", "perception", "stealth", "survival",
						},
						ArmorProficiencies:  []string{"light armor", "medium armor", "shields"},
						WeaponProficiencies: []string{"simple weapons", "martial weapons"},
						ToolProficiencies:   []string{},
						StartingEquipment: []string{
							"scale mail", "two shortswords", "simple melee weapon",
							"explorer's pack", "longbow and quiver of 20 arrows",
						},
						StartingEquipmentOptions: []*external.EquipmentChoiceData{},
						ProficiencyChoices:       []*external.ChoiceData{},
						LevelOneFeatures:         []*external.FeatureData{},
						Spellcasting:             nil,
						Choices:                  []dnd5e.Choice{},
					}, nil)

				s.mockExternalClient.EXPECT().
					GetBackgroundData(s.ctx, dnd5e.BackgroundOutlander).
					Return(&external.BackgroundData{
						ID:                 dnd5e.BackgroundOutlander,
						Name:               "Outlander",
						Description:        "You grew up in the wilds",
						SkillProficiencies: []string{dnd5e.SkillAthletics, dnd5e.SkillSurvival},
						Languages:          1,
						Equipment:          []string{"staff", "hunting trap", "traveler's clothes", "belt pouch"},
						Feature:            "Wanderer",
					}, nil)

				// Calculate stats
				s.mockEngine.EXPECT().
					CalculateCharacterStats(s.ctx, gomock.Any()).
					DoAndReturn(func(
						_ context.Context,
						input *engine.CalculateCharacterStatsInput,
					) (*engine.CalculateCharacterStatsOutput, error) {
						// Verify the input has the character and hydrated info
						s.NotNil(input.Character)
						s.Equal(completeDraft.Name, input.Character.Name)
						s.Equal(completeDraft.ClassID, input.Character.ClassID)
						s.Equal(completeDraft.RaceID, input.Character.RaceID)
						s.NotNil(input.Race)
						s.NotNil(input.Class)
						s.NotNil(input.Background)

						// Verify choices were compiled
						// Human gets +1 to all abilities
						s.Equal(int32(17), input.Character.AbilityScores.Strength)     // 16 + 1
						s.Equal(int32(15), input.Character.AbilityScores.Dexterity)    // 14 + 1
						s.Equal(int32(16), input.Character.AbilityScores.Constitution) // 15 + 1
						s.Equal(int32(11), input.Character.AbilityScores.Intelligence) // 10 + 1
						s.Equal(int32(14), input.Character.AbilityScores.Wisdom)       // 13 + 1
						s.Equal(int32(13), input.Character.AbilityScores.Charisma)     // 12 + 1

						// Verify skill proficiencies include both background and choices
						s.Contains(input.Character.SkillProficiencies, dnd5e.SkillAthletics)      // From background
						s.Contains(input.Character.SkillProficiencies, dnd5e.SkillSurvival)       // From background + choice
						s.Contains(input.Character.SkillProficiencies, dnd5e.SkillPerception)     // From choice
						s.Contains(input.Character.SkillProficiencies, dnd5e.SkillAnimalHandling) // From choice

						// Verify languages
						s.Contains(input.Character.Languages, dnd5e.LanguageCommon) // From human
						s.Contains(input.Character.Languages, dnd5e.LanguageElvish) // From choice

						// Verify class proficiencies
						s.Contains(input.Character.ArmorProficiencies, "light armor")
						s.Contains(input.Character.ArmorProficiencies, "medium armor")
						s.Contains(input.Character.ArmorProficiencies, "shields")
						s.Contains(input.Character.WeaponProficiencies, "simple weapons")
						s.Contains(input.Character.WeaponProficiencies, "martial weapons")
						s.Contains(input.Character.SavingThrows, "strength")
						s.Contains(input.Character.SavingThrows, "dexterity")

						return &engine.CalculateCharacterStatsOutput{
							MaxHP:            12, // 10 + CON mod
							ArmorClass:       11, // 10 + DEX mod
							Initiative:       2,
							Speed:            30,
							ProficiencyBonus: 2,
						}, nil
					})

				// Validate character
				s.mockEngine.EXPECT().
					ValidateCharacter(s.ctx, gomock.Any()).
					DoAndReturn(func(
						_ context.Context,
						input *engine.ValidateCharacterInput,
					) (*engine.ValidateCharacterOutput, error) {
						// Verify the input has the character and hydrated info
						s.NotNil(input.Character)
						s.Equal(int32(12), input.Character.CurrentHP)
						s.NotNil(input.Race)
						s.NotNil(input.Class)
						s.NotNil(input.Background)

						return &engine.ValidateCharacterOutput{
							IsValid:  true,
							Errors:   []engine.ValidationError{},
							Warnings: []engine.ValidationWarning{},
						}, nil
					})

				// Create character
				s.mockCharRepo.EXPECT().
					Create(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
						s.Equal("Aragorn", input.Character.Name)
						s.Equal(int32(1), input.Character.Level)
						s.Equal(int32(12), input.Character.CurrentHP)
						s.Equal(dnd5e.ClassRanger, input.Character.ClassID)
						// Repository returns the character with ID and timestamps set
						char := *input.Character
						char.ID = "generated-char-id"
						return &characterrepo.CreateOutput{Character: &char}, nil
					})

				// Delete draft
				s.mockDraftRepo.EXPECT().
					Delete(s.ctx, draftrepo.DeleteInput{ID: s.testDraftID}).
					Return(&draftrepo.DeleteOutput{}, nil)
			},
			wantErr: false,
			validate: func(output *characterorchestrator.FinalizeDraftOutput) {
				s.NotNil(output.Character)
				s.True(output.DraftDeleted)
				s.Equal("Aragorn", output.Character.Name)
				s.Equal(int32(1), output.Character.Level)
			},
		},
		{
			name: "incomplete draft",
			input: &characterorchestrator.FinalizeDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(s.testDraft)}, nil) // Missing ability scores

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
			input: &characterorchestrator.FinalizeDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(completeDraft)}, nil)

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
			input: &characterorchestrator.FinalizeDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(completeDraft)}, nil)

				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterDraftOutput{
						IsComplete: true,
						IsValid:    true,
					}, nil)

				// Mock external client calls for hydrateDraft
				s.mockExternalClient.EXPECT().
					GetRaceData(s.ctx, gomock.Any()).
					Return(&external.RaceData{
						ID:    dnd5e.RaceHuman,
						Name:  "Human",
						Speed: 30,
					}, nil)

				s.mockExternalClient.EXPECT().
					GetClassData(s.ctx, gomock.Any()).
					Return(&external.ClassData{
						ID:           dnd5e.ClassRanger,
						Name:         "Ranger",
						HitDice:      "1d10",
						SavingThrows: []string{"strength", "dexterity"},
					}, nil)

				s.mockExternalClient.EXPECT().
					GetBackgroundData(s.ctx, gomock.Any()).
					Return(&external.BackgroundData{
						ID:                 dnd5e.BackgroundOutlander,
						Name:               "Outlander",
						SkillProficiencies: []string{"athletics", "survival"},
					}, nil)

				s.mockEngine.EXPECT().
					CalculateCharacterStats(s.ctx, gomock.Any()).
					Return(&engine.CalculateCharacterStatsOutput{
						MaxHP: 12,
					}, nil)

				s.mockEngine.EXPECT().
					ValidateCharacter(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterOutput{
						IsValid:  true,
						Errors:   []engine.ValidationError{},
						Warnings: []engine.ValidationWarning{},
					}, nil)

				s.mockCharRepo.EXPECT().
					Create(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
						// Repository returns the character with ID and timestamps set
						char := *input.Character
						char.ID = "generated-char-id"
						return &characterrepo.CreateOutput{Character: &char}, nil
					})

				// Draft deletion fails
				s.mockDraftRepo.EXPECT().
					Delete(s.ctx, draftrepo.DeleteInput{ID: s.testDraftID}).
					Return(nil, errors.New("delete failed"))
			},
			wantErr: false, // Character creation succeeded, so we don't fail
			validate: func(output *characterorchestrator.FinalizeDraftOutput) {
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
		input     *characterorchestrator.GetCharacterInput
		setupMock func()
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful retrieval",
			input: &characterorchestrator.GetCharacterInput{
				CharacterID: s.testCharacterID,
			},
			setupMock: func() {
				s.mockCharRepo.EXPECT().
					Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
					Return(&characterrepo.GetOutput{Character: s.testCharacter}, nil)
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
			input: &characterorchestrator.GetCharacterInput{
				CharacterID: "nonexistent",
			},
			setupMock: func() {
				s.mockCharRepo.EXPECT().
					Get(s.ctx, characterrepo.GetInput{ID: "nonexistent"}).
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
		input     *characterorchestrator.DeleteCharacterInput
		setupMock func()
		wantErr   bool
	}{
		{
			name: "successful deletion",
			input: &characterorchestrator.DeleteCharacterInput{
				CharacterID: s.testCharacterID,
			},
			setupMock: func() {
				s.mockCharRepo.EXPECT().
					Delete(s.ctx, characterrepo.DeleteInput{ID: s.testCharacterID}).
					Return(&characterrepo.DeleteOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name: "repository error",
			input: &characterorchestrator.DeleteCharacterInput{
				CharacterID: s.testCharacterID,
			},
			setupMock: func() {
				s.mockCharRepo.EXPECT().
					Delete(s.ctx, characterrepo.DeleteInput{ID: s.testCharacterID}).
					Return(nil, errors.New("database error"))
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
