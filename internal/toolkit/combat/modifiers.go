package combat

import (
	"context"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/events"
)

// DiceModifier represents a dice-based modifier (like Bless's 1d4)
type DiceModifier struct {
	source        string
	modType       string
	diceExpr      string
	priority      int
	description   string
	sourceDetails *events.ModifierSource
}

// NewDiceModifier creates a new dice-based modifier
func NewDiceModifier(source, modType, diceExpr string, priority int, description string) *DiceModifier {
	return &DiceModifier{
		source:      source,
		modType:     modType,
		diceExpr:    diceExpr,
		priority:    priority,
		description: description,
		sourceDetails: &events.ModifierSource{
			Type:        "spell",
			Name:        source,
			Description: description,
		},
	}
}

func (m *DiceModifier) Source() string     { return m.source }
func (m *DiceModifier) Type() string       { return m.modType }
func (m *DiceModifier) Value() interface{} { return m.diceExpr }
func (m *DiceModifier) ModifierValue() events.ModifierValue {
	// Return the dice expression directly as the ModifierValue interface
	return nil // For now, return nil as ModifierValue may not be needed
}
func (m *DiceModifier) Priority() int { return m.priority }
func (m *DiceModifier) Condition(event events.Event) bool {
	// Always applies when added
	return true
}
func (m *DiceModifier) Duration() events.Duration {
	// For now, modifiers last until removed
	return nil
}
func (m *DiceModifier) SourceDetails() *events.ModifierSource {
	return m.sourceDetails
}

// FlatModifier represents a flat numeric modifier (like +2 from a magic weapon)
type FlatModifier struct {
	source        string
	modType       string
	value         int
	priority      int
	description   string
	sourceDetails *events.ModifierSource
}

// NewFlatModifier creates a new flat numeric modifier
func NewFlatModifier(source, modType string, value, priority int, description string) *FlatModifier {
	return &FlatModifier{
		source:      source,
		modType:     modType,
		value:       value,
		priority:    priority,
		description: description,
		sourceDetails: &events.ModifierSource{
			Type:        "item",
			Name:        source,
			Description: description,
		},
	}
}

func (m *FlatModifier) Source() string     { return m.source }
func (m *FlatModifier) Type() string       { return m.modType }
func (m *FlatModifier) Value() interface{} { return m.value }
func (m *FlatModifier) ModifierValue() events.ModifierValue {
	// Return the value directly as the ModifierValue interface
	return nil // For now, return nil as ModifierValue may not be needed
}
func (m *FlatModifier) Priority() int { return m.priority }
func (m *FlatModifier) Condition(event events.Event) bool {
	return true
}
func (m *FlatModifier) Duration() events.Duration {
	return nil
}
func (m *FlatModifier) SourceDetails() *events.ModifierSource {
	return m.sourceDetails
}

// AdvantageModifier represents advantage/disadvantage on a roll
type AdvantageModifier struct {
	source        string
	advantage     bool // true for advantage, false for disadvantage
	priority      int
	description   string
	sourceDetails *events.ModifierSource
}

// NewAdvantageModifier creates a new advantage modifier
func NewAdvantageModifier(source string, advantage bool, priority int, description string) *AdvantageModifier {
	modType := "condition"
	if advantage {
		modType = "feature"
	}

	return &AdvantageModifier{
		source:      source,
		advantage:   advantage,
		priority:    priority,
		description: description,
		sourceDetails: &events.ModifierSource{
			Type:        modType,
			Name:        source,
			Description: description,
		},
	}
}

func (m *AdvantageModifier) Source() string { return m.source }
func (m *AdvantageModifier) Type() string {
	if m.advantage {
		return "advantage"
	}
	return "disadvantage"
}
func (m *AdvantageModifier) Value() interface{} { return m.advantage }
func (m *AdvantageModifier) ModifierValue() events.ModifierValue {
	// Return the advantage boolean directly as the ModifierValue interface
	return nil // For now, return nil as ModifierValue may not be needed
}
func (m *AdvantageModifier) Priority() int { return m.priority }
func (m *AdvantageModifier) Condition(event events.Event) bool {
	return true
}
func (m *AdvantageModifier) Duration() events.Duration {
	return nil
}
func (m *AdvantageModifier) SourceDetails() *events.ModifierSource {
	return m.sourceDetails
}

