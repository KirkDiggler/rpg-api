# Event-Driven Features Architecture

**Date**: 2025-11-24
**Context**: Understanding how passive features (Sneak Attack, Rage damage, etc.) modify combat without `if (attack == "sneak_attack")` checks

---

## The Problem

How do features like Sneak Attack, Rage damage bonus, and other passive effects modify combat behavior without hard-coded conditionals?

**Bad Pattern (What We Want to Avoid)**:
```go
// ❌ Hard-coded feature checks
func calculateDamage(attack Attack) int {
    damage := attack.BaseDamage

    if attack.AttackerFeature == "rage" {
        damage += 2
    }

    if attack.AttackerFeature == "sneak_attack" && hasAdvantage {
        damage += rollDice("3d6")
    }

    return damage
}
```

This doesn't scale - every new feature requires modifying the damage calculation function.

---

## The Solution: Event Chains

rpg-toolkit uses a **Chain of Responsibility** pattern with **event-driven subscriptions**.

### Core Concept

1. **Combat publishes events** as things happen (attack, damage, turn start, etc.)
2. **Features subscribe to relevant events** and add modifiers to a chain
3. **Chain executes in staged order** to apply all modifiers correctly

### The Journey Pattern

From the toolkit documentation:

> Events travel through features, each adding its contribution. The chain collects all modifiers, then applies them in staged order.

```
Attack Event → [Rage] → [Bless] → [Hunter's Mark] → Execute Chain → Final Damage
               +2        +1d4       +1d6                          = 10 + 2 + 4 + 6 = 22
```

---

## How It Works: Detailed Example

### 1. Define Event Types

```go
// From github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat

// DamageChain is a ChainedTopic that accumulates damage modifiers
var DamageChain = events.DefineChainedTopic[*DamageChainEvent]("dnd5e.combat.damage.chain")

type DamageChainEvent struct {
    AttackerID   string
    TargetID     string
    Components   []DamageComponent
    DamageType   string
    IsCritical   bool
    WeaponDamage string            // e.g., "1d8"
    AbilityUsed  abilities.Ability
}
```

### 2. Features Subscribe to Events

When a character is loaded with `LoadFromData(ctx, data, bus)`, features automatically subscribe:

```go
// Pseudo-code for how Rage subscribes to damage events

type Rage struct {
    ownerID     string
    damageBonus int // +2 at level 1
    subscriptionID string
}

// During character load, Rage subscribes to DamageChain
func (r *Rage) subscribeToEvents(ctx context.Context, bus events.EventBus) error {
    damageChain := combat.DamageChain.On(bus)

    subID, err := damageChain.SubscribeWithChain(ctx, r.modifyDamage)
    r.subscriptionID = subID
    return err
}

// Handler that adds rage bonus to damage
func (r *Rage) modifyDamage(
    ctx context.Context,
    event *combat.DamageChainEvent,
    chain chain.Chain[*combat.DamageChainEvent],
) (chain.Chain[*combat.DamageChainEvent], error) {

    // Only modify if this is MY attack
    if event.AttackerID != r.ownerID {
        return chain, nil
    }

    // Only for melee weapon attacks using Strength
    if event.AbilityUsed != abilities.STR {
        return chain, nil
    }

    // Add rage damage modifier to chain
    chain.Add(StageFeatures, "rage_damage", func(ctx context.Context, e *combat.DamageChainEvent) (*combat.DamageChainEvent, error) {
        // Add +2 damage
        e.Components = append(e.Components, combat.DamageComponent{
            Source: combat.DamageSourceFeature,
            Name:   "Rage",
            Amount: r.damageBonus,
        })
        return e, nil
    })

    return chain, nil
}
```

### 3. Combat Publishes Events

```go
// From ResolveAttack in combat package

func ResolveAttack(ctx context.Context, input *AttackInput) (*AttackResult, error) {
    // ... attack roll logic ...

    // Create damage event
    damageEvent := &DamageChainEvent{
        AttackerID:   input.AttackerID,
        TargetID:     input.TargetID,
        WeaponDamage: weapon.Damage,
        AbilityUsed:  abilities.STR,
        IsCritical:   isNaturalTwenty,
        Components:   []DamageComponent{
            {Source: DamageSourceWeapon, Name: weapon.Name, Amount: weaponDamage},
        },
    }

    // Create staged chain for proper ordering
    damageChain := chain.NewStagedChain[*DamageChainEvent](damageStages)

    // Publish event - ALL subscribed features add their modifiers
    modifiedChain, err := combat.DamageChain.On(bus).PublishWithChain(ctx, damageEvent, damageChain)

    // Execute chain to apply all modifiers in order
    finalEvent, err := modifiedChain.Execute(ctx, damageEvent)

    // finalEvent.Components now includes:
    // - Weapon damage
    // - Rage damage (+2)
    // - Any other active feature bonuses

    totalDamage := sumDamageComponents(finalEvent.Components)

    return &AttackResult{
        Hit:           true,
        Damage:        totalDamage,
        DamageBreakdown: finalEvent.Components,
    }, nil
}
```

