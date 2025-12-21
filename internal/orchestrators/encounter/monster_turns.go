package encounter

import (
	"context"
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/actions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monstertraits"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
)

// executeMonsterTurns runs all monster turns until reaching a player character
// Returns the results of all monster turns executed
func (o *Orchestrator) executeMonsterTurns(
	ctx context.Context,
	enc *encounterrepo.EncounterData,
	characterIDs []string,
) ([]*MonsterTurnResult, error) {
	if enc == nil {
		return nil, fmt.Errorf("encounter data is required")
	}
	if enc.InitiativeData == nil {
		return nil, fmt.Errorf("initiative data is required")
	}

	var results []*MonsterTurnResult
	maxIterations := len(enc.InitiativeData.Order) * 2 // Safety limit
	iterationCount := 0

	for iterationCount < maxIterations {
		iterationCount++

		// Check if current entity is a monster
		if !o.isMonsterTurn(enc) {
			// Reached a character turn, stop
			break
		}

		// Get current monster data
		currentEntity := enc.InitiativeData.Order[enc.InitiativeData.Current]
		monsterData := o.findMonsterData(enc, currentEntity.ID)
		if monsterData == nil {
			// Monster not found in encounter data - skip
			// Advance to next turn
			enc.InitiativeData.Current++
			if enc.InitiativeData.Current >= len(enc.InitiativeData.Order) {
				enc.InitiativeData.Current = 0
				enc.InitiativeData.Round++
			}
			continue
		}

		// Skip dead monsters
		if monsterData.HitPoints <= 0 {
			// Advance to next turn
			enc.InitiativeData.Current++
			if enc.InitiativeData.Current >= len(enc.InitiativeData.Order) {
				enc.InitiativeData.Current = 0
				enc.InitiativeData.Round++
			}
			continue
		}

		// Execute monster's turn
		result, err := o.executeSingleMonsterTurn(ctx, enc, monsterData, characterIDs)
		if err != nil {
			return results, fmt.Errorf("failed to execute monster turn for %s: %w", monsterData.ID, err)
		}

		// Add result to collection
		results = append(results, result)

		// Advance to next turn
		enc.InitiativeData.Current++
		if enc.InitiativeData.Current >= len(enc.InitiativeData.Order) {
			enc.InitiativeData.Current = 0
			enc.InitiativeData.Round++
		}
	}

	// If we hit the safety limit, it means we couldn't find a player character
	// This is fine - return the monster turns we've executed so far
	// The caller (EndTurn/CreateDungeon) will check if combat should end
	return results, nil
}

// executeSingleMonsterTurn runs one monster's turn using the toolkit
func (o *Orchestrator) executeSingleMonsterTurn(
	ctx context.Context,
	enc *encounterrepo.EncounterData,
	monsterData *monster.Data,
	characterIDs []string,
) (*MonsterTurnResult, error) {
	if monsterData == nil {
		return nil, fmt.Errorf("monster data is required")
	}

	// 1. Create EventBus for the monster turn
	bus := events.NewEventBus()

	// 2. Load monster from data
	mon, err := monster.LoadFromData(ctx, monsterData, bus)
	if err != nil {
		return nil, fmt.Errorf("failed to load monster from data: %w", err)
	}
	defer func() {
		_ = mon.Cleanup(ctx)
	}()

	// 3. Load monster actions (required for combat - LoadFromData doesn't load them)
	if err = actions.LoadMonsterActions(mon, monsterData.Actions); err != nil {
		return nil, fmt.Errorf("failed to load monster actions: %w", err)
	}

	// 4. Load monster conditions/traits (vulnerability, immunity, pack tactics, etc.)
	if err = monstertraits.LoadMonsterConditions(ctx, mon, monsterData.Conditions, bus, o.roller); err != nil {
		return nil, fmt.Errorf("failed to load monster conditions: %w", err)
	}

	// 5. Create action economy for the turn
	actionEconomy := combat.NewActionEconomy()

	// 6. Build perception from room data
	var roomData *spatial.RoomData
	if enc.RoomData != nil {
		if rd, ok := enc.RoomData.(*spatial.RoomData); ok {
			roomData = rd
		} else if rdVal, ok := enc.RoomData.(spatial.RoomData); ok {
			roomData = &rdVal
		}
	}

	perception := buildPerception(roomData, monsterData.ID, characterIDs, enc.Monsters)

	// 7. Create turn input
	turnInput := &monster.TurnInput{
		Bus:           bus,
		ActionEconomy: actionEconomy,
		Perception:    perception,
		Roller:        o.roller,
	}

	// 8. Execute the turn
	turnResult, err := mon.TakeTurn(ctx, turnInput)
	if err != nil {
		return nil, fmt.Errorf("monster turn failed: %w", err)
	}

	// 9. Process executed actions and resolve attacks
	// The toolkit publishes AttackEvent but doesn't populate ExecutedAction.Details
	// We need to resolve attacks ourselves based on action type
	actions := make([]MonsterExecutedAction, len(turnResult.Actions))
	for i, action := range turnResult.Actions {
		actions[i] = MonsterExecutedAction{
			ActionID:   action.ActionID,
			ActionType: string(action.ActionType),
			TargetID:   action.TargetID,
			Success:    action.Success,
			Details:    action.Details,
		}

		// If this was an attack action and it succeeded, resolve it
		if action.ActionType == monster.TypeMeleeAttack && action.Success {
			// Resolve the attack
			attackResult, resolveErr := o.resolveMonsterAttack(ctx, mon, monsterData, action.TargetID)
			if resolveErr != nil {
				return nil, fmt.Errorf("failed to resolve attack: %w", resolveErr)
			}

			// Store attack result in action details
			actions[i].Details = attackResult

			// Update character HP in encounter data for TPK tracking
			if attackResult.Hit && enc.CharacterHP != nil {
				currentHP := enc.CharacterHP[action.TargetID]
				newHP := currentHP - attackResult.TotalDamage
				if newHP < 0 {
					newHP = 0 // HP can't go below 0
				}
				enc.CharacterHP[action.TargetID] = newHP
			}
		}
	}

	// 10. Convert movement path (cube coordinates) to Position for result
	movement := make([]Position, len(turnResult.Movement))
	for i, pos := range turnResult.Movement {
		movement[i] = Position{
			X: float64(pos.X),
			Y: float64(pos.Y),
			Z: float64(pos.Z),
		}
	}

	// Update monster's position in room data if it moved
	if len(turnResult.Movement) > 0 && roomData != nil {
		finalPos := turnResult.Movement[len(turnResult.Movement)-1]

		if placement, exists := roomData.CubeEntities[monsterData.ID]; exists {
			placement.CubePosition = finalPos // Direct assignment - both are cube coordinates
			roomData.CubeEntities[monsterData.ID] = placement
		}
	}

	// 11. Create result
	result := &MonsterTurnResult{
		MonsterID:   turnResult.MonsterID,
		MonsterName: monsterData.Name,
		Actions:     actions,
		Movement:    movement,
	}

	return result, nil
}

