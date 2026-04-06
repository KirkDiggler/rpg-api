package encounter_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	dungeontoolkit "github.com/KirkDiggler/rpg-api/internal/components/dungeon/toolkit"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	dungeonrepo "github.com/KirkDiggler/rpg-api/internal/repositories/dungeons"
	dungeonmock "github.com/KirkDiggler/rpg-api/internal/repositories/dungeons/mock"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
	encountermock "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

type OpenDoorTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	ctx             context.Context
	mockCharRepo    *charactermock.MockRepository
	mockEncRepo     *encountermock.MockRepository
	mockDungeonRepo *dungeonmock.MockRepository
	orchestrator    encounter.Service
}

func TestOpenDoorSuite(t *testing.T) {
	suite.Run(t, new(OpenDoorTestSuite))
}

func (s *OpenDoorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.ctx = context.Background()
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.mockEncRepo = encountermock.NewMockRepository(s.ctrl)
	s.mockDungeonRepo = dungeonmock.NewMockRepository(s.ctrl)

	orc, err := encounter.New(&encounter.Config{
		CharacterRepo: s.mockCharRepo,
		EncounterRepo: s.mockEncRepo,
		DungeonRepo:   s.mockDungeonRepo,
		DungeonGen:    dungeontoolkit.CreateGenerator(&dungeontoolkit.ToolkitConfig{}),
	})
	s.Require().NoError(err)
	s.orchestrator = orc
}

func (s *OpenDoorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// Helper to create a test dungeon with two rooms connected by a door
func (s *OpenDoorTestSuite) createTestDungeon() *entities.Dungeon {
	return &entities.Dungeon{
		ID:          "dng-123",
		EncounterID: "enc-123",
		Connections: []*environments.ConnectionEdge{
			{
				ID:            "conn-1",
				FromRoomID:    "room-1",
				ToRoomID:      "room-2",
				Bidirectional: true,
				Type:          "north door",
			},
		},
		StartRoomID:   "room-1",
		CurrentRoomID: "room-1",
		RevealedRooms: map[string]bool{"room-1": true},
		OpenDoors:     map[string]bool{},
		Rooms: map[string]*dungeon.Room{
			"room-1": {
				ID: "room-1",
				Shape: &dungeon.Shape{
					Width:  20,
					Height: 20,
				},
			},
			"room-2": {
				ID: "room-2",
				Shape: &dungeon.Shape{
					Width:  15,
					Height: 15,
				},
				Encounter: &dungeon.Encounter{
					Monsters: []dungeon.MonsterPlacement{
						{
							ID:        "mp-1",
							MonsterID: "skeleton",
							Position:  dungeon.Position{X: 5, Y: 5},
						},
						{
							ID:        "mp-2",
							MonsterID: "skeleton",
							Position:  dungeon.Position{X: 7, Y: 5},
						},
					},
				},
			},
		},
		State: entities.DungeonStateActive,
	}
}

// Helper to create test encounter data with initiative
func (s *OpenDoorTestSuite) createTestEncounterData() *encounterrepo.EncounterData {
	return &encounterrepo.EncounterData{
		ID: "enc-123",
		RoomData: &spatial.RoomData{
			ID:       "room-1",
			Width:    20,
			Height:   20,
			GridType: spatial.GridTypeHex,
			Entities: map[string]spatial.EntityPlacement{
				"char-1": {EntityID: "char-1", EntityType: "character"},
			},
			// Character positioned adjacent to north door (door is at x=10, z=0)
			// Position at z=1 is one row down from door, which is adjacent
			CubeEntities: map[string]spatial.EntityCubePlacement{
				"char-1": {
					EntityID:   "char-1",
					EntityType: "character",
					CubePosition: spatial.CubeCoordinate{
						X: 10,
						Y: -11, // y = -x - z
						Z: 1,
					},
				},
			},
		},
		InitiativeData: &initiative.TrackerData{
			Order: []initiative.EntityData{
				{ID: "char-1", Type: "character"},
			},
			Current: 0,
			Round:   1,
		},
		InitiativeRolls: []initiative.Roll{
			{Entity: initiative.NewParticipant("char-1", "character"), Roll: 13, Modifier: 2, Total: 15},
		},
		Monsters: []*monster.Data{},
	}
}

// Helper to create test encounter data for room-2 (character at north door)
// Note: connection.Type "north door" is used directly for door position calculation,
// so even in room-2, the door is at the north edge (z=0)
func (s *OpenDoorTestSuite) createTestEncounterDataRoom2() *encounterrepo.EncounterData {
	return &encounterrepo.EncounterData{
		ID: "enc-123",
		RoomData: &spatial.RoomData{
			ID:       "room-2",
			Width:    15,
			Height:   15,
			GridType: spatial.GridTypeHex,
			Entities: map[string]spatial.EntityPlacement{
				"char-1": {EntityID: "char-1", EntityType: "character"},
			},
			// Character positioned adjacent to north door (door is at x=7, z=0)
			// Position at z=1 is one row down from door, which is adjacent
			CubeEntities: map[string]spatial.EntityCubePlacement{
				"char-1": {
					EntityID:   "char-1",
					EntityType: "character",
					CubePosition: spatial.CubeCoordinate{
						X: 7,
						Y: -8, // y = -x - z = -7 - 1
						Z: 1,
					},
				},
			},
		},
		InitiativeData: &initiative.TrackerData{
			Order: []initiative.EntityData{
				{ID: "char-1", Type: "character"},
			},
			Current: 0,
			Round:   1,
		},
		InitiativeRolls: []initiative.Roll{
			{Entity: initiative.NewParticipant("char-1", "character"), Roll: 13, Modifier: 2, Total: 15},
		},
		Monsters: []*monster.Data{},
	}
}

func (s *OpenDoorTestSuite) TestOpenDoor_NilInput() {
	// Act
	output, err := s.orchestrator.OpenDoor(s.ctx, nil)

	// Assert
	s.Require().Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "input is required")
}

