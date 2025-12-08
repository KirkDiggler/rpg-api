package encounter

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/damage"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
	encountermock "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/mock"
)

type OrchestratorTestSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	mockEncRepo  *encountermock.MockRepository
	orchestrator *Orchestrator
}

func (s *OrchestratorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.mockEncRepo = encountermock.NewMockRepository(s.ctrl)

	var err error
	s.orchestrator, err = New(&Config{
		CharacterRepo: s.mockCharRepo,
		EncounterRepo: s.mockEncRepo,
	})
	s.Require().NoError(err)
}

func (s *OrchestratorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}

func (s *OrchestratorTestSuite) TestNew_Success() {
	orch, err := New(&Config{
		CharacterRepo: s.mockCharRepo,
		EncounterRepo: s.mockEncRepo,
	})
	s.Require().NoError(err)
	s.Assert().NotNil(orch)
}

func (s *OrchestratorTestSuite) TestNew_NilConfig() {
	orch, err := New(nil)
	s.Require().Error(err)
	s.Assert().Nil(orch)
	s.Assert().Contains(err.Error(), "config is required")
}

func (s *OrchestratorTestSuite) TestNew_MissingCharacterRepo() {
	orch, err := New(&Config{
		EncounterRepo: s.mockEncRepo,
	})
	s.Require().Error(err)
	s.Assert().Nil(orch)
	s.Assert().Contains(err.Error(), "CharacterRepo")
}

func (s *OrchestratorTestSuite) TestNew_MissingEncounterRepo() {
	orch, err := New(&Config{
		CharacterRepo: s.mockCharRepo,
	})
	s.Require().Error(err)
	s.Assert().Nil(orch)
	s.Assert().Contains(err.Error(), "EncounterRepo")
}

func (s *OrchestratorTestSuite) TestResolveAttack_Success() {
	// Arrange - Create test character data
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{CharacterData: charData}, nil)

	// Mock equipment slots - use greataxe (default fallback behavior)
	s.mockCharRepo.EXPECT().
		GetEquipmentSlots(gomock.Any(), characterrepo.GetEquipmentSlotsInput{
			CharacterID: "char-1",
		}).
		Return(&characterrepo.GetEquipmentSlotsOutput{
			EquipmentSlots: &characterrepo.EquipmentSlots{
				MainHand: "greataxe",
			},
		}, nil)

	// Arrange - Mock encounter repo
	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)

	// Attack result should have valid values
	s.Assert().GreaterOrEqual(output.Result.AttackRoll, 1)
	s.Assert().LessOrEqual(output.Result.AttackRoll, 20)

	// Attack could hit or miss - both are valid
	if output.Result.Hit {
		s.Assert().Greater(output.Result.TotalDamage, 0)
		s.Assert().NotEmpty(output.Result.DamageRolls)
		s.Assert().Equal(damage.Slashing, output.Result.DamageType)

		// Verify breakdown is populated on hit
		s.Require().NotNil(output.Result.Breakdown, "Breakdown should be present on hit")
		s.Assert().NotEmpty(output.Result.Breakdown.Components, "Breakdown should have components")
		s.Assert().NotEmpty(output.Result.Breakdown.AbilityUsed, "Breakdown should have ability used")
		s.Assert().Greater(output.Result.Breakdown.TotalDamage, 0, "Breakdown total damage should be > 0")

		// Verify components have expected fields
		for i, comp := range output.Result.Breakdown.Components {
			s.Assert().NotEmpty(comp.Source, "Component %d should have source", i)
			s.Assert().NotEmpty(comp.DamageType, "Component %d should have damage type", i)
			// Either dice rolls or flat bonus should be present
			s.Assert().True(
				len(comp.FinalDiceRolls) > 0 || comp.FlatBonus != 0,
				"Component %d should have dice or flat bonus", i,
			)
		}
	} else {
		s.Assert().Equal(0, output.Result.TotalDamage)
		// Breakdown should be nil on miss
		s.Assert().Nil(output.Result.Breakdown, "Breakdown should be nil on miss")
	}

	// Monster HP should be calculated
	s.Assert().GreaterOrEqual(output.MonsterHP, 0)
	s.Assert().LessOrEqual(output.MonsterHP, 7) // Goblin max HP
}

func (s *OrchestratorTestSuite) TestResolveAttack_NilInput() {
	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *OrchestratorTestSuite) TestResolveAttack_MissingEncounterID() {
	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		AttackerID: "char-1",
		TargetID:   "goblin-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *OrchestratorTestSuite) TestResolveAttack_MissingAttackerID() {
	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "attacker ID is required")
}

func (s *OrchestratorTestSuite) TestResolveAttack_MissingTargetID() {
	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "target ID is required")
}

