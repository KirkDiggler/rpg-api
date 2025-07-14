# ADR-004: Repository-Owned ID and Timestamp Generation

## Status
Accepted

## Context
During implementation of Redis repositories for Character and CharacterDraft entities, we discovered that ID generation and timestamp management was happening in the orchestrator layer. This raised questions about the appropriate separation of concerns:

```go
// Current pattern in orchestrator
draft.ID = fmt.Sprintf("%s_%d", input.PlayerID, time.Now().UnixNano())
draft.CreatedAt = time.Now().Unix()
draft.UpdatedAt = time.Now().Unix()
draft.ExpiresAt = time.Now().Add(24 * time.Hour).Unix()
```

This approach has several issues:
1. Different orchestrators might generate IDs differently
2. Storage backends have their own ID generation strategies (PostgreSQL SERIAL, MongoDB ObjectID, etc.)
3. Timestamp generation is duplicated across orchestrators
4. Testing requires mocking time and ID generation at the orchestrator level

## Decision

**Repositories are responsible for ALL data persistence concerns**, including:
1. ID generation
2. CreatedAt/UpdatedAt timestamp management
3. Any other storage-specific metadata

### Implementation Pattern

```go
type Repository interface {
    Create(ctx context.Context, input CreateInput) (*CreateOutput, error)
}

type CreateInput struct {
    Draft *CharacterDraft  // No ID or timestamps required
}

type CreateOutput struct {
    Draft *CharacterDraft  // Has ID and timestamps populated
}

// Repository implementation
func (r *RedisRepository) Create(ctx context.Context, input CreateInput) (*CreateOutput, error) {
    draft := input.Draft
    
    // Repository generates ID
    draft.ID = r.idGenerator.Generate()
    
    // Repository sets timestamps
    now := r.clock.Now()
    draft.CreatedAt = now.Unix()
    draft.UpdatedAt = now.Unix()
    
    // Storage-specific concerns (like TTL)
    if draft.ExpiresAt == 0 {
        draft.ExpiresAt = now.Add(24 * time.Hour).Unix()
    }
    
    // ... persist and return
}
```

### Dependency Injection

Repositories receive Clock and IDGenerator interfaces to maintain testability:

```go
type Config struct {
    Client      redis.Client
    IDGenerator IDGenerator
    Clock       Clock  // Clock is a general utility, not repository-specific
}

func NewRedisRepository(cfg *Config) Repository {
    return &redisRepository{
        client:      cfg.Client,
        idGenerator: cfg.IDGenerator,
        clock:       cfg.Clock,
    }
}
```

Note: Clock is a general utility that should be available throughout the application, not just in repositories. It's needed in:
- Orchestrators for business logic time comparisons
- Services for time-based calculations  
- Middleware for request timing
- Background workers for scheduling

The Clock interface should live in a shared utilities package (e.g., `internal/pkg/clock`) and be injected wherever time operations are needed.

## Consequences

### Positive
1. **Storage Abstraction**: Orchestrators don't know or care about ID formats
2. **Consistency**: One place generates IDs per entity type
3. **Flexibility**: Easy to change ID strategies (UUID, ULID, snowflake, etc.)
4. **Natural Mapping**: Aligns with how databases work (auto-increment, generated columns)
5. **Testability**: Repository tests control ID generation deterministically

### Negative
1. **Less Control**: Orchestrators can't customize ID format for specific use cases
2. **Repository Complexity**: Repositories need more dependencies
3. **Migration Path**: Existing code needs refactoring

### Mitigation
For cases requiring specific ID formats, repositories can accept optional IDs:

```go
if input.CustomID != "" {
    draft.ID = input.CustomID  // Use provided ID
} else {
    draft.ID = r.idGenerator.Generate()  // Generate new ID
}
```

## Alternatives Considered

### 1. Orchestrator-Owned IDs (Current)
- ❌ Multiple orchestrators might generate differently
- ❌ Couples business logic to storage concerns
- ❌ Makes switching storage backends harder

### 2. Domain Model Methods
```go
func (d *CharacterDraft) GenerateID() {
    d.ID = fmt.Sprintf("draft_%d", time.Now().UnixNano())
}
```
- ❌ Domain models shouldn't have dependencies
- ❌ Hard to test
- ❌ Violates single responsibility

### 3. Separate ID Service
- ❌ Over-engineering for this use case
- ❌ Adds network calls or complexity
- ✅ Could be useful for distributed ID generation (future)

## References
- Issue #30: Inject clock and ID generator for deterministic testing
- Similar pattern in rpg-toolkit's repository layer
- Industry standard in ORMs (Hibernate, ActiveRecord, etc.)
