// Package encounter provides orchestration for D&D 5e encounter management and combat resolution.
package encounter

import (
	"context"
	"fmt"
	"time"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/dice"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/armor"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	dnd5eEvents "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/features"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/gamectx"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
)

// Config holds orchestrator dependencies
type Config struct {
	CharacterRepo characterrepo.Repository
	EncounterRepo encounterrepo.Repository
}

// Validate ensures all required dependencies are present
func (c *Config) Validate() error {
	if c.CharacterRepo == nil {
		return fmt.Errorf("CharacterRepo is required")
	}
	if c.EncounterRepo == nil {
		return fmt.Errorf("EncounterRepo is required")
	}
	return nil
}

// Orchestrator implements the Service interface
type Orchestrator struct {
	charRepo characterrepo.Repository
	encRepo  encounterrepo.Repository
	roller   dice.Roller
}

// New creates a new encounter orchestrator
func New(cfg *Config) (*Orchestrator, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Orchestrator{
		charRepo: cfg.CharacterRepo,
		encRepo:  cfg.EncounterRepo,
		roller:   dice.NewRoller(),
	}, nil
}

// ResolveAttack implements the Service interface
func (o *Orchestrator) ResolveAttack(ctx context.Context, input *ResolveAttackInput) (*ResolveAttackOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.AttackerID == "" {
		return nil, fmt.Errorf("attacker ID is required")
	}
	if input.TargetID == "" {
		return nil, fmt.Errorf("target ID is required")
	}

	// 1. Create EventBus (critical for Rage and other features)
	bus := events.NewEventBus()

	// 2. Load character data from repository
	charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{
		ID: input.AttackerID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load character: %w", err)
	}

	// 3. Load Character from Data (reconstructs features, subscribes to events)
	char, err := character.LoadFromData(ctx, charOutput.CharacterData, bus)
	if err != nil {
		return nil, fmt.Errorf("failed to load character from data: %w", err)
	}

	// 4. Load encounter state
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load encounter: %w", err)
	}

	// Verify encounter exists (even if we don't use its data yet)
	if encOutput.Data == nil {
		return nil, fmt.Errorf("encounter not found: %s", input.EncounterID)
	}

	// 5. Create monster (Phase 2: use NewGoblin factory)
	// TODO: Load monster state from encounter data in future
	goblin := monster.NewGoblin(input.TargetID)

	// 6. Get weapon and equipment slots from equipped items (with fallback to greataxe)
	weapon, equipmentSlots := o.getEquippedWeaponAndSlots(ctx, input.AttackerID)

	// 7. Build GameContext with character equipment for fighting style checks (e.g., Dueling)
	gameCtx := o.buildGameContextFromEquipment(input.AttackerID, &weapon, equipmentSlots)
	ctx = gamectx.WithGameContext(ctx, gameCtx)

	// 8. Call toolkit combat (event-driven, Rage and fighting styles participate here!)
	result, err := combat.ResolveAttack(ctx, &combat.AttackInput{
		Attacker:         char,
		Defender:         goblin,
		Weapon:           &weapon,
		AttackerScores:   charOutput.CharacterData.AbilityScores,
		DefenderAC:       goblin.AC(),
		ProficiencyBonus: char.GetProficiencyBonus(),
		EventBus:         bus,
		Roller:           o.roller,
	})
	if err != nil {
		return nil, fmt.Errorf("combat resolution failed: %w", err)
	}

	// 9. Calculate new monster HP
	newHP := goblin.HP()
	if result.Hit {
		newHP = goblin.HP() - result.TotalDamage
		if newHP < 0 {
			newHP = 0
		}
		// TODO: Persist updated HP to encounter repository
	}

	// 10. Convert toolkit result to our output format
	attackResult := &AttackResult{
		AttackRoll:      result.AttackRoll,
		AttackBonus:     result.AttackBonus,
		TotalAttack:     result.TotalAttack,
		TargetAC:        result.TargetAC,
		Hit:             result.Hit,
		Critical:        result.Critical,
		IsNaturalTwenty: result.IsNaturalTwenty,
		IsNaturalOne:    result.IsNaturalOne,
		DamageRolls:     result.DamageRolls,
		DamageBonus:     result.DamageBonus,
		TotalDamage:     result.TotalDamage,
		DamageType:      result.DamageType,
	}

	// 11. Map breakdown if present (only exists on hit)
	if result.Breakdown != nil {
		attackResult.Breakdown = convertToolkitBreakdown(result.Breakdown)
	}

	return &ResolveAttackOutput{
		Result:      attackResult,
		MonsterHP:   newHP,
		MonsterDead: newHP <= 0,
	}, nil
}