func (s *OpenDoorTestSuite) TestOpenDoor_EmptyDungeonID() {
	// Act
	output, err := s.orchestrator.OpenDoor(s.ctx, &encounter.OpenDoorInput{
		ConnectionID: "conn-1",
	})

	// Assert
	s.Require().Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "dungeon ID is required")
}

func (s *OpenDoorTestSuite) TestOpenDoor_EmptyConnectionID() {
	// Act
	output, err := s.orchestrator.OpenDoor(s.ctx, &encounter.OpenDoorInput{
		DungeonID: "dng-123",
	})

	// Assert
	s.Require().Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "connection ID is required")
}

func (s *OpenDoorTestSuite) TestOpenDoor_DungeonNotFound() {
	// Arrange
	s.mockDungeonRepo.EXPECT().
		Get(gomock.Any(), &dungeonrepo.GetInput{DungeonID: "dng-123"}).
		Return(nil, errors.New("dungeon not found"))

	// Act
	output, err := s.orchestrator.OpenDoor(s.ctx, &encounter.OpenDoorInput{
		DungeonID:    "dng-123",
		ConnectionID: "conn-1",
	})

	// Assert
	s.Require().Error(err)
	s.Nil(output)
}

func (s *OpenDoorTestSuite) TestOpenDoor_ConnectionNotFound() {
	// Arrange
	testDungeon := s.createTestDungeon()
	s.mockDungeonRepo.EXPECT().
		Get(gomock.Any(), &dungeonrepo.GetInput{DungeonID: "dng-123"}).
		Return(&dungeonrepo.GetOutput{Dungeon: testDungeon}, nil)

	// Act
	output, err := s.orchestrator.OpenDoor(s.ctx, &encounter.OpenDoorInput{
		DungeonID:    "dng-123",
		ConnectionID: "nonexistent-conn",
	})

	// Assert
	s.Require().Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "connection not found")
}

func (s *OpenDoorTestSuite) TestOpenDoor_DoorAlreadyOpen() {
	// Arrange
	testDungeon := s.createTestDungeon()
	testDungeon.OpenDoors["conn-1"] = true

	s.mockDungeonRepo.EXPECT().
		Get(gomock.Any(), &dungeonrepo.GetInput{DungeonID: "dng-123"}).
		Return(&dungeonrepo.GetOutput{Dungeon: testDungeon}, nil)

	// Act
	output, err := s.orchestrator.OpenDoor(s.ctx, &encounter.OpenDoorInput{
		DungeonID:    "dng-123",
		ConnectionID: "conn-1",
	})

	// Assert
	s.Require().Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "door is already open")
}

