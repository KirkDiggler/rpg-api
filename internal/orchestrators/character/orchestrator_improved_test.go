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

// UpdateName Tests - Each test function uses SetupSubTest for clean mocks and test data

func (s *OrchestratorTestSuite) TestUpdateName_SuccessfulUpdate() {
	// Base draft is created fresh in SetupSubTest
	draftData := dnd5e.FromCharacterDraft(s.testDraft)

	// Set up mocks
	mocks.ExpectDraftGet(s.ctx, s.mockDraftRepo, s.testDraftID, draftData, nil)
	s.mockDraftRepo.EXPECT().
		Update(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			s.Equal("Aragorn", input.Draft.Name)
			s.True(input.Draft.Progress.HasName())
			return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
		})

	// Execute
	input := &characterorchestrator.UpdateNameInput{
		DraftID: s.testDraftID,
		Name:    "Aragorn",
	}
	output, err := s.orchestrator.UpdateName(s.ctx, input)

	// Verify
	s.NoError(err)
	s.NotNil(output)
	s.Equal("Aragorn", output.Draft.Name)
}

func (s *OrchestratorTestSuite) TestUpdateName_EmptyName() {
	// No repository mocks needed - validation fails before any repository calls

	// Execute
	input := &characterorchestrator.UpdateNameInput{
		DraftID: s.testDraftID,
		Name:    "",
	}
	output, err := s.orchestrator.UpdateName(s.ctx, input)

	// Verify
	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "name: is required")
}

func (s *OrchestratorTestSuite) TestUpdateName_DraftNotFound() {
	// Set up mocks - return not found error
	mocks.ExpectDraftGet(s.ctx, s.mockDraftRepo, s.testDraftID, nil, errors.NotFound("draft not found"))

	// Execute
	input := &characterorchestrator.UpdateNameInput{
		DraftID: s.testDraftID,
		Name:    "Legolas",
	}
	output, err := s.orchestrator.UpdateName(s.ctx, input)

	// Verify
	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "draft not found")
}

func (s *OrchestratorTestSuite) TestUpdateName_NameAlreadySet() {
	// Modify the base draft to have an existing name
	draftCopy := *s.testDraft
	draftCopy.Name = "OldName"
	draftCopy.Progress.SetStep(dnd5e.ProgressStepName, true)
	draftData := dnd5e.FromCharacterDraft(&draftCopy)

	// Set up mocks
	mocks.ExpectDraftGet(s.ctx, s.mockDraftRepo, s.testDraftID, draftData, nil)
	s.mockDraftRepo.EXPECT().
		Update(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			s.Equal("Gimli", input.Draft.Name)
			s.True(input.Draft.Progress.HasName())
			return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
		})

	// Execute
	input := &characterorchestrator.UpdateNameInput{
		DraftID: s.testDraftID,
		Name:    "Gimli",
	}
	output, err := s.orchestrator.UpdateName(s.ctx, input)

	// Verify
	s.NoError(err)
	s.NotNil(output)
	s.Equal("Gimli", output.Draft.Name)
}

// CreateDraft Tests - Each test function uses SetupSubTest for clean mocks and test data

func (s *OrchestratorTestSuite) TestCreateDraft_Minimal() {
	// Set up engine validation mock
	s.mockEngine.EXPECT().
		ValidateCharacterDraft(s.ctx, gomock.Any()).
		Return(&engine.ValidateCharacterDraftOutput{
			IsValid: true,
			Errors:  nil,
		}, nil)

	// Set up repository mock
	mocks.ExpectDraftCreate(s.ctx, s.mockDraftRepo)

	// Execute
	input := &characterorchestrator.CreateDraftInput{
		PlayerID: s.testPlayerID,
	}
	output, err := s.orchestrator.CreateDraft(s.ctx, input)

	// Verify
	s.NoError(err)
	s.NotNil(output)
	s.Equal(s.testPlayerID, output.Draft.PlayerID)
	s.Empty(output.Draft.Name)
	s.Equal(int32(0), output.Draft.Progress.CompletionPercentage)
}

func (s *OrchestratorTestSuite) TestCreateDraft_WithInitialName() {
	// Set up engine validation mock
	s.mockEngine.EXPECT().
		ValidateCharacterDraft(s.ctx, gomock.Any()).
		Return(&engine.ValidateCharacterDraftOutput{
			IsValid: true,
			Errors:  nil,
		}, nil)

	// Set up repository mock
	mocks.ExpectDraftCreate(s.ctx, s.mockDraftRepo)

	// Execute
	input := &characterorchestrator.CreateDraftInput{
		PlayerID: s.testPlayerID,
		InitialData: builders.NewCharacterDraftBuilder().
			WithName("Bilbo").
			Build(),
	}
	output, err := s.orchestrator.CreateDraft(s.ctx, input)

	// Verify
	s.NoError(err)
	s.NotNil(output)
	s.Equal("Bilbo", output.Draft.Name)
	s.True(output.Draft.Progress.HasName())
	s.Greater(output.Draft.Progress.CompletionPercentage, int32(0))
}

func (s *OrchestratorTestSuite) TestCreateDraft_WithRaceAndClass() {
	// Set up engine validation mock
	s.mockEngine.EXPECT().
		ValidateCharacterDraft(s.ctx, gomock.Any()).
		Return(&engine.ValidateCharacterDraftOutput{
			IsValid: true,
			Errors:  nil,
		}, nil)

	// Set up repository mock
	mocks.ExpectDraftCreate(s.ctx, s.mockDraftRepo)

	// Execute
	input := &characterorchestrator.CreateDraftInput{
		PlayerID:  s.testPlayerID,
		SessionID: s.testSessionID,
		InitialData: builders.NewCharacterDraftBuilder().
			WithName("Thorin").
			WithRace(dnd5e.RaceDwarf, dnd5e.SubraceMountainDwarf).
			WithClass(dnd5e.ClassFighter).
			Build(),
	}
	output, err := s.orchestrator.CreateDraft(s.ctx, input)

	// Verify
	s.NoError(err)
	s.NotNil(output)
	s.Equal("Thorin", output.Draft.Name)
	s.Equal(dnd5e.RaceDwarf, output.Draft.RaceID)
	s.Equal(dnd5e.SubraceMountainDwarf, output.Draft.SubraceID)
	s.Equal(dnd5e.ClassFighter, output.Draft.ClassID)
	s.True(output.Draft.Progress.HasName())
	s.True(output.Draft.Progress.HasRace())
	s.True(output.Draft.Progress.HasClass())
}

func (s *OrchestratorTestSuite) TestCreateDraft_InvalidEngineValidation() {
	// Set up engine validation mock to return validation errors
	s.mockEngine.EXPECT().
		ValidateCharacterDraft(s.ctx, gomock.Any()).
		Return(&engine.ValidateCharacterDraftOutput{
			IsValid: false,
			Errors: []engine.ValidationError{
				{Field: "intelligence", Message: "Wizards require 13+ Intelligence"},
			},
		}, nil)

	// No repository mock expected since validation fails

	// Execute
	input := &characterorchestrator.CreateDraftInput{
		PlayerID: s.testPlayerID,
		InitialData: builders.NewCharacterDraftBuilder().
			WithClass(dnd5e.ClassWizard).
			WithAbilityScores(8, 8, 8, 8, 8, 8). // Too low for wizard
			Build(),
	}
	output, err := s.orchestrator.CreateDraft(s.ctx, input)

	// Verify
	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "Wizards require 13+ Intelligence")
}
