package encounter

import (
	"context"
	"errors"

	pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
)

// Type aliases to entities - these are the canonical types
type (
	CombatState           = entities.CombatState
	InitiativeEntry       = entities.InitiativeEntry
	Position              = entities.Position
	MonsterExecutedAction = entities.MonsterExecutedAction
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

	// OpenDoor opens a door to reveal the connected room and adds its monsters to combat
	OpenDoor(ctx context.Context, input *OpenDoorInput) (*OpenDoorOutput, error)

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

	// State retrieval for load-then-stream pattern

	// GetEncounterState returns a full snapshot of the encounter state
	// Used by clients to sync state before processing streamed events
	GetEncounterState(ctx context.Context, input *GetEncounterStateInput) (*GetEncounterStateOutput, error)

	// GetEncounterHistory retrieves historical events for an encounter
	// Used by late joiners to populate event log before streaming new events
	GetEncounterHistory(ctx context.Context, input *GetEncounterHistoryInput) (*GetEncounterHistoryOutput, error)
}

// ResolveAttackInput contains attack parameters
type ResolveAttackInput struct {
	EncounterID string
	AttackerID  string
	TargetID    string
	WeaponID    string            // Optional, uses default weapon if empty
	AttackHand  combat.AttackHand // Which hand is attacking (main/off for TWF)
}

// ResolveAttackOutput returns attack results
// TODO: Replace AttackResult with toolkit combat.AttackResult when combat package is published
type ResolveAttackOutput struct {
	Result        *AttackResult  // Attack result with full breakdown
	MonsterHP     int            // Updated monster HP
	MonsterDead   bool           // Whether monster was defeated
	GrantedAction *GrantedAction // Action granted from this attack (e.g., OffHandStrike for TWF)
}

// GrantedAction represents an action granted during combat (e.g., OffHandStrike from two-weapon fighting)
type GrantedAction struct {
	ID       string // Unique action ID
	Type     string // Action type (e.g., "off-hand-strike")
	Name     string // Display name
	Reason   string // Why this action was granted (e.g., "dual-wielding light weapons")
	WeaponID string // Associated weapon (if applicable)
}

// AttackResult is an alias to entities.AttackResult for backwards compatibility
// The canonical type is in entities to avoid circular imports and enable proper JSON serialization
type AttackResult = entities.AttackResult

// DamageBreakdown is an alias to entities.DamageBreakdown
type DamageBreakdown = entities.DamageBreakdown

// DamageComponent is an alias to entities.DamageComponent
type DamageComponent = entities.DamageComponent

// RerollEvent is an alias to entities.RerollEvent
type RerollEvent = entities.RerollEvent

// CreateDungeonInput contains parameters for creating a dungeon encounter
type CreateDungeonInput struct {
	CharacterIDs []string // IDs of characters entering the dungeon
	ThemeID      string   // Theme identifier (crypt, cave, bandit-lair) - defaults to crypt
	Difficulty   string   // Difficulty level (easy, medium, hard, deadly) - defaults to easy
	Length       string   // Dungeon length (short, medium, long) - defaults to medium
	PartyLevel   int      // Average party level for CR calculation - defaults to 1
	Seed         int64    // Optional seed for reproducibility (0 = random)
}

// DoorInfo describes a door or passage connecting the current room to another room.
// Each door corresponds to a connection in the dungeon graph, allowing players
// to navigate between rooms during exploration.
type DoorInfo struct {
	ConnectionID string    // Unique ID of this connection, used to reference it when opening/traversing
	TargetRoomID string    // ID of the room this door leads to
	Direction    string    // Physical description of the exit (e.g., "north door", "stairs down", "eastern passage")
	Position     *Position // Position of the door within the current room (cube coordinates)
	IsOpen       bool      // Whether the door has been opened (affects visibility and traversal)
}

// CreateDungeonOutput returns the created encounter details
type CreateDungeonOutput struct {
	EncounterID  string               // ID of the created encounter
	DungeonID    string               // ID of the generated dungeon
	Room         interface{}          // Room data (using interface{} to match spatial.RoomData)
	Doors        []DoorInfo           // Doors/exits from the starting room
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
	MonsterID         string                  // ID of the monster that took the turn
	MonsterName       string                  // Name for self-contained streaming
	Actions           []MonsterExecutedAction // All actions taken this turn
	Movement          []Position              // Path traversed during movement
	UpdatedCharacters []*toolkitchar.Data     // Characters that took damage (with updated HP)
}