// convertToolkitBreakdown maps toolkit DamageBreakdown to orchestrator type
func convertToolkitBreakdown(breakdown *combat.DamageBreakdown) *DamageBreakdown {
	if breakdown == nil {
		return nil
	}

	components := make([]DamageComponent, len(breakdown.Components))
	for i, comp := range breakdown.Components {
		components[i] = convertToolkitComponent(comp)
	}

	return &DamageBreakdown{
		Components:  components,
		AbilityUsed: string(breakdown.AbilityUsed), // Convert abilities.Ability to string
		TotalDamage: breakdown.TotalDamage,
	}
}

// convertToolkitComponent maps toolkit DamageComponent to orchestrator type
func convertToolkitComponent(comp dnd5eEvents.DamageComponent) DamageComponent {
	// Convert original dice rolls (toolkit uses int, orchestrator uses int)
	originalRolls := make([]int, len(comp.OriginalDiceRolls))
	copy(originalRolls, comp.OriginalDiceRolls)

	// Convert final dice rolls
	finalRolls := make([]int, len(comp.FinalDiceRolls))
	copy(finalRolls, comp.FinalDiceRolls)

	// Convert reroll events
	rerolls := make([]RerollEvent, len(comp.Rerolls))
	for i, r := range comp.Rerolls {
		rerolls[i] = RerollEvent{
			DieIndex: r.DieIndex,
			Before:   r.Before,
			After:    r.After,
			Reason:   r.Reason,
		}
	}

	return DamageComponent{
		Source:            string(comp.Source), // DamageSourceType to string
		SourceRef:         comp.SourceRef,      // Pass through the core.Ref
		OriginalDiceRolls: originalRolls,
		FinalDiceRolls:    finalRolls,
		Rerolls:           rerolls,
		FlatBonus:         comp.FlatBonus,
		DamageType:        comp.DamageType,
		IsCritical:        comp.IsCritical,
	}
}

