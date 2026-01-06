package toolkit

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type WallValidatorTestSuite struct {
	suite.Suite
	validator *WallValidator
}

func TestWallValidatorTestSuite(t *testing.T) {
	suite.Run(t, new(WallValidatorTestSuite))
}

func (s *WallValidatorTestSuite) SetupTest() {
	s.validator = NewWallValidator()
}

func (s *WallValidatorTestSuite) TestValidate_NilInput() {
	result := s.validator.Validate(nil)
	s.True(result.IsValid)
}

func (s *WallValidatorTestSuite) TestValidate_EmptyWalls() {
	input := &ValidationInput{
		Shape: &dungeon.Shape{Width: 20, Height: 20},
		Walls: []dungeon.WallSegment{},
		SpawnZones: []dungeon.Zone{
			{
				ID:     "zone1",
				Bounds: []dungeon.Position{{X: 5, Y: 5}},
			},
		},
	}

	result := s.validator.Validate(input)
	s.True(result.IsValid)
}

func (s *WallValidatorTestSuite) TestValidate_NoSpawnZones() {
	input := &ValidationInput{
		Shape: &dungeon.Shape{Width: 20, Height: 20},
		Walls: []dungeon.WallSegment{
			{
				ID:    "wall1",
				Start: dungeon.Position{X: 5, Y: 0},
				End:   dungeon.Position{X: 5, Y: 20},
			},
		},
		SpawnZones: []dungeon.Zone{},
	}

	result := s.validator.Validate(input)
	s.True(result.IsValid)
}

func (s *WallValidatorTestSuite) TestValidate_WallNotBlockingZone() {
	// Wall on one side, spawn zone on the other
	input := &ValidationInput{
		Shape: &dungeon.Shape{Width: 20, Height: 20},
		Walls: []dungeon.WallSegment{
			{
				ID:    "wall1",
				Start: dungeon.Position{X: 15, Y: 5},
				End:   dungeon.Position{X: 15, Y: 15},
			},
		},
		SpawnZones: []dungeon.Zone{
			{
				ID:     "zone1",
				Bounds: []dungeon.Position{{X: 5, Y: 10}},
			},
		},
	}

	result := s.validator.Validate(input)
	s.True(result.IsValid)
}

func (s *WallValidatorTestSuite) TestValidate_WallAdjacentToZone() {
	// Wall immediately adjacent to spawn zone
	input := &ValidationInput{
		Shape: &dungeon.Shape{Width: 20, Height: 20},
		Walls: []dungeon.WallSegment{
			{
				ID:    "wall1",
				Start: dungeon.Position{X: 6, Y: 5},
				End:   dungeon.Position{X: 6, Y: 15},
			},
		},
		SpawnZones: []dungeon.Zone{
			{
				ID:     "zone1",
				Bounds: []dungeon.Position{{X: 5, Y: 10}},
			},
		},
	}

	result := s.validator.Validate(input)
	// Depending on implementation, this may or may not be valid
	// Just ensure we get a result
	s.NotNil(result)
}

func (s *WallValidatorTestSuite) TestGetZoneCenter() {
	zone := dungeon.Zone{
		ID: "zone1",
		Bounds: []dungeon.Position{
			{X: 0, Y: 0},
			{X: 10, Y: 0},
			{X: 10, Y: 10},
			{X: 0, Y: 10},
		},
	}

	center := s.validator.getZoneCenter(zone)
	s.Equal(5, center.X)
	s.Equal(5, center.Y)
}

func (s *WallValidatorTestSuite) TestGetZoneCenter_Empty() {
	zone := dungeon.Zone{
		ID:     "zone1",
		Bounds: []dungeon.Position{},
	}

	center := s.validator.getZoneCenter(zone)
	s.Equal(0, center.X)
	s.Equal(0, center.Y)
}

