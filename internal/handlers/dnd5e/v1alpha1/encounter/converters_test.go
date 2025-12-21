package encounter

import (
	"testing"

	"github.com/stretchr/testify/suite"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/damage"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

type ConvertersTestSuite struct {
	suite.Suite
}

func TestConvertersSuite(t *testing.T) {
	suite.Run(t, new(ConvertersTestSuite))
}

// =============================================================================
// abilityRefToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestAbilityRefToProto_FastPath_Singleton() {
	// Fast path uses pointer comparison with singleton refs
	tests := []struct {
		name     string
		ref      *core.Ref
		expected dnd5ev1alpha1.Ability
	}{
		{"Strength", refs.Abilities.Strength(), dnd5ev1alpha1.Ability_ABILITY_STRENGTH},
		{"Dexterity", refs.Abilities.Dexterity(), dnd5ev1alpha1.Ability_ABILITY_DEXTERITY},
		{"Constitution", refs.Abilities.Constitution(), dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION},
		{"Intelligence", refs.Abilities.Intelligence(), dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE},
		{"Wisdom", refs.Abilities.Wisdom(), dnd5ev1alpha1.Ability_ABILITY_WISDOM},
		{"Charisma", refs.Abilities.Charisma(), dnd5ev1alpha1.Ability_ABILITY_CHARISMA},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := abilityRefToProto(tt.ref)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *ConvertersTestSuite) TestAbilityRefToProto_SlowPath_ManualRef() {
	// Slow path uses string comparison for manually created refs
	// IDs are the short form: str, dex, con, int, wis, cha
	tests := []struct {
		name     string
		refID    string
		expected dnd5ev1alpha1.Ability
	}{
		{"Strength", "str", dnd5ev1alpha1.Ability_ABILITY_STRENGTH},
		{"Dexterity", "dex", dnd5ev1alpha1.Ability_ABILITY_DEXTERITY},
		{"Constitution", "con", dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION},
		{"Intelligence", "int", dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE},
		{"Wisdom", "wis", dnd5ev1alpha1.Ability_ABILITY_WISDOM},
		{"Charisma", "cha", dnd5ev1alpha1.Ability_ABILITY_CHARISMA},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Manually created ref - won't match pointer comparison
			manualRef := &core.Ref{Module: refs.Module, Type: refs.TypeAbilities, ID: tt.refID}
			result := abilityRefToProto(manualRef)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *ConvertersTestSuite) TestAbilityRefToProto_Unknown() {
	unknownRef := &core.Ref{Module: refs.Module, Type: refs.TypeAbilities, ID: "unknown_ability"}
	result := abilityRefToProto(unknownRef)
	s.Equal(dnd5ev1alpha1.Ability_ABILITY_UNSPECIFIED, result)
}

// =============================================================================
// weaponRefToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestWeaponRefToProto_FastPath_Singleton() {
	// Test a representative sample of weapons via fast path
	tests := []struct {
		name     string
		ref      *core.Ref
		expected dnd5ev1alpha1.Weapon
	}{
		// Simple melee
		{"Club", refs.Weapons.Club(), dnd5ev1alpha1.Weapon_WEAPON_CLUB},
		{"Dagger", refs.Weapons.Dagger(), dnd5ev1alpha1.Weapon_WEAPON_DAGGER},
		{"Quarterstaff", refs.Weapons.Quarterstaff(), dnd5ev1alpha1.Weapon_WEAPON_QUARTERSTAFF},
		// Simple ranged
		{"Shortbow", refs.Weapons.Shortbow(), dnd5ev1alpha1.Weapon_WEAPON_SHORTBOW},
		{"LightCrossbow", refs.Weapons.LightCrossbow(), dnd5ev1alpha1.Weapon_WEAPON_LIGHT_CROSSBOW},
		// Martial melee
		{"Longsword", refs.Weapons.Longsword(), dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD},
		{"Greataxe", refs.Weapons.Greataxe(), dnd5ev1alpha1.Weapon_WEAPON_GREATAXE},
		{"Greatsword", refs.Weapons.Greatsword(), dnd5ev1alpha1.Weapon_WEAPON_GREATSWORD},
		{"Rapier", refs.Weapons.Rapier(), dnd5ev1alpha1.Weapon_WEAPON_RAPIER},
		// Martial ranged
		{"Longbow", refs.Weapons.Longbow(), dnd5ev1alpha1.Weapon_WEAPON_LONGBOW},
		{"HeavyCrossbow", refs.Weapons.HeavyCrossbow(), dnd5ev1alpha1.Weapon_WEAPON_HEAVY_CROSSBOW},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := weaponRefToProto(tt.ref)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *ConvertersTestSuite) TestWeaponRefToProto_SlowPath_ManualRef() {
	// Test slow path with manually created refs
	tests := []struct {
		name     string
		refID    string
		expected dnd5ev1alpha1.Weapon
	}{
		{"longsword", "longsword", dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD},
		{"greataxe", "greataxe", dnd5ev1alpha1.Weapon_WEAPON_GREATAXE},
		{"dagger", "dagger", dnd5ev1alpha1.Weapon_WEAPON_DAGGER},
		{"light-hammer", "light-hammer", dnd5ev1alpha1.Weapon_WEAPON_LIGHT_HAMMER},
		{"hand-crossbow", "hand-crossbow", dnd5ev1alpha1.Weapon_WEAPON_HAND_CROSSBOW},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			manualRef := &core.Ref{Module: refs.Module, Type: refs.TypeWeapons, ID: tt.refID}
			result := weaponRefToProto(manualRef)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *ConvertersTestSuite) TestWeaponRefToProto_Unknown() {
	unknownRef := &core.Ref{Module: refs.Module, Type: refs.TypeWeapons, ID: "vorpal_blade"}
	result := weaponRefToProto(unknownRef)
	s.Equal(dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED, result)
}

// =============================================================================
// conditionRefToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestConditionRefToProto_FastPath_Singleton() {
	tests := []struct {
		name     string
		ref      *core.Ref
		expected dnd5ev1alpha1.ConditionId
	}{
		{"Raging", refs.Conditions.Raging(), dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING},
		{"BrutalCritical", refs.Conditions.BrutalCritical(), dnd5ev1alpha1.ConditionId_CONDITION_ID_BRUTAL_CRITICAL},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := conditionRefToProto(tt.ref)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *ConvertersTestSuite) TestConditionRefToProto_SlowPath_ManualRef() {
	tests := []struct {
		name     string
		refID    string
		expected dnd5ev1alpha1.ConditionId
	}{
		{"raging", "raging", dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING},
		{"brutal_critical", "brutal_critical", dnd5ev1alpha1.ConditionId_CONDITION_ID_BRUTAL_CRITICAL},
		{"sneak_attack", "sneak_attack", dnd5ev1alpha1.ConditionId_CONDITION_ID_SNEAK_ATTACK},
		{"divine_smite", "divine_smite", dnd5ev1alpha1.ConditionId_CONDITION_ID_DIVINE_SMITE},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			manualRef := &core.Ref{Module: refs.Module, Type: refs.TypeConditions, ID: tt.refID}
			result := conditionRefToProto(manualRef)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *ConvertersTestSuite) TestConditionRefToProto_Unknown() {
	unknownRef := &core.Ref{Module: refs.Module, Type: refs.TypeConditions, ID: "homebrew_condition"}
	result := conditionRefToProto(unknownRef)
	s.Equal(dnd5ev1alpha1.ConditionId_CONDITION_ID_UNSPECIFIED, result)
}

// =============================================================================
// featureRefToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestFeatureRefToProto_SlowPath() {
	// Note: No fast path implemented yet for features
	tests := []struct {
		name     string
		refID    string
		expected dnd5ev1alpha1.FeatureId
	}{
		{"breath_weapon", "breath_weapon", dnd5ev1alpha1.FeatureId_FEATURE_ID_BREATH_WEAPON},
		{"hellish_rebuke", "hellish_rebuke", dnd5ev1alpha1.FeatureId_FEATURE_ID_HELLISH_REBUKE},
		{"radiance_of_dawn", "radiance_of_dawn", dnd5ev1alpha1.FeatureId_FEATURE_ID_RADIANCE_OF_DAWN},
		{"wrath_of_the_storm", "wrath_of_the_storm", dnd5ev1alpha1.FeatureId_FEATURE_ID_WRATH_OF_THE_STORM},
		{"deflect_missiles", "deflect_missiles", dnd5ev1alpha1.FeatureId_FEATURE_ID_DEFLECT_MISSILES},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			ref := &core.Ref{Module: refs.Module, Type: refs.TypeFeatures, ID: tt.refID}
			result := featureRefToProto(ref)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *ConvertersTestSuite) TestFeatureRefToProto_Unknown() {
	// Rage and SecondWind don't have proto enums yet
	unknownRef := &core.Ref{Module: refs.Module, Type: refs.TypeFeatures, ID: "rage"}
	result := featureRefToProto(unknownRef)
	s.Equal(dnd5ev1alpha1.FeatureId_FEATURE_ID_UNSPECIFIED, result)
}

// =============================================================================
// fightingStyleRefToProto Tests
// =============================================================================

// TestConditionRefToProto_FightingStyles tests that fighting style conditions
// (merged from refs.FightingStyles in toolkit PR #485) are properly converted.
func (s *ConvertersTestSuite) TestConditionRefToProto_FightingStyles_FastPath() {
	tests := []struct {
		name     string
		ref      *core.Ref
		expected dnd5ev1alpha1.ConditionId
	}{
		{"Dueling", refs.Conditions.FightingStyleDueling(), dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_DUELING},
		{"TwoWeaponFighting", refs.Conditions.FightingStyleTwoWeaponFighting(), dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_TWO_WEAPON_FIGHTING},
		{"GreatWeaponFighting", refs.Conditions.FightingStyleGreatWeaponFighting(), dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_GREAT_WEAPON_FIGHTING},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := conditionRefToProto(tt.ref)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *ConvertersTestSuite) TestConditionRefToProto_FightingStyles_SlowPath() {
	tests := []struct {
		name     string
		refID    string
		expected dnd5ev1alpha1.ConditionId
	}{
		{"dueling", "fighting_style_dueling", dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_DUELING},
		{"two_weapon_fighting", "fighting_style_two_weapon_fighting", dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_TWO_WEAPON_FIGHTING},
		{"great_weapon_fighting", "fighting_style_great_weapon_fighting", dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_GREAT_WEAPON_FIGHTING},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Fighting styles are now conditions (Type: "conditions")
			manualRef := &core.Ref{Module: refs.Module, Type: refs.TypeConditions, ID: tt.refID}
			result := conditionRefToProto(manualRef)
			s.Equal(tt.expected, result)
		})
	}
}

// =============================================================================
// convertCoreRefToProtoSourceRef Tests
// =============================================================================

func (s *ConvertersTestSuite) TestConvertCoreRefToProtoSourceRef_Nil() {
	result := convertCoreRefToProtoSourceRef(nil)
	s.Nil(result)
}

func (s *ConvertersTestSuite) TestConvertCoreRefToProtoSourceRef_AbilityType() {
	ref := refs.Abilities.Strength()
	result := convertCoreRefToProtoSourceRef(ref)

	s.Require().NotNil(result)
	ability, ok := result.Source.(*dnd5ev1alpha1.SourceRef_Ability)
	s.Require().True(ok, "expected SourceRef_Ability")
	s.Equal(dnd5ev1alpha1.Ability_ABILITY_STRENGTH, ability.Ability)
}

func (s *ConvertersTestSuite) TestConvertCoreRefToProtoSourceRef_WeaponType() {
	ref := refs.Weapons.Longsword()
	result := convertCoreRefToProtoSourceRef(ref)

	s.Require().NotNil(result)
	weapon, ok := result.Source.(*dnd5ev1alpha1.SourceRef_Weapon)
	s.Require().True(ok, "expected SourceRef_Weapon")
	s.Equal(dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD, weapon.Weapon)
}

func (s *ConvertersTestSuite) TestConvertCoreRefToProtoSourceRef_ConditionType() {
	ref := refs.Conditions.Raging()
	result := convertCoreRefToProtoSourceRef(ref)

	s.Require().NotNil(result)
	condition, ok := result.Source.(*dnd5ev1alpha1.SourceRef_Condition)
	s.Require().True(ok, "expected SourceRef_Condition")
	s.Equal(dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING, condition.Condition)
}

func (s *ConvertersTestSuite) TestConvertCoreRefToProtoSourceRef_FeatureType() {
	ref := &core.Ref{Module: refs.Module, Type: refs.TypeFeatures, ID: "breath_weapon"}
	result := convertCoreRefToProtoSourceRef(ref)

	s.Require().NotNil(result)
	feature, ok := result.Source.(*dnd5ev1alpha1.SourceRef_Feature)
	s.Require().True(ok, "expected SourceRef_Feature")
	s.Equal(dnd5ev1alpha1.FeatureId_FEATURE_ID_BREATH_WEAPON, feature.Feature)
}

func (s *ConvertersTestSuite) TestConvertCoreRefToProtoSourceRef_FightingStyleCondition() {
	// Fighting styles are now conditions (PR #485 merged them into refs.Conditions)
	ref := refs.Conditions.FightingStyleDueling()
	result := convertCoreRefToProtoSourceRef(ref)

	s.Require().NotNil(result)
	// Fighting styles map to ConditionId since they're conditions
	condition, ok := result.Source.(*dnd5ev1alpha1.SourceRef_Condition)
	s.Require().True(ok, "expected SourceRef_Condition for fighting style condition")
	s.Equal(dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_DUELING, condition.Condition)
}

func (s *ConvertersTestSuite) TestConvertCoreRefToProtoSourceRef_UnknownType() {
	ref := &core.Ref{Module: refs.Module, Type: "unknown_type", ID: "something"}
	result := convertCoreRefToProtoSourceRef(ref)
	s.Nil(result, "unknown type should return nil")
}

// =============================================================================
// convertAttackResultToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestConvertAttackResultToProto_Nil() {
	result := convertAttackResultToProto(nil)
	s.Nil(result)
}

func (s *ConvertersTestSuite) TestConvertAttackResultToProto_Hit() {
	attackResult := &encounter.AttackResult{
		AttackRoll:  15,
		AttackBonus: 5,
		TotalAttack: 20,
		TargetAC:    16,
		Hit:         true,
		Critical:    false,
		DamageRolls: []int{6, 4},
		DamageBonus: 3,
		TotalDamage: 13,
		DamageType:  damage.Slashing,
	}

	result := convertAttackResultToProto(attackResult)

	s.Require().NotNil(result)
	s.True(result.Hit)
	s.False(result.Critical)
	s.Equal(int32(15), result.AttackRoll)
	s.Equal(int32(20), result.AttackTotal)
	s.Equal(int32(16), result.TargetAc)
	s.Equal(int32(13), result.Damage)
	s.Equal("slashing", result.DamageType)
}

func (s *ConvertersTestSuite) TestConvertAttackResultToProto_CriticalHit() {
	attackResult := &encounter.AttackResult{
		AttackRoll:      20,
		AttackBonus:     5,
		TotalAttack:     25,
		TargetAC:        18,
		Hit:             true,
		Critical:        true,
		IsNaturalTwenty: true,
		DamageRolls:     []int{6, 6, 4, 3},
		DamageBonus:     4,
		TotalDamage:     23,
		DamageType:      damage.Piercing,
	}

	result := convertAttackResultToProto(attackResult)

	s.Require().NotNil(result)
	s.True(result.Hit)
	s.True(result.Critical)
	s.Equal(int32(20), result.AttackRoll)
	s.Equal(int32(23), result.Damage)
	s.Equal("piercing", result.DamageType)
}

func (s *ConvertersTestSuite) TestConvertAttackResultToProto_Miss() {
	attackResult := &encounter.AttackResult{
		AttackRoll:  8,
		AttackBonus: 4,
		TotalAttack: 12,
		TargetAC:    18,
		Hit:         false,
		Critical:    false,
		TotalDamage: 0,
		DamageType:  damage.Bludgeoning,
	}

	result := convertAttackResultToProto(attackResult)

	s.Require().NotNil(result)
	s.False(result.Hit)
	s.Equal(int32(0), result.Damage)
	s.Equal("bludgeoning", result.DamageType)
}

func (s *ConvertersTestSuite) TestConvertAttackResultToProto_WithBreakdown() {
	attackResult := &encounter.AttackResult{
		AttackRoll:  15,
		TotalAttack: 20,
		TargetAC:    14,
		Hit:         true,
		TotalDamage: 12,
		DamageType:  damage.Slashing,
		Breakdown: &encounter.DamageBreakdown{
			AbilityUsed: "STR",
			TotalDamage: 12,
			Components: []encounter.DamageComponent{
				{
					Source:            "weapon",
					SourceRef:         refs.Weapons.Greataxe(),
					OriginalDiceRolls: []int{8},
					FinalDiceRolls:    []int{8},
					FlatBonus:         4,
					DamageType:        damage.Slashing,
					IsCritical:        false,
				},
			},
		},
	}

	result := convertAttackResultToProto(attackResult)

	s.Require().NotNil(result)
	s.Require().NotNil(result.DamageBreakdown)
	s.Equal("STR", result.DamageBreakdown.AbilityUsed)
	s.Equal(int32(12), result.DamageBreakdown.TotalDamage)
	s.Require().Len(result.DamageBreakdown.Components, 1)

	comp := result.DamageBreakdown.Components[0]
	s.Equal("weapon", comp.Source) //nolint:staticcheck // testing deprecated field for backwards compatibility
	s.Equal([]int32{8}, comp.OriginalDiceRolls)
	s.Equal([]int32{8}, comp.FinalDiceRolls)
	s.Equal(int32(4), comp.FlatBonus)
	s.Equal("slashing", comp.DamageType)
}

// =============================================================================
// convertDamageBreakdownToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestConvertDamageBreakdownToProto_Nil() {
	result := convertDamageBreakdownToProto(nil)
	s.Nil(result)
}

func (s *ConvertersTestSuite) TestConvertDamageBreakdownToProto_MultipleComponents() {
	breakdown := &encounter.DamageBreakdown{
		AbilityUsed: "STR",
		TotalDamage: 20,
		Components: []encounter.DamageComponent{
			{
				Source:            "weapon",
				SourceRef:         refs.Weapons.Greataxe(),
				OriginalDiceRolls: []int{10},
				FinalDiceRolls:    []int{10},
				FlatBonus:         4,
				DamageType:        damage.Slashing,
			},
			{
				Source:            "feature",
				SourceRef:         refs.Conditions.Raging(),
				OriginalDiceRolls: []int{},
				FinalDiceRolls:    []int{},
				FlatBonus:         2,
				DamageType:        damage.Slashing,
			},
		},
	}

	result := convertDamageBreakdownToProto(breakdown)

	s.Require().NotNil(result)
	s.Equal("STR", result.AbilityUsed)
	s.Equal(int32(20), result.TotalDamage)
	s.Require().Len(result.Components, 2)

	// Verify weapon component
	s.Equal("weapon", result.Components[0].Source) //nolint:staticcheck // testing deprecated field for backwards compatibility
	s.Equal(int32(4), result.Components[0].FlatBonus)

	// Verify rage bonus component
	s.Equal("feature", result.Components[1].Source) //nolint:staticcheck // testing deprecated field for backwards compatibility
	s.Equal(int32(2), result.Components[1].FlatBonus)
}

// =============================================================================
// convertDamageComponentToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestConvertDamageComponentToProto_Nil() {
	result := convertDamageComponentToProto(nil)
	s.Nil(result)
}

