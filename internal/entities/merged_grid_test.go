package entities

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

type MergedGridTestSuite struct {
	suite.Suite
	grid *MergedGrid
}

func (s *MergedGridTestSuite) SetupTest() {
	s.grid = NewMergedGrid()
}

func TestMergedGridSuite(t *testing.T) {
	suite.Run(t, new(MergedGridTestSuite))
}

// CubeBounds tests

func (s *MergedGridTestSuite) TestCubeBoundsContains() {
	bounds := CubeBounds{
		MinX: 0, MinY: -5, MinZ: 0,
		MaxX: 5, MaxY: 0, MaxZ: 5,
	}

	s.Run("point inside bounds returns true", func() {
		coord := spatial.CubeCoordinate{X: 2, Y: -2, Z: 0} // x+y+z = 0
		s.True(bounds.Contains(coord))
	})

	s.Run("point at boundary returns true", func() {
		coord := spatial.CubeCoordinate{X: 0, Y: 0, Z: 0}
		s.True(bounds.Contains(coord))
	})

	s.Run("point outside bounds returns false", func() {
		coord := spatial.CubeCoordinate{X: 10, Y: -5, Z: -5} // x+y+z = 0
		s.False(bounds.Contains(coord))
	})
}

// NewMergedGrid tests

func (s *MergedGridTestSuite) TestNewMergedGrid() {
	s.Run("creates empty grid with initialized maps", func() {
		grid := NewMergedGrid()

		s.NotNil(grid)
		s.NotNil(grid.Entities)
		s.NotNil(grid.Blocked)
		s.NotNil(grid.RoomBounds)
		s.NotNil(grid.RoomOffsets)
		s.NotNil(grid.DoorPositions)
		s.Empty(grid.Entities)
		s.Empty(grid.Blocked)
		s.Empty(grid.RoomBounds)
		s.Empty(grid.RoomOffsets)
		s.Empty(grid.DoorPositions)
	})
}

// AddRoom tests

func (s *MergedGridTestSuite) TestAddRoom() {
	s.Run("adds room with bounds and offset", func() {
		bounds := CubeBounds{
			MinX: 0, MinY: -10, MinZ: 0,
			MaxX: 10, MaxY: 0, MaxZ: 10,
		}
		offset := spatial.CubeCoordinate{X: 0, Y: 0, Z: 0}

		s.grid.AddRoom("room-1", bounds, offset)

		s.Contains(s.grid.RoomBounds, "room-1")
		s.Contains(s.grid.RoomOffsets, "room-1")
		s.Equal(bounds, s.grid.RoomBounds["room-1"])
		s.Equal(offset, s.grid.RoomOffsets["room-1"])
	})

	s.Run("adds second room with different offset", func() {
		bounds1 := CubeBounds{MinX: 0, MinY: -10, MinZ: 0, MaxX: 10, MaxY: 0, MaxZ: 10}
		bounds2 := CubeBounds{MinX: 11, MinY: -10, MinZ: 0, MaxX: 21, MaxY: 0, MaxZ: 10}
		offset1 := spatial.CubeCoordinate{X: 0, Y: 0, Z: 0}
		offset2 := spatial.CubeCoordinate{X: 11, Y: 0, Z: -11}

		s.grid.AddRoom("room-1", bounds1, offset1)
		s.grid.AddRoom("room-2", bounds2, offset2)

		s.Len(s.grid.RoomBounds, 2)
		s.Len(s.grid.RoomOffsets, 2)
	})
}

// LocalToMerged/MergedToLocal transform tests

func (s *MergedGridTestSuite) TestLocalToMerged() {
	s.Run("transforms local coord with zero offset", func() {
		// Room from (0,0,0) to (10,-20,10) in local space
		// Y bounds: if X=0..10 and Z=0..10, then Y = -X-Z ranges from 0 to -20
		bounds := CubeBounds{MinX: 0, MinY: -20, MinZ: 0, MaxX: 10, MaxY: 0, MaxZ: 10}
		offset := spatial.CubeCoordinate{X: 0, Y: 0, Z: 0}
		s.grid.AddRoom("room-1", bounds, offset)

		local := spatial.CubeCoordinate{X: 5, Y: -8, Z: 3} // 5 + (-8) + 3 = 0 ✓
		merged := s.grid.LocalToMerged("room-1", local)

		s.Equal(local, merged)
		s.True(merged.IsValid(), "merged coord should satisfy x+y+z=0")
	})

	s.Run("transforms local coord with non-zero offset", func() {
		bounds := CubeBounds{MinX: 0, MinY: -20, MinZ: 0, MaxX: 10, MaxY: 0, MaxZ: 10}
		offset := spatial.CubeCoordinate{X: 20, Y: -10, Z: -10}
		s.grid.AddRoom("room-1", bounds, offset)

		local := spatial.CubeCoordinate{X: 5, Y: -8, Z: 3} // Valid local coord
		merged := s.grid.LocalToMerged("room-1", local)

		// (5,-8,3) + (20,-10,-10) = (25,-18,-7)
		expected := spatial.CubeCoordinate{X: 25, Y: -18, Z: -7}
		s.Equal(expected, merged)
		s.True(merged.IsValid(), "merged coord should satisfy x+y+z=0")
	})

	s.Run("returns zero for unknown room", func() {
		local := spatial.CubeCoordinate{X: 5, Y: -8, Z: 3}
		merged := s.grid.LocalToMerged("unknown-room", local)

		s.Equal(spatial.CubeCoordinate{}, merged)
	})
}