// CreateDungeon creates a new encounter with an initial room
func (o *Orchestrator) CreateDungeon(ctx context.Context, input *CreateDungeonInput) (*CreateDungeonOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Generate unique encounter ID using timestamp
	encounterID := fmt.Sprintf("enc-%d", time.Now().UnixNano())

	// Create initial room data (20x20 hex grid)
	roomData := &spatial.RoomData{
		ID:       encounterID + "-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		Entities: make(map[string]spatial.EntityPlacement),
	}

	// Add goblin target dummy at fixed position (center of room)
	goblinID := "goblin-dummy"
	roomData.Entities[goblinID] = spatial.EntityPlacement{
		EntityID:       goblinID,
		EntityType:     "monster",
		Position:       spatial.Position{X: 10, Y: 10}, // Center of room
		Size:           1,
		BlocksMovement: true,
	}

	// Define 4 spawn points close to the goblin (within 30ft/6 squares movement)
	// Players can move and attack on turn 1
	spawnPoints := []spatial.Position{
		{X: 5, Y: 8},  // 5 squares from goblin - reachable in one move
		{X: 5, Y: 10}, // 5 squares from goblin
		{X: 5, Y: 12}, // 5 squares from goblin
		{X: 4, Y: 10}, // 6 squares from goblin - just reachable
	}

	// Place characters at spawn points (up to 4 characters)
	for i, characterID := range input.CharacterIDs {
		if i >= len(spawnPoints) {
			break // Only place up to 4 characters
		}

		roomData.Entities[characterID] = spatial.EntityPlacement{
			EntityID:       characterID,
			EntityType:     "character",
			Position:       spawnPoints[i],
			Size:           1,
			BlocksMovement: true,
		}
	}

	// Add some static walls/obstacles for navigation
	// Create a simple layout with pillars around the edges
	obstacles := []spatial.Position{
		{X: 3, Y: 3},   // Top-left pillar
		{X: 3, Y: 17},  // Bottom-left pillar
		{X: 17, Y: 3},  // Top-right pillar
		{X: 17, Y: 17}, // Bottom-right pillar
	}

	for i, pos := range obstacles {
		obstacleID := fmt.Sprintf("pillar-%d", i)
		roomData.Entities[obstacleID] = spatial.EntityPlacement{
			EntityID:       obstacleID,
			EntityType:     "obstacle",
			Position:       pos,
			Size:           1,
			BlocksMovement: true,
		}
	}

	// Roll initiative for all combatants
	entities := make(map[core.Entity]int)

	// Add characters with their DEX modifiers
	for _, characterID := range input.CharacterIDs {
		// Load character to get DEX modifier
		charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
		if err != nil {
			return nil, fmt.Errorf("failed to load character %s: %w", characterID, err)
		}

		// Calculate DEX modifier from ability scores
		dexModifier := charOutput.CharacterData.AbilityScores.Modifier(abilities.DEX)

		// Create participant entity
		char := initiative.NewParticipant(characterID, "character")
		entities[char] = dexModifier
	}

	// Add goblin with hardcoded DEX modifier
	goblin := initiative.NewParticipant(goblinID, "monster")
	entities[goblin] = 2 // Goblin DEX +2

	// Roll initiative using rpg-toolkit
	rolls := initiative.RollForOrder(entities, o.roller)

	// Create tracker and extract data
	initiativeOrder := make([]core.Entity, len(rolls))
	for i, roll := range rolls {
		initiativeOrder[i] = roll.Entity
	}
	tracker := initiative.New(initiativeOrder)
	trackerData := tracker.ToData()

	// Convert rolls to service layer InitiativeEntry with positions from room
	turnOrder := make([]InitiativeEntry, len(rolls))
	for i, roll := range rolls {
		entityID := roll.Entity.GetID()

		// Get position from room data
		var position *Position
		if placement, exists := roomData.Entities[entityID]; exists {
			position = &Position{
				X: placement.Position.X,
				Y: placement.Position.Y,
			}
		}

		turnOrder[i] = InitiativeEntry{
			EntityID:           entityID,
			EntityType:         string(roll.Entity.GetType()),
			InitiativeRoll:     roll.Roll,
			InitiativeModifier: roll.Modifier,
			InitiativeTotal:    roll.Total,
			Position:           position,
		}
	}

	// Create combat state
	combatState := &CombatState{
		EncounterID:       encounterID,
		Round:             1,
		TurnOrder:         turnOrder,
		ActiveIndex:       0,
		MovementRemaining: 30, // Default movement speed for D&D 5e
		CombatStarted:     true,
		CombatEnded:       false,
	}

	// Create monster data for the goblin with scimitar action
	goblinMonster := monster.NewGoblin(goblinID)
	goblinMonster.AddAction(monster.NewScimitarAction(monster.ScimitarConfig{
		AttackBonus: 4,       // +2 DEX + 2 proficiency
		DamageDice:  "1d6+2", // Scimitar 1d6 + DEX
		DamageBonus: 2,       // DEX modifier
	}))
	goblinData := goblinMonster.ToData()

	// Save encounter data with room, initiative, and monsters
	_, err := o.encRepo.Save(ctx, &encounterrepo.SaveInput{
		EncounterID:       encounterID,
		RoomData:          roomData,
		InitiativeData:    &trackerData,
		InitiativeRolls:   rolls,
		MovementRemaining: combatState.MovementRemaining,
		Monsters:          []*monster.Data{goblinData},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save encounter: %w", err)
	}

	// Check if a monster goes first in initiative
	var monsterTurns []*MonsterTurnResult
	if trackerData.Order[0].Type == entityTypeMonster {
		// Monster(s) go first - execute all monster turns until a player's turn
		// Create EncounterData for monster turn execution
		encData := &encounterrepo.EncounterData{
			ID:                encounterID,
			RoomData:          roomData,
			InitiativeData:    &trackerData,
			InitiativeRolls:   rolls,
			MovementRemaining: combatState.MovementRemaining,
			Monsters:          []*monster.Data{goblinData},
		}

		// Execute monster turns
		monsterTurns, err = o.executeMonsterTurns(ctx, encData, input.CharacterIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to execute monster turns: %w", err)
		}

		// Update combat state to reflect the new active index after monster turns
		combatState.ActiveIndex = encData.InitiativeData.Current
		combatState.Round = encData.InitiativeData.Round

		// Persist updated initiative, monster state, and room positions
		_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
			EncounterID:    encounterID,
			InitiativeData: encData.InitiativeData,
			Monsters:       encData.Monsters, // Persist monster HP/state changes
			RoomData:       encData.RoomData, // Persist monster position changes
		})
		if err != nil {
			return nil, fmt.Errorf("failed to save initiative after monster turns: %w", err)
		}
	} else {
		// Player goes first - no monster turns to execute
		// Active index is already set to the first entity (index 0)
		combatState.ActiveIndex = 0
	}

	// Return encounter ID, room data, combat state, and any monster turns
	return &CreateDungeonOutput{
		EncounterID:  encounterID,
		Room:         roomData,
		CombatState:  combatState,
		MonsterTurns: monsterTurns,
	}, nil
}

