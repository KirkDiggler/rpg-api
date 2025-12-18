// Package encounter provides orchestration for D&D 5e encounter management and combat resolution.
package encounter

import (
	"context"
	"fmt"
	"sort"
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
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	encounterpub "github.com/KirkDiggler/rpg-api/internal/publishers/encounter"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	dungeonrepo "github.com/KirkDiggler/rpg-api/internal/repositories/dungeons"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
)

// Config holds orchestrator dependencies
type Config struct {
	CharacterRepo   characterrepo.Repository
	EncounterRepo   encounterrepo.Repository
	DungeonRepo     dungeonrepo.Repository // Optional: for dungeon persistence
	DungeonGen      *dungeon.Generator     // Optional: for procedural dungeon generation
	Publisher       encounterpub.Publisher // Optional: for publishing encounter events
	EventIDGen      idgen.Generator        // Optional: for generating event IDs (defaults to ULID)
	EncounterIDGen  idgen.Generator        // Optional: for generating encounter IDs (defaults to "enc-" prefix)
	DungeonIDGen    idgen.Generator        // Optional: for generating dungeon IDs (defaults to "dng-" prefix)
	ConnectionIDGen idgen.Generator        // Optional: for generating connection IDs (defaults to "conn-" prefix)
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
	charRepo        characterrepo.Repository
	encRepo         encounterrepo.Repository
	dungeonRepo     dungeonrepo.Repository // Optional: for dungeon persistence
	dungeonGen      *dungeon.Generator     // Optional: for procedural dungeon generation
	roller          dice.Roller
	publisher       encounterpub.Publisher // Optional: for publishing encounter events
	eventIDGen      idgen.Generator        // For generating event IDs
	encounterIDGen  idgen.Generator        // For generating encounter IDs
	dungeonIDGen    idgen.Generator        // For generating dungeon IDs
	connectionIDGen idgen.Generator        // For generating connection IDs
}

// New creates a new encounter orchestrator
func New(cfg *Config) (*Orchestrator, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Default to ULID generator for event IDs if not provided
	eventIDGen := cfg.EventIDGen
	if eventIDGen == nil {
		eventIDGen = idgen.NewULID("")
	}

	// Default to prefixed generators for encounter/dungeon/connection IDs if not provided
	encounterIDGen := cfg.EncounterIDGen
	if encounterIDGen == nil {
		encounterIDGen = idgen.NewPrefixed("enc-")
	}
	dungeonIDGen := cfg.DungeonIDGen
	if dungeonIDGen == nil {
		dungeonIDGen = idgen.NewPrefixed("dng-")
	}
	connectionIDGen := cfg.ConnectionIDGen
	if connectionIDGen == nil {
		connectionIDGen = idgen.NewPrefixed("conn-")
	}

	return &Orchestrator{
		charRepo:        cfg.CharacterRepo,
		encRepo:         cfg.EncounterRepo,
		dungeonRepo:     cfg.DungeonRepo,
		dungeonGen:      cfg.DungeonGen,
		roller:          dice.NewRoller(),
		publisher:       cfg.Publisher,
		eventIDGen:      eventIDGen,
		encounterIDGen:  encounterIDGen,
		dungeonIDGen:    dungeonIDGen,
		connectionIDGen: connectionIDGen,
	}, nil
}

// publishEvent publishes an event if publisher is configured.
// Errors are logged but not returned to avoid breaking combat flow.
func (o *Orchestrator) publishEvent(ctx context.Context, encounterID string, eventType entities.EventType, data interface{}) {
	if o.publisher == nil {
		return
	}

	event := &entities.EncounterEvent{
		ID:          o.eventIDGen.Generate(),
		Type:        eventType,
		EncounterID: encounterID,
		Timestamp:   time.Now(),
	}

	// Set the appropriate typed field based on event type (oneof pattern)
	switch v := data.(type) {
	case *entities.PlayerJoinedEvent:
		event.PlayerJoined = v
	case *entities.PlayerLeftEvent:
		event.PlayerLeft = v
	case *entities.PlayerReadyEvent:
		event.PlayerReady = v
	case *entities.PlayerDisconnectedEvent:
		event.PlayerDisconnected = v
	case *entities.PlayerReconnectedEvent:
		event.PlayerReconnected = v
	case *entities.CombatStartedEvent:
		event.CombatStarted = v
	case *entities.CombatEndedEvent:
		event.CombatEnded = v
	case *entities.CombatPausedEvent:
		event.CombatPaused = v
	case *entities.CombatResumedEvent:
		event.CombatResumed = v
	case *entities.MovementCompletedEvent:
		event.MovementCompleted = v
	case *entities.AttackResolvedEvent:
		event.AttackResolved = v
	case *entities.FeatureActivatedEvent:
		event.FeatureActivated = v
	case *entities.TurnEndedEvent:
		event.TurnEnded = v
	case *entities.MonsterTurnCompletedEvent:
		event.MonsterTurnCompleted = v
	default:
		fmt.Printf("unknown event data type for %s: %T\n", eventType, data)
		return
	}

	_, err := o.publisher.Publish(ctx, &encounterpub.PublishInput{
		EncounterID: encounterID,
		Event:       event,
	})
	if err != nil {
		// Log but don't fail - event publishing is non-critical
		fmt.Printf("failed to publish %s event for encounter %s: %v\n", eventType, encounterID, err)
	}
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

	// Verify encounter exists
	if encOutput.Data == nil {
		return nil, fmt.Errorf("encounter not found: %s", input.EncounterID)
	}

	// 5. Check action economy - attack requires an action
	actionEconomy := encOutput.Data.ActionEconomy
	if actionEconomy == nil {
		// Initialize if not present (backwards compatibility)
		actionEconomy = entities.NewActionEconomyState()
	}
	if !actionEconomy.HasAction() {
		return nil, fmt.Errorf("no action available: action already used this turn")
	}
	// Note: Action is consumed later (step 10) after all validation succeeds

	// 6. Load monster from encounter data (preserves HP across attacks)
	monsterData := o.findMonsterData(encOutput.Data, input.TargetID)
	if monsterData == nil {
		return nil, fmt.Errorf("monster not found: %s", input.TargetID)
	}

	// Create monster instance from stored data
	goblin, err := monster.LoadFromData(ctx, monsterData, bus)
	if err != nil {
		return nil, fmt.Errorf("failed to load monster from data: %w", err)
	}

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

		// Update monster data with new HP
		monsterData.HitPoints = newHP
	}

	// 10. Consume action and persist (action consumed only after all validation succeeds)
	actionEconomy.UseAction()
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID:   input.EncounterID,
		ActionEconomy: actionEconomy,
		Monsters:      encOutput.Data.Monsters,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save encounter state: %w", err)
	}

	// 11. Convert toolkit result to our output format
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

	// 12. Map breakdown if present (only exists on hit)
	if result.Breakdown != nil {
		attackResult.Breakdown = convertToolkitBreakdown(result.Breakdown)
	}

	// 13. Publish AttackResolved event
	o.publishEvent(ctx, input.EncounterID, entities.EventTypeAttackResolved, &entities.AttackResolvedEvent{
		AttackerID: input.AttackerID,
		TargetID:   input.TargetID,
		Result:     attackResult,
		TargetHP:   newHP,
		TargetDead: newHP <= 0,
	})

	// 14. Check for dungeon victory if monster died
	if newHP <= 0 {
		o.checkAndHandleVictory(ctx, input.EncounterID, encOutput.Data, input.TargetID)
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

	// Generate unique encounter ID
	encounterID := o.encounterIDGen.Generate()

	// If dungeon generator is available, use it for procedural generation
	if o.dungeonGen != nil {
		return o.createDungeonWithGenerator(ctx, encounterID, input)
	}

	// Fallback: Create hardcoded room for backwards compatibility
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
	combatants := make(map[core.Entity]int)

	// Initialize CharacterHP map for TPK tracking
	characterHP := make(map[string]int)

	// Add characters with their DEX modifiers
	for _, characterID := range input.CharacterIDs {
		// Load character to get DEX modifier
		charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
		if err != nil {
			return nil, fmt.Errorf("failed to load character %s: %w", characterID, err)
		}

		// Calculate DEX modifier from ability scores
		dexModifier := charOutput.CharacterData.AbilityScores.Modifier(abilities.DEX)

		// Track character's current HP for TPK detection
		characterHP[characterID] = charOutput.CharacterData.HitPoints

		// Create participant entity
		char := initiative.NewParticipant(characterID, "character")
		combatants[char] = dexModifier
	}

	// Add goblin with hardcoded DEX modifier
	goblin := initiative.NewParticipant(goblinID, "monster")
	combatants[goblin] = 2 // Goblin DEX +2

	// Roll initiative using rpg-toolkit
	rolls := initiative.RollForOrder(combatants, o.roller)

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

	// Create combat state with fresh action economy
	combatState := &CombatState{
		EncounterID:       encounterID,
		Round:             1,
		TurnOrder:         turnOrder,
		ActiveIndex:       0,
		MovementRemaining: 30,                               // Default movement speed for D&D 5e
		ActionEconomy:     entities.NewActionEconomyState(), // Fresh action economy for first turn
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

	// Save encounter data with room, initiative, monsters, and action economy
	_, err := o.encRepo.Save(ctx, &encounterrepo.SaveInput{
		EncounterID:       encounterID,
		RoomData:          roomData,
		InitiativeData:    &trackerData,
		InitiativeRolls:   rolls,
		MovementRemaining: combatState.MovementRemaining,
		ActionEconomy:     combatState.ActionEconomy,
		Monsters:          []*monster.Data{goblinData},
		CharacterHP:       characterHP,
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
			CharacterHP:       characterHP,
		}

		// Execute monster turns
		monsterTurns, err = o.executeMonsterTurns(ctx, encData, input.CharacterIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to execute monster turns: %w", err)
		}

		// Update combat state to reflect the new active index after monster turns
		combatState.ActiveIndex = encData.InitiativeData.Current
		combatState.Round = encData.InitiativeData.Round

		// Persist updated initiative, monster state, room positions, and character HP
		_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
			EncounterID:    encounterID,
			InitiativeData: encData.InitiativeData,
			Monsters:       encData.Monsters,    // Persist monster HP/state changes
			RoomData:       encData.RoomData,    // Persist monster position changes
			CharacterHP:    encData.CharacterHP, // Persist character HP changes from monster attacks
		})
		if err != nil {
			return nil, fmt.Errorf("failed to save initiative after monster turns: %w", err)
		}

		// Check for TPK after monster turns
		o.checkAndHandleFailure(ctx, encounterID, encData)
	} else {
		// Player goes first - no monster turns to execute
		// Active index is already set to the first entity (index 0)
		combatState.ActiveIndex = 0
	}

	// Return encounter ID, room data, combat state, and any monster turns
	return &CreateDungeonOutput{
		EncounterID:  encounterID,
		DungeonID:    "", // Empty for fallback path (no generator)
		Room:         roomData,
		Doors:        nil, // No doors for fallback path
		CombatState:  combatState,
		MonsterTurns: monsterTurns,
	}, nil
}

