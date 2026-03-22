// Package encounter provides orchestration for D&D 5e encounter management and combat resolution.
package encounter

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/dice"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/actions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/armor"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	dnd5eEvents "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/gamectx"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monstertraits"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	dungeontoolkit "github.com/KirkDiggler/rpg-api/internal/components/dungeon/toolkit"
	"github.com/KirkDiggler/rpg-api/internal/components/spawner"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	eventprocessor "github.com/KirkDiggler/rpg-api/internal/processors/event"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	dungeonrepo "github.com/KirkDiggler/rpg-api/internal/repositories/dungeons"
	encounterlogrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounterlog"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
)

// Config holds orchestrator dependencies
type Config struct {
	CharacterRepo    characterrepo.Repository
	EncounterRepo    encounterrepo.Repository
	DungeonRepo      dungeonrepo.Repository      // Required: for dungeon persistence
	DungeonGen       *dungeon.Generator          // Required: for procedural dungeon generation
	EventProcessor   eventprocessor.Processor    // Optional: for persisting and publishing events
	EncounterLogRepo encounterlogrepo.Repository // Optional: for reading event history
	Roller           dice.Roller                 // Optional: for dice rolls (defaults to random roller)
	Spawner          spawner.Spawner             // Optional: for entity placement (defaults to DefaultSpawner)
	EncounterIDGen   idgen.Generator             // Optional: for generating encounter IDs (defaults to "enc-" prefix)
	DungeonIDGen     idgen.Generator             // Optional: for generating dungeon IDs (defaults to "dng-" prefix)
	ConnectionIDGen  idgen.Generator             // Optional: for generating connection IDs (defaults to "conn-" prefix)
}

// Validate ensures all required dependencies are present
func (c *Config) Validate() error {
	if c.CharacterRepo == nil {
		return fmt.Errorf("CharacterRepo is required")
	}
	if c.EncounterRepo == nil {
		return fmt.Errorf("EncounterRepo is required")
	}
	if c.DungeonRepo == nil {
		return fmt.Errorf("DungeonRepo is required")
	}
	if c.DungeonGen == nil {
		return fmt.Errorf("DungeonGen is required")
	}
	return nil
}

// Orchestrator implements the Service interface
type Orchestrator struct {
	charRepo         characterrepo.Repository
	encRepo          encounterrepo.Repository
	dungeonRepo      dungeonrepo.Repository // Required: for dungeon persistence
	dungeonGen       *dungeon.Generator     // Required: for procedural dungeon generation
	roller           dice.Roller
	spawner          spawner.Spawner             // For entity placement
	eventProcessor   eventprocessor.Processor    // Optional: for persisting and publishing events
	encounterLogRepo encounterlogrepo.Repository // Optional: for reading event history
	encounterIDGen   idgen.Generator             // For generating encounter IDs
	dungeonIDGen     idgen.Generator             // For generating dungeon IDs
	connectionIDGen  idgen.Generator             // For generating connection IDs
}

// New creates a new encounter orchestrator
func New(cfg *Config) (*Orchestrator, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
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
	roller := cfg.Roller
	if roller == nil {
		roller = dice.NewRoller()
	}
	spwn := cfg.Spawner
	if spwn == nil {
		spwn = spawner.NewSpawner()
	}

	return &Orchestrator{
		charRepo:         cfg.CharacterRepo,
		encRepo:          cfg.EncounterRepo,
		dungeonRepo:      cfg.DungeonRepo,
		dungeonGen:       cfg.DungeonGen,
		roller:           roller,
		spawner:          spwn,
		eventProcessor:   cfg.EventProcessor,
		encounterLogRepo: cfg.EncounterLogRepo,
		encounterIDGen:   encounterIDGen,
		dungeonIDGen:     dungeonIDGen,
		connectionIDGen:  connectionIDGen,
	}, nil
}

// publishEvent persists and publishes an event if EventProcessor is configured.
// Also updates the encounter's LastEventID for the load-then-stream pattern.
// Errors are logged but not returned to avoid breaking combat flow.
func (o *Orchestrator) publishEvent(ctx context.Context, encounterID string, eventType entities.EventType, data interface{}) {
	if o.eventProcessor == nil {
		return
	}

	event := &entities.EncounterEvent{
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
	case *entities.DungeonVictoryEvent:
		event.DungeonVictory = v
	case *entities.DungeonFailureEvent:
		event.DungeonFailure = v
	case *entities.RoomRevealedEvent:
		event.RoomRevealed = v
	default:
		fmt.Printf("unknown event data type for %s: %T\n", eventType, data)
		return
	}

	// Process persists to encounter log and publishes to subscribers
	output, err := o.eventProcessor.Process(ctx, &eventprocessor.ProcessInput{
		EncounterID: encounterID,
		Event:       event,
	})
	if err != nil {
		// Log but don't fail - event processing is non-critical to combat flow
		fmt.Printf("failed to process %s event for encounter %s: %v\n", eventType, encounterID, err)
		return
	}

	// Update LastEventID in the encounter record for load-then-stream pattern
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID: encounterID,
		LastEventID: &output.EventID,
	})
	if err != nil {
		// Log but don't fail - this is non-critical for the current operation
		fmt.Printf("failed to update LastEventID for encounter %s: %v\n", encounterID, err)
	}
}

// getCurrentRoomOrigin returns the origin of the current room from the dungeon entity.
// Returns nil if the dungeon or room origins are not available.
func getCurrentRoomOrigin(dungeonEntity *entities.Dungeon) *dungeon.Position {
	if dungeonEntity == nil || dungeonEntity.RoomOrigins == nil {
		return nil
	}
	currentRoomID := dungeonEntity.CurrentRoomID
	if currentRoomID == "" {
		currentRoomID = dungeonEntity.StartRoomID
	}
	if origin, ok := dungeonEntity.RoomOrigins[currentRoomID]; ok {
		return &origin
	}
	return nil
}

// publishCharacterTurnEnd publishes a TurnEndEvent to the character's event bus.
// This allows conditions like Rage and Sneak Attack to process turn-end logic.
// The character is loaded with their persisted condition state, the event is published,
// and the updated state is persisted back.
func (o *Orchestrator) publishCharacterTurnEnd(ctx context.Context, characterID string, round int) error {
	// 1. Load character data from repository
	charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{
		ID: characterID,
	})
	if err != nil {
		return fmt.Errorf("failed to load character: %w", err)
	}

	// 2. Create fresh event bus and load character (conditions subscribe to bus)
	bus := events.NewEventBus()
	char, err := character.LoadFromData(ctx, charOutput.Character.Data, bus)
	if err != nil {
		return fmt.Errorf("failed to load character from data: %w", err)
	}

	// 3. Publish TurnEndEvent to the character's event bus
	// Conditions like Rage subscribe to this and will:
	// - Track turn count
	// - Check if attack/damage occurred (Rage maintenance)
	// - Reset per-turn flags (Sneak Attack)
	turnEndTopic := dnd5eEvents.TurnEndTopic.On(bus)
	err = turnEndTopic.Publish(ctx, dnd5eEvents.TurnEndEvent{
		CharacterID: characterID,
		Round:       round,
	})
	if err != nil {
		return fmt.Errorf("failed to publish turn end event: %w", err)
	}

	// 4. Persist updated character state (conditions may have changed)
	updatedData := char.ToData()
	_, err = o.charRepo.Update(ctx, characterrepo.UpdateInput{
		Character: &entities.Character{
			Data:       updatedData,
			Appearance: charOutput.Character.Appearance,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to persist character state: %w", err)
	}

	return nil
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
	char, err := character.LoadFromData(ctx, charOutput.Character.Data, bus)
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

	// Load monster conditions/traits (vulnerability, immunity, etc.) so they affect combat
	if err = monstertraits.LoadMonsterConditions(ctx, goblin, monsterData.Conditions, bus, o.roller); err != nil {
		return nil, fmt.Errorf("failed to load monster conditions: %w", err)
	}

	// 7. Get weapon and equipment slots from equipped items (with fallback to greataxe)
	weapon, equipmentSlots := o.getEquippedWeaponAndSlots(ctx, input.AttackerID)

	// 8. Build GameContext with character equipment for fighting style checks (e.g., Dueling)
	gameCtx := o.buildGameContextFromEquipment(input.AttackerID, &weapon, equipmentSlots)
	ctx = gamectx.WithGameContext(ctx, gameCtx)

	// 9. Create CombatantRegistry for damage chain lookups (vulnerability, resistance, etc.)
	// The registry allows traits to look up combatants during damage resolution.
	registry := gamectx.NewCombatantRegistry()
	registry.Add(char)
	registry.Add(goblin)
	// Use combat.WithCombatantLookup for the new ID-based lookup API
	ctx = combat.WithCombatantLookup(ctx, registry)

	// 10. Call toolkit combat (event-driven, Rage and fighting styles participate here!)
	// Damage is applied to the monster via DamageReceivedEvent -> monster.TakeDamage
	// New API uses IDs - combatants are looked up from context
	result, err := combat.ResolveAttack(ctx, &combat.AttackInput{
		AttackerID: input.AttackerID,
		TargetID:   input.TargetID,
		Weapon:     &weapon,
		EventBus:   bus,
		Roller:     o.roller,
		AttackHand: input.AttackHand, // For two-weapon fighting
	})
	if err != nil {
		return nil, fmt.Errorf("combat resolution failed: %w", err)
	}

	// 11. Get monster HP after damage was applied via event-driven flow
	// ResolveAttack publishes DamageReceivedEvent which monster.TakeDamage handles
	// Damage includes vulnerability/resistance multipliers from the damage chain
	newHP := goblin.HP()
	if result.Hit {
		// Update monster data with new HP from the monster instance
		monsterData.HitPoints = newHP
	}

	// 11. Grant bonus strike after main-hand attack
	// Priority: Monks get Martial Arts check first, others get TWF first
	var grantedAction *GrantedAction

	isMo := charOutput.Character.Data.ClassID == classes.Monk

	// 11a. Martial Arts: grant bonus strike after Attack action (Monk only)
	if isMo {
		maResult, _ := actions.CheckAndGrantMartialArtsBonusStrike(ctx, &actions.MartialArtsGranterInput{
			CharacterID:   input.AttackerID,
			WeaponID:      weapon.ID,
			IsUnarmed:     false, // TODO: Detect unarmed strikes when implemented
			SourceAbility: "attack",
			EventBus:      bus,
		})
		if maResult != nil && maResult.Granted {
			grantedAction = &GrantedAction{
				ID:     maResult.Action.GetID(),
				Type:   "martial-arts-bonus-strike",
				Name:   "Martial Arts Bonus Strike",
				Reason: maResult.Reason,
			}
		}
	}

	// 11b. Two-weapon fighting: grant off-hand strike after main-hand attack
	// For Monks, only triggers if Martial Arts didn't grant
	// For non-Monks, this is the primary bonus strike path
	if grantedAction == nil && equipmentSlots != nil {
		var mainWeapon, offWeapon *actions.EquippedWeaponInfo
		if mainID := equipmentSlots.Get(character.SlotMainHand); mainID != "" {
			mainWeapon = &actions.EquippedWeaponInfo{WeaponID: mainID}
		}
		if offID := equipmentSlots.Get(character.SlotOffHand); offID != "" {
			offWeapon = &actions.EquippedWeaponInfo{WeaponID: offID}
		}

		twfResult, _ := actions.CheckAndGrantOffHandStrike(ctx, &actions.TwoWeaponGranterInput{
			CharacterID:    input.AttackerID,
			AttackHand:     actions.AttackHand(input.AttackHand),
			MainHandWeapon: mainWeapon,
			OffHandWeapon:  offWeapon,
			ActionHolder:   char,
			EventBus:       bus,
		})
		if twfResult != nil && twfResult.Granted {
			grantedAction = &GrantedAction{
				ID:       twfResult.Action.GetID(),
				Type:     "off-hand-strike",
				Name:     "Off-Hand Strike",
				Reason:   twfResult.Reason,
				WeaponID: twfResult.Action.GetWeaponID(),
			}
		}
	}

	// 12. Consume action and persist (action consumed only after all validation succeeds)
	actionEconomy.UseAction()
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID:   input.EncounterID,
		ActionEconomy: actionEconomy,
		Monsters:      encOutput.Data.Monsters,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save encounter state: %w", err)
	}

	// 13. Convert toolkit result to our output format
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

	// 14. Map breakdown if present (only exists on hit)
	if result.Breakdown != nil {
		attackResult.Breakdown = convertToolkitBreakdown(result.Breakdown)
	}

	// 15. Get room data for the event (if available)
	var roomData *spatial.RoomData
	if encOutput.Data.RoomData != nil {
		if rd, ok := encOutput.Data.RoomData.(*spatial.RoomData); ok {
			roomData = rd
		}
	}

	// 16. Convert granted action for event
	var grantedActionInfo *entities.GrantedActionInfo
	if grantedAction != nil {
		grantedActionInfo = &entities.GrantedActionInfo{
			ID:       grantedAction.ID,
			Type:     grantedAction.Type,
			Name:     grantedAction.Name,
			Reason:   grantedAction.Reason,
			WeaponID: grantedAction.WeaponID,
		}
	}

	// 16b. Load dungeon to get walls and room origin for the current room
	var dungeonWalls []dungeon.WallSegment
	var attackRoomOrigin *dungeon.Position
	if o.dungeonRepo != nil {
		dungeonOutput, dungeonErr := o.dungeonRepo.GetByEncounterID(ctx, &dungeonrepo.GetByEncounterIDInput{
			EncounterID: input.EncounterID,
		})
		if dungeonErr == nil && dungeonOutput.Dungeon != nil {
			dungeonEntity := dungeonOutput.Dungeon
			attackRoomOrigin = getCurrentRoomOrigin(dungeonEntity)
			currentRoomID := dungeonEntity.CurrentRoomID
			if currentRoomID == "" {
				currentRoomID = dungeonEntity.StartRoomID
			}
			if currentRoom := dungeonEntity.GetRoom(currentRoomID); currentRoom != nil {
				dungeonWalls = currentRoom.Walls
			}
		}
	}

	// 17. Publish AttackResolved event
	o.publishEvent(ctx, input.EncounterID, entities.EventTypeAttackResolved, &entities.AttackResolvedEvent{
		AttackerID:    input.AttackerID,
		TargetID:      input.TargetID,
		Result:        attackResult,
		TargetHP:      newHP,
		TargetDead:    newHP <= 0,
		Room:          roomData,
		RoomOrigin:    attackRoomOrigin,
		Walls:         dungeonWalls,
		GrantedAction: grantedActionInfo,
	})

	// 18. Check for dungeon victory if monster died
	if newHP <= 0 {
		o.checkAndHandleVictory(ctx, input.EncounterID, encOutput.Data, input.TargetID)
	}

	return &ResolveAttackOutput{
		Result:        attackResult,
		MonsterHP:     newHP,
		MonsterDead:   newHP <= 0,
		GrantedAction: grantedAction,
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
		Multiplier:        comp.Multiplier,
	}
}