// MoveCharacter implements character movement within an encounter
// Phase 2: Simple position validation and update
func (o *Orchestrator) MoveCharacter(ctx context.Context, input *MoveCharacterInput) (*MoveCharacterOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.EntityID == "" {
		return nil, fmt.Errorf("entity ID is required")
	}
	if input.TargetPosition == nil {
		return nil, fmt.Errorf("target position is required")
	}

	// 1. Load encounter with room data
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load encounter: %w", err)
	}
	if encOutput.Data == nil {
		return nil, fmt.Errorf("encounter not found: %s", input.EncounterID)
	}

	// Load movement remaining from encounter data
	movementRemaining := encOutput.Data.MovementRemaining
	if movementRemaining == 0 {
		movementRemaining = 30 // Default if not set
	}

	// 2. Check if room data exists (Phase 2: might be nil for early encounters)
	if encOutput.Data.RoomData == nil {
		// Create a basic room for testing
		roomData := &spatial.RoomData{
			ID:       input.EncounterID + "-room",
			Type:     "dungeon",
			Width:    20,
			Height:   20,
			GridType: spatial.GridTypeSquare,
			Entities: make(map[string]spatial.EntityPlacement),
		}
		encOutput.Data.RoomData = roomData
	}

	// 3. Type assert room data to spatial.RoomData
	roomData, ok := encOutput.Data.RoomData.(*spatial.RoomData)
	if !ok {
		// Try non-pointer type
		if roomDataVal, ok := encOutput.Data.RoomData.(spatial.RoomData); ok {
			roomData = &roomDataVal
		} else {
			return nil, fmt.Errorf("invalid room data type in encounter")
		}
	}

	// 4. Validate target position is within bounds
	targetPos := spatial.Position{
		X: input.TargetPosition.X,
		Y: input.TargetPosition.Y,
	}

	if targetPos.X < 0 || targetPos.X >= float64(roomData.Width) ||
		targetPos.Y < 0 || targetPos.Y >= float64(roomData.Height) {
		return &MoveCharacterOutput{
			Success:           false,
			FinalPosition:     input.TargetPosition,
			MovementRemaining: 0,
			StopReason:        "out_of_bounds",
			UpdatedRoom:       roomData,
		}, nil
	}

	// 5. Check if target position is occupied
	for id, entity := range roomData.Entities {
		if id != input.EntityID && entity.BlocksMovement {
			if entity.Position.Equals(targetPos) {
				// Position is blocked by another entity
				var currentPos *Position
				stopReason := "position_occupied"
				if currentEntity, exists := roomData.Entities[input.EntityID]; exists {
					currentPos = &Position{
						X: currentEntity.Position.X,
						Y: currentEntity.Position.Y,
					}
				} else {
					// Entity does not exist in the room
					currentPos = nil
					stopReason = "entity_not_found"
				}
				return &MoveCharacterOutput{
					Success:           false,
					FinalPosition:     currentPos,
					MovementRemaining: 0,
					StopReason:        stopReason,
					UpdatedRoom:       roomData,
				}, nil
			}
		}
	}

	// 6. Calculate distance and update movement
	var oldPos spatial.Position
	entityPlacement, exists := roomData.Entities[input.EntityID]
	if exists {
		oldPos = entityPlacement.Position
	} else {
		// Entity doesn't exist yet - treat as starting from target (0 distance)
		oldPos = targetPos
	}

	// Calculate distance moved (simple distance for now - will use hex pathfinding later)
	// For hex grids, approximate with manhattan distance
	dx := targetPos.X - oldPos.X
	dy := targetPos.Y - oldPos.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	distance := int(dx + dy)
	//nolint:gosec // G115: Game values are bounded by room size limits, no overflow risk
	movementCost := int32(distance * 5) // Each hex is 5 feet

	// Check if enough movement remaining
	if movementCost > movementRemaining {
		return &MoveCharacterOutput{
			Success: false,
			FinalPosition: &Position{
				X: oldPos.X,
				Y: oldPos.Y,
			},
			MovementRemaining: movementRemaining,
			StopReason:        "insufficient_movement",
			UpdatedRoom:       roomData,
		}, nil
	}

	// Decrement movement
	movementRemaining -= movementCost

	// 7. Update entity position
	if !exists {
		// Create new entity placement if it doesn't exist
		entityPlacement = spatial.EntityPlacement{
			EntityID:       input.EntityID,
			EntityType:     "character", // Default type for Phase 2
			Position:       targetPos,
			Size:           1,
			BlocksMovement: true,
		}
	} else {
		// Update existing entity position
		entityPlacement.Position = targetPos
	}
	roomData.Entities[input.EntityID] = entityPlacement

	// 8. Save updated room data and movement remaining
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID:       input.EncounterID,
		RoomData:          roomData,
		MovementRemaining: &movementRemaining,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save updated room: %w", err)
	}

	// 9. Return success with updated position
	return &MoveCharacterOutput{
		Success: true,
		FinalPosition: &Position{
			X: targetPos.X,
			Y: targetPos.Y,
		},
		MovementRemaining: movementRemaining,
		StopReason:        "completed",
		UpdatedRoom:       roomData,
	}, nil
}

