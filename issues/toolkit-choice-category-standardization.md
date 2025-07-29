# Standardize Choice Category Names Between API and Toolkit

## Problem Statement

The rpg-api and rpg-toolkit have a fundamental mismatch in how they handle character creation choices:

1. **API behavior**: Stores choices with dynamic category names like `"class_fighter_proficiencies_1"` or `"race_human_language_1"`
2. **Toolkit expectation**: Expects standard category names from `shared.ChoiceCategory` constants like `"skills"` or `"languages"`

This mismatch prevents the toolkit from properly processing character choices into skills, languages, and proficiencies.

## Current State

### API Storage Pattern
When the API receives choices from the frontend, it stores them like:
```go
// In UpdateClass method
choiceKey := shared.ChoiceCategory(fmt.Sprintf("class_%s", choice.ChoiceID))
updatedData.Choices[choiceKey] = choice.SelectedKeys

// Results in: "class_fighter_proficiencies_1" -> ["athletics", "intimidation"]
```

### Toolkit Expectation
The toolkit's `draft.go` expects standard categories:
```go
// In compileCharacter method
if skills, ok := d.Choices[shared.ChoiceSkills].([]string); ok {
    // Process skills...
}
if languages, ok := d.Choices[shared.ChoiceLanguages].([]string); ok {
    // Process languages...
}
```

### Toolkit's ChoiceData Structure
The toolkit already has the perfect structure for tracking choice sources:
```go
type ChoiceData struct {
    Category  string `json:"category"`   // Should be "skills", "languages", etc.
    Source    string `json:"source"`     // "race", "class", "background"
    Selection any    `json:"selection"`  // The actual choices made
}
```

## Root Cause

The original design goal was to track which specific choice (e.g., "fighter_proficiencies_1") a skill came from, but this was implemented at the wrong layer. The API created dynamic category names when it should have used standard categories and tracked the specific choice ID separately.

## Proposed Solution

### Option 1: Fix at Toolkit Level (Recommended)

Enhance the toolkit's `ChoiceData` structure to include the specific choice ID:

```go
type ChoiceData struct {
    Category  string `json:"category"`   // Standard: "skills", "languages", etc.
    Source    string `json:"source"`     // "race", "class", "background"
    ChoiceID  string `json:"choice_id"`  // Specific: "fighter_proficiencies_1"
    Selection any    `json:"selection"`  // The actual choices made
}
```

Benefits:
- All toolkit users benefit from better choice tracking
- API can properly map choices without complex translation logic
- Frontend can still show which specific choice was made
- Maintains backward compatibility with existing standard categories

### Option 2: Fix at API Level Only

Keep current toolkit structure but change how API stores choices:
- Use standard categories when storing in draft choices map
- Track specific choice IDs in a separate field or structure

Drawbacks:
- Only fixes the problem for rpg-api
- Other toolkit users don't benefit
- Loses granular tracking of which specific choice was made

## Implementation Plan

### Phase 1: Toolkit Enhancement
1. Add `ChoiceID` field to `ChoiceData` struct
2. Update character creation logic to preserve choice IDs
3. Ensure backward compatibility (empty ChoiceID for old data)
4. Add tests for choice tracking

### Phase 2: API Updates
1. Remove dynamic category name generation
2. Use standard `shared.ChoiceCategory` constants
3. Set the new `ChoiceID` field when storing choices
4. Remove the temporary `mapChoicesToStandardCategories` workaround

### Phase 3: Validation
1. Test character creation flow end-to-end
2. Verify skills and languages are properly populated
3. Ensure frontend can still display choice sources

## Benefits

1. **Consistency**: API and toolkit speak the same language
2. **Transparency**: Can track exactly which choice provided which skill/language
3. **Extensibility**: Other systems can use the same pattern
4. **Simplicity**: No complex mapping logic needed

## Example

Instead of:
```go
// Current: Dynamic category
Choices["class_fighter_proficiencies_1"] = ["athletics", "intimidation"]
```

We'll have:
```go
// Proposed: Standard category with tracked source
Choices = []ChoiceData{
    {
        Category: "skills",
        Source:   "class",
        ChoiceID: "fighter_proficiencies_1",
        Selection: []string{"athletics", "intimidation"},
    },
}
```

## Related Issues

- rpg-toolkit #125: Refactor ChoiceData.Selection to use consistent type
- rpg-toolkit #87: Technical Debt: Clean up choice system implementation
- rpg-toolkit #106: Add language choice support to character creation
