package character_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	externalmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	enginemock "github.com/KirkDiggler/rpg-api/internal/engine/mock"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	characterorchestrator "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	characterrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	"github.com/KirkDiggler/rpg-api/internal/testutils/builders"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type OrchestratorTestSuite struct {
	suite.Suite
	ctrl               *gomock.Controller
	mockCharRepo       *characterrepomock.MockRepository
	mockDraftRepo      *draftrepomock.MockRepository
	mockEngine         *enginemock.MockEngine
	mockExternalClient *externalmock.MockClient
	mockDiceService    *dicemock.MockService
	orchestrator       *characterorchestrator.Orchestrator
	ctx                context.Context

	// Test data
	testDraftID   string
	testPlayerID  string
	testSessionID string
}

func (s *OrchestratorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = characterrepomock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftrepomock.NewMockRepository(s.ctrl)
	s.mockEngine = enginemock.NewMockEngine(s.ctrl)
	s.mockExternalClient = externalmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)

	cfg := &characterorchestrator.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		Engine:             s.mockEngine,
		ExternalClient:     s.mockExternalClient,
		DiceService:        s.mockDiceService,
	}

	orchestrator, err := characterorchestrator.New(cfg)
	s.Require().NoError(err)
	s.orchestrator = orchestrator

	s.ctx = context.Background()

	// Initialize base test IDs that won't change
	s.testDraftID = "draft-123"
	s.testPlayerID = "player-789"
	s.testSessionID = "session-012"
}

func (s *OrchestratorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// Test CreateDraft
func (s *OrchestratorTestSuite) TestCreateDraft_Success() {
	s.mockDraftRepo.EXPECT().
		Create(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input draftrepo.CreateInput) (*draftrepo.CreateOutput, error) {
			// Repository sets ID and timestamps
			draft := *input.Draft
			draft.ID = "generated-id"
			draft.CreatedAt = time.Now()
			draft.UpdatedAt = time.Now()
			return &draftrepo.CreateOutput{Draft: &draft}, nil
		})

	input := &characterorchestrator.CreateDraftInput{
		PlayerID: s.testPlayerID,
	}
	output, err := s.orchestrator.CreateDraft(s.ctx, input)

	s.NoError(err)
	s.NotNil(output)
	s.NotNil(output.Draft)
	s.Equal(s.testPlayerID, output.Draft.PlayerID)
	s.Equal(int32(0), output.Draft.Progress.CompletionPercentage)
}

func (s *OrchestratorTestSuite) TestCreateDraft_WithInitialData() {

	s.mockDraftRepo.EXPECT().
		Create(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input draftrepo.CreateInput) (*draftrepo.CreateOutput, error) {
			// Verify initial data was applied
			s.Equal("Frodo", input.Draft.Name)
			s.Equal(shared.ChoiceCategory("name"), shared.ChoiceName)
			s.NotNil(input.Draft.Choices[shared.ChoiceName])
			
			// Verify race and class were set
			raceChoice := input.Draft.Choices[shared.ChoiceRace].(character.RaceChoice)
			s.Equal(dnd5e.RaceHalfling, raceChoice.RaceID)
			
			classChoice := input.Draft.Choices[shared.ChoiceClass].(string)
			s.Equal(dnd5e.ClassRogue, classChoice)
			
			// Repository sets ID and timestamps
			draft := *input.Draft
			draft.ID = "generated-id"
			draft.CreatedAt = time.Now()
			draft.UpdatedAt = time.Now()
			return &draftrepo.CreateOutput{Draft: &draft}, nil
		})

	// Mock external client calls for hydration
	s.mockExternalClient.EXPECT().
		GetRaceData(s.ctx, dnd5e.RaceHalfling).
		Return(nil, nil).AnyTimes()
		
	s.mockExternalClient.EXPECT().
		GetClassData(s.ctx, dnd5e.ClassRogue).
		Return(nil, nil).AnyTimes()

	input := &characterorchestrator.CreateDraftInput{
		PlayerID:  s.testPlayerID,
		SessionID: s.testSessionID,
		InitialData: &dnd5e.CharacterDraft{
			Name:    "Frodo",
			RaceID:  dnd5e.RaceHalfling,
			ClassID: dnd5e.ClassRogue,
		},
	}
	output, err := s.orchestrator.CreateDraft(s.ctx, input)

	s.NoError(err)
	s.NotNil(output)
	s.Equal("Frodo", output.Draft.Name)
	s.Equal(dnd5e.RaceHalfling, output.Draft.RaceID)
	s.Equal(dnd5e.ClassRogue, output.Draft.ClassID)
	s.Greater(output.Draft.Progress.CompletionPercentage, int32(0))
}

