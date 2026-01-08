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

// DoorInfo describes a door or passage connecting the current room to another room
type DoorInfo struct {
	ConnectionID string    `json:"connection_id"` // Unique ID of this connection
	TargetRoomID string    `json:"target_room_id"`
	Direction    string    `json:"direction"` // Physical description (e.g., "north door", "stairs down")
	Position     *Position `json:"position,omitempty"`
	IsOpen       bool      `json:"is_open"`
}

// MonsterActionDetails holds the action-specific result data (oneof-style pattern)
// Only one field should be populated based on the action type
type MonsterActionDetails struct {
	AttackResult *AttackResult `json:"attack_result,omitempty"`
	HealResult   *HealResult   `json:"heal_result,omitempty"`
}

// MonsterExecutedAction represents an action taken by a monster during its turn
type MonsterExecutedAction struct {
	ActionID   string                `json:"action_id"`
	ActionType string                `json:"action_type"` // melee_attack, ranged_attack, spell, heal, etc.
	TargetID   string                `json:"target_id,omitempty"`
	Success    bool                  `json:"success"`
	Details    *MonsterActionDetails `json:"details,omitempty"`
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

	// Attack-related fields (granted by ATTACK ability)
	AttacksRemaining        int `json:"attacks_remaining"`
	OffHandAttacksRemaining int `json:"off_hand_attacks_remaining"`
	FlurryStrikesRemaining  int `json:"flurry_strikes_remaining"`

	// Status flags (set by abilities)
	DisengageActive bool `json:"disengage_active"`
	DodgeActive     bool `json:"dodge_active"`
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

// HasAttacks returns true if attacks are available
func (a *ActionEconomyState) HasAttacks() bool {
	return a != nil && a.AttacksRemaining > 0
}

// UseAttack consumes an attack. Call HasAttacks() first to check availability.
func (a *ActionEconomyState) UseAttack() {
	if a.HasAttacks() {
		a.AttacksRemaining--
	}
}

// HasOffHandAttacks returns true if off-hand attacks are available
func (a *ActionEconomyState) HasOffHandAttacks() bool {
	return a != nil && a.OffHandAttacksRemaining > 0
}

// UseOffHandAttack consumes an off-hand attack. Call HasOffHandAttacks() first to check availability.
func (a *ActionEconomyState) UseOffHandAttack() {
	if a.HasOffHandAttacks() {
		a.OffHandAttacksRemaining--
	}
}

// HasFlurryStrikes returns true if flurry strikes are available
func (a *ActionEconomyState) HasFlurryStrikes() bool {
	return a != nil && a.FlurryStrikesRemaining > 0
}

// UseFlurryStrike consumes a flurry strike. Call HasFlurryStrikes() first to check availability.
func (a *ActionEconomyState) UseFlurryStrike() {
	if a.HasFlurryStrikes() {
		a.FlurryStrikesRemaining--
	}
}