func (s *OrchestratorTestSuite) TestResolveAttack_CharacterNotFound() {
	// Arrange
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "nonexistent"}).
		Return(nil, errors.NotFound("character not found"))

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "nonexistent",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to load character")
}

func (s *OrchestratorTestSuite) TestResolveAttack_EncounterNotFound() {
	// Arrange - Character exists
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{CharacterData: charData}, nil)

	// Arrange - Encounter not found
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "nonexistent"}).
		Return(nil, errors.NotFound("encounter not found"))

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "nonexistent",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to load encounter")
}

func (s *OrchestratorTestSuite) TestResolveAttack_MultipleAttacks() {
	// This test verifies that multiple attacks produce different results
	// (due to different dice rolls) and that the EventBus is created fresh each time

	charData := createTestCharacterData("char-1", "Grog")
	encData := createTestEncounterData("enc-1")

	// Set up expectations for multiple calls
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{CharacterData: charData}, nil).
		Times(3)

	// Mock equipment slots for each attack
	s.mockCharRepo.EXPECT().
		GetEquipmentSlots(gomock.Any(), characterrepo.GetEquipmentSlotsInput{
			CharacterID: "char-1",
		}).
		Return(&characterrepo.GetEquipmentSlotsOutput{
			EquipmentSlots: &characterrepo.EquipmentSlots{
				MainHand: "greataxe",
			},
		}, nil).
		Times(3)

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil).
		Times(3)

	// Perform multiple attacks
	results := make([]*ResolveAttackOutput, 3)
	for i := 0; i < 3; i++ {
		output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
			EncounterID: "enc-1",
			AttackerID:  "char-1",
			TargetID:    "goblin-1",
		})
		s.Require().NoError(err)
		s.Require().NotNil(output)
		results[i] = output
	}

	// Verify that results are independent (not all identical)
	// With dice rolls, it's extremely unlikely all three attacks have identical results
	allIdentical := true
	for i := 1; i < len(results); i++ {
		if results[i].Result.AttackRoll != results[0].Result.AttackRoll ||
			results[i].Result.TotalDamage != results[0].Result.TotalDamage {
			allIdentical = false
			break
		}
	}
	s.Assert().False(allIdentical, "Multiple attacks should produce different results due to dice rolls")
}

// Helper functions for test data

func createTestCharacterData(id, name string) *character.Data {
	// Create basic barbarian character with standard ability scores
	return &character.Data{
		ID:               id,
		Name:             name,
		Level:            1,
		RaceID:           "human",
		ClassID:          "barbarian",
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, // +3 modifier
			abilities.DEX: 14, // +2 modifier
			abilities.CON: 14, // +2 modifier
			abilities.INT: 10, // +0 modifier
			abilities.WIS: 12, // +1 modifier
			abilities.CHA: 8,  // -1 modifier
		},
		Features: []json.RawMessage{}, // No features for basic test
	}
}

func createTestEncounterData(id string) *encounterrepo.EncounterData {
	return &encounterrepo.EncounterData{
		ID: id,
	}
}

// CreateDungeon Tests

func (s *OrchestratorTestSuite) TestCreateDungeon_Success() {
	// Arrange - Mock character repo to return character with DEX score
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{
			CharacterData: &character.Data{
				ID: "char-1",
				AbilityScores: shared.AbilityScores{
					abilities.DEX: 14, // +2 modifier
				},
			},
		}, nil)

	// Arrange - Mock encounter repo save
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.SaveInput) (*encounterrepo.SaveOutput, error) {
			// Verify encounter ID is present and follows expected format
			s.Assert().NotEmpty(input.EncounterID)
			s.Assert().Contains(input.EncounterID, "enc-")

			// Verify room data is present
			s.Assert().NotNil(input.RoomData)
			roomData, ok := input.RoomData.(*spatial.RoomData)
			s.Assert().True(ok, "RoomData should be *spatial.RoomData")
			s.Assert().Equal(input.EncounterID+"-room", roomData.ID)
			s.Assert().Equal("dungeon", roomData.Type)
			s.Assert().Equal(20, roomData.Width)
			s.Assert().Equal(20, roomData.Height)
			s.Assert().Equal(spatial.GridTypeHex, roomData.GridType)
			s.Assert().NotNil(roomData.Entities)

			// Verify initiative data is now saved
			s.Assert().NotNil(input.InitiativeData, "InitiativeData should be present")
			s.Assert().NotNil(input.InitiativeRolls, "InitiativeRolls should be present")

			return &encounterrepo.SaveOutput{Success: true}, nil
		})

	// Act
	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		CharacterIDs: []string{"char-1"},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotEmpty(output.EncounterID)
	s.Assert().Contains(output.EncounterID, "enc-")

	// Verify room data in output
	s.Assert().NotNil(output.Room)
	roomData, ok := output.Room.(*spatial.RoomData)
	s.Assert().True(ok, "Room should be *spatial.RoomData")
	s.Assert().Equal(output.EncounterID+"-room", roomData.ID)
	s.Assert().Equal("dungeon", roomData.Type)
	s.Assert().Equal(20, roomData.Width)
	s.Assert().Equal(20, roomData.Height)
	s.Assert().Equal(spatial.GridTypeHex, roomData.GridType)

	// Verify combat state in output
	s.Assert().NotNil(output.CombatState, "CombatState should be present")
	s.Assert().Equal(output.EncounterID, output.CombatState.EncounterID)
	s.Assert().True(output.CombatState.CombatStarted)
	s.Assert().False(output.CombatState.CombatEnded)
	s.Assert().Len(output.CombatState.TurnOrder, 2) // 1 character + 1 goblin

	// Verify active turn is a player character (monsters are auto-skipped)
	s.Require().GreaterOrEqual(output.CombatState.ActiveIndex, 0)
	s.Require().Less(output.CombatState.ActiveIndex, len(output.CombatState.TurnOrder))
	activeTurn := output.CombatState.TurnOrder[output.CombatState.ActiveIndex]
	s.Assert().Equal("character", activeTurn.EntityType, "Active turn should be a character, not a monster")
}