func (s *ConvertersTestSuite) TestConvertDamageComponentToProto_WithRerolls() {
	comp := &encounter.DamageComponent{
		Source:            "weapon",
		SourceRef:         refs.Weapons.Greatsword(),
		OriginalDiceRolls: []int{1, 2, 6},
		FinalDiceRolls:    []int{4, 5, 6},
		Rerolls: []encounter.RerollEvent{
			{DieIndex: 0, Before: 1, After: 4, Reason: "great_weapon_fighting"},
			{DieIndex: 1, Before: 2, After: 5, Reason: "great_weapon_fighting"},
		},
		FlatBonus:  4,
		DamageType: damage.Slashing,
		IsCritical: false,
	}

	result := convertDamageComponentToProto(comp)

	s.Require().NotNil(result)
	s.Equal("weapon", result.Source) //nolint:staticcheck // testing deprecated field for backwards compatibility
	s.Equal([]int32{1, 2, 6}, result.OriginalDiceRolls)
	s.Equal([]int32{4, 5, 6}, result.FinalDiceRolls)
	s.Require().Len(result.Rerolls, 2)

	// Check first reroll
	s.Equal(int32(0), result.Rerolls[0].DieIndex)
	s.Equal(int32(1), result.Rerolls[0].Before)
	s.Equal(int32(4), result.Rerolls[0].After)
	s.Equal("great_weapon_fighting", result.Rerolls[0].Reason)
}

