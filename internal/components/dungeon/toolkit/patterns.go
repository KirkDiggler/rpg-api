package toolkit

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// PatternFunc generates internal walls for a room
type PatternFunc func(input *PatternInput) *PatternOutput

// PatternInput contains parameters for pattern generation
type PatternInput struct {
	Shape         *dungeon.Shape
	Density       dungeon.DensityRange
	ObstacleTypes []dungeon.ObstacleType
	Seed          int64
}

// PatternOutput contains generated walls and obstacles
type PatternOutput struct {
	Walls     []dungeon.WallSegment
	Obstacles []dungeon.Obstacle
}

// PatternRegistry holds all available pattern functions
type PatternRegistry struct {
	patterns map[dungeon.PatternType]PatternFunc
}

// NewPatternRegistry creates a registry with all built-in patterns
func NewPatternRegistry() *PatternRegistry {
	r := &PatternRegistry{
		patterns: make(map[dungeon.PatternType]PatternFunc),
	}

	// Register built-in patterns
	r.patterns[dungeon.PatternEmpty] = EmptyPattern
	r.patterns[dungeon.PatternSparse] = SparsePattern
	r.patterns[dungeon.PatternCoverClusters] = CoverClustersPattern
	r.patterns[dungeon.PatternChokepoints] = ChokepointsPattern
	r.patterns[dungeon.PatternCentralFeature] = CentralFeaturePattern
	r.patterns[dungeon.PatternPerimeterCover] = PerimeterCoverPattern
	r.patterns[dungeon.PatternPillarGrid] = PillarGridPattern

	return r
}

// GetPattern returns a pattern function by type
func (r *PatternRegistry) GetPattern(patternType dungeon.PatternType) (PatternFunc, bool) {
	pattern, exists := r.patterns[patternType]
	return pattern, exists
}

// RegisterPattern adds a custom pattern to the registry
func (r *PatternRegistry) RegisterPattern(patternType dungeon.PatternType, fn PatternFunc) {
	r.patterns[patternType] = fn
}

// EmptyPattern generates no internal walls
func EmptyPattern(_ *PatternInput) *PatternOutput {
	return &PatternOutput{
		Walls:     []dungeon.WallSegment{},
		Obstacles: []dungeon.Obstacle{},
	}
}

// SparsePattern generates 1-2 single obstacles for easy navigation
func SparsePattern(input *PatternInput) *PatternOutput {
	if input == nil || input.Shape == nil {
		return &PatternOutput{}
	}

	// #nosec G404 - Using math/rand for seeded, reproducible wall pattern generation
	rng := rand.New(rand.NewSource(input.Seed))
	numWalls := 1 + rng.Intn(2) // 1-2 walls

	var walls []dungeon.WallSegment
	margin := 2.0

	for i := 0; i < numWalls; i++ {
		// Place wall in random position with margin from edges
		x := margin + rng.Float64()*(float64(input.Shape.Width)-2*margin)
		y := margin + rng.Float64()*(float64(input.Shape.Height)-2*margin)

		// Random short wall segment
		length := 2.0 + rng.Float64()*2.0
		horizontal := rng.Float64() < 0.5

		var start, end dungeon.Position
		if horizontal {
			start = dungeon.Position{X: int(x - length/2), Y: int(y)}
			end = dungeon.Position{X: int(x + length/2), Y: int(y)}
		} else {
			start = dungeon.Position{X: int(x), Y: int(y - length/2)}
			end = dungeon.Position{X: int(x), Y: int(y + length/2)}
		}

		walls = append(walls, dungeon.WallSegment{
			ID:                fmt.Sprintf("internal_%d", i),
			Start:             start,
			End:               end,
			Type:              dungeon.WallTypeDestructible,
			BlocksMovement:    true,
			BlocksLineOfSight: true,
		})
	}

	return &PatternOutput{
		Walls:     walls,
		Obstacles: []dungeon.Obstacle{},
	}
}