func (s *OrchestratorTestSuite) TestCreateDungeon_NilInput() {
	// Act
	output, err := s.orchestrator.CreateDungeon(context.Background(), nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *OrchestratorTestSuite) TestCreateDungeon_SaveError() {
	// Arrange - Mock character repo
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{
			CharacterData: &character.Data{
				ID: "char-1",
				AbilityScores: shared.AbilityScores{
					abilities.DEX: 14,
				},
			},
		}, nil)

	// Arrange - Mock repo save to return error
	expectedError := fmt.Errorf("database error")
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(nil, expectedError)

	// Act
	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		CharacterIDs: []string{"char-1"},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to save encounter")
	s.Assert().ErrorIs(err, expectedError)
}

func (s *OrchestratorTestSuite) TestCreateDungeon_MinimalInput() {
	// Test with empty CharacterIDs (dungeon with just goblin)
	// Since there are no player characters, combat should end automatically
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.SaveOutput{Success: true}, nil)

	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		CharacterIDs: []string{},
	})

	s.Require().NoError(err)
	s.Assert().NotEmpty(output.EncounterID)
	s.Assert().NotNil(output.CombatState)
	s.Assert().Len(output.CombatState.TurnOrder, 1) // Only goblin
	// Combat should end immediately since no player characters exist
	s.Assert().True(output.CombatState.CombatEnded, "Combat should end when no player characters exist")
}

func (s *OrchestratorTestSuite) TestCreateDungeon_UniqueIDs() {
	// Verify that multiple calls generate unique IDs
	var capturedIDs []string

	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.SaveInput) (*encounterrepo.SaveOutput, error) {
			capturedIDs = append(capturedIDs, input.EncounterID)
			return &encounterrepo.SaveOutput{Success: true}, nil
		}).
		Times(3)

	// Create three encounters
	for i := 0; i < 3; i++ {
		output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
			CharacterIDs: []string{}, // Empty - just goblin
		})
		s.Require().NoError(err)
		s.Assert().NotEmpty(output.EncounterID)

		// Small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
	}

	// Assert all IDs are unique
	s.Assert().Len(capturedIDs, 3)
	s.Assert().NotEqual(capturedIDs[0], capturedIDs[1])
	s.Assert().NotEqual(capturedIDs[1], capturedIDs[2])
	s.Assert().NotEqual(capturedIDs[0], capturedIDs[2])
}

func (s *OrchestratorTestSuite) TestCreateDungeon_SavesInitiativeData() {
	// Verify that encounter is created with room data AND initiative data
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.SaveInput) (*encounterrepo.SaveOutput, error) {
			// Verify encounter ID and room data are present
			s.Assert().NotEmpty(input.EncounterID)
			s.Assert().NotNil(input.RoomData, "RoomData should be present")

			// Verify initiative data is now saved
			s.Assert().NotNil(input.InitiativeData, "InitiativeData should be present")
			s.Assert().NotNil(input.InitiativeRolls, "InitiativeRolls should be present")
			s.Assert().Len(input.InitiativeRolls, 1, "Should have 1 entity (goblin)")
			return &encounterrepo.SaveOutput{Success: true}, nil
		})

	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		CharacterIDs: []string{}, // Empty - just goblin
	})

	s.Require().NoError(err)
	s.Assert().NotEmpty(output.EncounterID)
	s.Assert().NotNil(output.Room, "Room should be present in output")
	s.Assert().NotNil(output.CombatState, "CombatState should be present in output")
}