### 4. Staged Execution Order

The chain uses **stages** to ensure modifiers apply in the correct order:

```go
const (
    StageBase       = 0   // Base values (weapon damage, ability modifier)
    StageFeatures   = 100 // Feature bonuses (Rage, Sneak Attack)
    StageConditions = 200 // Condition modifiers (Bless, Bane)
    StageMultipliers = 300 // Critical hits, vulnerabilities
    StageReductions = 400 // Resistances, armor
)
```

This ensures:
1. Base weapon damage is calculated first
2. Rage adds +2 damage
3. Bless adds +1d4
4. Critical hit doubles the total
5. Resistance cuts it in half

---

## Sneak Attack Example

Now let's see how Sneak Attack would work:

```go
type SneakAttack struct {
    ownerID    string
    level      int // Determines dice (3d6 at level 5)
    usedThisTurn bool
}

func (s *SneakAttack) modifyDamage(
    ctx context.Context,
    event *combat.DamageChainEvent,
    chain chain.Chain[*combat.DamageChainEvent],
) (chain.Chain[*combat.DamageChainEvent], error) {

    // Only my attacks
    if event.AttackerID != s.ownerID {
        return chain, nil
    }

    // Already used this turn
    if s.usedThisTurn {
        return chain, nil
    }

    // Check conditions programmatically (no hard-coded strings!)
    if !s.canApplySneakAttack(ctx, event) {
        return chain, nil
    }

    // Add sneak attack damage
    chain.Add(StageFeatures, "sneak_attack", func(ctx context.Context, e *combat.DamageChainEvent) (*combat.DamageChainEvent, error) {
        dice := fmt.Sprintf("%dd6", (s.level + 1) / 2) // 1d6 at level 1, 2d6 at level 3, etc.
        sneakDamage := rollDice(dice)

        e.Components = append(e.Components, combat.DamageComponent{
            Source: combat.DamageSourceFeature,
            Name:   "Sneak Attack",
            Amount: sneakDamage,
        })

        s.usedThisTurn = true
        return e, nil
    })

    return chain, nil
}

// Programmatic condition checking - no string comparisons!
func (s *SneakAttack) canApplySneakAttack(ctx context.Context, event *combat.DamageChainEvent) bool {
    // TODO: Check for advantage or ally within 5ft of target
    // This would query the combat state/grid from context

    // For now, simplified:
    return true // Assume conditions are met
}
```

---

## Benefits of This Pattern

### ✅ No Hard-Coded Feature Checks
Combat code doesn't know about Rage, Sneak Attack, or any specific features. It just publishes events.

### ✅ Features Are Self-Contained
Each feature contains:
- Its own logic for when it applies
- Its own subscription setup
- Its own modifier calculations

### ✅ Easy to Add New Features
New features just:
1. Implement the `Feature` interface
2. Subscribe to relevant events in their constructor
3. Add modifiers when their conditions are met

### ✅ Proper Execution Order
Staged chains ensure modifiers apply in the correct D&D 5e order:
- Base damage
- Feature bonuses
- Condition bonuses
- Multipliers (critical hits)
- Reductions (resistance)

### ✅ Clean Debugging
The damage breakdown shows exactly what contributed:
```json
{
  "total_damage": 24,
  "components": [
    {"source": "weapon", "name": "Longsword", "amount": 8},
    {"source": "ability", "name": "Strength Modifier", "amount": 3},
    {"source": "feature", "name": "Rage", "amount": 2},
    {"source": "feature", "name": "Sneak Attack", "amount": 11}
  ]
}
```

---

## How This Works in rpg-api

### When Character Is Loaded

```go
// In internal/orchestrators/encounter/orchestrator.go

// CRITICAL: Create EventBus
bus := events.NewEventBus()

// Load character - features auto-subscribe
char, err := character.LoadFromData(ctx, attackerData, bus)
defer char.Cleanup(ctx) // Unsubscribes all features

// Now all features (Rage, Sneak Attack, etc.) are listening on the bus
```

### When Attack Happens

