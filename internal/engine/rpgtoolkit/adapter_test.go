package rpgtoolkit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
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

	t.Run("missing external client", func(t *testing.T) {
		cfg := &AdapterConfig{
			EventBus:   &stubEventBus{},
			DiceRoller: &stubDiceRoller{},
		}

		adapter, err := NewAdapter(cfg)
		assert.Error(t, err)
		assert.Nil(t, adapter)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Contains(t, err.Error(), "external client is required")
	})

	t.Run("valid config", func(t *testing.T) {
		cfg := &AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: &stubExternalClient{},
		}

		adapter, err := NewAdapter(cfg)
		assert.NoError(t, err)
		assert.NotNil(t, adapter)
	})
}

// Simple stubs for testing validation logic
type stubEventBus struct{}
type stubDiceRoller struct{}
type stubExternalClient struct{}

// testExternalClient is a more configurable stub for specific test scenarios
type testExternalClient struct {
	classData       *external.ClassData
	classError      error
	backgroundData  *external.BackgroundData
	backgroundError error
	raceData        *external.RaceData
	raceError       error
}

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

// Minimal implementation to satisfy external.Client interface
func (s *stubExternalClient) GetRaceData(_ context.Context, _ string) (*external.RaceData, error) {
	return nil, errors.NotFound("race not found")
}
func (s *stubExternalClient) GetClassData(_ context.Context, _ string) (*external.ClassData, error) {
	return nil, errors.NotFound("class not found")
}
func (s *stubExternalClient) GetBackgroundData(_ context.Context, _ string) (*external.BackgroundData, error) {
	return nil, errors.NotFound("background not found")
}
func (s *stubExternalClient) GetSpellData(_ context.Context, _ string) (*external.SpellData, error) {
	return nil, errors.NotFound("spell not found")
}
func (s *stubExternalClient) ListAvailableRaces(_ context.Context) ([]*external.RaceData, error) {
	return []*external.RaceData{}, nil
}
func (s *stubExternalClient) ListAvailableClasses(_ context.Context) ([]*external.ClassData, error) {
	return []*external.ClassData{}, nil
}
func (s *stubExternalClient) ListAvailableBackgrounds(_ context.Context) ([]*external.BackgroundData, error) {
	return []*external.BackgroundData{}, nil
}
func (s *stubExternalClient) ListAvailableSpells(
	_ context.Context, _ *external.ListSpellsInput,
) ([]*external.SpellData, error) {
	return []*external.SpellData{}, nil
}

func (s *stubExternalClient) ListAvailableEquipment(_ context.Context) ([]*external.EquipmentData, error) {
	return []*external.EquipmentData{}, nil
}

func (s *stubExternalClient) ListEquipmentByCategory(_ context.Context, _ string) ([]*external.EquipmentData, error) {
	return []*external.EquipmentData{}, nil
}

func (s *stubExternalClient) GetEquipmentData(_ context.Context, _ string) (*external.EquipmentData, error) {
	return nil, errors.NotFound("equipment not found")
}

// testExternalClient implementations
func (c *testExternalClient) GetRaceData(_ context.Context, _ string) (*external.RaceData, error) {
	if c.raceError != nil {
		return nil, c.raceError
	}
	return c.raceData, nil
}

func (c *testExternalClient) GetClassData(_ context.Context, _ string) (*external.ClassData, error) {
	if c.classError != nil {
		return nil, c.classError
	}
	return c.classData, nil
}

func (c *testExternalClient) GetBackgroundData(_ context.Context, _ string) (*external.BackgroundData, error) {
	if c.backgroundError != nil {
		return nil, c.backgroundError
	}
	return c.backgroundData, nil
}

func (c *testExternalClient) GetSpellData(_ context.Context, _ string) (*external.SpellData, error) {
	return nil, errors.NotFound("spell not found")
}

func (c *testExternalClient) ListAvailableRaces(_ context.Context) ([]*external.RaceData, error) {
	return []*external.RaceData{}, nil
}

func (c *testExternalClient) ListAvailableClasses(_ context.Context) ([]*external.ClassData, error) {
	return []*external.ClassData{}, nil
}

func (c *testExternalClient) ListAvailableBackgrounds(_ context.Context) ([]*external.BackgroundData, error) {
	return []*external.BackgroundData{}, nil
}
func (c *testExternalClient) ListAvailableSpells(
	_ context.Context, _ *external.ListSpellsInput,
) ([]*external.SpellData, error) {
	return []*external.SpellData{}, nil
}