// TestMoveCharacter_Success tests successful movement
func (s *OrchestratorTestSuite) TestMoveCharacter_Success() {
	// Arrange
	roomData := &spatial.RoomData{
		ID:       "enc-1-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeSquare,
		Entities: map[string]spatial.EntityPlacement{
			"char-1": {
				EntityID:       "char-1",
				EntityType:     "character",
				Position:       spatial.Position{X: 0, Y: 0},
				Size:           1,
				BlocksMovement: true,
			},
		},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				RoomData:          roomData,
				MovementRemaining: 60, // Enough movement for the test (10 hexes = 50 feet)
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			// Verify the room was updated with new position
			updatedRoom, ok := input.RoomData.(*spatial.RoomData)
			s.Require().True(ok)
			s.Assert().Equal(float64(5), updatedRoom.Entities["char-1"].Position.X)
			s.Assert().Equal(float64(5), updatedRoom.Entities["char-1"].Position.Y)

			// Verify movement was decremented
			s.Require().NotNil(input.MovementRemaining, "MovementRemaining should be updated")
			// Moving from (0,0) to (5,5) = 10 hexes = 50 feet, so 60 - 50 = 10
			s.Assert().Equal(int32(10), *input.MovementRemaining, "Movement should be decremented")

			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		TargetPosition: &Position{
			X: 5,
			Y: 5,
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().Equal(float64(5), output.FinalPosition.X)
	s.Assert().Equal(float64(5), output.FinalPosition.Y)
	s.Assert().Equal("completed", output.StopReason)
	s.Assert().Equal(int32(10), output.MovementRemaining) // 60 - 50 = 10
}

// TestMoveCharacter_OutOfBounds tests movement to invalid position
func (s *OrchestratorTestSuite) TestMoveCharacter_OutOfBounds() {
	// Arrange
	roomData := &spatial.RoomData{
		ID:       "enc-1-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeSquare,
		Entities: make(map[string]spatial.EntityPlacement),
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:       "enc-1",
				RoomData: roomData,
			},
		}, nil)

	// Note: No Update call expected for out of bounds

	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		TargetPosition: &Position{
			X: 100,
			Y: 100,
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().False(output.Success)
	s.Assert().Equal(float64(100), output.FinalPosition.X)
	s.Assert().Equal(float64(100), output.FinalPosition.Y)
	s.Assert().Equal("out_of_bounds", output.StopReason)
	s.Assert().Equal(int32(0), output.MovementRemaining)
}

// TestMoveCharacter_PositionOccupied tests movement to occupied position
func (s *OrchestratorTestSuite) TestMoveCharacter_PositionOccupied() {
	// Arrange
	roomData := &spatial.RoomData{
		ID:       "enc-1-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeSquare,
		Entities: map[string]spatial.EntityPlacement{
			"char-1": {
				EntityID:       "char-1",
				EntityType:     "character",
				Position:       spatial.Position{X: 0, Y: 0},
				Size:           1,
				BlocksMovement: true,
			},
			"goblin-1": {
				EntityID:       "goblin-1",
				EntityType:     "monster",
				Position:       spatial.Position{X: 5, Y: 5},
				Size:           1,
				BlocksMovement: true,
			},
		},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:       "enc-1",
				RoomData: roomData,
			},
		}, nil)

	// Note: No Update call expected for blocked position

	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		TargetPosition: &Position{
			X: 5,
			Y: 5,
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().False(output.Success)
	s.Assert().Equal(float64(0), output.FinalPosition.X) // Returns current position
	s.Assert().Equal(float64(0), output.FinalPosition.Y)
	s.Assert().Equal("position_occupied", output.StopReason)
	s.Assert().Equal(int32(0), output.MovementRemaining)
}

// TestMoveCharacter_CreatesRoomIfMissing tests creating default room when none exists
func (s *OrchestratorTestSuite) TestMoveCharacter_CreatesRoomIfMissing() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:       "enc-1",
				RoomData: nil, // No room data
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			// Verify a room was created
			updatedRoom, ok := input.RoomData.(*spatial.RoomData)
			s.Require().True(ok)
			s.Assert().Equal("enc-1-room", updatedRoom.ID)
			s.Assert().Equal(20, updatedRoom.Width)
			s.Assert().Equal(20, updatedRoom.Height)
			s.Assert().Contains(updatedRoom.Entities, "char-1")
			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		TargetPosition: &Position{
			X: 5,
			Y: 5,
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().Equal("completed", output.StopReason)
}

