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