func (s *ConvertersTestSuite) TestConvertDamageComponentToProto_CriticalDamage() {
	comp := &encounter.DamageComponent{
		Source:            "weapon",
		SourceRef:         refs.Weapons.Longsword(),
		OriginalDiceRolls: []int{8, 6},
		FinalDiceRolls:    []int{8, 6},
		FlatBonus:         3,
		DamageType:        damage.Slashing,
		IsCritical:        true,
	}

	result := convertDamageComponentToProto(comp)

	s.Require().NotNil(result)
	s.True(result.IsCritical)
	s.Equal([]int32{8, 6}, result.OriginalDiceRolls)
}

func (s *ConvertersTestSuite) TestConvertDamageComponentToProto_VulnerabilityMultiplier() {
	comp := &encounter.DamageComponent{
		Source:     "monster_trait",
		SourceRef:  refs.MonsterTraits.Vulnerability(),
		DamageType: damage.Bludgeoning,
		Multiplier: 2.0,
	}

	result := convertDamageComponentToProto(comp)

	s.Require().NotNil(result)
	s.Require().NotNil(result.Multiplier)
	s.Equal(float32(2.0), *result.Multiplier)
	s.Equal("monster_trait", result.Source)
}

func (s *ConvertersTestSuite) TestConvertDamageComponentToProto_ImmunityMultiplier() {
	// Immunity uses multiplier = 0, which would be filtered out without the source check
	comp := &encounter.DamageComponent{
		Source:     "monster_trait",
		SourceRef:  refs.MonsterTraits.Immunity(),
		DamageType: damage.Poison,
		Multiplier: 0.0, // Immunity = multiply by 0
	}

	result := convertDamageComponentToProto(comp)

	s.Require().NotNil(result)
	s.Require().NotNil(result.Multiplier, "immunity multiplier should be set even when 0")
	s.Equal(float32(0.0), *result.Multiplier)
}

