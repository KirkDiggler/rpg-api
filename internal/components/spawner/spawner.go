package spawner

import (
	"fmt"
)

// Spawner places entities in valid positions within a room.
type Spawner interface {
	Spawn(input *SpawnInput) (*SpawnOutput, error)
}

// DefaultSpawner implements spatial-aware entity placement.
type DefaultSpawner struct{}

// NewSpawner creates a new DefaultSpawner.
func NewSpawner() *DefaultSpawner {
	return &DefaultSpawner{}
}

// Spawn places entities in valid positions, avoiding walls and other entities.
func (s *DefaultSpawner) Spawn(input *SpawnInput) (*SpawnOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Build set of blocked positions (walls + already occupied)
	blocked := s.buildBlockedSet(input)

	// Filter zone positions to only valid (unblocked) ones
	availableByZone := s.filterAvailablePositions(input.Zones, blocked)

	// Track positions we assign during this spawn call
	assigned := make(map[string]bool)

	placements := make([]EntityPlacement, 0, len(input.Entities))
	var errors []PlacementError

	// Place each entity
	for _, entity := range input.Entities {
		placement, err := s.placeEntity(entity, availableByZone, assigned)
		if err != nil {
			errors = append(errors, PlacementError{
				EntityID: entity.ID,
				Reason:   err.Error(),
			})
			continue
		}

		placements = append(placements, *placement)

		// Mark this position as assigned so subsequent entities don't overlap
		key := posKey(placement.Position)
		assigned[key] = true
	}

	return &SpawnOutput{
		Placements: placements,
		Errors:     errors,
	}, nil
}

// buildBlockedSet creates a set of positions that are blocked by walls or occupied entities.
func (s *DefaultSpawner) buildBlockedSet(input *SpawnInput) map[string]bool {
	blocked := make(map[string]bool)

	// Add occupied positions
	for _, pos := range input.Occupied {
		blocked[posKey(pos)] = true
	}

	// Add wall positions
	for _, wall := range input.Walls {
		if !wall.BlocksMovement {
			continue
		}
		// Add all positions along the wall segment
		wallPositions := s.getWallPositions(wall)
		for _, pos := range wallPositions {
			blocked[posKey(pos)] = true
		}
	}

	// Note: We don't auto-block perimeter positions here because:
	// 1. The room already has perimeter walls in input.Walls
	// 2. The dungeon generator ensures spawn zones contain only valid interior positions
	// This keeps the spawner flexible and trusts the caller to provide valid zone bounds.

	return blocked
}

// getWallPositions returns all grid positions covered by a wall segment.
func (s *DefaultSpawner) getWallPositions(wall WallSegment) []CubePosition {
	var positions []CubePosition

	// For now, treat walls as line segments and include start and end
	// A more sophisticated approach would interpolate between them
	positions = append(positions, wall.Start, wall.End)

	// If start and end are adjacent, we're done
	// If they span multiple cells, interpolate
	dx := abs(wall.End.X - wall.Start.X)
	dz := abs(wall.End.Z - wall.Start.Z)

	if dx > 1 || dz > 1 {
		// Interpolate positions between start and end
		steps := maxInt(dx, dz)
		for i := 1; i < steps; i++ {
			t := float64(i) / float64(steps)
			x := wall.Start.X + int(float64(wall.End.X-wall.Start.X)*t)
			z := wall.Start.Z + int(float64(wall.End.Z-wall.Start.Z)*t)
			y := -x - z // Maintain cube coordinate invariant
			positions = append(positions, CubePosition{X: x, Y: y, Z: z})
		}
	}

	return positions
}

// zoneInfo stores both positions and type for a zone.
type zoneInfo struct {
	positions []CubePosition
	zoneType  ZoneType
}

// filterAvailablePositions returns zone positions that are not blocked.
func (s *DefaultSpawner) filterAvailablePositions(zones []SpawnZone, blocked map[string]bool) map[string]zoneInfo {
	available := make(map[string]zoneInfo)

	for _, zone := range zones {
		var validPositions []CubePosition
		for _, pos := range zone.Bounds {
			if !blocked[posKey(pos)] {
				validPositions = append(validPositions, pos)
			}
		}
		if len(validPositions) > 0 {
			available[zone.ID] = zoneInfo{
				positions: validPositions,
				zoneType:  zone.Type,
			}
		}
	}

	return available
}

// placeEntity finds a valid position for an entity.
func (s *DefaultSpawner) placeEntity(
	entity EntityToSpawn,
	availableByZone map[string]zoneInfo,
	assigned map[string]bool,
) (*EntityPlacement, error) {
	// First pass: find zones matching the entity's preferred zone type
	preferredZoneIDs := make([]string, 0)
	fallbackZoneIDs := make([]string, 0)

	for zoneID, info := range availableByZone {
		if len(info.positions) == 0 {
			continue
		}

		// Check if any positions are still available (not assigned)
		hasAvailable := false
		for _, pos := range info.positions {
			if !assigned[posKey(pos)] {
				hasAvailable = true
				break
			}
		}
		if !hasAvailable {
			continue
		}

		// Match zone type to entity's preferred zone
		if info.zoneType == entity.PreferredZone {
			preferredZoneIDs = append(preferredZoneIDs, zoneID)
		} else {
			fallbackZoneIDs = append(fallbackZoneIDs, zoneID)
		}
	}

	// Try preferred zones first
	for _, zoneID := range preferredZoneIDs {
		info := availableByZone[zoneID]
		for _, pos := range info.positions {
			if !assigned[posKey(pos)] {
				return &EntityPlacement{
					EntityID: entity.ID,
					Position: pos,
					ZoneID:   zoneID,
				}, nil
			}
		}
	}

	// Fall back to other zones only if no preferred zones available
	for _, zoneID := range fallbackZoneIDs {
		info := availableByZone[zoneID]
		for _, pos := range info.positions {
			if !assigned[posKey(pos)] {
				return &EntityPlacement{
					EntityID: entity.ID,
					Position: pos,
					ZoneID:   zoneID,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no valid spawn position available")
}

// posKey creates a map key from a cube position.
func posKey(pos CubePosition) string {
	return fmt.Sprintf("%d,%d,%d", pos.X, pos.Y, pos.Z)
}

// offsetToCube converts offset coordinates (col, row) to cube coordinates.
// Uses odd-r offset layout where odd rows are shifted right.
func offsetToCube(col, row int) CubePosition {
	x := col - (row-(row&1))/2
	z := row
	y := -x - z
	return CubePosition{X: x, Y: y, Z: z}
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// maxInt returns the larger of two integers.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
