package toolkit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type PerimeterGeneratorTestSuite struct {
	suite.Suite
	generator *PerimeterGenerator
}

func TestPerimeterGeneratorTestSuite(t *testing.T) {
	suite.Run(t, new(PerimeterGeneratorTestSuite))
}

func (s *PerimeterGeneratorTestSuite) SetupTest() {
	s.generator = NewPerimeterGenerator()
}

func (s *PerimeterGeneratorTestSuite) TestGenerate_RectangularRoom() {
	shape := &dungeon.Shape{
		Bounds: []dungeon.Position{
			{X: 0, Y: 0},
			{X: 10, Y: 0},
			{X: 10, Y: 10},
			{X: 0, Y: 10},
		},
		Width:  10,
		Height: 10,
	}

	result := s.generator.Generate(&PerimeterInput{
		Shape:       shape,
		Connections: []*dungeon.RoomConnection{},
	})

	require.NotNil(s.T(), result)
	assert.Len(s.T(), result.Walls, 4, "rectangular room should have 4 wall segments")

	// All walls should be indestructible perimeter walls
	for _, wall := range result.Walls {
		assert.Equal(s.T(), dungeon.WallTypeIndestructible, wall.Type)
		assert.True(s.T(), wall.BlocksMovement)
		assert.True(s.T(), wall.BlocksLineOfSight)
	}
}

func (s *PerimeterGeneratorTestSuite) TestGenerate_WithDoorOpening() {
	shape := &dungeon.Shape{
		Bounds: []dungeon.Position{
			{X: 0, Y: 0},
			{X: 10, Y: 0},
			{X: 10, Y: 10},
			{X: 0, Y: 10},
		},
		Width:  10,
		Height: 10,
	}

	connections := []*dungeon.RoomConnection{
		{
			FromRoom:     "room1",
			ToRoom:       "room2",
			Type:         dungeon.ConnectionTypeDoor,
			PhysicalHint: "south",
		},
	}

	result := s.generator.Generate(&PerimeterInput{
		Shape:       shape,
		Connections: connections,
	})

	require.NotNil(s.T(), result)
	// Should have more segments due to door gap
	assert.Greater(s.T(), len(result.Walls), 4, "room with door should have split wall segments")
	assert.Len(s.T(), result.DoorPositions, 1, "should have one door position")
}

func (s *PerimeterGeneratorTestSuite) TestGenerate_NilInput() {
	result := s.generator.Generate(nil)

	require.NotNil(s.T(), result)
	assert.Empty(s.T(), result.Walls)
	assert.Empty(s.T(), result.DoorPositions)
}

func (s *PerimeterGeneratorTestSuite) TestGenerate_NilShape() {
	result := s.generator.Generate(&PerimeterInput{
		Shape:       nil,
		Connections: []*dungeon.RoomConnection{},
	})

	require.NotNil(s.T(), result)
	assert.Empty(s.T(), result.Walls)
}

func (s *PerimeterGeneratorTestSuite) TestGenerate_TooFewBounds() {
	shape := &dungeon.Shape{
		Bounds: []dungeon.Position{
			{X: 0, Y: 0},
			{X: 10, Y: 0},
		},
		Width:  10,
		Height: 10,
	}

	result := s.generator.Generate(&PerimeterInput{
		Shape:       shape,
		Connections: []*dungeon.RoomConnection{},
	})

	require.NotNil(s.T(), result)
	assert.Empty(s.T(), result.Walls, "less than 3 bounds should produce no walls")
}

func (s *PerimeterGeneratorTestSuite) TestGenerate_MultipleDoors() {
	shape := &dungeon.Shape{
		Bounds: []dungeon.Position{
			{X: 0, Y: 0},
			{X: 10, Y: 0},
			{X: 10, Y: 10},
			{X: 0, Y: 10},
		},
		Width:  10,
		Height: 10,
	}

	connections := []*dungeon.RoomConnection{
		{
			FromRoom:     "room1",
			ToRoom:       "room2",
			Type:         dungeon.ConnectionTypeDoor,
			PhysicalHint: "south",
		},
		{
			FromRoom:     "room1",
			ToRoom:       "room3",
			Type:         dungeon.ConnectionTypeDoor,
			PhysicalHint: "east",
		},
	}

	result := s.generator.Generate(&PerimeterInput{
		Shape:       shape,
		Connections: connections,
	})

	require.NotNil(s.T(), result)
	assert.Len(s.T(), result.DoorPositions, 2, "should have two door positions")
}

func (s *PerimeterGeneratorTestSuite) TestUpdatePerimeter_WithDoor() {
	shape := &dungeon.Shape{
		Bounds: []dungeon.Position{
			{X: 0, Y: 0},
			{X: 10, Y: 0},
			{X: 10, Y: 10},
			{X: 0, Y: 10},
		},
		Width:  10,
		Height: 10,
	}

	connections := []*dungeon.RoomConnection{
		{
			FromRoom:     "room1",
			ToRoom:       "room2",
			Type:         dungeon.ConnectionTypeDoor,
			PhysicalHint: "south",
		},
	}

	result := s.generator.UpdatePerimeter(&dungeon.UpdatePerimeterInput{
		Shape:       shape,
		Connections: connections,
	})

	require.NotNil(s.T(), result)
	// Should have 5 wall segments: 3 solid + 2 around door
	assert.Len(s.T(), result.Walls, 5, "should have 5 wall segments (3 solid + 2 split)")
	assert.Len(s.T(), result.DoorPositions, 1, "should have one door position")
}