```go
// Combat publishes damage event
attackResult, err := combat.ResolveAttack(ctx, &combat.AttackInput{
    AttackerID: "char-123",
    TargetID:   "goblin-456",
    WeaponRef:  "longsword",
}, bus) // Bus contains all feature subscriptions

// Rage automatically added +2
// Sneak Attack automatically added 3d6
// No feature-specific code in ResolveAttack!
```

### The Magic

The **bus** carries the feature subscriptions. When combat publishes to `DamageChain`, the bus routes the event to all subscribed features (Rage, Sneak Attack, etc.), they each add their modifiers, and the chain executes them in order.

**No string checks. No switch statements. Just events flowing through the system.**

---

## Pattern for Checking Sneak Attack Conditions

You asked: *"how it gets loaded onto the bus if that direction. I wonder if that check if sneak attack can be performed if we can do it programmatically and not with an if attack == 'sneak_attack'"*

### The Answer

Sneak Attack checks its own conditions **inside the event handler**:

```go
func (s *SneakAttack) modifyDamage(ctx, event, chain) (chain, error) {
    // Programmatic checks - no string comparisons

    // Check 1: Is this my attack?
    if event.AttackerID != s.ownerID {
        return chain, nil // Not my attack, don't modify
    }

    // Check 2: Already used this turn?
    if s.usedThisTurn {
        return chain, nil
    }

    // Check 3: Do I have advantage? (from event or context)
    hasAdvantage := queryAdvantageFromContext(ctx, event)

    // Check 4: Is ally within 5ft of target? (from combat grid)
    allyNearby := queryAllyProximity(ctx, event.TargetID)

    // Check 5: Must be finesse or ranged weapon
    isValidWeapon := isWeaponFinesse(event.WeaponRef) || !event.IsMelee

    // Only apply if conditions met
    if (hasAdvantage || allyNearby) && isValidWeapon {
        // Add sneak attack damage to chain
        chain.Add(StageFeatures, "sneak_attack", ...)
        s.usedThisTurn = true
    }

    return chain, nil
}
```

The feature **self-determines** if it should apply. Combat doesn't need to know about Sneak Attack at all.

---

## Key Architectural Points

### 1. EventBus Is Per-Request
Each request creates its own bus:
```go
bus := events.NewEventBus()
char, _ := character.LoadFromData(ctx, data, bus)
// Do combat/actions
char.Cleanup(ctx) // Unsubscribe
```

This ensures:
- No cross-contamination between requests
- Features only affect their owner's actions
- Clean subscription lifecycle

### 2. Features Subscribe During Load
`LoadFromData()` internally calls:
```go
for _, feature := range char.features {
    feature.subscribeToEvents(ctx, bus)
}
```

So by the time you call `combat.ResolveAttack()`, all features are already listening.

### 3. Conditions Are Just Features
Active conditions (Raging, Blessed, Frightened) are also features that subscribe to events:
```go
type RagingCondition struct {
    // Adds +2 damage and damage resistance
}

func (r *RagingCondition) Apply(ctx, bus) error {
    // Subscribe to DamageChain to add bonus
    // Subscribe to DamageReceivedEvent to apply resistance
}
```

When Rage is activated:
1. Creates "Raging" condition
2. Condition is added to `character.Conditions`
3. On next `LoadFromData()`, condition subscribes to events
4. Damage bonus and resistance automatically apply

---

## Summary

### How Passive Features Work

1. **Features subscribe to events** during character load
2. **Combat publishes events** when things happen (attack, damage, turn start)
3. **Features check their own conditions** inside event handlers
4. **Features add modifiers to chains** if conditions are met
5. **Chains execute in stages** to apply all modifiers in order

### No String Checks Required

Instead of:
```go
if attack == "sneak_attack" {
    // Hard-coded logic
}
```

We have:
```go
// Sneak Attack subscribes to DamageChain
// Checks its own conditions
// Adds damage if conditions met
// Combat never knows Sneak Attack exists
```

### The Magic Ingredient

**The EventBus** is the connector. It carries feature subscriptions from character load to combat resolution.

```go
bus := events.NewEventBus()           // Create bus
char, _ := LoadFromData(ctx, data, bus) // Features subscribe
combat.ResolveAttack(ctx, input, bus)   // Features modify combat
```

That's the entire pattern!

---

## References

- [rpg-toolkit GitHub](https://github.com/KirkDiggler/rpg-toolkit)
- [rpg-toolkit Go Packages](https://pkg.go.dev/github.com/KirkDiggler/rpg-toolkit)
- Event chain documentation from `go doc github.com/KirkDiggler/rpg-toolkit/events`

---

*This architecture enables extensible, maintainable feature systems without hard-coded conditionals for each feature.*
