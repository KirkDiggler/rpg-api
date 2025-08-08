package encounter_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
)

type OrchestratorTestSuite struct {
	suite.Suite
	orchestrator encounter.Service
	idGen        idgen.Generator
}

func (s *OrchestratorTestSuite) SetupTest() {
	s.idGen = idgen.NewSequential("test")

	cfg := &encounter.Config{
		IDGenerator: s.idGen,
	}

	var err error
	s.orchestrator, err = encounter.NewOrchestrator(cfg)
	s.Require().NoError(err)
}

func (s *OrchestratorTestSuite) TestDungeonStart_WithInitiative() {
	// Arrange
	input := &encounter.DungeonStartInput{
		CharacterIDs: []string{"fighter-123", "wizard-456", "rogue-789"},
	}

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
	s.Equal(0, output.InitiativeData.Current, "Should start at index 0")
}

func (s *OrchestratorTestSuite) TestNextTurn() {
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

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}