// TestMoveCharacter_NilInput tests nil input handling
func (s *OrchestratorTestSuite) TestMoveCharacter_NilInput() {
	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

// TestMoveCharacter_MissingEncounterID tests missing encounter ID
func (s *OrchestratorTestSuite) TestMoveCharacter_MissingEncounterID() {
	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EntityID:       "char-1",
		TargetPosition: &Position{X: 5, Y: 5},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

// TestMoveCharacter_MissingEntityID tests missing entity ID
func (s *OrchestratorTestSuite) TestMoveCharacter_MissingEntityID() {
	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID:    "enc-1",
		TargetPosition: &Position{X: 5, Y: 5},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "entity ID is required")
}

// TestMoveCharacter_MissingTargetPosition tests missing target position
func (s *OrchestratorTestSuite) TestMoveCharacter_MissingTargetPosition() {
	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "target position is required")
}

// TestMoveCharacter_EncounterNotFound tests encounter not found error
func (s *OrchestratorTestSuite) TestMoveCharacter_EncounterNotFound() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: nil,
		}, nil)

	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID:    "enc-1",
		EntityID:       "char-1",
		TargetPosition: &Position{X: 5, Y: 5},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter not found")
}

// TestMoveCharacter_UpdateError tests repository update error
func (s *OrchestratorTestSuite) TestMoveCharacter_UpdateError() {
	// Arrange
	roomData := &spatial.RoomData{
		ID:       "enc-1-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeSquare,
		Entities: make(map[string]spatial.EntityPlacement),
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:       "enc-1",
				RoomData: roomData,
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID:    "enc-1",
		EntityID:       "char-1",
		TargetPosition: &Position{X: 5, Y: 5},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to save updated room")
}

// ============================================================================
// EndTurn Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestEndTurn_Success() {
	// Arrange - Set up encounter with 2 characters and 1 monster
	// Turn order: char-1 (current), goblin-1, char-2
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "goblin-1", Type: "monster"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0, // char-1's turn
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("goblin-1", "monster"), Roll: 15, Modifier: 2, Total: 17},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			// Verify initiative was advanced past monster to char-2
			s.Assert().Equal("enc-1", input.EncounterID)
			s.Require().NotNil(input.InitiativeData)
			s.Assert().Equal(2, input.InitiativeData.Current, "Should skip monster and advance to char-2")
			s.Assert().Equal(1, input.InitiativeData.Round, "Should still be round 1")

			// Verify movement was reset
			s.Require().NotNil(input.MovementRemaining)
			s.Assert().Equal(int32(30), *input.MovementRemaining)

			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act - No EntityID needed, server determines active entity from state
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)

	// Verify combat state
	s.Require().NotNil(output.CombatState)
	s.Assert().Equal("enc-1", output.CombatState.EncounterID)
	s.Assert().Equal(1, output.CombatState.Round)
	s.Assert().Equal(2, output.CombatState.ActiveIndex, "Should be char-2's turn (index 2)")
	s.Assert().Equal(int32(30), output.CombatState.MovementRemaining)
	s.Assert().True(output.CombatState.CombatStarted)
	s.Assert().False(output.CombatState.CombatEnded)

	// Verify turn change event
	s.Require().NotNil(output.TurnChange)
	s.Assert().Equal("char-1", output.TurnChange.PreviousEntityID)
	s.Assert().Equal("char-2", output.TurnChange.NextEntityID)
	s.Assert().Equal(1, output.TurnChange.Round)
	s.Assert().False(output.TurnChange.NewRound)
}

func (s *OrchestratorTestSuite) TestEndTurn_AdvancesToNewRound() {
	// Arrange - char-2 is last in order, ending turn should start round 2
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 1, // char-2's turn (last)
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			s.Assert().Equal(0, input.InitiativeData.Current, "Should wrap to index 0")
			s.Assert().Equal(2, input.InitiativeData.Round, "Should be round 2")
			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act - No EntityID needed, server knows it's char-2's turn from state
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)

	s.Assert().Equal(2, output.CombatState.Round)
	s.Assert().Equal(0, output.CombatState.ActiveIndex)

	s.Assert().True(output.TurnChange.NewRound)
	s.Assert().Equal(2, output.TurnChange.Round)
	s.Assert().Equal("char-2", output.TurnChange.PreviousEntityID)
	s.Assert().Equal("char-1", output.TurnChange.NextEntityID)
}

func (s *OrchestratorTestSuite) TestEndTurn_SkipsMultipleMonsters() {
	// Arrange - char-1 followed by 2 monsters, then char-2
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "goblin-1", Type: "monster"},
			{ID: "goblin-2", Type: "monster"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 20, Modifier: 2, Total: 22},
		{Entity: initiative.NewParticipant("goblin-1", "monster"), Roll: 15, Modifier: 2, Total: 17},
		{Entity: initiative.NewParticipant("goblin-2", "monster"), Roll: 14, Modifier: 2, Total: 16},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			s.Assert().Equal(3, input.InitiativeData.Current, "Should skip both monsters to char-2 at index 3")
			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act - No EntityID needed
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal(3, output.CombatState.ActiveIndex)
	s.Assert().Equal("char-2", output.TurnChange.NextEntityID)
}

