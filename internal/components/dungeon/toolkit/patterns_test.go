package toolkit

import (
	"testing"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PatternRegistryTestSuite struct {
	suite.Suite
	registry *PatternRegistry
}

func TestPatternRegistryTestSuite(t *testing.T) {
	suite.Run(t, new(PatternRegistryTestSuite))
}

func (s *PatternRegistryTestSuite) SetupTest() {
	s.registry = NewPatternRegistry()
}

func (s *PatternRegistryTestSuite) TestGetPattern_Empty() {
	pattern, exists := s.registry.GetPattern(dungeon.PatternEmpty)
	require.True(s.T(), exists, "empty pattern should exist")
	require.NotNil(s.T(), pattern)

	result := pattern(&PatternInput{
		Shape: &dungeon.Shape{Width: 10, Height: 10},
	})

	assert.Empty(s.T(), result.Walls, "empty pattern should produce no walls")
}

func (s *PatternRegistryTestSuite) TestGetPattern_Sparse() {
	pattern, exists := s.registry.GetPattern(dungeon.PatternSparse)
	require.True(s.T(), exists, "sparse pattern should exist")
	require.NotNil(s.T(), pattern)

	result := pattern(&PatternInput{
		Shape:   &dungeon.Shape{Width: 15, Height: 15},
		Density: dungeon.DensityLow,
		Seed:    12345,
	})

	assert.LessOrEqual(s.T(), len(result.Walls), 3, "sparse pattern should have few walls")
}

func (s *PatternRegistryTestSuite) TestGetPattern_UnknownPattern() {
	pattern, exists := s.registry.GetPattern("nonexistent")
	assert.False(s.T(), exists, "unknown pattern should not exist")
	assert.Nil(s.T(), pattern)
}

func (s *PatternRegistryTestSuite) TestSparsePattern_Deterministic() {
	pattern, _ := s.registry.GetPattern(dungeon.PatternSparse)

	input := &PatternInput{
		Shape:   &dungeon.Shape{Width: 15, Height: 15},
		Density: dungeon.DensityMedium,
		Seed:    42,
	}

	result1 := pattern(input)
	result2 := pattern(input)

	// Same seed should produce same results
	assert.Equal(s.T(), len(result1.Walls), len(result2.Walls), "same seed should produce same wall count")
	for i := range result1.Walls {
		assert.Equal(s.T(), result1.Walls[i].Start, result2.Walls[i].Start, "same seed should produce same positions")
	}
}

func (s *PatternRegistryTestSuite) TestSparsePattern_NilInput() {
	pattern, _ := s.registry.GetPattern(dungeon.PatternSparse)

	result := pattern(nil)

	assert.Empty(s.T(), result.Walls)
}

func (s *PatternRegistryTestSuite) TestSparsePattern_NilShape() {
	pattern, _ := s.registry.GetPattern(dungeon.PatternSparse)

	result := pattern(&PatternInput{
		Shape: nil,
		Seed:  123,
	})

	assert.Empty(s.T(), result.Walls)
}

func (s *PatternRegistryTestSuite) TestSparsePattern_WallProperties() {
	pattern, _ := s.registry.GetPattern(dungeon.PatternSparse)

	result := pattern(&PatternInput{
		Shape:   &dungeon.Shape{Width: 20, Height: 20},
		Density: dungeon.DensityMedium,
		Seed:    123,
	})

	require.NotEmpty(s.T(), result.Walls)

	for _, wall := range result.Walls {
		assert.Equal(s.T(), dungeon.WallTypeDestructible, wall.Type, "internal walls should be destructible")
		assert.True(s.T(), wall.BlocksMovement)
		assert.True(s.T(), wall.BlocksLineOfSight)
		assert.NotEmpty(s.T(), wall.ID)
	}
}

func (s *PatternRegistryTestSuite) TestRegisterPattern_Custom() {
	customPattern := func(input *PatternInput) *PatternOutput {
		return &PatternOutput{
			Walls: []dungeon.WallSegment{
				{ID: "custom_0", Type: dungeon.WallTypeDestructible},
			},
		}
	}

	s.registry.RegisterPattern("custom_test", customPattern)

	pattern, exists := s.registry.GetPattern("custom_test")
	require.True(s.T(), exists, "custom pattern should exist after registration")

	result := pattern(&PatternInput{})
	assert.Len(s.T(), result.Walls, 1)
	assert.Equal(s.T(), "custom_0", result.Walls[0].ID)
}
