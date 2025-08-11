// Package combat provides combat resolution for the RPG API
// This is a prototype implementation that will eventually be moved to rpg-toolkit
package combat

import (
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
)

// AttackType represents the type of attack being made
type AttackType string

const (
	AttackTypeMelee  AttackType = "melee"
	AttackTypeRanged AttackType = "ranged"
	AttackTypeSpell  AttackType = "spell"
)

// AttackInput contains all information needed to resolve an attack
type AttackInput struct {
	// Attacker information
	AttackerID       string
	AttackerStats    *CombatStats
	AttackType       AttackType
	WeaponID         string // Optional, for weapon attacks
	
	// Target information
	TargetID         string
	TargetStats      *CombatStats
	
	// Situational modifiers
	Advantage        bool
	Disadvantage     bool
	Range            int // Distance in feet
}

// AttackOutput contains the results of an attack resolution
type AttackOutput struct {
	Hit              bool
	Critical         bool
	Damage           int
	DamageType       string
	AttackRoll       int // The d20 roll
	AttackModifier   int // Total modifier applied
	AttackTotal      int // Roll + modifier
	TargetAC         int // What we tried to hit
	Description      string // Narrative description
}

// CombatStats represents the combat-relevant stats of an entity
type CombatStats struct {
	// Basic stats
	Level            int
	ProficiencyBonus int
	
	// Ability scores
	Strength         int
	Dexterity        int
	Constitution     int
	Intelligence     int
	Wisdom           int
	Charisma         int
	
	// Combat stats
	ArmorClass       int
	HitPoints        int
	MaxHitPoints     int
	Speed            int
	
	// Attack bonuses
	MeleeAttackBonus  int
	RangedAttackBonus int
	SpellAttackBonus  int
	
	// Damage bonuses
	MeleeDamageBonus  int
	RangedDamageBonus int
}

// GetAbilityModifier calculates the modifier for an ability score
func (cs *CombatStats) GetAbilityModifier(ability constants.Ability) int {
	var score int
	switch ability {
	case constants.STR:
		score = cs.Strength
	case constants.DEX:
		score = cs.Dexterity
	case constants.CON:
		score = cs.Constitution
	case constants.INT:
		score = cs.Intelligence
	case constants.WIS:
		score = cs.Wisdom
	case constants.CHA:
		score = cs.Charisma
	default:
		return 0
	}
	
	// D&D 5e modifier calculation
	return (score - 10) / 2
}

// Weapon represents a weapon's combat properties
type Weapon struct {
	ID           string
	Name         string
	DamageDice   string // e.g., "1d8", "2d6"
	DamageType   string // e.g., "slashing", "piercing"
	Properties   []string // e.g., ["finesse", "light", "thrown"]
	Range        int // in feet, 0 for melee
	Reach        int // in feet, typically 5 for melee
}

// CombatEntity represents an entity in combat
type CombatEntity interface {
	core.Entity
	GetCombatStats() *CombatStats
	TakeDamage(amount int, damageType string) int // Returns actual damage taken
	Heal(amount int) int // Returns actual healing done
	IsAlive() bool
}