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
		s.Assert().Equal("slashing", output.Result.DamageType)

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
	// Arrange - Mock encounter repo save
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.SaveInput) (*encounterrepo.SaveOutput, error) {
			// Verify encounter ID is present and follows expected format
			s.Assert().NotEmpty(input.EncounterID)
			s.Assert().Contains(input.EncounterID, "enc-")
			return &encounterrepo.SaveOutput{Success: true}, nil
		})

	// Act
	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		PlayerID: "player-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotEmpty(output.EncounterID)
	s.Assert().Contains(output.EncounterID, "enc-")
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
	// Arrange - Mock repo save to return error
	expectedError := fmt.Errorf("database error")
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(nil, expectedError)

	// Act
	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		PlayerID: "player-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to save encounter")
	s.Assert().ErrorIs(err, expectedError)
}

func (s *OrchestratorTestSuite) TestCreateDungeon_MinimalInput() {
	// Test with empty PlayerID (optional field for Phase 2)
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.SaveOutput{Success: true}, nil)

	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		PlayerID: "",
	})

	s.Require().NoError(err)
	s.Assert().NotEmpty(output.EncounterID)
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
			PlayerID: fmt.Sprintf("player-%d", i),
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

func (s *OrchestratorTestSuite) TestCreateDungeon_SavesMinimalData() {
	// Verify that for Phase 2, only EncounterID is set
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.SaveInput) (*encounterrepo.SaveOutput, error) {
			// Verify Phase 2: minimal data
			s.Assert().NotEmpty(input.EncounterID)
			s.Assert().Nil(input.RoomData, "RoomData should be nil for Phase 2")
			s.Assert().Nil(input.InitiativeData, "InitiativeData should be nil for Phase 2")
			s.Assert().Nil(input.InitiativeRolls, "InitiativeRolls should be nil for Phase 2")
			return &encounterrepo.SaveOutput{Success: true}, nil
		})

	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		PlayerID: "player-1",
	})

	s.Require().NoError(err)
	s.Assert().NotEmpty(output.EncounterID)
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
				ID:       "enc-1",
				RoomData: roomData,
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
	s.Assert().Equal(int32(30), output.MovementRemaining)
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
