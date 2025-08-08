// Package encounter implements the encounter orchestrator for managing D&D 5e encounters
package encounter

//go:generate mockgen -destination=mock/mock_service.go -package=encountermock github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter Service

import (
	"context"
	"log/slog"
	"sync"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-toolkit/core"
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
}

// Config holds the dependencies for the encounter orchestrator
type Config struct {
	IDGenerator idgen.Generator
}

// Validate ensures all required dependencies are provided
func (c *Config) Validate() error {
	vb := errors.NewValidationBuilder()

	if c.IDGenerator == nil {
		vb.RequiredField("IDGenerator")
	}

	return vb.Build()
}

type orchestrator struct {
	idGen idgen.Generator

	// In-memory storage for demo - would be in repository in production
	mu         sync.RWMutex
	encounters map[string]*encounterState
}

// encounterState holds the state of an active encounter
type encounterState struct {
	room    *spatial.BasicRoom
	tracker *initiative.Tracker
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
		idGen:      cfg.IDGenerator,
		encounters: make(map[string]*encounterState),
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

	// Add characters with default DEX modifier (for demo, using 0)
	for _, characterID := range input.CharacterIDs {
		charEntity := &simpleEntity{
			id:         characterID,
			entityType: "character",
		}
		entities[charEntity] = 0 // TODO: Get actual DEX modifier from character service
	}

	// Add the monster
	entities[monsterEntity] = 2 // Give monster a +2 DEX modifier for demo

	// Roll initiative and create tracker
	order := initiative.RollForOrder(entities, nil) // nil uses default dice roller
	tracker := initiative.New(order)

	// Store encounter state (in-memory for demo)
	o.mu.Lock()
	o.encounters[encounterID] = &encounterState{
		room:    room,
		tracker: tracker,
	}
	o.mu.Unlock()

	// Get current turn
	current := tracker.Current()
	currentTurn := ""
	if current != nil {
		currentTurn = current.GetID()
	}

	// Convert to response data
	roomData := room.ToData()
	trackerData := tracker.ToData()

	return &DungeonStartOutput{
		EncounterID:    encounterID,
		RoomData:       &roomData,
		InitiativeData: &trackerData,
		CurrentTurn:    currentTurn,
	}, nil
}

// NextTurn advances to the next turn in the encounter
func (o *orchestrator) NextTurn(ctx context.Context, input *NextTurnInput) (*NextTurnOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	// Get encounter state
	o.mu.RLock()
	state, exists := o.encounters[input.EncounterID]
	o.mu.RUnlock()

	if !exists {
		return nil, errors.NotFound("encounter not found")
	}

	// Advance turn
	next := state.tracker.Next()
	currentTurn := ""
	if next != nil {
		currentTurn = next.GetID()
	}

	slog.Info("Advanced turn",
		"encounter_id", input.EncounterID,
		"current_turn", currentTurn,
		"round", state.tracker.Round(),
	)

	return &NextTurnOutput{
		CurrentTurn: currentTurn,
		Round:       state.tracker.Round(),
	}, nil
}

// GetTurnOrder returns the current turn order
func (o *orchestrator) GetTurnOrder(ctx context.Context, input *GetTurnOrderInput) (*GetTurnOrderOutput, error) {
	if input == nil {
		return nil, errors.InvalidArgument("input is required")
	}

	// Get encounter state
	o.mu.RLock()
	state, exists := o.encounters[input.EncounterID]
	o.mu.RUnlock()

	if !exists {
		return nil, errors.NotFound("encounter not found")
	}

	// Get current state
	current := state.tracker.Current()
	currentTurn := ""
	if current != nil {
		currentTurn = current.GetID()
	}

	trackerData := state.tracker.ToData()

	return &GetTurnOrderOutput{
		InitiativeData: &trackerData,
		CurrentTurn:    currentTurn,
	}, nil
}
