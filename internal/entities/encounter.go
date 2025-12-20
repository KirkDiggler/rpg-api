// Package entities defines the core data structures for the RPG API
package entities

import (
	"time"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// Player represents a player in an encounter
type Player struct {
	PlayerID      string          `json:"player_id"`
	CharacterID   string          `json:"character_id"`
	CharacterData *character.Data `json:"character_data,omitempty"`
	IsReady       bool            `json:"is_ready"`
	IsConnected   bool            `json:"is_connected"`
	IsHost        bool            `json:"is_host"`
	JoinedAt      time.Time       `json:"joined_at"`
}

// Position represents a position using cube coordinates (x + y + z = 0)
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// MonsterExecutedAction represents an action taken by a monster during its turn
type MonsterExecutedAction struct {
	ActionID   string      `json:"action_id"`
	ActionType string      `json:"action_type"` // melee_attack, ranged_attack, spell, heal, etc.
	TargetID   string      `json:"target_id,omitempty"`
	Success    bool        `json:"success"`
	Details    interface{} `json:"details,omitempty"` // AttackResult, HealResult, etc.
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
	EncounterID       string              `json:"encounter_id"`
	Round             int                 `json:"round"`
	TurnOrder         []InitiativeEntry   `json:"turn_order"`
	ActiveIndex       int                 `json:"active_index"`
	MovementRemaining int32               `json:"movement_remaining"`
	ActionEconomy     *ActionEconomyState `json:"action_economy,omitempty"`
	CombatStarted     bool                `json:"combat_started"`
	CombatEnded       bool                `json:"combat_ended"`
}

// ActionEconomyState tracks available actions for the current turn
type ActionEconomyState struct {
	ActionsRemaining      int `json:"actions_remaining"`
	BonusActionsRemaining int `json:"bonus_actions_remaining"`
	ReactionsRemaining    int `json:"reactions_remaining"`
}

// NewActionEconomyState creates a fresh action economy with default values
func NewActionEconomyState() *ActionEconomyState {
	return &ActionEconomyState{
		ActionsRemaining:      1,
		BonusActionsRemaining: 1,
		ReactionsRemaining:    1,
	}
}

// HasAction returns true if an action is available
func (a *ActionEconomyState) HasAction() bool {
	return a != nil && a.ActionsRemaining > 0
}

// HasBonusAction returns true if a bonus action is available
func (a *ActionEconomyState) HasBonusAction() bool {
	return a != nil && a.BonusActionsRemaining > 0
}

// HasReaction returns true if a reaction is available
func (a *ActionEconomyState) HasReaction() bool {
	return a != nil && a.ReactionsRemaining > 0
}

// UseAction consumes an action. Call HasAction() first to check availability.
func (a *ActionEconomyState) UseAction() {
	if a.HasAction() {
		a.ActionsRemaining--
	}
}

// UseBonusAction consumes a bonus action. Call HasBonusAction() first to check availability.
func (a *ActionEconomyState) UseBonusAction() {
	if a.HasBonusAction() {
		a.BonusActionsRemaining--
	}
}

// UseReaction consumes a reaction. Call HasReaction() first to check availability.
func (a *ActionEconomyState) UseReaction() {
	if a.HasReaction() {
		a.ReactionsRemaining--
	}
}
