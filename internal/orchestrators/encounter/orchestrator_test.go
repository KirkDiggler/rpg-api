package encounter_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
	chardata "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type OrchestratorTestSuite struct {
	suite.Suite
	orchestrator    encounter.Service
	idGen           idgen.Generator
	mockCharService *charactermock.MockService
	mockCtrl        *gomock.Controller
}

func (s *OrchestratorTestSuite) SetupTest() {
	s.idGen = idgen.NewSequential("test")
	s.mockCtrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.mockCtrl)

	cfg := &encounter.Config{
		IDGenerator:      s.idGen,
		Repository:       encounters.NewInMemory(),
		CharacterService: s.mockCharService,
	}

	var err error
	s.orchestrator, err = encounter.NewOrchestrator(cfg)
	s.Require().NoError(err)
}

func (s *OrchestratorTestSuite) TearDownTest() {
	s.mockCtrl.Finish()
}

func (s *OrchestratorTestSuite) TestDungeonStart_WithInitiative() {
	// Arrange
	input := &encounter.DungeonStartInput{
		CharacterIDs: []string{"fighter-123", "wizard-456", "rogue-789"},
	}

	// Mock character service to return characters with different DEX scores
	fighterChar := &chardata.Data{
		ID:            "fighter-123",
		Name:          "Fighter",
		AbilityScores: shared.AbilityScores{constants.DEX: 14}, // +2 modifier
	}
	wizardChar := &chardata.Data{
		ID:            "wizard-456",
		Name:          "Wizard",
		AbilityScores: shared.AbilityScores{constants.DEX: 10}, // +0 modifier
	}
	rogueChar := &chardata.Data{
		ID:            "rogue-789",
		Name:          "Rogue",
		AbilityScores: shared.AbilityScores{constants.DEX: 18}, // +4 modifier
	}

	s.mockCharService.EXPECT().GetCharacter(gomock.Any(), &character.GetCharacterInput{
		CharacterID: "fighter-123",
	}).Return(&character.GetCharacterOutput{Character: fighterChar}, nil)

	s.mockCharService.EXPECT().GetCharacter(gomock.Any(), &character.GetCharacterInput{
		CharacterID: "wizard-456",
	}).Return(&character.GetCharacterOutput{Character: wizardChar}, nil)

	s.mockCharService.EXPECT().GetCharacter(gomock.Any(), &character.GetCharacterInput{
		CharacterID: "rogue-789",
	}).Return(&character.GetCharacterOutput{Character: rogueChar}, nil)

	// Act
	output, err := s.orchestrator.DungeonStart(context.Background(), input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)

	// Verify encounter basics
	s.NotEmpty(output.EncounterID)
	s.NotNil(output.RoomData)

	// Verify initiative was set up
	s.NotNil(output.InitiativeData)
	s.NotEmpty(output.CurrentTurn, "Should have someone's turn to start")

	// Initiative order should have all characters + monster (4 total)
	s.Len(output.InitiativeData.Order, 4, "Should have 3 characters + 1 monster")

	// Current turn should be one of the entities
	validTurns := make(map[string]bool)
	for _, entity := range output.InitiativeData.Order {
		validTurns[entity.ID] = true
	}
	s.True(validTurns[output.CurrentTurn], "Current turn should be in initiative order")

	// Should start at round 1
	s.Equal(1, output.InitiativeData.Round)

	// Current index should match the current turn entity
	// (might not be 0 if monster went first and was auto-skipped)
	currentEntity := ""
	for i, entity := range output.InitiativeData.Order {
		if entity.ID == output.CurrentTurn {
			s.Equal(i, output.InitiativeData.Current, "Current index should match current turn entity")
			currentEntity = entity.Type
			break
		}
	}

	// Current turn should always be a character (not a monster)
	s.Equal("character", currentEntity, "Current turn should be a character after auto-advancing")
}

