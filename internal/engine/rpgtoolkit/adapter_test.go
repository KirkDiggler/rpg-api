package rpgtoolkit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/engine"

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
		assert.Contains(t, err.Error(), "config is required")
	})

	t.Run("missing event bus", func(t *testing.T) {
		cfg := &AdapterConfig{
			DiceRoller: nil, // Will also fail, but test event bus first
		}

		adapter, err := NewAdapter(cfg)
		assert.Error(t, err)
		assert.Nil(t, adapter)
		assert.Contains(t, err.Error(), "event bus is required")
	})

	t.Run("missing dice roller", func(t *testing.T) {
		cfg := &AdapterConfig{
			EventBus: &stubEventBus{}, // Simple stub for testing
		}

		adapter, err := NewAdapter(cfg)
		assert.Error(t, err)
		assert.Nil(t, adapter)
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
