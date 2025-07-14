# RPG-Toolkit Engine Adapter

This package contains the concrete implementation of the engine interface using rpg-toolkit modules.

## Responsibilities

**Primary Focus**: Character creation validation and stat calculation for D&D 5e

### What This Adapter Does
- ✅ **Validates character creation choices** using D&D 5e rules
- ✅ **Calculates derived stats** (HP, AC, saves, skills, etc.)
- ✅ **Enforces ability score rules** (standard array, point buy, manual)
- ✅ **Validates race/class prerequisites** and compatibility
- ✅ **Manages skill selection** with proficiency tracking
- ✅ **Provides utility calculations** (modifiers, proficiency bonus)

### What This Adapter Does NOT Do
- ❌ **Data persistence** - that's rpg-api's job via repositories
- ❌ **Workflow orchestration** - that's the orchestrator's job
- ❌ **API concerns** - that's the handler's job
- ❌ **Business logic beyond game rules** - that's the service layer's job

## Architecture

```
┌─────────────────────┐
│   Orchestrator      │
└──────────┬──────────┘
           │ uses Engine interface
┌──────────▼──────────┐     ┌─────────────────────┐
│   rpgtoolkit        │────▶│   rpg-toolkit       │
│   Adapter           │uses │   modules           │
│                     │     │   (core, events,    │
│   • Entity wrappers │     │    dice, mechanics) │
│   • Input/Output    │     │                     │
│   • Validation      │     │                     │
│   • Calculations    │     │                     │
└─────────────────────┘     └─────────────────────┘
```

## Implementation Pattern

### Constructor Pattern
```go
type AdapterConfig struct {
    EventBus   events.Bus
    DiceRoller dice.Roller
    // Add other rpg-toolkit components as needed
}

func (c *AdapterConfig) Validate() error {
    // Validate required dependencies
}

func NewAdapter(cfg *AdapterConfig) (*Adapter, error) {
    // Create adapter with validated config
}
```

### Entity Wrapper Pattern
```go
type Adapter struct {
    eventBus   events.Bus
    diceRoller dice.Roller
}

func (a *Adapter) wrapCharacterDraft(draft *dnd5e.CharacterDraft) *CharacterDraftEntity {
    return &CharacterDraftEntity{CharacterDraft: draft}
}
```

### Validation Pattern
```go
func (a *Adapter) ValidateRaceChoice(ctx context.Context, input *engine.ValidateRaceChoiceInput) (*engine.ValidateRaceChoiceOutput, error) {
    // 1. Load race data from toolkit (or identify missing data)
    // 2. Validate race/subrace combination
    // 3. Return traits and ability modifiers
    // 4. Document any missing toolkit features
}
```

## Dependencies Strategy

### Phase 1: Core Dependencies
```go
require (
    github.com/KirkDiggler/rpg-toolkit/core v0.x.x
    github.com/KirkDiggler/rpg-toolkit/events v0.x.x
    github.com/KirkDiggler/rpg-toolkit/dice v0.x.x
)
```

### Phase 2: Mechanics Dependencies (As Needed)
```go
require (
    github.com/KirkDiggler/rpg-toolkit/mechanics/proficiency v0.x.x
    // Add others as implementation requires them
)
```

## Gap Documentation Strategy

When implementing adapter methods, document missing rpg-toolkit features:

```go
func (a *Adapter) ValidateRaceChoice(ctx context.Context, input *engine.ValidateRaceChoiceInput) (*engine.ValidateRaceChoiceOutput, error) {
    // TODO(#XX): Need D&D 5e race data in rpg-toolkit
    // For now, return placeholder validation
    
    return &engine.ValidateRaceChoiceOutput{
        IsValid: true, // TODO: Real validation
        Errors:  []engine.ValidationError{},
        // TODO: Return actual race traits when available
    }, nil
}
```

## Testing Strategy

### Unit Tests
- Test adapter methods with mock rpg-toolkit components
- Focus on input/output conversion and validation logic
- Use testify suite pattern consistent with project standards

### Integration Tests  
- Test with real rpg-toolkit components when available
- Verify end-to-end character creation workflows
- Performance testing for real-time validation needs

### Mock Strategy
- Mock rpg-toolkit components, not the adapter itself
- Use interfaces from rpg-toolkit for mockability
- Keep mocks focused on specific functionality

## Error Handling

### Validation Errors
```go
type ValidationError struct {
    Field   string // "race_id", "ability_scores", etc.
    Message string // User-friendly error message
    Code    string // Machine-readable error code
}
```

### Error Categories
- **NotFound**: Race, class, or background doesn't exist
- **InvalidPrerequisite**: Ability scores don't meet class requirements  
- **InvalidChoice**: Skill selection exceeds allowed count
- **InvalidConfiguration**: Ability scores violate generation method rules

## Future Considerations

### Rulebook Integration
- Design adapter to eventually use rulebook data from rpg-toolkit
- Keep D&D 5e rules separate from adapter logic
- Plan for multiple ruleset support (5e, Pathfinder, etc.)

### Performance
- Character creation should validate in real-time (< 100ms)
- Cache frequently accessed rule data
- Minimize object allocations in hot paths

### Extensibility  
- Design for additional D&D 5e features (multiclassing, feats, etc.)
- Plan for combat mechanics integration later
- Consider event bus integration for advanced features

## Implementation Checklist

- [ ] Basic adapter structure with constructor pattern
- [ ] Entity wrappers for core.Entity compatibility
- [ ] Utility methods (ability modifier, proficiency bonus)
- [ ] Ability score validation (all three methods)
- [ ] Race and class validation with prerequisites
- [ ] Skill system integration with proficiency tracking
- [ ] Character stat calculations (HP, AC, saves, skills)
- [ ] Complete character draft validation
- [ ] Comprehensive test coverage
- [ ] Performance optimization
- [ ] Documentation of rpg-toolkit gaps
