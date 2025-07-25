# Character Orchestrator

Implements the `character.Service` interface for D&D 5e character creation and management workflows.

## Scope

**Bounded Context**: Character lifecycle from draft creation through finalization

**Business Flows**:
- Character draft creation and modification
- Section-by-section character building (race, class, background, etc.)
- Validation against D&D 5e rules
- Draft finalization into complete characters
- Character retrieval and management

## Dependencies

- **CharacterRepository**: Finalized character storage
- **CharacterDraftRepository**: Draft character storage  
- **Engine**: D&D 5e rules validation via rpg-toolkit
- **ExternalClient**: Race/class/background data from APIs

## Key Methods

### Draft Lifecycle
- `CreateDraft`: Start new character creation
- `GetDraft`/`ListDrafts`: Retrieve drafts in progress
- `DeleteDraft`: Clean up abandoned drafts

### Section Updates  
- `UpdateName`: Set character name
- `UpdateRace`: Apply race choice with ability modifiers
- `UpdateClass`: Apply class with skill/proficiency requirements
- `UpdateBackground`: Apply background with equipment/skills
- `UpdateAbilityScores`: Validate and apply ability scores
- `UpdateSkills`: Validate skill choices against class/background

### Validation & Finalization
- `ValidateDraft`: Check completeness and D&D 5e rules compliance
- `FinalizeDraft`: Convert valid draft to final character

### Character Operations
- `GetCharacter`/`ListCharacters`: Access finalized characters
- `DeleteCharacter`: Remove characters

## Validation Rules

**Must delegate to Engine for all D&D 5e rules**:
- Ability score requirements for classes
- Skill availability based on class/background
- Race/subrace combinations
- Equipment and proficiency rules

## Error Handling

- Wrap repository errors with context
- Return validation errors from engine unchanged
- Handle external API failures gracefully
- Provide clear error messages for API consumers

## Implementation Details

### Progress Tracking with Bitflags

Character creation progress is tracked using bitflags for efficiency:

```go
// Progress step bitflags
const (
    ProgressStepName          uint8 = 1 << iota // 1
    ProgressStepRace                             // 2
    ProgressStepClass                            // 4
    ProgressStepBackground                       // 8
    ProgressStepAbilityScores                    // 16
    ProgressStepSkills                           // 32
    ProgressStepLanguages                        // 64
)
```

The `CreationProgress` struct uses a single `uint8` field to track all completed steps:
- **Memory efficient**: 1 byte instead of 7 booleans
- **Fast operations**: Bitwise operations for checking/setting steps
- **Scalable**: Easy to add more steps up to 8 with uint8
- **Clean API**: Helper methods like `HasName()` for readability

Progress calculation uses bit counting to determine completion percentage.

## Testing Strategy

- Mock all dependencies (repos, engine, external client)
- Test each method independently
- Test complex workflows end-to-end
- Test error scenarios and edge cases
- Verify proper delegation to dependencies

## Architecture Pattern: Data vs Rich Objects

### Overview
The character orchestrator follows a clear separation between data transfer and game logic:

- **API Layer**: Works with data objects only (`character.Data`)
- **Orchestrator Layer**: Converts between data and rich toolkit objects as needed
- **Repository Layer**: Stores and retrieves data objects only

### Current Implementation (Character Creation)

For character creation and listing, we only need data objects:

```go
// API returns data
GetCharacter() -> character.Data
ListCharacters() -> []character.Data
FinalizeDraft() -> character.Data
```

### Future Game Actions Pattern

When implementing game actions (Attack, Cast Spell, etc.), the orchestrator will:

1. **Load Rich Objects**
   ```go
   character := o.loadCharacter(ctx, characterID)  // character.Data -> toolkit Character
   room := o.loadRoom(ctx, roomID)                  // Load spatial room
   monsters := o.loadMonsters(ctx, roomIDs)         // Load monster objects
   ```

2. **Execute Game Logic**
   ```go
   result := character.Attack(target)
   room.UpdatePositions(...)
   character.AddCondition(...)
   ```

3. **Persist Changes**
   ```go
   o.characterRepo.Update(ctx, character.ToData())
   o.monsterRepo.Update(ctx, monster.ToData())
   ```

### Helper Methods

The orchestrator provides internal helpers for data/object conversion:

- `loadFullCharacter(ctx, data)` - Converts character.Data to rich Character object
  - Fetches race, class, background data from external client
  - Calls `character.LoadCharacterFromData()`
  
- Future: `loadRoom()`, `loadMonster()`, etc.

### Repository Contract

Repositories work ONLY with data objects:
- Store `character.Data` (not `dnd5e.Character` entity)
- No knowledge of rich objects or game rules
- Simple JSON marshaling for persistence

### Session Management Note

The toolkit's `character.Data` doesn't include SessionID. This is managed at the orchestrator level through additional metadata or a separate session tracking system.

## Current Scope

This PR focuses on:
- Converting draft to character using toolkit's `ToCharacter()`
- Storing character data using toolkit's `character.Data` struct
- Loading character lists as data objects

Future work (separate issues):
- Implement game action methods (Attack, Cast, etc.)
- Add room/monster loading helpers
- Session management integration