// Test UpdateName
func (s *OrchestratorTestSuite) TestUpdateName_Success() {
	// Create toolkit draft data
	draftData := builders.NewToolkitDraftDataBuilder().
		WithID(s.testDraftID).
		WithPlayerID(s.testPlayerID).
		WithName("OldName").
		Build()

	s.mockDraftRepo.EXPECT().
		Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
		Return(&draftrepo.GetOutput{Draft: draftData}, nil)

	s.mockDraftRepo.EXPECT().
		Update(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			s.Equal("NewName", input.Draft.Name)
			s.Equal("NewName", input.Draft.Choices[shared.ChoiceName])
			return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
		})

	input := &characterorchestrator.UpdateNameInput{
		DraftID: s.testDraftID,
		Name:    "NewName",
	}
	output, err := s.orchestrator.UpdateName(s.ctx, input)

	s.NoError(err)
	s.NotNil(output)
	s.Equal("NewName", output.Draft.Name)
}

// Test UpdateRace with choices
func (s *OrchestratorTestSuite) TestUpdateRace_WithChoices() {
	draftData := builders.NewToolkitDraftDataBuilder().
		WithID(s.testDraftID).
		WithPlayerID(s.testPlayerID).
		Build()

	s.mockDraftRepo.EXPECT().
		Get(s.ctx, gomock.Any()).
		Return(&draftrepo.GetOutput{Draft: draftData}, nil)

	// Mock external validation - UpdateRace fetches race data multiple times
	s.mockExternalClient.EXPECT().
		GetRaceData(s.ctx, "dwarf").
		Return(&external.RaceData{
			ID:   "dwarf",
			Name: "Dwarf",
		}, nil).AnyTimes()

	s.mockDraftRepo.EXPECT().
		Update(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			// Verify race was set
			raceChoice := input.Draft.Choices[shared.ChoiceRace].(character.RaceChoice)
			s.Equal("dwarf", raceChoice.RaceID)
			
			// Verify race choices were stored
			toolChoice := input.Draft.Choices[shared.ChoiceCategory("race_dwarf_tool_1")]
			s.Equal([]string{"brewers-supplies"}, toolChoice)
			
			return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
		})

	input := &characterorchestrator.UpdateRaceInput{
		DraftID: s.testDraftID,
		RaceID:  "dwarf",
		Choices: []dnd5e.ChoiceSelection{
			{
				ChoiceID:     "dwarf_tool_1",
				SelectedKeys: []string{"brewers-supplies"},
			},
		},
	}
	output, err := s.orchestrator.UpdateRace(s.ctx, input)

	s.NoError(err)
	s.NotNil(output)
	s.Equal("dwarf", output.Draft.RaceID)
	// Verify choices are returned
	found := false
	for _, choice := range output.Draft.ChoiceSelections {
		if choice.ChoiceID == "dwarf_tool_1" && choice.Source == dnd5e.ChoiceSourceRace {
			found = true
			s.Equal([]string{"brewers-supplies"}, choice.SelectedKeys)
		}
	}
	s.True(found, "Race choice should be returned in ChoiceSelections")
}

