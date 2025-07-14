# Engine Package

The engine package provides the integration layer between rpg-api and rpg-toolkit. It acts as an adapter, converting between our simple data models and rpg-toolkit's rich game objects.

## Core Principle

**"rpg-api stores data. rpg-toolkit handles rules."**

This package serves as the boundary where game mechanics meet data orchestration:
- **rpg-api side**: Simple entities (data), workflow orchestration, persistence  
- **rpg-toolkit side**: Game rules, validation, calculations
- **Engine adapter**: Translates between the two, manages integration

## Architecture

```
gRPC Handlers → Orchestrators → Engine Interface → Engine Adapter → rpg-toolkit
   (API)         (workflow)        (stable)         (converts)      (game rules)
```

## Current Focus: Character Creation

The initial implementation focuses on character creation validation and stat calculation, supporting all official D&D 5e races and classes.

## Example Usage

```go
// In a character creation orchestrator
func (o *CharacterOrchestrator) ValidateRace(ctx context.Context, input *ValidateRaceInput) (*ValidateRaceOutput, error) {
    // Use engine for game rules validation
    result, err := o.engine.ValidateRaceChoice(ctx, &engine.ValidateRaceChoiceInput{
        RaceID:    input.RaceID,
        SubraceID: input.SubraceID,
    })
    if err != nil {
        return nil, err
    }
    
    // Return validation result
    return &ValidateRaceOutput{
        IsValid:     result.IsValid,
        Errors:      result.Errors,
        RaceTraits:  result.RaceTraits,
        AbilityMods: result.AbilityMods,
    }, nil
}
```

## Current Engine Interface

The engine interface focuses on character creation validation and calculations:

```go
type Engine interface {
    // Character validation and calculations
    ValidateCharacterDraft(ctx context.Context, input *ValidateCharacterDraftInput) (*ValidateCharacterDraftOutput, error)
    CalculateCharacterStats(ctx context.Context, input *CalculateCharacterStatsInput) (*CalculateCharacterStatsOutput, error)

    // Race and class validation
    ValidateRaceChoice(ctx context.Context, input *ValidateRaceChoiceInput) (*ValidateRaceChoiceOutput, error)
    ValidateClassChoice(ctx context.Context, input *ValidateClassChoiceInput) (*ValidateClassChoiceOutput, error)

    // Ability score validation
    ValidateAbilityScores(ctx context.Context, input *ValidateAbilityScoresInput) (*ValidateAbilityScoresOutput, error)

    // Skill validation
    ValidateSkillChoices(ctx context.Context, input *ValidateSkillChoicesInput) (*ValidateSkillChoicesOutput, error)
    GetAvailableSkills(ctx context.Context, input *GetAvailableSkillsInput) (*GetAvailableSkillsOutput, error)

    // Background validation
    ValidateBackgroundChoice(ctx context.Context, input *ValidateBackgroundChoiceInput) (*ValidateBackgroundChoiceOutput, error)

    // Utility methods
    CalculateProficiencyBonus(level int32) int32
    CalculateAbilityModifier(score int32) int32
}
```

## Entity Integration Pattern

### rpg-toolkit Entity Wrappers

rpg-toolkit uses a `core.Entity` interface that requires `GetID()` and `GetType()` methods. We create wrapper types around our existing entities:

```go
// Wrapper for Character
type CharacterEntity struct {
    *dnd5e.Character
}

func (c *CharacterEntity) GetID() string   { return c.Character.ID }
func (c *CharacterEntity) GetType() string { return "character" }

// Wrapper for CharacterDraft  
type CharacterDraftEntity struct {
    *dnd5e.CharacterDraft
}

func (c *CharacterDraftEntity) GetID() string   { return c.CharacterDraft.ID }
func (c *CharacterDraftEntity) GetType() string { return "character_draft" }
```

### Our Current Entities

```go
// Simple, focused data structures
type Character struct {
    ID           string
    Name         string
    Level        int32
    RaceID       string
    ClassID      string
    BackgroundID string
    AbilityScores *AbilityScores
}

type CharacterDraft struct {
    ID              string
    Name            string
    RaceID          string
    ClassID         string
    BackgroundID    string
    AbilityScores   *AbilityScores
    SelectedSkills  []string
    Progress        CreationProgress // Bitflags for tracking completion
}
```

## Testing

The engine interface is designed to be easily mocked:

```go
type MockEngine struct {
    mock.Mock
}

func (m *MockEngine) RollAttack(ctx context.Context, input *RollAttackInput) (*RollAttackOutput, error) {
    args := m.Called(ctx, input)
    return args.Get(0).(*RollAttackOutput), args.Error(1)
}

// In tests
mockEngine := new(MockEngine)
mockEngine.On("RollAttack", mock.Anything, mock.Anything).Return(&RollAttackOutput{
    Total: 18,
    Hit:   true,
}, nil)
```

## Benefits

1. **Isolation**: rpg-toolkit changes don't ripple through the codebase
2. **Testability**: Orchestrators can be tested without the full game engine
3. **Flexibility**: Could swap game engines or support multiple rulesets
4. **Clarity**: Clear boundary between data and game mechanics
5. **Stability**: Engine interface remains stable even as toolkit evolves

## rpg-toolkit Integration Strategy

### Incremental Dependencies
- Add rpg-toolkit modules only as needed (`core`, `events`, `dice` first)
- Each module is independent with its own versioning
- Avoid unnecessary dependencies that increase complexity

### Adapter-Driven Development  
- Implement engine adapter methods to reveal gaps in rpg-toolkit
- Document missing toolkit components as GitHub issues
- Let real usage drive toolkit feature development

### Character Creation Focus
- Initial goal: Support all official D&D 5e races and classes
- Comprehensive validation and stat calculation
- Foundation for future rulebook data integration

## Implementation Phases

**Phase 1: Foundation** (#32, #33, #34)
- Add core dependencies (`core`, `events`, `dice`)
- Create entity wrappers for `core.Entity` compatibility
- Implement basic adapter structure with utility methods

**Phase 2: Core Validation** (#35, #36, #37)  
- Ability score validation (standard array, point buy, manual)
- Race and class validation with prerequisites
- Skill system integration with proficiency tracking

**Phase 3: Complete Integration** (#38, #39)
- Full character stat calculations (HP, AC, saves, skills)  
- Comprehensive character draft validation
- End-to-end character creation support

## Guidelines

- **Keep the Engine interface focused** on what orchestrators need
- **Don't expose rpg-toolkit types** outside this package
- **All conversions happen here**, not in orchestrators
- **Use Input/Output types** for all methods to maintain interface stability
- **Mock the Engine interface** in orchestrator tests, not rpg-toolkit
- **Document gaps found** during implementation as toolkit issues
- **Follow incremental approach** - don't over-engineer early