// EndTurn advances combat to the next entity's turn
func (o *Orchestrator) EndTurn(ctx context.Context, input *EndTurnInput) (*EndTurnOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}

	// 1. Load encounter data
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load encounter: %w", err)
	}
	if encOutput.Data == nil {
		return nil, fmt.Errorf("encounter not found: %s", input.EncounterID)
	}

	// 2. Validate initiative data exists
	if encOutput.Data.InitiativeData == nil {
		return nil, fmt.Errorf("no initiative data for encounter: %s", input.EncounterID)
	}
	if len(encOutput.Data.InitiativeData.Order) == 0 {
		return nil, fmt.Errorf("empty turn order for encounter: %s", input.EncounterID)
	}

	// 3. Build combat state from stored data
	initiativeData := encOutput.Data.InitiativeData
	initiativeRolls := encOutput.Data.InitiativeRolls

	// Create turn order from initiative data with rolls
	turnOrder := buildTurnOrderFromData(initiativeData, initiativeRolls, encOutput.Data.RoomData)

	// 4. Get the active entity from state (server is authoritative)
	currentIndex := initiativeData.Current
	if currentIndex < 0 || currentIndex >= len(turnOrder) {
		return nil, fmt.Errorf("invalid active index %d for turn order of length %d", currentIndex, len(turnOrder))
	}

	activeEntity := turnOrder[currentIndex]

	// 4a. Validate player ownership if PlayerID is provided
	if input.PlayerID != "" {
		if validateErr := o.validateTurnOwnership(ctx, activeEntity, input.PlayerID); validateErr != nil {
			return nil, validateErr
		}
	}

	// 5. Store previous state for turn change event
	previousEntityID := activeEntity.EntityID
	_ = initiativeData.Round // previousRound - stored but not currently used

	// 6. Advance to next turn
	newRound := false
	currentIndex++
	if currentIndex >= len(turnOrder) {
		// Wrap around to start of order and increment round
		currentIndex = 0
		initiativeData.Round++
		newRound = true
	}

	// 7. Update initiative data with the advanced index
	initiativeData.Current = currentIndex

	// 8. Execute monster turns if the next entity is a monster
	// Collect character IDs for monster AI targeting
	var characterIDs []string
	for _, entry := range turnOrder {
		if entry.EntityType == entityTypeCharacter {
			characterIDs = append(characterIDs, entry.EntityID)
		}
	}

	// Execute all consecutive monster turns until reaching a player character
	var monsterTurns []*MonsterTurnResult
	var encounterResult *EncounterResult

	if o.isMonsterTurn(encOutput.Data) {
		// Run monster turns
		monsterTurns, err = o.executeMonsterTurns(ctx, encOutput.Data, characterIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to execute monster turns: %w", err)
		}

		// Update currentIndex and round from modified encounter data
		currentIndex = encOutput.Data.InitiativeData.Current
		initiativeData.Round = encOutput.Data.InitiativeData.Round

		// Check for combat ending conditions
		encounterResult = o.checkCombatEnd(encOutput.Data)
	}

	// 9. Persist updated state (including monster state and position changes from their turns)
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID:       input.EncounterID,
		InitiativeData:    initiativeData,
		MovementRemaining: ptrInt32(defaultMovementSpeed), // Reset movement for new turn
		Monsters:          encOutput.Data.Monsters,        // Persist monster HP/state changes
		RoomData:          encOutput.Data.RoomData,        // Persist monster position changes
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save turn state: %w", err)
	}

	// 10. Build output
	var nextEntity InitiativeEntry
	var nextEntityID string
	combatEnded := encounterResult != nil

	if !combatEnded && currentIndex >= 0 && currentIndex < len(turnOrder) {
		nextEntity = turnOrder[currentIndex]
		nextEntityID = nextEntity.EntityID
	}

	combatState := &CombatState{
		EncounterID:       input.EncounterID,
		Round:             initiativeData.Round,
		TurnOrder:         turnOrder,
		ActiveIndex:       currentIndex,
		MovementRemaining: defaultMovementSpeed,
		CombatStarted:     true,
		CombatEnded:       combatEnded,
	}

	return &EndTurnOutput{
		CombatState: combatState,
		TurnChange: &TurnChangeEvent{
			PreviousEntityID: previousEntityID,
			NextEntityID:     nextEntityID,
			Round:            initiativeData.Round,
			NewRound:         newRound,
		},
		MonsterTurns:    monsterTurns,
		EncounterResult: encounterResult,
	}, nil
}

