package toolkit

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/google/uuid"

	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// ToolkitFeatureGenerator implements dungeon.FeatureGenerator
type ToolkitFeatureGenerator struct {
	random    *rand.Rand
	patterns  *PatternRegistry
	validator *WallValidator
}

// NewToolkitFeatureGenerator creates a new feature generator
func NewToolkitFeatureGenerator() *ToolkitFeatureGenerator {
	// #nosec G404 - Using math/rand for seeded procedural generation, not cryptographic purposes
	return &ToolkitFeatureGenerator{
		random:    rand.New(rand.NewSource(0)), // Will be reseeded per generation
		patterns:  NewPatternRegistry(),
		validator: NewWallValidator(),
	}
}

// Generate places obstacles, terrain, spawn zones, and internal walls in a room
func (g *ToolkitFeatureGenerator) Generate(_ context.Context, input *dungeon.FeatureInput) (*dungeon.FeatureOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	if input.Shape == nil {
		return nil, fmt.Errorf("shape is required")
	}

	// Reseed the random generator for deterministic results
	if input.Seed != 0 {
		g.random.Seed(input.Seed)
	}

	// Generate spawn zones based on room type (need these before validating walls)
	spawnZones := g.generateSpawnZones(input.Shape, input.RoomType)

	// Generate internal walls using pattern system
	var walls []dungeon.WallSegment
	if input.Tables != nil {
		walls = g.generateWallsFromPattern(input)

		// Validate walls don't block spawn zones
		validationResult := g.validator.Validate(&ValidationInput{
			Shape:      input.Shape,
			Walls:      walls,
			SpawnZones: spawnZones,
		})

		// If validation fails, use suggested safe walls
		if !validationResult.IsValid {
			walls = validationResult.SuggestedWalls
		}
	}

	// Generate obstacles based on feature rules
	obstacles := g.generateObstacles(input.Shape, input.Rules)

	// Generate terrain patches based on feature rules
	terrain := g.generateTerrain(input.Shape, input.Rules)

	return &dungeon.FeatureOutput{
		Features: &dungeon.FeatureLayout{
			Obstacles:  obstacles,
			Terrain:    terrain,
			SpawnZones: spawnZones,
		},
		Walls: walls,
	}, nil
}

// generateWallsFromPattern uses theme tables to select and apply a pattern
func (g *ToolkitFeatureGenerator) generateWallsFromPattern(input *dungeon.FeatureInput) []dungeon.WallSegment {
	if input.Tables == nil {
		return nil
	}

	// Select pattern based on room type
	patternType, err := input.Tables.SelectPattern(input.RoomType, g.random)
	if err != nil {
		patternType = dungeon.PatternEmpty
	}

	// Get pattern function
	patternFunc, exists := g.patterns.GetPattern(patternType)
	if !exists {
		return nil
	}

	// Select density
	density := dungeon.DensityMedium
	if len(input.Tables.Density) > 0 {
		density = input.Tables.SelectDensity(g.random)
	}

	// Generate walls using pattern
	patternOutput := patternFunc(&PatternInput{
		Shape:   input.Shape,
		Density: density,
		Seed:    input.Seed,
	})

	return patternOutput.Walls
}

// generateObstacles places obstacles in the room based on rules
func (g *ToolkitFeatureGenerator) generateObstacles(shape *dungeon.Shape, rules dungeon.FeatureRules) []dungeon.Obstacle {
	var obstacles []dungeon.Obstacle

	// Check if we should place obstacles
	if g.random.Float64() > rules.ObstacleChance || len(rules.ObstacleTypes) == 0 {
		return obstacles
	}

	// Calculate number of obstacles based on room area
	numObstacles := int(float64(shape.Area) * 0.1) // ~10% of area
	if numObstacles < 1 {
		numObstacles = 1
	}
	if numObstacles > 10 {
		numObstacles = 10 // Cap at 10 obstacles
	}

	// Place obstacles at random positions using cube coordinates
	for i := 0; i < numObstacles; i++ {
		obstacleType := rules.ObstacleTypes[g.random.Intn(len(rules.ObstacleTypes))]

		// Random position within bounds (col, row)
		col := g.random.Intn(shape.Width)
		row := g.random.Intn(shape.Height)

		obstacles = append(obstacles, dungeon.Obstacle{
			ID:                uuid.New().String(),
			Type:              obstacleType,
			Position:          offsetToCube(col, row),
			BlocksMovement:    true,
			BlocksLineOfSight: isObstacleBlockingSight(obstacleType),
		})
	}

	return obstacles
}

// isObstacleBlockingSight determines if an obstacle blocks line of sight
func isObstacleBlockingSight(obstacleType dungeon.ObstacleType) bool {
	switch obstacleType {
	case dungeon.ObstacleTypePillar, dungeon.ObstacleTypeSarcophagus, dungeon.ObstacleTypeBoulder, dungeon.ObstacleTypeStalagmite:
		return true
	case dungeon.ObstacleTypeCrate, dungeon.ObstacleTypeBarrel:
		return false // Can see over small objects
	default:
		return false
	}
}

