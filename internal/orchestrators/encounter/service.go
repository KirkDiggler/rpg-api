package encounter

import (
	"context"
	"errors"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/damage"
)

// Type aliases to entities - these are the canonical types
type (
	CombatState     = entities.CombatState
	InitiativeEntry = entities.InitiativeEntry
	Position        = entities.Position
)

// Sentinel errors for encounter operations
var (
	// ErrEncounterNotFound is returned when an encounter cannot be found
	ErrEncounterNotFound = errors.New("encounter not found")

	// ErrPlayerNotInEncounter is returned when a player is not part of the encounter
	ErrPlayerNotInEncounter = errors.New("player not in encounter")

	// ErrPlayerAlreadyInEncounter is returned when a player tries to join an encounter they're already in
	ErrPlayerAlreadyInEncounter = errors.New("player already in encounter")

	// ErrCombatAlreadyStarted is returned when an action requires waiting state but combat has started
	ErrCombatAlreadyStarted = errors.New("combat already started")

	// ErrNotHost is returned when a non-host player tries to perform a host-only action
	ErrNotHost = errors.New("only the host can perform this action")

	// ErrPlayersNotReady is returned when trying to start combat but not all players are ready
	ErrPlayersNotReady = errors.New("not all players are ready")

	// ErrPlayerAlreadyDisconnected is returned when marking a player as disconnected who is already disconnected
	ErrPlayerAlreadyDisconnected = errors.New("player is already disconnected")

	// ErrPlayerAlreadyConnected is returned when reconnecting a player who is already connected
	ErrPlayerAlreadyConnected = errors.New("player is already connected")

	// ErrEncounterPaused is returned when trying to perform an action on a paused encounter
	ErrEncounterPaused = errors.New("encounter is paused")
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

	// Multiplayer lobby methods

	// CreateEncounter creates a new multiplayer encounter lobby
	CreateEncounter(ctx context.Context, input *CreateEncounterInput) (*CreateEncounterOutput, error)

	// JoinEncounter joins an existing encounter via join code
	JoinEncounter(ctx context.Context, input *JoinEncounterInput) (*JoinEncounterOutput, error)

	// SetReady marks a player as ready or not ready to start combat
	SetReady(ctx context.Context, input *SetReadyInput) (*SetReadyOutput, error)

	// StartCombat begins combat (host only, all players must be ready)
	StartCombat(ctx context.Context, input *StartCombatInput) (*StartCombatOutput, error)

	// LeaveEncounter removes a player from the encounter
	LeaveEncounter(ctx context.Context, input *LeaveEncounterInput) (*LeaveEncounterOutput, error)

	// Connection management methods

	// PlayerDisconnected marks a player as disconnected
	// If combat is active and all remaining players are disconnected, pauses the encounter
	PlayerDisconnected(ctx context.Context, input *PlayerDisconnectedInput) (*PlayerDisconnectedOutput, error)

	// PlayerReconnected marks a player as reconnected
	// If encounter was paused due to disconnection, resumes when a player reconnects
	PlayerReconnected(ctx context.Context, input *PlayerReconnectedInput) (*PlayerReconnectedOutput, error)
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
	DamageRolls []int       // Individual damage dice rolls
	DamageBonus int         // Total damage bonus
	TotalDamage int         // Final damage dealt
	DamageType  damage.Type // Type of damage (slashing, piercing, etc.)

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
	Source            string        // Type of damage source ("weapon", "ability", "feature", etc.)
	SourceRef         *core.Ref     // Type-safe reference identifying the specific source
	OriginalDiceRolls []int         // Dice values as first rolled
	FinalDiceRolls    []int         // Dice values after all rerolls
	Rerolls           []RerollEvent // History of rerolls
	FlatBonus         int           // Flat modifier (0 if none)
	DamageType        damage.Type   // Type of damage (slashing, fire, radiant, etc.)
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
	EncounterID  string               // ID of the created encounter
	Room         interface{}          // Room data (using interface{} to match spatial.RoomData)
	CombatState  *CombatState         // Combat state with initiative order
	MonsterTurns []*MonsterTurnResult // Monster turns if monsters go first in initiative
}

// MoveCharacterInput contains movement parameters
// Phase 2: Simple movement to a single target position
type MoveCharacterInput struct {
	EncounterID    string    // ID of the encounter
	EntityID       string    // ID of entity being moved
	TargetPosition *Position // Target position to move to
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
	CombatState     *CombatState         // Updated combat state with new active turn
	TurnChange      *TurnChangeEvent     // Details about the turn transition
	MonsterTurns    []*MonsterTurnResult // Monster turns executed after player ended turn
	EncounterResult *EncounterResult     // Set if combat ended (victory or TPK)
}

