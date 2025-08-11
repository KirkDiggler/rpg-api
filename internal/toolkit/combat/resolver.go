package combat

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
)

// Resolver handles combat resolution
type Resolver struct {
	// We can add dice roller dependency here later
}

// NewResolver creates a new combat resolver
func NewResolver() *Resolver {
	return &Resolver{}
}

// ResolveAttack resolves a single attack
func (r *Resolver) ResolveAttack(input *AttackInput) (*AttackOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("attack input is required")
	}
	
	// Determine attack modifier based on attack type
	attackModifier := r.calculateAttackModifier(input)
	
	// Roll the attack
	attackRoll := r.rollD20()
	
	// Check for critical (nat 20) or critical miss (nat 1)
	critical := attackRoll == 20
	criticalMiss := attackRoll == 1
	
	// Handle advantage/disadvantage
	if input.Advantage && !input.Disadvantage {
		secondRoll := r.rollD20()
		if secondRoll > attackRoll {
			attackRoll = secondRoll
			critical = attackRoll == 20
			criticalMiss = false
		}
	} else if input.Disadvantage && !input.Advantage {
		secondRoll := r.rollD20()
		if secondRoll < attackRoll {
			attackRoll = secondRoll
			critical = false
			criticalMiss = attackRoll == 1
		}
	}
	
	// Calculate total attack
	attackTotal := attackRoll + attackModifier
	
	// Determine if hit (nat 20 always hits, nat 1 always misses)
	hit := critical || (!criticalMiss && attackTotal >= input.TargetStats.ArmorClass)
	
	// Calculate damage if hit
	damage := 0
	damageType := "none"
	if hit {
		damage, damageType = r.calculateDamage(input, critical)
	}
	
	// Create description
	description := r.createAttackDescription(input, attackRoll, hit, critical, criticalMiss)
	
	return &AttackOutput{
		Hit:            hit,
		Critical:       critical,
		Damage:         damage,
		DamageType:     damageType,
		AttackRoll:     attackRoll,
		AttackModifier: attackModifier,
		AttackTotal:    attackTotal,
		TargetAC:       input.TargetStats.ArmorClass,
		Description:    description,
	}, nil
}

// calculateAttackModifier determines the attack bonus
func (r *Resolver) calculateAttackModifier(input *AttackInput) int {
	modifier := 0
	
	switch input.AttackType {
	case AttackTypeMelee:
		// Melee uses STR (or DEX for finesse weapons)
		modifier = input.AttackerStats.GetAbilityModifier(constants.STR)
		modifier += input.AttackerStats.ProficiencyBonus // Assuming proficiency
		modifier += input.AttackerStats.MeleeAttackBonus
		
	case AttackTypeRanged:
		// Ranged uses DEX
		modifier = input.AttackerStats.GetAbilityModifier(constants.DEX)
		modifier += input.AttackerStats.ProficiencyBonus
		modifier += input.AttackerStats.RangedAttackBonus
		
	case AttackTypeSpell:
		// Spell attacks use spellcasting ability (INT/WIS/CHA)
		// For now, assume INT
		modifier = input.AttackerStats.GetAbilityModifier(constants.INT)
		modifier += input.AttackerStats.ProficiencyBonus
		modifier += input.AttackerStats.SpellAttackBonus
	}
	
	return modifier
}

// calculateDamage determines damage dealt
func (r *Resolver) calculateDamage(input *AttackInput, critical bool) (int, string) {
	// For prototype, use simple damage calculation
	// TODO: Integrate with actual weapon/spell data
	
	diceDamage := 0
	staticDamage := 0
	damageType := "slashing" // Default
	
	switch input.AttackType {
	case AttackTypeMelee:
		// 1d8 + STR for a longsword
		// On critical: roll damage dice twice, add modifier once
		if critical {
			diceDamage = r.rollDice(2, 8) // Double the dice
		} else {
			diceDamage = r.rollDice(1, 8)
		}
		staticDamage = input.AttackerStats.GetAbilityModifier(constants.STR)
		staticDamage += input.AttackerStats.MeleeDamageBonus
		damageType = "slashing"
		
	case AttackTypeRanged:
		// 1d8 + DEX for a longbow
		// On critical: roll damage dice twice, add modifier once
		if critical {
			diceDamage = r.rollDice(2, 8) // Double the dice
		} else {
			diceDamage = r.rollDice(1, 8)
		}
		staticDamage = input.AttackerStats.GetAbilityModifier(constants.DEX)
		staticDamage += input.AttackerStats.RangedDamageBonus
		damageType = "piercing"
		
	case AttackTypeSpell:
		// 1d10 for fire bolt (cantrip)
		// Cantrips don't add ability modifiers to damage in 5e
		// On critical: roll damage dice twice
		if critical {
			diceDamage = r.rollDice(2, 10) // Double the dice
		} else {
			diceDamage = r.rollDice(1, 10)
		}
		// Spell damage typically doesn't add ability modifiers for cantrips
		staticDamage = 0
		damageType = "fire"
	}
	
	totalDamage := diceDamage + staticDamage
	
	// Minimum 1 damage if hit (optional rule, not standard 5e)
	if totalDamage < 1 {
		totalDamage = 1
	}
	
	return totalDamage, damageType
}

