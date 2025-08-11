package encounter

import (
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// DungeonStartInput defines the request for starting a dungeon encounter
type DungeonStartInput struct {
	CharacterIDs       []string
	MonsterDexModifier *int          // Optional DEX modifier for the demo monster
	RoomTemplate       *RoomTemplate // Optional room template (defaults to random)
}

// DungeonStartOutput defines the response for starting a dungeon encounter
type DungeonStartOutput struct {
	EncounterID     string
	RoomData        *spatial.RoomData
	InitiativeData  *initiative.TrackerData // Turn order for the encounter
	InitiativeRolls []initiative.Roll       // Details of what was rolled
	CurrentTurn     string                  // ID of whose turn it is
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
	InitiativeData  *initiative.TrackerData
	InitiativeRolls []initiative.Roll // Details of what was rolled
	CurrentTurn     string            // ID of whose turn it is
}

// MoveCharacterInput defines the request for moving a character
type MoveCharacterInput struct {
	EncounterID    string
	EntityID       string
	TargetPosition spatial.Position
}

// MoveCharacterOutput defines the response for moving a character
type MoveCharacterOutput struct {
	Success      bool
	MovementUsed int // How much movement was used
	MovementLeft int // How much movement remains
	NewPosition  spatial.Position
	CurrentRound int
	RoomData     *spatial.RoomData // Updated room state after movement
}

// AttackInput defines the request for making an attack
type AttackInput struct {
	EncounterID string
	AttackerID  string
	TargetID    string
	WeaponID    string // Optional, uses default weapon if empty
	AttackType  string // "melee", "ranged", "spell"
}

// AttackOutput defines the response for an attack
type AttackOutput struct {
	Success       bool
	Hit           bool
	Critical      bool
	Damage        int
	DamageType    string
	AttackRoll    int // The d20 roll
	AttackTotal   int // Roll + modifiers
	TargetAC      int
	TargetNewHP   int // Target's HP after damage
	TargetMaxHP   int
	Description   string // Narrative description
	RoomData      *spatial.RoomData // Updated room if target dies
}