func (s *ConvertersTestSuite) TestConvertDamageComponentToProto_NoMultiplierForRegularComponent() {
	// Regular components should not have a multiplier set
	comp := &encounter.DamageComponent{
		Source:         "weapon",
		SourceRef:      refs.Weapons.Longsword(),
		FinalDiceRolls: []int{6},
		FlatBonus:      3,
		DamageType:     damage.Slashing,
		Multiplier:     0, // Not set (zero value)
	}

	result := convertDamageComponentToProto(comp)

	s.Require().NotNil(result)
	s.Nil(result.Multiplier, "regular components should not have multiplier set")
}

// =============================================================================
// convertRerollEventToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestConvertRerollEventToProto_Nil() {
	result := convertRerollEventToProto(nil)
	s.Nil(result)
}

func (s *ConvertersTestSuite) TestConvertRerollEventToProto_Success() {
	event := &encounter.RerollEvent{
		DieIndex: 0,
		Before:   1,
		After:    5,
		Reason:   "great_weapon_fighting",
	}

	result := convertRerollEventToProto(event)

	s.Require().NotNil(result)
	s.Equal(int32(0), result.DieIndex)
	s.Equal(int32(1), result.Before)
	s.Equal(int32(5), result.After)
	s.Equal("great_weapon_fighting", result.Reason)
}

