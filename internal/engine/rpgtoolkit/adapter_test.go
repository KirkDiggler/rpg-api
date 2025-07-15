package rpgtoolkit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"

	"github.com/KirkDiggler/rpg-toolkit/events"
)

type AdapterTestSuite struct {
	suite.Suite
}

func TestAdapterSuite(t *testing.T) {
	suite.Run(t, new(AdapterTestSuite))
}

func TestNewAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter, err := NewAdapter(nil)
		assert.Error(t, err)
		assert.Nil(t, adapter)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Contains(t, err.Error(), "config is required")
	})

	t.Run("missing event bus", func(t *testing.T) {
		cfg := &AdapterConfig{
			DiceRoller: nil, // Will also fail, but test event bus first
		}

		adapter, err := NewAdapter(cfg)
		assert.Error(t, err)
		assert.Nil(t, adapter)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Contains(t, err.Error(), "event bus is required")
	})

	t.Run("missing dice roller", func(t *testing.T) {
		cfg := &AdapterConfig{
			EventBus: &stubEventBus{}, // Simple stub for testing
		}

		adapter, err := NewAdapter(cfg)
		assert.Error(t, err)
		assert.Nil(t, adapter)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Contains(t, err.Error(), "dice roller is required")
	})

	t.Run("valid config", func(t *testing.T) {
		cfg := &AdapterConfig{
			EventBus:   &stubEventBus{},
			DiceRoller: &stubDiceRoller{},
		}

		adapter, err := NewAdapter(cfg)
		assert.NoError(t, err)
		assert.NotNil(t, adapter)
	})
}

// Simple stubs for testing validation logic
type stubEventBus struct{}
type stubDiceRoller struct{}

// Minimal implementation to satisfy events.EventBus interface
func (s *stubEventBus) Publish(_ context.Context, _ events.Event) error { return nil }
func (s *stubEventBus) Subscribe(_ string, _ events.Handler) string     { return "sub-id" }
func (s *stubEventBus) SubscribeFunc(_ string, _ int, _ events.HandlerFunc) string {
	return "sub-id"
}
func (s *stubEventBus) Unsubscribe(_ string) error { return nil }
func (s *stubEventBus) Clear(_ string)             {}
func (s *stubEventBus) ClearAll()                  {}

// Minimal implementation to satisfy dice.Roller interface
func (s *stubDiceRoller) Roll(_ int) (int, error)       { return 10, nil }
func (s *stubDiceRoller) RollN(_, _ int) ([]int, error) { return []int{10}, nil }

func TestCalculateAbilityModifier(t *testing.T) {
	// Create adapter with stubs for testing utility methods
	adapter, err := NewAdapter(&AdapterConfig{
		EventBus:   &stubEventBus{},
		DiceRoller: &stubDiceRoller{},
	})
	assert.NoError(t, err)

	testCases := []struct {
		name     string
		score    int32
		expected int32
	}{
		{"score 1", 1, -5},
		{"score 8", 8, -1},
		{"score 10", 10, 0},
		{"score 11", 11, 0},
		{"score 12", 12, 1},
		{"score 15", 15, 2},
		{"score 20", 20, 5},
		{"score 30", 30, 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := adapter.CalculateAbilityModifier(tc.score)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCalculateProficiencyBonus(t *testing.T) {
	// Create adapter with stubs for testing utility methods
	adapter, err := NewAdapter(&AdapterConfig{
		EventBus:   &stubEventBus{},
		DiceRoller: &stubDiceRoller{},
	})
	assert.NoError(t, err)

	testCases := []struct {
		name     string
		level    int32
		expected int32
	}{
		{"level 0", 0, 0},
		{"level 1", 1, 2},
		{"level 4", 4, 2},
		{"level 5", 5, 3},
		{"level 8", 8, 3},
		{"level 9", 9, 4},
		{"level 12", 12, 4},
		{"level 13", 13, 5},
		{"level 16", 16, 5},
		{"level 17", 17, 6},
		{"level 20", 20, 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := adapter.CalculateProficiencyBonus(tc.level)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Test interface compliance
func TestAdapterImplementsEngine(t *testing.T) {
	adapter, err := NewAdapter(&AdapterConfig{
		EventBus:   &stubEventBus{},
		DiceRoller: &stubDiceRoller{},
	})
	assert.NoError(t, err)

	// Verify adapter implements engine.Engine interface
	var _ engine.Engine = adapter
}

func TestValidateAbilityScores(t *testing.T) {
	// Create adapter with stubs for testing
	adapter, err := NewAdapter(&AdapterConfig{
		EventBus:   &stubEventBus{},
		DiceRoller: &stubDiceRoller{},
	})
	assert.NoError(t, err)

	ctx := context.Background()

	t.Run("nil input", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, nil)
		assert.Error(t, err)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Nil(t, result)
	})

	t.Run("nil ability scores", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodStandardArray,
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "ability_scores", result.Errors[0].Field)
		assert.Equal(t, "REQUIRED", result.Errors[0].Code)
	})

	t.Run("invalid method", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method:        engine.AbilityScoreMethod("invalid_method"),
			AbilityScores: &dnd5e.AbilityScores{},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "method", result.Errors[0].Field)
		assert.Equal(t, "INVALID_METHOD", result.Errors[0].Code)
	})
}

