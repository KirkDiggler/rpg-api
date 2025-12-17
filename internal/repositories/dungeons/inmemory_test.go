package dungeons

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
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

// Helper to create a test dungeon using toolkit and component types
func (s *InMemoryRepositoryTestSuite) createTestDungeon(id, encounterID string) *entities.Dungeon {
	return &entities.Dungeon{
		ID:          id,
		EncounterID: encounterID,
		Seed:        12345,
		State:       entities.DungeonStateActive,
		Rooms: map[string]*dungeon.Room{
			"room-1": {
				ID: "room-1",
				Shape: &dungeon.Shape{
					Width:  10,
					Height: 10,
				},
			},
			"room-2": {
				ID: "room-2",
				Shape: &dungeon.Shape{
					Width:  15,
					Height: 15,
				},
			},
		},
		Connections: []*environments.ConnectionEdge{
			{
				ID:            "conn-1",
				FromRoomID:    "room-1",
				ToRoomID:      "room-2",
				Type:          "door",
				Bidirectional: true,
			},
		},
		StartRoomID:   "room-1",
		BossRoomID:    "room-2",
		CurrentRoomID: "room-1",
		RevealedRooms: map[string]bool{"room-1": true},
		OpenDoors:     map[string]bool{},
		CreatedAt:     time.Now(),
	}
}

// Save Tests

func (s *InMemoryRepositoryTestSuite) TestSave_Success() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	input := &SaveInput{Dungeon: d}

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

func (s *InMemoryRepositoryTestSuite) TestSave_NilDungeon() {
	// Arrange
	input := &SaveInput{Dungeon: nil}

	// Act
	output, err := s.repo.Save(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "dungeon is required")
}

func (s *InMemoryRepositoryTestSuite) TestSave_EmptyDungeonID() {
	// Arrange
	d := s.createTestDungeon("", "enc-1")
	input := &SaveInput{Dungeon: d}

	// Act
	output, err := s.repo.Save(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "dungeon ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestSave_UpdatesEncounterIndex() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act - Should be retrievable by encounter ID
	output, err := s.repo.GetByEncounterID(s.ctx, &GetByEncounterIDInput{EncounterID: "enc-1"})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal("dng-1", output.Dungeon.ID)
}

// Get Tests

func (s *InMemoryRepositoryTestSuite) TestGet_Success() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act
	output, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.Dungeon)
	s.Assert().Equal("dng-1", output.Dungeon.ID)
	s.Assert().Equal("enc-1", output.Dungeon.EncounterID)
	s.Assert().Len(output.Dungeon.Rooms, 2)
	s.Assert().Len(output.Dungeon.Connections, 1)
}

func (s *InMemoryRepositoryTestSuite) TestGet_NilInput() {
	// Act
	output, err := s.repo.Get(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *InMemoryRepositoryTestSuite) TestGet_EmptyDungeonID() {
	// Act
	output, err := s.repo.Get(s.ctx, &GetInput{DungeonID: ""})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "dungeon ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestGet_NotFound() {
	// Act
	output, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "nonexistent"})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "dungeon not found")
}

func (s *InMemoryRepositoryTestSuite) TestGet_ReturnsCopy() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act - Get and modify the returned data
	output1, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})
	s.Require().NoError(err)
	output1.Dungeon.State = entities.DungeonStateVictorious
	output1.Dungeon.RevealedRooms["room-2"] = true

	// Get again and verify original is unchanged
	output2, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal(entities.DungeonStateActive, output2.Dungeon.State, "State should not be modified")
	s.Assert().False(output2.Dungeon.RevealedRooms["room-2"], "RevealedRooms should not be modified")
}

// GetByEncounterID Tests

