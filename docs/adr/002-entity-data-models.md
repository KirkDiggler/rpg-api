# ADR-002: Entities as Pure Data Models

Date: 2025-01-13

## Status

Accepted

## Context

In typical Domain-Driven Design, entities contain both data and behavior. A `Character` entity might have methods like:
- `CalculateAC()`
- `GetProficiencyBonus()`
- `RollInitiative()`
- `ApplyDamage(amount int)`

However, our architecture separates rpg-api (storage/API) from rpg-toolkit (rules engine).

## Decision

Entities in rpg-api are **pure data structures** with no behavior or calculation methods.

### What Goes Where

**rpg-api entities** (e.g., `/internal/entities/dnd5e/character.go`):
- Simple structs with fields only
- No methods (except maybe convenience getters)
- No game logic or calculations
- Just data storage representation

```go
// This is ALL a Character entity has in rpg-api
type Character struct {
    ID               string
    Name             string
    Level            int32
    ExperiencePoints int32
    RaceID           string
    ClassID          string
    AbilityScores    AbilityScores
    CurrentHP        int32
    // ... just data fields
}
```

**rpg-toolkit domain models**:
- Rich objects with behavior
- All calculations and game rules
- Methods for game mechanics
- Domain logic implementation

```go
// In rpg-toolkit (not rpg-api)
type Character struct {
    // ... fields ...
}

func (c *Character) CalculateAC() int {
    // Complex armor class calculation
}

func (c *Character) GetProficiencyBonus() int {
    // Level-based calculation
}
```

**Engine interface** (in rpg-api):
- Bridge to rpg-toolkit
- Accepts data, returns calculations
- All game logic delegated here

```go
// Engine does calculations, not entities
output, err := engine.CalculateCharacterStats(ctx, &engine.CalculateCharacterStatsInput{
    Character: characterData, // Just data
})
// output has AC, proficiency bonus, etc.
```

## Consequences

### Positive
- **Clear separation of concerns**: Storage vs game rules
- **rpg-api stays simple**: Just an API and storage layer
- **rpg-toolkit owns complexity**: All D&D rules in one place
- **Easy to swap rules**: Different toolkit = different game system
- **API stability**: Data structures change less than game rules

### Negative
- **Not traditional DDD**: Entities without behavior feels wrong at first
- **More interfaces**: Need engine interface for all calculations
- **Data duplication**: Similar structures in both projects
- **No rich models in API**: Can't do `character.CalculateAC()`

### Neutral
- **Orchestrators coordinate**: They use engine for calculations, repos for storage
- **Two mental models**: Data model (api) vs domain model (toolkit)

## Examples

### What NOT to do in rpg-api:
```go
// ❌ BAD: Entities should not have behavior
func (c *Character) LevelUp() {
    c.Level++
    c.ExperiencePoints = 0
    // Don't do this!
}

// ❌ BAD: No calculations on entities
func (c *Character) GetModifier(ability int32) int32 {
    return (ability - 10) / 2
}
```

### What TO do:
```go
// ✅ GOOD: Orchestrator uses engine for logic
func (o *Orchestrator) LevelUpCharacter(ctx context.Context, input *LevelUpInput) (*LevelUpOutput, error) {
    // Get character data
    char, err := o.characterRepo.Get(ctx, input.CharacterID)
    
    // Use ENGINE for level up logic
    result, err := o.engine.CalculateLevelUp(ctx, &engine.LevelUpInput{
        Character: char,
    })
    
    // Update data based on engine results
    char.Level = result.NewLevel
    char.CurrentHP = result.NewMaxHP
    
    // Save data
    err = o.characterRepo.Update(ctx, char)
}
```

## References

- Original architecture decision to separate api from toolkit
- Similar pattern in many microservice architectures
- "Anemic Domain Model" anti-pattern (which this intentionally is)