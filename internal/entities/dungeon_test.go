package entities_test

import (
	"testing"
	"time"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
	"github.com/stretchr/testify/suite"
)

type DungeonTestSuite struct {
	suite.Suite
}

func TestDungeonSuite(t *testing.T) {
	suite.Run(t, new(DungeonTestSuite))
}

func (s *DungeonTestSuite) TestIsBossRoom_True() {
	dungeon := &entities.Dungeon{
		ID:         "dng-1",
		BossRoomID: "boss-room",
	}

	s.True(dungeon.IsBossRoom("boss-room"))
}

func (s *DungeonTestSuite) TestIsBossRoom_False() {
	dungeon := &entities.Dungeon{
		ID:         "dng-1",
		BossRoomID: "boss-room",
	}

	s.False(dungeon.IsBossRoom("other-room"))
}

func (s *DungeonTestSuite) TestIsBossRoom_EmptyBossRoomID() {
	dungeon := &entities.Dungeon{
		ID:         "dng-1",
		BossRoomID: "",
	}

	s.False(dungeon.IsBossRoom("any-room"))
}

func (s *DungeonTestSuite) TestMarkVictory() {
	dungeon := &entities.Dungeon{
		ID:    "dng-1",
		State: entities.DungeonStateActive,
	}

	before := time.Now()
	dungeon.MarkVictory()
	after := time.Now()

	s.Equal(entities.DungeonStateVictorious, dungeon.State)
	s.NotNil(dungeon.CompletedAt)
	s.True(dungeon.CompletedAt.After(before) || dungeon.CompletedAt.Equal(before))
	s.True(dungeon.CompletedAt.Before(after) || dungeon.CompletedAt.Equal(after))
}

func (s *DungeonTestSuite) TestMarkVictory_AlreadyCompleted() {
	// Should still update state even if already completed
	existingTime := time.Now().Add(-time.Hour)
	dungeon := &entities.Dungeon{
		ID:          "dng-1",
		State:       entities.DungeonStateFailed,
		CompletedAt: &existingTime,
	}

	dungeon.MarkVictory()

	s.Equal(entities.DungeonStateVictorious, dungeon.State)
	// CompletedAt should be updated to new time
	s.True(dungeon.CompletedAt.After(existingTime))
}

func (s *DungeonTestSuite) TestMarkFailed() {
	dungeon := &entities.Dungeon{
		ID:    "dng-1",
		State: entities.DungeonStateActive,
	}

	before := time.Now()
	dungeon.MarkFailed()
	after := time.Now()

	s.Equal(entities.DungeonStateFailed, dungeon.State)
	s.NotNil(dungeon.CompletedAt)
	s.True(dungeon.CompletedAt.After(before) || dungeon.CompletedAt.Equal(before))
	s.True(dungeon.CompletedAt.Before(after) || dungeon.CompletedAt.Equal(after))
}

func (s *DungeonTestSuite) TestMarkFailed_AlreadyCompleted() {
	existingTime := time.Now().Add(-time.Hour)
	dungeon := &entities.Dungeon{
		ID:          "dng-1",
		State:       entities.DungeonStateVictorious,
		CompletedAt: &existingTime,
	}

	dungeon.MarkFailed()

	s.Equal(entities.DungeonStateFailed, dungeon.State)
	s.True(dungeon.CompletedAt.After(existingTime))
}

// Test existing methods still work
func (s *DungeonTestSuite) TestIsRoomRevealed() {
	dungeon := &entities.Dungeon{
		RevealedRooms: map[string]bool{"room-1": true},
	}

	s.True(dungeon.IsRoomRevealed("room-1"))
	s.False(dungeon.IsRoomRevealed("room-2"))
}

func (s *DungeonTestSuite) TestIsRoomRevealed_NilMap() {
	dungeon := &entities.Dungeon{}

	s.False(dungeon.IsRoomRevealed("any-room"))
}