// EncounterResult indicates combat has ended
type EncounterResult struct {
	Reason string // "victory" (all monsters dead) or "defeat" (all players down)
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

// MonsterTurnResult represents the outcome of a monster's turn
// This mirrors toolkit monster.TurnResult for handler layer use
type MonsterTurnResult struct {
	MonsterID   string                  // ID of the monster that took the turn
	MonsterName string                  // Name for self-contained streaming
	Actions     []MonsterExecutedAction // All actions taken this turn
	Movement    []Position              // Path traversed during movement
}

// MonsterExecutedAction represents a single action taken by a monster
// This mirrors toolkit monster.ExecutedAction for handler layer use
type MonsterExecutedAction struct {
	ActionID   string      // ID of the action used
	ActionType string      // Type of action (melee_attack, heal, etc.) - matches monster.ActionType
	TargetID   string      // ID of the target (if applicable)
	Success    bool        // Whether the action succeeded
	Details    interface{} // Action-specific details (AttackResult, heal amount, etc.)
}

// Multiplayer lobby types

// CreateEncounterInput contains parameters for creating a multiplayer encounter
type CreateEncounterInput struct {
	PlayerID     string   // ID of the player creating the encounter (becomes host)
	CharacterIDs []string // Character IDs to add to the encounter
}

// CreateEncounterOutput returns the created encounter details
type CreateEncounterOutput struct {
	EncounterID string      // ID of the created encounter
	JoinCode    string      // 6-char code for others to join
	Room        interface{} // Generated room data
}

// PartyMember represents a player and their character in the encounter
type PartyMember struct {
	PlayerID      string
	CharacterID   string
	CharacterData interface{} // Character data for the handler to convert
	IsHost        bool
	IsReady       bool
	IsConnected   bool
}

// JoinEncounterInput contains parameters for joining an encounter
type JoinEncounterInput struct {
	JoinCode     string   // 6-char join code
	PlayerID     string   // ID of the player joining
	CharacterIDs []string // Character IDs to add to the encounter
}

// JoinEncounterOutput returns the encounter state after joining
type JoinEncounterOutput struct {
	EncounterID string         // ID of the joined encounter
	Room        interface{}    // Room data
	Party       []*PartyMember // All players in the encounter
	State       string         // Current state (waiting, active, etc.)
}

// SetReadyInput contains parameters for setting ready status
type SetReadyInput struct {
	EncounterID string // ID of the encounter
	PlayerID    string // ID of the player
	IsReady     bool   // Ready status to set
}

// SetReadyOutput confirms the ready status change
type SetReadyOutput struct {
	Success bool
}

// StartCombatInput contains parameters for starting combat
type StartCombatInput struct {
	EncounterID string // ID of the encounter
	PlayerID    string // ID of the player requesting start (must be host)
}

// StartCombatOutput returns the initial combat state
type StartCombatOutput struct {
	CombatState  *CombatState         // Combat state with initiative order
	MonsterTurns []*MonsterTurnResult // Monster turns if monsters go first
}

// LeaveEncounterInput contains parameters for leaving an encounter
type LeaveEncounterInput struct {
	EncounterID string // ID of the encounter
	PlayerID    string // ID of the player leaving
}

// LeaveEncounterOutput confirms the player left
type LeaveEncounterOutput struct {
	Success          bool // Whether the player successfully left
	EncounterDeleted bool // Whether the encounter was deleted (last player left)
}

// PlayerDisconnectedInput contains parameters for marking a player as disconnected
type PlayerDisconnectedInput struct {
	EncounterID string // ID of the encounter
	PlayerID    string // ID of the player who disconnected
	Reason      string // Reason for disconnection ("timeout", "network_error", "client_closed", etc.)
}

// PlayerDisconnectedOutput returns the result of marking a player as disconnected
type PlayerDisconnectedOutput struct {
	Success         bool   // Whether the operation succeeded
	EncounterPaused bool   // Whether the encounter was paused due to this disconnection
	State           string // Current encounter state after disconnection
}

// PlayerReconnectedInput contains parameters for marking a player as reconnected
type PlayerReconnectedInput struct {
	EncounterID string // ID of the encounter
	PlayerID    string // ID of the player who reconnected
}

// PlayerReconnectedOutput returns the result of marking a player as reconnected
type PlayerReconnectedOutput struct {
	Success          bool         // Whether the operation succeeded
	EncounterResumed bool         // Whether the encounter was resumed due to this reconnection
	State            string       // Current encounter state after reconnection
	CombatState      *CombatState // Combat state if encounter was resumed (nil if not in combat)
}