func (s *OrchestratorTestSuite) TestEndTurn_SkipsMonstersAcrossRoundBoundary() {
	// Arrange - char-1 at end, followed by monsters at start of order
	// Turn order: goblin-1, goblin-2, char-1 (current)
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "goblin-1", Type: "monster"},
			{ID: "goblin-2", Type: "monster"},
			{ID: "char-1", Type: "character"},
		},
		Current: 2, // char-1's turn (last)
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("goblin-1", "monster"), Roll: 20, Modifier: 2, Total: 22},
		{Entity: initiative.NewParticipant("goblin-2", "monster"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			s.Assert().Equal(2, input.InitiativeData.Current, "Should wrap back to char-1 at index 2")
			s.Assert().Equal(2, input.InitiativeData.Round, "Should advance to round 2")
			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act - No EntityID needed
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal(2, output.CombatState.Round)
	s.Assert().Equal(2, output.CombatState.ActiveIndex)
	s.Assert().True(output.TurnChange.NewRound)
	s.Assert().Equal("char-1", output.TurnChange.NextEntityID) // Back to char-1
}

func (s *OrchestratorTestSuite) TestEndTurn_NilInput() {
	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *OrchestratorTestSuite) TestEndTurn_MissingEncounterID() {
	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *OrchestratorTestSuite) TestEndTurn_EncounterNotFound() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "nonexistent"}).
		Return(&encounterrepo.GetOutput{Data: nil}, nil)

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "nonexistent",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter not found")
}

func (s *OrchestratorTestSuite) TestEndTurn_GetError() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(nil, fmt.Errorf("database error"))

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to load encounter")
}

func (s *OrchestratorTestSuite) TestEndTurn_NoInitiativeData() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:             "enc-1",
				InitiativeData: nil,
			},
		}, nil)

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "no initiative data")
}

func (s *OrchestratorTestSuite) TestEndTurn_EmptyTurnOrder() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID: "enc-1",
				InitiativeData: &initiative.TrackerData{
					Order:   []initiative.EntityData{},
					Current: 0,
					Round:   1,
				},
			},
		}, nil)

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "empty turn order")
}

func (s *OrchestratorTestSuite) TestEndTurn_UpdateError() {
	// Arrange
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:             "enc-1",
				InitiativeData: initiativeData,
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to save turn state")
}

func (s *OrchestratorTestSuite) TestEndTurn_WithRoomData() {
	// Arrange - Test that positions are included when room data exists
	roomData := &spatial.RoomData{
		ID:       "enc-1-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		Entities: map[string]spatial.EntityPlacement{
			"char-1": {
				EntityID:       "char-1",
				EntityType:     "character",
				Position:       spatial.Position{X: 2, Y: 8},
				Size:           1,
				BlocksMovement: true,
			},
			"char-2": {
				EntityID:       "char-2",
				EntityType:     "character",
				Position:       spatial.Position{X: 2, Y: 10},
				Size:           1,
				BlocksMovement: true,
			},
		},
	}

	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				RoomData:        roomData,
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)

	// Verify turn order has positions
	s.Require().Len(output.CombatState.TurnOrder, 2)
	s.Require().NotNil(output.CombatState.TurnOrder[0].Position)
	s.Assert().Equal(float64(2), output.CombatState.TurnOrder[0].Position.X)
	s.Assert().Equal(float64(8), output.CombatState.TurnOrder[0].Position.Y)

	s.Require().NotNil(output.CombatState.TurnOrder[1].Position)
	s.Assert().Equal(float64(2), output.CombatState.TurnOrder[1].Position.X)
	s.Assert().Equal(float64(10), output.CombatState.TurnOrder[1].Position.Y)
}

// ============================================================================
// EndTurn Ownership Validation Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestEndTurn_WithPlayerID_OwnershipValid() {
	// Arrange - char-1 owned by player-1
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0, // char-1's turn
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	// Mock character lookup to verify ownership
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{
			CharacterData: &character.Data{
				ID:       "char-1",
				PlayerID: "player-1", // Owned by player-1
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Act - player-1 ends their own character's turn
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
		PlayerID:    "player-1", // Correct owner
	})

	// Assert - should succeed
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal("char-1", output.TurnChange.PreviousEntityID)
	s.Assert().Equal("char-2", output.TurnChange.NextEntityID)
}