// createDungeonWithGenerator uses the procedural generator to create a dungeon
func (o *Orchestrator) createDungeonWithGenerator(
	ctx context.Context,
	encounterID string,
	input *CreateDungeonInput,
) (*CreateDungeonOutput, error) {
	// Map input parameters to generator input
	genInput := MapToGeneratorInput(&MapToGeneratorInputParams{
		PartySize:  len(input.CharacterIDs),
		PartyLevel: input.PartyLevel,
		ThemeID:    input.ThemeID,
		Difficulty: input.Difficulty,
		Length:     input.Length,
		Seed:       input.Seed,
	})

	// Generate the dungeon
	genOutput, err := o.dungeonGen.Generate(ctx, genInput)
	if err != nil {
		return nil, fmt.Errorf("failed to generate dungeon: %w", err)
	}

	generatedDungeon := genOutput.Dungeon

	// Validate generated dungeon
	if generatedDungeon == nil {
		return nil, fmt.Errorf("dungeon generator returned nil dungeon")
	}
	if len(generatedDungeon.Rooms) == 0 {
		return nil, fmt.Errorf("dungeon generator returned empty dungeon with no rooms")
	}
	if generatedDungeon.StartRoom == "" {
		return nil, fmt.Errorf("dungeon generator did not specify a start room")
	}

	// Create entities.Dungeon for storage
	dungeonEntity := o.convertToDungeonEntity(encounterID, generatedDungeon, genOutput.Seed)

	// Save dungeon to repository if available
	if o.dungeonRepo != nil {
		_, err = o.dungeonRepo.Save(ctx, &dungeonrepo.SaveInput{
			Dungeon: dungeonEntity,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to save dungeon: %w", err)
		}
	}

	// Get the start room
	startRoom := o.findRoom(generatedDungeon, generatedDungeon.StartRoom)
	if startRoom == nil {
		return nil, fmt.Errorf("start room not found: %s", generatedDungeon.StartRoom)
	}

	// Convert start room to spatial.RoomData
	roomData := o.convertToRoomData(encounterID, startRoom)

	// Place characters at spawn zones
	spawnPositions := o.getSpawnPositions(startRoom)
	for i, characterID := range input.CharacterIDs {
		if i >= len(spawnPositions) {
			break
		}
		roomData.Entities[characterID] = spatial.EntityPlacement{
			EntityID:       characterID,
			EntityType:     entityTypeCharacter,
			Position:       spawnPositions[i],
			Size:           1,
			BlocksMovement: true,
		}
	}

	// Place monsters from the encounter
	monsters := o.placeMonsters(roomData, startRoom)

	// Extract doors for response using entity connections (which have proper IDs)
	doors := o.getDoorInfoForRoom(dungeonEntity, generatedDungeon.StartRoom)

	// Roll initiative for all combatants
	combatants := make(map[core.Entity]int)

	// Initialize CharacterHP map for TPK tracking
	characterHP := make(map[string]int)

	// Add characters with their DEX modifiers
	for _, characterID := range input.CharacterIDs {
		charOutput, charErr := o.charRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
		if charErr != nil {
			return nil, fmt.Errorf("failed to load character %s: %w", characterID, charErr)
		}
		dexModifier := charOutput.CharacterData.AbilityScores.Modifier(abilities.DEX)

		// Track character's current HP for TPK detection
		characterHP[characterID] = charOutput.CharacterData.HitPoints

		char := initiative.NewParticipant(characterID, entityTypeCharacter)
		combatants[char] = dexModifier
	}

	// Add monsters to initiative with their actual DEX modifiers
	for _, m := range monsters {
		monsterEntity := initiative.NewParticipant(m.ID, entityTypeMonster)
		dexModifier := m.AbilityScores.Modifier(abilities.DEX)
		combatants[monsterEntity] = dexModifier
	}

	// Roll initiative
	rolls := initiative.RollForOrder(combatants, o.roller)

	// Create tracker and extract data
	initiativeOrder := make([]core.Entity, len(rolls))
	for i, roll := range rolls {
		initiativeOrder[i] = roll.Entity
	}
	tracker := initiative.New(initiativeOrder)
	trackerData := tracker.ToData()

	// Convert rolls to service layer InitiativeEntry
	turnOrder := make([]InitiativeEntry, len(rolls))
	for i, roll := range rolls {
		entityID := roll.Entity.GetID()
		var position *Position
		if placement, exists := roomData.Entities[entityID]; exists {
			position = &Position{X: placement.Position.X, Y: placement.Position.Y}
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
		MovementRemaining: defaultMovementSpeed,
		ActionEconomy:     entities.NewActionEconomyState(),
		CombatStarted:     true,
		CombatEnded:       false,
	}

	// Save encounter data
	_, err = o.encRepo.Save(ctx, &encounterrepo.SaveInput{
		EncounterID:       encounterID,
		RoomData:          roomData,
		InitiativeData:    &trackerData,
		InitiativeRolls:   rolls,
		MovementRemaining: combatState.MovementRemaining,
		ActionEconomy:     combatState.ActionEconomy,
		Monsters:          monsters,
		CharacterHP:       characterHP,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save encounter: %w", err)
	}

	// Execute monster turns if they go first
	var monsterTurns []*MonsterTurnResult
	if len(trackerData.Order) > 0 && trackerData.Order[0].Type == entityTypeMonster {
		encData := &encounterrepo.EncounterData{
			ID:                encounterID,
			RoomData:          roomData,
			InitiativeData:    &trackerData,
			InitiativeRolls:   rolls,
			MovementRemaining: combatState.MovementRemaining,
			Monsters:          monsters,
			CharacterHP:       characterHP,
		}

		monsterTurns, err = o.executeMonsterTurns(ctx, encData, input.CharacterIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to execute monster turns: %w", err)
		}

		combatState.ActiveIndex = encData.InitiativeData.Current
		combatState.Round = encData.InitiativeData.Round

		_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
			EncounterID:    encounterID,
			InitiativeData: encData.InitiativeData,
			Monsters:       encData.Monsters,
			RoomData:       encData.RoomData,
			CharacterHP:    encData.CharacterHP,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to save initiative after monster turns: %w", err)
		}

		// Check for TPK after monster turns
		o.checkAndHandleFailure(ctx, encounterID, encData)
	}

	return &CreateDungeonOutput{
		EncounterID:  encounterID,
		DungeonID:    dungeonEntity.ID,
		Room:         roomData,
		Doors:        doors,
		CombatState:  combatState,
		MonsterTurns: monsterTurns,
	}, nil
}

// convertToDungeonEntity converts a generated dungeon to an entities.Dungeon
func (o *Orchestrator) convertToDungeonEntity(
	encounterID string,
	genDungeon *dungeon.Dungeon,
	seed int64,
) *entities.Dungeon {
	// Convert rooms to map
	rooms := make(map[string]*dungeon.Room)
	for _, room := range genDungeon.Rooms {
		rooms[room.ID] = room
	}

	// Convert connections to toolkit format
	connections := make([]*environments.ConnectionEdge, len(genDungeon.Connections))
	for i, conn := range genDungeon.Connections {
		connections[i] = &environments.ConnectionEdge{
			ID:            o.connectionIDGen.Generate(),
			FromRoomID:    conn.FromRoom,
			ToRoomID:      conn.ToRoom,
			Bidirectional: true,
			Required:      conn.IsMainPath,   // Main path connections are required
			Type:          conn.PhysicalHint, // Store physical hint in Type field for DoorInfo
		}
	}

	return &entities.Dungeon{
		ID:            o.dungeonIDGen.Generate(),
		EncounterID:   encounterID,
		Seed:          seed,
		Connections:   connections,
		StartRoomID:   genDungeon.StartRoom,
		BossRoomID:    genDungeon.BossRoom,
		Rooms:         rooms,
		State:         entities.DungeonStateActive,
		CurrentRoomID: genDungeon.StartRoom,
		RevealedRooms: map[string]bool{genDungeon.StartRoom: true},
		OpenDoors:     make(map[string]bool),
		CreatedAt:     time.Now(),
	}
}

// findRoom finds a room by ID in the generated dungeon
func (o *Orchestrator) findRoom(genDungeon *dungeon.Dungeon, roomID string) *dungeon.Room {
	for _, room := range genDungeon.Rooms {
		if room.ID == roomID {
			return room
		}
	}
	return nil
}

// convertToRoomData converts a dungeon.Room to spatial.RoomData
func (o *Orchestrator) convertToRoomData(encounterID string, room *dungeon.Room) *spatial.RoomData {
	roomData := &spatial.RoomData{
		ID:       encounterID + "-" + room.ID,
		Type:     "dungeon",
		Width:    room.Shape.Width,
		Height:   room.Shape.Height,
		GridType: spatial.GridTypeHex,
		Entities: make(map[string]spatial.EntityPlacement),
	}

	// Add obstacles from features
	if room.Features != nil {
		for _, obstacle := range room.Features.Obstacles {
			roomData.Entities[obstacle.ID] = spatial.EntityPlacement{
				EntityID:       obstacle.ID,
				EntityType:     "obstacle",
				Position:       spatial.Position{X: float64(obstacle.Position.X), Y: float64(obstacle.Position.Y)},
				Size:           1,
				BlocksMovement: obstacle.BlocksMovement,
			}
		}
	}

	return roomData
}

// getSpawnPositions extracts player spawn positions from a room
func (o *Orchestrator) getSpawnPositions(room *dungeon.Room) []spatial.Position {
	var positions []spatial.Position

	for _, zone := range room.SpawnZones {
		if zone.Type == dungeon.ZoneTypePlayerSpawn || zone.Type == dungeon.ZoneTypeEntrance {
			for _, pos := range zone.Bounds {
				positions = append(positions, spatial.Position{
					X: float64(pos.X),
					Y: float64(pos.Y),
				})
				if len(positions) >= 4 {
					return positions
				}
			}
		}
	}

	// Fallback: if no spawn zones, use default positions
	if len(positions) == 0 {
		positions = []spatial.Position{
			{X: 2, Y: 2},
			{X: 2, Y: 4},
			{X: 4, Y: 2},
			{X: 4, Y: 4},
		}
	}

	return positions
}

// placeMonsters places monsters from the room's encounter into the room data
// Uses the monster factory to create theme-appropriate monsters (skeletons for crypt, etc.)
func (o *Orchestrator) placeMonsters(roomData *spatial.RoomData, room *dungeon.Room) []*monster.Data {
	if room.Encounter == nil {
		return nil
	}

	factory := dungeon.NewMonsterFactory()
	monsters := make([]*monster.Data, 0, len(room.Encounter.Monsters))
	for _, placement := range room.Encounter.Monsters {
		monsterID := fmt.Sprintf("monster-%s", placement.ID)

		// Add to room entities
		roomData.Entities[monsterID] = spatial.EntityPlacement{
			EntityID:       monsterID,
			EntityType:     entityTypeMonster,
			Position:       spatial.Position{X: float64(placement.Position.X), Y: float64(placement.Position.Y)},
			Size:           1,
			BlocksMovement: true,
		}

		// Create monster using factory based on MonsterID from theme
		m := factory.CreateMonster(monsterID, placement.MonsterID)
		monsters = append(monsters, m.ToData())
	}

	return monsters
}

// getDoorInfoForRoom extracts door information for a room using stored entity connections
func (o *Orchestrator) getDoorInfoForRoom(dungeonEntity *entities.Dungeon, roomID string) []DoorInfo {
	var doors []DoorInfo

	for _, conn := range dungeonEntity.Connections {
		if conn.FromRoomID == roomID || conn.ToRoomID == roomID {
			targetRoomID := conn.ToRoomID
			if conn.ToRoomID == roomID {
				targetRoomID = conn.FromRoomID
			}

			doors = append(doors, DoorInfo{
				ConnectionID: conn.ID,
				TargetRoomID: targetRoomID,
				Direction:    conn.Type, // Physical hint is stored in Type field
				IsOpen:       false,
			})
		}
	}

	return doors
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

	// 9. Publish MovementCompleted event
	o.publishEvent(ctx, input.EncounterID, entities.EventTypeMovementCompleted, &entities.MovementCompletedEvent{
		EntityID:          input.EntityID,
		EntityType:        entityPlacement.EntityType,
		FinalPosition:     &Position{X: targetPos.X, Y: targetPos.Y},
		MovementRemaining: movementRemaining,
		StopReason:        "completed",
	})

	// 10. Return success with updated position
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

		// Check for TPK after monster turns
		o.checkAndHandleFailure(ctx, input.EncounterID, encOutput.Data)
	}

	// 9. Persist updated state (including monster state and position changes from their turns)
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID:       input.EncounterID,
		InitiativeData:    initiativeData,
		MovementRemaining: ptrInt32(defaultMovementSpeed),   // Reset movement for new turn
		ActionEconomy:     entities.NewActionEconomyState(), // Reset action economy for new turn
		Monsters:          encOutput.Data.Monsters,          // Persist monster HP/state changes
		RoomData:          encOutput.Data.RoomData,          // Persist monster position changes
		CharacterHP:       encOutput.Data.CharacterHP,       // Persist character HP changes from monster attacks
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
		ActionEconomy:     entities.NewActionEconomyState(), // Fresh action economy for new turn
		CombatStarted:     true,
		CombatEnded:       combatEnded,
	}

	// 11. Publish events
	// Publish TurnEnded event
	o.publishEvent(ctx, input.EncounterID, entities.EventTypeTurnEnded, &entities.TurnEndedEvent{
		PreviousEntityID: previousEntityID,
		NextEntityID:     nextEntityID,
		Round:            initiativeData.Round,
		NewRound:         newRound,
		CombatState:      combatState,
	})

	// Publish MonsterTurnCompleted events for each monster turn
	for _, mt := range monsterTurns {
		// Convert actions to interface{} slice
		actions := make([]interface{}, len(mt.Actions))
		for i, a := range mt.Actions {
			actions[i] = a
		}
		// Convert movement to interface{} slice
		movement := make([]interface{}, len(mt.Movement))
		for i, m := range mt.Movement {
			movement[i] = m
		}
		o.publishEvent(ctx, input.EncounterID, entities.EventTypeMonsterTurnCompleted, &entities.MonsterTurnCompletedEvent{
			MonsterID:   mt.MonsterID,
			MonsterName: mt.MonsterName,
			Actions:     actions,
			Movement:    movement,
		})
	}

	// Publish CombatEnded event if combat ended
	if combatEnded {
		o.publishEvent(ctx, input.EncounterID, entities.EventTypeCombatEnded, &entities.CombatEndedEvent{
			EncounterResult: encounterResult,
		})
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

// Action cost types for features
type actionCostType int

const (
	actionCostNone        actionCostType = iota // No action required (free action)
	actionCostAction                            // Requires standard action
	actionCostBonusAction                       // Requires bonus action
	actionCostReaction                          // Requires reaction
)

// getFeatureActionCost returns the action cost for a feature.
// This maps D&D 5e feature activation costs.
func getFeatureActionCost(featureID string) actionCostType {
	// D&D 5e feature action costs
	switch featureID {
	// Bonus action features
	case "rage", "second-wind", "step-of-the-wind", "patient-defense", "flurry-of-blows":
		return actionCostBonusAction

	// Free abilities (no action economy cost)
	// Action Surge: Used "on your turn" without requiring action, bonus action, or reaction.
	// Note: Usage limits (per rest) are tracked separately by the feature itself.
	case "action-surge":
		return actionCostNone

	default:
		// Default to no action cost for unknown features.
		// This allows features to be used without action economy enforcement
		// until they are explicitly mapped. New features should be added above.
		return actionCostNone
	}
}

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

// buildCombatState creates a CombatState from encounter data
func (o *Orchestrator) buildCombatState(encounterID string, enc *encounterrepo.EncounterData) *CombatState {
	if enc.InitiativeData == nil {
		return nil
	}

	turnOrder := buildTurnOrderFromData(enc.InitiativeData, enc.InitiativeRolls, enc.RoomData)

	return &CombatState{
		EncounterID:       encounterID,
		Round:             enc.InitiativeData.Round,
		TurnOrder:         turnOrder,
		ActiveIndex:       enc.InitiativeData.Current,
		MovementRemaining: enc.MovementRemaining,
		CombatStarted:     enc.State == encounterrepo.StateActive || enc.State == encounterrepo.StatePaused,
		CombatEnded:       enc.State == encounterrepo.StateCompleted,
	}
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

	// 2. Load encounter to verify it exists and get action economy
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load encounter: %w", err)
	}
	if encOutput.Data == nil {
		return nil, fmt.Errorf("encounter not found: %s", input.EncounterID)
	}

	// 2a. Load action economy (initialize if not present for backwards compatibility)
	actionEconomy := encOutput.Data.ActionEconomy
	if actionEconomy == nil {
		actionEconomy = entities.NewActionEconomyState()
	}

	// 2b. Check action economy based on feature's action cost
	actionCost := getFeatureActionCost(input.FeatureID)
	switch actionCost {
	case actionCostAction:
		if !actionEconomy.HasAction() {
			return &ActivateFeatureOutput{
				Success: false,
				Message: "no action available: action already used this turn",
			}, nil
		}
	case actionCostBonusAction:
		if !actionEconomy.HasBonusAction() {
			return &ActivateFeatureOutput{
				Success: false,
				Message: "no bonus action available: bonus action already used this turn",
			}, nil
		}
	case actionCostReaction:
		if !actionEconomy.HasReaction() {
			return &ActivateFeatureOutput{
				Success: false,
				Message: "no reaction available: reaction already used this turn",
			}, nil
		}
	case actionCostNone:
		// No action economy check required for free abilities
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

	// 10. Consume the action based on feature's action cost
	switch actionCost {
	case actionCostAction:
		actionEconomy.UseAction()
	case actionCostBonusAction:
		actionEconomy.UseBonusAction()
	case actionCostReaction:
		actionEconomy.UseReaction()
	case actionCostNone:
		// No action to consume for free abilities
	}

	// 11. Convert back to data (includes new condition in Conditions slice)
	updatedData := char.ToData()

	// 12. Persist updated character
	_, err = o.charRepo.Update(ctx, characterrepo.UpdateInput{
		CharacterData: updatedData,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save character: %w", err)
	}

	// 13. Persist consumed action economy
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID:   input.EncounterID,
		ActionEconomy: actionEconomy,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save action economy: %w", err)
	}

	// 14. Publish FeatureActivated event
	o.publishEvent(ctx, input.EncounterID, entities.EventTypeFeatureActivated, &entities.FeatureActivatedEvent{
		CharacterID:   input.CharacterID,
		FeatureID:     input.FeatureID,
		Success:       true,
		Message:       fmt.Sprintf("%s activated successfully", input.FeatureID),
		CharacterData: updatedData,
	})

	// 15. Return success with updated data
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

// checkBossesDefeated checks if all boss monsters are dead
// Returns (allDead, bossInfo) where bossInfo contains the last boss for event data
func (o *Orchestrator) checkBossesDefeated(enc *encounterrepo.EncounterData) (bool, *monster.Data) {
	if len(enc.BossMonsterIDs) == 0 {
		return false, nil // No bosses tracked, victory not possible this way
	}

	var lastBoss *monster.Data
	for _, bossID := range enc.BossMonsterIDs {
		bossData := o.findMonsterData(enc, bossID)
		if bossData == nil {
			// Boss not found - shouldn't happen, but treat as dead
			continue
		}
		if bossData.HitPoints > 0 {
			// At least one boss is still alive
			return false, nil
		}
		lastBoss = bossData
	}

	// All bosses are dead
	return true, lastBoss
}

// checkAndHandleVictory checks if the killed monster was the last boss and handles victory
func (o *Orchestrator) checkAndHandleVictory(
	ctx context.Context,
	encounterID string,
	enc *encounterrepo.EncounterData,
	killedMonsterID string,
) {
	// Check if the killed monster was a boss
	isBoss := false
	for _, bossID := range enc.BossMonsterIDs {
		if bossID == killedMonsterID {
			isBoss = true
			break
		}
	}

	if !isBoss {
		return // Not a boss, no victory check needed
	}

	// Check if all bosses are now dead
	allBossesDead, bossData := o.checkBossesDefeated(enc)
	if !allBossesDead {
		return // Some bosses still alive
	}

	// Load dungeon to update its state
	dungeonOutput, err := o.dungeonRepo.GetByEncounterID(ctx, &dungeonrepo.GetByEncounterIDInput{
		EncounterID: encounterID,
	})
	if err != nil {
		// Log error but don't fail the attack
		fmt.Printf("failed to load dungeon for victory check: %v\n", err)
		return
	}
	dng := dungeonOutput.Dungeon

	// Mark dungeon as victorious
	dng.MarkVictory()
	dng.MonstersKilled++ // Increment for the boss that just died

	// Persist dungeon state
	_, err = o.dungeonRepo.Update(ctx, &dungeonrepo.UpdateInput{
		DungeonID:      dng.ID,
		State:          &dng.State,
		CompletedAt:    dng.CompletedAt,
		MonstersKilled: &dng.MonstersKilled,
	})
	if err != nil {
		fmt.Printf("failed to update dungeon state for victory: %v\n", err)
		return
	}

	// Get boss name for event
	bossName := "Boss"
	if bossData != nil {
		bossName = bossData.Name
	}

	// Publish victory event
	o.publishEvent(ctx, encounterID, entities.EventTypeDungeonVictory, &entities.DungeonVictoryEvent{
		DungeonID:      dng.ID,
		BossID:         killedMonsterID,
		BossName:       bossName,
		MonstersKilled: dng.MonstersKilled,
		RoomsExplored:  len(dng.RevealedRooms),
	})
}

// checkAllCharactersDead checks if all characters in the encounter are at 0 HP
func (o *Orchestrator) checkAllCharactersDead(enc *encounterrepo.EncounterData) bool {
	if len(enc.CharacterHP) == 0 {
		return false // No characters tracked, can't be TPK
	}

	for _, hp := range enc.CharacterHP {
		if hp > 0 {
			return false // At least one character still alive
		}
	}

	return true // All characters at 0 HP = TPK
}

// checkAndHandleFailure checks if all characters are down (TPK) and handles failure
func (o *Orchestrator) checkAndHandleFailure(
	ctx context.Context,
	encounterID string,
	enc *encounterrepo.EncounterData,
) {
	if !o.checkAllCharactersDead(enc) {
		return // Not a TPK
	}

	// Load dungeon to update its state
	dungeonOutput, err := o.dungeonRepo.GetByEncounterID(ctx, &dungeonrepo.GetByEncounterIDInput{
		EncounterID: encounterID,
	})
	if err != nil {
		fmt.Printf("failed to load dungeon for TPK check: %v\n", err)
		return
	}
	dng := dungeonOutput.Dungeon

	// Mark dungeon as failed
	dng.MarkFailed()

	// Persist dungeon state
	_, err = o.dungeonRepo.Update(ctx, &dungeonrepo.UpdateInput{
		DungeonID:   dng.ID,
		State:       &dng.State,
		CompletedAt: dng.CompletedAt,
	})
	if err != nil {
		fmt.Printf("failed to update dungeon state for TPK: %v\n", err)
		return
	}

	// Update encounter state to completed
	completedState := encounterrepo.StateCompleted
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID: encounterID,
		State:       &completedState,
	})
	if err != nil {
		fmt.Printf("failed to update encounter state for TPK: %v\n", err)
		return
	}

	// Publish failure event
	o.publishEvent(ctx, encounterID, entities.EventTypeDungeonFailure, &entities.DungeonFailureEvent{
		DungeonID: dng.ID,
		Reason:    "tpk", // Total party kill
	})
}

// OpenDoor opens a door to reveal the connected room and adds its monsters to combat
func (o *Orchestrator) OpenDoor(
	ctx context.Context,
	input *OpenDoorInput,
) (*OpenDoorOutput, error) {
	// 1. Validate input
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DungeonID == "" {
		return nil, fmt.Errorf("dungeon ID is required")
	}
	if input.ConnectionID == "" {
		return nil, fmt.Errorf("connection ID is required")
	}

	// 2. Load dungeon by ID
	dungeonOutput, err := o.dungeonRepo.Get(ctx, &dungeonrepo.GetInput{
		DungeonID: input.DungeonID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load dungeon: %w", err)
	}
	dng := dungeonOutput.Dungeon

	// 3. Find the connection
	var connection *environments.ConnectionEdge
	for _, conn := range dng.Connections {
		if conn.ID == input.ConnectionID {
			connection = conn
			break
		}
	}
	if connection == nil {
		return nil, fmt.Errorf("connection not found: %s", input.ConnectionID)
	}

	// 4. Check if door is already open
	if dng.IsDoorOpen(input.ConnectionID) {
		return nil, fmt.Errorf("door is already open")
	}

	// 5. Determine which room to reveal (the one that's NOT already revealed)
	var revealedRoomID string
	room1Revealed := dng.IsRoomRevealed(connection.FromRoomID)
	room2Revealed := dng.IsRoomRevealed(connection.ToRoomID)

	if room1Revealed && room2Revealed {
		return nil, fmt.Errorf("both rooms already revealed")
	}
	if !room1Revealed && !room2Revealed {
		// This shouldn't happen in normal gameplay (start room is always revealed)
		// but handle it defensively by revealing the "from" room
		return nil, fmt.Errorf("neither room is revealed - invalid dungeon state")
	}
	if !room1Revealed {
		revealedRoomID = connection.FromRoomID
	} else {
		revealedRoomID = connection.ToRoomID
	}

	// 6. Get the room data
	revealedRoom := dng.GetRoom(revealedRoomID)
	if revealedRoom == nil {
		return nil, fmt.Errorf("room not found: %s", revealedRoomID)
	}

	// 7. Load encounter data (using dungeon's associated encounter)
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: dng.EncounterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load encounter: %w", err)
	}

	// 8. Create monsters and roll initiative
	var monsters []MonsterInfo
	var newInitiativeRolls []initiative.Roll
	var newBossMonsterIDs []string
	factory := dungeon.NewMonsterFactory()
	isBossRoom := dng.IsBossRoom(revealedRoomID)

	if revealedRoom.Encounter != nil {
		for _, placement := range revealedRoom.Encounter.Monsters {
			monsterID := fmt.Sprintf("monster-%s", placement.ID)
			m := factory.CreateMonster(monsterID, placement.MonsterID)
			monsterData := m.ToData()

			// Track boss monsters in the boss room
			if isBossRoom && placement.Role == dungeon.RoleBoss {
				newBossMonsterIDs = append(newBossMonsterIDs, monsterID)
			}

			// Roll initiative for this monster
			dexMod := monsterData.AbilityScores.Modifier(abilities.DEX)
			roll, rollErr := o.roller.Roll(ctx, 20) // d20 roll
			if rollErr != nil {
				return nil, fmt.Errorf("failed to roll initiative for monster %s: %w", monsterID, rollErr)
			}
			total := roll + dexMod

			monsters = append(monsters, MonsterInfo{
				ID:         monsterID,
				MonsterID:  placement.MonsterID,
				Name:       monsterData.Name,
				Position:   &Position{X: float64(placement.Position.X), Y: float64(placement.Position.Y)},
				HP:         monsterData.HitPoints,
				MaxHP:      monsterData.MaxHitPoints,
				Initiative: total,
			})

			newInitiativeRolls = append(newInitiativeRolls, initiative.Roll{
				Entity:   initiative.NewParticipant(monsterID, entityTypeMonster),
				Roll:     roll,
				Modifier: dexMod,
				Total:    total,
			})

			// Add monster to encounter data
			encOutput.Data.Monsters = append(encOutput.Data.Monsters, monsterData)
		}
	}

	// 9. Validate initiative data exists
	if encOutput.Data.InitiativeData == nil {
		return nil, fmt.Errorf("no initiative data for encounter: %s", dng.EncounterID)
	}
	if len(encOutput.Data.InitiativeData.Order) == 0 {
		return nil, fmt.Errorf("empty turn order for encounter: %s", dng.EncounterID)
	}
	if encOutput.Data.InitiativeData.Current >= len(encOutput.Data.InitiativeData.Order) {
		return nil, fmt.Errorf("invalid current index for encounter: %s", dng.EncounterID)
	}

	// 10. Merge initiative orders - create new slice to avoid modifying original
	allRolls := make([]initiative.Roll, 0, len(encOutput.Data.InitiativeRolls)+len(newInitiativeRolls))
	allRolls = append(allRolls, encOutput.Data.InitiativeRolls...)
	allRolls = append(allRolls, newInitiativeRolls...)

	// Sort by total (descending)
	sort.Slice(allRolls, func(i, j int) bool {
		if allRolls[i].Total != allRolls[j].Total {
			return allRolls[i].Total > allRolls[j].Total
		}
		// Tie-breaker: higher modifier goes first
		return allRolls[i].Modifier > allRolls[j].Modifier
	})

	// Build new initiative order
	newOrder := make([]initiative.EntityData, len(allRolls))
	for i, roll := range allRolls {
		newOrder[i] = initiative.EntityData{
			ID:   roll.Entity.GetID(),
			Type: string(roll.Entity.GetType()),
		}
	}

	// Find the new current index (where the current entity moved to)
	currentEntityID := encOutput.Data.InitiativeData.Order[encOutput.Data.InitiativeData.Current].ID
	newCurrent := 0
	for i, entity := range newOrder {
		if entity.ID == currentEntityID {
			newCurrent = i
			break
		}
	}

	newInitiativeData := &initiative.TrackerData{
		Order:   newOrder,
		Current: newCurrent,
		Round:   encOutput.Data.InitiativeData.Round,
	}

	// 10. Update dungeon state (mark door open, room revealed)
	_, err = o.dungeonRepo.Update(ctx, &dungeonrepo.UpdateInput{
		DungeonID:     dng.ID,
		OpenDoors:     map[string]bool{input.ConnectionID: true},
		RevealedRooms: map[string]bool{revealedRoomID: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update dungeon: %w", err)
	}

	// 11. Update encounter state with new initiative and boss monster IDs
	// Merge new boss IDs with any existing ones
	allBossIDs := make([]string, 0, len(encOutput.Data.BossMonsterIDs)+len(newBossMonsterIDs))
	allBossIDs = append(allBossIDs, encOutput.Data.BossMonsterIDs...)
	allBossIDs = append(allBossIDs, newBossMonsterIDs...)
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID:    dng.EncounterID,
		InitiativeData: newInitiativeData,
		Monsters:       encOutput.Data.Monsters,
		BossMonsterIDs: allBossIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update encounter: %w", err)
	}

	// 12. Build room data for response
	roomData := &RoomData{
		ID:       revealedRoom.ID,
		Width:    revealedRoom.Shape.Width,
		Height:   revealedRoom.Shape.Height,
		Entities: make(map[string]interface{}),
	}

	// 13. Get doors for the newly revealed room
	newDoors := o.getDoorInfoForRoom(dng, revealedRoomID)

	// 14. Build combat state for response
	combatState := &CombatState{
		TurnOrder:   make([]InitiativeEntry, len(newOrder)),
		ActiveIndex: newCurrent,
		Round:       newInitiativeData.Round,
	}
	for i, entity := range newOrder {
		// Find the roll for this entity
		var initTotal int
		for _, roll := range allRolls {
			if roll.Entity.GetID() == entity.ID {
				initTotal = roll.Total
				break
			}
		}
		combatState.TurnOrder[i] = InitiativeEntry{
			EntityID:        entity.ID,
			EntityType:      entity.Type,
			InitiativeTotal: initTotal,
		}
	}

	// TODO: Implement monster turns for monsters that act before the current entity
	// For now, monsters are added to initiative but don't immediately take turns.
	// This would require checking if any new monsters have higher initiative than
	// the current entity and executing their turns.

	return &OpenDoorOutput{
		RevealedRoom: roomData,
		RoomOffset:   nil, // TODO: Calculate offset for grid merge when implementing multi-room display
		NewDoors:     newDoors,
		Monsters:     monsters,
		CombatState:  combatState,
		MonsterTurns: nil, // Monsters don't immediately act; they wait for their turn in initiative
	}, nil
}

// Multiplayer lobby methods

// CreateEncounter creates a new multiplayer encounter lobby
func (o *Orchestrator) CreateEncounter(
	ctx context.Context,
	input *CreateEncounterInput,
) (*CreateEncounterOutput, error) {
	// 1. Validate input
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.PlayerID == "" {
		return nil, fmt.Errorf("player ID is required")
	}
	if len(input.CharacterIDs) == 0 {
		return nil, fmt.Errorf("at least one character ID is required")
	}

	// 2. Generate unique encounter ID
	encounterID := o.encounterIDGen.Generate()

	// 3. Generate join code
	joinCode := encounterrepo.GenerateJoinCode()

	// 4. Create room (20x20 hex grid, no entities yet - they're placed when combat starts)
	roomData := &spatial.RoomData{
		ID:       encounterID + "-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		Entities: make(map[string]spatial.EntityPlacement),
	}

	// Add pillars for atmosphere
	// Note: Using positions that stay inside bounds after offset->cube conversion
	// For odd-q hex, z = row - (col/2), so avoid high-X low-Y combinations
	obstacles := []spatial.Position{
		{X: 5, Y: 5},   // Inner left pillar
		{X: 5, Y: 14},  // Inner left pillar (bottom)
		{X: 14, Y: 10}, // Inner right pillar (middle to ensure z >= 0)
		{X: 10, Y: 15}, // Center-bottom pillar
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

	// 5. Create host player entry with first character
	players := make(map[string]*encounterrepo.Player)
	players[input.PlayerID] = &encounterrepo.Player{
		PlayerID:    input.PlayerID,
		CharacterID: input.CharacterIDs[0], // First character for now
		IsReady:     false,
		IsConnected: true,
		JoinedAt:    time.Now(),
	}

	// 6. Save encounter to repository
	_, err := o.encRepo.Save(ctx, &encounterrepo.SaveInput{
		EncounterID: encounterID,
		RoomData:    roomData,
		State:       encounterrepo.StateWaiting,
		JoinCode:    joinCode,
		HostID:      input.PlayerID,
		Players:     players,
		CreatedAt:   time.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save encounter: %w", err)
	}

	return &CreateEncounterOutput{
		EncounterID: encounterID,
		JoinCode:    joinCode,
		Room:        roomData,
	}, nil
}

// JoinEncounter joins an existing encounter via join code
func (o *Orchestrator) JoinEncounter(
	ctx context.Context,
	input *JoinEncounterInput,
) (*JoinEncounterOutput, error) {
	// 1. Validate input
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.JoinCode == "" {
		return nil, fmt.Errorf("join code is required")
	}
	if input.PlayerID == "" {
		return nil, fmt.Errorf("player ID is required")
	}
	if len(input.CharacterIDs) == 0 {
		return nil, fmt.Errorf("at least one character ID is required")
	}

	// 2. Look up encounter by join code
	encOutput, err := o.encRepo.GetByJoinCode(ctx, &encounterrepo.GetByJoinCodeInput{
		JoinCode: input.JoinCode,
	})
	if err != nil {
		return nil, ErrEncounterNotFound
	}

	// 3. Validate encounter state
	if encOutput.Data.State != encounterrepo.StateWaiting {
		return nil, ErrCombatAlreadyStarted
	}

	// 4. Check if player is already in encounter
	if _, exists := encOutput.Data.Players[input.PlayerID]; exists {
		return nil, ErrPlayerAlreadyInEncounter
	}

	// 5. Add player to encounter
	if encOutput.Data.Players == nil {
		encOutput.Data.Players = make(map[string]*encounterrepo.Player)
	}
	encOutput.Data.Players[input.PlayerID] = &encounterrepo.Player{
		PlayerID:    input.PlayerID,
		CharacterID: input.CharacterIDs[0],
		IsReady:     false,
		IsConnected: true,
		JoinedAt:    time.Now(),
	}

	// 6. Update encounter in repository
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID: encOutput.Data.ID,
		Players:     encOutput.Data.Players,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update encounter: %w", err)
	}

	// 7. Load character data for the joining player
	charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{ID: input.CharacterIDs[0]})
	if err != nil {
		return nil, fmt.Errorf("failed to load character %s: %w", input.CharacterIDs[0], err)
	}

	// 8. Publish PlayerJoined event with character data
	o.publishEvent(ctx, encOutput.Data.ID, entities.EventTypePlayerJoined, &entities.PlayerJoinedEvent{
		PlayerID:      input.PlayerID,
		CharacterID:   input.CharacterIDs[0],
		CharacterData: charOutput.CharacterData,
	})

	// 9. Build party list for response
	party := o.buildPartyFromPlayers(ctx, encOutput.Data.Players, encOutput.Data.HostID)

	return &JoinEncounterOutput{
		EncounterID: encOutput.Data.ID,
		Room:        encOutput.Data.RoomData,
		Party:       party,
		State:       string(encOutput.Data.State),
	}, nil
}

// SetReady marks a player as ready or not ready to start combat
func (o *Orchestrator) SetReady(
	ctx context.Context,
	input *SetReadyInput,
) (*SetReadyOutput, error) {
	// 1. Validate input
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.PlayerID == "" {
		return nil, fmt.Errorf("player ID is required")
	}

	// 2. Load encounter
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, ErrEncounterNotFound
	}

	// 3. Validate state
	if encOutput.Data.State != encounterrepo.StateWaiting {
		return nil, ErrCombatAlreadyStarted
	}

	// 4. Find and update player
	player, exists := encOutput.Data.Players[input.PlayerID]
	if !exists {
		return nil, ErrPlayerNotInEncounter
	}
	player.IsReady = input.IsReady

	// 5. Update encounter
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID: input.EncounterID,
		Players:     encOutput.Data.Players,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update encounter: %w", err)
	}

	// 6. Publish PlayerReady event
	o.publishEvent(ctx, input.EncounterID, entities.EventTypePlayerReady, &entities.PlayerReadyEvent{
		PlayerID:    input.PlayerID,
		CharacterID: player.CharacterID,
		Ready:       input.IsReady,
	})

	return &SetReadyOutput{Success: true}, nil
}

