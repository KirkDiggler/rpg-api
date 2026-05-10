package encounter_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rpgerr"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"

	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
)

// CombatantRegistryTestSuite tests the CombatantRegistry (CombatantLookup
// implementation) and documents that *character.Character and *monster.Monster
// already satisfy the combat.Combatant interface without adapter structs.
type CombatantRegistryTestSuite struct {
	suite.Suite
	ctx context.Context
}

func TestCombatantRegistryTestSuite(t *testing.T) {
	suite.Run(t, new(CombatantRegistryTestSuite))
}

func (s *CombatantRegistryTestSuite) SetupTest() {
	s.ctx = context.Background()
}

// ---------------------------------------------------------------------------
// Interface satisfaction — documents that no adapter layer is needed
// ---------------------------------------------------------------------------

// TestCharacterImplementsCombatant verifies that a rehydrated *character.Character
// satisfies the combat.Combatant interface for all methods used by the
// rulebook's combat chain: AC, GetHitPoints, GetMaxHitPoints, AbilityScores,
// ProficiencyBonus, ApplyDamage.
func (s *CombatantRegistryTestSuite) TestCharacterImplementsCombatant() {
	bus := events.NewEventBus()
	data := &character.Data{
		ID:               "char-test",
		Name:             "Aethelwynn",
		Level:            3,
		ProficiencyBonus: 2,
		HitPoints:        24,
		MaxHitPoints:     24,
		ArmorClass:       16,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 12,
			abilities.CON: 14,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
	}

	char, err := character.LoadFromData(s.ctx, data, bus)
	s.Require().NoError(err)

	// Verify via the interface — this also confirms the compile-time var
	// assertions in combatant.go hold at runtime.
	var c combat.Combatant = char

	s.Equal("char-test", c.GetID())
	s.Equal(24, c.GetHitPoints())
	s.Equal(24, c.GetMaxHitPoints())
	s.Equal(16, c.AC())
	s.Equal(2, c.ProficiencyBonus())
	scores := c.AbilityScores()
	s.Equal(16, scores[abilities.STR])

	// ApplyDamage round-trip: deal 5 points of piercing damage.
	result := c.ApplyDamage(s.ctx, &combat.ApplyDamageInput{
		Instances: []combat.DamageInstance{{Amount: 5, Type: "piercing"}},
	})
	s.Require().NotNil(result)
	s.Equal(5, result.TotalDamage)
	s.Equal(19, result.CurrentHP)
	s.Equal(24, result.PreviousHP)
	s.False(result.DroppedToZero)

	// HP is reflected on subsequent call.
	s.Equal(19, c.GetHitPoints())
}

// TestMonsterImplementsCombatant verifies that a *monster.Monster (built via
// monster.New) satisfies the combat.Combatant interface.
func (s *CombatantRegistryTestSuite) TestMonsterImplementsCombatant() {
	m := monster.New(monster.Config{
		ID:               "goblin-1",
		Name:             "Goblin",
		HP:               7,
		AC:               15,
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 8,
			abilities.DEX: 14,
			abilities.CON: 10,
			abilities.INT: 10,
			abilities.WIS: 8,
			abilities.CHA: 8,
		},
	})

	var c combat.Combatant = m

	s.Equal("goblin-1", c.GetID())
	s.Equal(7, c.GetHitPoints())
	s.Equal(7, c.GetMaxHitPoints())
	s.Equal(15, c.AC())
	s.Equal(2, c.ProficiencyBonus())
	scores := c.AbilityScores()
	s.Equal(8, scores[abilities.STR])
	s.Equal(14, scores[abilities.DEX])

	// ApplyDamage round-trip: deal 7 points (exactly killing the goblin).
	result := c.ApplyDamage(s.ctx, &combat.ApplyDamageInput{
		Instances: []combat.DamageInstance{{Amount: 7, Type: "slashing"}},
	})
	s.Require().NotNil(result)
	s.Equal(7, result.TotalDamage)
	s.Equal(0, result.CurrentHP)
	s.True(result.DroppedToZero)
}

