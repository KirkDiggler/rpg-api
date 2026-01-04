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

	centerX := sumX / len(zone.Bounds)
	centerY := sumY / len(zone.Bounds)

	return dungeon.Position{
		X: centerX,
		Y: centerY,
		Z: -centerX - centerY, // Maintain cube coordinate constraint: x + y + z = 0
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

// PathCrossesWall checks if a movement path from start to end crosses any wall
// Returns true if the path is blocked by a wall, false if the path is clear
func (v *WallValidator) PathCrossesWall(start, end dungeon.Position, walls []dungeon.WallSegment) bool {
	for _, wall := range walls {
		if !wall.BlocksMovement {
			continue // Skip walls that don't block movement
		}
		if v.segmentsIntersect(start, end, wall.Start, wall.End) {
			return true
		}
	}
	return false
}

// segmentsIntersect checks if two line segments intersect
// Uses the standard cross product method for 2D line segment intersection
func (v *WallValidator) segmentsIntersect(p1, p2, p3, p4 dungeon.Position) bool {
	// Calculate orientation of triplets
	d1 := v.orientation(p3, p4, p1)
	d2 := v.orientation(p3, p4, p2)
	d3 := v.orientation(p1, p2, p3)
	d4 := v.orientation(p1, p2, p4)

	// General case: segments intersect if orientations differ
	if d1 != d2 && d3 != d4 {
		return true
	}

	// Special cases: collinear points
	if d1 == 0 && v.onSegment(p3, p1, p4) {
		return true
	}
	if d2 == 0 && v.onSegment(p3, p2, p4) {
		return true
	}
	if d3 == 0 && v.onSegment(p1, p3, p2) {
		return true
	}
	if d4 == 0 && v.onSegment(p1, p4, p2) {
		return true
	}

	return false
}

// orientation returns the orientation of triplet (p, q, r)
// 0 -> Collinear, 1 -> Clockwise, 2 -> Counter-clockwise
func (v *WallValidator) orientation(p, q, r dungeon.Position) int {
	val := (q.Y-p.Y)*(r.X-q.X) - (q.X-p.X)*(r.Y-q.Y)
	if val == 0 {
		return 0 // Collinear
	}
	if val > 0 {
		return 1 // Clockwise
	}
	return 2 // Counter-clockwise
}

// onSegment checks if point q lies on segment pr (given p, q, r are collinear)
func (v *WallValidator) onSegment(p, q, r dungeon.Position) bool {
	return q.X <= max(p.X, r.X) && q.X >= min(p.X, r.X) &&
		q.Y <= max(p.Y, r.Y) && q.Y >= min(p.Y, r.Y)
}
