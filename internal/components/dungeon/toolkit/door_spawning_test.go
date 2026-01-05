package toolkit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type DoorSpawningTestSuite struct {
	suite.Suite
}

func TestDoorSpawningTestSuite(t *testing.T) {
	suite.Run(t, new(DoorSpawningTestSuite))
}

func (s *DoorSpawningTestSuite) TestEntranceRoomHasTwoDoors() {
	// Generate a linear layout
	layoutGen := NewToolkitLayoutGenerator(nil)
	layoutOutput, err := layoutGen.Generate(context.Background(), &dungeon.LayoutInput{
		Length:     3,
		LayoutType: dungeon.LayoutLinear,
		Seed:       12345,
	})

	s.Require().NoError(err)
	s.Require().NotNil(layoutOutput)

	// Count connections involving the entrance room (room 0)
	entranceRoomIdx := layoutOutput.StartRoom
	connectionsToEntrance := 0

	for _, conn := range layoutOutput.Connections {
		if conn.FromRoom == entranceRoomIdx || conn.ToRoom == entranceRoomIdx {
			connectionsToEntrance++
		}
	}

	// Entrance room should have 2 connections:
	// 1. From "outside" (entrance door - where party came from)
	// 2. To room 1 (exit door - where party is going)
	s.Equal(2, connectionsToEntrance, "entrance room should have 2 doors (entrance from outside + exit to next room)")
}

func (s *DoorSpawningTestSuite) TestEntranceRoomHasOutsideConnection() {
	layoutGen := NewToolkitLayoutGenerator(nil)
	layoutOutput, err := layoutGen.Generate(context.Background(), &dungeon.LayoutInput{
		Length:     3,
		LayoutType: dungeon.LayoutLinear,
		Seed:       12345,
	})

	s.Require().NoError(err)

	// Find the "outside" connection - it should have FromRoom as a special marker
	// indicating it comes from outside the dungeon
	hasOutsideConnection := false
	entranceRoomIdx := layoutOutput.StartRoom

	for _, conn := range layoutOutput.Connections {
		// Outside connection: ToRoom is entrance, FromRoom is -1 (outside marker)
		if conn.ToRoom == entranceRoomIdx && conn.FromRoom == -1 {
			hasOutsideConnection = true
			break
		}
	}

	s.True(hasOutsideConnection, "entrance room should have a connection from outside (FromRoom=-1)")
}

func (s *DoorSpawningTestSuite) TestPlayerSpawnNearSouthDoor() {
	// Generate a full dungeon to get actual spawn zones
	generator := CreateGenerator(&ToolkitConfig{})

	output, err := generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeSmall,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  2,
		Seed:      12345,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)

	// Find the entrance room (StartRoom)
	var entranceRoom *dungeon.Room
	for _, room := range output.Dungeon.Rooms {
		if room.ID == output.Dungeon.StartRoom {
			entranceRoom = room
			break
		}
	}
	s.Require().NotNil(entranceRoom, "should find entrance room")

	// Find player spawn zone
	var playerSpawnZone *dungeon.Zone
	for _, zone := range entranceRoom.SpawnZones {
		if zone.Type == dungeon.ZoneTypePlayerSpawn {
			playerSpawnZone = zone
			break
		}
	}
	s.Require().NotNil(playerSpawnZone, "entrance room should have player spawn zone")

	// Player spawn positions should be near the south wall (low Z values)
	// South door is at Z ~= 1 (just inside the south wall)
	// Spawn positions should be at Z = 1 or Z = 2 (within 2 rows of south wall)
	for _, pos := range playerSpawnZone.Bounds {
		s.LessOrEqual(pos.Z, 2, "player spawn position Z=%d should be <= 2 (near south door)", pos.Z)
		s.GreaterOrEqual(pos.Z, 1, "player spawn position Z=%d should be >= 1 (inside perimeter)", pos.Z)
	}
}