func (s *MergedGridTestSuite) TestMergedToLocal() {
	s.Run("finds room and transforms coord back to local", func() {
		// Room bounds: X=0..10, Z=0..10, Y=-20..0 (derived from x+y+z=0)
		bounds := CubeBounds{MinX: 0, MinY: -20, MinZ: 0, MaxX: 10, MaxY: 0, MaxZ: 10}
		offset := spatial.CubeCoordinate{X: 20, Y: -10, Z: -10}
		s.grid.AddRoom("room-1", bounds, offset)

		// Local (5,-8,3) + offset (20,-10,-10) = merged (25,-18,-7)
		merged := spatial.CubeCoordinate{X: 25, Y: -18, Z: -7}
		roomID, local := s.grid.MergedToLocal(merged)

		s.Equal("room-1", roomID)
		expected := spatial.CubeCoordinate{X: 5, Y: -8, Z: 3}
		s.Equal(expected, local)
		s.True(local.IsValid(), "local coord should satisfy x+y+z=0")
	})

	s.Run("returns empty for coord not in any room", func() {
		bounds := CubeBounds{MinX: 0, MinY: -20, MinZ: 0, MaxX: 10, MaxY: 0, MaxZ: 10}
		offset := spatial.CubeCoordinate{X: 0, Y: 0, Z: 0}
		s.grid.AddRoom("room-1", bounds, offset)

		// Coord far outside room bounds
		merged := spatial.CubeCoordinate{X: 100, Y: -50, Z: -50}
		roomID, local := s.grid.MergedToLocal(merged)

		s.Empty(roomID)
		s.Equal(spatial.CubeCoordinate{}, local)
	})
}

// Entity operation tests

func (s *MergedGridTestSuite) TestAddEntity() {
	s.Run("adds entity and marks position as blocked", func() {
		placement := spatial.EntityCubePlacement{
			EntityID:     "hero-1",
			EntityType:   "character",
			CubePosition: spatial.CubeCoordinate{X: 5, Y: -3, Z: -2},
			Size:         1,
		}

		s.grid.AddEntity(placement)

		s.Contains(s.grid.Entities, "hero-1")
		s.Equal(placement, s.grid.Entities["hero-1"])
		s.True(s.grid.IsBlocked(placement.CubePosition))
	})
}

func (s *MergedGridTestSuite) TestRemoveEntity() {
	s.Run("removes entity and unblocks position", func() {
		pos := spatial.CubeCoordinate{X: 5, Y: -3, Z: -2}
		placement := spatial.EntityCubePlacement{
			EntityID:     "hero-1",
			EntityType:   "character",
			CubePosition: pos,
			Size:         1,
		}
		s.grid.AddEntity(placement)

		s.grid.RemoveEntity("hero-1")

		s.NotContains(s.grid.Entities, "hero-1")
		s.False(s.grid.IsBlocked(pos))
	})

	s.Run("removing nonexistent entity is safe", func() {
		s.NotPanics(func() {
			s.grid.RemoveEntity("nonexistent")
		})
	})
}

