package encounter

import (
	"sort"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// buildPerception creates PerceptionData for a monster from room spatial data
// Characters are enemies (from the monster's perspective)
// Other monsters are allies
// Enemies are sorted by distance (closest first)
// Adjacent is marked if distance == 1 hex
// Walls are converted to BlockedHexes for pathfinding
func buildPerception(
	roomData *spatial.RoomData,
	monsterID string,
	characterIDs []string,
	monsters []*monster.Data,
	walls []dungeon.WallSegment,
) *monster.PerceptionData {
	if roomData == nil {
		return &monster.PerceptionData{
			Enemies:      []monster.PerceivedEntity{},
			Allies:       []monster.PerceivedEntity{},
			BlockedHexes: wallsToBlockedHexes(walls),
		}
	}

	// Get monster's cube position from CubeEntities
	monsterPlacement, exists := roomData.CubeEntities[monsterID]
	if !exists {
		return &monster.PerceptionData{
			Enemies:      []monster.PerceivedEntity{},
			Allies:       []monster.PerceivedEntity{},
			BlockedHexes: wallsToBlockedHexes(walls),
		}
	}

	monsterPos := monsterPlacement.CubePosition

	// Build monster ID map for quick lookup (excluding self)
	monsterIDMap := make(map[string]*monster.Data)
	for _, m := range monsters {
		if m.ID != monsterID {
			monsterIDMap[m.ID] = m
		}
	}

	// Collect enemies (characters)
	enemies := make([]monster.PerceivedEntity, 0, len(characterIDs))
	for _, charID := range characterIDs {
		charPlacement, exists := roomData.CubeEntities[charID]
		if !exists {
			continue
		}

		// Calculate cube distance in hexes
		distance := cubeDistance(monsterPos, charPlacement.CubePosition)

		enemies = append(enemies, monster.PerceivedEntity{
			Entity: &entityAdapter{
				id:         charID,
				entityType: entityTypeCharacter,
			},
			Position: charPlacement.CubePosition,
			Distance: distance,
			Adjacent: distance == 1,
		})
	}

	// Sort enemies by distance (closest first)
	sort.Slice(enemies, func(i, j int) bool {
		return enemies[i].Distance < enemies[j].Distance
	})

	// Collect allies (other monsters)
	allies := make([]monster.PerceivedEntity, 0, len(monsterIDMap))
	for allyID := range monsterIDMap {
		allyPlacement, exists := roomData.CubeEntities[allyID]
		if !exists {
			continue
		}

		distance := cubeDistance(monsterPos, allyPlacement.CubePosition)

		allies = append(allies, monster.PerceivedEntity{
			Entity: &entityAdapter{
				id:         allyID,
				entityType: entityTypeMonster,
			},
			Position: allyPlacement.CubePosition,
			Distance: distance,
			Adjacent: distance == 1,
		})
	}

	return &monster.PerceptionData{
		MyPosition:   monsterPos,
		Enemies:      enemies,
		Allies:       allies,
		BlockedHexes: wallsToBlockedHexes(walls),
	}
}

// cubeDistance calculates the distance in hexes between two cube coordinates
// Uses the hex cube distance formula: (|dx| + |dy| + |dz|) / 2
func cubeDistance(from, to spatial.CubeCoordinate) int {
	dx := to.X - from.X
	dy := to.Y - from.Y
	dz := to.Z - from.Z
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dz < 0 {
		dz = -dz
	}
	return (dx + dy + dz) / 2
}

// entityAdapter implements core.Entity for ID lookup
type entityAdapter struct {
	id         string
	entityType core.EntityType
}

// GetID returns the entity's ID
func (e *entityAdapter) GetID() string {
	return e.id
}

// GetType returns the entity's type
func (e *entityAdapter) GetType() core.EntityType {
	return e.entityType
}

// wallsToBlockedHexes converts wall segments to a list of blocked cube coordinates
// Each wall segment is rasterized to find all hexes it passes through
func wallsToBlockedHexes(walls []dungeon.WallSegment) []spatial.CubeCoordinate {
	if len(walls) == 0 {
		return nil
	}

	// Use a map to deduplicate blocked positions
	blocked := make(map[spatial.CubeCoordinate]bool)

	for _, wall := range walls {
		// Add all hexes along the wall segment
		hexes := rasterizeWallSegment(wall.Start, wall.End)
		for _, hex := range hexes {
			blocked[hex] = true
		}
	}

	// Convert map to slice
	result := make([]spatial.CubeCoordinate, 0, len(blocked))
	for pos := range blocked {
		result = append(result, pos)
	}

	return result
}

// rasterizeWallSegment returns all hex positions along a wall segment
// Uses a simple line rasterization for cube coordinates
func rasterizeWallSegment(start, end dungeon.Position) []spatial.CubeCoordinate {
	var result []spatial.CubeCoordinate

	// Calculate the number of steps needed
	dx := end.X - start.X
	dy := end.Y - start.Y
	dz := end.Z - start.Z
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dz < 0 {
		dz = -dz
	}

	// For cube coordinates, use the maximum absolute difference
	steps := dx
	if dy > steps {
		steps = dy
	}
	if dz > steps {
		steps = dz
	}

	if steps == 0 {
		// Start and end are the same point
		return []spatial.CubeCoordinate{{X: start.X, Y: start.Y, Z: start.Z}}
	}

	// Interpolate along the line
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := float64(start.X) + t*float64(end.X-start.X)
		y := float64(start.Y) + t*float64(end.Y-start.Y)
		z := float64(start.Z) + t*float64(end.Z-start.Z)

		// Round to nearest cube coordinate
		cube := roundCube(x, y, z)
		result = append(result, cube)
	}

	return result
}

// roundCube rounds floating point cube coordinates to the nearest valid cube coordinate
// Ensures x + y + z = 0 constraint is maintained
func roundCube(x, y, z float64) spatial.CubeCoordinate {
	rx := int(x + 0.5)
	ry := int(y + 0.5)
	rz := int(z + 0.5)

	// Fix rounding errors to maintain x + y + z = 0
	xDiff := float64(rx) - x
	yDiff := float64(ry) - y
	zDiff := float64(rz) - z

	if xDiff < 0 {
		xDiff = -xDiff
	}
	if yDiff < 0 {
		yDiff = -yDiff
	}
	if zDiff < 0 {
		zDiff = -zDiff
	}

	if xDiff > yDiff && xDiff > zDiff {
		rx = -ry - rz
	} else if yDiff > zDiff {
		ry = -rx - rz
	} else {
		rz = -rx - ry
	}

	return spatial.CubeCoordinate{X: rx, Y: ry, Z: rz}
}
