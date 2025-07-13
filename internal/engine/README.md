# Engine Package

The engine package provides the integration layer between rpg-api and rpg-toolkit. It acts as an adapter, converting between our simple data models and rpg-toolkit's rich game objects.

## Purpose

This package serves as the boundary where game mechanics meet data orchestration:
- **rpg-api side**: Works with simple entities (just data)
- **rpg-toolkit side**: Leverages rich game objects and rules
- **Engine adapter**: Translates between the two

## Architecture

```
Orchestrators → Engine Interface → Engine Adapter → rpg-toolkit
                    (stable)         (converts)      (game rules)
```

## Example Usage

```go
// In an orchestrator
func (o *CombatOrchestrator) RollAttack(ctx context.Context, input *RollAttackInput) (*RollAttackOutput, error) {
    // Load simple data
    character, err := o.charRepo.Get(ctx, input.CharacterID)
    if err != nil {
        return nil, err
    }
    
    // Use engine for game mechanics
    result, err := o.engine.RollAttack(ctx, &engine.RollAttackInput{
        Character: character,
        Weapon:    input.Weapon,
        Target:    input.Target,
    })
    if err != nil {
        return nil, err
    }
    
    // Return result
    return &RollAttackOutput{
        Hit:    result.Total >= input.Target.AC,
        Damage: result.Damage,
    }, nil
}
```

## Key Interfaces

### Engine
The main interface that orchestrators depend on:
```go
type Engine interface {
    // Character operations
    CreateCharacter(ctx context.Context, input *CreateCharacterInput) (*CreateCharacterOutput, error)
    ValidateCharacter(ctx context.Context, input *ValidateCharacterInput) (*ValidateCharacterOutput, error)
    
    // Combat operations  
    RollAttack(ctx context.Context, input *RollAttackInput) (*RollAttackOutput, error)
    CalculateDamage(ctx context.Context, input *CalculateDamageInput) (*CalculateDamageOutput, error)
    
    // Dice operations
    Roll(ctx context.Context, input *RollInput) (*RollOutput, error)
    
    // Rulebook queries
    CalculateProficiencyBonus(level int) int
    CalculateAbilityModifier(score int) int
    GetSpellSlots(class string, level int) map[int]int
}
```

## Conversion Pattern

The adapter converts between our simple entities and toolkit's rich objects:

```go
// Our simple entity
type Character struct {
    ID         string
    Name       string
    Level      int
    RaceID     string
    ClassID    string
    BaseStats  Stats
}

// Convert to toolkit's rich character
func (e *engineAdapter) convertToToolkitCharacter(char *entities.Character) *toolkit.Character {
    return &toolkit.Character{
        Level: char.Level,
        Race:  e.toolkit.Races.Get(char.RaceID),
        Class: e.toolkit.Classes.Get(char.ClassID),
        Abilities: toolkit.AbilityScores{
            Strength:     char.BaseStats.Strength,
            Dexterity:    char.BaseStats.Dexterity,
            Constitution: char.BaseStats.Constitution,
            Intelligence: char.BaseStats.Intelligence,
            Wisdom:       char.BaseStats.Wisdom,
            Charisma:     char.BaseStats.Charisma,
        },
    }
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

## Guidelines

- Keep the Engine interface focused on what orchestrators need
- Don't expose rpg-toolkit types outside this package
- All conversions happen here, not in orchestrators
- Use Input/Output types for all methods
- Mock the Engine interface in orchestrator tests, not rpg-toolkit
