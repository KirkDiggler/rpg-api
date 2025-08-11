package combat

import (
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/events"
)

// Combat-specific event context keys
const (
	// Attack context keys
	ContextKeyAttackType   = "attack_type"  // "melee", "ranged", "spell"
	ContextKeyWeaponID     = "weapon_id"    // ID of weapon being used
	ContextKeyAttackRoll   = "attack_roll"  // The d20 roll
	ContextKeyAttackBonus  = "attack_bonus" // Total bonus to attack
	ContextKeyTargetAC     = "target_ac"    // Target's AC
	ContextKeyCriticalHit  = "critical_hit" // Whether it's a critical
	ContextKeyHit          = "hit"          // Whether the attack hit
	ContextKeyAdvantage    = "advantage"    // Attack has advantage
	ContextKeyDisadvantage = "disadvantage" // Attack has disadvantage

	// Damage context keys
	ContextKeyDamageRoll    = "damage_roll"   // The damage dice result
	ContextKeyDamageType    = "damage_type"   // "slashing", "fire", etc
	ContextKeyBaseDamage    = "base_damage"   // Base damage before modifiers
	ContextKeyTotalDamage   = "total_damage"  // Final damage after all modifiers
	ContextKeyResistance    = "resistance"    // Target has resistance
	ContextKeyVulnerability = "vulnerability" // Target has vulnerability
	ContextKeyImmunity      = "immunity"      // Target has immunity

	// Saving throw context keys
	ContextKeySaveType    = "save_type"    // "STR", "DEX", etc
	ContextKeySaveDC      = "save_dc"      // DC to beat
	ContextKeySaveRoll    = "save_roll"    // The d20 roll
	ContextKeySaveBonus   = "save_bonus"   // Total bonus to save
	ContextKeySaveSuccess = "save_success" // Whether save succeeded

	// General combat context
	ContextKeyRound      = "round"      // Current combat round
	ContextKeyInitiative = "initiative" // Current initiative count
)

// AttackEvent represents an attack being made
type AttackEvent struct {
	*events.GameEvent
	Attacker   core.Entity
	Defender   core.Entity
	AttackType string // "melee", "ranged", "spell"
	WeaponID   string // Optional weapon being used
}

// NewAttackEvent creates a new attack event
func NewAttackEvent(eventType events.EventType, attacker, defender core.Entity) *AttackEvent {
	return &AttackEvent{
		GameEvent: events.NewTypedGameEvent(eventType, attacker, defender),
		Attacker:  attacker,
		Defender:  defender,
	}
}

// DamageEvent represents damage being dealt
type DamageEvent struct {
	*events.GameEvent
	DamageType string
	BaseDamage int
}

// NewDamageEvent creates a new damage event
func NewDamageEvent(eventType events.EventType, source, target core.Entity) *DamageEvent {
	return &DamageEvent{
		GameEvent: events.NewTypedGameEvent(eventType, source, target),
	}
}

// SavingThrowEvent represents a saving throw being made
type SavingThrowEvent struct {
	*events.GameEvent
	SaveType string // "STR", "DEX", "CON", "INT", "WIS", "CHA"
	DC       int
}

// NewSavingThrowEvent creates a new saving throw event
func NewSavingThrowEvent(eventType events.EventType, target, source core.Entity) *SavingThrowEvent {
	return &SavingThrowEvent{
		GameEvent: events.NewTypedGameEvent(eventType, target, source),
	}
}

// CombatRoundEvent represents turn/round management
type CombatRoundEvent struct {
	*events.GameEvent
	Round      int
	Initiative int
}

// NewCombatRoundEvent creates a new round event
func NewCombatRoundEvent(eventType events.EventType, entity core.Entity) *CombatRoundEvent {
	return &CombatRoundEvent{
		GameEvent: events.NewTypedGameEvent(eventType, entity, nil),
	}
}

// AttackResult contains the complete result of an attack
type AttackResult struct {
	Hit         bool
	Critical    bool
	AttackRoll  int
	AttackBonus int
	TotalAttack int
	TargetAC    int
	DamageRoll  int
	DamageBonus int
	TotalDamage int
	DamageType  string
	Modifiers   []events.Modifier // All modifiers that affected the attack
}

// SaveResult contains the complete result of a saving throw
type SaveResult struct {
	Success      bool
	CriticalSave bool // Natural 20
	CriticalFail bool // Natural 1
	SaveRoll     int
	SaveBonus    int
	TotalSave    int
	DC           int
	SaveType     string
	Modifiers    []events.Modifier // All modifiers that affected the save
}
