# Character Draft Repository

## Goals

The Character Draft repository manages temporary character creation data with the following design goals:

### 1. Single Draft per Player
- Each player mapping points to one draft at a time
- Creating a new draft automatically replaces that player's existing draft
- Isolation assumes distinct draft IDs: a supplied ID colliding with another owner's
  record currently overwrites it and leaves both mappings pointing there (#1058)
- Replacement updates the player mapping and removes the previous draft

### 2. Simple Access Patterns
- `Create` - Creates or replaces the player's draft
- `Get` - Retrieves a draft by ID
- `GetByPlayerID` - Retrieves the player's single draft
- `Update` - Updates the existing draft
- `Delete` - Removes a draft (usually when finalized)

### 3. Automatic Expiration
- Drafts expire 24 hours after Create or the latest Update; reads do not refresh TTL
- Redis handles expiration automatically via TTL
- No background cleanup needed due to single-draft design

## Implementation Notes

### Why Single Draft?
Originally designed to support multiple drafts per player with index sets, but this created complexity:
- Stale index entries when drafts expire
- Need for cleanup patterns (janitor, lazy cleanup, etc.)
- Complex UX decisions about draft management

The single-draft approach eliminates these issues:
- Indexes have at most one entry per player
- Expired draft mappings are lazily removed on GetByPlayerID
- Clear UX: "Continue your character or start over?"
- Natural cleanup when creating new drafts

### Redis Key Structure
```
draft:{id}                    # The draft data (with TTL)
draft:player:{playerID}       # Points to the player's current draft ID
```

The player mapping has no TTL. GetByPlayerID removes it when the referenced draft
is missing; Delete removes both keys. Update refreshes the draft's TTL but does not
migrate the mapping if PlayerID changes. #1047 tests same-owner lifecycle behavior,
not ownership reassignment.

See [the method inventory](../../../docs/quality/repository-contracts.md) for populated
round trips, replacement/isolation, controlled expiry, malformed data, and failure tests.

Note: All keys are prefixed with `draft:` to group them together for easier management and potential scanning.

### Future Considerations
If we need multiple drafts per player later:
1. Add a draft limit (e.g., max 5)
2. Implement FIFO replacement when hitting the limit
3. Consider aggressive TTLs (2-4 hours instead of 24)

## Usage Example

```go
// Player starts character creation
draft := &dnd5e.CharacterDraft{
    PlayerID: "player123",
    // ... other fields
}

// This automatically replaces any existing draft
err := repo.Create(ctx, CreateInput{Draft: draft})

// Get the player's current draft
output, err := repo.GetByPlayerID(ctx, GetByPlayerIDInput{
    PlayerID: "player123",
})

// When character is finalized, draft is deleted
err = repo.Delete(ctx, DeleteInput{ID: draftID})
```