func (s *InMemoryRepositoryTestSuite) TestGetByEncounterID_Success() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act
	output, err := s.repo.GetByEncounterID(s.ctx, &GetByEncounterIDInput{EncounterID: "enc-1"})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal("dng-1", output.Dungeon.ID)
	s.Assert().Equal("enc-1", output.Dungeon.EncounterID)
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounterID_NilInput() {
	// Act
	output, err := s.repo.GetByEncounterID(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounterID_EmptyEncounterID() {
	// Act
	output, err := s.repo.GetByEncounterID(s.ctx, &GetByEncounterIDInput{EncounterID: ""})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestGetByEncounterID_NotFound() {
	// Act
	output, err := s.repo.GetByEncounterID(s.ctx, &GetByEncounterIDInput{EncounterID: "nonexistent"})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "dungeon not found")
}

// Update Tests

func (s *InMemoryRepositoryTestSuite) TestUpdate_Success() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act
	newState := entities.DungeonStateVictorious
	output, err := s.repo.Update(s.ctx, &UpdateInput{
		DungeonID: "dng-1",
		State:     &newState,
	})

	// Assert
	s.Require().NoError(err)
	s.Assert().True(output.Success)

	// Verify by Get
	getOutput, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})
	s.Require().NoError(err)
	s.Assert().Equal(entities.DungeonStateVictorious, getOutput.Dungeon.State)
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_NilInput() {
	// Act
	output, err := s.repo.Update(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_EmptyDungeonID() {
	// Act
	output, err := s.repo.Update(s.ctx, &UpdateInput{DungeonID: ""})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "dungeon ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_NotFound() {
	// Act
	output, err := s.repo.Update(s.ctx, &UpdateInput{DungeonID: "nonexistent"})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "dungeon not found")
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_RevealedRooms() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act - Reveal room-2
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		DungeonID:     "dng-1",
		RevealedRooms: map[string]bool{"room-2": true},
	})
	s.Require().NoError(err)

	// Verify
	getOutput, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})
	s.Require().NoError(err)
	s.Assert().True(getOutput.Dungeon.RevealedRooms["room-1"], "room-1 should still be revealed")
	s.Assert().True(getOutput.Dungeon.RevealedRooms["room-2"], "room-2 should now be revealed")
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_OpenDoors() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act - Open conn-1
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		DungeonID: "dng-1",
		OpenDoors: map[string]bool{"conn-1": true},
	})
	s.Require().NoError(err)

	// Verify
	getOutput, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})
	s.Require().NoError(err)
	s.Assert().True(getOutput.Dungeon.OpenDoors["conn-1"], "conn-1 should be open")
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_CurrentRoomID() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act
	newRoom := "room-2"
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		DungeonID:     "dng-1",
		CurrentRoomID: &newRoom,
	})
	s.Require().NoError(err)

	// Verify
	getOutput, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})
	s.Require().NoError(err)
	s.Assert().Equal("room-2", getOutput.Dungeon.CurrentRoomID)
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_Metrics() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act
	roomsCleared := 2
	monstersKilled := 5
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		DungeonID:      "dng-1",
		RoomsCleared:   &roomsCleared,
		MonstersKilled: &monstersKilled,
	})
	s.Require().NoError(err)

	// Verify
	getOutput, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})
	s.Require().NoError(err)
	s.Assert().Equal(2, getOutput.Dungeon.RoomsCleared)
	s.Assert().Equal(5, getOutput.Dungeon.MonstersKilled)
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_CompletedAt() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act
	completedAt := time.Now()
	newState := entities.DungeonStateVictorious
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		DungeonID:   "dng-1",
		State:       &newState,
		CompletedAt: &completedAt,
	})
	s.Require().NoError(err)

	// Verify
	getOutput, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})
	s.Require().NoError(err)
	s.Require().NotNil(getOutput.Dungeon.CompletedAt)
	s.Assert().Equal(completedAt.Unix(), getOutput.Dungeon.CompletedAt.Unix())
}

func (s *InMemoryRepositoryTestSuite) TestUpdate_OnlyUpdatesProvided() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act - Update only rooms cleared
	roomsCleared := 1
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		DungeonID:    "dng-1",
		RoomsCleared: &roomsCleared,
	})
	s.Require().NoError(err)

	// Verify - other fields unchanged
	getOutput, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})
	s.Require().NoError(err)
	s.Assert().Equal(1, getOutput.Dungeon.RoomsCleared)
	s.Assert().Equal(entities.DungeonStateActive, getOutput.Dungeon.State, "State should not change")
	s.Assert().Equal("room-1", getOutput.Dungeon.CurrentRoomID, "CurrentRoomID should not change")
	s.Assert().Equal(0, getOutput.Dungeon.MonstersKilled, "MonstersKilled should not change")
}

// Delete Tests

func (s *InMemoryRepositoryTestSuite) TestDelete_Success() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act
	output, err := s.repo.Delete(s.ctx, &DeleteInput{DungeonID: "dng-1"})

	// Assert
	s.Require().NoError(err)
	s.Assert().True(output.Success)

	// Verify it's gone
	_, err = s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})
	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "dungeon not found")
}

func (s *InMemoryRepositoryTestSuite) TestDelete_NilInput() {
	// Act
	output, err := s.repo.Delete(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *InMemoryRepositoryTestSuite) TestDelete_EmptyDungeonID() {
	// Act
	output, err := s.repo.Delete(s.ctx, &DeleteInput{DungeonID: ""})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "dungeon ID is required")
}

func (s *InMemoryRepositoryTestSuite) TestDelete_NotFound() {
	// Act
	output, err := s.repo.Delete(s.ctx, &DeleteInput{DungeonID: "nonexistent"})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "dungeon not found")
}

