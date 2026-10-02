# Character Draft Repository

## Goals

The Character Draft repository manages temporary character creation data with the following design goals:

### 1. Single Draft per (World, Player)
- Each `(world, player)` mapping points to one draft at a time
- Creating a new draft automatically replaces that player's existing draft **in that world only**
- Isolation assumes distinct draft IDs within a world: a supplied ID colliding with another owner's
  record in the same world currently overwrites it and leaves both mappings pointing there (#1058)
- Replacement updates the world-scoped player mapping and removes the previous draft

### 2. Simple Access Patterns
- `Create` - Creates or replaces the player's draft in the draft's world (`WorldID` required)
- `Get` - Retrieves a draft by world and ID
- `GetByPlayerID` - Retrieves the player's single draft in a world
- `Update` - Updates the existing draft; world/player ownership is immutable
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

The single-draft approach eliminates these issues. Each `(world, player)` pair owns at most one draft:
- Indexes have at most one entry per `(world, player)`
- Expired draft mappings are lazily removed on GetByPlayerID
- Clear UX: "Continue your character or start over?"
- Natural cleanup when creating new drafts

### Redis Key Structure
```
draft:{worldID}:{id}                     # The draft data (with TTL)
draft:player:{worldID}:{playerID}        # Points to the player's current draft ID
```

World IDs are canonical decimal guild-shaped identifiers; draft IDs never contain
the `:` delimiter. The ownerless `draft:{id}` and `draft:player:{playerID}` keys are
legacy and are never read as a fallback on a world-scoped miss.

The player mapping has no TTL. GetByPlayerID removes it when the referenced draft
is missing; Delete removes both keys. Update refreshes the draft's TTL but does not
migrate the mapping when PlayerID changes — and Update rejects a PlayerID change
with `InvalidArgument` because world and player ownership are immutable. #1047 tests
same-owner lifecycle behavior, not ownership reassignment.

GetByPlayerID also rejects (as `Internal` corruption) a mapping that resolves to a
draft owned by another player, rather than projecting or deleting it. A stored draft
whose `WorldID` disagrees with the world-scoped key is likewise `Internal` corruption.

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
// Player starts character creation in world 123456789012345678
draft := &entities.CharacterDraft{
    WorldID: "123456789012345678",
    Data: &dnd5e.DraftData{PlayerID: "player123"},
}

// This automatically replaces any existing draft for that (world, player)
err := repo.Create(ctx, CreateInput{Draft: draft})

// Get the player's current draft in that world
output, err := repo.GetByPlayerID(ctx, GetByPlayerIDInput{
    WorldID:  "123456789012345678",
    PlayerID: "player123",
})

// When character is finalized, draft is deleted
err = repo.Delete(ctx, DeleteInput{
    WorldID: "123456789012345678",
    ID:      draftID,
})
```