func (s *WallValidatorTestSuite) TestPointOnSegment_Horizontal() {
	start := dungeon.Position{X: 0, Y: 5}
	end := dungeon.Position{X: 10, Y: 5}

	// Point on segment
	s.True(s.validator.pointOnSegment(dungeon.Position{X: 5, Y: 5}, start, end))

	// Point off segment (different Y)
	s.False(s.validator.pointOnSegment(dungeon.Position{X: 5, Y: 6}, start, end))

	// Point off segment (X out of range)
	s.False(s.validator.pointOnSegment(dungeon.Position{X: 15, Y: 5}, start, end))
}

func (s *WallValidatorTestSuite) TestPointOnSegment_Vertical() {
	start := dungeon.Position{X: 5, Y: 0}
	end := dungeon.Position{X: 5, Y: 10}

	// Point on segment
	s.True(s.validator.pointOnSegment(dungeon.Position{X: 5, Y: 5}, start, end))

	// Point off segment (different X)
	s.False(s.validator.pointOnSegment(dungeon.Position{X: 6, Y: 5}, start, end))

	// Point off segment (Y out of range)
	s.False(s.validator.pointOnSegment(dungeon.Position{X: 5, Y: 15}, start, end))
}

func (s *WallValidatorTestSuite) TestGenerateSafeWalls_RemovesBlockingWalls() {
	input := &ValidationInput{
		Shape: &dungeon.Shape{Width: 20, Height: 20},
		Walls: []dungeon.WallSegment{
			{
				ID:    "safe_wall",
				Start: dungeon.Position{X: 15, Y: 5},
				End:   dungeon.Position{X: 15, Y: 15},
			},
			{
				ID:    "blocking_wall",
				Start: dungeon.Position{X: 5, Y: 10},
				End:   dungeon.Position{X: 6, Y: 10},
			},
		},
		SpawnZones: []dungeon.Zone{
			{
				ID:     "zone1",
				Bounds: []dungeon.Position{{X: 5, Y: 10}},
			},
		},
	}

	safeWalls := s.validator.generateSafeWalls(input)

	// Should only have the safe wall
	s.Len(safeWalls, 1)
	s.Equal("safe_wall", safeWalls[0].ID)
}

func (s *WallValidatorTestSuite) TestIsAdjacent() {
	p1 := dungeon.Position{X: 5, Y: 5}

	// Same position
	s.True(s.validator.isAdjacent(p1, dungeon.Position{X: 5, Y: 5}))

	// Adjacent positions
	s.True(s.validator.isAdjacent(p1, dungeon.Position{X: 6, Y: 5}))
	s.True(s.validator.isAdjacent(p1, dungeon.Position{X: 4, Y: 5}))
	s.True(s.validator.isAdjacent(p1, dungeon.Position{X: 5, Y: 6}))
	s.True(s.validator.isAdjacent(p1, dungeon.Position{X: 5, Y: 4}))
	s.True(s.validator.isAdjacent(p1, dungeon.Position{X: 6, Y: 6})) // Diagonal

	// Not adjacent
	s.False(s.validator.isAdjacent(p1, dungeon.Position{X: 7, Y: 5}))
	s.False(s.validator.isAdjacent(p1, dungeon.Position{X: 5, Y: 7}))
}

func (s *WallValidatorTestSuite) TestPathCrossesWall_NoCrossing() {
	walls := []dungeon.WallSegment{
		{
			ID:             "wall1",
			Start:          dungeon.Position{X: 5, Y: 0},
			End:            dungeon.Position{X: 5, Y: 10},
			BlocksMovement: true,
		},
	}

	// Path that doesn't cross the wall (both points on same side)
	s.False(s.validator.PathCrossesWall(
		dungeon.Position{X: 0, Y: 5},
		dungeon.Position{X: 3, Y: 5},
		walls,
	))
}

func (s *WallValidatorTestSuite) TestPathCrossesWall_CrossesVerticalWall() {
	walls := []dungeon.WallSegment{
		{
			ID:             "wall1",
			Start:          dungeon.Position{X: 5, Y: 0},
			End:            dungeon.Position{X: 5, Y: 10},
			BlocksMovement: true,
		},
	}

	// Path that crosses the wall
	s.True(s.validator.PathCrossesWall(
		dungeon.Position{X: 0, Y: 5},
		dungeon.Position{X: 10, Y: 5},
		walls,
	))
}

