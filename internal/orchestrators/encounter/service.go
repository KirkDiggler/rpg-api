package encounter

import (
	"context"
)

//go:generate mockgen -destination=mock/mock_service.go -package=encountermock github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter Service

// Service defines the encounter orchestrator interface
type Service interface {
	// ResolveAttack handles a combat attack action
	ResolveAttack(ctx context.Context, input *ResolveAttackInput) (*ResolveAttackOutput, error)

	// CreateDungeon starts a new dungeon encounter
	CreateDungeon(ctx context.Context, input *CreateDungeonInput) (*CreateDungeonOutput, error)

	// MoveCharacter handles character movement in the encounter
	MoveCharacter(ctx context.Context, input *MoveCharacterInput) (*MoveCharacterOutput, error)

	// EndTurn advances combat to the next entity's turn
	EndTurn(ctx context.Context, input *EndTurnInput) (*EndTurnOutput, error)

	// ActivateFeature activates a combat feature (e.g., Rage)
	ActivateFeature(ctx context.Context, input *ActivateFeatureInput) (*ActivateFeatureOutput, error)
}

// ResolveAttackInput contains attack parameters
type ResolveAttackInput struct {
	EncounterID string
	AttackerID  string
	TargetID    string
	WeaponID    string // Optional, uses default weapon if empty
}

// ResolveAttackOutput returns attack results
// TODO: Replace AttackResult with toolkit combat.AttackResult when combat package is published
type ResolveAttackOutput struct {
	Result      *AttackResult // Attack result with full breakdown
	MonsterHP   int           // Updated monster HP
	MonsterDead bool          // Whether monster was defeated
}

// AttackResult contains the outcome of an attack
// TODO: Replace with toolkit combat.AttackResult when available
// This mirrors the toolkit type structure for compatibility
type AttackResult struct {
	// Attack roll details
	AttackRoll      int  // The d20 roll
	AttackBonus     int  // Total bonus applied
	TotalAttack     int  // Roll + bonus
	TargetAC        int  // Target's armor class
	Hit             bool // Did the attack hit?
	Critical        bool // Was it a critical hit?
	IsNaturalTwenty bool // Natural 20
	IsNaturalOne    bool // Natural 1

	// Damage details
	DamageRolls []int  // Individual damage dice rolls
	DamageBonus int    // Total damage bonus
	TotalDamage int    // Final damage dealt
	DamageType  string // Type of damage (slashing, piercing, etc.)

	// Detailed breakdown
	Breakdown *DamageBreakdown // Detailed damage breakdown (nil if attack missed)
}

// DamageBreakdown provides detailed component breakdown of damage calculation
type DamageBreakdown struct {
	Components  []DamageComponent
	AbilityUsed string // Ability used for attack ("STR", "DEX", etc.)
	TotalDamage int    // Sum of all components
}

// DamageComponent represents damage from one source
type DamageComponent struct {
	Source            string        // Type of damage source ("weapon", "ability", "rage", etc.)
	OriginalDiceRolls []int         // Dice values as first rolled
	FinalDiceRolls    []int         // Dice values after all rerolls
	Rerolls           []RerollEvent // History of rerolls
	FlatBonus         int           // Flat modifier (0 if none)
	DamageType        string        // "slashing", "fire", "radiant", etc.
	IsCritical        bool          // Was this component doubled for crit?
}

// RerollEvent tracks a single die reroll
type RerollEvent struct {
	DieIndex int    // Which die was rerolled (0-based index in original_dice_rolls)
	Before   int    // Value before reroll
	After    int    // Value after reroll
	Reason   string // Feature that caused reroll (e.g., "great_weapon_fighting")
}

// CreateDungeonInput contains parameters for creating a dungeon encounter
type CreateDungeonInput struct {
	CharacterIDs []string // IDs of characters entering the dungeon
}

// CreateDungeonOutput returns the created encounter details
type CreateDungeonOutput struct {
	EncounterID string       // ID of the created encounter
	Room        interface{}  // Room data (using interface{} to match spatial.RoomData)
	CombatState *CombatState // Combat state with initiative order
}

// CombatState represents the state of combat in an encounter
type CombatState struct {
	EncounterID       string
	Round             int
	TurnOrder         []InitiativeEntry
	ActiveIndex       int
	MovementRemaining int32 // Movement remaining for the active turn
	CombatStarted     bool
	CombatEnded       bool
}

// InitiativeEntry represents one entity in the initiative order
type InitiativeEntry struct {
	EntityID           string
	EntityType         string
	InitiativeRoll     int       // The d20 roll
	InitiativeModifier int       // DEX modifier
	InitiativeTotal    int       // Roll + Modifier
	Position           *Position // Entity's position in the room
}

// MoveCharacterInput contains movement parameters
// Phase 2: Simple movement to a single target position
type MoveCharacterInput struct {
	EncounterID    string    // ID of the encounter
	EntityID       string    // ID of entity being moved
	TargetPosition *Position // Target position to move to
}

// Position represents a 2D position in the room
// This mirrors spatial.Position for handler layer use
type Position struct {
	X float64
	Y float64
}

// MoveCharacterOutput returns movement results
type MoveCharacterOutput struct {
	Success           bool      // Whether the movement succeeded
	FinalPosition     *Position // Final position of the entity
	MovementRemaining int32     // Movement points remaining (Phase 3)
	// Why movement stopped: "completed", "position_occupied", "out_of_bounds", "entity_not_found"
	StopReason  string
	UpdatedRoom interface{} // Updated room data (using interface{} until spatial is fixed)
}

// EndTurnInput contains parameters for ending a turn
type EndTurnInput struct {
	EncounterID string // ID of the encounter
	// EntityID is no longer required - the server determines whose turn it is from encounter state

	// PlayerID is the authenticated player attempting to end the turn.
	// The server validates that this player owns the character whose turn it is.
	// If empty, ownership validation is skipped (for backward compatibility/testing).
	PlayerID string
}

// EndTurnOutput returns the result of ending a turn
type EndTurnOutput struct {
	CombatState *CombatState     // Updated combat state with new active turn
	TurnChange  *TurnChangeEvent // Details about the turn transition
}

// TurnChangeEvent describes a turn transition
type TurnChangeEvent struct {
	PreviousEntityID string // Entity that ended their turn
	NextEntityID     string // Entity whose turn is starting
	Round            int    // Current round number
	NewRound         bool   // True if this starts a new round
}

// ActivateFeatureInput contains parameters for activating a combat feature
type ActivateFeatureInput struct {
	EncounterID string // ID of the encounter (for context/validation)
	CharacterID string // ID of the character activating the feature
	FeatureID   string // ID of the feature to activate (e.g., "rage")
}

// ActivateFeatureOutput returns the result of feature activation
type ActivateFeatureOutput struct {
	Success       bool   // Whether activation succeeded
	Message       string // Human-readable result message
	CharacterData interface{}
}
