# Journey: Implementing Redis Repositories

## The Problem

After implementing error handling, we moved on to the repository layer. We found ourselves with two nearly identical repository interfaces:
- `CharacterRepository` 
- `CharacterDraftRepository`

Both had the same CRUD operations, list methods, and player/session lookups. The only difference was that drafts had a `DeleteExpired()` method for cleanup.

## Initial Thoughts

My first instinct was "this is duplication - we should unify them!" But then I started thinking about the different use cases:

1. **Drafts are temporary** - They expire after a period (probably 24 hours)
2. **Characters are permanent** - They live as long as the player wants them
3. **Drafts can be partial** - Missing required fields during creation
4. **Characters must be complete** - All fields validated and filled

## Options Explored

### Option 1: Generic Repository Pattern
```go
type Repository[T any] interface {
    Create(ctx context.Context, entity T) error
    // ... etc
}
```

This seemed elegant at first, but Go's generics have limitations:
- Can't easily add type-specific methods (like DeleteExpired)
- Interface constraints get messy
- We'd need runtime type checking for special operations

### Option 2: Unified Repository
One repository handling both types:
```go
type CharacterRepository interface {
    CreateDraft(...) error
    CreateCharacter(...) error
    // ... double the methods
}
```

This felt wrong immediately:
- Violates single responsibility
- Bloated interface
- Confusing for consumers

### Option 3: Keep Them Separate (But Smart)
This is what we chose. Keep the interfaces separate but share implementation through composition.

## The Solution

We created a base Redis repository that handles common operations:
- CRUD with proper error handling
- List management (using Redis sets for indexes)
- Batch operations
- TTL support for drafts

Then each specific repository embeds this base and adds its unique methods.

## Implementation Details

The base repository uses generics internally (where they shine):
```go
type BaseRepository[T any] struct {
    client redis.UniversalClient
    logger *zap.Logger
    prefix string
    ttl    time.Duration
}
```

Key decisions:
1. **Prefix-based keys**: `character:123`, `draft:456`
2. **Set-based indexes**: `character:list:player:abc`
3. **JSON serialization**: Simple and debuggable
4. **TTL support**: Built-in for drafts

## Lessons Learned

1. **Don't over-engineer**: Our first instinct to unify everything would have made things worse
2. **Use composition wisely**: Share implementation, not interfaces
3. **Respect domain boundaries**: Drafts and characters are different concepts
4. **Generics have their place**: Internal implementation, not public APIs

## What's Next

Now we need to implement the actual Character and CharacterDraft repositories using this base. They'll be much simpler - mostly just wiring and adding any type-specific methods.

## What We Actually Did

After discussing with Kirk, we went with Option 2 - simple, independent implementations:
- Each repository is self-contained
- Using slog for logging (Go's standard structured logger)
- Redis sets for indexing by player/session IDs
- CharacterDraft adds TTL support and expiration tracking
- No shared base class - we can add complexity later if needed

The implementations are straightforward and easy to understand. If we find ourselves copying too much code later, we can refactor then.

## References
- [Character Redis Repository](../../internal/repositories/character/redis.go)
- [CharacterDraft Redis Repository](../../internal/repositories/character_draft/redis.go)
