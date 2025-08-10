// Package encounter implements the encounter orchestrator for managing D&D 5e encounters
package encounter

//go:generate mockgen -destination=mock/mock_service.go -package=encountermock github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter Service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// Service defines the interface for encounter operations
type Service interface {
	// DungeonStart creates a simple dungeon encounter for testing
	DungeonStart(ctx context.Context, input *DungeonStartInput) (*DungeonStartOutput, error)

	// NextTurn advances to the next turn in the encounter
	NextTurn(ctx context.Context, input *NextTurnInput) (*NextTurnOutput, error)

	// GetTurnOrder returns the current turn order
	GetTurnOrder(ctx context.Context, input *GetTurnOrderInput) (*GetTurnOrderOutput, error)

	// MoveCharacter moves a character to a new position
	MoveCharacter(ctx context.Context, input *MoveCharacterInput) (*MoveCharacterOutput, error)
}

// Config holds the dependencies for the encounter orchestrator
type Config struct {
	IDGenerator      idgen.Generator
	Repository       encounters.Repository
	CharacterService character.Service
}

// Validate ensures all required dependencies are provided
func (c *Config) Validate() error {
	vb := errors.NewValidationBuilder()

	if c.IDGenerator == nil {
		vb.RequiredField("IDGenerator")
	}

	if c.Repository == nil {
		vb.RequiredField("Repository")
	}

	if c.CharacterService == nil {
		vb.RequiredField("CharacterService")
	}

	return vb.Build()
}

type orchestrator struct {
	idGen            idgen.Generator
	repo             encounters.Repository
	characterService character.Service
}

// simpleEntity implements core.Entity for demo purposes
type simpleEntity struct {
	id         string
	entityType string
}

func (e *simpleEntity) GetID() string {
	return e.id
}

func (e *simpleEntity) GetType() string {
	return e.entityType
}

// NewOrchestrator creates a new encounter orchestrator with the provided dependencies
func NewOrchestrator(cfg *Config) (Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	return &orchestrator{
		idGen:            cfg.IDGenerator,
		repo:             cfg.Repository,
		characterService: cfg.CharacterService,
	}, nil
}