// CoverClustersPattern creates 2-4 groups of connected walls for tactical movement
func CoverClustersPattern(input *PatternInput) *PatternOutput {
	if input == nil || input.Shape == nil {
		return &PatternOutput{}
	}

	// #nosec G404 - Using math/rand for seeded, reproducible wall pattern generation
	rng := rand.New(rand.NewSource(input.Seed))

	// Determine number of clusters based on room size
	minClusters := 2
	maxClusters := 4
	numClusters := minClusters + rng.Intn(maxClusters-minClusters+1)

	var walls []dungeon.WallSegment
	wallID := 0

	// Divide room into quadrants for cluster placement
	quadrantWidth := float64(input.Shape.Width) / 2
	quadrantHeight := float64(input.Shape.Height) / 2
	margin := 3.0

	for i := 0; i < numClusters; i++ {
		// Place cluster in a quadrant
		quadrantX := float64(i%2) * quadrantWidth
		quadrantY := float64(i/2) * quadrantHeight

		// Random position within quadrant
		clusterX := quadrantX + margin + rng.Float64()*(quadrantWidth-2*margin)
		clusterY := quadrantY + margin + rng.Float64()*(quadrantHeight-2*margin)

		// Generate 2-3 walls per cluster in L or T shape
		clusterWalls := generateCluster(clusterX, clusterY, rng, &wallID)
		walls = append(walls, clusterWalls...)
	}

	return &PatternOutput{
		Walls:     walls,
		Obstacles: []dungeon.Obstacle{},
	}
}

func generateCluster(centerX, centerY float64, rng *rand.Rand, wallID *int) []dungeon.WallSegment {
	var walls []dungeon.WallSegment
	numWalls := 2 + rng.Intn(2) // 2-3 walls per cluster
	wallLength := 2.0 + rng.Float64()*2.0

	for i := 0; i < numWalls; i++ {
		angle := float64(i) * (math.Pi / float64(numWalls))
		dx := wallLength * math.Cos(angle) / 2
		dy := wallLength * math.Sin(angle) / 2

		walls = append(walls, dungeon.WallSegment{
			ID:                fmt.Sprintf("cluster_%d", *wallID),
			Start:             dungeon.Position{X: int(centerX - dx), Y: int(centerY - dy)},
			End:               dungeon.Position{X: int(centerX + dx), Y: int(centerY + dy)},
			Type:              dungeon.WallTypeDestructible,
			BlocksMovement:    true,
			BlocksLineOfSight: true,
		})
		*wallID++
	}

	return walls
}