// convertDoorsToEntityDoors converts service DoorInfo to entities DoorInfo for events
func convertDoorsToEntityDoors(doors []DoorInfo) []*entities.DoorInfo {
	if len(doors) == 0 {
		return nil
	}
	result := make([]*entities.DoorInfo, len(doors))
	for i, d := range doors {
		var pos *entities.Position
		if d.Position != nil {
			pos = &entities.Position{
				X: float64(d.Position.X),
				Y: float64(d.Position.Y),
				Z: float64(d.Position.Z),
			}
		}
		result[i] = &entities.DoorInfo{
			ConnectionID: d.ConnectionID,
			TargetRoomID: d.TargetRoomID,
			Direction:    d.Direction,
			Position:     pos,
			IsOpen:       d.IsOpen,
		}
	}
	return result
}

// convertMonsterTurnsToEntityEvents converts service MonsterTurnResult to entity events
func convertMonsterTurnsToEntityEvents(turns []*MonsterTurnResult, roomData *spatial.RoomData) []*entities.MonsterTurnCompletedEvent {
	if len(turns) == 0 {
		return nil
	}
	result := make([]*entities.MonsterTurnCompletedEvent, len(turns))
	for i, mt := range turns {
		// Convert positions to entity positions
		var movement []entities.Position
		for _, pos := range mt.Movement {
			movement = append(movement, entities.Position{
				X: float64(pos.X),
				Y: float64(pos.Y),
				Z: float64(pos.Z),
			})
		}
		result[i] = &entities.MonsterTurnCompletedEvent{
			MonsterID:         mt.MonsterID,
			MonsterName:       mt.MonsterName,
			Actions:           mt.Actions,
			Movement:          movement,
			Room:              roomData,
			UpdatedCharacters: mt.UpdatedCharacters,
		}
	}
	return result
}

