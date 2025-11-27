package encounters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
)

type InMemoryRepositoryTestSuite struct {
	suite.Suite
	repo *InMemoryRepository
	ctx  context.Context
}

func (s *InMemoryRepositoryTestSuite) SetupTest() {
	s.repo = NewInMemory()
	s.ctx = context.Background()
}

func TestInMemoryRepositorySuite(t *testing.T) {
	suite.Run(t, new(InMemoryRepositoryTestSuite))
}

// Save Tests

func (s *InMemoryRepositoryTestSuite) TestSave_Success() {
	// Arrange
	input := &SaveInput{
		EncounterID:       "enc-1",
		RoomData:          "test-room",
		InitiativeData:    &initiative.TrackerData{},
		InitiativeRolls:   []initiative.Roll{{Total: 15}},
		MovementRemaining: 30,
	}

	// Act
	output, err := s.repo.Save(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Assert().True(output.Success)
}

func (s *InMemoryRepositoryTestSuite) TestSave_NilInput() {
	// Act
	output, err := s.repo.Save(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *InMemoryRepositoryTestSuite) TestSave_EmptyEncounterID() {
	// Arrange
	input := &SaveInput{
		EncounterID: "",
	}

	// Act
	output, err := s.repo.Save(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestSave_PersistsMovementRemaining() {
	// Arrange
	input := &SaveInput{
		EncounterID:       "enc-1",
		MovementRemaining: 25,
	}

	// Act
	_, err := s.repo.Save(s.ctx, input)
	s.Require().NoError(err)

	// Verify by Get
	getOutput, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-1"})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal(int32(25), getOutput.Data.MovementRemaining)
}

// Get Tests

func (s *InMemoryRepositoryTestSuite) TestGet_Success() {
	// Arrange - Save an encounter first
	_, err := s.repo.Save(s.ctx, &SaveInput{
		EncounterID:       "enc-1",
		RoomData:          "test-room",
		InitiativeData:    &initiative.TrackerData{},
		InitiativeRolls:   []initiative.Roll{{Total: 15}},
		MovementRemaining: 30,
	})
	s.Require().NoError(err)

	// Act
	output, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-1"})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.Data)
	s.Assert().Equal("enc-1", output.Data.ID)
	s.Assert().Equal("test-room", output.Data.RoomData)
	s.Assert().NotNil(output.Data.InitiativeData)
	s.Assert().Len(output.Data.InitiativeRolls, 1)
	s.Assert().Equal(int32(30), output.Data.MovementRemaining)
}

func (s *InMemoryRepositoryTestSuite) TestGet_NilInput() {
	// Act
	output, err := s.repo.Get(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *InMemoryRepositoryTestSuite) TestGet_EmptyEncounterID() {
	// Arrange
	input := &GetInput{
		EncounterID: "",
	}

	// Act
	output, err := s.repo.Get(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestGet_NotFound() {
	// Act
	output, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "nonexistent"})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter not found")
}

func (s *InMemoryRepositoryTestSuite) TestGet_ReturnsMovementRemaining() {
	// Arrange - Save an encounter with specific movement
	_, err := s.repo.Save(s.ctx, &SaveInput{
		EncounterID:       "enc-1",
		MovementRemaining: 15,
	})
	s.Require().NoError(err)

	// Act
	output, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-1"})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal(int32(15), output.Data.MovementRemaining)
}

func (s *InMemoryRepositoryTestSuite) TestGet_ReturnsCopy() {
	// Arrange - Save an encounter
	_, err := s.repo.Save(s.ctx, &SaveInput{
		EncounterID:       "enc-1",
		MovementRemaining: 30,
	})
	s.Require().NoError(err)

	// Act - Get and modify the returned data
	output1, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-1"})
	s.Require().NoError(err)
	output1.Data.MovementRemaining = 0

	// Get again and verify original is unchanged
	output2, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-1"})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal(int32(30), output2.Data.MovementRemaining, "Original data should not be modified")
}

// Update Tests

func (s *InMemoryRepositoryTestSuite) TestUpdate_Success() {
	// Arrange - Save an encounter first
	_, err := s.repo.Save(s.ctx, &SaveInput{
		EncounterID:       "enc-1",
		RoomData:          "test-room",
		MovementRemaining: 30,
	})
	s.Require().NoError(err)

	// Act
	newMovement := int32(20)
	output, err := s.repo.Update(s.ctx, &UpdateInput{
		EncounterID:       "enc-1",
		MovementRemaining: &newMovement,
	})

	// Assert
	s.Require().NoError(err)
	s.Assert().True(output.Success)

	// Verify by Get
	getOutput, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-1"})
	s.Require().NoError(err)
	s.Assert().Equal(int32(20), getOutput.Data.MovementRemaining)
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_NilInput() {
	// Act
	output, err := s.repo.Update(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_EmptyEncounterID() {
	// Arrange
	input := &UpdateInput{
		EncounterID: "",
	}

	// Act
	output, err := s.repo.Update(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_NotFound() {
	// Act
	output, err := s.repo.Update(s.ctx, &UpdateInput{EncounterID: "nonexistent"})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter not found")
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_OnlyUpdatesProvided_MovementRemaining() {
	// Arrange - Save an encounter with all fields
	_, err := s.repo.Save(s.ctx, &SaveInput{
		EncounterID:       "enc-1",
		RoomData:          "original-room",
		InitiativeData:    &initiative.TrackerData{},
		MovementRemaining: 30,
	})
	s.Require().NoError(err)

	// Act - Update only MovementRemaining
	newMovement := int32(10)
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		EncounterID:       "enc-1",
		MovementRemaining: &newMovement,
	})
	s.Require().NoError(err)

	// Verify
	getOutput, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-1"})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal(int32(10), getOutput.Data.MovementRemaining, "MovementRemaining should be updated")
	s.Assert().Equal("original-room", getOutput.Data.RoomData, "RoomData should not change")
	s.Assert().NotNil(getOutput.Data.InitiativeData, "InitiativeData should not change")
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_DoesNotUpdateMovementWhenNil() {
	// Arrange - Save an encounter
	_, err := s.repo.Save(s.ctx, &SaveInput{
		EncounterID:       "enc-1",
		MovementRemaining: 30,
	})
	s.Require().NoError(err)

	// Act - Update with nil MovementRemaining
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		EncounterID:       "enc-1",
		MovementRemaining: nil, // Explicitly nil
		RoomData:          "new-room",
	})
	s.Require().NoError(err)

	// Verify
	getOutput, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-1"})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal(int32(30), getOutput.Data.MovementRemaining, "MovementRemaining should NOT be updated when nil")
	s.Assert().Equal("new-room", getOutput.Data.RoomData, "RoomData should be updated")
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_CanSetMovementToZero() {
	// Arrange - Save an encounter with movement
	_, err := s.repo.Save(s.ctx, &SaveInput{
		EncounterID:       "enc-1",
		MovementRemaining: 30,
	})
	s.Require().NoError(err)

	// Act - Update to zero movement (explicitly)
	zeroMovement := int32(0)
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		EncounterID:       "enc-1",
		MovementRemaining: &zeroMovement,
	})
	s.Require().NoError(err)

	// Verify
	getOutput, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-1"})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal(int32(0), getOutput.Data.MovementRemaining, "MovementRemaining should be set to 0")
}

// Delete Tests

func (s *InMemoryRepositoryTestSuite) TestDelete_Success() {
	// Arrange - Save an encounter first
	_, err := s.repo.Save(s.ctx, &SaveInput{
		EncounterID: "enc-1",
	})
	s.Require().NoError(err)

	// Act
	output, err := s.repo.Delete(s.ctx, &DeleteInput{EncounterID: "enc-1"})

	// Assert
	s.Require().NoError(err)
	s.Assert().True(output.Success)

	// Verify it's gone
	_, err = s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-1"})
	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "encounter not found")
}

func (s *InMemoryRepositoryTestSuite) TestDelete_NilInput() {
	// Act
	output, err := s.repo.Delete(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *InMemoryRepositoryTestSuite) TestDelete_EmptyEncounterID() {
	// Arrange
	input := &DeleteInput{
		EncounterID: "",
	}

	// Act
	output, err := s.repo.Delete(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestDelete_NotFound() {
	// Act
	output, err := s.repo.Delete(s.ctx, &DeleteInput{EncounterID: "nonexistent"})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter not found")
}

// Integration Tests - Full workflow

func (s *InMemoryRepositoryTestSuite) TestWorkflow_SaveGetUpdateDelete() {
	// Save
	_, err := s.repo.Save(s.ctx, &SaveInput{
		EncounterID:       "enc-workflow",
		RoomData:          "initial-room",
		MovementRemaining: 30,
	})
	s.Require().NoError(err)

	// Get - verify initial state
	getOutput, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-workflow"})
	s.Require().NoError(err)
	s.Assert().Equal(int32(30), getOutput.Data.MovementRemaining)

	// Update movement
	newMovement := int32(15)
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		EncounterID:       "enc-workflow",
		MovementRemaining: &newMovement,
	})
	s.Require().NoError(err)

	// Get - verify update
	getOutput, err = s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-workflow"})
	s.Require().NoError(err)
	s.Assert().Equal(int32(15), getOutput.Data.MovementRemaining)

	// Delete
	_, err = s.repo.Delete(s.ctx, &DeleteInput{EncounterID: "enc-workflow"})
	s.Require().NoError(err)

	// Get - verify deleted
	_, err = s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-workflow"})
	s.Require().Error(err)
}

func (s *InMemoryRepositoryTestSuite) TestWorkflow_MovementDepletion() {
	// This test simulates a character moving multiple times until movement is depleted

	// Start encounter with full movement (30 feet)
	_, err := s.repo.Save(s.ctx, &SaveInput{
		EncounterID:       "enc-movement",
		MovementRemaining: 30,
	})
	s.Require().NoError(err)

	// First move - use 10 feet
	movement := int32(20)
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		EncounterID:       "enc-movement",
		MovementRemaining: &movement,
	})
	s.Require().NoError(err)

	// Second move - use another 15 feet
	movement = int32(5)
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		EncounterID:       "enc-movement",
		MovementRemaining: &movement,
	})
	s.Require().NoError(err)

	// Third move - use remaining 5 feet
	movement = int32(0)
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		EncounterID:       "enc-movement",
		MovementRemaining: &movement,
	})
	s.Require().NoError(err)

	// Verify movement is depleted
	getOutput, err := s.repo.Get(s.ctx, &GetInput{EncounterID: "enc-movement"})
	s.Require().NoError(err)
	s.Assert().Equal(int32(0), getOutput.Data.MovementRemaining)
}