func TestValidateStandardArray(t *testing.T) {
	// Create adapter with stubs for testing
	adapter, err := NewAdapter(&AdapterConfig{
		EventBus:   &stubEventBus{},
		DiceRoller: &stubDiceRoller{},
	})
	assert.NoError(t, err)

	ctx := context.Background()

	t.Run("valid standard array", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodStandardArray,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     15,
				Dexterity:    14,
				Constitution: 13,
				Intelligence: 12,
				Wisdom:       10,
				Charisma:     8,
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsValid)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.Warnings)
	})

	t.Run("valid standard array different order", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodStandardArray,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     8,
				Dexterity:    15,
				Constitution: 14,
				Intelligence: 10,
				Wisdom:       12,
				Charisma:     13,
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsValid)
		assert.Empty(t, result.Errors)
	})

	t.Run("invalid standard array", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodStandardArray,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     16, // Not in standard array
				Dexterity:    14,
				Constitution: 13,
				Intelligence: 12,
				Wisdom:       10,
				Charisma:     8,
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "ability_scores", result.Errors[0].Field)
		assert.Equal(t, "INVALID_STANDARD_ARRAY", result.Errors[0].Code)
	})
}

func TestValidatePointBuy(t *testing.T) {
	// Create adapter with stubs for testing
	adapter, err := NewAdapter(&AdapterConfig{
		EventBus:   &stubEventBus{},
		DiceRoller: &stubDiceRoller{},
	})
	assert.NoError(t, err)

	ctx := context.Background()

	t.Run("valid point buy exactly 27 points", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodPointBuy,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     15, // 9 points
				Dexterity:    15, // 9 points
				Constitution: 15, // 9 points
				Intelligence: 8,  // 0 points
				Wisdom:       8,  // 0 points
				Charisma:     8,  // 0 points
			}, // Total: 27 points
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsValid)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.Warnings)
	})

	t.Run("valid point buy under 27 points", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodPointBuy,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     14, // 7 points
				Dexterity:    14, // 7 points
				Constitution: 13, // 5 points
				Intelligence: 12, // 4 points
				Wisdom:       10, // 2 points
				Charisma:     10, // 2 points
			}, // Total: 27 points
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsValid)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.Warnings)
	})

	t.Run("point buy with unspent points", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodPointBuy,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     13, // 5 points
				Dexterity:    13, // 5 points
				Constitution: 13, // 5 points
				Intelligence: 10, // 2 points
				Wisdom:       10, // 2 points
				Charisma:     10, // 2 points
			}, // Total: 21 points
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsValid)
		assert.Empty(t, result.Errors)
		assert.Len(t, result.Warnings, 1)
		assert.Equal(t, "ability_scores", result.Warnings[0].Field)
		assert.Equal(t, "UNSPENT_POINTS", result.Warnings[0].Code)
	})

	t.Run("point buy exceeds 27 points", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodPointBuy,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     15, // 9 points
				Dexterity:    15, // 9 points
				Constitution: 15, // 9 points
				Intelligence: 15, // 9 points
				Wisdom:       8,  // 0 points
				Charisma:     8,  // 0 points
			}, // Total: 36 points
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "ability_scores", result.Errors[0].Field)
		assert.Equal(t, "POINT_BUY_EXCEEDED", result.Errors[0].Code)
	})

	t.Run("point buy score below minimum", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodPointBuy,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     7, // Below minimum
				Dexterity:    14,
				Constitution: 13,
				Intelligence: 12,
				Wisdom:       10,
				Charisma:     10,
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "strength", result.Errors[0].Field)
		assert.Equal(t, "INVALID_POINT_BUY_RANGE", result.Errors[0].Code)
	})

	t.Run("point buy score above maximum", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodPointBuy,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     16, // Above maximum
				Dexterity:    14,
				Constitution: 13,
				Intelligence: 12,
				Wisdom:       10,
				Charisma:     10,
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "strength", result.Errors[0].Field)
		assert.Equal(t, "INVALID_POINT_BUY_RANGE", result.Errors[0].Code)
	})

	t.Run("point buy multiple errors", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodPointBuy,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     7,  // Below minimum
				Dexterity:    16, // Above maximum
				Constitution: 15,
				Intelligence: 15,
				Wisdom:       15,
				Charisma:     15,
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 3) // 2 range errors + 1 exceeded error
	})
}

