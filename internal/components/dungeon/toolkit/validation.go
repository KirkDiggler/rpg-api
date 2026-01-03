package toolkit

import (
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// WallValidator checks if wall placement is valid for gameplay
type WallValidator struct{}

// NewWallValidator creates a new wall validator
func NewWallValidator() *WallValidator {
	return &WallValidator{}
}

// ValidationInput contains parameters for wall validation
type ValidationInput struct {
	Shape      *dungeon.Shape
	Walls      []dungeon.WallSegment
	SpawnZones []dungeon.Zone
}

// ValidationOutput contains the validation result
type ValidationOutput struct {
	IsValid        bool
	BlockedZones   []string // IDs of spawn zones that are blocked
	SuggestedWalls []dungeon.WallSegment
}

// Validate checks if the wall placement allows access to all spawn zones
// Returns a simplified result indicating if walls are valid
func (v *WallValidator) Validate(input *ValidationInput) *ValidationOutput {
	if input == nil || input.Shape == nil {
		return &ValidationOutput{IsValid: true}
	}

	// If no spawn zones, walls are valid
	if len(input.SpawnZones) == 0 {
		return &ValidationOutput{IsValid: true}
	}

	// If no walls, trivially valid
	if len(input.Walls) == 0 {
		return &ValidationOutput{IsValid: true}
	}

	// Check each spawn zone for accessibility
	var blockedZones []string
	for _, zone := range input.SpawnZones {
		if v.isZoneBlocked(zone, input.Walls, input.Shape) {
			blockedZones = append(blockedZones, zone.ID)
		}
	}

	if len(blockedZones) > 0 {
		// Generate fallback walls that avoid blocking zones
		suggestedWalls := v.generateSafeWalls(input)
		return &ValidationOutput{
			IsValid:        false,
			BlockedZones:   blockedZones,
			SuggestedWalls: suggestedWalls,
		}
	}

	return &ValidationOutput{IsValid: true}
}

// isZoneBlocked checks if a spawn zone is completely surrounded by walls
func (v *WallValidator) isZoneBlocked(zone dungeon.Zone, walls []dungeon.WallSegment, shape *dungeon.Shape) bool {
	if len(zone.Bounds) == 0 {
		return false
	}

	// Get zone center
	zoneCenter := v.getZoneCenter(zone)

	// Check if walls completely surround the zone
	// A simple heuristic: check if there's a clear path from zone center to room edges
	// in at least one cardinal direction
	directions := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} // N, S, W, E

	for _, dir := range directions {
		if v.hasPathToEdge(zoneCenter, dir, walls, shape) {
			return false // Found at least one clear path
		}
	}

	return true // All directions blocked
}

// getZoneCenter calculates the center point of a zone
func (v *WallValidator) getZoneCenter(zone dungeon.Zone) dungeon.Position {
	if len(zone.Bounds) == 0 {
		return dungeon.Position{}
	}

	var sumX, sumY int
	for _, pos := range zone.Bounds {
		sumX += pos.X
		sumY += pos.Y
	}

	return dungeon.Position{
		X: sumX / len(zone.Bounds),
		Y: sumY / len(zone.Bounds),
	}
}

// hasPathToEdge checks if there's a clear path from a point to the room edge
func (v *WallValidator) hasPathToEdge(start dungeon.Position, direction [2]int, walls []dungeon.WallSegment, shape *dungeon.Shape) bool {
	current := start

	// Walk in the direction until we hit an edge or wall
	for {
		// Check if we've reached the edge
		if current.X <= 0 || current.X >= shape.Width-1 ||
			current.Y <= 0 || current.Y >= shape.Height-1 {
			return true // Reached edge without hitting wall
		}

		// Check if current position intersects any wall
		if v.intersectsWall(current, walls) {
			return false // Hit a wall
		}

		// Move to next position
		current = dungeon.Position{
			X: current.X + direction[0],
			Y: current.Y + direction[1],
		}
	}
}

// intersectsWall checks if a point intersects any wall segment
func (v *WallValidator) intersectsWall(pos dungeon.Position, walls []dungeon.WallSegment) bool {
	for _, wall := range walls {
		if v.pointOnSegment(pos, wall.Start, wall.End) {
			return true
		}
	}
	return false
}

// pointOnSegment checks if a point lies on a line segment
func (v *WallValidator) pointOnSegment(p, start, end dungeon.Position) bool {
	// Check if point is within bounding box of segment
	minX := min(start.X, end.X)
	maxX := max(start.X, end.X)
	minY := min(start.Y, end.Y)
	maxY := max(start.Y, end.Y)

	if p.X < minX || p.X > maxX || p.Y < minY || p.Y > maxY {
		return false
	}

	// Check if point is on the line
	// For axis-aligned segments (which most of our walls are)
	if start.X == end.X {
		return p.X == start.X
	}
	if start.Y == end.Y {
		return p.Y == start.Y
	}

	// For diagonal segments, use cross product
	cross := (p.Y-start.Y)*(end.X-start.X) - (p.X-start.X)*(end.Y-start.Y)
	return cross == 0
}

// generateSafeWalls creates a simplified wall layout that avoids spawn zones
func (v *WallValidator) generateSafeWalls(input *ValidationInput) []dungeon.WallSegment {
	var safeWalls []dungeon.WallSegment

	for _, wall := range input.Walls {
		isBlocking := false
		for _, zone := range input.SpawnZones {
			if v.wallBlocksZone(wall, zone) {
				isBlocking = true
				break
			}
		}

		if !isBlocking {
			safeWalls = append(safeWalls, wall)
		}
	}

	return safeWalls
}

// wallBlocksZone checks if a wall segment blocks access to a spawn zone
func (v *WallValidator) wallBlocksZone(wall dungeon.WallSegment, zone dungeon.Zone) bool {
	for _, pos := range zone.Bounds {
		if v.pointOnSegment(pos, wall.Start, wall.End) {
			return true
		}
		// Also check if wall is adjacent to zone position
		if v.isAdjacent(pos, wall.Start) || v.isAdjacent(pos, wall.End) {
			// Wall is very close to spawn zone, might be problematic
			// Return true to be safe
			return true
		}
	}
	return false
}

// isAdjacent checks if two positions are adjacent (within 1 unit)
func (v *WallValidator) isAdjacent(p1, p2 dungeon.Position) bool {
	dx := p1.X - p2.X
	dy := p1.Y - p2.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx <= 1 && dy <= 1
}