// =============================================================================
// convertGridTypeToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestConvertGridTypeToProto() {
	tests := []struct {
		name     string
		gridType string
		expected apiv1alpha1.GridType
	}{
		{"Square", spatial.GridTypeSquare, apiv1alpha1.GridType_GRID_TYPE_SQUARE},
		{"Hex", spatial.GridTypeHex, apiv1alpha1.GridType_GRID_TYPE_HEX_POINTY},
		{"HexPointy", "hex_pointy", apiv1alpha1.GridType_GRID_TYPE_HEX_POINTY},
		{"HexFlat", "hex_flat", apiv1alpha1.GridType_GRID_TYPE_HEX_FLAT},
		{"Gridless", spatial.GridTypeGridless, apiv1alpha1.GridType_GRID_TYPE_GRIDLESS},
		{"Unknown", "unknown", apiv1alpha1.GridType_GRID_TYPE_UNSPECIFIED},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := convertGridTypeToProto(tt.gridType)
			s.Equal(tt.expected, result)
		})
	}
}

// =============================================================================
// convertRoomDataToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestConvertRoomDataToProto_Nil() {
	result := convertRoomDataToProto(nil)
	s.Nil(result)
}

func (s *ConvertersTestSuite) TestConvertRoomDataToProto_Pointer() {
	roomData := &spatial.RoomData{
		ID:       "room-1",
		Type:     "dungeon",
		Width:    10,
		Height:   10,
		GridType: spatial.GridTypeSquare,
		Entities: map[string]spatial.EntityPlacement{
			"hero-1": {
				EntityID:   "hero-1",
				EntityType: "character",
				Position:   spatial.Position{X: 5, Y: 5},
				Size:       1,
			},
		},
	}

	result := convertRoomDataToProto(roomData)

	s.Require().NotNil(result)
	s.Equal("room-1", result.Id)
	s.Equal("dungeon", result.Type)
	s.Equal(int32(10), result.Width)
	s.Equal(int32(10), result.Height)
	s.Equal(apiv1alpha1.GridType_GRID_TYPE_SQUARE, result.GridType)
	s.Require().Contains(result.Entities, "hero-1")
	s.Equal(float64(5), result.Entities["hero-1"].Position.X)
}

