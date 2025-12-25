// Package entities defines the core data structures for the RPG API
package entities

import (
	"time"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
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
	CombatState *CombatState      `json:"combat_state"` // Full combat state including initiative order
	Room        *spatial.RoomData `json:"room"`         // Room with entity positions
	Party       []*Player         `json:"party"`        // Party members at combat start
	Monsters    []*MonsterState   `json:"monsters"`     // Monster state with types for UI textures
}

// CombatEndedEvent is emitted when combat ends
type CombatEndedEvent struct {
	EncounterResult interface{} `json:"encounter_result"` // Victory or defeat
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
	EntityID          string            `json:"entity_id"`
	EntityType        string            `json:"entity_type"` // "character" or "monster"
	FinalPosition     *Position         `json:"final_position"`
	MovementRemaining int32             `json:"movement_remaining"`
	StopReason        string            `json:"stop_reason"` // "completed", "position_occupied", etc.
	UpdatedRoom       *spatial.RoomData `json:"updated_room,omitempty"`
}

// AttackResolvedEvent is emitted when an attack is resolved
type AttackResolvedEvent struct {
	AttackerID string            `json:"attacker_id"`
	TargetID   string            `json:"target_id"`
	Result     interface{}       `json:"result"`
	TargetHP   int               `json:"target_hp"`      // HP after attack
	TargetDead bool              `json:"target_dead"`    // Whether target was killed
	Room       *spatial.RoomData `json:"room,omitempty"` // Updated room with entity positions
}

// FeatureActivatedEvent is emitted when a combat feature is activated
type FeatureActivatedEvent struct {
	CharacterID   string      `json:"character_id"`
	FeatureID     string      `json:"feature_id"`
	Success       bool        `json:"success"`
	Message       string      `json:"message"`
	CharacterData interface{} `json:"character_data,omitempty"`
}

// TurnEndedEvent is emitted when a turn ends
type TurnEndedEvent struct {
	PreviousEntityID string            `json:"previous_entity_id"`
	NextEntityID     string            `json:"next_entity_id"`
	Round            int               `json:"round"`
	NewRound         bool              `json:"new_round"`
	CombatState      *CombatState      `json:"combat_state"`   // Full updated combat state
	Room             *spatial.RoomData `json:"room,omitempty"` // Updated room with entity positions
}

// MonsterTurnCompletedEvent is emitted when a monster completes its turn
type MonsterTurnCompletedEvent struct {
	MonsterID         string                  `json:"monster_id"`
	MonsterName       string                  `json:"monster_name"`
	Actions           []MonsterExecutedAction `json:"actions"`
	Movement          []Position              `json:"movement"`
	Room              *spatial.RoomData       `json:"room,omitempty"`              // Updated room with entity positions
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