// StartCombat begins combat (host only, all players must be ready)
func (o *Orchestrator) StartCombat(
	ctx context.Context,
	input *StartCombatInput,
) (*StartCombatOutput, error) {
	// 1. Validate input
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.PlayerID == "" {
		return nil, fmt.Errorf("player ID is required")
	}

	// 2. Load encounter
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, ErrEncounterNotFound
	}

	// 3. Validate state
	if encOutput.Data.State != encounterrepo.StateWaiting {
		return nil, ErrCombatAlreadyStarted
	}

	// 4. Validate caller is host
	if encOutput.Data.HostID != input.PlayerID {
		return nil, ErrNotHost
	}

	// 5. Validate all players are ready
	for _, player := range encOutput.Data.Players {
		if !player.IsReady {
			return nil, ErrPlayersNotReady
		}
	}

	// 6. Collect character IDs and place them in the room
	roomData, ok := encOutput.Data.RoomData.(*spatial.RoomData)
	if !ok {
		return nil, fmt.Errorf("invalid room data type")
	}
	characterIDs := make([]string, 0, len(encOutput.Data.Players))

	// Define spawn points for players
	spawnPoints := []spatial.Position{
		{X: 5, Y: 8},
		{X: 5, Y: 10},
		{X: 5, Y: 12},
		{X: 4, Y: 10},
	}

	i := 0
	for _, player := range encOutput.Data.Players {
		characterIDs = append(characterIDs, player.CharacterID)

		// Place character in room
		if i < len(spawnPoints) {
			roomData.Entities[player.CharacterID] = spatial.EntityPlacement{
				EntityID:       player.CharacterID,
				EntityType:     entityTypeCharacter,
				Position:       spawnPoints[i],
				Size:           1,
				BlocksMovement: true,
			}
		}
		i++
	}

	// 7. Spawn monsters equal to number of players, spread across the room
	// Monster spawn points on the right side of the room (away from players)
	monsterSpawnPoints := []spatial.Position{
		{X: 14, Y: 8},  // Right side
		{X: 14, Y: 12}, // Right side lower
		{X: 12, Y: 10}, // Center-right
		{X: 13, Y: 6},  // Right upper
	}

	numPlayers := len(encOutput.Data.Players)
	monsters := make([]*monster.Data, 0, numPlayers)

	for i := 0; i < numPlayers; i++ {
		goblinID := fmt.Sprintf("goblin-%d", i+1)
		spawnPos := monsterSpawnPoints[i%len(monsterSpawnPoints)]

		roomData.Entities[goblinID] = spatial.EntityPlacement{
			EntityID:       goblinID,
			EntityType:     entityTypeMonster,
			Position:       spawnPos,
			Size:           1,
			BlocksMovement: true,
		}

		// Create monster data for each goblin
		monsters = append(monsters, o.createGoblinData(goblinID))
	}

	// 9. Roll initiative
	participants := make(map[core.Entity]int)

	// Initialize CharacterHP map for TPK tracking
	characterHP := make(map[string]int)

	// Add characters
	for _, charID := range characterIDs {
		var charOutput *characterrepo.GetOutput
		charOutput, err = o.charRepo.Get(ctx, characterrepo.GetInput{ID: charID})
		if err != nil {
			return nil, fmt.Errorf("failed to load character %s: %w", charID, err)
		}
		dexMod := charOutput.CharacterData.AbilityScores.Modifier(abilities.DEX)
		participants[initiative.NewParticipant(charID, entityTypeCharacter)] = dexMod

		// Track character's current HP for TPK detection
		characterHP[charID] = charOutput.CharacterData.HitPoints
	}

	// Add all goblins to initiative
	for _, m := range monsters {
		participants[initiative.NewParticipant(m.ID, entityTypeMonster)] = 2 // Goblin DEX +2
	}

	// Roll initiative
	rolls := initiative.RollForOrder(participants, o.roller)

	// Create tracker
	initiativeOrder := make([]core.Entity, len(rolls))
	for i, roll := range rolls {
		initiativeOrder[i] = roll.Entity
	}
	tracker := initiative.New(initiativeOrder)
	trackerData := tracker.ToData()

	// Build turn order
	turnOrder := make([]InitiativeEntry, len(rolls))
	for i, roll := range rolls {
		entityID := roll.Entity.GetID()
		var position *Position
		if placement, exists := roomData.Entities[entityID]; exists {
			position = &Position{X: placement.Position.X, Y: placement.Position.Y}
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

	// 10. Update encounter state to active with fresh action economy
	activeState := encounterrepo.StateActive
	initialActionEconomy := entities.NewActionEconomyState()
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID:       input.EncounterID,
		State:             &activeState,
		RoomData:          roomData,
		InitiativeData:    &trackerData,
		MovementRemaining: ptrInt32(defaultMovementSpeed),
		ActionEconomy:     initialActionEconomy,
		Monsters:          monsters,    // Save all monsters
		CharacterHP:       characterHP, // Track character HP for TPK detection
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update encounter: %w", err)
	}

	// 11. Build combat state
	combatState := &CombatState{
		EncounterID:       input.EncounterID,
		Round:             1,
		TurnOrder:         turnOrder,
		ActiveIndex:       0,
		MovementRemaining: defaultMovementSpeed,
		ActionEconomy:     initialActionEconomy, // Fresh action economy for first turn
		CombatStarted:     true,
		CombatEnded:       false,
	}

	// 12. Execute monster turns if they go first
	var monsterTurns []*MonsterTurnResult
	if trackerData.Order[0].Type == entityTypeMonster {
		// Monster(s) go first - execute all monster turns until a player's turn
		encData := &encounterrepo.EncounterData{
			ID:                input.EncounterID,
			RoomData:          roomData,
			InitiativeData:    &trackerData,
			InitiativeRolls:   rolls,
			MovementRemaining: combatState.MovementRemaining,
			Monsters:          monsters,    // All monsters
			CharacterHP:       characterHP, // Track character HP for TPK
		}

		// Execute monster turns
		monsterTurns, err = o.executeMonsterTurns(ctx, encData, characterIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to execute monster turns: %w", err)
		}

		// Update combat state to reflect the new active index after monster turns
		combatState.ActiveIndex = encData.InitiativeData.Current
		combatState.Round = encData.InitiativeData.Round

		// Persist updated initiative, monster state, room positions, and character HP
		_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
			EncounterID:    input.EncounterID,
			InitiativeData: encData.InitiativeData,
			Monsters:       encData.Monsters,
			RoomData:       encData.RoomData,
			CharacterHP:    encData.CharacterHP,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to save initiative after monster turns: %w", err)
		}

		// Check for TPK after monster turns
		o.checkAndHandleFailure(ctx, input.EncounterID, encData)
	}

	// 13. Build party list for event (with character data)
	party := make([]*entities.Player, 0, len(encOutput.Data.Players))
	for _, player := range encOutput.Data.Players {
		// Load character data for the event
		charOutput, charErr := o.charRepo.Get(ctx, characterrepo.GetInput{ID: player.CharacterID})
		var charData *character.Data
		if charErr == nil && charOutput != nil {
			charData = charOutput.CharacterData
		}
		party = append(party, &entities.Player{
			PlayerID:      player.PlayerID,
			CharacterID:   player.CharacterID,
			CharacterData: charData,
			IsReady:       player.IsReady,
			IsConnected:   player.IsConnected,
			IsHost:        player.PlayerID == encOutput.Data.HostID,
			JoinedAt:      player.JoinedAt,
		})
	}

	// 13. Publish CombatStarted event
	o.publishEvent(ctx, input.EncounterID, entities.EventTypeCombatStarted, &entities.CombatStartedEvent{
		CombatState: combatState,
		Room:        roomData,
		Party:       party,
	})

	return &StartCombatOutput{
		CombatState:  combatState,
		Room:         roomData,
		MonsterTurns: monsterTurns,
	}, nil
}

