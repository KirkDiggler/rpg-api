// Package entities defines the core data structures for the RPG API
package entities

import (
	"time"
)

// Player represents a player in an encounter
type Player struct {
	PlayerID    string    `json:"player_id"`
	CharacterID string    `json:"character_id"`
	IsReady     bool      `json:"is_ready"`
	IsConnected bool      `json:"is_connected"`
	IsHost      bool      `json:"is_host"`
	JoinedAt    time.Time `json:"joined_at"`
}

// Position represents a position using cube coordinates (x + y + z = 0)
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// InitiativeEntry represents one entity in the initiative order
type InitiativeEntry struct {
	EntityID           string    `json:"entity_id"`
	EntityType         string    `json:"entity_type"` // "character" or "monster"
	InitiativeRoll     int       `json:"initiative_roll"`
	InitiativeModifier int       `json:"initiative_modifier"`
	InitiativeTotal    int       `json:"initiative_total"`
	Position           *Position `json:"position,omitempty"`
}

// CombatState represents the state of combat in an encounter
type CombatState struct {
	EncounterID       string            `json:"encounter_id"`
	Round             int               `json:"round"`
	TurnOrder         []InitiativeEntry `json:"turn_order"`
	ActiveIndex       int               `json:"active_index"`
	MovementRemaining int32             `json:"movement_remaining"`
	CombatStarted     bool              `json:"combat_started"`
	CombatEnded       bool              `json:"combat_ended"`
}
