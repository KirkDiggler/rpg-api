# Dice Orchestrator

The dice orchestrator implements the business logic for dice rolling operations in the RPG API. It provides both generic dice rolling and specialized D&D 5e ability score generation.

## Architecture

The orchestrator follows the established patterns:
- **Service Interface**: Defines contracts with Input/Output types
- **Implementation**: Handles business logic, validation, and coordination
- **Dependencies**: Repository for storage, rpg-toolkit for dice mechanics

## Service Interface

```go
type Service interface {
    // Generic dice rolling
    RollDice(ctx context.Context, input *RollDiceInput) (*RollDiceOutput, error)
    GetRollSession(ctx context.Context, input *GetRollSessionInput) (*GetRollSessionOutput, error)
    ClearRollSession(ctx context.Context, input *ClearRollSessionInput) (*ClearRollSessionOutput, error)

    // Specialized ability score rolling for character creation
    RollAbilityScores(ctx context.Context, input *RollAbilityScoresInput) (*RollAbilityScoresOutput, error)
}
```

## Key Features

### Generic Dice Rolling
- **Notation Support**: Parses standard dice notation (`2d6`, `4d6`, etc.)
- **Session Management**: Groups rolls by entity and context
- **TTL Support**: Configurable session expiration (default 15 minutes)
- **rpg-toolkit Integration**: Uses production-tested dice rolling engine

### Specialized Ability Score Rolling
- **Multiple Methods**: Standard (4d6 drop lowest), Classic (3d6), Heroic (4d6 reroll 1s)
- **Batch Generation**: Creates 6 ability score rolls in one operation
- **Automatic Grouping**: Uses `ability_scores` context for character creation
- **Drop Lowest Logic**: Implements 4d6 drop lowest with proper sorting

### Session Management
- **Entity-Context Grouping**: Sessions identified by `(entity_id, context)` pairs
- **Persistent Storage**: Redis-backed with automatic TTL
- **Roll Accumulation**: New rolls added to existing sessions
- **Cleanup Operations**: Manual and automatic session clearing

## Implementation Details

### Dice Notation Parsing
```go
// Supports patterns like "2d6", "4d6", "1d20"
var diceNotationRegex = regexp.MustCompile(`^(\d+)d(\d+)$`)
```

### Rolling Methods for Ability Scores
```go
const (
    MethodStandard = "4d6_drop_lowest"  // Default D&D 5e method
    MethodClassic  = "3d6"              // Original D&D method
    MethodHeroic   = "4d6_reroll_1s"    // Heroic character variant
)
```

### Default Configuration
```go
const (
    ContextAbilityScores = "ability_scores"
    DefaultSessionTTL    = 15 * time.Minute
    AbilityScoreNotation = "4d6"
)
```

## Usage Examples

### Generic Dice Rolling
```go
// Roll damage for a weapon attack
input := &RollDiceInput{
    EntityID:    "char_123",
    Context:     "combat_round_1", 
    Notation:    "2d6",
    Description: "Greatsword damage",
}
output, err := service.RollDice(ctx, input)
```

### Ability Score Generation
```go
// Generate ability scores for character creation
input := &RollAbilityScoresInput{
    EntityID: "char_draft_456",
    Method:   MethodStandard, // 4d6 drop lowest
}
output, err := service.RollAbilityScores(ctx, input)
// Returns 6 rolls with unique IDs for flexible assignment
```

### Session Retrieval
```go
// Get existing rolls for assignment to abilities
input := &GetRollSessionInput{
    EntityID: "char_draft_456",
    Context:  "ability_scores",
}
output, err := service.GetRollSession(ctx, input)
```

## rpg-toolkit Integration

The orchestrator uses rpg-toolkit for actual dice mechanics:

```go
// Create dice roll using rpg-toolkit
roll, err := dice.NewRoll(count, size)
total := roll.GetValue()
description := roll.GetDescription()
```

### Drop Lowest Implementation
- Parses individual dice from toolkit description
- Implements custom sorting for drop lowest logic
- Recalculates totals after dropping dice
- Maintains dropped dice for audit trails

## Error Handling

- **Validation Errors**: Invalid notation, missing required fields
- **Repository Errors**: Storage failures, session not found
- **Toolkit Errors**: Dice rolling failures
- **Context Propagation**: All errors wrapped with operation context

## Dependencies

- **dice_session.Repository**: Storage interface for sessions
- **idgen.Generator**: Unique ID generation for rolls
- **rpg-toolkit/dice**: Core dice rolling mechanics
- **Internal errors**: Standardized error handling

## Testing Approach

- **Mock Dependencies**: Repository and ID generator mocked
- **Real Toolkit**: Uses actual rpg-toolkit for dice mechanics
- **Session Scenarios**: Tests creation, retrieval, updates, and cleanup
- **Ability Score Methods**: Validates all rolling methods and drop logic

## Thread Safety

The orchestrator is stateless and thread-safe:
- No shared mutable state
- All state stored in repository
- ID generation through injected generator
- Safe for concurrent use

## Performance Considerations

- **Session Reuse**: Existing sessions updated rather than recreated
- **Batch Operations**: Ability scores generated in single repository call
- **TTL Management**: Redis handles automatic session cleanup
- **Minimal Allocations**: Efficient slice operations for dice manipulation