func (s *ConvertersTestSuite) TestConvertRoomDataToProto_Value() {
	roomData := spatial.RoomData{
		ID:       "room-2",
		Type:     "cave",
		Width:    15,
		Height:   20,
		GridType: spatial.GridTypeHex,
	}

	result := convertRoomDataToProto(roomData)

	s.Require().NotNil(result)
	s.Equal("room-2", result.Id)
	s.Equal(apiv1alpha1.GridType_GRID_TYPE_HEX_POINTY, result.GridType)
}

func (s *ConvertersTestSuite) TestConvertRoomDataToProto_InvalidType() {
	// Pass something that's not a RoomData
	result := convertRoomDataToProto("not a room")
	s.Nil(result)
}

// =============================================================================
// Position Conversion Tests (Offset <-> Cube Round-trip)
// =============================================================================

func (s *ConvertersTestSuite) TestConvertPositionToProto_HexGrid_PointyTop() {
	// Test offset (10, 10) converts to cube correctly for pointy-top hex
	pos := spatial.Position{X: 10, Y: 10}
	gridType := spatial.GridTypeHex
	hexOrientation := spatial.HexOrientationPointyTop

	result := convertPositionToProto(pos, gridType, hexOrientation)

	s.Require().NotNil(result)
	// For pointy-top hex, (10, 10) offset -> cube coordinates
	// The sum x + y + z should equal 0 for valid cube coordinates
	s.Equal(float64(0), result.X+result.Y+result.Z, "cube coordinates must satisfy x+y+z=0")
}

