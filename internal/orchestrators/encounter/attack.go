package encounter

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
	"github.com/KirkDiggler/rpg-api/internal/toolkit/combat"
	ruleCharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// Attack performs an attack action during combat
func (o *orchestrator) Attack(ctx context.Context, input *AttackInput) (*AttackOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	// Get encounter from repository
	getOutput, err := o.repo.Get(ctx, &encounters.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, err
	}

	// Verify it's the attacker's turn
	tracker := initiative.LoadFromData(*getOutput.Data.InitiativeData)
	current := tracker.Current()
	if current == nil || current.GetID() != input.AttackerID {
		return nil, errors.InvalidArgument("can only attack on your turn")
	}

	// Get attacker and target positions from room
	roomData := getOutput.Data.RoomData
	attackerPos, hasAttacker := getPositionFromRoom(roomData, input.AttackerID)
	targetPos, hasTarget := getPositionFromRoom(roomData, input.TargetID)
	
	if !hasAttacker {
		return nil, errors.NotFound("attacker not found in room")
	}
	if !hasTarget {
		return nil, errors.NotFound("target not found in room")
	}

	// Calculate range
	hexGrid := spatial.NewHexGrid(spatial.HexGridConfig{
		Width:     float64(roomData.Width),
		Height:    float64(roomData.Height),
		PointyTop: true,
	})
	distance := hexGrid.Distance(attackerPos, targetPos)
	rangeInFeet := int(distance) * 5

	// Validate attack range
	attackType := combat.AttackTypeMelee
	if input.AttackType != "" {
		attackType = combat.AttackType(input.AttackType)
	}

	if err := validateAttackRange(attackType, rangeInFeet); err != nil {
		return nil, err
	}

	// Get combat stats for attacker and target
	attackerStats, err := o.getCombatStats(ctx, input.AttackerID, getOutput.Data.EntityHP)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get attacker stats")
	}

	targetStats, err := o.getCombatStats(ctx, input.TargetID, getOutput.Data.EntityHP)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get target stats")
	}

	// Resolve the attack
	resolver := combat.NewResolver()
	attackResult, err := resolver.ResolveAttack(&combat.AttackInput{
		AttackerID:    input.AttackerID,
		AttackerStats: attackerStats,
		AttackType:    attackType,
		WeaponID:      input.WeaponID,
		TargetID:      input.TargetID,
		TargetStats:   targetStats,
		Range:         rangeInFeet,
		// TODO: Check for advantage/disadvantage conditions
		Advantage:    false,
		Disadvantage: false,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve attack")
	}

	// Apply damage if hit
	updatedHP := getOutput.Data.EntityHP
	if updatedHP == nil {
		updatedHP = make(map[string]*encounters.EntityHealth)
	}

	targetHealth, exists := updatedHP[input.TargetID]
	if !exists {
		// Initialize HP for target if not tracked yet
		targetHealth = &encounters.EntityHealth{
			CurrentHP: targetStats.HitPoints,
			MaxHP:     targetStats.MaxHitPoints,
		}
		updatedHP[input.TargetID] = targetHealth
	}

	// Apply damage
	if attackResult.Hit {
		targetHealth.CurrentHP -= attackResult.Damage
		if targetHealth.CurrentHP < 0 {
			targetHealth.CurrentHP = 0
		}

		slog.Info("Attack hit!",
			"attacker", input.AttackerID,
			"target", input.TargetID,
			"damage", attackResult.Damage,
			"remaining_hp", targetHealth.CurrentHP,
		)
	} else {
		slog.Info("Attack missed",
			"attacker", input.AttackerID,
			"target", input.TargetID,
			"roll", attackResult.AttackRoll,
			"total", attackResult.AttackTotal,
			"ac", attackResult.TargetAC,
		)
	}

	// Check if target died
	var updatedRoomData *spatial.RoomData
	if targetHealth.CurrentHP <= 0 {
		// Remove dead entity from room
		updatedRoomData = removeEntityFromRoom(roomData, input.TargetID)
		slog.Info("Target defeated!", "target", input.TargetID)
	}

	// Update encounter with new HP values
	_, err = o.repo.Update(ctx, &encounters.UpdateInput{
		EncounterID: input.EncounterID,
		EntityHP:    updatedHP,
		RoomData:    updatedRoomData, // Only if entity died
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update encounter")
	}

	return &AttackOutput{
		Success:     true,
		Hit:         attackResult.Hit,
		Critical:    attackResult.Critical,
		Damage:      attackResult.Damage,
		DamageType:  attackResult.DamageType,
		AttackRoll:  attackResult.AttackRoll,
		AttackTotal: attackResult.AttackTotal,
		TargetAC:    attackResult.TargetAC,
		TargetNewHP: targetHealth.CurrentHP,
		TargetMaxHP: targetHealth.MaxHP,
		Description: attackResult.Description,
		RoomData:    updatedRoomData, // Only set if target died
	}, nil
}

// getCombatStats retrieves or creates combat stats for an entity
func (o *orchestrator) getCombatStats(ctx context.Context, entityID string, entityHP map[string]*encounters.EntityHealth) (*combat.CombatStats, error) {
	// Try to get character data if it looks like a character ID
	if len(entityID) > 4 && entityID[:4] != "mon_" { // Simple check, improve as needed
		charOutput, err := o.characterService.GetCharacter(ctx, &character.GetCharacterInput{
			CharacterID: entityID,
		})
		if err == nil && charOutput != nil && charOutput.Character != nil {
			// Convert character to combat stats
			charData := charOutput.Character
			stats := &combat.CombatStats{
				Level:            charData.Level,
				ProficiencyBonus: calculateProficiencyBonus(charData.Level),
				Strength:         charData.AbilityScores[constants.STR],
				Dexterity:        charData.AbilityScores[constants.DEX],
				Constitution:     charData.AbilityScores[constants.CON],
				Intelligence:     charData.AbilityScores[constants.INT],
				Wisdom:           charData.AbilityScores[constants.WIS],
				Charisma:         charData.AbilityScores[constants.CHA],
				ArmorClass:       calculateACFromCharacter(charData),
				HitPoints:        charData.HitPoints,
				MaxHitPoints:     charData.MaxHitPoints,
				Speed:            30, // Default
			}
			
			// Set attack bonuses based on class
			// For prototype, use STR for melee, DEX for ranged
			stats.MeleeAttackBonus = 0  // Could add from magic items
			stats.RangedAttackBonus = 0
			stats.MeleeDamageBonus = 0
			stats.RangedDamageBonus = 0
			
			// Check if we have HP tracking
			if health, ok := entityHP[entityID]; ok {
				stats.HitPoints = health.CurrentHP
				stats.MaxHitPoints = health.MaxHP
			}
			
			return stats, nil
		}
	}

	// Default monster stats for prototype
	stats := &combat.CombatStats{
		Level:            1,
		ProficiencyBonus: 2,
		Strength:         14,
		Dexterity:        12,
		Constitution:     12,
		Intelligence:     6,
		Wisdom:           10,
		Charisma:         6,
		ArmorClass:       13,
		HitPoints:        15,
		MaxHitPoints:     15,
		Speed:            30,
	}

	// Check if we have HP tracking
	if health, ok := entityHP[entityID]; ok {
		stats.HitPoints = health.CurrentHP
		stats.MaxHitPoints = health.MaxHP
	}

	return stats, nil
}

// Helper functions

func getPositionFromRoom(roomData *spatial.RoomData, entityID string) (spatial.Position, bool) {
	for id, placement := range roomData.Entities {
		if id == entityID {
			return placement.Position, true
		}
	}
	return spatial.Position{}, false
}

func validateAttackRange(attackType combat.AttackType, rangeInFeet int) error {
	switch attackType {
	case combat.AttackTypeMelee:
		// Melee attacks typically have 5ft reach (adjacent hex)
		if rangeInFeet > 5 {
			return errors.InvalidArgument(fmt.Sprintf("target is out of melee range (%d feet away, need 5 feet)", rangeInFeet))
		}
	case combat.AttackTypeRanged:
		// Ranged attacks have various ranges, use longbow as default (150/600)
		if rangeInFeet > 150 {
			return errors.InvalidArgument(fmt.Sprintf("target is out of range (%d feet away, max 150 feet)", rangeInFeet))
		}
	case combat.AttackTypeSpell:
		// Spell range varies, use fire bolt as default (120 feet)
		if rangeInFeet > 120 {
			return errors.InvalidArgument(fmt.Sprintf("target is out of spell range (%d feet away, max 120 feet)", rangeInFeet))
		}
	}
	return nil
}

func calculateProficiencyBonus(level int) int {
	// D&D 5e proficiency bonus
	return 2 + (level-1)/4
}

func calculateACFromCharacter(charData *ruleCharacter.Data) int {
	// Simple AC calculation for prototype
	// Base 10 + DEX modifier
	dexMod := charData.AbilityScores.Modifier(constants.DEX)
	
	// Check for armor (simplified)
	// TODO: Implement proper armor AC calculation
	baseAC := 10
	
	// Add DEX modifier (capped by armor type)
	return baseAC + dexMod
}

func removeEntityFromRoom(roomData *spatial.RoomData, entityID string) *spatial.RoomData {
	// Create a copy of room data without the dead entity
	newEntities := make(map[string]spatial.EntityPlacement)
	for id, placement := range roomData.Entities {
		if id != entityID {
			newEntities[id] = placement
		}
	}

	return &spatial.RoomData{
		ID:       roomData.ID,
		Type:     roomData.Type,
		Width:    roomData.Width,
		Height:   roomData.Height,
		GridType: roomData.GridType,
		Entities: newEntities,
	}
}