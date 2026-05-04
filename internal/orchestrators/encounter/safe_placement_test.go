package encounter

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type SafePlacementTestSuite struct {
	suite.Suite
}

func TestSafePlacementTestSuite(t *testing.T) {
	suite.Run(t, new(SafePlacementTestSuite))
}

func (s *SafePlacementTestSuite) TestFindSafeFallbackPosition_AvoidsWalls() {
	// Room with a pillar in the center
	room := &dungeon.Room{
		ID: "test-room",
		Shape: &dungeon.Shape{
			Width:  10,
			Height: 10,
		},
		Walls: []dungeon.WallSegment{
			// Perimeter walls
			{ID: "perimeter_0", Start: dungeon.LocalPosition{X: 0, Y: 0, Z: 0}, End: dungeon.LocalPosition{X: 9, Y: -9, Z: 0}, BlocksMovement: true},
			{ID: "perimeter_1", Start: dungeon.LocalPosition{X: 9, Y: -9, Z: 0}, End: dungeon.LocalPosition{X: 9, Y: -18, Z: 9}, BlocksMovement: true},
			{ID: "perimeter_2", Start: dungeon.LocalPosition{X: 9, Y: -18, Z: 9}, End: dungeon.LocalPosition{X: 0, Y: -9, Z: 9}, BlocksMovement: true},
			{ID: "perimeter_3", Start: dungeon.LocalPosition{X: 0, Y: -9, Z: 9}, End: dungeon.LocalPosition{X: 0, Y: 0, Z: 0}, BlocksMovement: true},
			// Center pillar (3x3 block at positions around center)
			{ID: "pillar_h", Start: dungeon.LocalPosition{X: 4, Y: -9, Z: 5}, End: dungeon.LocalPosition{X: 6, Y: -11, Z: 5}, BlocksMovement: true},
			{ID: "pillar_v", Start: dungeon.LocalPosition{X: 5, Y: -9, Z: 4}, End: dungeon.LocalPosition{X: 5, Y: -11, Z: 6}, BlocksMovement: true},
		},
	}

	roomData := &spatial.RoomData{
		CubeEntities: make(map[string]spatial.EntityCubePlacement),
	}

	// Find safe fallback - should NOT be on the pillar
	pos := findSafeFallbackPosition(room, roomData)

	// Build wall position set
	wallPositions := buildWallSet(room.Walls)

	// Verify the position is not on a wall
	key := fmt.Sprintf("%d,%d,%d", pos.X, pos.Y, pos.Z)
	s.False(wallPositions[key], "Fallback position %s should not be on a wall", key)

	// Verify it's within playable bounds
	s.GreaterOrEqual(pos.X, 1, "X should be >= 1 (inside perimeter)")
	s.Less(pos.X, room.Shape.Width-1, "X should be < width-1 (inside perimeter)")
	s.GreaterOrEqual(pos.Z, 1, "Z should be >= 1 (inside perimeter)")
	s.Less(pos.Z, room.Shape.Height-1, "Z should be < height-1 (inside perimeter)")
}

func (s *SafePlacementTestSuite) TestFindSafeFallbackPosition_AvoidsOccupied() {
	room := &dungeon.Room{
		ID: "test-room",
		Shape: &dungeon.Shape{
			Width:  6,
			Height: 6,
		},
		Walls: []dungeon.WallSegment{
			// Just perimeter walls
			{ID: "perimeter_0", Start: dungeon.LocalPosition{X: 0, Y: 0, Z: 0}, End: dungeon.LocalPosition{X: 5, Y: -5, Z: 0}, BlocksMovement: true},
			{ID: "perimeter_1", Start: dungeon.LocalPosition{X: 5, Y: -5, Z: 0}, End: dungeon.LocalPosition{X: 5, Y: -10, Z: 5}, BlocksMovement: true},
			{ID: "perimeter_2", Start: dungeon.LocalPosition{X: 5, Y: -10, Z: 5}, End: dungeon.LocalPosition{X: 0, Y: -5, Z: 5}, BlocksMovement: true},
			{ID: "perimeter_3", Start: dungeon.LocalPosition{X: 0, Y: -5, Z: 5}, End: dungeon.LocalPosition{X: 0, Y: 0, Z: 0}, BlocksMovement: true},
		},
	}

	// Center position is occupied
	centerX, centerZ := 3, 3
	centerY := -centerX - centerZ

	roomData := &spatial.RoomData{
		CubeEntities: map[string]spatial.EntityCubePlacement{
			"existing-entity": {
				EntityID: "existing-entity",
				CubePosition: spatial.CubeCoordinate{
					X: centerX,
					Y: centerY,
					Z: centerZ,
				},
			},
		},
	}

	pos := findSafeFallbackPosition(room, roomData)

	// Should not be at the center (occupied)
	s.False(pos.X == centerX && pos.Z == centerZ, "Should not place at occupied center position")

	// Should still be in valid bounds
	s.GreaterOrEqual(pos.X, 1)
	s.Less(pos.X, room.Shape.Width-1)
	s.GreaterOrEqual(pos.Z, 1)
	s.Less(pos.Z, room.Shape.Height-1)
}

func (s *SafePlacementTestSuite) TestFindSafeFallbackPosition_EmptyRoom() {
	room := &dungeon.Room{
		ID: "test-room",
		Shape: &dungeon.Shape{
			Width:  8,
			Height: 8,
		},
		Walls: []dungeon.WallSegment{
			// Just perimeter walls
			{ID: "perimeter_0", Start: dungeon.LocalPosition{X: 0, Y: 0, Z: 0}, End: dungeon.LocalPosition{X: 7, Y: -7, Z: 0}, BlocksMovement: true},
		},
	}

	roomData := &spatial.RoomData{
		CubeEntities: make(map[string]spatial.EntityCubePlacement),
	}

	pos := findSafeFallbackPosition(room, roomData)

	// Center should be returned for empty room (4, -8, 4)
	s.Equal(4, pos.X, "X should be center column")
	s.Equal(4, pos.Z, "Z should be center row")
	s.Equal(-8, pos.Y, "Y should be -X-Z")
}

// buildWallSet creates a set of all wall positions for testing
func buildWallSet(walls []dungeon.WallSegment) map[string]bool {
	positions := make(map[string]bool)
	for _, wall := range walls {
		if !wall.BlocksMovement {
			continue
		}
		wallPos := getWallPositionsForFallback(wall)
		for _, pos := range wallPos {
			key := fmt.Sprintf("%d,%d,%d", pos.X, pos.Y, pos.Z)
			positions[key] = true
		}
	}
	return positions
}