// LeaveEncounter removes a player from the encounter
func (o *Orchestrator) LeaveEncounter(
	ctx context.Context,
	input *LeaveEncounterInput,
) (*LeaveEncounterOutput, error) {
	// 1. Validate input
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.PlayerID == "" {
		return nil, fmt.Errorf("player ID is required")
	}

	// 2. Load encounter
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, ErrEncounterNotFound
	}

	// 3. Check if player is in encounter
	player, exists := encOutput.Data.Players[input.PlayerID]
	if !exists {
		return nil, ErrPlayerNotInEncounter
	}
	characterID := player.CharacterID // Save before deleting

	// 4. Remove player
	delete(encOutput.Data.Players, input.PlayerID)

	// 5. If no players left, delete encounter
	if len(encOutput.Data.Players) == 0 {
		// Publish event before deleting
		o.publishEvent(ctx, input.EncounterID, entities.EventTypePlayerLeft, &entities.PlayerLeftEvent{
			PlayerID:    input.PlayerID,
			CharacterID: characterID,
			Reason:      "voluntary",
		})

		_, err = o.encRepo.Delete(ctx, &encounterrepo.DeleteInput{
			EncounterID: input.EncounterID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to delete encounter: %w", err)
		}
		return &LeaveEncounterOutput{
			Success:          true,
			EncounterDeleted: true,
		}, nil
	}

	// 6. If leaving player was host, transfer host to next player
	var newHostID *string
	if encOutput.Data.HostID == input.PlayerID {
		// Find first remaining player to be new host
		for playerID := range encOutput.Data.Players {
			newHostID = &playerID
			break
		}
	}

	// 7. Update encounter
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID: input.EncounterID,
		HostID:      newHostID,
		Players:     encOutput.Data.Players,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update encounter: %w", err)
	}

	// 8. Publish PlayerLeft event
	o.publishEvent(ctx, input.EncounterID, entities.EventTypePlayerLeft, &entities.PlayerLeftEvent{
		PlayerID:    input.PlayerID,
		CharacterID: characterID,
		Reason:      "voluntary",
	})

	return &LeaveEncounterOutput{
		Success:          true,
		EncounterDeleted: false,
	}, nil
}

