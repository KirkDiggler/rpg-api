# Combat Event Bus Design

## Overview
This document outlines the event bus pattern for combat systems, discovered during our prototype development. The event bus will enable reactive combat mechanics, real-time updates, and clean separation of concerns.

## Current State
Combat resolution is tightly coupled with encounter management. We need an event-driven architecture to:
- Enable real-time updates to connected clients
- Support combat reactions and triggers
- Maintain audit logs of combat actions
- Decouple combat mechanics from UI updates

## Prototype Design (Phase 1)

### Location
`rpg-api/internal/toolkit/combat/events.go`

### Core Interfaces
```go
// CombatEvent is the base interface for all combat events
type CombatEvent interface {
    GetID() string
    GetType() EventType
    GetTimestamp() time.Time
    GetEncounterID() string
    GetActorID() string
}

// EventType represents different combat event types
type EventType string

const (
    EventTypeAttack        EventType = "attack"
    EventTypeDefend        EventType = "defend"
    EventTypeDamage        EventType = "damage"
    EventTypeHeal          EventType = "heal"
    EventTypeStatusApplied EventType = "status_applied"
    EventTypeStatusRemoved EventType = "status_removed"
    EventTypeTurnStart     EventType = "turn_start"
    EventTypeTurnEnd       EventType = "turn_end"
    EventTypeDeath         EventType = "death"
    EventTypeRevive        EventType = "revive"
)
```

### Concrete Event Types
```go
// AttackEvent represents an attack attempt
type AttackEvent struct {
    ID          string
    Timestamp   time.Time
    EncounterID string
    AttackerID  string
    TargetID    string
    AttackRoll  int
    Modifier    int
    Total       int
    TargetAC    int
    Hit         bool
    Critical    bool
}

// DamageEvent represents damage being dealt
type DamageEvent struct {
    ID          string
    Timestamp   time.Time
    EncounterID string
    SourceID    string
    TargetID    string
    Amount      int
    DamageType  string
    IsCritical  bool
    Description string
}

// StatusEvent represents a status effect change
type StatusEvent struct {
    ID          string
    Timestamp   time.Time
    EncounterID string
    TargetID    string
    Status      string
    Duration    int
    Applied     bool // true for applied, false for removed
}
```

### Event Bus Implementation
```go
// EventHandler processes combat events
type EventHandler func(ctx context.Context, event CombatEvent) error

// CombatEventBus manages event distribution
type CombatEventBus struct {
    handlers map[EventType][]EventHandler
    mu       sync.RWMutex
}

// Subscribe registers a handler for specific event types
func (bus *CombatEventBus) Subscribe(eventType EventType, handler EventHandler) {
    bus.mu.Lock()
    defer bus.mu.Unlock()
    bus.handlers[eventType] = append(bus.handlers[eventType], handler)
}

// Publish sends an event to all registered handlers
func (bus *CombatEventBus) Publish(ctx context.Context, event CombatEvent) error {
    bus.mu.RLock()
    handlers := bus.handlers[event.GetType()]
    bus.mu.RUnlock()
    
    for _, handler := range handlers {
        if err := handler(ctx, event); err != nil {
            // Log error but continue processing
            // This ensures one failed handler doesn't break the chain
        }
    }
    return nil
}
```

## Integration Points

### With Combat Resolver
```go
func (r *Resolver) ResolveAttack(ctx context.Context, input *AttackInput) (*AttackOutput, error) {
    // ... resolve attack ...
    
    // Publish attack event
    attackEvent := &AttackEvent{
        ID:          generateID(),
        Timestamp:   time.Now(),
        EncounterID: input.EncounterID,
        AttackerID:  input.AttackerID,
        TargetID:    input.TargetID,
        AttackRoll:  output.AttackRoll,
        Hit:         output.Hit,
        Critical:    output.Critical,
    }
    r.eventBus.Publish(ctx, attackEvent)
    
    // If hit, publish damage event
    if output.Hit {
        damageEvent := &DamageEvent{
            ID:          generateID(),
            Timestamp:   time.Now(),
            EncounterID: input.EncounterID,
            SourceID:    input.AttackerID,
            TargetID:    input.TargetID,
            Amount:      output.Damage,
            DamageType:  output.DamageType,
            IsCritical:  output.Critical,
        }
        r.eventBus.Publish(ctx, damageEvent)
    }
    
    return output, nil
}
```

