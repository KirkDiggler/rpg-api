package dungeon

import (
	"math/rand"
)

// ThemeTables contains selectables for theme-based generation
type ThemeTables struct {
	// WallPatterns maps room type to weighted pattern selection
	WallPatterns map[RoomType][]WeightedPattern
	// Obstacles contains weighted obstacle type selection
	Obstacles []WeightedObstacle
	// Density contains weighted density range selection
	Density []WeightedDensity
}

// WeightedPattern pairs a pattern type with its selection weight
type WeightedPattern struct {
	Pattern PatternType
	Weight  int
}

// WeightedObstacle pairs an obstacle type with its selection weight
type WeightedObstacle struct {
	Obstacle ObstacleType
	Weight   int
}

// WeightedDensity pairs a density range with its selection weight
type WeightedDensity struct {
	Density DensityRange
	Weight  int
}

// SelectPattern randomly selects a pattern for the given room type
func (t *ThemeTables) SelectPattern(roomType RoomType, rng *rand.Rand) (PatternType, error) {
	patterns, exists := t.WallPatterns[roomType]
	if !exists || len(patterns) == 0 {
		return PatternEmpty, nil
	}

	totalWeight := 0
	for _, wp := range patterns {
		totalWeight += wp.Weight
	}

	if totalWeight == 0 {
		return PatternEmpty, nil
	}

	roll := rng.Intn(totalWeight)
	cumulative := 0
	for _, wp := range patterns {
		cumulative += wp.Weight
		if roll < cumulative {
			return wp.Pattern, nil
		}
	}

	return patterns[0].Pattern, nil
}

// SelectObstacle randomly selects an obstacle type
func (t *ThemeTables) SelectObstacle(rng *rand.Rand) ObstacleType {
	if len(t.Obstacles) == 0 {
		return ObstacleTypePillar // default
	}

	totalWeight := 0
	for _, wo := range t.Obstacles {
		totalWeight += wo.Weight
	}

	if totalWeight == 0 {
		return t.Obstacles[0].Obstacle
	}

	roll := rng.Intn(totalWeight)
	cumulative := 0
	for _, wo := range t.Obstacles {
		cumulative += wo.Weight
		if roll < cumulative {
			return wo.Obstacle
		}
	}

	return t.Obstacles[0].Obstacle
}

// SelectDensity randomly selects a density range
func (t *ThemeTables) SelectDensity(rng *rand.Rand) DensityRange {
	if len(t.Density) == 0 {
		return DensityMedium // default
	}

	totalWeight := 0
	for _, wd := range t.Density {
		totalWeight += wd.Weight
	}

	if totalWeight == 0 {
		return t.Density[0].Density
	}

	roll := rng.Intn(totalWeight)
	cumulative := 0
	for _, wd := range t.Density {
		cumulative += wd.Weight
		if roll < cumulative {
			return wd.Density
		}
	}

	return t.Density[0].Density
}
