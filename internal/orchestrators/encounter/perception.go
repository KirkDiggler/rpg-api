package encounter

import (
	"sort"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// buildPerception creates PerceptionData for a monster from room spatial data
// Characters are enemies (from the monster's perspective)
// Other monsters are allies
// Enemies are sorted by distance (closest first)
// Adjacent is marked if distance == 1 hex
func buildPerception(
	roomData *spatial.RoomData,
	monsterID string,
	characterIDs []string,
	monsters []*monster.Data,
) *monster.PerceptionData {
	if roomData == nil {
		return &monster.PerceptionData{
			Enemies: []monster.PerceivedEntity{},
			Allies:  []monster.PerceivedEntity{},
		}
	}

	// Get monster's cube position from CubeEntities
	monsterPlacement, exists := roomData.CubeEntities[monsterID]
	if !exists {
		return &monster.PerceptionData{
			Enemies: []monster.PerceivedEntity{},
			Allies:  []monster.PerceivedEntity{},
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
		MyPosition: monsterPos,
		Enemies:    enemies,
		Allies:     allies,
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