func (s *OpenDoorTestSuite) TestOpenDoor_Success_RevealsRoom() {
	// Arrange
	testDungeon := s.createTestDungeon()
	testEncounter := s.createTestEncounterData()

	s.mockDungeonRepo.EXPECT().
		Get(gomock.Any(), &dungeonrepo.GetInput{DungeonID: "dng-123"}).
		Return(&dungeonrepo.GetOutput{Dungeon: testDungeon}, nil)

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-123"}).
		Return(&encounterrepo.GetOutput{Data: testEncounter}, nil)

	// Expect dungeon update to mark door open and room revealed
	s.mockDungeonRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *dungeonrepo.UpdateInput) (*dungeonrepo.UpdateOutput, error) {
			s.Equal("dng-123", input.DungeonID)
			s.True(input.OpenDoors["conn-1"], "door should be marked open")
			s.True(input.RevealedRooms["room-2"], "room-2 should be revealed")
			return &dungeonrepo.UpdateOutput{Success: true}, nil
		})

	// Expect encounter update with new initiative order and merged RoomData
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			s.Equal("enc-123", input.EncounterID)
			s.NotNil(input.InitiativeData, "initiative data should be updated")
			// Should now have 3 entities (1 char + 2 skeletons)
			s.Len(input.InitiativeData.Order, 3)
			// RoomData must be persisted so monsters are visible to buildPerception on EndTurn
			s.NotNil(input.RoomData, "RoomData must be persisted so new monsters can be perceived")
			roomData, ok := input.RoomData.(*spatial.RoomData)
			s.True(ok, "RoomData should be *spatial.RoomData")
			if ok {
				// Original character placement is preserved
				s.Contains(roomData.CubeEntities, "char-1", "character position should be preserved in RoomData")
				// New monsters are present so buildPerception can locate them
				s.Contains(roomData.CubeEntities, "monster-room-2-mp-1", "new monster should be in RoomData")
				s.Contains(roomData.CubeEntities, "monster-room-2-mp-2", "new monster should be in RoomData")
			}
			return &encounterrepo.UpdateOutput{}, nil
		})

	// Act
	output, err := s.orchestrator.OpenDoor(s.ctx, &encounter.OpenDoorInput{
		DungeonID:    "dng-123",
		ConnectionID: "conn-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.NotNil(output.RevealedRoom)
	s.Equal("room-2", output.RevealedRoom.ID)
	s.Len(output.Monsters, 2, "should have 2 monsters")
	s.NotNil(output.CombatState)
}

func (s *OpenDoorTestSuite) TestOpenDoor_NewMonstersPersistedInRoomData() {
	// Regression test: OpenDoor must persist RoomData containing new monster positions
	// so that EndTurn's executeMonsterTurns -> buildPerception can find targets.
	// Without this fix, monsters opened into via a door would see no enemies and stand idle.

	// Arrange
	testDungeon := s.createTestDungeon()
	testEncounter := s.createTestEncounterData()

	s.mockDungeonRepo.EXPECT().
		Get(gomock.Any(), &dungeonrepo.GetInput{DungeonID: "dng-123"}).
		Return(&dungeonrepo.GetOutput{Dungeon: testDungeon}, nil)

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-123"}).
		Return(&encounterrepo.GetOutput{Data: testEncounter}, nil)

	s.mockDungeonRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.UpdateOutput{Success: true}, nil)

	var capturedRoomData *spatial.RoomData
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			s.Require().NotNil(input.RoomData, "RoomData must not be nil: monsters would be invisible to EndTurn")
			rd, ok := input.RoomData.(*spatial.RoomData)
			s.Require().True(ok, "RoomData must be *spatial.RoomData")
			capturedRoomData = rd
			return &encounterrepo.UpdateOutput{}, nil
		})

	// Act
	output, err := s.orchestrator.OpenDoor(s.ctx, &encounter.OpenDoorInput{
		DungeonID:    "dng-123",
		ConnectionID: "conn-1",
	})
	s.Require().NoError(err)
	s.Require().NotNil(output)

	// The persisted RoomData must contain all entities needed for perception:
	// - The character (so monsters can target them)
	// - Each new monster (so buildPerception can locate their own position)
	s.Require().NotNil(capturedRoomData)
	s.Contains(capturedRoomData.CubeEntities, "char-1",
		"character must remain in RoomData after OpenDoor so monsters can perceive them")

	for _, m := range output.Monsters {
		s.Contains(capturedRoomData.CubeEntities, m.ID,
			"monster %q must be in persisted RoomData so it can perceive targets on EndTurn", m.ID)

		placement := capturedRoomData.CubeEntities[m.ID]
		s.Equal(m.ID, placement.EntityID)
		s.Equal("monster", placement.EntityType)
		// Cube constraint: x + y + z == 0
		s.Equal(0, placement.CubePosition.X+placement.CubePosition.Y+placement.CubePosition.Z,
			"cube coordinate must satisfy x+y+z=0 for monster %q", m.ID)
	}
}

