package toolkit

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/google/uuid"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// ToolkitFeatureGenerator implements dungeon.FeatureGenerator
type ToolkitFeatureGenerator struct {
	random *rand.Rand
}

// NewToolkitFeatureGenerator creates a new feature generator
func NewToolkitFeatureGenerator() *ToolkitFeatureGenerator {
	// #nosec G404 - Using math/rand for seeded procedural generation, not cryptographic purposes
	return &ToolkitFeatureGenerator{
		random: rand.New(rand.NewSource(0)), // Will be reseeded per generation
	}
}

// Generate places obstacles, terrain, and spawn zones in a room
func (g *ToolkitFeatureGenerator) Generate(ctx context.Context, input *dungeon.FeatureInput) (*dungeon.FeatureOutput, error) {
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

	// Generate obstacles based on feature rules
	obstacles := g.generateObstacles(input.Shape, input.Rules)

	// Generate terrain patches based on feature rules
	terrain := g.generateTerrain(input.Shape, input.Rules)

	// Generate spawn zones based on room type
	spawnZones := g.generateSpawnZones(input.Shape, input.RoomType)

	return &dungeon.FeatureOutput{
		Features: &dungeon.FeatureLayout{
			Obstacles:  obstacles,
			Terrain:    terrain,
			SpawnZones: spawnZones,
		},
	}, nil
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

	// Place obstacles at random positions
	for i := 0; i < numObstacles; i++ {
		obstacleType := rules.ObstacleTypes[g.random.Intn(len(rules.ObstacleTypes))]

		// Random position within bounds
		x := g.random.Intn(shape.Width)
		y := g.random.Intn(shape.Height)

		obstacles = append(obstacles, dungeon.Obstacle{
			ID:   uuid.New().String(),
			Type: obstacleType,
			Position: dungeon.Position{
				X: x,
				Y: y,
				Z: 0,
			},
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

// generateSpawnZones creates spawn zones based on room type
func (g *ToolkitFeatureGenerator) generateSpawnZones(shape *dungeon.Shape, roomType string) []dungeon.Zone {
	var zones []dungeon.Zone

	switch roomType {
	case "entrance":
		// Entrance rooms have a player spawn zone near the entrance
		zones = append(zones, dungeon.Zone{
			ID:   uuid.New().String(),
			Type: dungeon.ZoneTypePlayerSpawn,
			Bounds: []dungeon.Position{
				{X: 0, Y: 0, Z: 0},
				{X: 3, Y: 0, Z: 0},
				{X: 3, Y: 3, Z: 0},
				{X: 0, Y: 3, Z: 0},
			},
			Capacity: 4, // Standard party size
		})

	case "boss":
		// Boss rooms have a boss zone in the center and monster spawn zones around it
		centerX := shape.Width / 2
		centerY := shape.Height / 2

		// Boss zone in center and monster spawn zones in corners
		zones = append(zones,
			dungeon.Zone{
				ID:   uuid.New().String(),
				Type: dungeon.ZoneTypeBoss,
				Bounds: []dungeon.Position{
					{X: centerX - 2, Y: centerY - 2, Z: 0},
					{X: centerX + 2, Y: centerY - 2, Z: 0},
					{X: centerX + 2, Y: centerY + 2, Z: 0},
					{X: centerX - 2, Y: centerY + 2, Z: 0},
				},
				Capacity: 1,
			},
			dungeon.Zone{
				ID:   uuid.New().String(),
				Type: dungeon.ZoneTypeMonsterSpawn,
				Bounds: []dungeon.Position{
					{X: 0, Y: 0, Z: 0},
					{X: 4, Y: 0, Z: 0},
					{X: 4, Y: 4, Z: 0},
					{X: 0, Y: 4, Z: 0},
				},
				Capacity: 3,
			},
		)

	default:
		// Regular rooms have monster spawn zones
		// Place spawn zone in the center
		centerX := shape.Width / 2
		centerY := shape.Height / 2

		zones = append(zones, dungeon.Zone{
			ID:   uuid.New().String(),
			Type: dungeon.ZoneTypeMonsterSpawn,
			Bounds: []dungeon.Position{
				{X: centerX - 3, Y: centerY - 3, Z: 0},
				{X: centerX + 3, Y: centerY - 3, Z: 0},
				{X: centerX + 3, Y: centerY + 3, Z: 0},
				{X: centerX - 3, Y: centerY + 3, Z: 0},
			},
			Capacity: 6,
		})
	}

	return zones
}