func (s *ConvertersTestSuite) TestConvertPositionToProto_SquareGrid() {
	// Square grid should just pass through offset coordinates
	pos := spatial.Position{X: 5, Y: 7}
	gridType := spatial.GridTypeSquare
	hexOrientation := spatial.HexOrientationPointyTop // Ignored for square

	result := convertPositionToProto(pos, gridType, hexOrientation)

	s.Require().NotNil(result)
	s.Equal(float64(5), result.X)
	s.Equal(float64(7), result.Y)
	s.Equal(float64(0), result.Z)
}

func (s *ConvertersTestSuite) TestConvertProtoPositionToOffset_HexGrid_PointyTop() {
	// Test cube coordinates convert back to offset for pointy-top hex
	// First convert offset to cube, then back
	originalOffset := spatial.Position{X: 10, Y: 10}
	gridType := spatial.GridTypeHex
	hexOrientation := spatial.HexOrientationPointyTop

	// Convert to proto (cube)
	protoPos := convertPositionToProto(originalOffset, gridType, hexOrientation)

	// Convert back to offset
	result := convertProtoPositionToOffset(protoPos, gridType, hexOrientation)

	// Should get back the original offset coordinates
	s.Equal(originalOffset.X, result.X)
	s.Equal(originalOffset.Y, result.Y)
}

func (s *ConvertersTestSuite) TestConvertProtoPositionToOffset_SquareGrid() {
	// Square grid should pass through
	protoPos := &apiv1alpha1.Position{X: 5, Y: 7, Z: 0}
	gridType := spatial.GridTypeSquare
	hexOrientation := spatial.HexOrientationPointyTop

	result := convertProtoPositionToOffset(protoPos, gridType, hexOrientation)

	s.Equal(float64(5), result.X)
	s.Equal(float64(7), result.Y)
}

func (s *ConvertersTestSuite) TestConvertProtoPositionToOffset_Nil() {
	gridType := spatial.GridTypeHex
	hexOrientation := spatial.HexOrientationPointyTop

	result := convertProtoPositionToOffset(nil, gridType, hexOrientation)

	s.Equal(float64(0), result.X)
	s.Equal(float64(0), result.Y)
}

func (s *ConvertersTestSuite) TestPositionConversion_RoundTrip_MultiplePositions() {
	// Test round-trip for various positions
	testCases := []struct {
		name   string
		offset spatial.Position
	}{
		{"origin", spatial.Position{X: 0, Y: 0}},
		{"positive_even_row", spatial.Position{X: 10, Y: 10}},
		{"positive_odd_row", spatial.Position{X: 10, Y: 11}},
		{"near_origin", spatial.Position{X: 1, Y: 1}},
		{"larger_coords", spatial.Position{X: 20, Y: 15}},
	}

	gridType := spatial.GridTypeHex
	hexOrientation := spatial.HexOrientationPointyTop

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Offset -> Cube
			protoPos := convertPositionToProto(tc.offset, gridType, hexOrientation)

			// Verify cube coordinate validity (x + y + z = 0)
			s.Equal(
				float64(0),
				protoPos.X+protoPos.Y+protoPos.Z,
				"cube coordinates must satisfy x+y+z=0 for %s", tc.name,
			)

			// Cube -> Offset
			result := convertProtoPositionToOffset(protoPos, gridType, hexOrientation)

			// Verify round-trip preserves original
			s.Equal(tc.offset.X, result.X, "X coordinate mismatch for %s", tc.name)
			s.Equal(tc.offset.Y, result.Y, "Y coordinate mismatch for %s", tc.name)
		})
	}
}