func (s *MergedGridTestSuite) TestMoveEntity() {
	s.Run("moves entity to new position", func() {
		oldPos := spatial.CubeCoordinate{X: 5, Y: -3, Z: -2}
		newPos := spatial.CubeCoordinate{X: 6, Y: -4, Z: -2}
		placement := spatial.EntityCubePlacement{
			EntityID:     "hero-1",
			EntityType:   "character",
			CubePosition: oldPos,
			Size:         1,
		}
		s.grid.AddEntity(placement)

		success := s.grid.MoveEntity("hero-1", newPos)

		s.True(success)
		s.Equal(newPos, s.grid.Entities["hero-1"].CubePosition)
		s.False(s.grid.IsBlocked(oldPos))
		s.True(s.grid.IsBlocked(newPos))
	})

	s.Run("fails to move to blocked position", func() {
		pos1 := spatial.CubeCoordinate{X: 5, Y: -3, Z: -2}
		pos2 := spatial.CubeCoordinate{X: 6, Y: -4, Z: -2}
		s.grid.AddEntity(spatial.EntityCubePlacement{
			EntityID:     "hero-1",
			EntityType:   "character",
			CubePosition: pos1,
			Size:         1,
		})
		s.grid.AddEntity(spatial.EntityCubePlacement{
			EntityID:     "hero-2",
			EntityType:   "character",
			CubePosition: pos2,
			Size:         1,
		})

		success := s.grid.MoveEntity("hero-1", pos2)

		s.False(success)
		s.Equal(pos1, s.grid.Entities["hero-1"].CubePosition)
	})

	s.Run("fails to move nonexistent entity", func() {
		newPos := spatial.CubeCoordinate{X: 6, Y: -4, Z: -2}
		success := s.grid.MoveEntity("nonexistent", newPos)
		s.False(success)
	})
}

func (s *MergedGridTestSuite) TestGetEntityPosition() {
	s.Run("returns position for existing entity", func() {
		pos := spatial.CubeCoordinate{X: 5, Y: -3, Z: -2}
		s.grid.AddEntity(spatial.EntityCubePlacement{
			EntityID:     "hero-1",
			EntityType:   "character",
			CubePosition: pos,
			Size:         1,
		})

		result, found := s.grid.GetEntityPosition("hero-1")

		s.True(found)
		s.Equal(pos, result)
	})

	s.Run("returns false for nonexistent entity", func() {
		_, found := s.grid.GetEntityPosition("nonexistent")
		s.False(found)
	})
}

// Door operation tests

func (s *MergedGridTestSuite) TestSetDoorPosition() {
	s.Run("records door position", func() {
		pos := spatial.CubeCoordinate{X: 10, Y: -5, Z: -5}
		s.grid.SetDoorPosition("door-1", pos)

		s.Contains(s.grid.DoorPositions, "door-1")
		s.Equal(pos, s.grid.DoorPositions["door-1"])
	})
}

func (s *MergedGridTestSuite) TestOpenCloseDoor() {
	s.Run("close door blocks position", func() {
		pos := spatial.CubeCoordinate{X: 10, Y: -5, Z: -5}
		s.grid.SetDoorPosition("door-1", pos)

		s.grid.CloseDoor("door-1")

		s.True(s.grid.IsBlocked(pos))
	})

	s.Run("open door unblocks position", func() {
		pos := spatial.CubeCoordinate{X: 10, Y: -5, Z: -5}
		s.grid.SetDoorPosition("door-1", pos)
		s.grid.CloseDoor("door-1")

		s.grid.OpenDoor("door-1")

		s.False(s.grid.IsBlocked(pos))
	})

	s.Run("open/close unknown door is safe", func() {
		s.NotPanics(func() {
			s.grid.OpenDoor("unknown")
			s.grid.CloseDoor("unknown")
		})
	})
}

func (s *MergedGridTestSuite) TestIsBlocked() {
	s.Run("empty position is not blocked", func() {
		pos := spatial.CubeCoordinate{X: 5, Y: -3, Z: -2}
		s.False(s.grid.IsBlocked(pos))
	})

	s.Run("position with entity is blocked", func() {
		pos := spatial.CubeCoordinate{X: 5, Y: -3, Z: -2}
		s.grid.AddEntity(spatial.EntityCubePlacement{
			EntityID:     "hero-1",
			EntityType:   "character",
			CubePosition: pos,
			Size:         1,
		})
		s.True(s.grid.IsBlocked(pos))
	})

	s.Run("closed door position is blocked", func() {
		pos := spatial.CubeCoordinate{X: 10, Y: -5, Z: -5}
		s.grid.SetDoorPosition("door-1", pos)
		s.grid.CloseDoor("door-1")
		s.True(s.grid.IsBlocked(pos))
	})
}

// CalculateRoomOffset tests

