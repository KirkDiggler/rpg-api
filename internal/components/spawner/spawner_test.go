package spawner

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type SpawnerTestSuite struct {
	suite.Suite
	spawner *DefaultSpawner
}

func TestSpawnerTestSuite(t *testing.T) {
	suite.Run(t, new(SpawnerTestSuite))
}

func (s *SpawnerTestSuite) SetupTest() {
	s.spawner = NewSpawner()
}

func (s *SpawnerTestSuite) TestSpawn_NilInput() {
	output, err := s.spawner.Spawn(nil)

	s.Assert().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *SpawnerTestSuite) TestSpawn_SingleEntityInZone() {
	// Create a 10x10 room with a spawn zone in the center
	input := &SpawnInput{
		Width:  10,
		Height: 10,
		Zones: []SpawnZone{
			{
				ID:   "monster_spawn_1",
				Type: ZoneTypeMonsterSpawn,
				Bounds: []CubePosition{
					offsetToCube(5, 5), // Center position
				},
				Capacity: 1,
			},
		},
		Entities: []EntityToSpawn{
			{
				ID:            "monster-1",
				Type:          EntityTypeMonster,
				PreferredZone: ZoneTypeMonsterSpawn,
				Size:          1,
			},
		},
	}

	output, err := s.spawner.Spawn(input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 1)
	s.Assert().Len(output.Errors, 0)
	s.Assert().Equal("monster-1", output.Placements[0].EntityID)
	s.Assert().Equal("monster_spawn_1", output.Placements[0].ZoneID)
}

func (s *SpawnerTestSuite) TestSpawn_AvoidsOccupiedPositions() {
	// Spawn zone has 2 positions, but one is occupied
	zone1Pos := offsetToCube(5, 5)
	zone2Pos := offsetToCube(6, 5)

	input := &SpawnInput{
		Width:  10,
		Height: 10,
		Occupied: []CubePosition{
			zone1Pos, // First position is occupied
		},
		Zones: []SpawnZone{
			{
				ID:   "monster_spawn_1",
				Type: ZoneTypeMonsterSpawn,
				Bounds: []CubePosition{
					zone1Pos,
					zone2Pos,
				},
				Capacity: 2,
			},
		},
		Entities: []EntityToSpawn{
			{
				ID:            "monster-1",
				Type:          EntityTypeMonster,
				PreferredZone: ZoneTypeMonsterSpawn,
				Size:          1,
			},
		},
	}

	output, err := s.spawner.Spawn(input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 1)
	s.Assert().Len(output.Errors, 0)

	// Should have placed at the second position since first is occupied
	s.Assert().Equal(zone2Pos, output.Placements[0].Position)
}

func (s *SpawnerTestSuite) TestSpawn_AvoidsPerimeterWalls() {
	// Spawn zone includes positions on the perimeter - but perimeter is blocked via walls
	perimeterPos := offsetToCube(0, 5) // Left wall position
	interiorPos := offsetToCube(5, 5)  // Interior

	input := &SpawnInput{
		Width:  10,
		Height: 10,
		// Perimeter is blocked via wall segments (as dungeon generator provides)
		Walls: []WallSegment{
			{
				Start:          perimeterPos,
				End:            perimeterPos,
				BlocksMovement: true,
			},
		},
		Zones: []SpawnZone{
			{
				ID:   "monster_spawn_1",
				Type: ZoneTypeMonsterSpawn,
				Bounds: []CubePosition{
					perimeterPos,
					interiorPos,
				},
				Capacity: 2,
			},
		},
		Entities: []EntityToSpawn{
			{
				ID:            "monster-1",
				Type:          EntityTypeMonster,
				PreferredZone: ZoneTypeMonsterSpawn,
				Size:          1,
			},
		},
	}

	output, err := s.spawner.Spawn(input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 1)

	// Should have placed at interior position, not on perimeter wall
	s.Assert().Equal(interiorPos, output.Placements[0].Position)
}

func (s *SpawnerTestSuite) TestSpawn_AvoidsInternalWalls() {
	interiorPos := offsetToCube(5, 5)
	wallPos := offsetToCube(6, 5)
	safePos := offsetToCube(7, 5)

	input := &SpawnInput{
		Width:  10,
		Height: 10,
		Walls: []WallSegment{
			{
				Start:          wallPos,
				End:            wallPos,
				BlocksMovement: true,
			},
		},
		Zones: []SpawnZone{
			{
				ID:   "monster_spawn_1",
				Type: ZoneTypeMonsterSpawn,
				Bounds: []CubePosition{
					wallPos, // Blocked by wall
					safePos, // Safe
				},
				Capacity: 2,
			},
		},
		Entities: []EntityToSpawn{
			{
				ID:            "monster-1",
				Type:          EntityTypeMonster,
				PreferredZone: ZoneTypeMonsterSpawn,
				Size:          1,
			},
		},
	}

	output, err := s.spawner.Spawn(input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 1)

	// Should have placed at safe position, not on wall
	s.Assert().Equal(safePos, output.Placements[0].Position)
	_ = interiorPos // Not used in this test
}

