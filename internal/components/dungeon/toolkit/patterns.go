package toolkit

import (
	"fmt"
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
