# Journey 006: External Client Interface Discovery

**Date**: July 14, 2025  
**Context**: Implementing race and class validation (#36) using external D&D 5e data

## The Process

While implementing race and class validation for the engine adapter, we needed rich D&D 5e data that we don't have internally. This led us through the standard outside-in development process of interface discovery.

## What We Discovered We Need

### From the D&D 5e SRD API
The external API (dnd5eapi.co) provides exactly what we need:

**Race Data**:
- Ability score bonuses (`+2 Dex` for Elf)
- Racial traits with descriptions (Darkvision, Fey Ancestry)
- Subrace relationships (High Elf belongs to Elf)
- Size, speed, languages, proficiencies

**Class Data**:
- Ability score prerequisites (`INT 13+` for Wizard multiclassing)
- Hit dice (`d6` for Wizard)
- Saving throw proficiencies
- Skill choice counts and available options
- Starting equipment and features

## The Outside-In Approach

### 1. Interface First
```go
// We added what we need to AdapterConfig, even though it doesn't exist yet
type AdapterConfig struct {
    EventBus       events.EventBus
    DiceRoller     dice.Roller
    ExternalClient external.Client  // <-- This doesn't exist yet!
}
```

### 2. Mock-Driven Implementation
Using generated mocks, we can implement full validation logic:
- Test with realistic D&D 5e data structures
- Validate ability score prerequisites
- Handle subrace validation
- Return proper traits and modifiers

### 3. Requirements Discovery
The mock usage reveals exactly what we need:
```go
// This tells us precisely what GetRaceData should return
mockClient.EXPECT().GetRaceData(ctx, "elf").Return(&external.RaceData{
    ID: "elf",
    Name: "Elf",
    AbilityBonuses: map[string]int32{"dexterity": 2},
    Traits: []string{"Darkvision", "Fey Ancestry", "Trance"},
    Subraces: []external.SubraceData{{ID: "high-elf", Name: "High Elf"}},
}, nil)
```

## Issues This Generates for dnd5e-api

Our mock usage naturally creates the backlog:

1. **Implement GetRaceData endpoint**
   - Map D&D 5e API race data to our structures
   - Include ability bonuses and trait descriptions
   - Handle subrace relationships

2. **Implement GetClassData endpoint**  
   - Map class data with prerequisites
   - Include proficiency choices and skill options
   - Handle multiclassing requirements

3. **Add comprehensive caching**
   - Cache race/class data (rarely changes)
   - Optimize for frequent character creation lookups

4. **Error handling and resilience**
   - Handle API failures gracefully
   - Implement retries and fallback strategies

## The Beauty of This Approach

### Unblocked Development
We can implement complete race/class validation now, not later. The engine adapter becomes fully functional with mocked data.

### Perfect Interface Design
Using the external API reveals exactly what data structures we need. No guessing, no over-engineering.

### Realistic Testing
Our tests use real D&D 5e data structures, so when the actual client is implemented, everything should just work.

### Clear Dependencies
We document exactly what external services we depend on and how we use them.

## The D&D 5e API Database

The underlying API (dnd5eapi.co) is remarkably well-designed:
- Complete SRD data as REST endpoints
- Consistent JSON structure
- Rich descriptions and interconnected references
- Everything needed for character creation validation

Having the entire Player's Handbook as a queryable database is game-changing for D&D applications. Our dnd5e-api wrapper adds caching and our preferred data structures on top.

## What We're Building

```go
// Engine validation with rich external data
result, err := adapter.ValidateRaceChoice(ctx, &engine.ValidateRaceChoiceInput{
    RaceID: "elf",
    SubraceID: "high-elf",
})

// Returns actual D&D 5e traits and ability modifiers
// result.RaceTraits = ["Darkvision", "Fey Ancestry", "Trance", "Keen Senses"]  
// result.AbilityMods = {"dexterity": 2, "intelligence": 1}
```

## Lessons Reinforced

1. **Always work outside-in**: Start with what you need, not what exists
2. **Interfaces over implementations**: Define what you need, let mocks fill the gap
3. **Usage drives requirements**: Mock usage reveals the perfect API contract
4. **Document the journey**: The discovery process is as valuable as the destination

This is how we always work - let the needs drive the interfaces, and let the interfaces drive the implementation requirements.

---

*This journey captures our standard outside-in development process and how it naturally generates precise requirements for external dependencies.*