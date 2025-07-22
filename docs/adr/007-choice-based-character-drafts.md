# ADR-007: Choice-Based Character Drafts

## Status
Proposed

## Context
Currently, character drafts store computed state - the result of applying choices rather than the choices themselves. For example, when a barbarian chooses equipment, we store:
```
equipment: [greataxe, handaxe, handaxe, explorer's pack]
```

This approach has several problems:
1. **Unclear provenance**: When a player changes class, we can't distinguish what came from the class vs. what the player chose
2. **Lost information**: Two handaxes become indistinguishable from one handaxe added twice
3. **Complex mutations**: Changing race or class requires complex logic to remove old grants and add new ones
4. **Validation complexity**: We validate the final state rather than validating choices in context

## Decision
Character drafts will store player choices, not computed state. The draft becomes a template of decisions that gets "compiled" into a full character during finalization.

### Draft Structure
```go
type CharacterDraft struct {
    // Identity - what the player chose to be
    PlayerID     string
    SessionID    string
    Name         string
    RaceID       dnd5e.Race
    SubraceID    dnd5e.Subrace  
    ClassID      dnd5e.Class
    BackgroundID dnd5e.Background
    
    // Choices - what the player selected from available options
    Choices      []ChoiceSelection  // Tracks all choices made
    
    // Metadata
    Progress     DraftProgress
    CreatedAt    int64
    UpdatedAt    int64
    ExpiresAt    int64
}

// ChoiceSelection represents a single choice made by the player
type ChoiceSelection struct {
    ChoiceID     string            // ID from the Choice in RaceInfo/ClassInfo
    ChoiceType   dnd5e.ChoiceType  // EQUIPMENT, SKILL, LANGUAGE, etc.
    Source       string            // "race", "class", "background"
    SelectedKeys []string          // The options selected (equipment keys, skill enums, etc.)
}
```

This leverages the existing proto Choice structure which already supports:
- Equipment choices with quantities (via CountedItemReference)
- Nested choices (e.g., "choose a martial weapon")
- Category references (e.g., "choose from martial-weapons")
- Multiple selections (via choose_count)

### Ability Score Choices
For ability score improvements (Half-Orc +2/+1, Human +1 all), we'll use a special choice type:
```go
type AbilityScoreSelection struct {
    ChoiceID  string  // "half_orc_ability_bonus"
    Choices   []AbilityScoreChoice
}

type AbilityScoreChoice struct {
    Ability dnd5e.Ability
    Bonus   int32
}
```

### Validation Approach
1. **On each update**: Validate that choices are valid for current race/class combination
2. **On finalization**: 
   - Verify all required choices are made
   - Apply automatic grants from race/class/background
   - Combine with choices to create final character

### Example: Fighter Creation
```yaml
# Class info defines available choices
ClassInfo:
  equipment_choices:
    - choice_id: "martial_weapon"
      options: ["greatsword", "longsword_shield", "any_martial"]
    - choice_id: "ranged_option"  
      options: ["light_crossbow_20_bolts", "handaxe_2"]
    - choice_id: "pack"
      options: ["dungeoneers_pack", "explorers_pack"]
  
  fighting_style_choices:
    level: 1
    count: 1
    options: [FEAT_FIGHTING_STYLE_ARCHERY, FEAT_FIGHTING_STYLE_DEFENSE, ...]

# Player's draft stores their selections
Draft:
  class_id: CLASS_FIGHTER
  choices:
    equipment_choices:
      - {equipment_key: "greatsword", quantity: 1}
      - {equipment_key: "handaxe", quantity: 2}
      - {equipment_key: "explorers_pack", quantity: 1}
    feat_choices:
      - FEAT_FIGHTING_STYLE_DEFENSE
```

## Consequences

### Positive
- **Clear separation**: Drafts store intent, characters store results
- **Clean mutations**: Change class = validate new choices, no complex removal logic
- **Better validation**: Can validate choices in context of available options
- **Preserves information**: Quantity and source of each choice is maintained
- **Simpler mental model**: Draft as a "build order" for a character

### Negative  
- **Migration complexity**: Existing drafts need conversion to new format
- **More computation**: Must resolve choices to final state during finalization
- **API changes**: Update endpoints need to accept choices, not final state

### Neutral
- **Aligns with architecture**: rpg-api stores data (choices), rpg-toolkit computes results
- **Similar to other systems**: Many character builders use this approach

## Implementation Plan

1. **Phase 1: Core Structure**
   - Add new ChoiceSelections to CharacterDraft proto
   - Update repository to handle new structure
   - Create migration for existing drafts

2. **Phase 2: Update Endpoints**
   - Modify update endpoints to accept choices
   - Add validation for choices against available options
   - Update finalization to compile choices into character

3. **Phase 3: Client Updates**
   - Update clients to send choices instead of computed state
   - Ensure backward compatibility during transition

## References
- Issue #[TBD]: Validation returns 200 with warnings instead of error
- Original discussion: Character drafts should store choices, not computed state