// DungeonStart creates a simple dungeon encounter for testing
func (o *orchestrator) DungeonStart(ctx context.Context, input *DungeonStartInput) (*DungeonStartOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	// Generate unique encounter ID
	encounterID := o.idGen.Generate()

	slog.Info("Dungeon encounter creation requested",
		"encounter_id", encounterID,
		"character_count", len(input.CharacterIDs),
	)

	// Create a 10x10 hex grid room with pointy-top orientation (D&D 5e standard)
	hexGrid := spatial.NewHexGrid(spatial.HexGridConfig{
		Width:     10,   // width in hex units
		Height:    10,   // height in hex units
		PointyTop: true, // pointy-top orientation for D&D 5e
	})

	// Create room with the hex grid
	room := spatial.NewBasicRoom(spatial.BasicRoomConfig{
		ID:   o.idGen.Generate(),
		Type: "dungeon",
		Grid: hexGrid,
	})

	// Add obstacles to make the room interesting
	// Use specified template or random if not provided
	var template RoomTemplate
	if input.RoomTemplate != nil {
		template = *input.RoomTemplate
	} else {
		template = GetRandomTemplate()
	}
	GenerateRoomObstacles(room, template, o.idGen.Generate)

	slog.Info("Generated room with obstacles",
		"template", template,
		"encounter_id", encounterID,
	)

	// Add character placements - spread them out in the starting area
	for i, characterID := range input.CharacterIDs {
		entityPos := spatial.Position{
			X: float64(2 + i), // Spread characters horizontally
			Y: 3.0,            // Starting row
		}

		// Create a simple entity for the character
		charEntity := &simpleEntity{
			id:         characterID,
			entityType: "character",
		}

		if err := room.PlaceEntity(charEntity, entityPos); err != nil {
			slog.Warn("Failed to place character entity",
				"character_id", characterID,
				"position", entityPos,
				"error", err,
			)
		}
	}

	// Add a demo monster for the encounter
	monsterPos := spatial.Position{
		X: 7.0, // Opposite side from characters
		Y: 6.0,
	}
	monsterID := o.idGen.Generate()
	monsterEntity := &simpleEntity{
		id:         monsterID,
		entityType: "monster",
	}
	if err := room.PlaceEntity(monsterEntity, monsterPos); err != nil {
		slog.Warn("Failed to place monster entity",
			"monster_id", monsterID,
			"position", monsterPos,
			"error", err,
		)
	}

	// Create initiative order - characters and monsters
	entities := make(map[core.Entity]int)

	// Add characters with their actual DEX modifiers
	for _, characterID := range input.CharacterIDs {
		charEntity := &simpleEntity{
			id:         characterID,
			entityType: "character",
		}

		// Get character data to extract DEX modifier
		dexModifier := 0 // Default if we can't fetch character
		charOutput, err := o.characterService.GetCharacter(ctx, &character.GetCharacterInput{
			CharacterID: characterID,
		})
		if err != nil {
			slog.Warn("Failed to fetch character for DEX modifier, using default",
				"character_id", characterID,
				"error", err,
			)
		} else if charOutput != nil && charOutput.Character != nil {
			dexModifier = charOutput.Character.AbilityScores.Modifier(constants.DEX)
			slog.Debug("Using character DEX modifier for initiative",
				"character_id", characterID,
				"dex_modifier", dexModifier,
			)
		}

		entities[charEntity] = dexModifier
	}

	// Add the monster with a configurable DEX modifier
	monsterDexModifier := 2 // Default monster DEX modifier
	if input.MonsterDexModifier != nil {
		monsterDexModifier = *input.MonsterDexModifier
	}
	entities[monsterEntity] = monsterDexModifier

	// Roll initiative and create tracker
	rolls := initiative.RollForOrder(entities, nil) // nil uses default dice roller

	// Log the initiative rolls for debugging
	for _, roll := range rolls {
		slog.Info("Initiative rolled",
			"entity_id", roll.Entity.GetID(),
			"d20_roll", roll.Roll,
			"modifier", roll.Modifier,
			"total", roll.Total,
		)
	}

	// Convert rolls to entities for tracker (extract just the entities in order)
	orderedEntities := make([]core.Entity, len(rolls))
	for i, roll := range rolls {
		orderedEntities[i] = roll.Entity
	}

	tracker := initiative.New(orderedEntities)

	// Auto-advance through monster turns
	// Keep advancing until we reach a non-monster entity
	for {
		current := tracker.Current()
		if current == nil {
			break
		}

		// Check if current turn is a monster
		if current.GetType() == "monster" {
			slog.Info("Auto-ending monster turn",
				"entity_id", current.GetID(),
				"round", tracker.Round(),
			)
			tracker.Next()
		} else {
			// It's a player's turn, stop advancing
			break
		}
	}

	// Get current turn after auto-advancing
	current := tracker.Current()
	currentTurn := ""
	if current != nil {
		currentTurn = current.GetID()
	}

	// Convert to response data
	roomData := room.ToData()
	trackerData := tracker.ToData()

	// Save encounter to repository (including initiative rolls)
	_, err := o.repo.Save(ctx, &encounters.SaveInput{
		EncounterID:     encounterID,
		RoomData:        &roomData,
		InitiativeData:  &trackerData,
		InitiativeRolls: rolls,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to save encounter")
	}

	return &DungeonStartOutput{
		EncounterID:     encounterID,
		RoomData:        &roomData,
		InitiativeData:  &trackerData,
		InitiativeRolls: rolls,
		CurrentTurn:     currentTurn,
	}, nil
}

// NextTurn advances to the next turn in the encounter
func (o *orchestrator) NextTurn(ctx context.Context, input *NextTurnInput) (*NextTurnOutput, error) {
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

	// Recreate tracker from stored data
	tracker := initiative.LoadFromData(*getOutput.Data.InitiativeData)

	// Advance turn
	tracker.Next()

	// Auto-advance through monster turns
	// Keep advancing until we reach a non-monster entity
	for {
		current := tracker.Current()
		if current == nil {
			break
		}

		// Check if current turn is a monster
		if current.GetType() == "monster" {
			slog.Info("Auto-ending monster turn",
				"entity_id", current.GetID(),
				"encounter_id", input.EncounterID,
				"round", tracker.Round(),
			)
			tracker.Next()
		} else {
			// It's a player's turn, stop advancing
			break
		}
	}

	// Get current turn after auto-advancing
	current := tracker.Current()
	currentTurn := ""
	if current != nil {
		currentTurn = current.GetID()
	}

	// Update stored initiative data
	updatedData := tracker.ToData()
	_, err = o.repo.Update(ctx, &encounters.UpdateInput{
		EncounterID:    input.EncounterID,
		InitiativeData: &updatedData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update encounter")
	}

	slog.Info("Advanced turn",
		"encounter_id", input.EncounterID,
		"current_turn", currentTurn,
		"round", tracker.Round(),
	)

	return &NextTurnOutput{
		CurrentTurn: currentTurn,
		Round:       tracker.Round(),
	}, nil
}

// GetTurnOrder returns the current turn order
func (o *orchestrator) GetTurnOrder(ctx context.Context, input *GetTurnOrderInput) (*GetTurnOrderOutput, error) {
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

	// Recreate tracker to get current turn
	tracker := initiative.LoadFromData(*getOutput.Data.InitiativeData)
	current := tracker.Current()
	currentTurn := ""
	if current != nil {
		currentTurn = current.GetID()
	}

	return &GetTurnOrderOutput{
		InitiativeData:  getOutput.Data.InitiativeData,
		InitiativeRolls: getOutput.Data.InitiativeRolls,
		CurrentTurn:     currentTurn,
	}, nil
}

// MoveCharacter moves a character to a new position during their turn
func (o *orchestrator) MoveCharacter(ctx context.Context, input *MoveCharacterInput) (*MoveCharacterOutput, error) {
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

	// Verify it's the entity's turn
	tracker := initiative.LoadFromData(*getOutput.Data.InitiativeData)
	current := tracker.Current()
	if current == nil || current.GetID() != input.EntityID {
		return nil, errors.InvalidArgument("can only move on your turn")
	}

	// Recreate the room from stored data
	// TODO: Use LoadRoomFromContext when we have proper game context
	roomData := getOutput.Data.RoomData
	hexGrid := spatial.NewHexGrid(spatial.HexGridConfig{
		Width:     float64(roomData.Width),
		Height:    float64(roomData.Height),
		PointyTop: true, // D&D 5e standard
	})

	room := spatial.NewBasicRoom(spatial.BasicRoomConfig{
		ID:   roomData.ID,
		Type: roomData.Type,
		Grid: hexGrid,
	})

	// Restore entity positions
	for entityID, placement := range roomData.Entities {
		var entity core.Entity

		// Check if it's an obstacle type
		if placement.EntityType == "wall" || placement.EntityType == "pillar" ||
			placement.EntityType == "barrel" || placement.EntityType == "crate" ||
			placement.EntityType == "rubble" {
			entity = &obstacleEntity{
				id:         entityID,
				entityType: placement.EntityType,
				blocksMove: placement.BlocksMovement,
				blocksLOS:  placement.BlocksLineOfSight,
			}
		} else {
			// Regular entity (character/monster)
			entity = &simpleEntity{
				id:         entityID,
				entityType: placement.EntityType,
			}
		}

		if err := room.PlaceEntity(entity, placement.Position); err != nil {
			slog.Warn("Failed to restore entity position",
				"entity_id", entityID,
				"position", placement.Position,
				"error", err,
			)
		}
	}

	// Get current position of the entity
	currentPosition, exists := room.GetEntityPosition(input.EntityID)
	if !exists {
		return nil, errors.NotFound("entity not found in room")
	}

	// Calculate distance for movement
	grid := room.GetGrid()
	distance := grid.Distance(currentPosition, input.TargetPosition)

	// For now, use standard movement speed of 30 feet (6 hexes in D&D 5e)
	// TODO: Get actual speed from character data
	maxMovement := 30
	movementInHexes := int(distance)
	movementInFeet := movementInHexes * 5 // Each hex is 5 feet in D&D 5e

	slog.Debug("Movement calculation",
		"from", currentPosition,
		"to", input.TargetPosition,
		"distance", distance,
		"hexes", movementInHexes,
		"feet", movementInFeet,
		"maxFeet", maxMovement,
	)

	if movementInFeet > maxMovement {
		return nil, errors.InvalidArgument(fmt.Sprintf("target position exceeds movement speed: %d feet > %d feet maximum", movementInFeet, maxMovement))
	}

	// Check if target position is valid (not blocked)
	targetEntities := room.GetEntitiesAt(input.TargetPosition)
	for _, entity := range targetEntities {
		if entity.GetID() != input.EntityID { // Not ourselves
			// Check if it's an obstacle that blocks movement
			if obs, ok := entity.(*obstacleEntity); ok && obs.blocksMove {
				return nil, errors.InvalidArgument("target position is blocked by " + obs.entityType)
			}
			// Check if it's another character/monster
			if entity.GetType() == "character" || entity.GetType() == "monster" {
				return nil, errors.InvalidArgument("target position is occupied by another entity")
			}
		}
	}

	// Move the entity
	if err := room.MoveEntity(input.EntityID, input.TargetPosition); err != nil {
		return nil, errors.Wrap(err, "failed to move entity")
	}

	// Update room data in repository
	updatedRoomData := room.ToData()
	_, err = o.repo.Update(ctx, &encounters.UpdateInput{
		EncounterID: input.EncounterID,
		RoomData:    &updatedRoomData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update encounter")
	}

	slog.Info("Character moved",
		"encounter_id", input.EncounterID,
		"entity_id", input.EntityID,
		"from", currentPosition,
		"to", input.TargetPosition,
		"distance_feet", movementInFeet,
	)

	return &MoveCharacterOutput{
		Success:      true,
		MovementUsed: movementInFeet,
		MovementLeft: maxMovement - movementInFeet,
		NewPosition:  input.TargetPosition,
		CurrentRound: tracker.Round(),
		RoomData:     &updatedRoomData,
	}, nil
}
