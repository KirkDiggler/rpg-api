package combat

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/dice"
	"github.com/KirkDiggler/rpg-toolkit/events"
)

// CombatResolver orchestrates combat resolution using events
type CombatResolver struct {
	eventBus *CombatEventBus
}

// NewCombatResolver creates a new combat resolver
func NewCombatResolver() *CombatResolver {
	return &CombatResolver{
		eventBus: NewCombatEventBus(),
	}
}

// ResolveAttack resolves an attack through the event pipeline
func (r *CombatResolver) ResolveAttack(
	ctx context.Context,
	attacker core.Entity,
	defender core.Entity,
	attackBonus int,
	targetAC int,
	weaponDamage string, // e.g., "1d8+3"
	damageType string, // e.g., "slashing"
) (*AttackResult, error) {

	result := &AttackResult{
		TargetAC:   targetAC,
		DamageType: damageType,
		Modifiers:  []events.Modifier{},
	}

	// Phase 1: Before Attack Roll - conditions can add modifiers
	beforeEvent := NewAttackEvent(events.EventTypeBeforeAttackRoll, attacker, defender)
	beforeEvent.Context().Set(ContextKeyAttackType, "melee") // TODO: pass this in
	beforeEvent.Context().Set(ContextKeyAttackBonus, attackBonus)
	beforeEvent.Context().Set(ContextKeyTargetAC, targetAC)

	if err := r.eventBus.PublishAttackEvent(ctx, beforeEvent); err != nil {
		return nil, fmt.Errorf("before attack event failed: %w", err)
	}

	// Collect modifiers that affect the attack roll
	attackModifiers := beforeEvent.Context().Modifiers()
	result.Modifiers = append(result.Modifiers, attackModifiers...)

	// Phase 2: On Attack Roll - actually roll the dice
	onEvent := NewAttackEvent(events.EventTypeOnAttackRoll, attacker, defender)
	// Copy context from before event
	r.copyContext(beforeEvent.Context(), onEvent.Context())

	// Roll the d20
	d20Roll := dice.D20(1)
	result.AttackRoll = d20Roll.GetValue()
	onEvent.Context().Set(ContextKeyAttackRoll, result.AttackRoll)

	// Check for advantage/disadvantage
	hasAdvantage, hasDisadvantage := r.checkAdvantageDisadvantage(attackModifiers)
	if hasAdvantage || hasDisadvantage {
		secondRoll := dice.D20(1)
		if hasAdvantage && !hasDisadvantage {
			// Take the higher roll
			if secondRoll.GetValue() > result.AttackRoll {
				result.AttackRoll = secondRoll.GetValue()
			}
		} else if hasDisadvantage && !hasAdvantage {
			// Take the lower roll
			if secondRoll.GetValue() < result.AttackRoll {
				result.AttackRoll = secondRoll.GetValue()
			}
		}
		// If both, they cancel out and we use the first roll
	}

	// Apply modifiers to attack bonus
	totalBonus := attackBonus
	for _, mod := range attackModifiers {
		if mod.Type() == "attack_roll" {
			// Handle dice modifiers (like Bless)
			if diceExpr, ok := mod.Value().(string); ok {
				// Parse dice expression and roll it
				// For simplicity, handle common cases like "1d4"
				if diceExpr == "1d4" {
					roll := dice.D4(1)
					totalBonus += roll.GetValue()
				}
			} else if bonus, ok := mod.Value().(int); ok {
				totalBonus += bonus
			}
		}
	}

	result.AttackBonus = totalBonus
	result.TotalAttack = result.AttackRoll + totalBonus

	// Check for critical hit (natural 20)
	result.Critical = (result.AttackRoll == 20)
	result.Hit = result.Critical || (result.TotalAttack >= targetAC)

	onEvent.Context().Set(ContextKeyHit, result.Hit)
	onEvent.Context().Set(ContextKeyCriticalHit, result.Critical)

	if err := r.eventBus.PublishAttackEvent(ctx, onEvent); err != nil {
		return nil, fmt.Errorf("on attack event failed: %w", err)
	}

	// Phase 3: After Attack Roll
	afterEvent := NewAttackEvent(events.EventTypeAfterAttackRoll, attacker, defender)
	r.copyContext(onEvent.Context(), afterEvent.Context())

	if err := r.eventBus.PublishAttackEvent(ctx, afterEvent); err != nil {
		return nil, fmt.Errorf("after attack event failed: %w", err)
	}

	// If the attack missed, we're done
	if !result.Hit {
		return result, nil
	}

	// Phase 4: Before Damage Roll
	beforeDamageEvent := NewDamageEvent(events.EventTypeBeforeDamageRoll, attacker, defender)
	beforeDamageEvent.Context().Set(ContextKeyDamageType, damageType)
	beforeDamageEvent.Context().Set(ContextKeyCriticalHit, result.Critical)
	beforeDamageEvent.Context().Set(ContextKeyAttackType, "melee") // Carry forward

	if err := r.eventBus.PublishAttackEvent(ctx, beforeDamageEvent); err != nil {
		return nil, fmt.Errorf("before damage event failed: %w", err)
	}

	// Collect damage modifiers
	damageModifiers := beforeDamageEvent.Context().Modifiers()
	result.Modifiers = append(result.Modifiers, damageModifiers...)

	// Phase 5: On Damage Roll
	onDamageEvent := NewDamageEvent(events.EventTypeOnDamageRoll, attacker, defender)
	r.copyContext(beforeDamageEvent.Context(), onDamageEvent.Context())

	// Roll base damage
	baseDamage := r.rollDamage(weaponDamage, result.Critical)
	result.DamageRoll = baseDamage
	onDamageEvent.Context().Set(ContextKeyBaseDamage, baseDamage)

	// Apply damage modifiers
	totalDamage := baseDamage
	for _, mod := range damageModifiers {
		if mod.Type() == "damage_bonus" {
			if bonus, ok := mod.Value().(int); ok {
				totalDamage += bonus
			}
		}
	}

	result.DamageBonus = totalDamage - baseDamage
	result.TotalDamage = totalDamage
	onDamageEvent.Context().Set(ContextKeyTotalDamage, totalDamage)

	if err := r.eventBus.PublishAttackEvent(ctx, onDamageEvent); err != nil {
		return nil, fmt.Errorf("on damage event failed: %w", err)
	}

	// Phase 6: After Damage Roll
	afterDamageEvent := NewDamageEvent(events.EventTypeAfterDamageRoll, attacker, defender)
	r.copyContext(onDamageEvent.Context(), afterDamageEvent.Context())

	if err := r.eventBus.PublishAttackEvent(ctx, afterDamageEvent); err != nil {
		return nil, fmt.Errorf("after damage event failed: %w", err)
	}

	// Phase 7: Before Take Damage (defender's chance to react)
	beforeTakeEvent := NewDamageEvent(events.EventTypeBeforeTakeDamage, attacker, defender)
	beforeTakeEvent.Context().Set(ContextKeyDamageType, damageType)
	beforeTakeEvent.Context().Set(ContextKeyTotalDamage, totalDamage)

	if err := r.eventBus.PublishAttackEvent(ctx, beforeTakeEvent); err != nil {
		return nil, fmt.Errorf("before take damage event failed: %w", err)
	}

	// Check for resistance/vulnerability/immunity
	if resistant, _ := beforeTakeEvent.Context().GetBool(ContextKeyResistance); resistant {
		result.TotalDamage = result.TotalDamage / 2
	}
	if vulnerable, _ := beforeTakeEvent.Context().GetBool(ContextKeyVulnerability); vulnerable {
		result.TotalDamage = result.TotalDamage * 2
	}
	if immune, _ := beforeTakeEvent.Context().GetBool(ContextKeyImmunity); immune {
		result.TotalDamage = 0
	}

	// Collect any modifiers from damage reduction
	defenseModifiers := beforeTakeEvent.Context().Modifiers()
	result.Modifiers = append(result.Modifiers, defenseModifiers...)

	return result, nil
}