// CreateDungeon creates a new encounter with an initial room
func (o *Orchestrator) CreateDungeon(ctx context.Context, input *CreateDungeonInput) (*CreateDungeonOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Generate unique encounter ID
	encounterID := o.encounterIDGen.Generate()

	return o.createDungeonWithGenerator(ctx, encounterID, input)
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

	// Place characters at spawn zones using cube coordinates
	spawnPositions := o.getPlayerSpawnPositions(startRoom)
	for i, characterID := range input.CharacterIDs {
		if i >= len(spawnPositions) {
			break
		}
		roomData.CubeEntities[characterID] = spatial.EntityCubePlacement{
			EntityID:       characterID,
			EntityType:     entityTypeCharacter,
			CubePosition:   spawnPositions[i],
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
	// Also perform a long rest to restore HP, feature charges, and clear conditions
	// Single event bus shared by all characters in the encounter
	restBus := events.NewEventBus()

	for _, characterID := range input.CharacterIDs {
		charOutput, charErr := o.charRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
		if charErr != nil {
			return nil, fmt.Errorf("failed to load character %s: %w", characterID, charErr)
		}

		// Perform long rest before dungeon starts
		loadedChar, charErr := character.LoadFromData(ctx, charOutput.Character.Data, restBus)
		if charErr != nil {
			return nil, fmt.Errorf("failed to load character %s for rest: %w", characterID, charErr)
		}

		if restErr := loadedChar.LongRest(ctx); restErr != nil {
			_ = loadedChar.Cleanup(ctx)
			return nil, fmt.Errorf("failed to perform long rest for character %s: %w", characterID, restErr)
		}

		// Save the rested character
		updatedData := loadedChar.ToData()
		_, updateErr := o.charRepo.Update(ctx, characterrepo.UpdateInput{
			Character: &entities.Character{
				Data:       updatedData,
				Appearance: charOutput.Character.Appearance,
			},
		})
		if updateErr != nil {
			_ = loadedChar.Cleanup(ctx)
			return nil, fmt.Errorf("failed to save rested character %s: %w", characterID, updateErr)
		}
		_ = loadedChar.Cleanup(ctx)

		dexModifier := updatedData.AbilityScores.Modifier(abilities.DEX)

		// Track character's current HP for TPK detection (now at full after rest)
		characterHP[characterID] = updatedData.HitPoints

		combatant := initiative.NewParticipant(characterID, entityTypeCharacter)
		combatants[combatant] = dexModifier
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
		RoomOrigin:   getCurrentRoomOrigin(dungeonEntity),
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
	// Convert rooms to map and extract room origins
	rooms := make(map[string]*dungeon.Room)
	roomOrigins := make(map[string]dungeon.Position)
	for _, room := range genDungeon.Rooms {
		rooms[room.ID] = room
		roomOrigins[room.ID] = room.Origin
	}

	// Convert connections to toolkit format
	// Type field encodes "direction|physical_hint" for later parsing
	connections := make([]*environments.ConnectionEdge, len(genDungeon.Connections))
	for i, conn := range genDungeon.Connections {
		// Encode direction and physical hint as "direction|hint"
		// Direction is used for positioning, hint is for player display
		connType := string(conn.Direction) + "|" + conn.PhysicalHint
		connections[i] = &environments.ConnectionEdge{
			ID:            o.connectionIDGen.Generate(),
			FromRoomID:    conn.FromRoom,
			ToRoomID:      conn.ToRoom,
			Bidirectional: true,
			Required:      conn.IsMainPath, // Main path connections are required
			Type:          connType,
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
		RoomOrigins:   roomOrigins,
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
// Uses CubeEntities for hex grids (cube coordinates are the native format)
func (o *Orchestrator) convertToRoomData(encounterID string, room *dungeon.Room) *spatial.RoomData {
	roomData := &spatial.RoomData{
		ID:           encounterID + "-" + room.ID,
		Type:         "dungeon",
		Width:        room.Shape.Width,
		Height:       room.Shape.Height,
		GridType:     spatial.GridTypeHex,
		CubeEntities: make(map[string]spatial.EntityCubePlacement),
	}

	// Add obstacles from features using cube coordinates
	if room.Features != nil {
		for _, obstacle := range room.Features.Obstacles {
			roomData.CubeEntities[obstacle.ID] = spatial.EntityCubePlacement{
				EntityID:       obstacle.ID,
				EntityType:     "obstacle",
				CubePosition:   spatial.CubeCoordinate{X: obstacle.Position.X, Y: obstacle.Position.Y, Z: obstacle.Position.Z},
				Size:           1,
				BlocksMovement: obstacle.BlocksMovement,
			}
		}
	}

	return roomData
}

// convertToWallInfo converts dungeon walls to WallInfo for the API response
func (o *Orchestrator) convertToWallInfo(room *dungeon.Room) []WallInfo {
	if room == nil || len(room.Walls) == 0 {
		return nil
	}

	walls := make([]WallInfo, len(room.Walls))
	for i, wall := range room.Walls {
		// Determine material based on wall type
		material := "stone"
		if wall.Type == dungeon.WallTypeDestructible {
			material = "wood"
		}

		walls[i] = WallInfo{
			ID: wall.ID,
			Start: &Position{
				X: float64(wall.Start.X),
				Y: float64(wall.Start.Y),
				Z: float64(wall.Start.Z),
			},
			End: &Position{
				X: float64(wall.End.X),
				Y: float64(wall.End.Y),
				Z: float64(wall.End.Z),
			},
			Material:          material,
			BlocksMovement:    wall.BlocksMovement,
			BlocksLineOfSight: wall.BlocksLineOfSight,
		}
	}

	return walls
}

// getPlayerSpawnPositions extracts player spawn positions from a room as cube coordinates
func (o *Orchestrator) getPlayerSpawnPositions(room *dungeon.Room) []spatial.CubeCoordinate {
	var positions []spatial.CubeCoordinate

	for _, zone := range room.SpawnZones {
		if zone.Type == dungeon.ZoneTypePlayerSpawn || zone.Type == dungeon.ZoneTypeEntrance {
			for _, pos := range zone.Bounds {
				positions = append(positions, spatial.CubeCoordinate{
					X: pos.X,
					Y: pos.Y,
					Z: pos.Z,
				})
				if len(positions) >= 4 {
					return positions
				}
			}
		}
	}

	// Fallback: if no spawn zones, use default cube coordinates
	// These are valid cube coords (x + y + z = 0) near the room entrance
	if len(positions) == 0 {
		positions = []spatial.CubeCoordinate{
			{X: 2, Y: -4, Z: 2},
			{X: 3, Y: -5, Z: 2},
			{X: 2, Y: -5, Z: 3},
			{X: 3, Y: -6, Z: 3},
		}
	}

	return positions
}

// placeMonsters places monsters from the room's encounter into the room data
// Uses the spawner component for spatial-aware placement that avoids walls and other entities
func (o *Orchestrator) placeMonsters(roomData *spatial.RoomData, room *dungeon.Room) []*monster.Data {
	if room.Encounter == nil {
		return nil
	}

	// Build list of monster IDs to spawn
	monsterIDs := make([]string, 0, len(room.Encounter.Monsters))
	for _, placement := range room.Encounter.Monsters {
		monsterIDs = append(monsterIDs, fmt.Sprintf("monster-%s-%s", room.ID, placement.ID))
	}

	// Get currently occupied positions (characters already in room)
	occupiedPositions := make([]dungeon.Position, 0)
	for _, entity := range roomData.CubeEntities {
		occupiedPositions = append(occupiedPositions, dungeon.Position{
			X: entity.CubePosition.X,
			Y: entity.CubePosition.Y,
			Z: entity.CubePosition.Z,
		})
	}

	// Use spawner to place monsters in valid positions
	spawnOutput, err := spawner.SpawnInRoom(o.spawner, &spawner.DungeonSpawnInput{
		Room:              room,
		OccupiedPositions: occupiedPositions,
		EntitiesToSpawn:   spawner.CreateMonsterSpawnEntities(monsterIDs),
	})

	// Build a map of entity ID to placement position
	placementMap := make(map[string]spawner.CubePosition)
	if err == nil && spawnOutput != nil {
		for _, placement := range spawnOutput.Placements {
			placementMap[placement.EntityID] = placement.Position
		}
	}

	factory := dungeon.NewMonsterFactory()
	monsters := make([]*monster.Data, 0, len(room.Encounter.Monsters))
	for _, placement := range room.Encounter.Monsters {
		monsterID := fmt.Sprintf("monster-%s-%s", room.ID, placement.ID)

		// Get spawn position from spawner results
		var spawnPos spatial.CubeCoordinate
		if pos, ok := placementMap[monsterID]; ok {
			spawnPos = spatial.CubeCoordinate{X: pos.X, Y: pos.Y, Z: pos.Z}
		} else {
			// Fallback: find a safe position that's not on a wall
			spawnPos = findSafeFallbackPosition(room, roomData)
		}

		// Add to room cube entities
		roomData.CubeEntities[monsterID] = spatial.EntityCubePlacement{
			EntityID:       monsterID,
			EntityType:     entityTypeMonster,
			CubePosition:   spawnPos,
			Size:           1,
			BlocksMovement: true,
		}

		// Create monster using factory based on MonsterID from theme
		m := factory.CreateMonster(monsterID, placement.MonsterID)
		monsters = append(monsters, m.ToData())
	}

	return monsters
}

// findSafeFallbackPosition finds a position that is not on a wall or occupied
// Searches outward from center until a valid position is found
func findSafeFallbackPosition(room *dungeon.Room, roomData *spatial.RoomData) spatial.CubeCoordinate {
	if room.Shape == nil {
		return spatial.CubeCoordinate{X: 5, Y: -10, Z: 5}
	}

	// Build set of blocked positions (walls)
	blocked := make(map[string]bool)
	for _, wall := range room.Walls {
		if !wall.BlocksMovement {
			continue
		}
		// Get all positions along the wall
		positions := getWallPositionsForFallback(wall)
		for _, pos := range positions {
			key := fmt.Sprintf("%d,%d,%d", pos.X, pos.Y, pos.Z)
			blocked[key] = true
		}
	}

	// Also mark occupied positions
	for _, entity := range roomData.CubeEntities {
		key := fmt.Sprintf("%d,%d,%d", entity.CubePosition.X, entity.CubePosition.Y, entity.CubePosition.Z)
		blocked[key] = true
	}

	// Search from center outward for a valid position
	centerCol := room.Shape.Width / 2
	centerRow := room.Shape.Height / 2

	// Spiral search: check increasingly distant positions from center
	for radius := 0; radius < room.Shape.Width; radius++ {
		for dCol := -radius; dCol <= radius; dCol++ {
			for dRow := -radius; dRow <= radius; dRow++ {
				// Only check positions at this radius
				if abs(dCol) != radius && abs(dRow) != radius {
					continue
				}

				col := centerCol + dCol
				row := centerRow + dRow

				// Skip positions outside playable area (perimeter is walls)
				if col < 1 || col >= room.Shape.Width-1 || row < 1 || row >= room.Shape.Height-1 {
					continue
				}

				// Convert to cube coordinates
				x := col
				z := row
				y := -x - z

				key := fmt.Sprintf("%d,%d,%d", x, y, z)
				if !blocked[key] {
					// Found a valid position
					return spatial.CubeCoordinate{X: x, Y: y, Z: z}
				}
			}
		}
	}

	// Ultimate fallback: just use center (shouldn't reach here in practice)
	x := centerCol
	z := centerRow
	y := -x - z
	return spatial.CubeCoordinate{X: x, Y: y, Z: z}
}

// getWallPositionsForFallback returns all positions along a wall segment
func getWallPositionsForFallback(wall dungeon.WallSegment) []spatial.CubeCoordinate {
	var positions []spatial.CubeCoordinate

	dx := wall.End.X - wall.Start.X
	dz := wall.End.Z - wall.Start.Z

	steps := abs(dx)
	if abs(dz) > steps {
		steps = abs(dz)
	}
	if steps == 0 {
		steps = 1
	}

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := wall.Start.X + int(float64(dx)*t)
		z := wall.Start.Z + int(float64(dz)*t)
		y := -x - z
		positions = append(positions, spatial.CubeCoordinate{X: x, Y: y, Z: z})
	}

	return positions
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// parseConnectionType extracts direction and physical hint from encoded Type field
// Format is "direction|physical_hint" (e.g., "south|heavy stone door")
func parseConnectionType(connType string) (direction, physicalHint string) {
	parts := strings.SplitN(connType, "|", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	// Fallback for old format without direction
	return "", connType
}

// getDoorInfoForRoom extracts door information for a room using stored entity connections
func (o *Orchestrator) getDoorInfoForRoom(dungeonEntity *entities.Dungeon, roomID string) []DoorInfo {
	// Get room for position lookup
	room := dungeonEntity.GetRoom(roomID)

	// Pre-allocate doors slice based on connection count
	doors := make([]DoorInfo, 0, len(dungeonEntity.Connections))

	for _, conn := range dungeonEntity.Connections {
		if conn.FromRoomID != roomID && conn.ToRoomID != roomID {
			continue
		}

		targetRoomID := conn.ToRoomID
		if conn.ToRoomID == roomID {
			targetRoomID = conn.FromRoomID
		}

		// Parse direction and physical hint from Type field
		direction, physicalHint := parseConnectionType(conn.Type)

		// Try to get door position from shape's ConnectionPoints first
		position := getDoorPositionFromShape(room, direction)

		// Fall back to calculated position if no ConnectionPoint found
		// Use conn.Type for the hint since it contains direction info like "north door"
		if position == nil && room != nil && room.Shape != nil {
			position = calculateDoorPosition(conn.Type, room.Shape.Width, room.Shape.Height)
		}

		doors = append(doors, DoorInfo{
			ConnectionID: conn.ID,
			TargetRoomID: targetRoomID,
			Direction:    physicalHint, // Show physical hint to players
			Position:     position,
			IsOpen:       false,
		})
	}

	return doors
}

// getDoorPositionFromShape finds the door position from the shape's ConnectionPoints
func getDoorPositionFromShape(room *dungeon.Room, direction string) *Position {
	if room == nil || room.Shape == nil {
		return nil
	}

	// Normalize direction for matching
	dirLower := strings.ToLower(direction)

	for _, cp := range room.Shape.ConnectionPoints {
		if strings.EqualFold(cp.Direction, direction) || strings.Contains(dirLower, strings.ToLower(cp.Direction)) {
			return &Position{
				X: float64(cp.Position.X),
				Y: float64(cp.Position.Y),
				Z: float64(cp.Position.Z),
			}
		}
	}

	return nil
}

// calculateDoorPosition determines where to place a door based on the physical hint
// and room dimensions. Uses cube coordinates for hex grid compatibility.
func calculateDoorPosition(physicalHint string, width, height int) *Position {
	if width == 0 || height == 0 {
		return nil
	}

	// Parse direction from physical hint (e.g., "north door", "eastern passage", "stairs down")
	hint := strings.ToLower(physicalHint)

	// Calculate center positions for each edge
	// In cube coordinates: x increases right, z increases down, y = -x-z
	centerX := width / 2
	centerZ := height / 2

	var x, z int

	switch {
	case strings.Contains(hint, "north"):
		// Top edge: z = 0, x = center
		x, z = centerX, 0
	case strings.Contains(hint, "south"):
		// Bottom edge: z = height-1, x = center
		x, z = centerX, height-1
	case strings.Contains(hint, "east"):
		// Right edge: x = width-1, z = center
		x, z = width-1, centerZ
	case strings.Contains(hint, "west"):
		// Left edge: x = 0, z = center
		x, z = 0, centerZ
	case strings.Contains(hint, "down"):
		// Stairs down - place in opposite corner (bottom-right)
		x, z = width-2, height-2
	case strings.Contains(hint, "up"), strings.Contains(hint, "stairs"):
		// Stairs up - place in corner (top-left)
		x, z = 1, 1
	default:
		// Unknown direction - place at center of room
		x, z = centerX, centerZ
	}

	// Ensure position is within bounds
	if x < 0 {
		x = 0
	}
	if x >= width {
		x = width - 1
	}
	if z < 0 {
		z = 0
	}
	if z >= height {
		z = height - 1
	}

	// Calculate y for cube coordinates (x + y + z = 0)
	y := -x - z

	return &Position{
		X: float64(x),
		Y: float64(y),
		Z: float64(z),
	}
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

	// Load dungeon to get walls for the current room
	var walls []WallInfo
	var dungeonWalls []dungeon.WallSegment // Raw walls for event
	var moveRoomOrigin *dungeon.Position
	if o.dungeonRepo != nil {
		dungeonOutput, dungeonErr := o.dungeonRepo.GetByEncounterID(ctx, &dungeonrepo.GetByEncounterIDInput{
			EncounterID: input.EncounterID,
		})
		if dungeonErr == nil && dungeonOutput != nil && dungeonOutput.Dungeon != nil {
			dungeonEntity := dungeonOutput.Dungeon
			moveRoomOrigin = getCurrentRoomOrigin(dungeonEntity)
			currentRoomID := dungeonEntity.CurrentRoomID
			if currentRoomID == "" {
				currentRoomID = dungeonEntity.StartRoomID
			}
			if currentRoom := dungeonEntity.GetRoom(currentRoomID); currentRoom != nil {
				walls = o.convertToWallInfo(currentRoom)
				dungeonWalls = currentRoom.Walls
			}
		}
	}

	// Load movement remaining from encounter data
	movementRemaining := encOutput.Data.MovementRemaining
	if movementRemaining == 0 {
		movementRemaining = 30 // Default if not set
	}

	// 2. Check if room data exists (Phase 2: might be nil for early encounters)
	if encOutput.Data.RoomData == nil {
		// Create a basic room for testing - use hex grid with CubeEntities
		roomData := &spatial.RoomData{
			ID:           input.EncounterID + "-room",
			Type:         "dungeon",
			Width:        20,
			Height:       20,
			GridType:     spatial.GridTypeHex,
			CubeEntities: make(map[string]spatial.EntityCubePlacement),
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

	// 4. Target position from input (cube coordinates for hex grids)
	targetCube := spatial.CubeCoordinate{
		X: int(input.TargetPosition.X),
		Y: int(input.TargetPosition.Y),
		Z: int(input.TargetPosition.Z),
	}

	// Validate cube coordinates sum to zero for hex grids
	if roomData.GridType == spatial.GridTypeHex {
		if targetCube.X+targetCube.Y+targetCube.Z != 0 {
			return &MoveCharacterOutput{
				Success:           false,
				FinalPosition:     input.TargetPosition,
				MovementRemaining: 0,
				StopReason:        "invalid_coordinates",
				UpdatedRoom:       roomData,
				Walls:             walls,
			}, nil
		}
	}

	// 5. Check if target position is occupied (using CubeEntities for hex grids)
	for id, entity := range roomData.CubeEntities {
		if id != input.EntityID && entity.BlocksMovement {
			if entity.CubePosition.X == targetCube.X &&
				entity.CubePosition.Y == targetCube.Y &&
				entity.CubePosition.Z == targetCube.Z {
				// Position is blocked by another entity
				var currentPos *Position
				stopReason := "position_occupied"
				if currentEntity, exists := roomData.CubeEntities[input.EntityID]; exists {
					currentPos = &Position{
						X: float64(currentEntity.CubePosition.X),
						Y: float64(currentEntity.CubePosition.Y),
						Z: float64(currentEntity.CubePosition.Z),
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
					Walls:             walls,
				}, nil
			}
		}
	}

	// 5b. Get current position for wall collision and distance calculation
	var oldCube spatial.CubeCoordinate
	cubePlacement, exists := roomData.CubeEntities[input.EntityID]
	if exists {
		oldCube = cubePlacement.CubePosition
	} else {
		// Entity doesn't exist yet - treat as starting from target (0 distance)
		oldCube = targetCube
	}

	// 5c. Check if path crosses any walls
	if len(dungeonWalls) > 0 && exists {
		wallValidator := dungeontoolkit.NewWallValidator()
		startPos := dungeon.Position{X: oldCube.X, Y: oldCube.Y, Z: oldCube.Z}
		endPos := dungeon.Position{X: targetCube.X, Y: targetCube.Y, Z: targetCube.Z}

		if wallValidator.PathCrossesWall(startPos, endPos, dungeonWalls) {
			return &MoveCharacterOutput{
				Success: false,
				FinalPosition: &Position{
					X: float64(oldCube.X),
					Y: float64(oldCube.Y),
					Z: float64(oldCube.Z),
				},
				MovementRemaining: movementRemaining,
				StopReason:        "blocked_by_wall",
				UpdatedRoom:       roomData,
				Walls:             walls,
			}, nil
		}
	}

	// 6. Calculate distance and update movement

	// Calculate cube distance for hex grids: (|dx| + |dy| + |dz|) / 2
	dx := targetCube.X - oldCube.X
	dy := targetCube.Y - oldCube.Y
	dz := targetCube.Z - oldCube.Z
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dz < 0 {
		dz = -dz
	}
	distance := (dx + dy + dz) / 2
	//nolint:gosec // G115: Game values are bounded by room size limits, no overflow risk
	movementCost := int32(distance * 5) // Each hex is 5 feet

	// Check if enough movement remaining
	if movementCost > movementRemaining {
		return &MoveCharacterOutput{
			Success: false,
			FinalPosition: &Position{
				X: float64(oldCube.X),
				Y: float64(oldCube.Y),
				Z: float64(oldCube.Z),
			},
			MovementRemaining: movementRemaining,
			StopReason:        "insufficient_movement",
			UpdatedRoom:       roomData,
			Walls:             walls,
		}, nil
	}

	// Decrement movement
	movementRemaining -= movementCost

	// 7. Update entity position in CubeEntities
	entityType := "character"
	if exists {
		entityType = cubePlacement.EntityType
	}
	roomData.CubeEntities[input.EntityID] = spatial.EntityCubePlacement{
		EntityID:       input.EntityID,
		EntityType:     entityType,
		CubePosition:   targetCube,
		Size:           1,
		BlocksMovement: true,
	}

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
		EntityID:   input.EntityID,
		EntityType: entityType,
		FinalPosition: &Position{
			X: float64(targetCube.X),
			Y: float64(targetCube.Y),
			Z: float64(targetCube.Z),
		},
		MovementRemaining: movementRemaining,
		StopReason:        "completed",
		UpdatedRoom:       roomData,
		RoomOrigin:        moveRoomOrigin,
		Walls:             dungeonWalls,
	})

	// 10. Return success with updated position
	return &MoveCharacterOutput{
		Success: true,
		FinalPosition: &Position{
			X: float64(targetCube.X),
			Y: float64(targetCube.Y),
			Z: float64(targetCube.Z),
		},
		MovementRemaining: movementRemaining,
		StopReason:        "completed",
		UpdatedRoom:       roomData,
		RoomOrigin:        moveRoomOrigin,
		Walls:             walls,
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
	previousEntityType := activeEntity.EntityType
	previousRound := initiativeData.Round

	// 6. Advance to next turn
	newRound := false
	currentIndex++
	if currentIndex >= len(turnOrder) {
		// Wrap around to start of order and increment round
		currentIndex = 0
		initiativeData.Round++
		newRound = true

		// Re-sort initiative order at the start of each new round
		// This ensures proper initiative order after new combatants were added mid-round
		// (they were appended at the end for their first partial round)
		if len(initiativeRolls) > 0 {
			sort.Slice(initiativeRolls, func(i, j int) bool {
				if initiativeRolls[i].Total != initiativeRolls[j].Total {
					return initiativeRolls[i].Total > initiativeRolls[j].Total
				}
				return initiativeRolls[i].Modifier > initiativeRolls[j].Modifier
			})

			// Rebuild order from sorted rolls
			newOrder := make([]initiative.EntityData, len(initiativeRolls))
			for i, roll := range initiativeRolls {
				newOrder[i] = initiative.EntityData{
					ID:   roll.Entity.GetID(),
					Type: string(roll.Entity.GetType()),
				}
			}
			initiativeData.Order = newOrder

			// Update turnOrder for monster execution below
			turnOrder = buildTurnOrderFromData(initiativeData, initiativeRolls, encOutput.Data.RoomData)
		}
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

	// 9b. Publish TurnEndEvent to character's event bus AFTER monster turns
	// This allows conditions like Rage to check for combat activity (attacks/damage taken)
	// that occurred during the monsters' turns before deciding if they should end.
	if previousEntityType == entityTypeCharacter {
		if publishErr := o.publishCharacterTurnEnd(ctx, previousEntityID, previousRound); publishErr != nil {
			// Log but don't fail - turn end events are important but not critical
			fmt.Printf("warning: failed to publish turn end event for character %s: %v\n", previousEntityID, publishErr)
		}
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

	// 11. Get room data for the event (if available)
	var turnEndedRoomData *spatial.RoomData
	if encOutput.Data.RoomData != nil {
		if rd, ok := encOutput.Data.RoomData.(*spatial.RoomData); ok {
			turnEndedRoomData = rd
		}
	}

	// 11b. Load dungeon to get walls and room origin for the current room
	var dungeonWalls []dungeon.WallSegment
	var turnRoomOrigin *dungeon.Position
	if o.dungeonRepo != nil {
		dungeonOutput, dungeonErr := o.dungeonRepo.GetByEncounterID(ctx, &dungeonrepo.GetByEncounterIDInput{
			EncounterID: input.EncounterID,
		})
		if dungeonErr == nil && dungeonOutput.Dungeon != nil {
			dungeonEntity := dungeonOutput.Dungeon
			turnRoomOrigin = getCurrentRoomOrigin(dungeonEntity)
			currentRoomID := dungeonEntity.CurrentRoomID
			if currentRoomID == "" {
				currentRoomID = dungeonEntity.StartRoomID
			}
			if currentRoom := dungeonEntity.GetRoom(currentRoomID); currentRoom != nil {
				dungeonWalls = currentRoom.Walls
			}
		}
	}

	// 12. Publish events
	// Publish TurnEnded event
	o.publishEvent(ctx, input.EncounterID, entities.EventTypeTurnEnded, &entities.TurnEndedEvent{
		PreviousEntityID: previousEntityID,
		NextEntityID:     nextEntityID,
		Round:            initiativeData.Round,
		NewRound:         newRound,
		CombatState:      combatState,
		Room:             turnEndedRoomData,
		RoomOrigin:       turnRoomOrigin,
		Walls:            dungeonWalls,
	})

	// Publish MonsterTurnCompleted events for each monster turn
	for _, mt := range monsterTurns {
		o.publishEvent(ctx, input.EncounterID, entities.EventTypeMonsterTurnCompleted, &entities.MonsterTurnCompletedEvent{
			MonsterID:         mt.MonsterID,
			MonsterName:       mt.MonsterName,
			Actions:           mt.Actions,
			Movement:          mt.Movement,
			Room:              turnEndedRoomData,
			RoomOrigin:        turnRoomOrigin,
			Walls:             dungeonWalls,
			UpdatedCharacters: mt.UpdatedCharacters,
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

// Stop reasons for movement
const (
	stopReasonCompleted            = "completed"
	stopReasonPositionOccupied     = "position_occupied"
	stopReasonInsufficientMovement = "insufficient_movement"
	stopReasonBlockedByWall        = "blocked_by_wall"
	stopReasonInvalidCoordinates   = "invalid_coordinates"
)

// Default movement speed in feet (30 feet = 6 hexes at 5ft/hex)
const defaultMovementSpeed = 30

// unarmedStrikeWeapon returns the default unarmed strike weapon definition.
// Base damage is 1d1 (1 + STR mod); Martial Arts conditions upgrade the die
// automatically through the damage chain.
func unarmedStrikeWeapon() weapons.Weapon {
	return weapons.Weapon{
		ID:         "unarmed-strike",
		Name:       "Unarmed Strike",
		Category:   weapons.CategorySimpleMelee,
		Damage:     "1d1",
		DamageType: "bludgeoning",
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
		ActionEconomy:     enc.ActionEconomy,
		CombatStarted:     enc.State == encounterrepo.StateActive || enc.State == encounterrepo.StatePaused,
		CombatEnded:       enc.State == encounterrepo.StateCompleted,
	}
}

// convertCharAbilitiesToEntities converts toolkit character AvailableAbility slice to entity types.
// Used by the orchestrator to return entity types that the handler layer converts to proto.
func convertCharAbilitiesToEntities(abilities []character.AvailableAbility) []*entities.AvailableAbility {
	result := make([]*entities.AvailableAbility, len(abilities))
	for i, a := range abilities {
		abilityID := ""
		if a.Ref != nil {
			abilityID = string(a.Ref.ID)
		}
		result[i] = &entities.AvailableAbility{
			AbilityID:       abilityID,
			Name:            a.Name,
			CanUse:          a.CanUse,
			Reason:          a.Reason,
			ResourceCurrent: a.ResourceCurrent,
			ResourceMax:     a.ResourceMax,
		}
	}
	return result
}

// convertCharActionsToEntities converts toolkit character AvailableAction slice to entity types.
// Used by the orchestrator to return entity types that the handler layer converts to proto.
func convertCharActionsToEntities(actions []character.AvailableAction) []*entities.AvailableAction {
	result := make([]*entities.AvailableAction, len(actions))
	for i, a := range actions {
		actionID := ""
		if a.Ref != nil {
			actionID = string(a.Ref.ID)
		}
		result[i] = &entities.AvailableAction{
			ActionID: actionID,
			Name:     a.Name,
			CanUse:   a.CanUse,
			Reason:   a.Reason,
		}
	}
	return result
}

// protoAbilityIDToRef maps a proto CombatAbilityId to a toolkit ref.
// Used by the orchestrator to convert the proto enum from the handler into a toolkit ref
// that can be passed to character.ActivateAbility.
func protoAbilityIDToRef(abilityID pb.CombatAbilityId) *core.Ref {
	switch abilityID {
	case pb.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK:
		return refs.CombatAbilities.Attack()
	case pb.CombatAbilityId_COMBAT_ABILITY_ID_DASH:
		return refs.CombatAbilities.Dash()
	case pb.CombatAbilityId_COMBAT_ABILITY_ID_DODGE:
		return refs.CombatAbilities.Dodge()
	case pb.CombatAbilityId_COMBAT_ABILITY_ID_DISENGAGE:
		return refs.CombatAbilities.Disengage()
	case pb.CombatAbilityId_COMBAT_ABILITY_ID_OFFHAND_ATTACK:
		return refs.CombatAbilities.OffHandAttack()
	case pb.CombatAbilityId_COMBAT_ABILITY_ID_RAGE:
		return refs.Features.Rage()
	case pb.CombatAbilityId_COMBAT_ABILITY_ID_SECOND_WIND:
		return refs.Features.SecondWind()
	case pb.CombatAbilityId_COMBAT_ABILITY_ID_FLURRY_OF_BLOWS:
		return refs.Features.FlurryOfBlows()
	case pb.CombatAbilityId_COMBAT_ABILITY_ID_MARTIAL_ARTS_BONUS:
		return refs.Actions.UnarmedStrike()
	default:
		return nil
	}
}

// featureIDToRef maps a feature ID string to a toolkit ref.
// Used by ActivateFeature to validate the feature ID before delegating.
func featureIDToRef(featureID string) *core.Ref {
	switch featureID {
	case refs.Features.Rage().ID:
		return refs.Features.Rage()
	case refs.Features.SecondWind().ID:
		return refs.Features.SecondWind()
	case refs.Features.FlurryOfBlows().ID:
		return refs.Features.FlurryOfBlows()
	case refs.Features.PatientDefense().ID:
		return refs.Features.PatientDefense()
	case refs.Features.StepOfTheWind().ID:
		return refs.Features.StepOfTheWind()
	case refs.Features.ActionSurge().ID:
		return refs.Features.ActionSurge()
	default:
		return nil
	}
}

// protoActionIDToRef maps a proto ActionId to a toolkit action ref.
// Used by the orchestrator to convert the proto enum from the handler into a toolkit ref
// that can be passed to character.ExecuteAction.
func protoActionIDToRef(actionID pb.ActionId) *core.Ref {
	switch actionID {
	case pb.ActionId_ACTION_ID_MOVE:
		return refs.Actions.Move()
	case pb.ActionId_ACTION_ID_STRIKE:
		return refs.Actions.Strike()
	case pb.ActionId_ACTION_ID_OFF_HAND_STRIKE:
		return refs.Actions.OffHandStrike()
	case pb.ActionId_ACTION_ID_FLURRY_STRIKE:
		return refs.Actions.FlurryStrike()
	case pb.ActionId_ACTION_ID_UNARMED_STRIKE:
		return refs.Actions.UnarmedStrike()
	default:
		return nil
	}
}

// computeTurnNumber creates a unique turn number from encounter initiative data.
// Each character gets a unique turn number per round.
func computeTurnNumber(data *encounterrepo.EncounterData) int {
	if data == nil || data.InitiativeData == nil {
		return 1
	}
	return data.InitiativeData.Round*100 + data.InitiativeData.Current
}

// loadCharacterForCombat loads a character from the repository and initializes it for combat.
// Uses the current turn number to detect stale action economy from a previous turn.
// Returns the loaded character, the repository output for later persistence, and any error.
func (o *Orchestrator) loadCharacterForCombat(
	ctx context.Context,
	characterID string,
	currentTurnNumber int,
) (*character.Character, *characterrepo.GetOutput, events.EventBus, error) {
	bus := events.NewEventBus()

	charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load character: %w", err)
	}

	char, err := character.LoadFromData(ctx, charOutput.Character.Data, bus)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load character from data: %w", err)
	}

	// Call StartTurn if:
	// 1. Character is not in combat yet (first time this encounter)
	// 2. Character's action economy is from a different turn (stale from previous turn)
	needsStartTurn := !char.InCombat()
	if !needsStartTurn {
		ae := char.GetActionEconomy()
		if ae != nil && ae.TurnNumber != currentTurnNumber {
			needsStartTurn = true
		}
	}

	if needsStartTurn {
		speed := char.GetSpeed()
		if _, startErr := char.StartTurn(ctx, &character.StartTurnInput{
			Speed:      speed,
			TurnNumber: currentTurnNumber,
		}); startErr != nil {
			return nil, nil, nil, fmt.Errorf("failed to start turn: %w", startErr)
		}
	}

	return char, charOutput, bus, nil
}

// persistCharacterData saves the character's current state back to the repository.
func (o *Orchestrator) persistCharacterData(
	ctx context.Context,
	char *character.Character,
	charOutput *characterrepo.GetOutput,
) error {
	charData := char.ToData()
	charOutput.Character.Data = charData
	_, err := o.charRepo.Update(ctx, characterrepo.UpdateInput{Character: charOutput.Character})
	if err != nil {
		return fmt.Errorf("failed to persist character: %w", err)
	}
	return nil
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
		if charOutput.Character.Data.PlayerID != playerID {
			return fmt.Errorf(
				"cannot end turn: you do not control character %s (owned by player %s)",
				activeEntity.EntityID,
				charOutput.Character.Data.PlayerID,
			)
		}
	}

	return nil
}

// ActivateFeature activates a combat feature (e.g., Rage, PatientDefense) for a character.
// Calls char.ActivateAbility directly with the feature ref to preserve feature identity
// and correct action costs (e.g., PatientDefense costs bonus action + ki, not a standard action).
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

	// 2. Map feature ID to toolkit ref
	abilityRef := featureIDToRef(input.FeatureID)
	if abilityRef == nil {
		return &ActivateFeatureOutput{
			Success: false,
			Message: fmt.Sprintf("unknown feature: %s", input.FeatureID),
		}, nil
	}

	// 3. Load encounter to get turn number
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load encounter: %w", err)
	}
	if encOutput.Data == nil {
		return nil, fmt.Errorf("encounter not found: %s", input.EncounterID)
	}

	// 4. Load character for combat (handles StartTurn if stale)
	turnNum := computeTurnNumber(encOutput.Data)
	char, charOutput, _, err := o.loadCharacterForCombat(ctx, input.CharacterID, turnNum)
	if err != nil {
		return nil, err
	}

	// 5. Call ActivateAbility directly with the feature ref — no proto conversion
	abilityOutput, err := char.ActivateAbility(ctx, &character.ActivateAbilityInput{
		AbilityRef: abilityRef,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to activate ability: %w", err)
	}

	// 6. Persist character data
	if persistErr := o.persistCharacterData(ctx, char, charOutput); persistErr != nil {
		return nil, persistErr
	}

	// 7. Publish FeatureActivatedEvent for multiplayer broadcast
	updatedData := char.ToData()
	if abilityOutput.Success {
		o.publishEvent(ctx, input.EncounterID, entities.EventTypeFeatureActivated, &entities.FeatureActivatedEvent{
			CharacterID:   input.CharacterID,
			FeatureID:     input.FeatureID,
			Success:       true,
			Message:       fmt.Sprintf("%s activated successfully", input.FeatureID),
			CharacterData: updatedData,
		})
	}

	// 8. Return response
	message := fmt.Sprintf("%s activated successfully", input.FeatureID)
	if !abilityOutput.Success {
		message = abilityOutput.Error
	}

	return &ActivateFeatureOutput{
		Success:       abilityOutput.Success,
		Message:       message,
		CharacterData: updatedData,
	}, nil
}

// getEquippedWeaponAndSlots retrieves the weapon equipped in the character's mainhand slot
// along with the full equipment slots data for GameContext building.
// If no weapon is equipped or the weapon cannot be found, it falls back to an unarmed strike.
// This ensures combat never fails due to missing equipment data.
func (o *Orchestrator) getEquippedWeaponAndSlots(
	ctx context.Context,
	characterID string,
) (weapons.Weapon, character.EquipmentSlots) {
	// Default fallback weapon
	fallbackWeapon := unarmedStrikeWeapon()

	// Try to load character data (equipment slots are part of character.Data)
	charResult, err := o.charRepo.Get(ctx, characterrepo.GetInput{
		ID: characterID,
	})
	if err != nil {
		return fallbackWeapon, nil
	}

	slots := charResult.Character.Data.EquipmentSlots

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
// NOTE: For dungeons with bosses, victory is handled by checkAndHandleVictory when boss is killed
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
	// BUT: If there are boss monsters defined, victory is only triggered by boss kill
	// (handled by checkAndHandleVictory), not by clearing current room's monsters
	if allMonstersDead && !allPlayersDead {
		// If bosses are defined, don't declare victory here - wait for boss kill
		if len(enc.BossMonsterIDs) > 0 {
			// Bosses exist, so victory only happens when boss is killed
			// Combat continues (player can open doors to find boss)
			return nil
		}
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

	// 7a. Validate proximity - a party member must be adjacent to the door
	// Get current room (the revealed one where players are)
	currentRoomID := dng.CurrentRoomID
	if currentRoomID == "" {
		currentRoomID = dng.StartRoomID
	}
	currentRoom := dng.GetRoom(currentRoomID)
	if currentRoom == nil {
		return nil, fmt.Errorf("current room not found: %s", currentRoomID)
	}

	// Calculate door position in the current room using same logic as getDoorInfoForRoom
	// to ensure consistency between what the client sees and what the server validates
	direction, _ := parseConnectionType(connection.Type)

	// Try to get door position from shape's ConnectionPoints first
	doorPos := getDoorPositionFromShape(currentRoom, direction)

	// Fall back to calculated position if no ConnectionPoint found
	// Use connection.Type for the hint since it contains direction info like "north door"
	if doorPos == nil && currentRoom.Shape != nil {
		doorPos = calculateDoorPosition(connection.Type, currentRoom.Shape.Width, currentRoom.Shape.Height)
	}
	if doorPos == nil {
		return nil, fmt.Errorf("cannot determine door position")
	}

	// Check if any character is adjacent to the door
	roomData, ok := encOutput.Data.RoomData.(*spatial.RoomData)
	if !ok || roomData == nil {
		return nil, fmt.Errorf("invalid room data")
	}
	isAdjacent := false
	for _, placement := range roomData.CubeEntities {
		// Only check character entities (not monsters)
		if placement.EntityType != entityTypeCharacter {
			continue
		}
		// Check adjacency using proper cube distance formula
		doorCube := spatial.CubeCoordinate{
			X: int(doorPos.X),
			Y: int(doorPos.Y),
			Z: int(doorPos.Z),
		}
		if cubeDistance(placement.CubePosition, doorCube) <= 1 {
			isAdjacent = true
			break
		}
	}
	if !isAdjacent {
		return nil, fmt.Errorf("no party member is adjacent to the door")
	}

	// 8. Create monsters and roll initiative
	var monsters []MonsterInfo
	var newInitiativeRolls []initiative.Roll
	var newBossMonsterIDs []string
	factory := dungeon.NewMonsterFactory()
	isBossRoom := dng.IsBossRoom(revealedRoomID)

	if revealedRoom.Encounter != nil {
		for _, placement := range revealedRoom.Encounter.Monsters {
			monsterID := fmt.Sprintf("monster-%s-%s", revealedRoomID, placement.ID)
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

	// 10. Merge initiative - preserve current round order, add new monsters at end
	// D&D rule: new combatants joining mid-round act after everyone who was already
	// in initiative finishes their turn this round. Next round they take their
	// proper sorted position.
	//
	// This prevents the confusing behavior of the initiative order visually changing
	// while maintaining correct turn sequence.
	allRolls := make([]initiative.Roll, 0, len(encOutput.Data.InitiativeRolls)+len(newInitiativeRolls))
	allRolls = append(allRolls, encOutput.Data.InitiativeRolls...)
	allRolls = append(allRolls, newInitiativeRolls...)

	// Build new initiative order: keep existing order, append new monsters sorted among themselves
	// Sort only the NEW monsters by initiative (they go after existing entities this round)
	sort.Slice(newInitiativeRolls, func(i, j int) bool {
		if newInitiativeRolls[i].Total != newInitiativeRolls[j].Total {
			return newInitiativeRolls[i].Total > newInitiativeRolls[j].Total
		}
		return newInitiativeRolls[i].Modifier > newInitiativeRolls[j].Modifier
	})

	// Preserve existing order, append new monsters at the end
	existingOrder := encOutput.Data.InitiativeData.Order
	newOrder := make([]initiative.EntityData, 0, len(existingOrder)+len(newInitiativeRolls))
	newOrder = append(newOrder, existingOrder...)
	for _, roll := range newInitiativeRolls {
		newOrder = append(newOrder, initiative.EntityData{
			ID:   roll.Entity.GetID(),
			Type: string(roll.Entity.GetType()),
		})
	}

	// Current index stays the same - we didn't reorder existing entities
	newCurrent := encOutput.Data.InitiativeData.Current

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
		EncounterID:     dng.EncounterID,
		InitiativeData:  newInitiativeData,
		InitiativeRolls: allRolls, // Persist merged rolls so EndTurn can re-sort at round start
		Monsters:        encOutput.Data.Monsters,
		BossMonsterIDs:  allBossIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update encounter: %w", err)
	}

	// 12. Build room data for response with entities and walls
	responseRoomData := &RoomData{
		ID:       revealedRoom.ID,
		Width:    revealedRoom.Shape.Width,
		Height:   revealedRoom.Shape.Height,
		Entities: make(map[string]*EntityPlacement),
		Walls:    o.convertToWallInfo(revealedRoom),
	}

	// Add monster placements to entities
	for _, m := range monsters {
		// Convert 2D position to cube coordinates for hex grid
		cubeX := int(m.Position.X)
		cubeZ := int(m.Position.Y) // Y in 2D maps to Z in cube coords
		cubeY := -cubeX - cubeZ    // y = -x - z for valid cube coordinate

		responseRoomData.Entities[m.ID] = &EntityPlacement{
			EntityID:          m.ID,
			EntityType:        "monster",
			Size:              1,
			BlocksMovement:    true,
			BlocksLineOfSight: false,
			Position: &Position{
				X: float64(cubeX),
				Y: float64(cubeY),
				Z: float64(cubeZ),
			},
		}
	}

	// 13. Get doors for the newly revealed room
	newDoors := o.getDoorInfoForRoom(dng, revealedRoomID)

	// 14. Build combat state for response
	combatState := &CombatState{
		EncounterID:   dng.EncounterID,
		TurnOrder:     make([]InitiativeEntry, len(newOrder)),
		ActiveIndex:   newCurrent,
		Round:         newInitiativeData.Round,
		CombatStarted: true, // Combat is active when opening doors
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

	// 15. Build monster states for event
	monsterStates := make([]*entities.MonsterState, 0, len(monsters))
	for _, m := range monsters {
		monsterStates = append(monsterStates, &entities.MonsterState{
			MonsterID:        m.ID,
			MonsterName:      m.Name,
			CurrentHitPoints: m.HP,
			MaxHitPoints:     m.MaxHP,
			MonsterType:      m.MonsterID, // Use monster type ID (e.g., "skeleton", "goblin")
		})
	}

	// 16. Build revealed room spatial data for event (including monsters)
	revealedSpatialRoom := o.convertToRoomData(dng.EncounterID, revealedRoom)

	// Add monster placements to the spatial room data for the event
	// This ensures the RoomRevealedEvent includes monsters, matching the OpenDoor response
	for _, m := range monsters {
		cubeX := int(m.Position.X)
		cubeZ := int(m.Position.Y) // Y in 2D maps to Z in cube coords
		cubeY := -cubeX - cubeZ    // y = -x - z for valid cube coordinate

		revealedSpatialRoom.CubeEntities[m.ID] = spatial.EntityCubePlacement{
			EntityID:          m.ID,
			EntityType:        "monster",
			CubePosition:      spatial.CubeCoordinate{X: cubeX, Y: cubeY, Z: cubeZ},
			Size:              1,
			BlocksMovement:    true,
			BlocksLineOfSight: false,
		}
	}

	// 17. Publish RoomRevealed event
	// Look up room origin from stored room origins for the event
	var revealedRoomOrigin *dungeon.Position
	if dng.RoomOrigins != nil {
		if origin, ok := dng.RoomOrigins[revealedRoomID]; ok {
			revealedRoomOrigin = &origin
		}
	}

	o.publishEvent(ctx, dng.EncounterID, entities.EventTypeRoomRevealed, &entities.RoomRevealedEvent{
		DungeonID:    dng.ID,
		ConnectionID: input.ConnectionID,
		RevealedRoom: revealedSpatialRoom,
		RoomOrigin:   revealedRoomOrigin,
		Walls:        revealedRoom.Walls,
		NewDoors:     convertDoorsToEntityDoors(newDoors),
		Monsters:     monsterStates,
		CombatState:  combatState,
		MonsterTurns: nil, // Monsters don't immediately act; they wait for their turn
	})

	// Calculate room offset from stored room origins
	var roomOffset *Position
	if dng.RoomOrigins != nil {
		if origin, ok := dng.RoomOrigins[revealedRoomID]; ok {
			roomOffset = &Position{
				X: float64(origin.X),
				Y: float64(origin.Y),
				Z: float64(origin.Z),
			}
		}
	}

	return &OpenDoorOutput{
		EncounterID:  dng.EncounterID,
		RevealedRoom: responseRoomData,
		RoomOffset:   roomOffset,
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
	// Using CubeEntities for hex grids (cube coordinates are the native format)
	roomData := &spatial.RoomData{
		ID:           encounterID + "-room",
		Type:         "dungeon",
		Width:        20,
		Height:       20,
		GridType:     spatial.GridTypeHex,
		CubeEntities: make(map[string]spatial.EntityCubePlacement),
	}

	// Add pillars for atmosphere using cube coordinates (x + y + z = 0)
	obstacles := []spatial.CubeCoordinate{
		{X: 5, Y: -10, Z: 5},   // Inner left pillar
		{X: 5, Y: -19, Z: 14},  // Inner left pillar (bottom)
		{X: 14, Y: -24, Z: 10}, // Inner right pillar
		{X: 10, Y: -25, Z: 15}, // Center-bottom pillar
	}
	for i, pos := range obstacles {
		obstacleID := fmt.Sprintf("pillar-%d", i)
		roomData.CubeEntities[obstacleID] = spatial.EntityCubePlacement{
			EntityID:       obstacleID,
			EntityType:     "obstacle",
			CubePosition:   pos,
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
		CharacterData: charOutput.Character.Data,
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
// Generates a dungeon using the dungeon generator and starts combat in the first room.
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

	// 6. Collect character IDs from players
	characterIDs := make([]string, 0, len(encOutput.Data.Players))
	for _, player := range encOutput.Data.Players {
		characterIDs = append(characterIDs, player.CharacterID)
	}

	// 7. Generate dungeon using the dungeon generator
	partyLevel := input.PartyLevel
	if partyLevel == 0 {
		partyLevel = 1 // Default to level 1
	}

	genInput := MapToGeneratorInput(&MapToGeneratorInputParams{
		PartySize:  len(characterIDs),
		PartyLevel: partyLevel,
		ThemeID:    input.ThemeID,    // Defaults to crypt if empty
		Difficulty: input.Difficulty, // Defaults to easy if empty
		Length:     input.Length,     // Defaults to short if empty
		Seed:       input.Seed,
	})

	genOutput, err := o.dungeonGen.Generate(ctx, genInput)
	if err != nil {
		return nil, fmt.Errorf("failed to generate dungeon: %w", err)
	}

	generatedDungeon := genOutput.Dungeon
	if generatedDungeon == nil {
		return nil, fmt.Errorf("dungeon generator returned nil dungeon")
	}
	if len(generatedDungeon.Rooms) == 0 {
		return nil, fmt.Errorf("dungeon generator returned empty dungeon with no rooms")
	}
	if generatedDungeon.StartRoom == "" {
		return nil, fmt.Errorf("dungeon generator did not specify a start room")
	}

	// 8. Create entities.Dungeon for storage and save it
	dungeonEntity := o.convertToDungeonEntity(input.EncounterID, generatedDungeon, genOutput.Seed)
	_, err = o.dungeonRepo.Save(ctx, &dungeonrepo.SaveInput{
		Dungeon: dungeonEntity,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save dungeon: %w", err)
	}

	// 9. Get the start room and convert to room data
	startRoom := o.findRoom(generatedDungeon, generatedDungeon.StartRoom)
	if startRoom == nil {
		return nil, fmt.Errorf("start room not found: %s", generatedDungeon.StartRoom)
	}

	roomData := o.convertToRoomData(input.EncounterID, startRoom)

	// 10. Place characters at spawn zones
	spawnPositions := o.getPlayerSpawnPositions(startRoom)
	for i, characterID := range characterIDs {
		if i >= len(spawnPositions) {
			break
		}
		roomData.CubeEntities[characterID] = spatial.EntityCubePlacement{
			EntityID:       characterID,
			EntityType:     entityTypeCharacter,
			CubePosition:   spawnPositions[i],
			Size:           1,
			BlocksMovement: true,
		}
	}

	// 11. Place monsters from the dungeon's encounter
	monsters := o.placeMonsters(roomData, startRoom)

	// 12. Perform long rest for all characters before dungeon starts
	// Restores HP, feature charges (Rage, Second Wind, etc.), and clears conditions
	restBus := events.NewEventBus()
	for _, characterID := range characterIDs {
		charOutput, charErr := o.charRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
		if charErr != nil {
			return nil, fmt.Errorf("failed to load character %s: %w", characterID, charErr)
		}

		loadedChar, charErr := character.LoadFromData(ctx, charOutput.Character.Data, restBus)
		if charErr != nil {
			return nil, fmt.Errorf("failed to load character %s for rest: %w", characterID, charErr)
		}

		if restErr := loadedChar.LongRest(ctx); restErr != nil {
			_ = loadedChar.Cleanup(ctx)
			return nil, fmt.Errorf("failed to perform long rest for character %s: %w", characterID, restErr)
		}

		updatedData := loadedChar.ToData()
		_, updateErr := o.charRepo.Update(ctx, characterrepo.UpdateInput{
			Character: &entities.Character{
				Data:       updatedData,
				Appearance: charOutput.Character.Appearance,
			},
		})
		if updateErr != nil {
			_ = loadedChar.Cleanup(ctx)
			return nil, fmt.Errorf("failed to save rested character %s: %w", characterID, updateErr)
		}
		_ = loadedChar.Cleanup(ctx)
	}

	// 13. Roll initiative
	combatants := make(map[core.Entity]int)
	characterHP := make(map[string]int)

	// Add characters with their DEX modifiers (re-read after rest to get updated data)
	for _, characterID := range characterIDs {
		charOutput, charErr := o.charRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
		if charErr != nil {
			return nil, fmt.Errorf("failed to load character %s: %w", characterID, charErr)
		}
		dexModifier := charOutput.Character.Data.AbilityScores.Modifier(abilities.DEX)
		characterHP[characterID] = charOutput.Character.Data.HitPoints
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

	// 13. Update encounter state to active with fresh action economy
	activeState := encounterrepo.StateActive
	initialActionEconomy := entities.NewActionEconomyState()
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID:       input.EncounterID,
		State:             &activeState,
		RoomData:          roomData,
		InitiativeData:    &trackerData,
		MovementRemaining: ptrInt32(defaultMovementSpeed),
		ActionEconomy:     initialActionEconomy,
		Monsters:          monsters,
		CharacterHP:       characterHP,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update encounter: %w", err)
	}

	// 14. Build combat state
	combatState := &CombatState{
		EncounterID:       input.EncounterID,
		Round:             1,
		TurnOrder:         turnOrder,
		ActiveIndex:       0,
		MovementRemaining: defaultMovementSpeed,
		ActionEconomy:     initialActionEconomy,
		CombatStarted:     true,
		CombatEnded:       false,
	}

	// 15. Execute monster turns if they go first
	var monsterTurns []*MonsterTurnResult
	if len(trackerData.Order) > 0 && trackerData.Order[0].Type == entityTypeMonster {
		encData := &encounterrepo.EncounterData{
			ID:                input.EncounterID,
			RoomData:          roomData,
			InitiativeData:    &trackerData,
			InitiativeRolls:   rolls,
			MovementRemaining: combatState.MovementRemaining,
			Monsters:          monsters,
			CharacterHP:       characterHP,
		}

		monsterTurns, err = o.executeMonsterTurns(ctx, encData, characterIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to execute monster turns: %w", err)
		}

		combatState.ActiveIndex = encData.InitiativeData.Current
		combatState.Round = encData.InitiativeData.Round

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

		o.checkAndHandleFailure(ctx, input.EncounterID, encData)
	}

	// 16. Build party list for event
	party := make([]*entities.Player, 0, len(encOutput.Data.Players))
	for _, player := range encOutput.Data.Players {
		charOutput, charErr := o.charRepo.Get(ctx, characterrepo.GetInput{ID: player.CharacterID})
		var charData *character.Data
		if charErr == nil && charOutput != nil {
			charData = charOutput.Character.Data
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

	// 17. Build monster state list for event
	monsterStates := make([]*entities.MonsterState, 0, len(monsters))
	for _, m := range monsters {
		monsterType := ""
		if m.Ref != nil {
			monsterType = m.Ref.ID // Use ref ID as monster type (e.g., "skeleton", "goblin")
		}
		monsterStates = append(monsterStates, &entities.MonsterState{
			MonsterID:        m.ID,
			MonsterName:      m.Name,
			CurrentHitPoints: m.HitPoints,
			MaxHitPoints:     m.MaxHitPoints,
			MonsterType:      monsterType,
		})
	}

	// 18. Extract doors for response and event
	doors := o.getDoorInfoForRoom(dungeonEntity, generatedDungeon.StartRoom)

	// 18b. Get room origin for the start room
	startRoomOrigin := getCurrentRoomOrigin(dungeonEntity)

	// 19. Publish CombatStarted event (includes monster turns if monsters won initiative)
	o.publishEvent(ctx, input.EncounterID, entities.EventTypeCombatStarted, &entities.CombatStartedEvent{
		CombatState:  combatState,
		Room:         roomData,
		RoomOrigin:   startRoomOrigin,
		Walls:        startRoom.Walls,
		Party:        party,
		Monsters:     monsterStates,
		Doors:        convertDoorsToEntityDoors(doors),
		DungeonID:    dungeonEntity.ID,
		MonsterTurns: convertMonsterTurnsToEntityEvents(monsterTurns, roomData),
	})

	// 20. Publish MonsterTurnCompleted events if monsters went first
	for _, mt := range monsterTurns {
		o.publishEvent(ctx, input.EncounterID, entities.EventTypeMonsterTurnCompleted, &entities.MonsterTurnCompletedEvent{
			MonsterID:         mt.MonsterID,
			MonsterName:       mt.MonsterName,
			Actions:           mt.Actions,
			Movement:          mt.Movement,
			Room:              roomData,
			RoomOrigin:        startRoomOrigin,
			Walls:             startRoom.Walls,
			UpdatedCharacters: mt.UpdatedCharacters,
		})
	}

	return &StartCombatOutput{
		CombatState:  combatState,
		Room:         roomData,
		RoomOrigin:   startRoomOrigin,
		Walls:        o.convertToWallInfo(startRoom),
		MonsterTurns: monsterTurns,
		Doors:        doors,
		DungeonID:    dungeonEntity.ID,
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
			member.CharacterData = charOutput.Character.Data
		}

		party = append(party, member)
	}

	return party
}

// GetEncounterState returns a full snapshot of the encounter state.
// Used by clients for the load-then-stream pattern to sync state before processing events.
func (o *Orchestrator) GetEncounterState(ctx context.Context, input *GetEncounterStateInput) (*GetEncounterStateOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.PlayerID == "" {
		return nil, fmt.Errorf("player ID is required")
	}

	// Load encounter data
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, ErrEncounterNotFound
	}

	encData := encOutput.Data

	// Validate player is in the encounter
	if _, exists := encData.Players[input.PlayerID]; !exists {
		return nil, ErrPlayerNotInEncounter
	}

	// Build party list with character data
	party := o.buildPartyFromPlayers(ctx, encData.Players, encData.HostID)

	// Map repository state to string state
	stateStr := string(encData.State)

	output := &GetEncounterStateOutput{
		EncounterID: encData.ID,
		State:       stateStr,
		Party:       party,
		JoinCode:    encData.JoinCode,
		HostID:      encData.HostID,
		LastEventID: encData.LastEventID,
	}

	// If combat is active or paused, include combat state
	if encData.State == encounterrepo.StateActive || encData.State == encounterrepo.StatePaused {
		// Build combat state
		if encData.InitiativeData != nil {
			combatState := o.buildCombatState(encData.ID, encData)
			output.CombatState = combatState
		}

		// Include room data
		output.Room = encData.RoomData

		// Build monster combat states
		if encData.Monsters != nil {
			monsters := make([]*MonsterCombatState, 0, len(encData.Monsters))
			for _, m := range encData.Monsters {
				if m.HitPoints > 0 { // Only include alive monsters
					monsters = append(monsters, &MonsterCombatState{
						MonsterID:        m.ID,
						MonsterName:      m.Name,
						CurrentHitPoints: m.HitPoints,
						MaxHitPoints:     m.MaxHitPoints,
						MonsterType:      dungeon.MonsterTypeFromRef(m.Ref),
					})
				}
			}
			output.Monsters = monsters
		}

		// Load dungeon to get doors, walls, and room origin for current room
		if o.dungeonRepo != nil {
			dungeonOutput, dungeonErr := o.dungeonRepo.GetByEncounterID(ctx, &dungeonrepo.GetByEncounterIDInput{
				EncounterID: input.EncounterID,
			})
			if dungeonErr == nil && dungeonOutput != nil && dungeonOutput.Dungeon != nil {
				dungeonEntity := dungeonOutput.Dungeon
				output.DungeonID = dungeonEntity.ID
				output.RoomOrigin = getCurrentRoomOrigin(dungeonEntity)
				// Get doors and walls for current room (start room or current exploration room)
				currentRoomID := dungeonEntity.CurrentRoomID
				if currentRoomID == "" {
					currentRoomID = dungeonEntity.StartRoomID
				}
				output.Doors = o.getDoorInfoForRoom(dungeonEntity, currentRoomID)
				// Get walls from dungeon room
				if currentRoom := dungeonEntity.GetRoom(currentRoomID); currentRoom != nil {
					output.Walls = o.convertToWallInfo(currentRoom)
				}
			}
		}
	}

	return output, nil
}

// GetEncounterHistory retrieves historical events for an encounter.
// Used by late joiners to populate their event log before streaming new events.
func (o *Orchestrator) GetEncounterHistory(ctx context.Context, input *GetEncounterHistoryInput) (*GetEncounterHistoryOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}

	// If encounterLogRepo is not configured, return empty history
	if o.encounterLogRepo == nil {
		return &GetEncounterHistoryOutput{
			Events:  []*entities.EncounterEvent{},
			HasMore: false,
		}, nil
	}

	// Retrieve events from the encounter log repository
	result, err := o.encounterLogRepo.GetByEncounter(ctx, &encounterlogrepo.GetByEncounterInput{
		EncounterID: input.EncounterID,
		UpToEventID: input.UpToEventID,
		Limit:       input.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get encounter history: %w", err)
	}

	return &GetEncounterHistoryOutput{
		Events:      result.Events,
		HasMore:     result.HasMore,
		LastEventID: result.LastEventID,
	}, nil
}

// ============================================================================
// TWO-LEVEL ACTION ECONOMY METHODS
// ============================================================================

// ActivateCombatAbility activates a combat ability (ATTACK, DASH, DODGE, etc.)
// This consumes action economy resources and grants capacity to execute actions
func (o *Orchestrator) ActivateCombatAbility(
	ctx context.Context,
	input *ActivateCombatAbilityInput,
) (*ActivateCombatAbilityOutput, error) {
	// 1. Validate input
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.EntityID == "" {
		return nil, fmt.Errorf("entity ID is required")
	}

	// 2. Load encounter data
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load encounter: %w", err)
	}
	if encOutput.Data == nil {
		return nil, fmt.Errorf("encounter not found: %s", input.EncounterID)
	}

	// 3. Validate initiative data exists and check it's the entity's turn
	if encOutput.Data.InitiativeData == nil {
		return nil, fmt.Errorf("no initiative data for encounter: %s", input.EncounterID)
	}
	if len(encOutput.Data.InitiativeData.Order) == 0 {
		return nil, fmt.Errorf("empty turn order for encounter: %s", input.EncounterID)
	}

	currentIndex := encOutput.Data.InitiativeData.Current
	if currentIndex < 0 || currentIndex >= len(encOutput.Data.InitiativeData.Order) {
		return nil, fmt.Errorf("invalid active index %d for turn order of length %d",
			currentIndex, len(encOutput.Data.InitiativeData.Order))
	}

	activeEntity := encOutput.Data.InitiativeData.Order[currentIndex]
	if activeEntity.ID != input.EntityID {
		return nil, fmt.Errorf("not entity's turn: active entity is %s, not %s",
			activeEntity.ID, input.EntityID)
	}

	// 4. Map proto ability ID to toolkit ref
	abilityRef := protoAbilityIDToRef(input.AbilityID)
	if abilityRef == nil {
		return &ActivateCombatAbilityOutput{
			Success: false,
			Error:   fmt.Sprintf("unknown or unimplemented ability: %v", input.AbilityID),
		}, nil
	}

	// 5. Load character and ensure in combat
	turnNum := computeTurnNumber(encOutput.Data)
	char, charOutput, _, err := o.loadCharacterForCombat(ctx, input.EntityID, turnNum)
	if err != nil {
		return nil, err
	}

	// 6. Delegate to toolkit Character
	abilityOutput, err := char.ActivateAbility(ctx, &character.ActivateAbilityInput{
		AbilityRef: abilityRef,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to activate ability: %w", err)
	}

	// 7. Persist character state (action economy is now on the character)
	if err = o.persistCharacterData(ctx, char, charOutput); err != nil {
		return nil, err
	}

	// 8. Build CombatState response
	combatState := o.buildCombatState(input.EncounterID, encOutput.Data)

	// 9. Convert toolkit ability/action lists to entity types
	availableAbilities := convertCharAbilitiesToEntities(abilityOutput.Abilities)
	availableActions := convertCharActionsToEntities(abilityOutput.Actions)

	if !abilityOutput.Success {
		return &ActivateCombatAbilityOutput{
			Success:            false,
			Error:              abilityOutput.Error,
			ActionEconomy:      encOutput.Data.ActionEconomy,
			CombatState:        combatState,
			AvailableAbilities: availableAbilities,
			AvailableActions:   availableActions,
		}, nil
	}

	return &ActivateCombatAbilityOutput{
		Success:            true,
		ActionEconomy:      encOutput.Data.ActionEconomy,
		GrantedCapacity:    abilityOutput.GrantedCapacity,
		CombatState:        combatState,
		AvailableAbilities: availableAbilities,
		AvailableActions:   availableActions,
	}, nil
}

// ExecuteAction executes an action that consumes granted capacity
// Use after ActivateCombatAbility to perform strikes, moves, etc.
func (o *Orchestrator) ExecuteAction(
	ctx context.Context,
	input *ExecuteActionInput,
) (*ExecuteActionOutput, error) {
	// 1. Validate input
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.EntityID == "" {
		return nil, fmt.Errorf("entity ID is required")
	}
	if input.ActionID == pb.ActionId_ACTION_ID_UNSPECIFIED {
		return nil, fmt.Errorf("action ID is required")
	}

	// 2. Load encounter data
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load encounter: %w", err)
	}
	if encOutput.Data == nil {
		return nil, fmt.Errorf("encounter not found: %s", input.EncounterID)
	}

	// 3. Get action economy from encounter data
	actionEconomy := encOutput.Data.ActionEconomy
	if actionEconomy == nil {
		// Initialize if not present (backwards compatibility)
		actionEconomy = entities.NewActionEconomyState()
	}

	// 4. Execute based on action type
	switch input.ActionID {
	case pb.ActionId_ACTION_ID_STRIKE:
		return o.executeStrike(ctx, input, encOutput.Data, actionEconomy, combat.AttackHandMain)

	case pb.ActionId_ACTION_ID_OFF_HAND_STRIKE:
		return o.executeStrike(ctx, input, encOutput.Data, actionEconomy, combat.AttackHandOff)

	case pb.ActionId_ACTION_ID_FLURRY_STRIKE:
		return o.executeFlurryStrike(ctx, input, encOutput.Data, actionEconomy)

	case pb.ActionId_ACTION_ID_UNARMED_STRIKE:
		return o.executeUnarmedStrike(ctx, input, encOutput.Data, actionEconomy)

	case pb.ActionId_ACTION_ID_MOVE:
		return o.executeMove(ctx, input, encOutput.Data, actionEconomy)

	default:
		return nil, fmt.Errorf("unsupported action ID: %s", input.ActionID.String())
	}
}

// executeStrike handles STRIKE and OFF_HAND_STRIKE actions
func (o *Orchestrator) executeStrike(
	ctx context.Context,
	input *ExecuteActionInput,
	encData *encounterrepo.EncounterData,
	_ *entities.ActionEconomyState,
	attackHand combat.AttackHand,
) (*ExecuteActionOutput, error) {
	// 1. Validate target
	if input.TargetID == "" {
		return nil, fmt.Errorf("target ID is required for strike actions")
	}

	// 2. Load character and ensure in combat
	char, charOutput, bus, err := o.loadCharacterForCombat(ctx, input.EntityID, computeTurnNumber(encData))
	if err != nil {
		return nil, err
	}

	// 3. Determine action ref based on attack hand
	actionRef := refs.Actions.Strike()
	if attackHand == combat.AttackHandOff {
		actionRef = refs.Actions.OffHandStrike()
	}

	// 4. Delegate action economy check to toolkit Character
	execOutput, err := char.ExecuteAction(ctx, &character.ExecuteActionInput{
		ActionRef: actionRef,
		TargetID:  input.TargetID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute action: %w", err)
	}

	if !execOutput.Success {
		return &ExecuteActionOutput{
			Success:            false,
			Error:              execOutput.Error,
			ActionEconomy:      encData.ActionEconomy,
			AvailableAbilities: convertCharAbilitiesToEntities(execOutput.Abilities),
			AvailableActions:   convertCharActionsToEntities(execOutput.Actions),
		}, nil
	}

	// 5. Load monster from encounter data
	monsterData := o.findMonsterData(encData, input.TargetID)
	if monsterData == nil {
		return nil, fmt.Errorf("monster not found: %s", input.TargetID)
	}

	// Create monster instance from stored data
	monsterInstance, err := monster.LoadFromData(ctx, monsterData, bus)
	if err != nil {
		return nil, fmt.Errorf("failed to load monster from data: %w", err)
	}

	// Load monster conditions/traits
	if err = monstertraits.LoadMonsterConditions(ctx, monsterInstance, monsterData.Conditions, bus, o.roller); err != nil {
		return nil, fmt.Errorf("failed to load monster conditions: %w", err)
	}

	// 6. Get weapon and equipment slots
	weapon, equipmentSlots := o.getEquippedWeaponAndSlots(ctx, input.EntityID)

	// Override weapon if specified in input
	if input.WeaponID != "" {
		if w, weaponErr := weapons.GetByID(input.WeaponID); weaponErr == nil {
			weapon = w
		}
	}

	// 7. Build GameContext
	gameCtx := o.buildGameContextFromEquipment(input.EntityID, &weapon, equipmentSlots)
	ctx = gamectx.WithGameContext(ctx, gameCtx)

	// 8. Create CombatantRegistry for damage chain lookups
	registry := gamectx.NewCombatantRegistry()
	registry.Add(char)
	registry.Add(monsterInstance)
	ctx = combat.WithCombatantLookup(ctx, registry)

	// 9. Call toolkit combat
	result, err := combat.ResolveAttack(ctx, &combat.AttackInput{
		AttackerID: input.EntityID,
		TargetID:   input.TargetID,
		Weapon:     &weapon,
		EventBus:   bus,
		Roller:     o.roller,
		AttackHand: attackHand,
	})
	if err != nil {
		return nil, fmt.Errorf("combat resolution failed: %w", err)
	}

	// 10. Get monster HP after damage
	newHP := monsterInstance.HP()
	if result.Hit {
		monsterData.HitPoints = newHP
	}

	// 11. Persist character state (action economy updated by ExecuteAction)
	if err = o.persistCharacterData(ctx, char, charOutput); err != nil {
		return nil, err
	}

	// 12. Persist encounter state (monster HP)
	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID: input.EncounterID,
		Monsters:    encData.Monsters,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save encounter state: %w", err)
	}

	// 13. Build attack result
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

	if result.Breakdown != nil {
		attackResult.Breakdown = convertToolkitBreakdown(result.Breakdown)
	}

	// 14. Get room data for response
	var roomData interface{}
	if encData.RoomData != nil {
		roomData = encData.RoomData
	}

	// 15. Build combat state for response
	combatState := o.buildCombatState(input.EncounterID, encData)

	// 16. Check for dungeon victory if monster died
	if newHP <= 0 {
		o.checkAndHandleVictory(ctx, input.EncounterID, encData, input.TargetID)
	}

	// 17. Use toolkit's ability/action lists from ExecuteAction output
	availableAbilities := convertCharAbilitiesToEntities(execOutput.Abilities)
	availableActions := convertCharActionsToEntities(execOutput.Actions)

	return &ExecuteActionOutput{
		Success:            true,
		ActionEconomy:      encData.ActionEconomy,
		AttackResult:       attackResult,
		CombatState:        combatState,
		Room:               roomData,
		AvailableAbilities: availableAbilities,
		AvailableActions:   availableActions,
	}, nil
}

// executeFlurryStrike handles FLURRY_STRIKE actions (Monk's Flurry of Blows unarmed strikes).
// Mechanically identical to a main-hand unarmed strike but consumes flurry strike capacity.
func (o *Orchestrator) executeFlurryStrike(
	ctx context.Context,
	input *ExecuteActionInput,
	encData *encounterrepo.EncounterData,
	_ *entities.ActionEconomyState,
) (*ExecuteActionOutput, error) {
	// 1. Validate target
	if input.TargetID == "" {
		return nil, fmt.Errorf("target ID is required for flurry strike")
	}

	// 2. Load character and ensure in combat
	char, charOutput, bus, err := o.loadCharacterForCombat(ctx, input.EntityID, computeTurnNumber(encData))
	if err != nil {
		return nil, err
	}

	// 3. Delegate action economy check to toolkit Character
	execOutput, err := char.ExecuteAction(ctx, &character.ExecuteActionInput{
		ActionRef: refs.Actions.FlurryStrike(),
		TargetID:  input.TargetID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute action: %w", err)
	}

	if !execOutput.Success {
		return &ExecuteActionOutput{
			Success:            false,
			Error:              execOutput.Error,
			ActionEconomy:      encData.ActionEconomy,
			AvailableAbilities: convertCharAbilitiesToEntities(execOutput.Abilities),
			AvailableActions:   convertCharActionsToEntities(execOutput.Actions),
		}, nil
	}

	// 4. Load monster
	monsterData := o.findMonsterData(encData, input.TargetID)
	if monsterData == nil {
		return nil, fmt.Errorf("monster not found: %s", input.TargetID)
	}

	monsterInstance, err := monster.LoadFromData(ctx, monsterData, bus)
	if err != nil {
		return nil, fmt.Errorf("failed to load monster from data: %w", err)
	}

	if err = monstertraits.LoadMonsterConditions(ctx, monsterInstance, monsterData.Conditions, bus, o.roller); err != nil {
		return nil, fmt.Errorf("failed to load monster conditions: %w", err)
	}

	// 5. Flurry strikes are always unarmed
	unarmedStrike := unarmedStrikeWeapon()

	// 6. Build GameContext
	gameCtx := o.buildGameContextFromEquipment(input.EntityID, &unarmedStrike, nil)
	ctx = gamectx.WithGameContext(ctx, gameCtx)

	// 7. CombatantRegistry
	registry := gamectx.NewCombatantRegistry()
	registry.Add(char)
	registry.Add(monsterInstance)
	ctx = combat.WithCombatantLookup(ctx, registry)

	// 8. Resolve attack
	result, err := combat.ResolveAttack(ctx, &combat.AttackInput{
		AttackerID: input.EntityID,
		TargetID:   input.TargetID,
		Weapon:     &unarmedStrike,
		EventBus:   bus,
		Roller:     o.roller,
		AttackHand: combat.AttackHandMain,
	})
	if err != nil {
		return nil, fmt.Errorf("flurry strike resolution failed: %w", err)
	}

	// 9. Update monster HP
	if result.Hit {
		monsterData.HitPoints = monsterInstance.HP()
	}

	// 10. Persist character and encounter state
	if err = o.persistCharacterData(ctx, char, charOutput); err != nil {
		return nil, err
	}

	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID: input.EncounterID,
		Monsters:    encData.Monsters,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save encounter state: %w", err)
	}

	// 11. Build result
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
	if result.Breakdown != nil {
		attackResult.Breakdown = convertToolkitBreakdown(result.Breakdown)
	}

	combatState := o.buildCombatState(input.EncounterID, encData)

	if monsterInstance.HP() <= 0 {
		o.checkAndHandleVictory(ctx, input.EncounterID, encData, input.TargetID)
	}

	return &ExecuteActionOutput{
		Success:            true,
		ActionEconomy:      encData.ActionEconomy,
		AttackResult:       attackResult,
		CombatState:        combatState,
		Room:               encData.RoomData,
		AvailableAbilities: convertCharAbilitiesToEntities(execOutput.Abilities),
		AvailableActions:   convertCharActionsToEntities(execOutput.Actions),
	}, nil
}

// executeUnarmedStrike handles UNARMED_STRIKE actions (Martial Arts bonus strike).
// Mirrors executeFlurryStrike but consumes martial arts bonus capacity.
// Martial Arts conditions upgrade the 1d1 base damage via the damage chain.
func (o *Orchestrator) executeUnarmedStrike(
	ctx context.Context,
	input *ExecuteActionInput,
	encData *encounterrepo.EncounterData,
	_ *entities.ActionEconomyState,
) (*ExecuteActionOutput, error) {
	// 1. Validate target
	if input.TargetID == "" {
		return nil, fmt.Errorf("target ID is required for unarmed strike")
	}

	// 2. Load character and ensure in combat
	char, charOutput, bus, err := o.loadCharacterForCombat(ctx, input.EntityID, computeTurnNumber(encData))
	if err != nil {
		return nil, err
	}

	// 3. Delegate action economy check to toolkit Character
	execOutput, err := char.ExecuteAction(ctx, &character.ExecuteActionInput{
		ActionRef: refs.Actions.UnarmedStrike(),
		TargetID:  input.TargetID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute action: %w", err)
	}

	if !execOutput.Success {
		return &ExecuteActionOutput{
			Success:            false,
			Error:              execOutput.Error,
			ActionEconomy:      encData.ActionEconomy,
			AvailableAbilities: convertCharAbilitiesToEntities(execOutput.Abilities),
			AvailableActions:   convertCharActionsToEntities(execOutput.Actions),
		}, nil
	}

	// 4. Load monster
	monsterData := o.findMonsterData(encData, input.TargetID)
	if monsterData == nil {
		return nil, fmt.Errorf("monster not found: %s", input.TargetID)
	}

	monsterInstance, err := monster.LoadFromData(ctx, monsterData, bus)
	if err != nil {
		return nil, fmt.Errorf("failed to load monster from data: %w", err)
	}

	if err = monstertraits.LoadMonsterConditions(ctx, monsterInstance, monsterData.Conditions, bus, o.roller); err != nil {
		return nil, fmt.Errorf("failed to load monster conditions: %w", err)
	}

	// 5. Use shared unarmed strike weapon
	unarmedStrike := unarmedStrikeWeapon()

	// 6. Build GameContext
	gameCtx := o.buildGameContextFromEquipment(input.EntityID, &unarmedStrike, nil)
	ctx = gamectx.WithGameContext(ctx, gameCtx)

	// 7. CombatantRegistry
	registry := gamectx.NewCombatantRegistry()
	registry.Add(char)
	registry.Add(monsterInstance)
	ctx = combat.WithCombatantLookup(ctx, registry)

	// 8. Resolve attack
	result, err := combat.ResolveAttack(ctx, &combat.AttackInput{
		AttackerID: input.EntityID,
		TargetID:   input.TargetID,
		Weapon:     &unarmedStrike,
		EventBus:   bus,
		Roller:     o.roller,
		AttackHand: combat.AttackHandMain,
	})
	if err != nil {
		return nil, fmt.Errorf("unarmed strike resolution failed: %w", err)
	}

	// 9. Update monster HP
	if result.Hit {
		monsterData.HitPoints = monsterInstance.HP()
	}

	// 10. Check for dungeon victory if monster died
	if monsterInstance.HP() <= 0 {
		o.checkAndHandleVictory(ctx, input.EncounterID, encData, input.TargetID)
	}

	// 11. Persist character and encounter state
	if err = o.persistCharacterData(ctx, char, charOutput); err != nil {
		return nil, err
	}

	_, err = o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID: input.EncounterID,
		Monsters:    encData.Monsters,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save encounter state: %w", err)
	}

	// 12. Build result
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
	if result.Breakdown != nil {
		attackResult.Breakdown = convertToolkitBreakdown(result.Breakdown)
	}

	combatState := o.buildCombatState(input.EncounterID, encData)

	return &ExecuteActionOutput{
		Success:            true,
		AttackResult:       attackResult,
		ActionEconomy:      encData.ActionEconomy,
		CombatState:        combatState,
		Room:               encData.RoomData,
		AvailableAbilities: convertCharAbilitiesToEntities(execOutput.Abilities),
		AvailableActions:   convertCharActionsToEntities(execOutput.Actions),
	}, nil
}

// executeMove handles MOVE actions
func (o *Orchestrator) executeMove(
	ctx context.Context,
	input *ExecuteActionInput,
	encData *encounterrepo.EncounterData,
	_ *entities.ActionEconomyState,
) (*ExecuteActionOutput, error) {
	// 1. Validate path
	if len(input.Path) == 0 {
		return nil, fmt.Errorf("path is required for move actions")
	}

	// 2. Get movement remaining from encounter data
	movementRemaining := encData.MovementRemaining
	if movementRemaining == 0 {
		movementRemaining = 30 // Default if not set
	}

	// 3. Load dungeon walls and room origin for collision detection
	var dungeonWalls []dungeon.WallSegment
	var execActionRoomOrigin *dungeon.Position
	if o.dungeonRepo != nil {
		dungeonOutput, dungeonErr := o.dungeonRepo.GetByEncounterID(ctx, &dungeonrepo.GetByEncounterIDInput{
			EncounterID: input.EncounterID,
		})
		if dungeonErr == nil && dungeonOutput != nil && dungeonOutput.Dungeon != nil {
			dungeonEntity := dungeonOutput.Dungeon
			execActionRoomOrigin = getCurrentRoomOrigin(dungeonEntity)
			currentRoomID := dungeonEntity.CurrentRoomID
			if currentRoomID == "" {
				currentRoomID = dungeonEntity.StartRoomID
			}
			if currentRoom := dungeonEntity.GetRoom(currentRoomID); currentRoom != nil {
				dungeonWalls = currentRoom.Walls
			}
		}
	}

	// 4. Get or create room data
	if encData.RoomData == nil {
		encData.RoomData = &spatial.RoomData{
			ID:           input.EncounterID + "-room",
			Type:         "dungeon",
			Width:        20,
			Height:       20,
			GridType:     spatial.GridTypeHex,
			CubeEntities: make(map[string]spatial.EntityCubePlacement),
		}
	}

	roomData, ok := encData.RoomData.(*spatial.RoomData)
	if !ok {
		if roomDataVal, ok := encData.RoomData.(spatial.RoomData); ok {
			roomData = &roomDataVal
		} else {
			return nil, fmt.Errorf("invalid room data type in encounter")
		}
	}

	// 5. Get current position
	var oldCube spatial.CubeCoordinate
	cubePlacement, exists := roomData.CubeEntities[input.EntityID]
	if exists {
		oldCube = cubePlacement.CubePosition
	}

	// 6. Calculate total movement cost for the path
	totalMovementUsed := 0
	currentPos := oldCube
	var finalPosition *Position
	stopReason := stopReasonCompleted

	for _, targetPos := range input.Path {
		targetCube := spatial.CubeCoordinate{
			X: int(targetPos.X),
			Y: int(targetPos.Y),
			Z: int(targetPos.Z),
		}

		// Validate cube coordinates sum to zero
		if roomData.GridType == spatial.GridTypeHex {
			if targetCube.X+targetCube.Y+targetCube.Z != 0 {
				finalPosition = &Position{
					X: float64(currentPos.X),
					Y: float64(currentPos.Y),
					Z: float64(currentPos.Z),
				}
				stopReason = stopReasonInvalidCoordinates
				break
			}
		}

		// Check if position is occupied
		for id, entity := range roomData.CubeEntities {
			if id != input.EntityID && entity.BlocksMovement {
				if entity.CubePosition.X == targetCube.X &&
					entity.CubePosition.Y == targetCube.Y &&
					entity.CubePosition.Z == targetCube.Z {
					finalPosition = &Position{
						X: float64(currentPos.X),
						Y: float64(currentPos.Y),
						Z: float64(currentPos.Z),
					}
					stopReason = stopReasonPositionOccupied
					break
				}
			}
		}
		if stopReason != stopReasonCompleted {
			break
		}

		// Check wall collision
		if len(dungeonWalls) > 0 && exists {
			wallValidator := dungeontoolkit.NewWallValidator()
			startPos := dungeon.Position{X: currentPos.X, Y: currentPos.Y, Z: currentPos.Z}
			endPos := dungeon.Position{X: targetCube.X, Y: targetCube.Y, Z: targetCube.Z}

			if wallValidator.PathCrossesWall(startPos, endPos, dungeonWalls) {
				finalPosition = &Position{
					X: float64(currentPos.X),
					Y: float64(currentPos.Y),
					Z: float64(currentPos.Z),
				}
				stopReason = stopReasonBlockedByWall
				break
			}
		}

		// Calculate distance
		dx := targetCube.X - currentPos.X
		dy := targetCube.Y - currentPos.Y
		dz := targetCube.Z - currentPos.Z
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if dz < 0 {
			dz = -dz
		}
		distance := (dx + dy + dz) / 2
		movementCost := distance * 5 // Each hex is 5 feet

		// Check if enough movement
		//nolint:gosec // G115: Game values bounded by movement limits, no overflow risk
		if int32(totalMovementUsed+movementCost) > movementRemaining {
			finalPosition = &Position{
				X: float64(currentPos.X),
				Y: float64(currentPos.Y),
				Z: float64(currentPos.Z),
			}
			stopReason = stopReasonInsufficientMovement
			break
		}

		// Move succeeded, update position
		totalMovementUsed += movementCost
		currentPos = targetCube
		finalPosition = &Position{
			X: float64(targetCube.X),
			Y: float64(targetCube.Y),
			Z: float64(targetCube.Z),
		}
	}

	// 7. Update entity position in room data
	entityType := "character"
	if exists {
		entityType = cubePlacement.EntityType
	}
	roomData.CubeEntities[input.EntityID] = spatial.EntityCubePlacement{
		EntityID:       input.EntityID,
		EntityType:     entityType,
		CubePosition:   currentPos,
		Size:           1,
		BlocksMovement: true,
	}

	// 8. Update movement remaining
	//nolint:gosec // G115: Game values are bounded by room size limits, no overflow risk
	newMovementRemaining := movementRemaining - int32(totalMovementUsed)

	// 9. Persist updated state
	_, err := o.encRepo.Update(ctx, &encounterrepo.UpdateInput{
		EncounterID:       input.EncounterID,
		RoomData:          roomData,
		MovementRemaining: &newMovementRemaining,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save encounter state: %w", err)
	}

	// 10. Publish MovementCompleted event
	o.publishEvent(ctx, input.EncounterID, entities.EventTypeMovementCompleted, &entities.MovementCompletedEvent{
		EntityID:          input.EntityID,
		EntityType:        entityType,
		FinalPosition:     finalPosition,
		MovementRemaining: newMovementRemaining,
		StopReason:        stopReason,
		UpdatedRoom:       roomData,
		RoomOrigin:        execActionRoomOrigin,
		Walls:             dungeonWalls,
	})

	// 11. Update movement remaining in encounter data for combat state
	encData.MovementRemaining = newMovementRemaining

	// 12. Build combat state for response
	combatState := o.buildCombatState(input.EncounterID, encData)

	// 13. Load character for available abilities/actions from toolkit
	var availableAbilities []*entities.AvailableAbility
	var availableActions []*entities.AvailableAction

	char, _, _, loadErr := o.loadCharacterForCombat(ctx, input.EntityID, computeTurnNumber(encData))
	if loadErr == nil {
		availableAbilities = convertCharAbilitiesToEntities(char.AvailableAbilities())
		availableActions = convertCharActionsToEntities(char.AvailableActions())
	}

	return &ExecuteActionOutput{
		Success:       stopReason == stopReasonCompleted || stopReason == stopReasonInsufficientMovement,
		ActionEconomy: encData.ActionEconomy,
		MoveResult: &MoveResult{
			FinalPosition: finalPosition,
			MovementUsed:  totalMovementUsed,
			StopReason:    stopReason,
		},
		CombatState:        combatState,
		Room:               roomData,
		RoomOrigin:         execActionRoomOrigin,
		AvailableAbilities: availableAbilities,
		AvailableActions:   availableActions,
	}, nil
}