// MonsterExecutedAction is aliased from entities at the top of this file

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

	// Dungeon generation options (all optional with sensible defaults)
	ThemeID    string // Theme identifier (crypt, cave, bandit-lair) - defaults to crypt
	Difficulty string // Difficulty level (easy, medium, hard, deadly) - defaults to easy
	Length     string // Dungeon length (short, medium, long) - defaults to short
	PartyLevel int    // Average party level for CR calculation - defaults to 1
	Seed       int64  // Optional seed for reproducibility (0 = random)
}

// StartCombatOutput returns the initial combat state
type StartCombatOutput struct {
	CombatState  *CombatState         // Combat state with initiative order
	Room         interface{}          // Room with entity positions
	MonsterTurns []*MonsterTurnResult // Monster turns if monsters go first
	Doors        []DoorInfo           // Doors/exits from the starting room
	DungeonID    string               // ID of the generated dungeon
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

// OpenDoorInput contains parameters for opening a door
type OpenDoorInput struct {
	DungeonID    string // ID of the dungeon
	ConnectionID string // ID of the door/connection to open
}

// OpenDoorOutput returns the result of opening a door
type OpenDoorOutput struct {
	RevealedRoom *RoomData            // The newly revealed room with entities
	RoomOffset   *Position            // Offset to apply to revealed room positions for grid merge
	NewDoors     []DoorInfo           // Doors visible from the newly revealed room
	Monsters     []MonsterInfo        // Monsters in the revealed room with initiative
	CombatState  *CombatState         // Updated combat state with monsters inserted
	MonsterTurns []*MonsterTurnResult // Monster turns if any monsters act before current entity
}

// RoomData represents a room for the OpenDoor response
// This wraps spatial.RoomData for the service layer
type RoomData struct {
	ID       string                 // Room ID
	Width    int                    // Room width in cells
	Height   int                    // Room height in cells
	Entities map[string]interface{} // Entity placements
}

// MonsterInfo contains information about a monster in a revealed room
type MonsterInfo struct {
	ID         string    // Monster instance ID
	MonsterID  string    // Monster type ID (e.g., "skeleton", "goblin")
	Name       string    // Monster display name
	Position   *Position // Position in the room (with offset applied)
	HP         int       // Current hit points
	MaxHP      int       // Maximum hit points
	Initiative int       // Initiative roll total
}

// GetEncounterStateInput contains parameters for retrieving encounter state snapshot
type GetEncounterStateInput struct {
	EncounterID string // ID of the encounter
	PlayerID    string // ID of the player requesting the state
}

// MonsterCombatState contains monster HP information for rendering
// Entity positions are in Room, this provides combat stats
type MonsterCombatState struct {
	MonsterID        string         // Unique ID of this monster instance
	MonsterName      string         // Display name (e.g., "Goblin")
	CurrentHitPoints int            // Current HP
	MaxHitPoints     int            // Maximum HP
	MonsterType      pb.MonsterType // Type for UI texture selection
}

// GetEncounterStateOutput returns a full snapshot of the encounter
// Includes everything needed to render the current state without events
type GetEncounterStateOutput struct {
	// Encounter metadata
	EncounterID string // ID of the encounter
	State       string // Current state: "waiting", "active", "paused", "completed"

	// Lobby state (populated in all states)
	Party    []*PartyMember // All players and their characters
	JoinCode string         // 6-char join code
	HostID   string         // Player ID of the host

	// Combat state (populated when state is "active" or "paused")
	CombatState *CombatState          // Initiative order, current turn, etc.
	Room        interface{}           // Room data with entity positions
	Monsters    []*MonsterCombatState // Monster HP for rendering
	Doors       []DoorInfo            // Doors/exits from the current room
	DungeonID   string                // ID of the generated dungeon

	// Event synchronization
	// ULID of the most recent event - clients filter events where id > lastEventID
	LastEventID string
}

// GetEncounterHistoryInput contains parameters for retrieving encounter history
type GetEncounterHistoryInput struct {
	EncounterID string // ID of the encounter
	UpToEventID string // Get events up to this ID (from GetEncounterState.LastEventID; empty = all)
	Limit       int    // Max events to return (0 = no limit)
}

// GetEncounterHistoryOutput returns historical encounter events
type GetEncounterHistoryOutput struct {
	Events      []*entities.EncounterEvent // Events in chronological order
	HasMore     bool                       // True if more events exist beyond the limit
	LastEventID string                     // ID of the last event returned (for pagination)
}