// PlayerDisconnected marks a player as disconnected
// If combat is active and all remaining players are disconnected, pauses the encounter
func (o *Orchestrator) PlayerDisconnected(
	ctx context.Context,
	input *PlayerDisconnectedInput,
) (*PlayerDisconnectedOutput, error) {
	// 1. Validate input
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.PlayerID == "" {
		return nil, fmt.Errorf("player ID is required")
	}

	// 2. Load encounter
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, ErrEncounterNotFound
	}

	// 3. Check if player is in encounter
	player, exists := encOutput.Data.Players[input.PlayerID]
	if !exists {
		return nil, ErrPlayerNotInEncounter
	}

	// 4. Check if player is already disconnected
	if !player.IsConnected {
		return nil, ErrPlayerAlreadyDisconnected
	}

	// 5. Mark player as disconnected
	player.IsConnected = false
	encOutput.Data.Players[input.PlayerID] = player

	// 6. Check if we should pause (combat is active and all players disconnected)
	shouldPause := false
	if encOutput.Data.State == encounterrepo.StateActive {
		allDisconnected := true
		for _, p := range encOutput.Data.Players {
			if p.IsConnected {
				allDisconnected = false
				break
			}
		}
		if allDisconnected {
			shouldPause = true
		}
	}

	// 7. Update encounter state
	var newState *encounterrepo.EncounterState
	if shouldPause {
		paused := encounterrepo.StatePaused
		newState = &paused
	}

	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID: input.EncounterID,
		State:       newState,
		Players:     encOutput.Data.Players,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update encounter: %w", err)
	}

	// 8. Publish PlayerDisconnected event
	reason := input.Reason
	if reason == "" {
		reason = "unknown"
	}
	o.publishEvent(ctx, input.EncounterID, entities.EventTypePlayerDisconnected, &entities.PlayerDisconnectedEvent{
		PlayerID:    input.PlayerID,
		CharacterID: player.CharacterID,
		Reason:      reason,
	})

	// 9. Publish CombatPaused event if we paused
	if shouldPause {
		o.publishEvent(ctx, input.EncounterID, entities.EventTypeCombatPaused, &entities.CombatPausedEvent{
			PausedBy: input.PlayerID,
			Reason:   "all players disconnected",
		})
	}

	// Determine current state for response
	currentState := string(encOutput.Data.State)
	if shouldPause {
		currentState = string(encounterrepo.StatePaused)
	}

	return &PlayerDisconnectedOutput{
		Success:         true,
		EncounterPaused: shouldPause,
		State:           currentState,
	}, nil
}