// ChokepointsPattern creates wall segments that divide space into sections
func ChokepointsPattern(input *PatternInput) *PatternOutput {
	if input == nil || input.Shape == nil {
		return &PatternOutput{}
	}

	// #nosec G404 - Using math/rand for seeded, reproducible wall pattern generation
	rng := rand.New(rand.NewSource(input.Seed))

	var walls []dungeon.WallSegment
	width := float64(input.Shape.Width)
	height := float64(input.Shape.Height)

	// Create 1-3 dividing walls
	numWalls := 1 + rng.Intn(3)

	for i := 0; i < numWalls; i++ {
		// Alternate between horizontal and vertical chokepoints
		horizontal := i%2 == 0

		var start, end dungeon.Position
		margin := 3.0
		gapSize := 3.0 // Gap for passage

		if horizontal {
			// Horizontal wall with gap
			y := margin + rng.Float64()*(height-2*margin)
			gapStart := margin + rng.Float64()*(width-2*margin-gapSize)

			// Wall before gap
			walls = append(walls, dungeon.WallSegment{
				ID:                fmt.Sprintf("choke_%d_a", i),
				Start:             dungeon.Position{X: int(margin), Y: int(y)},
				End:               dungeon.Position{X: int(gapStart), Y: int(y)},
				Type:              dungeon.WallTypeDestructible,
				BlocksMovement:    true,
				BlocksLineOfSight: true,
			})

			// Wall after gap
			walls = append(walls, dungeon.WallSegment{
				ID:                fmt.Sprintf("choke_%d_b", i),
				Start:             dungeon.Position{X: int(gapStart + gapSize), Y: int(y)},
				End:               dungeon.Position{X: int(width - margin), Y: int(y)},
				Type:              dungeon.WallTypeDestructible,
				BlocksMovement:    true,
				BlocksLineOfSight: true,
			})
		} else {
			// Vertical wall with gap
			x := margin + rng.Float64()*(width-2*margin)
			gapStart := margin + rng.Float64()*(height-2*margin-gapSize)

			start = dungeon.Position{X: int(x), Y: int(margin)}
			end = dungeon.Position{X: int(x), Y: int(gapStart)}
			walls = append(walls, dungeon.WallSegment{
				ID:                fmt.Sprintf("choke_%d_a", i),
				Start:             start,
				End:               end,
				Type:              dungeon.WallTypeDestructible,
				BlocksMovement:    true,
				BlocksLineOfSight: true,
			})

			start = dungeon.Position{X: int(x), Y: int(gapStart + gapSize)}
			end = dungeon.Position{X: int(x), Y: int(height - margin)}
			walls = append(walls, dungeon.WallSegment{
				ID:                fmt.Sprintf("choke_%d_b", i),
				Start:             start,
				End:               end,
				Type:              dungeon.WallTypeDestructible,
				BlocksMovement:    true,
				BlocksLineOfSight: true,
			})
		}
	}

	return &PatternOutput{
		Walls:     walls,
		Obstacles: []dungeon.Obstacle{},
	}
}

// CentralFeaturePattern creates an obstacle cluster in room center
func CentralFeaturePattern(input *PatternInput) *PatternOutput {
	if input == nil || input.Shape == nil {
		return &PatternOutput{}
	}

	// #nosec G404 - Using math/rand for seeded, reproducible wall pattern generation
	rng := rand.New(rand.NewSource(input.Seed))

	var walls []dungeon.WallSegment
	centerX := float64(input.Shape.Width) / 2
	centerY := float64(input.Shape.Height) / 2

	// Create 3-5 walls arranged around center
	numWalls := 3 + rng.Intn(3)
	radius := 2.0 + rng.Float64()*2.0

	for i := 0; i < numWalls; i++ {
		angle := float64(i) * (2 * math.Pi / float64(numWalls))
		wallLength := 2.0 + rng.Float64()*2.0

		// Position on circle around center
		wx := centerX + radius*math.Cos(angle)
		wy := centerY + radius*math.Sin(angle)

		// Wall oriented tangent to circle
		perpAngle := angle + math.Pi/2
		dx := wallLength * math.Cos(perpAngle) / 2
		dy := wallLength * math.Sin(perpAngle) / 2

		walls = append(walls, dungeon.WallSegment{
			ID:                fmt.Sprintf("central_%d", i),
			Start:             dungeon.Position{X: int(wx - dx), Y: int(wy - dy)},
			End:               dungeon.Position{X: int(wx + dx), Y: int(wy + dy)},
			Type:              dungeon.WallTypeDestructible,
			BlocksMovement:    true,
			BlocksLineOfSight: true,
		})
	}

	return &PatternOutput{
		Walls:     walls,
		Obstacles: []dungeon.Obstacle{},
	}
}

