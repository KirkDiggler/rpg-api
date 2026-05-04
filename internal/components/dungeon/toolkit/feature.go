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

	// Filter spawn zone positions that land on walls
	// This handles cases where walls (like pillars) overlap with spawn positions
	spawnZones = g.filterSpawnZonesFromWalls(spawnZones, walls)

	// Generate obstacles based on feature rules
	obstacles := g.generateObstacles(input.Shape, input.Rules)

	// Generate terrain patches based on feature rules
	terrain := g.generateTerrain(input.Shape, input.Rules)

	return &dungeon.FeatureOutput{
		Features: &dungeon.FeatureLayout{
			Obstacles: obstacles,
			Terrain:   terrain,
		},
		Walls: walls,
		Zones: spawnZones,
	}, nil
}

// generateWallsFromPattern uses theme tables to select and apply a pattern
func (g *ToolkitFeatureGenerator) generateWallsFromPattern(input *dungeon.FeatureInput) []dungeon.WallSegment {
	if input.Tables == nil {
		return nil
	}

	// Check room size - small rooms don't need interior walls
	if input.Shape != nil {
		area := input.Shape.Width * input.Shape.Height
		if area < 80 {
			// Very small room - skip interior walls entirely
			return nil
		}
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

	// Scale walls based on room size
	walls := g.scaleWallsByRoomSize(patternOutput.Walls, input.Shape)

	return walls
}

// scaleWallsByRoomSize reduces the number of interior walls for smaller rooms
// Small rooms get cluttered quickly, so we reduce or eliminate interior walls
func (g *ToolkitFeatureGenerator) scaleWallsByRoomSize(walls []dungeon.WallSegment, shape *dungeon.Shape) []dungeon.WallSegment {
	if shape == nil || len(walls) == 0 {
		return walls
	}

	area := shape.Width * shape.Height

	// Small rooms (area < 120): max 2 walls
	if area < 120 {
		if len(walls) > 2 {
			// Randomly select 2 walls to keep
			g.random.Shuffle(len(walls), func(i, j int) {
				walls[i], walls[j] = walls[j], walls[i]
			})
			return walls[:2]
		}
		return walls
	}

	// Medium rooms (area < 200): max 4 walls
	if area < 200 {
		if len(walls) > 4 {
			g.random.Shuffle(len(walls), func(i, j int) {
				walls[i], walls[j] = walls[j], walls[i]
			})
			return walls[:4]
		}
		return walls
	}

	// Large rooms: keep all walls
	return walls
}

// generateObstacles places obstacles in the room based on rules
func (g *ToolkitFeatureGenerator) generateObstacles(shape *dungeon.Shape, rules dungeon.FeatureRules) []dungeon.Obstacle {
	var obstacles []dungeon.Obstacle

	// Check if we should place obstacles
	if g.random.Float64() > rules.ObstacleChance || len(rules.ObstacleTypes) == 0 {
		return obstacles
	}

	// Need at least 3x3 interior space for obstacles (margin of 1 on each side)
	if shape.Width < 3 || shape.Height < 3 {
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
	// Use margin of 1 to avoid placing on perimeter walls
	for i := 0; i < numObstacles; i++ {
		obstacleType := rules.ObstacleTypes[g.random.Intn(len(rules.ObstacleTypes))]

		// Random position within playable bounds (inside walls)
		// Range: 1 to Width-2, 1 to Height-2
		col := 1 + g.random.Intn(shape.Width-2)
		row := 1 + g.random.Intn(shape.Height-2)

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
		bounds := []dungeon.LocalPosition{
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

// gridToCube converts a grid position (col, row) to cube coordinates.
// Uses the same formula as offsetToCube in coords.go for consistency.
func gridToCube(col, row int) dungeon.LocalPosition {
	// Use same formula as offsetToCube for consistency with room bounds
	return offsetToCube(col, row)
}

// clampToPlayableArea ensures a grid position is inside the playable area (not on walls)
// Returns the clamped (col, row) values
func clampToPlayableArea(col, row, width, height int) (int, int) {
	// Playable area is from 1 to width-2, 1 to height-2
	minCol, maxCol := 1, width-2
	minRow, maxRow := 1, height-2

	if col < minCol {
		col = minCol
	}
	if col > maxCol {
		col = maxCol
	}
	if row < minRow {
		row = minRow
	}
	if row > maxRow {
		row = maxRow
	}
	return col, row
}

// safeGridToCube converts a grid position to cube coordinates, clamping to playable area
func safeGridToCube(col, row, width, height int) dungeon.LocalPosition {
	col, row = clampToPlayableArea(col, row, width, height)
	return gridToCube(col, row)
}

// filterSpawnZonesFromWalls removes spawn positions that overlap with wall positions
func (g *ToolkitFeatureGenerator) filterSpawnZonesFromWalls(zones []dungeon.Zone, walls []dungeon.WallSegment) []dungeon.Zone {
	if len(walls) == 0 {
		return zones
	}

	// Build set of all wall positions
	wallPositions := make(map[string]bool)
	for _, wall := range walls {
		if !wall.BlocksMovement {
			continue
		}
		// Enumerate positions along the wall segment
		positions := getWallPositions(wall)
		for _, pos := range positions {
			key := positionKey(pos)
			wallPositions[key] = true
		}
	}

	if len(wallPositions) == 0 {
		return zones
	}

	// Filter each zone's bounds
	filteredZones := make([]dungeon.Zone, 0, len(zones))
	for _, zone := range zones {
		filteredBounds := make([]dungeon.LocalPosition, 0, len(zone.Bounds))
		for _, pos := range zone.Bounds {
			key := positionKey(pos)
			if !wallPositions[key] {
				filteredBounds = append(filteredBounds, pos)
			}
		}

		// Only include zone if it still has valid positions
		if len(filteredBounds) > 0 {
			zone.Bounds = filteredBounds
			// Update capacity to match remaining positions
			if zone.Capacity > len(filteredBounds) {
				zone.Capacity = len(filteredBounds)
			}
			filteredZones = append(filteredZones, zone)
		}
	}

	return filteredZones
}

// getWallPositions returns all positions along a wall segment
func getWallPositions(wall dungeon.WallSegment) []dungeon.LocalPosition {
	var positions []dungeon.LocalPosition

	dx := wall.End.X - wall.Start.X
	dz := wall.End.Z - wall.Start.Z

	steps := absInt(dx)
	if absInt(dz) > steps {
		steps = absInt(dz)
	}
	if steps == 0 {
		steps = 1
	}

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := wall.Start.X + int(float64(dx)*t)
		z := wall.Start.Z + int(float64(dz)*t)
		y := -x - z // Cube coordinate constraint
		positions = append(positions, dungeon.LocalPosition{X: x, Y: y, Z: z})
	}

	return positions
}

// positionKey creates a unique string key for a position
func positionKey(pos dungeon.LocalPosition) string {
	return fmt.Sprintf("%d,%d,%d", pos.X, pos.Y, pos.Z)
}

// absInt returns the absolute value of an integer
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// maxInt returns the maximum of two integers
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// generateSpawnZones creates spawn zones based on room type
// All positions use cube coordinates via toolkit's converter
func (g *ToolkitFeatureGenerator) generateSpawnZones(shape *dungeon.Shape, roomType dungeon.RoomType) []dungeon.Zone {
	var zones []dungeon.Zone

	// Need minimum room size for spawn zones
	if shape.Width < 4 || shape.Height < 4 {
		// Tiny room - single spawn point in center
		centerCol, centerRow := clampToPlayableArea(shape.Width/2, shape.Height/2, shape.Width, shape.Height)
		zones = append(zones, dungeon.Zone{
			ID:   uuid.New().String(),
			Type: dungeon.ZoneTypeMonsterSpawn,
			Bounds: []dungeon.LocalPosition{
				gridToCube(centerCol, centerRow),
			},
			Capacity: 1,
		})
		return zones
	}

	switch roomType {
	case dungeon.RoomTypeEntrance:
		// Entrance rooms have player spawn near south door (entrance)
		// and monster spawn near north door (exit)
		//
		// North wall:  ═══════════════
		// Row H-2:       [M] [M] [M]     <- monsters near exit
		// Row H-3:     [M]   [D]   [M]   <- monsters flanking exit door
		//              ...
		// Row 2:       [2] [1] [3]       <- players (center, left, right)
		// Row 1:     [4]   [D]   [5]     <- players flanking entrance
		// South wall:  ═══════════════

		// Find actual door positions from ConnectionPoints
		southDoorCol := shape.Width / 2 // fallback
		northDoorCol := shape.Width / 2 // fallback
		for _, cp := range shape.ConnectionPoints {
			// ConnectionPoint Position is in cube coords, extract column
			// In our grid-to-cube: col = x, row = z
			switch cp.Direction {
			case "south":
				southDoorCol = cp.Position.X
			case "north":
				northDoorCol = cp.Position.X
			}
		}

		northRow := shape.Height - 2 // Just inside north wall

		// Player spawn zone (south) and monster spawn zone (north)
		zones = append(zones,
			// Player spawn - 5 positions in symmetrical fill order near south door
			dungeon.Zone{
				ID:   uuid.New().String(),
				Type: dungeon.ZoneTypePlayerSpawn,
				Bounds: []dungeon.LocalPosition{
					safeGridToCube(southDoorCol, 2, shape.Width, shape.Height),   // 0: center back (behind door)
					safeGridToCube(southDoorCol-1, 2, shape.Width, shape.Height), // 1: left back
					safeGridToCube(southDoorCol+1, 2, shape.Width, shape.Height), // 2: right back
					safeGridToCube(southDoorCol-1, 1, shape.Width, shape.Height), // 3: left flank
					safeGridToCube(southDoorCol+1, 1, shape.Width, shape.Height), // 4: right flank
				},
				Capacity: 5,
			},
			// Monster spawn - positions near north door (exit)
			dungeon.Zone{
				ID:   uuid.New().String(),
				Type: dungeon.ZoneTypeMonsterSpawn,
				Bounds: []dungeon.LocalPosition{
					safeGridToCube(northDoorCol, northRow, shape.Width, shape.Height),     // center, near door
					safeGridToCube(northDoorCol-1, northRow, shape.Width, shape.Height),   // left of door
					safeGridToCube(northDoorCol+1, northRow, shape.Width, shape.Height),   // right of door
					safeGridToCube(northDoorCol-1, northRow-1, shape.Width, shape.Height), // left, one row back
					safeGridToCube(northDoorCol+1, northRow-1, shape.Width, shape.Height), // right, one row back
					safeGridToCube(northDoorCol, northRow-1, shape.Width, shape.Height),   // center, one row back
				},
				Capacity: 6,
			},
		)

	case dungeon.RoomTypeBoss:
		// Boss rooms have a boss zone in the center and monster spawn zones around it
		centerCol := shape.Width / 2
		centerRow := shape.Height / 2

		// Boss zone - single position in center
		zones = append(zones,
			dungeon.Zone{
				ID:   uuid.New().String(),
				Type: dungeon.ZoneTypeBoss,
				Bounds: []dungeon.LocalPosition{
					safeGridToCube(centerCol, centerRow, shape.Width, shape.Height),
				},
				Capacity: 1,
			},
			// Monster spawn zone - positions in upper area
			dungeon.Zone{
				ID:   uuid.New().String(),
				Type: dungeon.ZoneTypeMonsterSpawn,
				Bounds: []dungeon.LocalPosition{
					safeGridToCube(centerCol-2, centerRow-2, shape.Width, shape.Height),
					safeGridToCube(centerCol, centerRow-2, shape.Width, shape.Height),
					safeGridToCube(centerCol+2, centerRow-2, shape.Width, shape.Height),
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
			Bounds: []dungeon.LocalPosition{
				safeGridToCube(centerCol-1, centerRow-1, shape.Width, shape.Height),
				safeGridToCube(centerCol, centerRow-1, shape.Width, shape.Height),
				safeGridToCube(centerCol+1, centerRow-1, shape.Width, shape.Height),
				safeGridToCube(centerCol-1, centerRow, shape.Width, shape.Height),
				safeGridToCube(centerCol+1, centerRow, shape.Width, shape.Height),
				safeGridToCube(centerCol, centerRow+1, shape.Width, shape.Height),
			},
			Capacity: 6,
		})
	}

	return zones
}
