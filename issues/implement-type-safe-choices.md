# Implement Type-Safe Choice System for Character Creation

## Summary
Update rpg-api to align with the new type-safe choice system implemented in rpg-toolkit PR #149. This replaces the generic `Choices map[shared.ChoiceCategory]any` with explicit typed fields for each choice type, providing compile-time safety and eliminating runtime type assertions.

## Background
rpg-toolkit PR #149 (https://github.com/KirkDiggler/rpg-toolkit/pull/149) introduced a major improvement to the character draft system by replacing the generic choices map with explicit typed fields. This change:
- Eliminates runtime type assertions
- Provides compile-time type safety
- Improves code clarity and IDE support
- Makes validation more straightforward

## Current State
The rpg-api currently uses a `ChoiceSelections []ChoiceSelection` approach in the `CharacterDraftData` and `CharacterDraft` entities, which needs to be updated to match the new rpg-toolkit structure.

## Implementation Tasks

### 1. Update Entity Models
Update `/internal/entities/dnd5e/draft_data.go` and `/internal/entities/dnd5e/character.go`:

Replace:
```go
ChoiceSelections []ChoiceSelection
```

With explicit typed fields matching rpg-toolkit:
```go
// Character creation choices
RaceChoice          RaceChoice           `json:"race_choice"`
ClassChoice         string               `json:"class_choice"`
BackgroundChoice    string               `json:"background_choice"`
AbilityScoreChoice  AbilityScores        `json:"ability_score_choice"`
SkillChoices        []string             `json:"skill_choices"`
LanguageChoices     []string             `json:"language_choices"`
FightingStyleChoice string               `json:"fighting_style_choice,omitempty"`
SpellChoices        []string             `json:"spell_choices,omitempty"`
CantripChoices      []string             `json:"cantrip_choices,omitempty"`
EquipmentChoices    []string             `json:"equipment_choices,omitempty"`
FeatChoices         []string             `json:"feat_choices,omitempty"`
```

### 2. Define RaceChoice Type
Create the `RaceChoice` type to handle race and optional subrace selection:
```go
type RaceChoice struct {
    RaceID    string `json:"race_id"`
    SubraceID string `json:"subrace_id,omitempty"`
}
```

### 3. Update Repository Layer
- Update character draft repository to handle the new structure
- Ensure Redis serialization/deserialization works with the new fields
- Update any existing draft migration logic if needed

### 4. Update Orchestrator Layer
- Update all draft update methods to work with typed fields:
  - `UpdateRace` - Use `RaceChoice` field
  - `UpdateClass` - Use `ClassChoice` field
  - `UpdateBackground` - Use `BackgroundChoice` field
  - `UpdateAbilityScores` - Use `AbilityScoreChoice` field
  - `UpdateSkills` - Use `SkillChoices` field
  - `UpdateChoices` - Handle remaining choice types (spells, cantrips, equipment, etc.)

### 5. Update Draft Validation
- Remove type assertions from validation logic
- Update validation to work directly with typed fields
- Add specific validation for each choice type

### 6. Implement FinalizeDraft Method
Complete the `FinalizeDraft` implementation in the character orchestrator:

```go
// FinalizeDraftInput
type FinalizeDraftInput struct {
    DraftID string
}

// FinalizeDraftOutput
type FinalizeDraftOutput struct {
    Character *dnd5e.Character
}
```

The implementation should:
1. Load the draft from repository
2. Validate the draft is complete (all required choices made)
3. Convert typed choice fields to rpg-toolkit Draft structure
4. Call rpg-toolkit's `Draft.ToCharacter()` method
5. Store the finalized character
6. Delete or archive the draft
7. Return the created character

### 7. Update Conversion Service
Update `/internal/services/conversion/draft_converter.go`:
- Remove references to `ChoiceSelections`
- Add conversion methods between rpg-api typed fields and rpg-toolkit typed fields

### 8. Update Handler Layer
- Update proto definitions if needed to support typed choices
- Update handler conversions to map between proto and internal types
- Ensure backward compatibility if proto changes are needed

### 9. Testing
- Update all existing tests to use new typed fields
- Add specific tests for each choice type
- Test draft finalization flow end-to-end
- Ensure validation provides clear error messages

## Benefits
1. **Type Safety**: No more runtime type assertions or casting
2. **Better Developer Experience**: IDE autocomplete and compile-time checks
3. **Clearer Code**: Explicit fields make the code self-documenting
4. **Easier Validation**: Each field can be validated independently
5. **Maintainability**: Adding new choice types is explicit and traceable

## Migration Considerations
- Existing drafts in Redis may need migration if format changes
- Consider backward compatibility during transition
- May need a migration script or dual-read capability temporarily

## Dependencies
- Requires rpg-toolkit with PR #149 merged
- May need to update rpg-api-protos if proto changes are required

## Testing Checklist
- [ ] Unit tests for new entity structures
- [ ] Repository layer tests with Redis
- [ ] Orchestrator tests for all update methods
- [ ] Draft validation tests
- [ ] FinalizeDraft implementation tests
- [ ] Integration tests for full character creation flow
- [ ] Handler layer tests if proto changes made

## References
- rpg-toolkit PR #149: https://github.com/KirkDiggler/rpg-toolkit/pull/149
- Original issue in rpg-toolkit: https://github.com/KirkDiggler/rpg-toolkit/issues/142
