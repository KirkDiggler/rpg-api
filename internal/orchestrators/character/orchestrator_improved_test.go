package character_test

import (
	"context"

	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	characterorchestrator "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-api/internal/testutils/builders"
	"github.com/KirkDiggler/rpg-api/internal/testutils/mocks"
)

// TestUpdateNameImproved demonstrates the lean test pattern with minimal data variations
func (s *OrchestratorTestSuite) TestUpdateNameImproved() {
	// Base draft is created fresh in SetupSubTest
	// We only modify what's needed for each test case

	testCases := []struct {
		name      string
		inputName string                                // Only the variation we care about
		draftMod  func(*dnd5e.CharacterDraft)           // Optional draft modifications
		setupMock func(draft *dnd5e.CharacterDraftData) // Pass expected data to mock setup
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "successful name update",
			inputName: "Aragorn",
			setupMock: func(draftData *dnd5e.CharacterDraftData) {
				// Use helper to set up common expectations
				mocks.ExpectDraftGet(s.ctx, s.mockDraftRepo, s.testDraftID, draftData, nil)

				// Expect update with validation on what changed
				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						// Only verify what should have changed
						s.Equal("Aragorn", input.Draft.Name)
						s.True(input.Draft.Progress.HasName())
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
		},
		{
			name:      "empty name",
			inputName: "",
			setupMock: func(draftData *dnd5e.CharacterDraftData) {
				mocks.ExpectDraftGet(s.ctx, s.mockDraftRepo, s.testDraftID, draftData, nil)
				// No update expected - should fail validation
			},
			wantErr: true,
			errMsg:  "name: is required",
		},
		{
			name:      "draft not found",
			inputName: "Legolas",
			setupMock: func(_ *dnd5e.CharacterDraftData) {
				mocks.ExpectDraftGet(s.ctx, s.mockDraftRepo, s.testDraftID, nil, errors.NotFound("draft not found"))
			},
			wantErr: true,
			errMsg:  "draft not found",
		},
		{
			name:      "name already set - updating",
			inputName: "Gimli",
			draftMod: func(d *dnd5e.CharacterDraft) {
				// Modify the base draft to have an existing name
				d.Name = "OldName"
				d.Progress.SetStep(dnd5e.ProgressStepName, true)
			},
			setupMock: func(draftData *dnd5e.CharacterDraftData) {
				mocks.ExpectDraftGet(s.ctx, s.mockDraftRepo, s.testDraftID, draftData, nil)

				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						s.Equal("Gimli", input.Draft.Name)
						s.True(input.Draft.Progress.HasName())
						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Start with base draft from SetupSubTest
			draft := s.testDraft

			// Apply any test-specific modifications
			if tc.draftMod != nil {
				// Create a copy to avoid modifying the suite's test data
				draftCopy := *draft
				tc.draftMod(&draftCopy)
				draft = &draftCopy
			}

			// Convert to data for repository
			draftData := dnd5e.FromCharacterDraft(draft)

			// Set up mocks with the expected data
			tc.setupMock(draftData)

			// Execute the test
			input := &characterorchestrator.UpdateNameInput{
				DraftID: s.testDraftID,
				Name:    tc.inputName,
			}

			output, err := s.orchestrator.UpdateName(s.ctx, input)

			// Verify results
			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
			} else {
				s.NoError(err)
				s.NotNil(output)
				s.Equal(tc.inputName, output.Draft.Name)
			}
		})
	}
}

// TestCreateDraftImproved shows how to use builders for cleaner test data setup
func (s *OrchestratorTestSuite) TestCreateDraftImproved() {
	testCases := []struct {
		name         string
		buildInput   func() *characterorchestrator.CreateDraftInput
		engineValid  bool
		engineErrors []engine.ValidationError
		wantErr      bool
		errMsg       string
		validate     func(*characterorchestrator.CreateDraftOutput)
	}{
		{
			name: "minimal draft",
			buildInput: func() *characterorchestrator.CreateDraftInput {
				return &characterorchestrator.CreateDraftInput{
					PlayerID: s.testPlayerID,
				}
			},
			engineValid: true,
			validate: func(output *characterorchestrator.CreateDraftOutput) {
				s.Equal(s.testPlayerID, output.Draft.PlayerID)
				s.Empty(output.Draft.Name)
				s.Equal(int32(0), output.Draft.Progress.CompletionPercentage)
			},
		},
		{
			name: "draft with initial name",
			buildInput: func() *characterorchestrator.CreateDraftInput {
				return &characterorchestrator.CreateDraftInput{
					PlayerID: s.testPlayerID,
					InitialData: builders.NewCharacterDraftBuilder().
						WithName("Bilbo").
						Build(),
				}
			},
			engineValid: true,
			validate: func(output *characterorchestrator.CreateDraftOutput) {
				s.Equal("Bilbo", output.Draft.Name)
				s.True(output.Draft.Progress.HasName())
				s.Greater(output.Draft.Progress.CompletionPercentage, int32(0))
			},
		},
		{
			name: "draft with race and class",
			buildInput: func() *characterorchestrator.CreateDraftInput {
				return &characterorchestrator.CreateDraftInput{
					PlayerID:  s.testPlayerID,
					SessionID: s.testSessionID,
					InitialData: builders.NewCharacterDraftBuilder().
						WithName("Thorin").
						WithRace(dnd5e.RaceDwarf, dnd5e.SubraceMountainDwarf).
						WithClass(dnd5e.ClassFighter).
						Build(),
				}
			},
			engineValid: true,
			validate: func(output *characterorchestrator.CreateDraftOutput) {
				s.Equal("Thorin", output.Draft.Name)
				s.Equal(dnd5e.RaceDwarf, output.Draft.RaceID)
				s.Equal(dnd5e.SubraceMountainDwarf, output.Draft.SubraceID)
				s.Equal(dnd5e.ClassFighter, output.Draft.ClassID)
				s.True(output.Draft.Progress.HasName())
				s.True(output.Draft.Progress.HasRace())
				s.True(output.Draft.Progress.HasClass())
			},
		},
		{
			name: "invalid draft - engine validation fails",
			buildInput: func() *characterorchestrator.CreateDraftInput {
				return &characterorchestrator.CreateDraftInput{
					PlayerID: s.testPlayerID,
					InitialData: builders.NewCharacterDraftBuilder().
						WithClass(dnd5e.ClassWizard).
						WithAbilityScores(8, 8, 8, 8, 8, 8). // Too low for wizard
						Build(),
				}
			},
			engineValid: false,
			engineErrors: []engine.ValidationError{
				{Field: "intelligence", Message: "Wizards require 13+ Intelligence"},
			},
			wantErr: true,
			errMsg:  "Wizards require 13+ Intelligence",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			input := tc.buildInput()

			// Set up engine validation mock
			s.mockEngine.EXPECT().
				ValidateCharacterDraft(s.ctx, gomock.Any()).
				Return(&engine.ValidateCharacterDraftOutput{
					IsValid: tc.engineValid,
					Errors:  tc.engineErrors,
				}, nil)

			// Set up repository mock if validation passes
			if tc.engineValid {
				mocks.ExpectDraftCreate(s.ctx, s.mockDraftRepo)
			}

			// Execute
			output, err := s.orchestrator.CreateDraft(s.ctx, input)

			// Verify
			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
			} else {
				s.NoError(err)
				s.NotNil(output)
				tc.validate(output)
			}
		})
	}
}