func (c *testExternalClient) ListAvailableEquipment(_ context.Context) ([]*external.EquipmentData, error) {
	return []*external.EquipmentData{}, nil
}

func (c *testExternalClient) ListEquipmentByCategory(_ context.Context, _ string) ([]*external.EquipmentData, error) {
	return []*external.EquipmentData{}, nil
}

func (c *testExternalClient) GetEquipmentData(_ context.Context, _ string) (*external.EquipmentData, error) {
	return nil, errors.NotFound("equipment not found")
}

// createTestAdapter creates an adapter with stubs for testing
func createTestAdapter(t *testing.T) *Adapter {
	adapter, err := NewAdapter(&AdapterConfig{
		EventBus:       &stubEventBus{},
		DiceRoller:     &stubDiceRoller{},
		ExternalClient: &stubExternalClient{},
	})
	assert.NoError(t, err)
	return adapter
}

// createTestAdapterWithClient creates an adapter with a specific external client for testing
func createTestAdapterWithClient(t *testing.T, client external.Client) *Adapter {
	adapter, err := NewAdapter(&AdapterConfig{
		EventBus:       &stubEventBus{},
		DiceRoller:     &stubDiceRoller{},
		ExternalClient: client,
	})
	assert.NoError(t, err)
	return adapter
}