func (s *OrchestratorTestSuite) TestNextTurn() {
	// Setup character mocks
	fighterChar := &chardata.Data{
		ID:            "fighter-123",
		Name:          "Fighter",
		AbilityScores: shared.AbilityScores{constants.DEX: 14},
	}
	wizardChar := &chardata.Data{
		ID:            "wizard-456",
		Name:          "Wizard",
		AbilityScores: shared.AbilityScores{constants.DEX: 10},
	}

	s.mockCharService.EXPECT().GetCharacter(gomock.Any(), &character.GetCharacterInput{
		CharacterID: "fighter-123",
	}).Return(&character.GetCharacterOutput{Character: fighterChar}, nil)

	s.mockCharService.EXPECT().GetCharacter(gomock.Any(), &character.GetCharacterInput{
		CharacterID: "wizard-456",
	}).Return(&character.GetCharacterOutput{Character: wizardChar}, nil)

	// First create an encounter
	startInput := &encounter.DungeonStartInput{
		CharacterIDs: []string{"fighter-123", "wizard-456"},
	}

	startOutput, err := s.orchestrator.DungeonStart(context.Background(), startInput)
	s.Require().NoError(err)

	firstTurn := startOutput.CurrentTurn

	// Advance to next turn
	nextInput := &encounter.NextTurnInput{
		EncounterID: startOutput.EncounterID,
	}

	nextOutput, err := s.orchestrator.NextTurn(context.Background(), nextInput)
	s.Require().NoError(err)
	s.Require().NotNil(nextOutput)

	// Should have advanced to next entity
	s.NotEmpty(nextOutput.CurrentTurn)
	s.NotEqual(firstTurn, nextOutput.CurrentTurn, "Should have moved to next entity")
	s.Equal(1, nextOutput.Round, "Should still be round 1")

	// Advance through all turns to trigger round 2
	for i := 0; i < 2; i++ { // 2 more advances to complete the round (3 total entities)
		nextOutput, err = s.orchestrator.NextTurn(context.Background(), nextInput)
		s.Require().NoError(err)
	}

	// After going through all entities, should be round 2
	s.Equal(2, nextOutput.Round, "Should advance to round 2 after all entities had a turn")
}

func (s *OrchestratorTestSuite) TestGetTurnOrder() {
	// Setup character mock
	fighterChar := &chardata.Data{
		ID:            "fighter-123",
		Name:          "Fighter",
		AbilityScores: shared.AbilityScores{constants.DEX: 14},
	}

	s.mockCharService.EXPECT().GetCharacter(gomock.Any(), &character.GetCharacterInput{
		CharacterID: "fighter-123",
	}).Return(&character.GetCharacterOutput{Character: fighterChar}, nil)

	// Create an encounter
	startInput := &encounter.DungeonStartInput{
		CharacterIDs: []string{"fighter-123"},
	}

	startOutput, err := s.orchestrator.DungeonStart(context.Background(), startInput)
	s.Require().NoError(err)

	// Get current turn order
	getInput := &encounter.GetTurnOrderInput{
		EncounterID: startOutput.EncounterID,
	}

	getOutput, err := s.orchestrator.GetTurnOrder(context.Background(), getInput)
	s.Require().NoError(err)
	s.Require().NotNil(getOutput)

	// Should match initial state
	s.Equal(startOutput.CurrentTurn, getOutput.CurrentTurn)
	s.Equal(startOutput.InitiativeData.Order, getOutput.InitiativeData.Order)
	s.Equal(1, getOutput.InitiativeData.Round)
}

func (s *OrchestratorTestSuite) TestNextTurn_EncounterNotFound() {
	input := &encounter.NextTurnInput{
		EncounterID: "non-existent",
	}

	output, err := s.orchestrator.NextTurn(context.Background(), input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "not found")
}

func (s *OrchestratorTestSuite) TestGetTurnOrder_EncounterNotFound() {
	input := &encounter.GetTurnOrderInput{
		EncounterID: "non-existent",
	}

	output, err := s.orchestrator.GetTurnOrder(context.Background(), input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "not found")
}

func (s *OrchestratorTestSuite) TestDungeonStart_CharacterServiceError() {
	// Setup - character service returns error for one character
	fighterChar := &chardata.Data{
		ID:            "fighter-123",
		Name:          "Fighter",
		AbilityScores: shared.AbilityScores{constants.DEX: 14},
	}

	s.mockCharService.EXPECT().GetCharacter(gomock.Any(), &character.GetCharacterInput{
		CharacterID: "fighter-123",
	}).Return(&character.GetCharacterOutput{Character: fighterChar}, nil)

	// This character fails to load
	s.mockCharService.EXPECT().GetCharacter(gomock.Any(), &character.GetCharacterInput{
		CharacterID: "wizard-456",
	}).Return(nil, context.DeadlineExceeded)

	// Arrange
	input := &encounter.DungeonStartInput{
		CharacterIDs: []string{"fighter-123", "wizard-456"},
	}

	// Act
	output, err := s.orchestrator.DungeonStart(context.Background(), input)

	// Assert - should still succeed but use default DEX modifier for wizard
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Len(output.InitiativeData.Order, 3, "Should have 2 characters + 1 monster even if one character failed to load")
}

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}