func TestValidateManualScores(t *testing.T) {
	// Create adapter with stubs for testing
	adapter, err := NewAdapter(&AdapterConfig{
		EventBus:   &stubEventBus{},
		DiceRoller: &stubDiceRoller{},
	})
	assert.NoError(t, err)

	ctx := context.Background()

	t.Run("valid manual scores", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodManual,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     18,
				Dexterity:    16,
				Constitution: 14,
				Intelligence: 12,
				Wisdom:       10,
				Charisma:     8,
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsValid)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.Warnings)
	})

	t.Run("manual scores at minimum", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodManual,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     3,
				Dexterity:    3,
				Constitution: 3,
				Intelligence: 3,
				Wisdom:       3,
				Charisma:     3,
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsValid)
		assert.Empty(t, result.Errors)
	})

	t.Run("manual scores at maximum", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodManual,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     18,
				Dexterity:    18,
				Constitution: 18,
				Intelligence: 18,
				Wisdom:       18,
				Charisma:     18,
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsValid)
		assert.Empty(t, result.Errors)
	})

	t.Run("manual score below minimum", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodManual,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     2, // Below minimum
				Dexterity:    10,
				Constitution: 10,
				Intelligence: 10,
				Wisdom:       10,
				Charisma:     10,
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "strength", result.Errors[0].Field)
		assert.Equal(t, "INVALID_ABILITY_SCORE_RANGE", result.Errors[0].Code)
	})

	t.Run("manual score above maximum", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodManual,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     10,
				Dexterity:    10,
				Constitution: 10,
				Intelligence: 10,
				Wisdom:       10,
				Charisma:     19, // Above maximum
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "charisma", result.Errors[0].Field)
		assert.Equal(t, "INVALID_ABILITY_SCORE_RANGE", result.Errors[0].Code)
	})

	t.Run("manual multiple invalid scores", func(t *testing.T) {
		result, err := adapter.ValidateAbilityScores(ctx, &engine.ValidateAbilityScoresInput{
			Method: engine.AbilityScoreMethodManual,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     2,  // Below minimum
				Dexterity:    19, // Above maximum
				Constitution: 0,  // Below minimum
				Intelligence: 10,
				Wisdom:       10,
				Charisma:     25, // Above maximum
			},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 4)
	})
}