// Constants for entity types
const (
	entityTypeCharacter = "character"
	entityTypeMonster   = "monster"
)

// Default movement speed in feet (30 feet = 6 hexes at 5ft/hex)
const defaultMovementSpeed = 30

// ptrInt32 returns a pointer to the given int32 value
func ptrInt32(v int32) *int32 {
	return &v
}

// buildTurnOrderFromData reconstructs the turn order from stored initiative data
func buildTurnOrderFromData(
	initiativeData *initiative.TrackerData,
	initiativeRolls []initiative.Roll,
	roomData interface{},
) []InitiativeEntry {
	// Create a map for quick roll lookup by entity ID
	rollMap := make(map[string]initiative.Roll)
	for _, roll := range initiativeRolls {
		rollMap[roll.Entity.GetID()] = roll
	}

	// Get room data for positions if available
	var spatialRoom *spatial.RoomData
	if roomData != nil {
		if sr, ok := roomData.(*spatial.RoomData); ok {
			spatialRoom = sr
		} else if srVal, ok := roomData.(spatial.RoomData); ok {
			spatialRoom = &srVal
		}
	}

	// Build turn order
	turnOrder := make([]InitiativeEntry, len(initiativeData.Order))
	for i, entityData := range initiativeData.Order {
		entry := InitiativeEntry{
			EntityID:   entityData.ID,
			EntityType: entityData.Type,
		}

		// Add roll data if available
		if roll, exists := rollMap[entityData.ID]; exists {
			entry.InitiativeRoll = roll.Roll
			entry.InitiativeModifier = roll.Modifier
			entry.InitiativeTotal = roll.Total
		}

		// Add position if available from room data
		if spatialRoom != nil {
			if placement, exists := spatialRoom.Entities[entityData.ID]; exists {
				entry.Position = &Position{
					X: placement.Position.X,
					Y: placement.Position.Y,
				}
			}
		}

		turnOrder[i] = entry
	}

	return turnOrder
}