// PlayerReconnected marks a player as reconnected
// If encounter was paused due to disconnection, resumes when a player reconnects
func (o *Orchestrator) PlayerReconnected(
	ctx context.Context,
	input *PlayerReconnectedInput,
) (*PlayerReconnectedOutput, error) {
	// 1. Validate input
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.PlayerID == "" {
		return nil, fmt.Errorf("player ID is required")
	}

	// 2. Load encounter
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, ErrEncounterNotFound
	}

	// 3. Check if player is in encounter
	player, exists := encOutput.Data.Players[input.PlayerID]
	if !exists {
		return nil, ErrPlayerNotInEncounter
	}

	// 4. Check if player is already connected
	if player.IsConnected {
		return nil, ErrPlayerAlreadyConnected
	}

	// 5. Mark player as connected
	player.IsConnected = true
	encOutput.Data.Players[input.PlayerID] = player

	// 6. Check if we should resume (encounter was paused)
	shouldResume := encOutput.Data.State == encounterrepo.StatePaused

	// 7. Update encounter state
	var newState *encounterrepo.EncounterState
	if shouldResume {
		active := encounterrepo.StateActive
		newState = &active
	}

	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID: input.EncounterID,
		State:       newState,
		Players:     encOutput.Data.Players,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update encounter: %w", err)
	}

	// 8. Publish PlayerReconnected event
	o.publishEvent(ctx, input.EncounterID, entities.EventTypePlayerReconnected, &entities.PlayerReconnectedEvent{
		PlayerID:    input.PlayerID,
		CharacterID: player.CharacterID,
	})

	// 9. Publish CombatResumed event if we resumed
	if shouldResume {
		o.publishEvent(ctx, input.EncounterID, entities.EventTypeCombatResumed, &entities.CombatResumedEvent{
			ResumedBy: input.PlayerID,
		})
	}

	// Determine current state for response
	currentState := string(encOutput.Data.State)
	if shouldResume {
		currentState = string(encounterrepo.StateActive)
	}

	// Build combat state if we resumed and have initiative data
	var combatState *CombatState
	if shouldResume && encOutput.Data.InitiativeData != nil {
		combatState = o.buildCombatState(input.EncounterID, encOutput.Data)
	}

	return &PlayerReconnectedOutput{
		Success:          true,
		EncounterResumed: shouldResume,
		State:            currentState,
		CombatState:      combatState,
	}, nil
}