func (s *DoorSpawningTestSuite) TestSymmetricalSpawnOrder() {
	// Generate a full dungeon to get actual spawn zones
	generator := CreateGenerator(&ToolkitConfig{})

	output, err := generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeSmall,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 5,
		TargetCR:  2,
		Seed:      12345,
	})

	s.Require().NoError(err)

	// Find entrance room
	var entranceRoom *dungeon.Room
	for _, room := range output.Dungeon.Rooms {
		if room.ID == output.Dungeon.StartRoom {
			entranceRoom = room
			break
		}
	}
	s.Require().NotNil(entranceRoom)

	// Find player spawn zone
	var playerSpawnZone *dungeon.Zone
	for _, zone := range entranceRoom.SpawnZones {
		if zone.Type == dungeon.ZoneTypePlayerSpawn {
			playerSpawnZone = zone
			break
		}
	}
	s.Require().NotNil(playerSpawnZone)

	// Should have at least 5 positions for 5 players
	s.GreaterOrEqual(len(playerSpawnZone.Bounds), 5, "should have at least 5 spawn positions")

	// Get room dimensions for center calculation
	centerX := entranceRoom.Shape.Width / 2

	// Verify symmetrical order:
	// Position 0: center back (row 2) - directly behind door
	// Position 1: left back (row 2)
	// Position 2: right back (row 2)
	// Position 3: left flank (row 1, beside door)
	// Position 4: right flank (row 1, beside door)

	positions := playerSpawnZone.Bounds

	// Position 0 should be center (X = centerX based on grid-to-cube)
	// Note: X in cube coords corresponds to column offset
	s.Equal(2, positions[0].Z, "position 0 should be in row 2 (back row)")

	// Positions 1 and 2 should also be in row 2 (back row)
	s.Equal(2, positions[1].Z, "position 1 should be in row 2 (back row)")
	s.Equal(2, positions[2].Z, "position 2 should be in row 2 (back row)")

	// Positions 3 and 4 should be in row 1 (beside door, flanking)
	s.Equal(1, positions[3].Z, "position 3 should be in row 1 (flank)")
	s.Equal(1, positions[4].Z, "position 4 should be in row 1 (flank)")

	// Verify left/right symmetry around center
	// Position 1 should be left of center, position 2 right of center
	s.Less(positions[1].X, positions[0].X, "position 1 should be left of center (position 0)")
	s.Greater(positions[2].X, positions[0].X, "position 2 should be right of center (position 0)")

	// Position 3 should be left flank, position 4 right flank
	s.Less(positions[3].X, positions[4].X, "position 3 should be left of position 4")

	_ = centerX // Used for understanding, actual assertion uses relative positions
}

func (s *DoorSpawningTestSuite) TestMonsterSpawnNearNorthDoor() {
	// Generate a full dungeon to get actual spawn zones
	generator := CreateGenerator(&ToolkitConfig{})

	output, err := generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeSmall,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  2,
		Seed:      12345,
	})

	s.Require().NoError(err)

	// Find the entrance room (StartRoom)
	var entranceRoom *dungeon.Room
	for _, room := range output.Dungeon.Rooms {
		if room.ID == output.Dungeon.StartRoom {
			entranceRoom = room
			break
		}
	}
	s.Require().NotNil(entranceRoom, "should find entrance room")

	// Find monster spawn zone
	var monsterSpawnZone *dungeon.Zone
	for _, zone := range entranceRoom.SpawnZones {
		if zone.Type == dungeon.ZoneTypeMonsterSpawn {
			monsterSpawnZone = zone
			break
		}
	}
	s.Require().NotNil(monsterSpawnZone, "entrance room should have monster spawn zone")

	// Monster spawn positions should be near the north wall (high Z values)
	// North door is at Z ~= height-2 (just inside north wall)
	// Spawn positions should be at Z >= height-3 (within 2 rows of north wall)
	roomHeight := entranceRoom.Shape.Height
	minZForNorth := roomHeight - 3 // Allow positions within 2-3 rows of north wall

	for _, pos := range monsterSpawnZone.Bounds {
		s.GreaterOrEqual(pos.Z, minZForNorth,
			"monster spawn position Z=%d should be >= %d (near north door, room height=%d)",
			pos.Z, minZForNorth, roomHeight)
	}
}