func (s *OrchestratorTestSuite) TestEndTurn_WithPlayerID_NotYourCharacter() {
	// Arrange - char-1 owned by player-1, but player-2 tries to end turn
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0, // char-1's turn
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	// Mock character lookup - char-1 is owned by player-1
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{
			CharacterData: &character.Data{
				ID:       "char-1",
				PlayerID: "player-1", // Owned by player-1
			},
		}, nil)

	// Act - player-2 tries to end someone else's turn
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
		PlayerID:    "player-2", // Wrong player!
	})

	// Assert - should fail with ownership error
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "you do not control character")
	s.Assert().Contains(err.Error(), "char-1")
	s.Assert().Contains(err.Error(), "player-1")
}

func (s *OrchestratorTestSuite) TestEndTurn_WithPlayerID_MonsterTurn() {
	// Arrange - it's a monster's turn, player cannot end it
	// Note: This shouldn't normally happen since we auto-skip monster turns,
	// but we need to handle it in case the state gets corrupted or initial load
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "goblin-1", Type: "monster"}, // Monster first
			{ID: "char-1", Type: "character"},
		},
		Current: 0, // Monster's turn (edge case)
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("goblin-1", "monster"), Roll: 20, Modifier: 2, Total: 22},
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	// Act - player tries to end monster's turn
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
		PlayerID:    "player-1",
	})

	// Assert - should fail with monster turn error
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "monster's turn")
	s.Assert().Contains(err.Error(), "goblin-1")
}

func (s *OrchestratorTestSuite) TestEndTurn_WithPlayerID_CharacterLookupFails() {
	// Arrange - character lookup fails
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:             "enc-1",
				InitiativeData: initiativeData,
			},
		}, nil)

	// Mock character lookup to fail
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(nil, errors.NotFound("character not found"))

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
		PlayerID:    "player-1",
	})

	// Assert - should fail with lookup error
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to validate character ownership")
}

func (s *OrchestratorTestSuite) TestEndTurn_WithoutPlayerID_SkipsValidation() {
	// Arrange - backward compatibility: no PlayerID means no validation
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	// Note: No character repo call expected - validation is skipped

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Act - no PlayerID provided
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
		// PlayerID intentionally omitted
	})

	// Assert - should succeed without validation
	s.Require().NoError(err)
	s.Require().NotNil(output)
}

// ============================================================================
// ActivateFeature Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestActivateFeature_NilInput() {
	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *OrchestratorTestSuite) TestActivateFeature_MissingEncounterID() {
	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		CharacterID: "char-1",
		FeatureID:   "rage",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *OrchestratorTestSuite) TestActivateFeature_MissingCharacterID() {
	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		FeatureID:   "rage",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "character ID is required")
}

func (s *OrchestratorTestSuite) TestActivateFeature_MissingFeatureID() {
	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		CharacterID: "char-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "feature ID is required")
}

func (s *OrchestratorTestSuite) TestActivateFeature_EncounterNotFound() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "nonexistent"}).
		Return(&encounterrepo.GetOutput{Data: nil}, nil)

	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "nonexistent",
		CharacterID: "char-1",
		FeatureID:   "rage",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter not found")
}

func (s *OrchestratorTestSuite) TestActivateFeature_CharacterNotFound() {
	// Arrange - Encounter exists
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{ID: "enc-1"},
		}, nil)

	// Character not found
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(nil, errors.NotFound("character not found"))

	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		CharacterID: "char-1",
		FeatureID:   "rage",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to load character")
}

func (s *OrchestratorTestSuite) TestActivateFeature_FeatureNotFound() {
	// Arrange - Encounter and character exist, but character has no rage feature
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{ID: "enc-1"},
		}, nil)

	// Character has no features (not a barbarian)
	charData := &character.Data{
		ID:      "char-1",
		Name:    "Tordek",
		Level:   1,
		ClassID: "fighter", // Not a barbarian, no rage
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 14,
			abilities.CON: 14,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		Features: []json.RawMessage{}, // No features
	}
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{CharacterData: charData}, nil)

	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		CharacterID: "char-1",
		FeatureID:   "rage",
	})

	// Assert - Returns success=false, not an error
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().False(output.Success)
	s.Assert().Contains(output.Message, "not found")
	s.Assert().NotNil(output.CharacterData)
}