### With WebSocket Updates
```go
// WebSocketHandler listens for combat events and broadcasts to clients
type WebSocketHandler struct {
    hub *websocket.Hub
}

func (h *WebSocketHandler) HandleDamageEvent(ctx context.Context, event CombatEvent) error {
    damageEvent := event.(*DamageEvent)
    
    // Create client update message
    update := map[string]interface{}{
        "type":       "damage",
        "targetId":   damageEvent.TargetID,
        "amount":     damageEvent.Amount,
        "damageType": damageEvent.DamageType,
        "critical":   damageEvent.IsCritical,
    }
    
    // Broadcast to all clients in encounter
    return h.hub.BroadcastToEncounter(damageEvent.EncounterID, update)
}
```

### With Combat Log
```go
// CombatLogger persists all combat events
type CombatLogger struct {
    store EventStore
}

func (l *CombatLogger) LogEvent(ctx context.Context, event CombatEvent) error {
    return l.store.Save(ctx, event)
}

// Can replay combat by retrieving and replaying events
func (l *CombatLogger) ReplayCombat(ctx context.Context, encounterID string) ([]CombatEvent, error) {
    return l.store.GetByEncounter(ctx, encounterID)
}
```

## Use Cases

### Reaction System
```go
// Register reaction handlers
eventBus.Subscribe(EventTypeDamage, func(ctx context.Context, event CombatEvent) error {
    damageEvent := event.(*DamageEvent)
    
    // Check if target has Shield spell reaction
    if hasShieldReaction(damageEvent.TargetID) {
        // Trigger shield reaction
        return triggerShieldReaction(ctx, damageEvent)
    }
    return nil
})
```

### Death Saves
```go
eventBus.Subscribe(EventTypeDamage, func(ctx context.Context, event CombatEvent) error {
    damageEvent := event.(*DamageEvent)
    
    // Check if damage reduces target to 0 HP
    if targetHP := getHP(damageEvent.TargetID); targetHP - damageEvent.Amount <= 0 {
        // Publish death event
        deathEvent := &DeathEvent{
            TargetID: damageEvent.TargetID,
            // ...
        }
        return eventBus.Publish(ctx, deathEvent)
    }
    return nil
})
```

### Combat Triggers
```go
// Rage on taking damage (Barbarian)
eventBus.Subscribe(EventTypeDamage, func(ctx context.Context, event CombatEvent) error {
    damageEvent := event.(*DamageEvent)
    
    if isBarbarian(damageEvent.TargetID) && !isRaging(damageEvent.TargetID) {
        // Auto-trigger rage
        return triggerRage(ctx, damageEvent.TargetID)
    }
    return nil
})
```

## Migration Path

### Phase 1: Prototype (Current)
- Implement basic event types and bus
- Use within combat package only
- Iterate on event structure

### Phase 2: Production
- Move to internal/combat/events
- Add persistence layer
- Integrate with WebSocket broadcasting
- Add comprehensive testing

### Phase 3: Toolkit
- Extract generic event interfaces to rpg-toolkit
- Keep specific event types in rpg-api
- Provide event bus as library component

## Benefits

1. **Decoupling**: Combat mechanics separate from UI updates
2. **Extensibility**: Easy to add new reactions and triggers
3. **Auditability**: Complete combat log for replay/debugging
4. **Real-time**: Instant updates to all connected clients
5. **Testability**: Can test combat logic without UI concerns

## Considerations

### Performance
- Async event processing for non-critical handlers
- Batch events for bulk operations
- Consider event queue for high-volume scenarios

### Error Handling
- Failed handlers shouldn't break combat flow
- Log errors for debugging
- Consider retry logic for critical handlers

### Event Ordering
- Maintain strict ordering for same encounter
- Use timestamps for event sequencing
- Consider event priority for reactions

## Next Steps

1. Implement basic event types in prototype
2. Add event bus to combat resolver
3. Test with simple handlers
4. Iterate on event structure based on needs
5. Add WebSocket integration when stable