// validateTurnOwnership checks if the requesting player owns the entity whose turn it is.
// Returns an error if:
// - The active entity is a monster (players cannot control monsters)
// - The active entity is a character owned by a different player
func (o *Orchestrator) validateTurnOwnership(
	ctx context.Context,
	activeEntity InitiativeEntry,
	playerID string,
) error {
	// Monsters cannot be controlled by players
	if activeEntity.EntityType == entityTypeMonster {
		return fmt.Errorf("cannot end turn: it is currently a monster's turn (%s)", activeEntity.EntityID)
	}

	// For characters, verify the player owns this character
	if activeEntity.EntityType == entityTypeCharacter {
		charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{
			ID: activeEntity.EntityID,
		})
		if err != nil {
			return fmt.Errorf("failed to validate character ownership: %w", err)
		}

		// Check if this player owns the character
		if charOutput.CharacterData.PlayerID != playerID {
			return fmt.Errorf(
				"cannot end turn: you do not control character %s (owned by player %s)",
				activeEntity.EntityID,
				charOutput.CharacterData.PlayerID,
			)
		}
	}

	return nil
}

// ActivateFeature activates a combat feature (e.g., Rage) for a character.
// The character must have the feature and meet any activation requirements.
func (o *Orchestrator) ActivateFeature(
	ctx context.Context,
	input *ActivateFeatureInput,
) (*ActivateFeatureOutput, error) {
	// 1. Validate input
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.CharacterID == "" {
		return nil, fmt.Errorf("character ID is required")
	}
	if input.FeatureID == "" {
		return nil, fmt.Errorf("feature ID is required")
	}

	// 2. Load encounter to verify it exists (and could check turn order later)
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load encounter: %w", err)
	}
	if encOutput.Data == nil {
		return nil, fmt.Errorf("encounter not found: %s", input.EncounterID)
	}

	// 3. Load character data from repository
	charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load character: %w", err)
	}

	// 4. Create EventBus (CRITICAL for conditions to work)
	bus := events.NewEventBus()

	// 5. Load character domain object with EventBus
	char, err := character.LoadFromData(ctx, charOutput.CharacterData, bus)
	if err != nil {
		return nil, fmt.Errorf("failed to load character from data: %w", err)
	}
	defer func() {
		_ = char.Cleanup(ctx)
	}()

	// 6. Get the feature by ID
	feature := char.GetFeature(input.FeatureID)
	if feature == nil {
		return &ActivateFeatureOutput{
			Success:       false,
			Message:       fmt.Sprintf("feature '%s' not found on character", input.FeatureID),
			CharacterData: charOutput.CharacterData,
		}, nil
	}

	// 7. Feature implements core.Action[FeatureInput], so we can use CanActivate/Activate
	// Create the feature input with the EventBus
	featureInput := features.FeatureInput{Bus: bus}

	// 8. Check if the feature can be activated
	if canErr := feature.CanActivate(ctx, char, featureInput); canErr != nil {
		return &ActivateFeatureOutput{
			Success:       false,
			Message:       fmt.Sprintf("cannot activate %s: %v", input.FeatureID, canErr),
			CharacterData: charOutput.CharacterData,
		}, nil
	}

	// 9. Activate the feature
	if activateErr := feature.Activate(ctx, char, featureInput); activateErr != nil {
		return nil, fmt.Errorf("failed to activate feature: %w", activateErr)
	}

	// 10. Convert back to data (includes new condition in Conditions slice)
	updatedData := char.ToData()

	// 11. Persist updated character
	_, err = o.charRepo.Update(ctx, characterrepo.UpdateInput{
		CharacterData: updatedData,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save character: %w", err)
	}

	// 12. Return success with updated data
	return &ActivateFeatureOutput{
		Success:       true,
		Message:       fmt.Sprintf("%s activated successfully", input.FeatureID),
		CharacterData: updatedData,
	}, nil
}

// getEquippedWeaponAndSlots retrieves the weapon equipped in the character's mainhand slot
// along with the full equipment slots data for GameContext building.
// If no weapon is equipped or the weapon cannot be found, it falls back to a greataxe.
// This ensures combat never fails due to missing equipment data.
func (o *Orchestrator) getEquippedWeaponAndSlots(
	ctx context.Context,
	characterID string,
) (weapons.Weapon, character.EquipmentSlots) {
	// Default fallback weapon
	fallbackWeapon, _ := weapons.GetByID(weapons.Greataxe)

	// Try to load character data (equipment slots are part of character.Data)
	charResult, err := o.charRepo.Get(ctx, characterrepo.GetInput{
		ID: characterID,
	})
	if err != nil {
		return fallbackWeapon, nil
	}

	slots := charResult.CharacterData.EquipmentSlots

	// Check if mainhand has a weapon equipped
	mainHandItemID := slots.Get(character.SlotMainHand)
	if mainHandItemID == "" {
		// No mainhand weapon, use fallback
		return fallbackWeapon, slots
	}

	// Try to get the equipped weapon by ID
	weapon, err := weapons.GetByID(mainHandItemID)
	if err != nil {
		return fallbackWeapon, slots
	}

	return weapon, slots
}