// =============================================================================
// convertCombatStateToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestConvertCombatStateToProto_Nil() {
	result := convertCombatStateToProto(nil, spatial.GridTypeHex, spatial.HexOrientationPointyTop)
	s.Nil(result)
}

func (s *ConvertersTestSuite) TestConvertCombatStateToProto_ActiveCombat() {
	state := &encounter.CombatState{
		EncounterID: "enc-1",
		Round:       2,
		TurnOrder: []encounter.InitiativeEntry{
			{
				EntityID:           "hero-1",
				EntityType:         "character",
				InitiativeRoll:     18,
				InitiativeModifier: 3,
				InitiativeTotal:    21,
				Position:           &encounter.Position{X: 5, Y: 5},
			},
			{
				EntityID:           "goblin-1",
				EntityType:         "monster",
				InitiativeRoll:     12,
				InitiativeModifier: 2,
				InitiativeTotal:    14,
			},
		},
		ActiveIndex:       0,
		MovementRemaining: 20,
		CombatStarted:     true,
		CombatEnded:       false,
	}

	result := convertCombatStateToProto(state, spatial.GridTypeSquare, spatial.HexOrientationPointyTop)

	s.Require().NotNil(result)
	s.Equal("enc-1", result.EncounterId)
	s.Equal(int32(2), result.Round)
	s.True(result.CombatStarted)
	s.False(result.CombatEnded)
	s.Equal(int32(0), result.ActiveIndex)

	// Check turn order
	s.Require().Len(result.TurnOrder, 2)
	s.Equal("hero-1", result.TurnOrder[0].EntityId)
	s.Equal(int32(21), result.TurnOrder[0].Initiative)
	s.Equal(int32(3), result.TurnOrder[0].Modifier)

	// Check current turn state
	s.Require().NotNil(result.CurrentTurn)
	s.Equal("hero-1", result.CurrentTurn.EntityId)
	s.Equal(int32(10), result.CurrentTurn.MovementUsed) // 30 - 20 = 10
	s.Equal(int32(30), result.CurrentTurn.MovementMax)
	s.Require().NotNil(result.CurrentTurn.Position)
	s.Equal(float64(5), result.CurrentTurn.Position.X) // Square grid: offset coords
}

func (s *ConvertersTestSuite) TestConvertCombatStateToProto_NotStarted() {
	state := &encounter.CombatState{
		EncounterID:   "enc-2",
		Round:         0,
		CombatStarted: false,
		CombatEnded:   false,
	}

	result := convertCombatStateToProto(state, spatial.GridTypeHex, spatial.HexOrientationPointyTop)

	s.Require().NotNil(result)
	s.False(result.CombatStarted)
	s.Nil(result.CurrentTurn, "no current turn when combat not started")
}

// =============================================================================
// convertTurnChangeToProto Tests
// =============================================================================

func (s *ConvertersTestSuite) TestConvertTurnChangeToProto_Nil() {
	result := convertTurnChangeToProto(nil)
	s.Nil(result)
}

func (s *ConvertersTestSuite) TestConvertTurnChangeToProto_NewRound() {
	event := &encounter.TurnChangeEvent{
		PreviousEntityID: "goblin-1",
		NextEntityID:     "hero-1",
		Round:            3,
		NewRound:         true,
	}

	result := convertTurnChangeToProto(event)

	s.Require().NotNil(result)
	s.Equal("goblin-1", result.PreviousEntityId)
	s.Equal("hero-1", result.NextEntityId)
	s.Equal(int32(3), result.Round)
	s.True(result.NewRound)
}

func (s *ConvertersTestSuite) TestConvertTurnChangeToProto_SameRound() {
	event := &encounter.TurnChangeEvent{
		PreviousEntityID: "hero-1",
		NextEntityID:     "goblin-1",
		Round:            2,
		NewRound:         false,
	}

	result := convertTurnChangeToProto(event)

	s.Require().NotNil(result)
	s.Equal(int32(2), result.Round)
	s.False(result.NewRound)
}