func (s *OpenDoorTestSuite) TestOpenDoor_RevealsCorrectRoom() {
	// Test that when player is in room-2 and opens door to room-1, room-1 is revealed
	// Arrange
	testDungeon := s.createTestDungeon()
	testDungeon.CurrentRoomID = "room-2"
	testDungeon.RevealedRooms = map[string]bool{"room-2": true}
	// Room-1 has no monsters for this test
	testDungeon.Rooms["room-1"].Encounter = nil

	testEncounter := s.createTestEncounterDataRoom2()

	s.mockDungeonRepo.EXPECT().
		Get(gomock.Any(), &dungeonrepo.GetInput{DungeonID: "dng-123"}).
		Return(&dungeonrepo.GetOutput{Dungeon: testDungeon}, nil)

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-123"}).
		Return(&encounterrepo.GetOutput{Data: testEncounter}, nil)

	s.mockDungeonRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *dungeonrepo.UpdateInput) (*dungeonrepo.UpdateOutput, error) {
			s.True(input.RevealedRooms["room-1"], "room-1 should be revealed")
			return &dungeonrepo.UpdateOutput{Success: true}, nil
		})

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{}, nil)

	// Act
	output, err := s.orchestrator.OpenDoor(s.ctx, &encounter.OpenDoorInput{
		DungeonID:    "dng-123",
		ConnectionID: "conn-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Equal("room-1", output.RevealedRoom.ID)
}

func (s *OpenDoorTestSuite) TestOpenDoor_MonsterIDsIncludeRoomID() {
	// Monster IDs should include the room ID to be globally unique across rooms.
	// e.g., "monster-room-2-mp-1" instead of just "monster-mp-1"
	// Arrange
	testDungeon := s.createTestDungeon()
	testEncounter := s.createTestEncounterData()

	s.mockDungeonRepo.EXPECT().
		Get(gomock.Any(), &dungeonrepo.GetInput{DungeonID: "dng-123"}).
		Return(&dungeonrepo.GetOutput{Dungeon: testDungeon}, nil)

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-123"}).
		Return(&encounterrepo.GetOutput{Data: testEncounter}, nil)

	s.mockDungeonRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.UpdateOutput{Success: true}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{}, nil)

	// Act
	output, err := s.orchestrator.OpenDoor(s.ctx, &encounter.OpenDoorInput{
		DungeonID:    "dng-123",
		ConnectionID: "conn-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().Len(output.Monsters, 2)

	// Monster IDs should contain the room ID ("room-2")
	for _, m := range output.Monsters {
		s.Contains(m.ID, "room-2", fmt.Sprintf("monster ID %q should contain room ID 'room-2'", m.ID))
	}

	// Monster IDs should be distinct and include placement ID
	s.Equal("monster-room-2-mp-1", output.Monsters[0].ID)
	s.Equal("monster-room-2-mp-2", output.Monsters[1].ID)

	// These IDs should differ from what room-1 monsters would be
	// (room-1 has no encounter in our test, but the format proves uniqueness)
	for _, m := range output.Monsters {
		s.NotEqual("monster-mp-1", m.ID, "monster ID should not be room-scoped")
		s.NotEqual("monster-mp-2", m.ID, "monster ID should not be room-scoped")
	}
}

func (s *OpenDoorTestSuite) TestOpenDoor_BothRoomsRevealed_Error() {
	// If both rooms connected by the door are already revealed, return error
	// Arrange
	testDungeon := s.createTestDungeon()
	testDungeon.RevealedRooms = map[string]bool{"room-1": true, "room-2": true}

	s.mockDungeonRepo.EXPECT().
		Get(gomock.Any(), &dungeonrepo.GetInput{DungeonID: "dng-123"}).
		Return(&dungeonrepo.GetOutput{Dungeon: testDungeon}, nil)

	// Act
	output, err := s.orchestrator.OpenDoor(s.ctx, &encounter.OpenDoorInput{
		DungeonID:    "dng-123",
		ConnectionID: "conn-1",
	})

	// Assert
	s.Require().Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "both rooms already revealed")
}