// createAttackDescription generates a narrative description
func (r *Resolver) createAttackDescription(input *AttackInput, roll int, hit bool, critical bool, criticalMiss bool) string {
	if criticalMiss {
		return fmt.Sprintf("Critical miss! The attack goes wide.")
	}
	
	if critical {
		return fmt.Sprintf("CRITICAL HIT! The attack finds a vulnerable spot!")
	}
	
	if hit {
		return fmt.Sprintf("Hit! The attack connects solidly.")
	}
	
	return fmt.Sprintf("Miss! The attack fails to penetrate armor.")
}

// rollD20 rolls a 20-sided die
func (r *Resolver) rollD20() int {
	return rand.Intn(20) + 1
}

// rollDice rolls multiple dice of a given size
func (r *Resolver) rollDice(count, size int) int {
	total := 0
	for i := 0; i < count; i++ {
		total += rand.Intn(size) + 1
	}
	return total
}

// ParseDiceString parses a dice string like "2d6+3" and rolls it
// Returns the total rolled value, or 0 if the string is invalid
func (r *Resolver) ParseDiceString(diceStr string) (int, error) {
	// Simple parser for dice notation
	// Format: XdY+Z or XdY-Z where X is count, Y is die size, Z is modifier
	
	if diceStr == "" {
		return 0, fmt.Errorf("empty dice string")
	}
	
	// Handle both + and - modifiers
	var parts []string
	var isNegative []bool
	
	if strings.Contains(diceStr, "-") {
		// Split on minus, keeping track of which parts are negative
		tempParts := strings.Split(diceStr, "-")
		for i, part := range tempParts {
			if i == 0 {
				parts = append(parts, part)
				isNegative = append(isNegative, false)
			} else {
				parts = append(parts, part)
				isNegative = append(isNegative, true)
			}
		}
	} else {
		parts = strings.Split(diceStr, "+")
		for range parts {
			isNegative = append(isNegative, false)
		}
	}
	
	total := 0
	
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		
		if strings.Contains(part, "d") {
			// Dice roll
			diceParts := strings.Split(part, "d")
			if len(diceParts) != 2 {
				return 0, fmt.Errorf("invalid dice format: %s", part)
			}
			
			var count int
			var err error
			
			if diceParts[0] == "" {
				count = 1 // "d6" means "1d6"
			} else {
				count, err = strconv.Atoi(diceParts[0])
				if err != nil {
					return 0, fmt.Errorf("invalid dice count: %s", diceParts[0])
				}
			}
			
			size, err := strconv.Atoi(diceParts[1])
			if err != nil {
				return 0, fmt.Errorf("invalid dice size: %s", diceParts[1])
			}
			
			if count <= 0 || size <= 0 {
				return 0, fmt.Errorf("dice count and size must be positive")
			}
			
			rollResult := r.rollDice(count, size)
			if isNegative[i] {
				total -= rollResult
			} else {
				total += rollResult
			}
		} else {
			// Static modifier
			mod, err := strconv.Atoi(part)
			if err != nil {
				return 0, fmt.Errorf("invalid modifier: %s", part)
			}
			
			if isNegative[i] {
				total -= mod
			} else {
				total += mod
			}
		}
	}
	
	return total, nil
}

// RollDiceString is a convenience wrapper that returns just the result
// For cases where error handling isn't critical (like prototypes)
func (r *Resolver) RollDiceString(diceStr string) int {
	result, err := r.ParseDiceString(diceStr)
	if err != nil {
		// Log error and return 0
		// In production, this should probably panic or handle differently
		return 0
	}
	return result
}

// CalculateRange calculates the range between two positions in feet
func CalculateRange(fromX, fromY, toX, toY float64) int {
	// For hex grids, this would use hex distance
	// For now, simple grid distance
	dx := fromX - toX
	dy := fromY - toY
	
	// Each hex is 5 feet
	hexDistance := int(max(abs(dx), abs(dy)))
	return hexDistance * 5
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}