// RageCondition provides an example of a more complex condition
type RageCondition struct {
	id       string
	entityID string
	entity   core.Entity
}

func NewRageCondition(entity core.Entity) *RageCondition {
	return &RageCondition{
		id:       "rage_" + entity.GetID(),
		entityID: entity.GetID(),
		entity:   entity,
	}
}

func (r *RageCondition) GetID() string { return r.id }

func (r *RageCondition) GetEventHandlers() map[string]events.Handler {
	handlers := make(map[string]events.Handler)

	// Rage gives advantage on STR checks and saves
	handlers[events.EventBeforeSavingThrow] = &rageStrengthHandler{
		entity: r.entity,
	}

	// Rage gives bonus damage on melee attacks
	handlers[events.EventBeforeDamageRoll] = &rageDamageHandler{
		entity: r.entity,
	}

	// Rage gives resistance to physical damage
	handlers[events.EventBeforeTakeDamage] = &rageResistanceHandler{
		entity: r.entity,
	}

	return handlers
}

// rageStrengthHandler gives advantage on STR saves
type rageStrengthHandler struct {
	entity core.Entity
}

func (h *rageStrengthHandler) Handle(ctx context.Context, event events.Event) error {
	// Check if this is a STR save for our entity
	if saveType, ok := event.Context().GetString(ContextKeySaveType); ok && saveType == "STR" {
		if event.Target() != nil && event.Target().GetID() == h.entity.GetID() {
			modifier := NewAdvantageModifier(
				"Rage",
				true, // advantage
				5,    // early priority
				"Rage gives advantage on Strength saving throws",
			)
			event.Context().AddModifier(modifier)
		}
	}
	return nil
}

func (h *rageStrengthHandler) Priority() int { return 5 }

// rageDamageHandler adds bonus damage to melee attacks
type rageDamageHandler struct {
	entity core.Entity
}

func (h *rageDamageHandler) Handle(ctx context.Context, event events.Event) error {
	// Check if this is a melee attack from our entity
	if attackType, ok := event.Context().GetString(ContextKeyAttackType); ok && attackType == "melee" {
		if event.Source() != nil && event.Source().GetID() == h.entity.GetID() {
			// Rage damage bonus (simplified - normally based on barbarian level)
			modifier := NewFlatModifier(
				"Rage",
				"damage_bonus",
				2,  // +2 damage
				20, // Applied after base damage
				"Rage adds bonus damage to melee attacks",
			)
			event.Context().AddModifier(modifier)
		}
	}
	return nil
}

func (h *rageDamageHandler) Priority() int { return 20 }

// rageResistanceHandler gives resistance to physical damage
type rageResistanceHandler struct {
	entity core.Entity
}

func (h *rageResistanceHandler) Handle(ctx context.Context, event events.Event) error {
	// Check if taking physical damage
	if damageType, ok := event.Context().GetString(ContextKeyDamageType); ok {
		if event.Target() != nil && event.Target().GetID() == h.entity.GetID() {
			// Check if it's physical damage
			if damageType == "slashing" || damageType == "piercing" || damageType == "bludgeoning" {
				event.Context().Set(ContextKeyResistance, true)
				// Add a modifier to track this
				modifier := &resistanceModifier{
					source:      "Rage",
					damageType:  damageType,
					description: "Rage grants resistance to physical damage",
				}
				event.Context().AddModifier(modifier)
			}
		}
	}
	return nil
}

func (h *rageResistanceHandler) Priority() int { return 10 }

// resistanceModifier tracks damage resistance
type resistanceModifier struct {
	source      string
	damageType  string
	description string
}

func (m *resistanceModifier) Source() string     { return m.source }
func (m *resistanceModifier) Type() string       { return "resistance" }
func (m *resistanceModifier) Value() interface{} { return m.damageType }
func (m *resistanceModifier) ModifierValue() events.ModifierValue {
	// Return the damage type directly as the ModifierValue interface
	return nil // For now, return nil as ModifierValue may not be needed
}
func (m *resistanceModifier) Priority() int                     { return 50 }
func (m *resistanceModifier) Condition(event events.Event) bool { return true }
func (m *resistanceModifier) Duration() events.Duration         { return nil }
func (m *resistanceModifier) SourceDetails() *events.ModifierSource {
	return &events.ModifierSource{
		Type:        "feature",
		Name:        m.source,
		Description: m.description,
	}
}