// TestGenerate_HorizontalWallDoorSegmentsOnBoundary verifies that wall segments created around
// a door on a horizontal (south or north) perimeter wall stay on the wall's Z-row.
//
// This is a regression test for issue #465 where the south-wall door segments had incorrect
// Z-coordinates (Z=-1 and Z=+1 instead of Z=0), causing them to be rendered off the boundary
// and appearing invisible to clients that expected boundary walls at Z=0.
func (s *PerimeterGeneratorTestSuite) TestGenerate_HorizontalWallDoorSegmentsOnBoundary() {
	// Use cube-coordinate bounds that match what the toolkit generates for a square room.
	// offsetToCube(col, row) = {X:col, Y:-col-row, Z:row}
	// For width=22, height=20 this matches the boss room size in the debug scenario.
	width := 22
	height := 20
	// Bounds: BL, BR, TR, TL using offsetToCube
	bl := dungeon.Position{X: 0, Y: 0, Z: 0}
	br := dungeon.Position{X: width - 1, Y: -(width - 1), Z: 0}
	tr := dungeon.Position{X: width - 1, Y: -(width - 1) - (height - 1), Z: height - 1}
	tl := dungeon.Position{X: 0, Y: -(height - 1), Z: height - 1}

	// Connection point at the midpoint of the south wall (Z=0 row).
	// midX=10 for width=22: southDoorPos = offsetToCube(10,0) = {X:10, Y:-10, Z:0}
	midX := (width - 1) / 2
	southDoorPos := dungeon.Position{X: midX, Y: -midX, Z: 0}
	southCP := dungeon.ConnectionPoint{
		Name:      "south",
		Position:  southDoorPos,
		Direction: "south",
		Type:      "door",
	}

	shape := &dungeon.Shape{
		Bounds:           []dungeon.Position{bl, br, tr, tl},
		ConnectionPoints: []dungeon.ConnectionPoint{southCP},
		Width:            width,
		Height:           height,
	}

	connections := []*dungeon.RoomConnection{
		{
			FromRoom:     "room-a",
			ToRoom:       "room-b",
			Type:         dungeon.ConnectionTypeDoor,
			PhysicalHint: "south", // door on south wall
		},
	}

	result := s.generator.Generate(&PerimeterInput{
		Shape:       shape,
		Connections: connections,
	})

	s.Require().NotNil(result)

	// The south wall (segment 0, BL→BR) is split into two segments around the door.
	// In the perimeter generator, these are the first two walls (perimeter_0 and perimeter_1).
	// They must both lie entirely on Z=0 (the south boundary row).
	//
	// Pre-fix, the door-adjacent endpoints had Z=±1 because the Y-coordinate from the door
	// position was incorrectly kept when only X was adjusted.
	//
	// We check this directly: perimeter_0 is the wall before the door (X ∈ [0, midX-1], Z=0)
	// and perimeter_1 is the wall after the door (X ∈ [midX+1, width-1], Z=0).
	s.Require().GreaterOrEqual(len(result.Walls), 2, "should have at least 2 walls for the split south wall")

	wallBeforeDoor := result.Walls[0] // perimeter_0
	wallAfterDoor := result.Walls[1]  // perimeter_1

	// Both start and end of wall-before-door must be at Z=0
	s.Assert().Equal(0, wallBeforeDoor.Start.Z,
		"wall before door: Start.Z must be 0 (south wall row), got %d — wall: %+v → %+v",
		wallBeforeDoor.Start.Z, wallBeforeDoor.Start, wallBeforeDoor.End)
	s.Assert().Equal(0, wallBeforeDoor.End.Z,
		"wall before door: End.Z must be 0 (south wall row), got %d — wall: %+v → %+v",
		wallBeforeDoor.End.Z, wallBeforeDoor.Start, wallBeforeDoor.End)

	// Both start and end of wall-after-door must be at Z=0
	s.Assert().Equal(0, wallAfterDoor.Start.Z,
		"wall after door: Start.Z must be 0 (south wall row), got %d — wall: %+v → %+v",
		wallAfterDoor.Start.Z, wallAfterDoor.Start, wallAfterDoor.End)
	s.Assert().Equal(0, wallAfterDoor.End.Z,
		"wall after door: End.Z must be 0 (south wall row), got %d — wall: %+v → %+v",
		wallAfterDoor.End.Z, wallAfterDoor.Start, wallAfterDoor.End)

	// Also verify cube coordinate invariant (X+Y+Z=0) is maintained for all wall endpoints
	for _, wall := range result.Walls {
		s.Assert().Equal(0, wall.Start.X+wall.Start.Y+wall.Start.Z,
			"Start cube invariant violated for wall %s: X(%d)+Y(%d)+Z(%d)=%d",
			wall.ID, wall.Start.X, wall.Start.Y, wall.Start.Z,
			wall.Start.X+wall.Start.Y+wall.Start.Z)
		s.Assert().Equal(0, wall.End.X+wall.End.Y+wall.End.Z,
			"End cube invariant violated for wall %s: X(%d)+Y(%d)+Z(%d)=%d",
			wall.ID, wall.End.X, wall.End.Y, wall.End.Z,
			wall.End.X+wall.End.Y+wall.End.Z)
	}
}

func (s *PerimeterGeneratorTestSuite) TestUpdatePerimeter_NilInput() {
	result := s.generator.UpdatePerimeter(nil)

	require.NotNil(s.T(), result)
	assert.Empty(s.T(), result.Walls)
	assert.Empty(s.T(), result.DoorPositions)
}

func (s *PerimeterGeneratorTestSuite) TestUpdatePerimeter_NilShape() {
	result := s.generator.UpdatePerimeter(&dungeon.UpdatePerimeterInput{
		Shape:       nil,
		Connections: []*dungeon.RoomConnection{},
	})

	require.NotNil(s.T(), result)
	assert.Empty(s.T(), result.Walls)
}
