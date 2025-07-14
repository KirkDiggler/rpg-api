# Repository Layer

This directory contains all repository implementations for the rpg-api.

## Core Principles

### Repository Responsibilities

Repositories are responsible for **ALL** data persistence concerns:

1. **ID Generation** - The repository generates IDs, not the orchestrator
2. **Timestamps** - CreatedAt/UpdatedAt are set by the repository
3. **Data Integrity** - Ensuring required fields are present
4. **Storage Details** - Connection management, serialization, transactions

### Why Repository-Owned IDs?

Consider different storage backends:
- **PostgreSQL**: Uses SERIAL or gen_random_uuid()
- **MongoDB**: Generates ObjectIDs
- **Redis**: We generate custom IDs
- **DynamoDB**: Partition/sort key generation

The orchestrator shouldn't know or care about these details. It asks the repository to "create a character" and gets back the created character with all fields populated.

### Anti-Patterns We Avoid

```go
// ❌ BAD: Orchestrator generates ID
draft.ID = fmt.Sprintf("%s_%d", playerID, time.Now().UnixNano())
draft.CreatedAt = time.Now().Unix()
repo.Create(ctx, draft)

// ✅ GOOD: Repository handles it
output, err := repo.Create(ctx, CreateInput{
    Draft: draft, // No ID or timestamps
})
createdDraft := output.Draft // Has ID and timestamps
```

### Benefits

1. **Consistency** - One place generates IDs per entity type
2. **Flexibility** - Easy to change ID strategies (UUID, ULID, etc.)
3. **Storage Agnostic** - Orchestrators work with any repository implementation
4. **Testability** - Repository tests control ID generation

### Implementation Pattern

```go
type RedisRepository struct {
    client      redis.Client
    idGenerator IDGenerator
    clock       Clock
}

func (r *RedisRepository) Create(ctx context.Context, input CreateInput) (*CreateOutput, error) {
    entity := input.Entity
    
    // Repository owns these concerns
    entity.ID = r.idGenerator.Generate()
    entity.CreatedAt = r.clock.Now().Unix()
    entity.UpdatedAt = entity.CreatedAt
    
    // Persist and return
    // ...
    
    return &CreateOutput{Entity: entity}, nil
}
```

## Repository Implementations

- `/character` - Character entity repository
- `/character_draft` - Character draft repository
- `/session` - Game session repository (future)
- `/encounter` - Combat encounter repository (future)

Each implementation follows the same patterns while handling storage-specific details internally.