//nolint:dupl // Race and class validation tests have similar structure by design
func TestValidateRaceChoice(t *testing.T) {
	adapter := createTestAdapter(t)
	ctx := context.Background()

	t.Run("nil input", func(t *testing.T) {
		result, err := adapter.ValidateRaceChoice(ctx, nil)
		assert.Error(t, err)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Nil(t, result)
	})

	t.Run("empty race ID", func(t *testing.T) {
		result, err := adapter.ValidateRaceChoice(ctx, &engine.ValidateRaceChoiceInput{
			RaceID: "",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "race_id", result.Errors[0].Field)
		assert.Equal(t, "REQUIRED", result.Errors[0].Code)
	})

	t.Run("external client error", func(t *testing.T) {
		// The stub external client returns an error (following "result or error, never neither" rule)
		result, err := adapter.ValidateRaceChoice(ctx, &engine.ValidateRaceChoiceInput{
			RaceID: "invalid-race",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "race_id", result.Errors[0].Field)
		assert.Equal(t, "INVALID_RACE", result.Errors[0].Code)
	})

	// Note: When we implement comprehensive tests with mocks, we'll test:
	// - Valid race without subrace (e.g., Human)
	// - Valid race with valid subrace (e.g., High Elf)
	// - Valid race with invalid subrace
	// - Proper trait and ability bonus aggregation
}

//nolint:dupl // Race and class validation tests have similar structure by design
func TestValidateClassChoice(t *testing.T) {
	adapter := createTestAdapter(t)
	ctx := context.Background()

	t.Run("nil input", func(t *testing.T) {
		result, err := adapter.ValidateClassChoice(ctx, nil)
		assert.Error(t, err)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Nil(t, result)
	})

	t.Run("empty class ID", func(t *testing.T) {
		result, err := adapter.ValidateClassChoice(ctx, &engine.ValidateClassChoiceInput{
			ClassID: "",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "class_id", result.Errors[0].Field)
		assert.Equal(t, "REQUIRED", result.Errors[0].Code)
	})

	t.Run("external client error", func(t *testing.T) {
		// The stub external client returns an error (following "result or error, never neither" rule)
		result, err := adapter.ValidateClassChoice(ctx, &engine.ValidateClassChoiceInput{
			ClassID: "invalid-class",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "class_id", result.Errors[0].Field)
		assert.Equal(t, "INVALID_CLASS", result.Errors[0].Code)
	})

	// Note: When we implement comprehensive tests with mocks, we'll test:
	// - Valid class (e.g., Fighter)
	// - Class with ability score prerequisites for multiclassing
	// - Proper hit dice, saving throws, and skill data return
}

func TestCalculateAbilityModifier(t *testing.T) {
	adapter := createTestAdapter(t)

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
	adapter := createTestAdapter(t)

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
	adapter := createTestAdapter(t)

	// Verify adapter implements engine.Engine interface
	var _ engine.Engine = adapter
}

func TestValidateAbilityScores(t *testing.T) {
	adapter := createTestAdapter(t)

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
	adapter := createTestAdapter(t)

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
	adapter := createTestAdapter(t)

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
	adapter := createTestAdapter(t)

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

func TestValidateSkillChoices(t *testing.T) {
	ctx := context.Background()

	t.Run("nil input", func(t *testing.T) {
		adapter := createTestAdapter(t)
		result, err := adapter.ValidateSkillChoices(ctx, nil)
		assert.Error(t, err)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Nil(t, result)
	})

	t.Run("empty class ID", func(t *testing.T) {
		adapter := createTestAdapter(t)
		result, err := adapter.ValidateSkillChoices(ctx, &engine.ValidateSkillChoicesInput{
			ClassID: "",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "class_id", result.Errors[0].Field)
		assert.Equal(t, "REQUIRED", result.Errors[0].Code)
	})

	t.Run("invalid class ID", func(t *testing.T) {
		testClient := &testExternalClient{
			classError: errors.NotFound("class not found"),
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.ValidateSkillChoices(ctx, &engine.ValidateSkillChoicesInput{
			ClassID: "invalid-class",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "class_id", result.Errors[0].Field)
		assert.Equal(t, "INVALID_CLASS", result.Errors[0].Code)
	})

	t.Run("valid skill selection", func(t *testing.T) {
		testClient := &testExternalClient{
			classData: &external.ClassData{
				ID:              "fighter",
				Name:            "Fighter",
				SkillsCount:     2,
				AvailableSkills: []string{"athletics", "intimidation", "survival", "perception"},
			},
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.ValidateSkillChoices(ctx, &engine.ValidateSkillChoicesInput{
			ClassID:          "fighter",
			SelectedSkillIDs: []string{"athletics", "intimidation"},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsValid)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.Warnings)
	})

	t.Run("too few skills selected", func(t *testing.T) {
		testClient := &testExternalClient{
			classData: &external.ClassData{
				ID:              "fighter",
				Name:            "Fighter",
				SkillsCount:     2,
				AvailableSkills: []string{"athletics", "intimidation", "survival", "perception"},
			},
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.ValidateSkillChoices(ctx, &engine.ValidateSkillChoicesInput{
			ClassID:          "fighter",
			SelectedSkillIDs: []string{"athletics"},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "selected_skills", result.Errors[0].Field)
		assert.Equal(t, "INCORRECT_SKILL_COUNT", result.Errors[0].Code)
		assert.Contains(t, result.Errors[0].Message, "Must select exactly 2 skills")
	})

	t.Run("too many skills selected", func(t *testing.T) {
		testClient := &testExternalClient{
			classData: &external.ClassData{
				ID:              "fighter",
				Name:            "Fighter",
				SkillsCount:     2,
				AvailableSkills: []string{"athletics", "intimidation", "survival", "perception"},
			},
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.ValidateSkillChoices(ctx, &engine.ValidateSkillChoicesInput{
			ClassID:          "fighter",
			SelectedSkillIDs: []string{"athletics", "intimidation", "survival"},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "selected_skills", result.Errors[0].Field)
		assert.Equal(t, "INCORRECT_SKILL_COUNT", result.Errors[0].Code)
	})

	//nolint:dupl // Similar test structure is intentional for different validation scenarios
	t.Run("duplicate skill selection", func(t *testing.T) {
		testClient := &testExternalClient{
			classData: &external.ClassData{
				ID:              "fighter",
				Name:            "Fighter",
				SkillsCount:     2,
				AvailableSkills: []string{"athletics", "intimidation", "survival", "perception"},
			},
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.ValidateSkillChoices(ctx, &engine.ValidateSkillChoicesInput{
			ClassID:          "fighter",
			SelectedSkillIDs: []string{"athletics", "athletics"},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 2) // duplicate error + incorrect count
		duplicateFound := false
		for _, err := range result.Errors {
			if err.Code == "DUPLICATE_SKILL" {
				duplicateFound = true
				assert.Equal(t, "selected_skills", err.Field)
				assert.Contains(t, err.Message, "Duplicate skill selection")
			}
		}
		assert.True(t, duplicateFound, "Expected to find DUPLICATE_SKILL error")
	})

	//nolint:dupl // Similar test structure is intentional for different validation scenarios
	t.Run("invalid skill for class", func(t *testing.T) {
		testClient := &testExternalClient{
			classData: &external.ClassData{
				ID:              "fighter",
				Name:            "Fighter",
				SkillsCount:     2,
				AvailableSkills: []string{"athletics", "intimidation", "survival", "perception"},
			},
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.ValidateSkillChoices(ctx, &engine.ValidateSkillChoicesInput{
			ClassID:          "fighter",
			SelectedSkillIDs: []string{"athletics", "arcana"}, // arcana not available for fighter
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 2) // invalid skill + incorrect count
		invalidFound := false
		for _, err := range result.Errors {
			if err.Code == "INVALID_SKILL_CHOICE" {
				invalidFound = true
				assert.Equal(t, "selected_skills", err.Field)
				assert.Contains(t, err.Message, "arcana")
			}
		}
		assert.True(t, invalidFound, "Expected to find INVALID_SKILL_CHOICE error")
	})

	t.Run("skill overlap warning with background", func(t *testing.T) {
		testClient := &testExternalClient{
			classData: &external.ClassData{
				ID:              "fighter",
				Name:            "Fighter",
				SkillsCount:     2,
				AvailableSkills: []string{"athletics", "intimidation", "survival", "perception"},
			},
			backgroundData: &external.BackgroundData{
				ID:                 "soldier",
				Name:               "Soldier",
				SkillProficiencies: []string{"athletics", "intimidation"},
			},
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.ValidateSkillChoices(ctx, &engine.ValidateSkillChoicesInput{
			ClassID:          "fighter",
			BackgroundID:     "soldier",
			SelectedSkillIDs: []string{"athletics", "survival"}, // athletics overlaps with background
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.IsValid)
		assert.Empty(t, result.Errors)
		assert.Len(t, result.Warnings, 1)
		assert.Equal(t, "selected_skills", result.Warnings[0].Field)
		assert.Equal(t, "SKILL_OVERLAP", result.Warnings[0].Code)
		assert.Contains(t, result.Warnings[0].Message, "athletics")
		assert.Contains(t, result.Warnings[0].Message, "maximize proficiencies")
	})

	t.Run("selecting background skill as class choice", func(t *testing.T) {
		testClient := &testExternalClient{
			classData: &external.ClassData{
				ID:              "fighter",
				Name:            "Fighter",
				SkillsCount:     2,
				AvailableSkills: []string{"athletics", "survival", "perception"},
			},
			backgroundData: &external.BackgroundData{
				ID:                 "soldier",
				Name:               "Soldier",
				SkillProficiencies: []string{"intimidation"},
			},
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.ValidateSkillChoices(ctx, &engine.ValidateSkillChoicesInput{
			ClassID:          "fighter",
			BackgroundID:     "soldier",
			SelectedSkillIDs: []string{"athletics", "intimidation"}, // intimidation is from background
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		backgroundSkillFound := false
		for _, err := range result.Errors {
			if err.Code == "BACKGROUND_SKILL_NOT_CHOICE" {
				backgroundSkillFound = true
				assert.Equal(t, "selected_skills", err.Field)
				assert.Contains(t, err.Message, "intimidation")
				assert.Contains(t, err.Message, "automatically granted by background")
			}
		}
		assert.True(t, backgroundSkillFound, "Expected to find BACKGROUND_SKILL_NOT_CHOICE error")
	})

	t.Run("invalid background ID", func(t *testing.T) {
		testClient := &testExternalClient{
			classData: &external.ClassData{
				ID:              "fighter",
				Name:            "Fighter",
				SkillsCount:     2,
				AvailableSkills: []string{"athletics", "intimidation", "survival", "perception"},
			},
			backgroundError: errors.NotFound("background not found"),
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.ValidateSkillChoices(ctx, &engine.ValidateSkillChoicesInput{
			ClassID:          "fighter",
			BackgroundID:     "invalid-background",
			SelectedSkillIDs: []string{"athletics", "intimidation"},
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.False(t, result.IsValid)
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, "background_id", result.Errors[0].Field)
		assert.Equal(t, "INVALID_BACKGROUND", result.Errors[0].Code)
	})
}

func TestGetAvailableSkills(t *testing.T) {
	ctx := context.Background()

	t.Run("nil input", func(t *testing.T) {
		adapter := createTestAdapter(t)
		result, err := adapter.GetAvailableSkills(ctx, nil)
		assert.Error(t, err)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Nil(t, result)
	})

	t.Run("empty input returns empty skills", func(t *testing.T) {
		adapter := createTestAdapter(t)
		result, err := adapter.GetAvailableSkills(ctx, &engine.GetAvailableSkillsInput{})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.ClassSkills)
		assert.Empty(t, result.BackgroundSkills)
	})

	t.Run("class skills only", func(t *testing.T) {
		testClient := &testExternalClient{
			classData: &external.ClassData{
				ID:              "fighter",
				Name:            "Fighter",
				SkillsCount:     2,
				AvailableSkills: []string{"athletics", "intimidation", "survival"},
			},
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.GetAvailableSkills(ctx, &engine.GetAvailableSkillsInput{
			ClassID: "fighter",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.ClassSkills, 3)
		assert.Empty(t, result.BackgroundSkills)

		// Check first skill details
		assert.Equal(t, "athletics", result.ClassSkills[0].SkillID)
		assert.Equal(t, "Athletics", result.ClassSkills[0].SkillName)
		assert.Equal(t, "strength", result.ClassSkills[0].Ability)
		assert.Contains(t, result.ClassSkills[0].Description, "Athletics")
	})

	t.Run("background skills only", func(t *testing.T) {
		testClient := &testExternalClient{
			backgroundData: &external.BackgroundData{
				ID:                 "soldier",
				Name:               "Soldier",
				SkillProficiencies: []string{"athletics", "intimidation"},
			},
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.GetAvailableSkills(ctx, &engine.GetAvailableSkillsInput{
			BackgroundID: "soldier",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.ClassSkills)
		assert.Len(t, result.BackgroundSkills, 2)

		// Check background skill details
		assert.Equal(t, "athletics", result.BackgroundSkills[0].SkillID)
		assert.Equal(t, "Athletics", result.BackgroundSkills[0].SkillName)
		assert.Contains(t, result.BackgroundSkills[0].Description, "from background")
	})

	t.Run("both class and background skills", func(t *testing.T) {
		testClient := &testExternalClient{
			classData: &external.ClassData{
				ID:              "fighter",
				Name:            "Fighter",
				SkillsCount:     2,
				AvailableSkills: []string{"athletics", "survival"},
			},
			backgroundData: &external.BackgroundData{
				ID:                 "soldier",
				Name:               "Soldier",
				SkillProficiencies: []string{"intimidation"},
			},
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.GetAvailableSkills(ctx, &engine.GetAvailableSkillsInput{
			ClassID:      "fighter",
			BackgroundID: "soldier",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.ClassSkills, 2)
		assert.Len(t, result.BackgroundSkills, 1)
	})

	t.Run("invalid class ID returns empty skills", func(t *testing.T) {
		testClient := &testExternalClient{
			classError: errors.NotFound("class not found"),
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.GetAvailableSkills(ctx, &engine.GetAvailableSkillsInput{
			ClassID: "invalid-class",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.ClassSkills)
		assert.Empty(t, result.BackgroundSkills)
	})

	t.Run("invalid background ID returns partial results", func(t *testing.T) {
		testClient := &testExternalClient{
			classData: &external.ClassData{
				ID:              "fighter",
				Name:            "Fighter",
				SkillsCount:     2,
				AvailableSkills: []string{"athletics"},
			},
			backgroundError: errors.NotFound("background not found"),
		}
		adapter, err := NewAdapter(&AdapterConfig{
			EventBus:       &stubEventBus{},
			DiceRoller:     &stubDiceRoller{},
			ExternalClient: testClient,
		})
		assert.NoError(t, err)

		result, err := adapter.GetAvailableSkills(ctx, &engine.GetAvailableSkillsInput{
			ClassID:      "fighter",
			BackgroundID: "invalid-background",
		})
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result.ClassSkills, 1)
		assert.Empty(t, result.BackgroundSkills)
	})
}

func TestFormatSkillName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"athletics", "Athletics"},
		{"sleight_of_hand", "Sleight Of Hand"},
		{"animal_handling", "Animal Handling"},
		{"arcana", "Arcana"},
		{"investigation", "Investigation"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := formatSkillName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetSkillAbility(t *testing.T) {
	testCases := []struct {
		skillID  string
		expected string
	}{
		{"athletics", "strength"},
		{"acrobatics", "dexterity"},
		{"sleight_of_hand", "dexterity"},
		{"arcana", "intelligence"},
		{"history", "intelligence"},
		{"animal_handling", "wisdom"},
		{"perception", "wisdom"},
		{"deception", "charisma"},
		{"persuasion", "charisma"},
		{"unknown_skill", "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.skillID, func(t *testing.T) {
			result := getSkillAbility(tc.skillID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCalculateCharacterStats(t *testing.T) {
	ctx := context.Background()

	t.Run("nil input", func(t *testing.T) {
		adapter := createTestAdapter(t)
		result, err := adapter.CalculateCharacterStats(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "draft is required")
	})

	t.Run("nil draft", func(t *testing.T) {
		adapter := createTestAdapter(t)
		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "draft is required")
	})

	t.Run("missing class ID", func(t *testing.T) {
		adapter := createTestAdapter(t)
		draft := &dnd5e.CharacterDraft{
			RaceID: "human",
			AbilityScores: &dnd5e.AbilityScores{
				Strength: 15, Dexterity: 14, Constitution: 13,
				Intelligence: 12, Wisdom: 10, Charisma: 8,
			},
		}
		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "class ID is required")
	})

	t.Run("missing race ID", func(t *testing.T) {
		adapter := createTestAdapter(t)
		draft := &dnd5e.CharacterDraft{
			ClassID: "fighter",
			AbilityScores: &dnd5e.AbilityScores{
				Strength: 15, Dexterity: 14, Constitution: 13,
				Intelligence: 12, Wisdom: 10, Charisma: 8,
			},
		}
		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "race ID is required")
	})

	t.Run("missing ability scores", func(t *testing.T) {
		adapter := createTestAdapter(t)
		draft := &dnd5e.CharacterDraft{
			ClassID: "fighter",
			RaceID:  "human",
		}
		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "ability scores are required")
	})

	t.Run("external client error for class", func(t *testing.T) {
		mockClient := &testExternalClient{
			classError: errors.Internal("api error"),
		}
		adapter := createTestAdapterWithClient(t, mockClient)
		draft := &dnd5e.CharacterDraft{
			ClassID: "fighter",
			RaceID:  "human",
			AbilityScores: &dnd5e.AbilityScores{
				Strength: 15, Dexterity: 14, Constitution: 13,
				Intelligence: 12, Wisdom: 10, Charisma: 8,
			},
		}
		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to get class data")
	})

	t.Run("invalid class ID", func(t *testing.T) {
		mockClient := &testExternalClient{
			classData: nil, // Returning nil indicates invalid ID
		}
		adapter := createTestAdapterWithClient(t, mockClient)
		draft := &dnd5e.CharacterDraft{
			ClassID: "invalid",
			RaceID:  "human",
			AbilityScores: &dnd5e.AbilityScores{
				Strength: 15, Dexterity: 14, Constitution: 13,
				Intelligence: 12, Wisdom: 10, Charisma: 8,
			},
		}
		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid class ID")
	})

	t.Run("external client error for race", func(t *testing.T) {
		mockClient := &testExternalClient{
			classData: &external.ClassData{
				ID: "fighter", Name: "Fighter", HitDice: "1d10",
				SavingThrows: []string{"strength", "constitution"},
			},
			raceError: errors.Internal("api error"),
		}
		adapter := createTestAdapterWithClient(t, mockClient)
		draft := &dnd5e.CharacterDraft{
			ClassID: "fighter",
			RaceID:  "human",
			AbilityScores: &dnd5e.AbilityScores{
				Strength: 15, Dexterity: 14, Constitution: 13,
				Intelligence: 12, Wisdom: 10, Charisma: 8,
			},
		}
		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to get race data")
	})

	t.Run("invalid race ID", func(t *testing.T) {
		mockClient := &testExternalClient{
			classData: &external.ClassData{
				ID: "fighter", Name: "Fighter", HitDice: "1d10",
				SavingThrows: []string{"strength", "constitution"},
			},
			raceData: nil, // Returning nil indicates invalid ID
		}
		adapter := createTestAdapterWithClient(t, mockClient)
		draft := &dnd5e.CharacterDraft{
			ClassID: "fighter",
			RaceID:  "invalid",
			AbilityScores: &dnd5e.AbilityScores{
				Strength: 15, Dexterity: 14, Constitution: 13,
				Intelligence: 12, Wisdom: 10, Charisma: 8,
			},
		}
		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid race ID")
	})

	t.Run("successful fighter calculation", func(t *testing.T) {
		mockClient := &testExternalClient{
			classData: &external.ClassData{
				ID: "fighter", Name: "Fighter", HitDice: "1d10",
				SavingThrows: []string{"strength", "constitution"},
			},
			raceData: &external.RaceData{
				ID: "human", Name: "Human", Speed: 30,
			},
		}
		adapter := createTestAdapterWithClient(t, mockClient)
		draft := &dnd5e.CharacterDraft{
			ClassID: "fighter",
			RaceID:  "human",
			AbilityScores: &dnd5e.AbilityScores{
				Strength: 16, Dexterity: 14, Constitution: 15,
				Intelligence: 10, Wisdom: 12, Charisma: 8,
			},
			StartingSkillIDs: []string{"athletics", "intimidation"},
		}

		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Check basic stats
		assert.Equal(t, int32(12), result.MaxHP)           // 10 (d10) + 2 (CON mod)
		assert.Equal(t, int32(12), result.ArmorClass)      // 10 + 2 (DEX mod)
		assert.Equal(t, int32(2), result.Initiative)       // DEX mod
		assert.Equal(t, int32(30), result.Speed)           // Human speed
		assert.Equal(t, int32(2), result.ProficiencyBonus) // Level 1

		// Check saving throws
		assert.Equal(t, int32(5), result.SavingThrows["strength"])     // 3 (STR mod) + 2 (prof)
		assert.Equal(t, int32(2), result.SavingThrows["dexterity"])    // 2 (DEX mod) + 0
		assert.Equal(t, int32(4), result.SavingThrows["constitution"]) // 2 (CON mod) + 2 (prof)
		assert.Equal(t, int32(0), result.SavingThrows["intelligence"]) // 0 (INT mod) + 0
		assert.Equal(t, int32(1), result.SavingThrows["wisdom"])       // 1 (WIS mod) + 0
		assert.Equal(t, int32(-1), result.SavingThrows["charisma"])    // -1 (CHA mod) + 0

		// Check skills (just a few examples)
		assert.Equal(t, int32(5), result.Skills["athletics"])    // 3 (STR) + 2 (prof)
		assert.Equal(t, int32(2), result.Skills["acrobatics"])   // 2 (DEX) + 0
		assert.Equal(t, int32(1), result.Skills["intimidation"]) // -1 (CHA) + 2 (prof)
	})

	t.Run("successful wizard calculation with negative modifiers", func(t *testing.T) {
		mockClient := &testExternalClient{
			classData: &external.ClassData{
				ID: "wizard", Name: "Wizard", HitDice: "1d6",
				SavingThrows: []string{"intelligence", "wisdom"},
			},
			raceData: &external.RaceData{
				ID: "elf", Name: "Elf", Speed: 30,
			},
		}
		adapter := createTestAdapterWithClient(t, mockClient)
		draft := &dnd5e.CharacterDraft{
			ClassID: "wizard",
			RaceID:  "elf",
			AbilityScores: &dnd5e.AbilityScores{
				Strength: 8, Dexterity: 14, Constitution: 12,
				Intelligence: 16, Wisdom: 13, Charisma: 10,
			},
			StartingSkillIDs: []string{"arcana", "investigation"},
		}

		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Check basic stats
		assert.Equal(t, int32(7), result.MaxHP)            // 6 (d6) + 1 (CON mod)
		assert.Equal(t, int32(12), result.ArmorClass)      // 10 + 2 (DEX mod)
		assert.Equal(t, int32(2), result.Initiative)       // DEX mod
		assert.Equal(t, int32(30), result.Speed)           // Elf speed
		assert.Equal(t, int32(2), result.ProficiencyBonus) // Level 1

		// Check saving throws
		assert.Equal(t, int32(-1), result.SavingThrows["strength"])    // -1 (STR mod) + 0
		assert.Equal(t, int32(2), result.SavingThrows["dexterity"])    // 2 (DEX mod) + 0
		assert.Equal(t, int32(1), result.SavingThrows["constitution"]) // 1 (CON mod) + 0
		assert.Equal(t, int32(5), result.SavingThrows["intelligence"]) // 3 (INT mod) + 2 (prof)
		assert.Equal(t, int32(3), result.SavingThrows["wisdom"])       // 1 (WIS mod) + 2 (prof)
		assert.Equal(t, int32(0), result.SavingThrows["charisma"])     // 0 (CHA mod) + 0

		// Check skills
		assert.Equal(t, int32(5), result.Skills["arcana"])        // 3 (INT) + 2 (prof)
		assert.Equal(t, int32(5), result.Skills["investigation"]) // 3 (INT) + 2 (prof)
		assert.Equal(t, int32(3), result.Skills["history"])       // 3 (INT) + 0
	})

	t.Run("calculation with background skills", func(t *testing.T) {
		mockClient := &testExternalClient{
			classData: &external.ClassData{
				ID: "rogue", Name: "Rogue", HitDice: "1d8",
				SavingThrows: []string{"dexterity", "intelligence"},
			},
			raceData: &external.RaceData{
				ID: "halfling", Name: "Halfling", Speed: 25,
			},
			backgroundData: &external.BackgroundData{
				ID: "criminal", Name: "Criminal",
				SkillProficiencies: []string{"deception", "stealth"},
			},
		}
		adapter := createTestAdapterWithClient(t, mockClient)
		draft := &dnd5e.CharacterDraft{
			ClassID:      "rogue",
			RaceID:       "halfling",
			BackgroundID: "criminal",
			AbilityScores: &dnd5e.AbilityScores{
				Strength: 10, Dexterity: 16, Constitution: 14,
				Intelligence: 13, Wisdom: 12, Charisma: 8,
			},
			StartingSkillIDs: []string{"acrobatics", "perception", "investigation", "sleight_of_hand"},
		}

		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Check basic stats
		assert.Equal(t, int32(10), result.MaxHP)      // 8 (d8) + 2 (CON mod)
		assert.Equal(t, int32(13), result.ArmorClass) // 10 + 3 (DEX mod)
		assert.Equal(t, int32(3), result.Initiative)  // DEX mod
		assert.Equal(t, int32(25), result.Speed)      // Halfling speed

		// Check skills with background proficiencies
		assert.Equal(t, int32(1), result.Skills["deception"])       // -1 (CHA) + 2 (prof from background)
		assert.Equal(t, int32(5), result.Skills["stealth"])         // 3 (DEX) + 2 (prof from background)
		assert.Equal(t, int32(5), result.Skills["acrobatics"])      // 3 (DEX) + 2 (prof from class)
		assert.Equal(t, int32(3), result.Skills["perception"])      // 1 (WIS) + 2 (prof from class)
		assert.Equal(t, int32(3), result.Skills["investigation"])   // 1 (INT) + 2 (prof from class)
		assert.Equal(t, int32(5), result.Skills["sleight_of_hand"]) // 3 (DEX) + 2 (prof from class)
	})

	t.Run("edge case with very low constitution", func(t *testing.T) {
		mockClient := &testExternalClient{
			classData: &external.ClassData{
				ID: "wizard", Name: "Wizard", HitDice: "1d6",
				SavingThrows: []string{"intelligence", "wisdom"},
			},
			raceData: &external.RaceData{
				ID: "human", Name: "Human", Speed: 30,
			},
		}
		adapter := createTestAdapterWithClient(t, mockClient)
		draft := &dnd5e.CharacterDraft{
			ClassID: "wizard",
			RaceID:  "human",
			AbilityScores: &dnd5e.AbilityScores{
				Strength: 10, Dexterity: 10, Constitution: 3, // Very low CON
				Intelligence: 16, Wisdom: 13, Charisma: 10,
			},
		}

		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Check HP with negative CON modifier
		assert.Equal(t, int32(2), result.MaxHP) // 6 (d6) + -4 (CON mod) = 2 (minimum 1 would be enforced in real game)
	})

	t.Run("invalid hit dice format", func(t *testing.T) {
		mockClient := &testExternalClient{
			classData: &external.ClassData{
				ID: "custom", Name: "Custom", HitDice: "2d8", // Invalid format
				SavingThrows: []string{"strength"},
			},
			raceData: &external.RaceData{
				ID: "human", Name: "Human", Speed: 30,
			},
		}
		adapter := createTestAdapterWithClient(t, mockClient)
		draft := &dnd5e.CharacterDraft{
			ClassID: "custom",
			RaceID:  "human",
			AbilityScores: &dnd5e.AbilityScores{
				Strength: 15, Dexterity: 14, Constitution: 13,
				Intelligence: 12, Wisdom: 10, Charisma: 8,
			},
		}

		result, err := adapter.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{Draft: draft})
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Should default to d6
		assert.Equal(t, int32(7), result.MaxHP) // 6 (default d6) + 1 (CON mod)
	})
}

func TestExtractMaxHitDie(t *testing.T) {
	testCases := []struct {
		hitDice  string
		expected int32
	}{
		{"1d6", 6},
		{"1d8", 8},
		{"1d10", 10},
		{"1d12", 12},
		{"", 6},        // Invalid format
		{"d8", 6},      // Invalid format
		{"2d6", 6},     // Invalid format
		{"1d20", 6},    // Unknown die
		{"invalid", 6}, // Invalid format
	}

	for _, tc := range testCases {
		t.Run(tc.hitDice, func(t *testing.T) {
			result := extractMaxHitDie(tc.hitDice)
			assert.Equal(t, tc.expected, result)
		})
	}
}
