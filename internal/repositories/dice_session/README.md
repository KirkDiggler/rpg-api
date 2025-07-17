# Dice Session Repository

The dice session repository provides storage interface and types for managing dice roll sessions with TTL support. Sessions group related dice rolls by entity and context, enabling complex rolling workflows like character creation.

## Core Concepts

### DiceSession
A session represents a collection of dice rolls grouped by entity and context:

```go
type DiceSession struct {
    EntityID  string     // Owner of the rolls (e.g., "char_draft_123")
    Context   string     // Purpose grouping (e.g., "ability_scores") 
    Rolls     []DiceRoll // The actual dice rolls
    CreatedAt time.Time  // Session creation timestamp
    ExpiresAt time.Time  // Automatic expiration time
}
```

### DiceRoll
Individual dice roll results with complete audit trail:

```go
type DiceRoll struct {
    RollID      string  // Unique identifier within session
    Notation    string  // Dice notation used (e.g., "4d6")
    Dice        []int32 // Individual dice values rolled
    Total       int32   // Final result after modifiers
    Dropped     []int32 // Any dice dropped (e.g., lowest in 4d6)
    Description string  // Human-readable description
    DiceTotal   int32   // Raw dice total before modifiers
    Modifier    int32   // Applied modifier for final total
}
```

## Repository Interface

```go
type Repository interface {
    Create(ctx context.Context, input CreateInput) (*CreateOutput, error)
    Get(ctx context.Context, input GetInput) (*GetOutput, error) 
    Delete(ctx context.Context, input DeleteInput) (*DeleteOutput, error)
    Update(ctx context.Context, session *DiceSession) error
}
```

### Operations

#### Create
Creates a new dice session with TTL:
```go
input := CreateInput{
    EntityID: "char_draft_123",
    Context:  "ability_scores",
    Rolls:    []DiceRoll{...},
    TTL:      15 * time.Minute,
}
```

#### Get  
Retrieves session by entity and context:
```go
input := GetInput{
    EntityID: "char_draft_123", 
    Context:  "ability_scores",
}
```

#### Update
Replaces existing session (used for adding rolls):
```go
session.Rolls = append(session.Rolls, newRoll)
err := repo.Update(ctx, session)
```

#### Delete
Removes session and returns count of deleted rolls:
```go
output, err := repo.Delete(ctx, DeleteInput{
    EntityID: "char_draft_123",
    Context:  "ability_scores", 
})
// output.RollsDeleted contains count
```

## Session Identification

Sessions are uniquely identified by `(EntityID, Context)` tuples:

- **EntityID**: The entity performing rolls
  - Character drafts: `"char_draft_123"`
  - Active characters: `"char_456"`
  - Players: `"player_789"`

- **Context**: The purpose of the rolling session
  - Character creation: `"ability_scores"`
  - Combat rounds: `"combat_round_1"`
  - Skill checks: `"investigation_check"`
  - Damage rolls: `"weapon_damage"`

This design allows multiple concurrent sessions per entity for different purposes.

## TTL and Expiration

### Automatic Cleanup
- Sessions automatically expire based on TTL
- Default TTL: 15 minutes (configurable)
- Redis handles automatic cleanup
- No manual intervention required

### Expiration Handling
- `ExpiresAt` timestamp stored with session
- Repository implementations should validate expiration
- Expired sessions return "not found" errors
- UI can warn users of impending expiration

## Data Model Design

### Session Storage
Sessions are stored as atomic units containing all rolls:
- Enables efficient retrieval of complete rolling context
- Supports transactional updates when adding rolls
- Facilitates easy cleanup and expiration

### Roll Immutability
Individual rolls are immutable once created:
- Roll results never change after generation
- Updates only add new rolls to sessions
- Audit trail preserved for all dice results

### Flexible Assignment
Rolls contain IDs but no assigned purpose:
- Generated rolls can be assigned to any ability score
- UI can present flexible assignment interfaces
- Business logic handles assignment validation

## Implementation Patterns

### Input/Output Types
All operations use dedicated Input/Output types:
- Clear contracts for repository operations
- Easy to extend without breaking interfaces
- Consistent with rpg-api patterns

### Error Handling
Repository returns standard errors:
- `ErrNotFound` for missing sessions
- `ErrExpired` for TTL exceeded
- Wrapped storage errors with context

### Mock Generation
Interface includes mock generation:
```go
//go:generate mockgen -destination=mock/mock_repository.go -package=dice_sessionmock
```

## Usage Examples

### Character Creation Workflow
```go
// 1. Create session with ability score rolls
createOutput, err := repo.Create(ctx, CreateInput{
    EntityID: "char_draft_123",
    Context:  "ability_scores",
    Rolls:    generateAbilityScoreRolls(),
    TTL:      15 * time.Minute,
})

// 2. Retrieve for assignment in UI
getOutput, err := repo.Get(ctx, GetInput{
    EntityID: "char_draft_123",
    Context:  "ability_scores",
})

// 3. Clean up after character finalized
deleteOutput, err := repo.Delete(ctx, DeleteInput{
    EntityID: "char_draft_123", 
    Context:  "ability_scores",
})
```

### Combat Rolling Session
```go
// Track damage rolls for a combat round
session := &DiceSession{
    EntityID: "char_456",
    Context:  "combat_round_5",
    Rolls: []DiceRoll{
        {RollID: "atk1", Notation: "1d20+5", Total: 18},
        {RollID: "dmg1", Notation: "2d6+3", Total: 11},
    },
}
err := repo.Update(ctx, session)
```

## Redis Implementation Notes

The Redis implementation should:
- Use composite keys: `dice_session:{entity_id}:{context}`
- Leverage Redis TTL for automatic expiration
- Store sessions as JSON for human readability
- Support atomic updates for concurrent access
- Handle connection failures gracefully

## Testing Strategy

### Unit Tests
- Mock repository for orchestrator testing
- Test all Input/Output type combinations
- Validate error conditions and edge cases

### Integration Tests  
- Real Redis instance (or miniredis)
- TTL expiration behavior
- Concurrent access patterns
- Storage and retrieval round-trips

### Repository Tests
- Implementation-specific test suite
- TTL behavior validation
- Error condition handling
- Performance characteristics