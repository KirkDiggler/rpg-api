package encounter

import (
	"math"
	"sort"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// buildPerception creates PerceptionData for a monster from room spatial data
// Characters are enemies (from the monster's perspective)
// Other monsters are allies
// Enemies are sorted by distance (closest first)
// Adjacent is marked if distance <= 5 feet
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

	// Get monster's position
	monsterPlacement, exists := roomData.Entities[monsterID]
	if !exists {
		return &monster.PerceptionData{
			Enemies: []monster.PerceivedEntity{},
			Allies:  []monster.PerceivedEntity{},
		}
	}

	// Convert to toolkit Position (monster.Position uses int, spatial.Position uses float64)
	monsterPos := monster.Position{
		X: int(monsterPlacement.Position.X),
		Y: int(monsterPlacement.Position.Y),
	}

	// Create Grid for distance calculations
	grid := createGridFromRoomData(roomData)

	// Build character ID map for quick lookup
	charIDMap := make(map[string]bool)
	for _, charID := range characterIDs {
		charIDMap[charID] = true
	}

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
		charPlacement, exists := roomData.Entities[charID]
		if !exists {
			continue
		}

		// Calculate distance in feet
		distance := calculateDistance(grid, monsterPlacement.Position, charPlacement.Position)

		// Create PerceivedEntity
		enemies = append(enemies, monster.PerceivedEntity{
			Entity: &entityAdapter{
				id:         charID,
				entityType: "character",
			},
			Position: monster.Position{
				X: int(charPlacement.Position.X),
				Y: int(charPlacement.Position.Y),
			},
			Distance: distance,
			Adjacent: distance <= 5,
		})
	}

	// Sort enemies by distance (closest first)
	sort.Slice(enemies, func(i, j int) bool {
		return enemies[i].Distance < enemies[j].Distance
	})

	// Collect allies (other monsters)
	allies := make([]monster.PerceivedEntity, 0, len(monsterIDMap))
	for allyID := range monsterIDMap {
		allyPlacement, exists := roomData.Entities[allyID]
		if !exists {
			continue
		}

		// Calculate distance in feet
		distance := calculateDistance(grid, monsterPlacement.Position, allyPlacement.Position)

		// Create PerceivedEntity
		allies = append(allies, monster.PerceivedEntity{
			Entity: &entityAdapter{
				id:         allyID,
				entityType: "monster",
			},
			Position: monster.Position{
				X: int(allyPlacement.Position.X),
				Y: int(allyPlacement.Position.Y),
			},
			Distance: distance,
			Adjacent: distance <= 5,
		})
	}

	return &monster.PerceptionData{
		MyPosition: monsterPos,
		Enemies:    enemies,
		Allies:     allies,
	}
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

// createGridFromRoomData creates a Grid instance from RoomData for distance calculations
func createGridFromRoomData(roomData *spatial.RoomData) spatial.Grid {
	switch roomData.GridType {
	case spatial.GridTypeHex:
		// D&D 5e uses pointy-top hex by default
		pointyTop := true
		if roomData.HexOrientation != nil {
			pointyTop = *roomData.HexOrientation
		}
		return spatial.NewHexGrid(spatial.HexGridConfig{
			Width:     float64(roomData.Width),
			Height:    float64(roomData.Height),
			PointyTop: pointyTop,
		})
	case spatial.GridTypeSquare:
		return spatial.NewSquareGrid(spatial.SquareGridConfig{
			Width:  float64(roomData.Width),
			Height: float64(roomData.Height),
		})
	default:
		// Default to square grid
		return spatial.NewSquareGrid(spatial.SquareGridConfig{
			Width:  float64(roomData.Width),
			Height: float64(roomData.Height),
		})
	}
}

// calculateDistance calculates the distance in feet between two positions
// Uses the grid's Distance method and converts to feet (5ft per grid square/hex)
func calculateDistance(grid spatial.Grid, from, to spatial.Position) int {
	gridDistance := grid.Distance(from, to)
	feetDistance := gridDistance * 5.0 // Each grid unit is 5 feet in D&D 5e
	return int(math.Round(feetDistance))
}
