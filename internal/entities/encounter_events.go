// Package entities defines the core data structures for the RPG API
package entities

import (
	"time"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/damage"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// EventType represents different types of encounter events
type EventType string

const (
	// Player lifecycle events
	EventTypePlayerJoined       EventType = "player_joined"
	EventTypePlayerLeft         EventType = "player_left"
	EventTypePlayerReady        EventType = "player_ready"
	EventTypePlayerDisconnected EventType = "player_disconnected"
	EventTypePlayerReconnected  EventType = "player_reconnected"

	// Combat lifecycle events
	EventTypeCombatStarted EventType = "combat_started"
	EventTypeCombatEnded   EventType = "combat_ended"
	EventTypeCombatPaused  EventType = "combat_paused"
	EventTypeCombatResumed EventType = "combat_resumed"

	// Combat action events
	EventTypeMovementCompleted    EventType = "movement_completed"
	EventTypeAttackResolved       EventType = "attack_resolved"
	EventTypeFeatureActivated     EventType = "feature_activated"
	EventTypeTurnEnded            EventType = "turn_ended"
	EventTypeMonsterTurnCompleted EventType = "monster_turn_completed"

	// Dungeon lifecycle events
	EventTypeDungeonVictory EventType = "dungeon_victory"
	EventTypeDungeonFailure EventType = "dungeon_failure"
	EventTypeRoomRevealed   EventType = "room_revealed"
)

// EncounterEvent wraps all event types with common metadata
// Purpose: Provides a consistent envelope for all encounter events with timestamp and type
// Only one of the event fields will be populated based on Type (oneof pattern)
type EncounterEvent struct {
	ID          string    `json:"id"`           // Unique event ID
	Type        EventType `json:"type"`         // Type of event
	EncounterID string    `json:"encounter_id"` // ID of the encounter this event belongs to
	Timestamp   time.Time `json:"timestamp"`    // When the event occurred

	// Event-specific data - only one populated based on Type (oneof pattern)
	PlayerJoined         *PlayerJoinedEvent         `json:"player_joined,omitempty"`
	PlayerLeft           *PlayerLeftEvent           `json:"player_left,omitempty"`
	PlayerReady          *PlayerReadyEvent          `json:"player_ready,omitempty"`
	PlayerDisconnected   *PlayerDisconnectedEvent   `json:"player_disconnected,omitempty"`
	PlayerReconnected    *PlayerReconnectedEvent    `json:"player_reconnected,omitempty"`
	CombatStarted        *CombatStartedEvent        `json:"combat_started,omitempty"`
	CombatEnded          *CombatEndedEvent          `json:"combat_ended,omitempty"`
	CombatPaused         *CombatPausedEvent         `json:"combat_paused,omitempty"`
	CombatResumed        *CombatResumedEvent        `json:"combat_resumed,omitempty"`
	MovementCompleted    *MovementCompletedEvent    `json:"movement_completed,omitempty"`
	AttackResolved       *AttackResolvedEvent       `json:"attack_resolved,omitempty"`
	FeatureActivated     *FeatureActivatedEvent     `json:"feature_activated,omitempty"`
	TurnEnded            *TurnEndedEvent            `json:"turn_ended,omitempty"`
	MonsterTurnCompleted *MonsterTurnCompletedEvent `json:"monster_turn_completed,omitempty"`
	DungeonVictory       *DungeonVictoryEvent       `json:"dungeon_victory,omitempty"`
	DungeonFailure       *DungeonFailureEvent       `json:"dungeon_failure,omitempty"`
	RoomRevealed         *RoomRevealedEvent         `json:"room_revealed,omitempty"`
}

// PlayerJoinedEvent is emitted when a player joins an encounter
type PlayerJoinedEvent struct {
	PlayerID      string          `json:"player_id"`
	CharacterID   string          `json:"character_id"`
	CharacterData *character.Data `json:"character_data,omitempty"`
	PlayerName    string          `json:"player_name,omitempty"`
}

// PlayerLeftEvent is emitted when a player leaves an encounter
type PlayerLeftEvent struct {
	PlayerID    string `json:"player_id"`
	CharacterID string `json:"character_id"`
	Reason      string `json:"reason,omitempty"` // "voluntary", "kicked", "timeout", etc.
}

// PlayerReadyEvent is emitted when a player marks themselves as ready
type PlayerReadyEvent struct {
	PlayerID    string `json:"player_id"`
	CharacterID string `json:"character_id"`
	Ready       bool   `json:"ready"` // true = ready, false = unready
}

// PlayerDisconnectedEvent is emitted when a player loses connection
type PlayerDisconnectedEvent struct {
	PlayerID    string `json:"player_id"`
	CharacterID string `json:"character_id"`
	Reason      string `json:"reason,omitempty"` // "timeout", "network_error", etc.
}

// PlayerReconnectedEvent is emitted when a player reconnects
type PlayerReconnectedEvent struct {
	PlayerID    string `json:"player_id"`
	CharacterID string `json:"character_id"`
}

// MonsterState represents a monster's combat state for events
type MonsterState struct {
	MonsterID        string `json:"monster_id"`         // Unique ID of this monster instance
	MonsterName      string `json:"monster_name"`       // Display name (e.g., "Skeleton")
	CurrentHitPoints int    `json:"current_hit_points"` // Current HP
	MaxHitPoints     int    `json:"max_hit_points"`     // Maximum HP
	MonsterType      string `json:"monster_type"`       // Type for UI texture (e.g., "skeleton", "goblin")
}

// CombatStartedEvent is emitted when combat begins
type CombatStartedEvent struct {
	CombatState  *CombatState                 `json:"combat_state"`            // Full combat state including initiative order
	Room         *spatial.RoomData            `json:"room"`                    // Room with entity positions
	Walls        []dungeon.WallSegment        `json:"walls,omitempty"`         // Walls in the room
	Party        []*Player                    `json:"party"`                   // Party members at combat start
	Monsters     []*MonsterState              `json:"monsters"`                // Monster state with types for UI textures
	Doors        []*DoorInfo                  `json:"doors"`                   // Doors/exits from the starting room
	DungeonID    string                       `json:"dungeon_id"`              // ID of the generated dungeon
	MonsterTurns []*MonsterTurnCompletedEvent `json:"monster_turns,omitempty"` // Monster turns if monsters won initiative
}

// EncounterResult describes the outcome of an encounter
type EncounterResult struct {
	Reason string `json:"reason"` // "victory" (all monsters dead) or "defeat" (all players down)
}

// CombatEndedEvent is emitted when combat ends
type CombatEndedEvent struct {
	EncounterResult *EncounterResult `json:"encounter_result"` // Victory or defeat
}

// CombatPausedEvent is emitted when combat is paused
type CombatPausedEvent struct {
	PausedBy string `json:"paused_by"` // Player ID who paused
	Reason   string `json:"reason,omitempty"`
}

// CombatResumedEvent is emitted when combat is resumed
type CombatResumedEvent struct {
	ResumedBy string `json:"resumed_by"` // Player ID who resumed
}

// MovementCompletedEvent is emitted when an entity completes movement
type MovementCompletedEvent struct {
	EntityID          string                `json:"entity_id"`
	EntityType        string                `json:"entity_type"` // "character" or "monster"
	FinalPosition     *Position             `json:"final_position"`
	MovementRemaining int32                 `json:"movement_remaining"`
	StopReason        string                `json:"stop_reason"` // "completed", "position_occupied", etc.
	UpdatedRoom       *spatial.RoomData     `json:"updated_room,omitempty"`
	Walls             []dungeon.WallSegment `json:"walls,omitempty"`
}

// AttackResult contains the full details of an attack resolution
type AttackResult struct {
	// Attack roll details
	AttackRoll      int  `json:"attack_roll"`       // The d20 roll
	AttackBonus     int  `json:"attack_bonus"`      // Total bonus applied
	TotalAttack     int  `json:"total_attack"`      // Roll + bonus
	TargetAC        int  `json:"target_ac"`         // Target's armor class
	Hit             bool `json:"hit"`               // Did the attack hit?
	Critical        bool `json:"critical"`          // Was it a critical hit?
	IsNaturalTwenty bool `json:"is_natural_twenty"` // Natural 20
	IsNaturalOne    bool `json:"is_natural_one"`    // Natural 1

	// Damage details
	DamageRolls []int       `json:"damage_rolls"` // Individual damage dice rolls
	DamageBonus int         `json:"damage_bonus"` // Total damage bonus
	TotalDamage int         `json:"total_damage"` // Final damage dealt
	DamageType  damage.Type `json:"damage_type"`  // Type of damage (slashing, piercing, etc.)

	// Detailed breakdown
	Breakdown *DamageBreakdown `json:"breakdown,omitempty"` // Detailed damage breakdown (nil if attack missed)
}

// HealResult contains the outcome of a healing action
type HealResult struct {
	AmountHealed int `json:"amount_healed"` // HP restored
	NewHP        int `json:"new_hp"`        // Current HP after healing
	MaxHP        int `json:"max_hp"`        // Maximum HP (for context)
}

// DamageBreakdown provides detailed component breakdown of damage calculation
type DamageBreakdown struct {
	Components  []DamageComponent `json:"components"`
	AbilityUsed string            `json:"ability_used"` // Ability used for attack ("STR", "DEX", etc.)
	TotalDamage int               `json:"total_damage"` // Sum of all components
}

// DamageComponent represents damage from one source
type DamageComponent struct {
	Source            string        `json:"source"`              // Type of damage source ("weapon", "ability", "feature", "monster_trait", etc.)
	SourceRef         *core.Ref     `json:"source_ref"`          // Type-safe reference identifying the specific source
	OriginalDiceRolls []int         `json:"original_dice_rolls"` // Dice values as first rolled
	FinalDiceRolls    []int         `json:"final_dice_rolls"`    // Dice values after all rerolls
	Rerolls           []RerollEvent `json:"rerolls,omitempty"`   // History of rerolls
	FlatBonus         int           `json:"flat_bonus"`          // Flat modifier (0 if none)
	DamageType        damage.Type   `json:"damage_type"`         // Type of damage (slashing, fire, radiant, etc.)
	IsCritical        bool          `json:"is_critical"`         // Was this component doubled for crit?
	Multiplier        float64       `json:"multiplier"`          // Multiplier for vulnerability (2.0), resistance (0.5), or immunity (0)
}

// RerollEvent tracks a single die reroll
type RerollEvent struct {
	DieIndex int    `json:"die_index"` // Which die was rerolled (0-based index in original_dice_rolls)
	Before   int    `json:"before"`    // Value before reroll
	After    int    `json:"after"`     // Value after reroll
	Reason   string `json:"reason"`    // Feature that caused reroll (e.g., "great_weapon_fighting")
}

// AttackResolvedEvent is emitted when an attack is resolved
type AttackResolvedEvent struct {
	AttackerID    string                `json:"attacker_id"`
	TargetID      string                `json:"target_id"`
	Result        *AttackResult         `json:"result"`
	TargetHP      int                   `json:"target_hp"`                // HP after attack
	TargetDead    bool                  `json:"target_dead"`              // Whether target was killed
	Room          *spatial.RoomData     `json:"room,omitempty"`           // Updated room with entity positions
	Walls         []dungeon.WallSegment `json:"walls,omitempty"`          // Walls in the room
	GrantedAction *GrantedActionInfo    `json:"granted_action,omitempty"` // Action granted from this attack
}

// GrantedActionInfo represents info about an action granted during combat
type GrantedActionInfo struct {
	ID       string `json:"id"`
	Type     string `json:"type"`   // e.g., "off-hand-strike"
	Name     string `json:"name"`   // Display name
	Reason   string `json:"reason"` // Why this action was granted
	WeaponID string `json:"weapon_id,omitempty"`
}

// FeatureActivatedEvent is emitted when a combat feature is activated
type FeatureActivatedEvent struct {
	CharacterID   string          `json:"character_id"`
	FeatureID     string          `json:"feature_id"`
	Success       bool            `json:"success"`
	Message       string          `json:"message"`
	CharacterData *character.Data `json:"character_data,omitempty"`
}

// TurnEndedEvent is emitted when a turn ends
type TurnEndedEvent struct {
	PreviousEntityID string                `json:"previous_entity_id"`
	NextEntityID     string                `json:"next_entity_id"`
	Round            int                   `json:"round"`
	NewRound         bool                  `json:"new_round"`
	CombatState      *CombatState          `json:"combat_state"`   // Full updated combat state
	Room             *spatial.RoomData     `json:"room,omitempty"` // Updated room with entity positions
	Walls            []dungeon.WallSegment `json:"walls,omitempty"`
}

// MonsterTurnCompletedEvent is emitted when a monster completes its turn
type MonsterTurnCompletedEvent struct {
	MonsterID         string                  `json:"monster_id"`
	MonsterName       string                  `json:"monster_name"`
	Actions           []MonsterExecutedAction `json:"actions"`
	Movement          []Position              `json:"movement"`
	Room              *spatial.RoomData       `json:"room,omitempty"`               // Updated room with entity positions
	Walls             []dungeon.WallSegment   `json:"walls,omitempty"`              // Walls in the current room
	UpdatedCharacters []*character.Data       `json:"updated_characters,omitempty"` // Characters that took damage
}

// DungeonVictoryEvent is emitted when the boss is defeated
// Note: Combat continues after victory - players can keep exploring
type DungeonVictoryEvent struct {
	DungeonID      string `json:"dungeon_id"`
	BossID         string `json:"boss_id"`         // ID of the defeated boss monster
	BossName       string `json:"boss_name"`       // Name of the boss for display
	MonstersKilled int    `json:"monsters_killed"` // Total monsters killed in the dungeon
	RoomsExplored  int    `json:"rooms_explored"`  // Number of rooms explored
}

// DungeonFailureEvent is emitted when all party members are defeated (TPK)
type DungeonFailureEvent struct {
	DungeonID string `json:"dungeon_id"`
	Reason    string `json:"reason"` // "tpk" (total party kill), "abandoned", etc.
}

// RoomRevealedEvent is emitted when a door is opened and a new room is revealed
type RoomRevealedEvent struct {
	DungeonID    string                       `json:"dungeon_id"`              // ID of the dungeon
	ConnectionID string                       `json:"connection_id"`           // ID of the opened door/connection
	RevealedRoom *spatial.RoomData            `json:"revealed_room"`           // The newly revealed room
	RoomOrigin   *dungeon.Position            `json:"room_origin,omitempty"`   // Absolute position of the revealed room in dungeon-space
	Walls        []dungeon.WallSegment        `json:"walls,omitempty"`         // Walls in the revealed room
	NewDoors     []*DoorInfo                  `json:"new_doors"`               // Doors visible from the newly revealed room
	Monsters     []*MonsterState              `json:"monsters"`                // Monsters in the revealed room
	CombatState  *CombatState                 `json:"combat_state"`            // Updated combat state with new monsters in initiative
	MonsterTurns []*MonsterTurnCompletedEvent `json:"monster_turns,omitempty"` // Monster turns if new monsters act immediately
}