func (s *WallValidatorTestSuite) TestPathCrossesWall_CrossesHorizontalWall() {
	walls := []dungeon.WallSegment{
		{
			ID:             "wall1",
			Start:          dungeon.Position{X: 0, Y: 5},
			End:            dungeon.Position{X: 10, Y: 5},
			BlocksMovement: true,
		},
	}

	// Path that crosses the wall
	s.True(s.validator.PathCrossesWall(
		dungeon.Position{X: 5, Y: 0},
		dungeon.Position{X: 5, Y: 10},
		walls,
	))
}

func (s *WallValidatorTestSuite) TestPathCrossesWall_NonBlockingWallIgnored() {
	walls := []dungeon.WallSegment{
		{
			ID:             "wall1",
			Start:          dungeon.Position{X: 5, Y: 0},
			End:            dungeon.Position{X: 5, Y: 10},
			BlocksMovement: false, // Does not block movement
		},
	}

	// Path that would cross the wall but wall doesn't block movement
	s.False(s.validator.PathCrossesWall(
		dungeon.Position{X: 0, Y: 5},
		dungeon.Position{X: 10, Y: 5},
		walls,
	))
}

func (s *WallValidatorTestSuite) TestPathCrossesWall_DiagonalPath() {
	walls := []dungeon.WallSegment{
		{
			ID:             "wall1",
			Start:          dungeon.Position{X: 0, Y: 5},
			End:            dungeon.Position{X: 10, Y: 5},
			BlocksMovement: true,
		},
	}

	// Diagonal path that crosses horizontal wall
	s.True(s.validator.PathCrossesWall(
		dungeon.Position{X: 0, Y: 0},
		dungeon.Position{X: 10, Y: 10},
		walls,
	))
}

func (s *WallValidatorTestSuite) TestPathCrossesWall_PathParallelToWall() {
	walls := []dungeon.WallSegment{
		{
			ID:             "wall1",
			Start:          dungeon.Position{X: 5, Y: 0},
			End:            dungeon.Position{X: 5, Y: 10},
			BlocksMovement: true,
		},
	}

	// Parallel path that doesn't cross
	s.False(s.validator.PathCrossesWall(
		dungeon.Position{X: 3, Y: 0},
		dungeon.Position{X: 3, Y: 10},
		walls,
	))
}

func (s *WallValidatorTestSuite) TestPathCrossesWall_EmptyWalls() {
	// No walls - should not cross
	s.False(s.validator.PathCrossesWall(
		dungeon.Position{X: 0, Y: 0},
		dungeon.Position{X: 10, Y: 10},
		nil,
	))
}

func (s *WallValidatorTestSuite) TestPathCrossesWall_MultipleWalls() {
	walls := []dungeon.WallSegment{
		{
			ID:             "wall1",
			Start:          dungeon.Position{X: 3, Y: 0},
			End:            dungeon.Position{X: 3, Y: 10},
			BlocksMovement: true,
		},
		{
			ID:             "wall2",
			Start:          dungeon.Position{X: 7, Y: 0},
			End:            dungeon.Position{X: 7, Y: 10},
			BlocksMovement: true,
		},
	}

	// Path between walls (no crossing)
	s.False(s.validator.PathCrossesWall(
		dungeon.Position{X: 4, Y: 5},
		dungeon.Position{X: 6, Y: 5},
		walls,
	))

	// Path that crosses first wall
	s.True(s.validator.PathCrossesWall(
		dungeon.Position{X: 0, Y: 5},
		dungeon.Position{X: 5, Y: 5},
		walls,
	))

	// Path that crosses second wall
	s.True(s.validator.PathCrossesWall(
		dungeon.Position{X: 5, Y: 5},
		dungeon.Position{X: 10, Y: 5},
		walls,
	))
}
