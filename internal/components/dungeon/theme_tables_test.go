package dungeon

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ThemeTablesTestSuite struct {
	suite.Suite
}

func TestThemeTablesTestSuite(t *testing.T) {
	suite.Run(t, new(ThemeTablesTestSuite))
}

func (s *ThemeTablesTestSuite) TestSelectPattern_ReturnsValidPattern() {
	tables := &ThemeTables{
		WallPatterns: map[RoomType][]WeightedPattern{
			RoomTypeChamber: {
				{Pattern: PatternCoverClusters, Weight: 50},
				{Pattern: PatternSparse, Weight: 30},
				{Pattern: PatternCentralFeature, Weight: 20},
			},
		},
	}

	// #nosec G404 - test code
	rng := rand.New(rand.NewSource(12345))
	pattern, err := tables.SelectPattern(RoomTypeChamber, rng)

	require.NoError(s.T(), err)
	validPatterns := []PatternType{PatternCoverClusters, PatternSparse, PatternCentralFeature}
	assert.Contains(s.T(), validPatterns, pattern)
}

func (s *ThemeTablesTestSuite) TestSelectPattern_UnknownRoomType() {
	tables := &ThemeTables{
		WallPatterns: map[RoomType][]WeightedPattern{
			RoomTypeChamber: {
				{Pattern: PatternSparse, Weight: 100},
			},
		},
	}

	// #nosec G404 - test code
	rng := rand.New(rand.NewSource(12345))
	pattern, err := tables.SelectPattern(RoomTypeBoss, rng)

	// Should return empty pattern for unknown room type
	require.NoError(s.T(), err)
	assert.Equal(s.T(), PatternEmpty, pattern)
}

func (s *ThemeTablesTestSuite) TestSelectPattern_EmptyPatterns() {
	tables := &ThemeTables{
		WallPatterns: map[RoomType][]WeightedPattern{
			RoomTypeChamber: {},
		},
	}

	// #nosec G404 - test code
	rng := rand.New(rand.NewSource(12345))
	pattern, err := tables.SelectPattern(RoomTypeChamber, rng)

	require.NoError(s.T(), err)
	assert.Equal(s.T(), PatternEmpty, pattern)
}

func (s *ThemeTablesTestSuite) TestSelectPattern_SinglePattern() {
	tables := &ThemeTables{
		WallPatterns: map[RoomType][]WeightedPattern{
			RoomTypeEntrance: {
				{Pattern: PatternSparse, Weight: 100},
			},
		},
	}

	// #nosec G404 - test code
	rng := rand.New(rand.NewSource(12345))
	pattern, err := tables.SelectPattern(RoomTypeEntrance, rng)

	require.NoError(s.T(), err)
	assert.Equal(s.T(), PatternSparse, pattern)
}

func (s *ThemeTablesTestSuite) TestSelectPattern_WeightedDistribution() {
	tables := &ThemeTables{
		WallPatterns: map[RoomType][]WeightedPattern{
			RoomTypeChamber: {
				{Pattern: PatternCoverClusters, Weight: 80},
				{Pattern: PatternSparse, Weight: 20},
			},
		},
	}

	// Run many selections to check distribution
	counts := make(map[PatternType]int)
	for i := int64(0); i < 1000; i++ {
		// #nosec G404 - test code
		rng := rand.New(rand.NewSource(i))
		pattern, _ := tables.SelectPattern(RoomTypeChamber, rng)
		counts[pattern]++
	}

	// CoverClusters should be selected more often (roughly 80%)
	assert.Greater(s.T(), counts[PatternCoverClusters], 700, "80% weight should result in >70% selection")
	assert.Less(s.T(), counts[PatternSparse], 300, "20% weight should result in <30% selection")
}

func (s *ThemeTablesTestSuite) TestSelectObstacle_ReturnsValidObstacle() {
	tables := &ThemeTables{
		Obstacles: []WeightedObstacle{
			{Obstacle: ObstacleTypePillar, Weight: 50},
			{Obstacle: ObstacleTypeSarcophagus, Weight: 50},
		},
	}

	// #nosec G404 - test code
	rng := rand.New(rand.NewSource(12345))
	obstacle := tables.SelectObstacle(rng)

	validObstacles := []ObstacleType{ObstacleTypePillar, ObstacleTypeSarcophagus}
	assert.Contains(s.T(), validObstacles, obstacle)
}

func (s *ThemeTablesTestSuite) TestSelectDensity_ReturnsValidDensity() {
	tables := &ThemeTables{
		Density: []WeightedDensity{
			{Density: DensityLow, Weight: 30},
			{Density: DensityMedium, Weight: 50},
			{Density: DensityHigh, Weight: 20},
		},
	}

	// #nosec G404 - test code
	rng := rand.New(rand.NewSource(12345))
	density := tables.SelectDensity(rng)

	assert.True(s.T(), density.Min >= 0.1 && density.Max <= 0.9)
}