func (s *DungeonTestSuite) TestCalculateRoomPositions_TwoConnectedRooms() {
	// Setup: Room A (start) connects to Room B via a door
	// Room A's door is on its north edge (top)
	// Room B's door is on its south edge (bottom)
	roomAID := "room-a"
	roomBID := "room-b"

	// Room dimensions
	width := 18
	height := 16

	// Door positions in each room's local coordinates
	// Room A: door on north edge (middle column, top row)
	// Using offsetToCube: X = col, Z = row, Y = -X - Z
	roomADoorCol, roomADoorRow := width/2, height-1 // (9, 15) in offset
	roomADoorPos := spatial.CubeCoordinate{
		X: roomADoorCol,
		Z: roomADoorRow,
		Y: -roomADoorCol - roomADoorRow, // -24
	}

	// Room B: door on south edge (middle column, bottom row)
	roomBDoorCol, roomBDoorRow := width/2, 0 // (9, 0) in offset
	roomBDoorPos := spatial.CubeCoordinate{
		X: roomBDoorCol,
		Z: roomBDoorRow,
		Y: -roomBDoorCol - roomBDoorRow, // -9
	}

	d := &entities.Dungeon{
		ID:          "test-dungeon",
		StartRoomID: roomAID,
		Rooms: map[string]*dungeon.Room{
			roomAID: {
				ID: roomAID,
				Shape: &dungeon.Shape{
					Width:  width,
					Height: height,
				},
			},
			roomBID: {
				ID: roomBID,
				Shape: &dungeon.Shape{
					Width:  width,
					Height: height,
				},
			},
		},
		Connections: []*environments.ConnectionEdge{
			{
				ID:            "conn-1",
				FromRoomID:    roomAID,
				ToRoomID:      roomBID,
				Type:          "north|wooden door",
				Bidirectional: true,
				FromPosition:  roomADoorPos,
				ToPosition:    roomBDoorPos,
			},
		},
	}

	// Act
	d.CalculateRoomPositions()

	// Assert
	s.Require().NotNil(d.RoomPositions)
	s.Require().Len(d.RoomPositions, 2)

	roomAOrigin, ok := d.GetRoomPosition(roomAID)
	s.Require().True(ok)
	roomBOrigin, ok := d.GetRoomPosition(roomBID)
	s.Require().True(ok)

	s.T().Logf("Room A origin: (%d, %d, %d)", roomAOrigin.X, roomAOrigin.Y, roomAOrigin.Z)
	s.T().Logf("Room B origin: (%d, %d, %d)", roomBOrigin.X, roomBOrigin.Y, roomBOrigin.Z)
	s.T().Logf("Room A door (local): (%d, %d, %d)", roomADoorPos.X, roomADoorPos.Y, roomADoorPos.Z)
	s.T().Logf("Room B door (local): (%d, %d, %d)", roomBDoorPos.X, roomBDoorPos.Y, roomBDoorPos.Z)

	// Room A should be at origin
	s.Equal(spatial.CubeCoordinate{X: 0, Y: 0, Z: 0}, roomAOrigin, "start room should be at origin")

	// Room B should NOT be at origin - it should be offset
	s.NotEqual(spatial.CubeCoordinate{X: 0, Y: 0, Z: 0}, roomBOrigin, "second room should NOT be at origin")

	// Verify the alignment formula:
	// neighborPos + neighborDoorPos = currentPos + currentDoorPos
	// roomBOrigin + roomBDoorPos = roomAOrigin + roomADoorPos
	roomADoorAbsolute := spatial.CubeCoordinate{
		X: roomAOrigin.X + roomADoorPos.X,
		Y: roomAOrigin.Y + roomADoorPos.Y,
		Z: roomAOrigin.Z + roomADoorPos.Z,
	}
	roomBDoorAbsolute := spatial.CubeCoordinate{
		X: roomBOrigin.X + roomBDoorPos.X,
		Y: roomBOrigin.Y + roomBDoorPos.Y,
		Z: roomBOrigin.Z + roomBDoorPos.Z,
	}

	s.T().Logf("Room A door (absolute): (%d, %d, %d)", roomADoorAbsolute.X, roomADoorAbsolute.Y, roomADoorAbsolute.Z)
	s.T().Logf("Room B door (absolute): (%d, %d, %d)", roomBDoorAbsolute.X, roomBDoorAbsolute.Y, roomBDoorAbsolute.Z)

	// The doors should align in absolute coordinates
	s.Equal(roomADoorAbsolute, roomBDoorAbsolute, "doors should align in absolute coordinates")

	// Expected: Room B origin should position Room B north of Room A
	// neighborPos = currentPos + currentDoorPos - neighborDoorPos
	expectedRoomBOrigin := spatial.CubeCoordinate{
		X: roomAOrigin.X + roomADoorPos.X - roomBDoorPos.X, // 0 + 9 - 9 = 0
		Y: roomAOrigin.Y + roomADoorPos.Y - roomBDoorPos.Y, // 0 + (-24) - (-9) = -15
		Z: roomAOrigin.Z + roomADoorPos.Z - roomBDoorPos.Z, // 0 + 15 - 0 = 15
	}
	s.T().Logf("Expected Room B origin: (%d, %d, %d)", expectedRoomBOrigin.X, expectedRoomBOrigin.Y, expectedRoomBOrigin.Z)

	s.Equal(expectedRoomBOrigin, roomBOrigin, "room B should be positioned north of room A")
}
