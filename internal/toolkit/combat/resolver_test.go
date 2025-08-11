package combat

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEntity implements core.Entity for testing
type mockEntity struct {
	id         string
	entityType string
}

func (m *mockEntity) GetID() string      { return m.id }
func (m *mockEntity) GetType() string    { return m.entityType }
func (m *mockEntity) GetName() string    { return "Test Entity" }
func (m *mockEntity) GetPosition() (int, int) { return 0, 0 }

func TestCombatResolver_ResolveAttack(t *testing.T) {
	ctx := context.Background()
	resolver := NewCombatResolver()

	attacker := &mockEntity{id: "attacker1", entityType: "character"}
	defender := &mockEntity{id: "defender1", entityType: "character"}

	t.Run("basic attack - hit", func(t *testing.T) {
		// Attack with a high bonus should hit a low AC
		result, err := resolver.ResolveAttack(
			ctx,
			attacker,
			defender,
			10,      // Attack bonus
			10,      // Target AC
			"1d8+3", // Weapon damage
			"slashing",
		)
		
		require.NoError(t, err)
		assert.NotNil(t, result)
		
		// Check basic fields are populated
		assert.Equal(t, 10, result.TargetAC)
		assert.Equal(t, "slashing", result.DamageType)
		assert.GreaterOrEqual(t, result.AttackRoll, 1)
		assert.LessOrEqual(t, result.AttackRoll, 20)
		
		// With +10 bonus, even a roll of 1 should hit AC 10
		// unless we got unlucky with modifiers
		if result.AttackRoll > 1 {
			assert.True(t, result.Hit || result.Critical)
		}
	})

	t.Run("critical hit", func(t *testing.T) {
		// Keep trying until we get a critical (nat 20)
		// In real tests we'd mock the dice roller
		maxAttempts := 100
		gotCritical := false
		
		for i := 0; i < maxAttempts; i++ {
			result, err := resolver.ResolveAttack(
				ctx,
				attacker,
				defender,
				5,       // Attack bonus
				15,      // Target AC
				"1d8+3", // Weapon damage
				"slashing",
			)
			
			require.NoError(t, err)
			
			if result.AttackRoll == 20 {
				gotCritical = true
				assert.True(t, result.Critical)
				assert.True(t, result.Hit)
				// Critical should double the damage dice
				assert.Greater(t, result.TotalDamage, 0)
				break
			}
		}
		
		// We should have gotten at least one critical in 100 attempts
		assert.True(t, gotCritical, "Should have gotten at least one critical hit in 100 attempts")
	})

	t.Run("attack with bless modifier", func(t *testing.T) {
		// Register a Bless condition for the attacker
		bless := NewBlessCondition(attacker.GetID())
		err := resolver.RegisterCondition(bless)
		require.NoError(t, err)
		
		result, err := resolver.ResolveAttack(
			ctx,
			attacker,
			defender,
			5,       // Attack bonus
			15,      // Target AC
			"1d8+3", // Weapon damage
			"slashing",
		)
		
		require.NoError(t, err)
		assert.NotNil(t, result)
		
		// Check that modifiers were applied
		hasBlessing := false
		for _, mod := range result.Modifiers {
			if mod.Source() == "Bless" {
				hasBlessing = true
				break
			}
		}
		assert.True(t, hasBlessing, "Attack should have Bless modifier")
	})

	t.Run("miss attack", func(t *testing.T) {
		// Very low bonus vs very high AC should miss most of the time
		result, err := resolver.ResolveAttack(
			ctx,
			attacker,
			defender,
			0,       // Attack bonus
			30,      // Target AC (very high)
			"1d8+3", // Weapon damage
			"slashing",
		)
		
		require.NoError(t, err)
		assert.NotNil(t, result)
		
		// Unless we rolled a natural 20, this should miss
		if result.AttackRoll < 20 {
			assert.False(t, result.Hit)
			assert.Equal(t, 0, result.TotalDamage)
		}
	})
}