// ---------------------------------------------------------------------------
// CombatantRegistry — lookup tests
// ---------------------------------------------------------------------------

// TestRegistry_GetRegistered verifies that a registered combatant is returned
// correctly by Get.
func (s *CombatantRegistryTestSuite) TestRegistry_GetRegistered() {
	reg := v2encounter.NewCombatantRegistry()
	m := monster.NewGoblin("goblin-1")
	reg.Register("goblin-1", m)

	got, err := reg.Get("goblin-1")
	s.Require().NoError(err)
	s.Equal(m, got)
}

// TestRegistry_GetUnknown verifies that Get returns a CodeNotFound rpgerr
// when the requested id is not registered.
func (s *CombatantRegistryTestSuite) TestRegistry_GetUnknown() {
	reg := v2encounter.NewCombatantRegistry()

	got, err := reg.Get("nonexistent-id")
	s.Nil(got)
	s.Require().Error(err)
	s.Equal(rpgerr.CodeNotFound, rpgerr.GetCode(err))
}

// TestRegistry_RegisterMultiple verifies that registering multiple combatants
// and looking each up works correctly.
func (s *CombatantRegistryTestSuite) TestRegistry_RegisterMultiple() {
	bus := events.NewEventBus()
	reg := v2encounter.NewCombatantRegistry()

	goblin := monster.NewGoblin("goblin-1")
	reg.Register("goblin-1", goblin)

	charData := &character.Data{
		ID:               "char-alice",
		Name:             "Alice",
		Level:            1,
		ProficiencyBonus: 2,
		HitPoints:        10,
		MaxHitPoints:     10,
		ArmorClass:       14,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 14,
			abilities.DEX: 12,
			abilities.CON: 12,
			abilities.INT: 10,
			abilities.WIS: 10,
			abilities.CHA: 10,
		},
	}
	alice, err := character.LoadFromData(s.ctx, charData, bus)
	s.Require().NoError(err)
	reg.Register("char-alice", alice)

	gotGoblin, err := reg.Get("goblin-1")
	s.Require().NoError(err)
	s.Equal("goblin-1", gotGoblin.GetID())

	gotAlice, err := reg.Get("char-alice")
	s.Require().NoError(err)
	s.Equal("char-alice", gotAlice.GetID())
}

// TestRegistry_OverwriteEntry verifies that registering a new combatant under
// an existing id replaces the prior entry — useful for tests that inject
// replaced combatants.
func (s *CombatantRegistryTestSuite) TestRegistry_OverwriteEntry() {
	reg := v2encounter.NewCombatantRegistry()

	first := monster.NewGoblin("goblin-1")
	reg.Register("goblin-1", first)

	// second is a fresh goblin (different pointer) registered under the same id.
	second := monster.NewGoblin("goblin-1")
	reg.Register("goblin-1", second)

	got, err := reg.Get("goblin-1")
	s.Require().NoError(err)
	s.Same(second, got, "second registration should overwrite first")
}

// TestRegistry_SatisfiesCombatantLookupInterface verifies that
// *CombatantRegistry satisfies the combat.CombatantLookup interface at the
// type level. This ensures it can be passed directly to
// combat.WithCombatantLookup.
func (s *CombatantRegistryTestSuite) TestRegistry_SatisfiesCombatantLookupInterface() {
	reg := v2encounter.NewCombatantRegistry()
	var _ combat.CombatantLookup = reg
	// If this compiles, the interface is satisfied.
}

// TestRegistry_ZeroValueUsable verifies that a zero-value CombatantRegistry
// (not constructed via NewCombatantRegistry) is safe to use. Register must
// lazy-initialise the internal map rather than panic on a nil-map write.
func (s *CombatantRegistryTestSuite) TestRegistry_ZeroValueUsable() {
	var reg v2encounter.CombatantRegistry

	goblin := monster.NewGoblin("goblin-1")
	// Must not panic — Register lazy-inits the nil map.
	reg.Register("goblin-1", goblin)

	got, err := reg.Get("goblin-1")
	s.Require().NoError(err)
	s.Equal("goblin-1", got.GetID())
}
