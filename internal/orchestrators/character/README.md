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

## Testing Strategy

- Mock all dependencies (repos, engine, external client)
- Test each method independently
- Test complex workflows end-to-end
- Test error scenarios and edge cases
- Verify proper delegation to dependencies