// buildPartyFromPlayers converts repository player data to PartyMember list
func (o *Orchestrator) buildPartyFromPlayers(
	ctx context.Context,
	players map[string]*encounterrepo.Player,
	hostID string,
) []*PartyMember {
	party := make([]*PartyMember, 0, len(players))

	for _, player := range players {
		member := &PartyMember{
			PlayerID:    player.PlayerID,
			CharacterID: player.CharacterID,
			IsHost:      player.PlayerID == hostID,
			IsReady:     player.IsReady,
			IsConnected: player.IsConnected,
		}

		// Try to load character data
		charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{ID: player.CharacterID})
		if err == nil {
			member.CharacterData = charOutput.CharacterData
		}

		party = append(party, member)
	}

	return party
}

// createGoblinData creates a goblin monster for encounters
func (o *Orchestrator) createGoblinData(goblinID string) *monster.Data {
	goblinMonster := monster.NewGoblin(goblinID)
	goblinMonster.AddAction(monster.NewScimitarAction(monster.ScimitarConfig{
		AttackBonus: 4,       // +2 DEX + 2 proficiency
		DamageDice:  "1d6+2", // Scimitar 1d6 + DEX
		DamageBonus: 2,       // DEX modifier
	}))
	data := goblinMonster.ToData()
	return data
}