func (s *InMemoryRepositoryTestSuite) TestDelete_RemovesEncounterIndex() {
	// Arrange
	d := s.createTestDungeon("dng-1", "enc-1")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Act
	_, err = s.repo.Delete(s.ctx, &DeleteInput{DungeonID: "dng-1"})
	s.Require().NoError(err)

	// Verify encounter index is removed
	_, err = s.repo.GetByEncounterID(s.ctx, &GetByEncounterIDInput{EncounterID: "enc-1"})
	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "dungeon not found")
}

// Integration Tests - Full workflow

func (s *InMemoryRepositoryTestSuite) TestWorkflow_DungeonRun() {
	// Create dungeon
	d := s.createTestDungeon("dng-workflow", "enc-workflow")
	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: d})
	s.Require().NoError(err)

	// Verify initial state
	getOutput, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-workflow"})
	s.Require().NoError(err)
	s.Assert().Equal(entities.DungeonStateActive, getOutput.Dungeon.State)
	s.Assert().Equal("room-1", getOutput.Dungeon.CurrentRoomID)
	s.Assert().True(getOutput.Dungeon.RevealedRooms["room-1"])
	s.Assert().False(getOutput.Dungeon.RevealedRooms["room-2"])

	// Open door and reveal boss room
	newRoom := "room-2"
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		DungeonID:     "dng-workflow",
		OpenDoors:     map[string]bool{"conn-1": true},
		RevealedRooms: map[string]bool{"room-2": true},
		CurrentRoomID: &newRoom,
	})
	s.Require().NoError(err)

	// Verify exploration state
	getOutput, err = s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-workflow"})
	s.Require().NoError(err)
	s.Assert().Equal("room-2", getOutput.Dungeon.CurrentRoomID)
	s.Assert().True(getOutput.Dungeon.OpenDoors["conn-1"])
	s.Assert().True(getOutput.Dungeon.RevealedRooms["room-2"])

	// Complete dungeon (victory)
	completedAt := time.Now()
	victoryState := entities.DungeonStateVictorious
	roomsCleared := 2
	monstersKilled := 3
	_, err = s.repo.Update(s.ctx, &UpdateInput{
		DungeonID:      "dng-workflow",
		State:          &victoryState,
		CompletedAt:    &completedAt,
		RoomsCleared:   &roomsCleared,
		MonstersKilled: &monstersKilled,
	})
	s.Require().NoError(err)

	// Verify final state
	getOutput, err = s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-workflow"})
	s.Require().NoError(err)
	s.Assert().Equal(entities.DungeonStateVictorious, getOutput.Dungeon.State)
	s.Assert().NotNil(getOutput.Dungeon.CompletedAt)
	s.Assert().Equal(2, getOutput.Dungeon.RoomsCleared)
	s.Assert().Equal(3, getOutput.Dungeon.MonstersKilled)
}

func (s *InMemoryRepositoryTestSuite) TestWorkflow_MultipleDungeons() {
	// Create multiple dungeons
	dungeon1 := s.createTestDungeon("dng-1", "enc-1")
	dungeon2 := s.createTestDungeon("dng-2", "enc-2")

	_, err := s.repo.Save(s.ctx, &SaveInput{Dungeon: dungeon1})
	s.Require().NoError(err)
	_, err = s.repo.Save(s.ctx, &SaveInput{Dungeon: dungeon2})
	s.Require().NoError(err)

	// Verify both retrievable by ID
	out1, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})
	s.Require().NoError(err)
	s.Assert().Equal("dng-1", out1.Dungeon.ID)

	out2, err := s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-2"})
	s.Require().NoError(err)
	s.Assert().Equal("dng-2", out2.Dungeon.ID)

	// Verify both retrievable by encounter ID
	out1, err = s.repo.GetByEncounterID(s.ctx, &GetByEncounterIDInput{EncounterID: "enc-1"})
	s.Require().NoError(err)
	s.Assert().Equal("dng-1", out1.Dungeon.ID)

	out2, err = s.repo.GetByEncounterID(s.ctx, &GetByEncounterIDInput{EncounterID: "enc-2"})
	s.Require().NoError(err)
	s.Assert().Equal("dng-2", out2.Dungeon.ID)

	// Delete one
	_, err = s.repo.Delete(s.ctx, &DeleteInput{DungeonID: "dng-1"})
	s.Require().NoError(err)

	// Verify only one remains
	_, err = s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-1"})
	s.Require().Error(err)

	out2, err = s.repo.Get(s.ctx, &GetInput{DungeonID: "dng-2"})
	s.Require().NoError(err)
	s.Assert().Equal("dng-2", out2.Dungeon.ID)
}
