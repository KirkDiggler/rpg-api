# Entities

Pure data structures representing game objects. 

## ⚠️ IMPORTANT: No Business Logic Here!

Entities in rpg-api are **data containers only**. They have:
- ✅ Fields for storing data
- ✅ Simple struct tags for serialization
- ❌ NO calculation methods
- ❌ NO game rule logic
- ❌ NO behavior methods

## Why?

All game logic lives in **rpg-toolkit** (accessed via the engine interface). This separation ensures:
- rpg-api remains a simple storage/API layer
- Game rules are centralized in rpg-toolkit
- Different game systems can use the same API

## Example

```go
// ✅ CORRECT: Entity with just data
type Character struct {
    ID        string
    Name      string
    Level     int32
    CurrentHP int32
}

// ❌ WRONG: Entity with behavior
type Character struct {
    // ... fields ...
}

func (c *Character) TakeDamage(amount int) {
    c.CurrentHP -= amount // NO! Use engine for this
}

func (c *Character) GetAC() int {
    return 10 + c.DexModifier // NO! Engine calculates this
}
```

## Where Does Logic Go?

1. **Simple validation**: In orchestrators (e.g., "name is required")
2. **Game rules**: In the engine (rpg-toolkit)
3. **Calculations**: In the engine (rpg-toolkit)
4. **State changes**: Orchestrators use engine, then update entities

See [ADR-002](../../docs/adr/002-entity-data-models.md) for detailed rationale.
