# Character Repository

## Goals

The Character repository manages persistent character data with the following design goals:

### 1. Full CRUD Operations
- `Create` - Creates a new character
- `Get` - Retrieves a character by ID
- `Update` - Updates an existing character
- `Delete` - Removes a character

### 2. Efficient Listing
- `ListByPlayerID` - Get all characters owned by a player
- `ListBySessionID` - Get all characters in a session

### 3. Index Management
Characters are indexed by:
- Player ID - for quick retrieval of a player's characters
- Session ID - for quick retrieval of session participants

## Implementation Notes

### Redis Key Structure
```
character:{id}                    # The character data (no TTL)
character:player:{playerID}       # Set of character IDs owned by player
character:session:{sessionID}     # Set of character IDs in session
```

### Key Design Decisions

1. **No TTL on Characters**: Unlike drafts, characters are permanent
2. **Set-based Indexes**: Using Redis sets for efficient membership operations
3. **Atomic Updates**: All index updates happen in transactions
4. **Lazy Cleanup**: Stale index entries are cleaned up during list operations

### Index Management

When a character's player or session changes:
1. Remove from old index
2. Add to new index
3. Both operations in a single transaction

This ensures indexes stay consistent even if:
- A character moves between sessions
- A character changes ownership
- Multiple updates happen concurrently

### Performance Considerations

- List operations fetch all characters individually
- For large character sets, consider pagination in the future
- Index cleanup is lazy to avoid performance impact

## Usage Example

```go
// Create a character
char := &dnd5e.Character{
    ID:        "char_123",
    PlayerID:  "player_456",
    SessionID: "session_789",
    Name:      "Thorin",
    Level:     5,
}
err := repo.Create(ctx, CreateInput{Character: char})

// List player's characters
output, err := repo.ListByPlayerID(ctx, ListByPlayerIDInput{
    PlayerID: "player_456",
})

// Update character (e.g., level up)
char.Level = 6
err = repo.Update(ctx, UpdateInput{Character: char})

// Move character to different session
char.SessionID = "session_999"
err = repo.Update(ctx, UpdateInput{Character: char})
```

## Future Considerations

1. **Pagination**: Add limit/offset to list operations
2. **Sorting**: Add ability to sort by name, level, etc.
3. **Filtering**: Add filters for class, race, level range
4. **Bulk Operations**: Support creating/updating multiple characters
5. **Change History**: Track character modifications over time