// PerimeterCoverPattern creates alcoves and pillars along room edges
func PerimeterCoverPattern(input *PatternInput) *PatternOutput {
	if input == nil || input.Shape == nil {
		return &PatternOutput{}
	}

	// #nosec G404 - Using math/rand for seeded, reproducible wall pattern generation
	rng := rand.New(rand.NewSource(input.Seed))

	var walls []dungeon.WallSegment
	width := float64(input.Shape.Width)
	height := float64(input.Shape.Height)

	// Create 4-8 short walls along the perimeter
	numWalls := 4 + rng.Intn(5)
	margin := 2.0
	wallID := 0

	for i := 0; i < numWalls; i++ {
		// Pick a random edge (0=top, 1=right, 2=bottom, 3=left)
		edge := rng.Intn(4)
		wallLength := 2.0 + rng.Float64()*2.0

		var start, end dungeon.Position

		switch edge {
		case 0: // Top edge
			x := margin + rng.Float64()*(width-2*margin)
			start = dungeon.Position{X: int(x), Y: int(margin)}
			end = dungeon.Position{X: int(x), Y: int(margin + wallLength)}
		case 1: // Right edge
			y := margin + rng.Float64()*(height-2*margin)
			start = dungeon.Position{X: int(width - margin - wallLength), Y: int(y)}
			end = dungeon.Position{X: int(width - margin), Y: int(y)}
		case 2: // Bottom edge
			x := margin + rng.Float64()*(width-2*margin)
			start = dungeon.Position{X: int(x), Y: int(height - margin - wallLength)}
			end = dungeon.Position{X: int(x), Y: int(height - margin)}
		case 3: // Left edge
			y := margin + rng.Float64()*(height-2*margin)
			start = dungeon.Position{X: int(margin), Y: int(y)}
			end = dungeon.Position{X: int(margin + wallLength), Y: int(y)}
		}

		walls = append(walls, dungeon.WallSegment{
			ID:                fmt.Sprintf("perimeter_cover_%d", wallID),
			Start:             start,
			End:               end,
			Type:              dungeon.WallTypeDestructible,
			BlocksMovement:    true,
			BlocksLineOfSight: true,
		})
		wallID++
	}

	return &PatternOutput{
		Walls:     walls,
		Obstacles: []dungeon.Obstacle{},
	}
}

// PillarGridPattern creates evenly spaced pillars for sightline breaks
func PillarGridPattern(input *PatternInput) *PatternOutput {
	if input == nil || input.Shape == nil {
		return &PatternOutput{}
	}

	var walls []dungeon.WallSegment
	width := float64(input.Shape.Width)
	height := float64(input.Shape.Height)

	// Calculate grid spacing (aim for 2-3 pillars per dimension)
	margin := 4.0
	spacingX := (width - 2*margin) / 3
	spacingY := (height - 2*margin) / 3
	wallID := 0

	// Create a 2x2 grid of small pillar-like walls
	for row := 0; row < 2; row++ {
		for col := 0; col < 2; col++ {
			x := margin + spacingX + float64(col)*spacingX
			y := margin + spacingY + float64(row)*spacingY

			// Create a small cross-shaped pillar (2 perpendicular walls)
			pillarSize := 1.5

			// Horizontal segment
			walls = append(walls, dungeon.WallSegment{
				ID:                fmt.Sprintf("pillar_%d_h", wallID),
				Start:             dungeon.Position{X: int(x - pillarSize), Y: int(y)},
				End:               dungeon.Position{X: int(x + pillarSize), Y: int(y)},
				Type:              dungeon.WallTypeIndestructible,
				BlocksMovement:    true,
				BlocksLineOfSight: true,
			})

			// Vertical segment
			walls = append(walls, dungeon.WallSegment{
				ID:                fmt.Sprintf("pillar_%d_v", wallID),
				Start:             dungeon.Position{X: int(x), Y: int(y - pillarSize)},
				End:               dungeon.Position{X: int(x), Y: int(y + pillarSize)},
				Type:              dungeon.WallTypeIndestructible,
				BlocksMovement:    true,
				BlocksLineOfSight: true,
			})

			wallID++
		}
	}

	return &PatternOutput{
		Walls:     walls,
		Obstacles: []dungeon.Obstacle{},
	}
}