func (s *SpawnerTestSuite) TestSpawn_MultipleEntitiesDontOverlap() {
	pos1 := offsetToCube(5, 5)
	pos2 := offsetToCube(6, 5)
	pos3 := offsetToCube(7, 5)

	input := &SpawnInput{
		Width:  10,
		Height: 10,
		Zones: []SpawnZone{
			{
				ID:   "monster_spawn_1",
				Type: ZoneTypeMonsterSpawn,
				Bounds: []CubePosition{
					pos1, pos2, pos3,
				},
				Capacity: 3,
			},
		},
		Entities: []EntityToSpawn{
			{ID: "monster-1", Type: EntityTypeMonster, PreferredZone: ZoneTypeMonsterSpawn, Size: 1},
			{ID: "monster-2", Type: EntityTypeMonster, PreferredZone: ZoneTypeMonsterSpawn, Size: 1},
			{ID: "monster-3", Type: EntityTypeMonster, PreferredZone: ZoneTypeMonsterSpawn, Size: 1},
		},
	}

	output, err := s.spawner.Spawn(input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 3)
	s.Assert().Len(output.Errors, 0)

	// Verify all placements have unique positions
	positions := make(map[string]bool)
	for _, placement := range output.Placements {
		key := posKey(placement.Position)
		s.Assert().False(positions[key], "Duplicate position found: %v", placement.Position)
		positions[key] = true
	}
}

func (s *SpawnerTestSuite) TestSpawn_NoValidPositions() {
	// Zone has positions but all are blocked by walls
	pos1 := offsetToCube(5, 5)
	pos2 := offsetToCube(6, 5)

	input := &SpawnInput{
		Width:  10,
		Height: 10,
		// Both zone positions are blocked by walls
		Walls: []WallSegment{
			{Start: pos1, End: pos1, BlocksMovement: true},
			{Start: pos2, End: pos2, BlocksMovement: true},
		},
		Zones: []SpawnZone{
			{
				ID:   "monster_spawn_1",
				Type: ZoneTypeMonsterSpawn,
				Bounds: []CubePosition{
					pos1,
					pos2,
				},
				Capacity: 2,
			},
		},
		Entities: []EntityToSpawn{
			{
				ID:            "monster-1",
				Type:          EntityTypeMonster,
				PreferredZone: ZoneTypeMonsterSpawn,
				Size:          1,
			},
		},
	}

	output, err := s.spawner.Spawn(input)

	s.Require().NoError(err) // Spawn itself doesn't fail, but reports errors
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 0)
	s.Assert().Len(output.Errors, 1)
	s.Assert().Equal("monster-1", output.Errors[0].EntityID)
	s.Assert().Contains(output.Errors[0].Reason, "no valid spawn position")
}

func (s *SpawnerTestSuite) TestSpawn_MoreEntitiesThanPositions() {
	// Zone has 2 positions but we try to spawn 3 entities
	pos1 := offsetToCube(5, 5)
	pos2 := offsetToCube(6, 5)

	input := &SpawnInput{
		Width:  10,
		Height: 10,
		Zones: []SpawnZone{
			{
				ID:   "monster_spawn_1",
				Type: ZoneTypeMonsterSpawn,
				Bounds: []CubePosition{
					pos1, pos2,
				},
				Capacity: 2,
			},
		},
		Entities: []EntityToSpawn{
			{ID: "monster-1", Type: EntityTypeMonster, PreferredZone: ZoneTypeMonsterSpawn, Size: 1},
			{ID: "monster-2", Type: EntityTypeMonster, PreferredZone: ZoneTypeMonsterSpawn, Size: 1},
			{ID: "monster-3", Type: EntityTypeMonster, PreferredZone: ZoneTypeMonsterSpawn, Size: 1},
		},
	}

	output, err := s.spawner.Spawn(input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 2)
	s.Assert().Len(output.Errors, 1)
	s.Assert().Equal("monster-3", output.Errors[0].EntityID)
}

func (s *SpawnerTestSuite) TestSpawn_EmptyEntities() {
	input := &SpawnInput{
		Width:    10,
		Height:   10,
		Entities: []EntityToSpawn{},
	}

	output, err := s.spawner.Spawn(input)

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 0)
	s.Assert().Len(output.Errors, 0)
}

func (s *SpawnerTestSuite) TestSpawn_NonBlockingWallIgnored() {
	wallPos := offsetToCube(5, 5)

	input := &SpawnInput{
		Width:  10,
		Height: 10,
		Walls: []WallSegment{
			{
				Start:          wallPos,
				End:            wallPos,
				BlocksMovement: false, // Doesn't block movement
			},
		},
		Zones: []SpawnZone{
			{
				ID:     "monster_spawn_1",
				Type:   ZoneTypeMonsterSpawn,
				Bounds: []CubePosition{wallPos},
			},
		},
		Entities: []EntityToSpawn{
			{ID: "monster-1", Type: EntityTypeMonster, PreferredZone: ZoneTypeMonsterSpawn, Size: 1},
		},
	}

	output, err := s.spawner.Spawn(input)

	s.Require().NoError(err)
	s.Assert().Len(output.Placements, 1)
	// Entity can spawn on non-blocking wall position
	s.Assert().Equal(wallPos, output.Placements[0].Position)
}

func (s *SpawnerTestSuite) TestOffsetToCube_Conversion() {
	// Test that offset to cube conversion maintains invariant x + y + z = 0
	testCases := []struct {
		col, row int
	}{
		{0, 0},
		{5, 5},
		{10, 10},
		{3, 7},
	}

	for _, tc := range testCases {
		pos := offsetToCube(tc.col, tc.row)
		sum := pos.X + pos.Y + pos.Z
		s.Assert().Equal(0, sum, "Cube coordinate invariant violated for col=%d, row=%d: x=%d, y=%d, z=%d",
			tc.col, tc.row, pos.X, pos.Y, pos.Z)
	}
}