// buildGameContextFromEquipment creates a GameContext with the character's equipped weapons.
// This enables fighting style conditions (like Dueling) to query weapon state
// during combat resolution without bloating event objects.
// Uses already-loaded equipment slots to avoid duplicate repository calls.
func (o *Orchestrator) buildGameContextFromEquipment(
	characterID string,
	mainHandWeapon *weapons.Weapon,
	slots character.EquipmentSlots,
) *gamectx.GameContext {
	// Create character registry
	registry := gamectx.NewBasicCharacterRegistry()

	// Build equipped weapons from main hand weapon
	var equippedWeapons []*gamectx.EquippedWeapon

	if mainHandWeapon != nil {
		equippedWeapons = append(equippedWeapons, &gamectx.EquippedWeapon{
			ID:          mainHandWeapon.ID,
			Name:        mainHandWeapon.Name,
			Slot:        gamectx.SlotMainHand,
			IsShield:    false,
			IsTwoHanded: mainHandWeapon.HasProperty(weapons.PropertyTwoHanded),
			IsMelee:     !mainHandWeapon.IsRanged(),
		})
	}

	// Add off-hand item from equipment slots if available
	// In toolkit model, shields go in the off-hand slot
	if slots != nil {
		offHandItemID := slots.Get(character.SlotOffHand)
		if offHandItemID != "" {
			// Check if it's a shield (armor type) or a weapon
			if offHandItemID == armor.Shield {
				// Shield equipped
				equippedWeapons = append(equippedWeapons, &gamectx.EquippedWeapon{
					ID:       offHandItemID,
					Name:     "Shield",
					Slot:     gamectx.SlotOffHand,
					IsShield: true,
				})
			} else {
				// Try to get as weapon
				offHandWeapon, weaponErr := weapons.GetByID(offHandItemID)
				if weaponErr == nil {
					equippedWeapons = append(equippedWeapons, &gamectx.EquippedWeapon{
						ID:          offHandWeapon.ID,
						Name:        offHandWeapon.Name,
						Slot:        gamectx.SlotOffHand,
						IsShield:    false,
						IsTwoHanded: offHandWeapon.HasProperty(weapons.PropertyTwoHanded),
						IsMelee:     !offHandWeapon.IsRanged(),
					})
				}
			}
		}
	}

	// Add character to registry
	charWeapons := gamectx.NewCharacterWeapons(equippedWeapons)
	registry.Add(characterID, charWeapons)

	// Create and return GameContext
	return gamectx.NewGameContext(gamectx.GameContextConfig{
		CharacterRegistry: registry,
	})
}

// checkCombatEnd checks if combat has ended (all monsters dead or all players dead)
// Returns EncounterResult if combat ended, nil otherwise
func (o *Orchestrator) checkCombatEnd(enc *encounterrepo.EncounterData) *EncounterResult {
	if enc == nil || enc.InitiativeData == nil {
		return nil
	}

	allMonstersDead := true
	allPlayersDead := true

	// Check each entity in initiative order
	for _, entity := range enc.InitiativeData.Order {
		switch entity.Type {
		case entityTypeMonster:
			// Check if this monster is alive
			monsterData := o.findMonsterData(enc, entity.ID)
			if monsterData != nil && monsterData.HitPoints > 0 {
				allMonstersDead = false
			}
		case entityTypeCharacter:
			// TODO: Check if character is alive (HP > 0, not unconscious, etc.)
			// For now, assume all players are alive
			// This will be implemented when we add character HP tracking
			allPlayersDead = false
		}
	}

	// Victory condition: all monsters dead and at least one player alive
	if allMonstersDead && !allPlayersDead {
		return &EncounterResult{
			Reason: "victory",
		}
	}

	// Defeat condition: all players dead/unconscious
	if allPlayersDead {
		return &EncounterResult{
			Reason: "defeat",
		}
	}

	// Combat continues
	return nil
}