// resolveMonsterAttack resolves a monster's attack against a character
func (o *Orchestrator) resolveMonsterAttack(
	ctx context.Context,
	mon *monster.Monster,
	monsterData *monster.Data,
	targetID string,
) (*AttackResult, error) {
	// Load target character data
	charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{
		ID: targetID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load target character: %w", err)
	}

	// Create a fresh EventBus for attack resolution
	attackBus := events.NewEventBus()

	// Load character for defense (no need for features to participate in defense yet)
	defender, err := character.LoadFromData(ctx, charOutput.CharacterData, attackBus)
	if err != nil {
		return nil, fmt.Errorf("failed to load defender: %w", err)
	}
	defer func() {
		_ = defender.Cleanup(ctx)
	}()

	// Get monster's primary weapon
	// For Phase 1, use a default scimitar weapon for goblins
	// TODO: Extract weapon from ActionData.Ref or Config in future phases
	weapon, err := weapons.GetByID(weapons.Scimitar)
	if err != nil {
		return nil, fmt.Errorf("failed to get weapon: %w", err)
	}

	// Get defender's AC
	acBreakdown := defender.EffectiveAC(ctx)
	defenderAC := acBreakdown.Total

	// Calculate proficiency bonus from monster data
	// D&D 5e doesn't have a ProficiencyBonus field on monsters directly
	// They have CR-based proficiency, but for now use +2 (goblin default)
	profBonus := 2

	// Resolve the attack using toolkit combat
	combatResult, err := combat.ResolveAttack(ctx, &combat.AttackInput{
		Attacker:         mon,
		Defender:         defender,
		Weapon:           &weapon,
		AttackerScores:   monsterData.AbilityScores,
		DefenderAC:       defenderAC,
		ProficiencyBonus: profBonus,
		EventBus:         attackBus,
		Roller:           o.roller,
	})
	if err != nil {
		return nil, fmt.Errorf("combat resolution failed: %w", err)
	}

	// Convert toolkit result to orchestrator AttackResult
	attackResult := &AttackResult{
		AttackRoll:      combatResult.AttackRoll,
		AttackBonus:     combatResult.AttackBonus,
		TotalAttack:     combatResult.TotalAttack,
		TargetAC:        combatResult.TargetAC,
		Hit:             combatResult.Hit,
		Critical:        combatResult.Critical,
		IsNaturalTwenty: combatResult.IsNaturalTwenty,
		IsNaturalOne:    combatResult.IsNaturalOne,
		DamageRolls:     combatResult.DamageRolls,
		DamageBonus:     combatResult.DamageBonus,
		TotalDamage:     combatResult.TotalDamage,
		DamageType:      combatResult.DamageType,
	}

	// Map breakdown if present
	if combatResult.Breakdown != nil {
		attackResult.Breakdown = convertToolkitBreakdown(combatResult.Breakdown)
	}

	return attackResult, nil
}

// isMonsterTurn checks if current entity in initiative is a monster
func (o *Orchestrator) isMonsterTurn(enc *encounterrepo.EncounterData) bool {
	if enc == nil || enc.InitiativeData == nil {
		return false
	}
	if enc.InitiativeData.Current < 0 || enc.InitiativeData.Current >= len(enc.InitiativeData.Order) {
		return false
	}

	currentEntity := enc.InitiativeData.Order[enc.InitiativeData.Current]
	return currentEntity.Type == entityTypeMonster
}

// findMonsterData finds a monster by ID in the encounter
func (o *Orchestrator) findMonsterData(enc *encounterrepo.EncounterData, id string) *monster.Data {
	if enc == nil {
		return nil
	}

	for _, m := range enc.Monsters {
		if m.ID == id {
			return m
		}
	}

	return nil
}