// generateTerrain places terrain patches based on rules
func (g *ToolkitFeatureGenerator) generateTerrain(shape *dungeon.Shape, rules dungeon.FeatureRules) []dungeon.TerrainPatch {
	var terrain []dungeon.TerrainPatch

	// Check if we should place terrain
	if g.random.Float64() > rules.TerrainChance || len(rules.TerrainTypes) == 0 {
		return terrain
	}

	// Place 1-3 terrain patches
	numPatches := 1 + g.random.Intn(3)

	for i := 0; i < numPatches; i++ {
		terrainType := rules.TerrainTypes[g.random.Intn(len(rules.TerrainTypes))]

		// Create a small patch (2x2 to 4x4)
		patchSize := 2 + g.random.Intn(3)

		// Random starting position
		startX := g.random.Intn(shape.Width - patchSize)
		startY := g.random.Intn(shape.Height - patchSize)

		// Create bounds for the patch
		bounds := []dungeon.Position{
			{X: startX, Y: startY, Z: 0},
			{X: startX + patchSize, Y: startY, Z: 0},
			{X: startX + patchSize, Y: startY + patchSize, Z: 0},
			{X: startX, Y: startY + patchSize, Z: 0},
		}

		terrain = append(terrain, dungeon.TerrainPatch{
			ID:           uuid.New().String(),
			Type:         terrainType,
			Bounds:       bounds,
			MovementCost: getTerrainMovementCost(terrainType),
		})
	}

	return terrain
}

// getTerrainMovementCost returns the movement cost multiplier for terrain types
func getTerrainMovementCost(terrainType dungeon.TerrainType) float64 {
	switch terrainType {
	case dungeon.TerrainTypeDifficult:
		return 2.0 // Costs double movement
	case dungeon.TerrainTypeWater:
		return 2.0
	case dungeon.TerrainTypeIce:
		return 1.5
	case dungeon.TerrainTypeHazardous, dungeon.TerrainTypeLava:
		return 3.0 // Very costly to cross
	default:
		return 1.0
	}
}

// gridToCube converts a grid position (col, row) to cube coordinates using toolkit's converter.
// This ensures we always use the same conversion logic as the toolkit.
func gridToCube(col, row int) dungeon.Position {
	// Use toolkit's standard offset-to-cube conversion
	cube := spatial.OffsetCoordinateToCube(spatial.Position{X: float64(col), Y: float64(row)})
	return dungeon.Position{X: cube.X, Y: cube.Y, Z: cube.Z}
}

// generateSpawnZones creates spawn zones based on room type
// All positions use cube coordinates via toolkit's converter
func (g *ToolkitFeatureGenerator) generateSpawnZones(shape *dungeon.Shape, roomType dungeon.RoomType) []dungeon.Zone {
	var zones []dungeon.Zone

	switch roomType {
	case dungeon.RoomTypeEntrance:
		// Entrance rooms have a player spawn zone near the entrance (bottom-left area)
		// Generate individual spawn positions for up to 4 players
		zones = append(zones, dungeon.Zone{
			ID:   uuid.New().String(),
			Type: dungeon.ZoneTypePlayerSpawn,
			Bounds: []dungeon.Position{
				gridToCube(1, 1),
				gridToCube(2, 1),
				gridToCube(1, 2),
				gridToCube(2, 2),
			},
			Capacity: 4, // Standard party size
		})

	case dungeon.RoomTypeBoss:
		// Boss rooms have a boss zone in the center and monster spawn zones around it
		centerCol := shape.Width / 2
		centerRow := shape.Height / 2

		// Boss zone - single position in center
		zones = append(zones,
			dungeon.Zone{
				ID:   uuid.New().String(),
				Type: dungeon.ZoneTypeBoss,
				Bounds: []dungeon.Position{
					gridToCube(centerCol, centerRow),
				},
				Capacity: 1,
			},
			// Monster spawn zone - positions in upper area
			dungeon.Zone{
				ID:   uuid.New().String(),
				Type: dungeon.ZoneTypeMonsterSpawn,
				Bounds: []dungeon.Position{
					gridToCube(centerCol-2, centerRow-2),
					gridToCube(centerCol, centerRow-2),
					gridToCube(centerCol+2, centerRow-2),
				},
				Capacity: 3,
			},
		)

	default:
		// Regular rooms have monster spawn zones in the center area
		centerCol := shape.Width / 2
		centerRow := shape.Height / 2

		// Generate individual spawn positions for monsters
		zones = append(zones, dungeon.Zone{
			ID:   uuid.New().String(),
			Type: dungeon.ZoneTypeMonsterSpawn,
			Bounds: []dungeon.Position{
				gridToCube(centerCol-1, centerRow-1),
				gridToCube(centerCol, centerRow-1),
				gridToCube(centerCol+1, centerRow-1),
				gridToCube(centerCol-1, centerRow),
				gridToCube(centerCol+1, centerRow),
				gridToCube(centerCol, centerRow+1),
			},
			Capacity: 6,
		})
	}

	return zones
}
