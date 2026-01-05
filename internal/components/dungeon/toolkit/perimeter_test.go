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
