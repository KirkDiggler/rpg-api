package encounter

import (
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// DungeonStartInput defines the request for starting a dungeon encounter
type DungeonStartInput struct {
	CharacterIDs []string
}

// DungeonStartOutput defines the response for starting a dungeon encounter
type DungeonStartOutput struct {
	EncounterID    string
	RoomData       *spatial.RoomData
	InitiativeData *initiative.TrackerData // Turn order for the encounter
	CurrentTurn    string                  // ID of whose turn it is
}

// Note: All spatial types (Position, EntityPlacement, RoomData) are now provided
// by the github.com/KirkDiggler/rpg-toolkit/tools/spatial package

// NextTurnInput defines the request for advancing to the next turn
type NextTurnInput struct {
	EncounterID string
}

// NextTurnOutput defines the response for advancing to the next turn
type NextTurnOutput struct {
	CurrentTurn string // ID of whose turn it is now
	Round       int    // Current round number
}

// GetTurnOrderInput defines the request for getting current turn order
type GetTurnOrderInput struct {
	EncounterID string
}

// GetTurnOrderOutput defines the response for getting current turn order
type GetTurnOrderOutput struct {
	InitiativeData *initiative.TrackerData
	CurrentTurn    string // ID of whose turn it is
}
