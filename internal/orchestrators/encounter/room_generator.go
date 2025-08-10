package encounter

import (
	"math/rand"

	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// obstacleEntity implements core.Entity and spatial.Placeable for walls and obstacles
type obstacleEntity struct {
	id         string
	entityType string
	blocksMove bool
	blocksLOS  bool
	size       int
}

func (e *obstacleEntity) GetID() string {
	return e.id
}

func (e *obstacleEntity) GetType() string {
	return e.entityType
}

func (e *obstacleEntity) GetSize() int {
	if e.size == 0 {
		return 1 // Default size
	}
	return e.size
}

func (e *obstacleEntity) BlocksMovement() bool {
	return e.blocksMove
}

func (e *obstacleEntity) BlocksLineOfSight() bool {
	return e.blocksLOS
}

// RoomTemplate defines different room layouts
type RoomTemplate string

const (
	RoomTemplateEmpty   RoomTemplate = "empty"
	RoomTemplatePillars RoomTemplate = "pillars"
	RoomTemplateCorners RoomTemplate = "corners"
	RoomTemplateMaze    RoomTemplate = "maze"
	RoomTemplateArena   RoomTemplate = "arena"
)

// GenerateRoomObstacles adds obstacles to a room based on a template
func GenerateRoomObstacles(room *spatial.BasicRoom, template RoomTemplate, idGen func() string) {
	switch template {
	case RoomTemplatePillars:
		generatePillars(room, idGen)
	case RoomTemplateCorners:
		generateCornerWalls(room, idGen)
	case RoomTemplateMaze:
		generateMazeWalls(room, idGen)
	case RoomTemplateArena:
		generateArenaObstacles(room, idGen)
	case RoomTemplateEmpty:
		// No obstacles
	default:
		// Random selection
		templates := []RoomTemplate{
			RoomTemplatePillars,
			RoomTemplateCorners,
			RoomTemplateArena,
		}
		selected := templates[rand.Intn(len(templates))]
		GenerateRoomObstacles(room, selected, idGen)
	}
}

// generatePillars adds pillar obstacles in a pattern
func generatePillars(room *spatial.BasicRoom, idGen func() string) {
	// Add 4 pillars in a square pattern
	pillarPositions := []spatial.Position{
		{X: 3, Y: 3},
		{X: 3, Y: 7},
		{X: 7, Y: 3},
		{X: 7, Y: 7},
	}

	for _, pos := range pillarPositions {
		pillar := &obstacleEntity{
			id:         idGen(),
			entityType: "pillar",
			blocksMove: true,
			blocksLOS:  true,
		}
		room.PlaceEntity(pillar, pos)
	}
}

// generateCornerWalls adds L-shaped walls in corners
func generateCornerWalls(room *spatial.BasicRoom, idGen func() string) {
	// Top-left corner wall
	topLeftPositions := []spatial.Position{
		{X: 1, Y: 1},
		{X: 2, Y: 1},
		{X: 1, Y: 2},
	}

	// Bottom-right corner wall
	bottomRightPositions := []spatial.Position{
		{X: 8, Y: 8},
		{X: 7, Y: 8},
		{X: 8, Y: 7},
	}

	for _, pos := range topLeftPositions {
		wall := &obstacleEntity{
			id:         idGen(),
			entityType: "wall",
			blocksMove: true,
			blocksLOS:  true,
		}
		room.PlaceEntity(wall, pos)
	}

	for _, pos := range bottomRightPositions {
		wall := &obstacleEntity{
			id:         idGen(),
			entityType: "wall",
			blocksMove: true,
			blocksLOS:  true,
		}
		room.PlaceEntity(wall, pos)
	}
}

// generateMazeWalls creates a simple maze-like structure
func generateMazeWalls(room *spatial.BasicRoom, idGen func() string) {
	// Create two partial walls that create a maze-like path

	// Horizontal wall with gap
	for x := 2.0; x <= 6.0; x++ {
		if x == 4.0 { // Gap for passage
			continue
		}
		wall := &obstacleEntity{
			id:         idGen(),
			entityType: "wall",
			blocksMove: true,
			blocksLOS:  true,
		}
		room.PlaceEntity(wall, spatial.Position{X: x, Y: 5})
	}

	// Vertical wall with gap
	for y := 3.0; y <= 7.0; y++ {
		if y == 5.0 { // Gap for passage
			continue
		}
		wall := &obstacleEntity{
			id:         idGen(),
			entityType: "wall",
			blocksMove: true,
			blocksLOS:  true,
		}
		room.PlaceEntity(wall, spatial.Position{X: 5, Y: y})
	}
}

// generateArenaObstacles creates scattered cover objects
func generateArenaObstacles(room *spatial.BasicRoom, idGen func() string) {
	// Add barrels and crates that block movement but not all block LOS
	obstacles := []struct {
		pos       spatial.Position
		obsType   string
		blocksLOS bool
	}{
		{pos: spatial.Position{X: 2, Y: 5}, obsType: "barrel", blocksLOS: false},
		{pos: spatial.Position{X: 4, Y: 4}, obsType: "crate", blocksLOS: true},
		{pos: spatial.Position{X: 6, Y: 3}, obsType: "barrel", blocksLOS: false},
		{pos: spatial.Position{X: 5, Y: 6}, obsType: "rubble", blocksLOS: true},
		{pos: spatial.Position{X: 8, Y: 5}, obsType: "crate", blocksLOS: true},
		{pos: spatial.Position{X: 3, Y: 8}, obsType: "barrel", blocksLOS: false},
	}

	for _, obs := range obstacles {
		obstacle := &obstacleEntity{
			id:         idGen(),
			entityType: obs.obsType,
			blocksMove: true,
			blocksLOS:  obs.blocksLOS,
		}
		room.PlaceEntity(obstacle, obs.pos)
	}
}

// GetRandomTemplate returns a random room template
func GetRandomTemplate() RoomTemplate {
	templates := []RoomTemplate{
		RoomTemplatePillars,
		RoomTemplateCorners,
		RoomTemplateArena,
		RoomTemplateMaze,
	}
	return templates[rand.Intn(len(templates))]
}