// RegisterCondition adds a condition to the combat system
func (r *CombatResolver) RegisterCondition(condition Condition) error {
	return r.eventBus.RegisterCondition(condition)
}

// checkAdvantageDisadvantage checks modifiers for advantage/disadvantage
func (r *CombatResolver) checkAdvantageDisadvantage(modifiers []events.Modifier) (bool, bool) {
	hasAdvantage := false
	hasDisadvantage := false

	for _, mod := range modifiers {
		if mod.Type() == "advantage" {
			hasAdvantage = true
		} else if mod.Type() == "disadvantage" {
			hasDisadvantage = true
		}
	}

	return hasAdvantage, hasDisadvantage
}

// rollDamage rolls damage dice, handling criticals
func (r *CombatResolver) rollDamage(damageExpr string, critical bool) int {
	// Parse damage expression (e.g., "1d8+3")
	total := 0
	parts := strings.Split(damageExpr, "+")

	if len(parts) >= 1 {
		dicePart := strings.TrimSpace(parts[0])
		if strings.Contains(dicePart, "d") {
			diceParts := strings.Split(dicePart, "d")
			if len(diceParts) == 2 {
				numDice, _ := strconv.Atoi(diceParts[0])
				dieSize, _ := strconv.Atoi(diceParts[1])

				// Double dice on critical
				if critical {
					numDice *= 2
				}

				// Roll the dice
				for i := 0; i < numDice; i++ {
					roll, _ := dice.NewRoll(1, dieSize)
					total += roll.GetValue()
				}
			}
		}

		// Add any flat modifier
		if len(parts) == 2 {
			modifier, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			total += modifier
		}
	}

	return total
}

// copyContext copies values from one context to another
func (r *CombatResolver) copyContext(from, to events.Context) {
	// Copy known keys
	keys := []string{
		ContextKeyAttackType,
		ContextKeyWeaponID,
		ContextKeyAttackRoll,
		ContextKeyAttackBonus,
		ContextKeyTargetAC,
		ContextKeyCriticalHit,
		ContextKeyHit,
		ContextKeyAdvantage,
		ContextKeyDisadvantage,
		ContextKeyDamageType,
		ContextKeyBaseDamage,
		ContextKeyTotalDamage,
	}

	for _, key := range keys {
		if val, ok := from.Get(key); ok {
			to.Set(key, val)
		}
	}

	// Copy modifiers
	for _, mod := range from.Modifiers() {
		to.AddModifier(mod)
	}
}
