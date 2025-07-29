# Redesign API Choice Storage to Use Toolkit Standards

## Problem Statement

The rpg-api currently stores character creation choices in a way that's incompatible with the rpg-toolkit's expectations. This forces us to use a translation layer (`mapChoicesToStandardCategories`) that adds complexity and can lose information.

## Current Implementation Problems

### 1. Dynamic Category Names
```go
// Current approach in UpdateClass
choiceKey := shared.ChoiceCategory(fmt.Sprintf("class_%s", choice.ChoiceID))
updatedData.Choices[choiceKey] = choice.SelectedKeys
// Results in: map["class_fighter_proficiencies_1"] = ["athletics", "intimidation"]
```

### 2. Loss of Structure
The current approach flattens choices into a simple map, losing the rich structure that the toolkit's `ChoiceData` provides:
- Which source (race/class/background) provided the choice
- The specific choice ID for UI display
- The type of choice (skills/languages/equipment)

### 3. Incompatible with Toolkit
The toolkit expects standard categories but gets dynamic ones, causing character data to be missing skills, languages, and proficiencies.

## Proposed Solution

### Step 1: Adopt Toolkit's Choice Model

Instead of storing choices in the draft as `map[shared.ChoiceCategory]any`, store them as `[]ChoiceData`:

```go
// In draft storage
type DraftData struct {
    ID            string
    PlayerID      string
    Name          string
    Choices       []character.ChoiceData  // Changed from map
    ProgressFlags uint32
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

### Step 2: Update Choice Handlers

Change how we process choices in update methods:

```go
// UpdateClass method
for _, choice := range input.Choices {
    choiceData := character.ChoiceData{
        Category:  mapChoiceTypeToStandardCategory(choice.Type), // "skills", "languages", etc.
        Source:    "class",
        ChoiceID:  choice.ChoiceID,  // Preserves "fighter_proficiencies_1"
        Selection: choice.SelectedKeys,
    }
    draftData.Choices = append(draftData.Choices, choiceData)
}
```

### Step 3: Remove Translation Layer

Once we're storing choices properly, we can remove:
- `mapChoicesToStandardCategories` function
- Complex choice extraction logic in `convertDraftDataToCharacterDraft`
- Workarounds in finalization

## Benefits

1. **Direct Compatibility**: Choices work with toolkit without translation
2. **Richer Data**: Preserve all context about each choice
3. **Cleaner Code**: Remove complex mapping logic
4. **Better Debugging**: Can see exactly what choices were made and why

## Migration Strategy

### Phase 1: Dual Support
1. Add new choice storage alongside existing
2. Write to both formats during updates
3. Read from new format if available, fall back to old

### Phase 2: Migration
1. Add migration command to convert existing drafts
2. Update all read paths to use new format
3. Add deprecation warnings for old format

### Phase 3: Cleanup
1. Remove old format support
2. Remove translation functions
3. Update tests

## Implementation Details

### Required Changes

1. **Update Proto Definitions**
   - Ensure proto `CharacterDraft.choices` can represent the full ChoiceData structure
   - May need to add a proper Choice message type

2. **Repository Updates**
   - Modify how choices are serialized/deserialized in Redis
   - Ensure backward compatibility during migration

3. **Handler Updates**
   - Change all Update* methods to use new choice format
   - Update choice extraction logic in convertDraftDataToCharacterDraft

4. **Frontend Compatibility**
   - Ensure frontend can still display previous choices
   - May need to adjust how choices are sent to frontend

## Example Data Structure

### Before (Current)
```json
{
  "choices": {
    "ability_scores": {"strength": 15, "dexterity": 14, ...},
    "class_fighter_proficiencies_1": ["athletics", "intimidation"],
    "race_human_language_1": ["elvish"]
  }
}
```

### After (Proposed)
```json
{
  "choices": [
    {
      "category": "ability_scores",
      "source": "player",
      "choice_id": "",
      "selection": {"strength": 15, "dexterity": 14, ...}
    },
    {
      "category": "skills",
      "source": "class",
      "choice_id": "fighter_proficiencies_1",
      "selection": ["athletics", "intimidation"]
    },
    {
      "category": "languages",
      "source": "race",
      "choice_id": "human_language_1",
      "selection": ["elvish"]
    }
  ]
}
```

## Testing Requirements

1. **Unit Tests**
   - Test choice storage and retrieval
   - Test migration from old to new format
   - Test backward compatibility

2. **Integration Tests**
   - Full character creation flow
   - Verify skills/languages properly populated
   - Test with existing frontend

3. **Migration Tests**
   - Test converting existing drafts
   - Verify no data loss
   - Performance testing for bulk migration

## Dependencies

- Requires rpg-toolkit to implement enhanced ChoiceData structure (if adding ChoiceID field)
- May require proto updates depending on implementation approach
- Frontend may need minor adjustments for displaying choices

## Timeline

1. **Week 1**: Implement dual storage support
2. **Week 2**: Add migration tools and test thoroughly  
3. **Week 3**: Deploy with backward compatibility
4. **Week 4**: Complete migration and remove old code

## Related Issues

- #[toolkit-issue]: Standardize Choice Category Names Between API and Toolkit
- rpg-api-protos #[proto-issue]: Add proper Choice message type for character drafts
