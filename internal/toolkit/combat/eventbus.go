package combat

import (
	"context"
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/events"
)

// CombatEventBus wraps the rpg-toolkit event bus for combat-specific operations
type CombatEventBus struct {
	bus events.EventBus
}

// NewCombatEventBus creates a new combat-specific event bus
func NewCombatEventBus() *CombatEventBus {
	return &CombatEventBus{
		bus: events.NewBus(),
	}
}

// RegisterCondition registers handlers for a condition (like Bless, Rage, etc)
func (c *CombatEventBus) RegisterCondition(condition Condition) error {
	handlers := condition.GetEventHandlers()
	for eventType, handler := range handlers {
		c.bus.Subscribe(eventType, handler)
	}
	return nil
}

// UnregisterCondition removes handlers for a condition
func (c *CombatEventBus) UnregisterCondition(conditionID string) error {
	// This would need to track subscription IDs per condition
	// For now, we'll implement this when needed
	return nil
}

// PublishAttackEvent publishes an attack event through the pipeline
func (c *CombatEventBus) PublishAttackEvent(ctx context.Context, event events.Event) error {
	return c.bus.Publish(ctx, event)
}

// Subscribe allows direct subscription to events
func (c *CombatEventBus) Subscribe(eventType string, handler events.Handler) string {
	return c.bus.Subscribe(eventType, handler)
}

// SubscribeFunc allows function-based subscriptions
func (c *CombatEventBus) SubscribeFunc(eventType string, priority int, fn events.HandlerFunc) string {
	return c.bus.SubscribeFunc(eventType, priority, fn)
}

// Condition represents something that can modify combat (Bless, Rage, etc)
type Condition interface {
	// GetEventHandlers returns a map of event type to handler
	GetEventHandlers() map[string]events.Handler
	// GetID returns the unique identifier for this condition instance
	GetID() string
}

// Example condition implementation for testing
type BlessCondition struct {
	id       string
	entityID string // Who has this condition
}

func NewBlessCondition(entityID string) *BlessCondition {
	return &BlessCondition{
		id:       fmt.Sprintf("bless_%s", entityID),
		entityID: entityID,
	}
}

func (b *BlessCondition) GetID() string {
	return b.id
}

func (b *BlessCondition) GetEventHandlers() map[string]events.Handler {
	handlers := make(map[string]events.Handler)

	// Bless adds 1d4 to attack rolls and saving throws
	handlers[events.EventBeforeAttackRoll] = &blessAttackHandler{
		entityID: b.entityID,
	}

	handlers[events.EventBeforeSavingThrow] = &blessSaveHandler{
		entityID: b.entityID,
	}

	return handlers
}

// blessAttackHandler adds 1d4 to attack rolls
type blessAttackHandler struct {
	entityID string
}

func (h *blessAttackHandler) Handle(ctx context.Context, event events.Event) error {
	// Only apply if this entity is the attacker
	if event.Source() != nil && event.Source().GetID() == h.entityID {
		// Add a modifier to the event context
		modifier := NewDiceModifier(
			"Bless",
			"attack_roll",
			"1d4",
			10, // Priority
			"Bless spell adds 1d4 to attack rolls",
		)
		event.Context().AddModifier(modifier)
	}
	return nil
}

func (h *blessAttackHandler) Priority() int {
	return 10 // Early in the chain
}

// blessSaveHandler adds 1d4 to saving throws
type blessSaveHandler struct {
	entityID string
}

func (h *blessSaveHandler) Handle(ctx context.Context, event events.Event) error {
	// Only apply if this entity is making the save
	if event.Target() != nil && event.Target().GetID() == h.entityID {
		modifier := NewDiceModifier(
			"Bless",
			"saving_throw",
			"1d4",
			10,
			"Bless spell adds 1d4 to saving throws",
		)
		event.Context().AddModifier(modifier)
	}
	return nil
}

func (h *blessSaveHandler) Priority() int {
	return 10
}