func (s *OrchestratorTestSuite) TestActivateFeature_CharacterLoadsSuccessfully() {
	// This test verifies the happy path up until feature lookup.
	// Testing actual feature activation requires integration tests with the toolkit.
	// The feature loading from JSON in LoadFromData is complex and toolkit-specific.

	// Arrange - Barbarian with rage feature
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{ID: "enc-1"},
		}, nil)

	// Create a basic character (features will be loaded from class by toolkit)
	charData := &character.Data{
		ID:               "char-1",
		Name:             "Grog",
		Level:            1,
		RaceID:           "human",
		ClassID:          "barbarian",
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 14,
			abilities.CON: 14,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		Features: []json.RawMessage{}, // Toolkit loads features from class
	}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{CharacterData: charData}, nil)

	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		CharacterID: "char-1",
		FeatureID:   "rage",
	})

	// Assert - Feature may or may not be found depending on toolkit behavior
	// The key is that we don't get a hard error, and character data is returned
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.CharacterData)
	// Note: Success depends on whether toolkit loads barbarian features by default
	// If not found, output.Success will be false with message about feature not found
}

func (s *OrchestratorTestSuite) TestActivateFeature_EncounterGetError() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(nil, fmt.Errorf("database error"))

	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		CharacterID: "char-1",
		FeatureID:   "rage",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to load encounter")
}

// ============================================================================
// ResolveAttack - Equipped Weapon Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestResolveAttack_UsesEquippedWeapon() {
	// Arrange - Create test character data
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{CharacterData: charData}, nil)

	// Mock equipment slots - character has a longsword equipped
	s.mockCharRepo.EXPECT().
		GetEquipmentSlots(gomock.Any(), characterrepo.GetEquipmentSlotsInput{
			CharacterID: "char-1",
		}).
		Return(&characterrepo.GetEquipmentSlotsOutput{
			EquipmentSlots: &characterrepo.EquipmentSlots{
				MainHand: "longsword",
			},
		}, nil)

	// Arrange - Mock encounter repo
	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)

	// Longsword does slashing damage
	if output.Result.Hit {
		s.Assert().Equal(damage.Slashing, output.Result.DamageType)
	}
}

func (s *OrchestratorTestSuite) TestResolveAttack_NoEquippedWeapon_FallsBackToGreataxe() {
	// Arrange - Create test character data
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{CharacterData: charData}, nil)

	// Mock equipment slots - no weapon equipped (empty mainhand)
	s.mockCharRepo.EXPECT().
		GetEquipmentSlots(gomock.Any(), characterrepo.GetEquipmentSlotsInput{
			CharacterID: "char-1",
		}).
		Return(&characterrepo.GetEquipmentSlotsOutput{
			EquipmentSlots: &characterrepo.EquipmentSlots{
				MainHand: "", // No weapon
			},
		}, nil)

	// Arrange - Mock encounter repo
	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert - Should succeed with fallback to greataxe
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)

	// Greataxe does slashing damage
	if output.Result.Hit {
		s.Assert().Equal(damage.Slashing, output.Result.DamageType)
	}
}

func (s *OrchestratorTestSuite) TestResolveAttack_EquipmentLookupFails_FallsBackToGreataxe() {
	// Arrange - Create test character data
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{CharacterData: charData}, nil)

	// Mock equipment slots - lookup fails (e.g., database error)
	s.mockCharRepo.EXPECT().
		GetEquipmentSlots(gomock.Any(), characterrepo.GetEquipmentSlotsInput{
			CharacterID: "char-1",
		}).
		Return(nil, fmt.Errorf("database connection error"))

	// Arrange - Mock encounter repo
	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Act - Should still succeed with fallback
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert - Should succeed with fallback to greataxe
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)
}

func (s *OrchestratorTestSuite) TestResolveAttack_NilEquipmentSlots_FallsBackToGreataxe() {
	// Arrange - Create test character data
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{CharacterData: charData}, nil)

	// Mock equipment slots - returns nil slots (character never set equipment)
	s.mockCharRepo.EXPECT().
		GetEquipmentSlots(gomock.Any(), characterrepo.GetEquipmentSlotsInput{
			CharacterID: "char-1",
		}).
		Return(&characterrepo.GetEquipmentSlotsOutput{
			EquipmentSlots: nil, // No equipment data at all
		}, nil)

	// Arrange - Mock encounter repo
	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Act - Should still succeed with fallback
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert - Should succeed with fallback to greataxe
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)
}

func (s *OrchestratorTestSuite) TestResolveAttack_UnknownWeaponID_FallsBackToGreataxe() {
	// Arrange - Create test character data
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{CharacterData: charData}, nil)

	// Mock equipment slots - character has an unknown weapon ID
	s.mockCharRepo.EXPECT().
		GetEquipmentSlots(gomock.Any(), characterrepo.GetEquipmentSlotsInput{
			CharacterID: "char-1",
		}).
		Return(&characterrepo.GetEquipmentSlotsOutput{
			EquipmentSlots: &characterrepo.EquipmentSlots{
				MainHand: "magic-sword-of-doom-9000", // Unknown weapon ID
			},
		}, nil)

	// Arrange - Mock encounter repo
	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Act - Should still succeed with fallback
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert - Should succeed with fallback to greataxe
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)
}