// Test UpdateAbilityScores
func (s *OrchestratorTestSuite) TestUpdateAbilityScores_Success() {
	draftData := builders.NewToolkitDraftDataBuilder().
		WithID(s.testDraftID).
		WithPlayerID(s.testPlayerID).
		Build()

	s.mockDraftRepo.EXPECT().
		Get(s.ctx, gomock.Any()).
		Return(&draftrepo.GetOutput{Draft: draftData}, nil)

	s.mockDraftRepo.EXPECT().
		Update(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			// Verify ability scores were set
			scores := input.Draft.Choices[shared.ChoiceAbilityScores].(shared.AbilityScores)
			s.Equal(15, scores.Strength)
			s.Equal(14, scores.Dexterity)
			s.Equal(13, scores.Constitution)
			s.Equal(12, scores.Intelligence)
			s.Equal(10, scores.Wisdom)
			s.Equal(8, scores.Charisma)
			
			return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
		})

	input := &characterorchestrator.UpdateAbilityScoresInput{
		DraftID: s.testDraftID,
		AbilityScores: dnd5e.AbilityScores{
			Strength:     15,
			Dexterity:    14,
			Constitution: 13,
			Intelligence: 12,
			Wisdom:       10,
			Charisma:     8,
		},
	}
	output, err := s.orchestrator.UpdateAbilityScores(s.ctx, input)

	s.NoError(err)
	s.NotNil(output)
	s.NotNil(output.Draft.AbilityScores)
	s.Equal(int32(15), output.Draft.AbilityScores.Strength)
	s.Equal(int32(14), output.Draft.AbilityScores.Dexterity)
	s.Equal(int32(13), output.Draft.AbilityScores.Constitution)
	s.Equal(int32(12), output.Draft.AbilityScores.Intelligence)
	s.Equal(int32(10), output.Draft.AbilityScores.Wisdom)
	s.Equal(int32(8), output.Draft.AbilityScores.Charisma)
}

// Test GetDraft
func (s *OrchestratorTestSuite) TestGetDraft_Success() {
	draftData := builders.NewToolkitDraftDataBuilder().
		WithID(s.testDraftID).
		WithPlayerID(s.testPlayerID).
		WithName("TestCharacter").
		WithRace("human", "").
		WithClass("fighter").
		WithAbilityScores(shared.AbilityScores{
			Strength:     16,
			Dexterity:    14,
			Constitution: 15,
			Intelligence: 10,
			Wisdom:       12,
			Charisma:     8,
		}).
		Build()

	s.mockDraftRepo.EXPECT().
		Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
		Return(&draftrepo.GetOutput{Draft: draftData}, nil)

	// Mock external client calls for hydration
	s.mockExternalClient.EXPECT().
		GetRaceData(s.ctx, "human").
		Return(nil, nil).AnyTimes()
		
	s.mockExternalClient.EXPECT().
		GetClassData(s.ctx, "fighter").
		Return(nil, nil).AnyTimes()

	input := &characterorchestrator.GetDraftInput{
		DraftID: s.testDraftID,
	}
	output, err := s.orchestrator.GetDraft(s.ctx, input)

	s.NoError(err)
	s.NotNil(output)
	s.NotNil(output.Draft)
	s.Equal(s.testDraftID, output.Draft.ID)
	s.Equal("TestCharacter", output.Draft.Name)
	s.Equal("human", output.Draft.RaceID)
	s.Equal("fighter", output.Draft.ClassID)
	s.NotNil(output.Draft.AbilityScores)
	s.Equal(int32(16), output.Draft.AbilityScores.Strength)
}

// Test validation errors
func (s *OrchestratorTestSuite) TestCreateDraft_ValidationError() {
	input := &characterorchestrator.CreateDraftInput{
		PlayerID: "", // Missing required field
	}
	output, err := s.orchestrator.CreateDraft(s.ctx, input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "playerID: is required")
}

func (s *OrchestratorTestSuite) TestUpdateName_EmptyName() {
	input := &characterorchestrator.UpdateNameInput{
		DraftID: s.testDraftID,
		Name:    "", // Empty name
	}
	output, err := s.orchestrator.UpdateName(s.ctx, input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "name: is required")
}

// Test repository errors
func (s *OrchestratorTestSuite) TestGetDraft_NotFound() {
	s.mockDraftRepo.EXPECT().
		Get(s.ctx, gomock.Any()).
		Return(nil, errors.NotFound("draft not found"))

	input := &characterorchestrator.GetDraftInput{
		DraftID: "nonexistent",
	}
	output, err := s.orchestrator.GetDraft(s.ctx, input)

	s.Error(err)
	s.Nil(output)
	s.True(errors.IsNotFound(err))
}

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}