func (s *MergedGridTestSuite) TestCalculateRoomOffset() {
	// In pointy-top hex orientation, the 6 directions and their cube coordinate deltas:
	// "e"  (east):      +1, -1,  0
	// "ne" (northeast): +1,  0, -1
	// "nw" (northwest):  0, +1, -1
	// "w"  (west):      -1, +1,  0
	// "sw" (southwest): -1,  0, +1
	// "se" (southeast):  0, -1, +1

	s.Run("east direction places room to the east", func() {
		doorInExisting := spatial.CubeCoordinate{X: 10, Y: -5, Z: -5}
		doorInNew := spatial.CubeCoordinate{X: 0, Y: 0, Z: 0}

		offset := CalculateRoomOffset(doorInExisting, doorInNew, "e")

		// New room's door (0,0,0) + offset should be adjacent (east) to existing door
		// East neighbor of (10,-5,-5) is (11,-6,-5)
		// So offset should be (11,-6,-5) - (0,0,0) = (11,-6,-5)
		expected := spatial.CubeCoordinate{X: 11, Y: -6, Z: -5}
		s.Equal(expected, offset)
		s.True(offset.IsValid(), "offset should satisfy x+y+z=0")
	})

	s.Run("west direction places room to the west", func() {
		doorInExisting := spatial.CubeCoordinate{X: 10, Y: -5, Z: -5}
		doorInNew := spatial.CubeCoordinate{X: 5, Y: -3, Z: -2}

		offset := CalculateRoomOffset(doorInExisting, doorInNew, "w")

		// West neighbor of (10,-5,-5) is (9,-4,-5)
		// New door at local (5,-3,-2) should end up at (9,-4,-5)
		// So offset = (9,-4,-5) - (5,-3,-2) = (4,-1,-3)
		expected := spatial.CubeCoordinate{X: 4, Y: -1, Z: -3}
		s.Equal(expected, offset)
		s.True(offset.IsValid(), "offset should satisfy x+y+z=0")
	})

	s.Run("all six directions produce valid offsets", func() {
		doorInExisting := spatial.CubeCoordinate{X: 5, Y: -2, Z: -3}
		doorInNew := spatial.CubeCoordinate{X: 0, Y: 0, Z: 0}
		directions := []string{"e", "ne", "nw", "w", "sw", "se"}

		for _, dir := range directions {
			offset := CalculateRoomOffset(doorInExisting, doorInNew, dir)
			s.True(offset.IsValid(), "offset for direction %s should satisfy x+y+z=0", dir)
		}
	})

	s.Run("unknown direction returns zero offset", func() {
		doorInExisting := spatial.CubeCoordinate{X: 10, Y: -5, Z: -5}
		doorInNew := spatial.CubeCoordinate{X: 0, Y: 0, Z: 0}

		offset := CalculateRoomOffset(doorInExisting, doorInNew, "invalid")

		s.Equal(spatial.CubeCoordinate{}, offset)
	})
}

// Integration tests

func (s *MergedGridTestSuite) TestRoundTripTransform() {
	s.Run("local -> merged -> local preserves coordinate", func() {
		// Room bounds: X=0..20, Z=0..20, Y=-40..0 (Y = -X - Z)
		bounds := CubeBounds{MinX: 0, MinY: -40, MinZ: 0, MaxX: 20, MaxY: 0, MaxZ: 20}
		offset := spatial.CubeCoordinate{X: 100, Y: -50, Z: -50}
		s.grid.AddRoom("room-1", bounds, offset)

		// Local coord within bounds: X=5, Z=8, Y=-13 (5+8=13, Y=-13)
		original := spatial.CubeCoordinate{X: 5, Y: -13, Z: 8}
		s.True(original.IsValid())
		merged := s.grid.LocalToMerged("room-1", original)
		roomID, local := s.grid.MergedToLocal(merged)

		s.Equal("room-1", roomID)
		s.Equal(original, local)
	})
}

func (s *MergedGridTestSuite) TestCoordinateInvariant() {
	s.Run("all operations preserve x+y+z=0 invariant", func() {
		// Add room with valid offset
		// Room bounds: X=0..10, Z=0..10, Y=-20..0
		bounds := CubeBounds{MinX: 0, MinY: -20, MinZ: 0, MaxX: 10, MaxY: 0, MaxZ: 10}
		offset := spatial.CubeCoordinate{X: 20, Y: -10, Z: -10}
		s.True(offset.IsValid())
		s.grid.AddRoom("room-1", bounds, offset)

		// Transform valid local coord within bounds: X=3, Z=5, Y=-8
		local := spatial.CubeCoordinate{X: 3, Y: -8, Z: 5}
		s.True(local.IsValid())
		merged := s.grid.LocalToMerged("room-1", local)
		s.True(merged.IsValid(), "merged coord should be valid")

		// Add entity at valid position
		s.grid.AddEntity(spatial.EntityCubePlacement{
			EntityID:     "test",
			EntityType:   "character",
			CubePosition: merged,
			Size:         1,
		})

		// Move to another valid position
		newPos := spatial.CubeCoordinate{X: 24, Y: -12, Z: -12}
		s.True(newPos.IsValid())
		s.grid.MoveEntity("test", newPos)

		pos, _ := s.grid.GetEntityPosition("test")
		s.True(pos.IsValid(), "entity position should be valid")
	})
}
