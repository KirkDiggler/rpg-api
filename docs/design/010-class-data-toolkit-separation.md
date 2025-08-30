# Design: Separating Toolkit Requirements from External API Data

## Status
Proposed

## Context

Currently, `ListClasses` in rpg-api relies entirely on the external client (dnd5e API) to populate both:
1. Game mechanics data (`class.Data`)
2. UI/flavor text (`ClassUIData`)

However, rpg-toolkit already has a robust choice requirements system that defines what choices each class needs, but it lacks the "database" of class information (hit dice, proficiencies, etc.).

## Current Data Flow

```
ListClasses() 
  → external.GetClassData() 
    → Fetches from dnd5e API
    → Converts to class.Data (mechanics)
    → Extracts ClassUIData (descriptions)
  → Returns ClassListItem{ClassData, UIData}
  → Handler converts to proto.ClassInfo
```

## Proto Requirements Analysis

The `ClassInfo` proto message requires:

### Basic Info (Lines 848-851)
- `id`, `name`, `description`
- **Current source**: `class.Data` + `ClassUIData`
- **Toolkit has**: Structure only, no data

### Mechanical Info (Lines 853-854)
- `hit_die` (e.g., "1d10")
- `primary_abilities` (which abilities are important)
- **Current source**: `class.Data.HitDice`, `class.Data.SavingThrows` (used as proxy)
- **Toolkit has**: Structure only

### Proficiencies (Lines 856-860)
- `armor_proficiencies`, `weapon_proficiencies`, `tool_proficiencies`
- `saving_throw_proficiencies`
- **Current source**: `class.Data` fields
- **Toolkit has**: Structure only

### Skills (Lines 863-864)
- `skill_choices_count` (how many to choose)
- `available_skills` (which skills can be chosen)
- **Current source**: `class.Data.SkillProficiencyCount`, `class.Data.SkillOptions`
- **Toolkit has**: Via `choices.GetClassRequirements()` → `Requirements.Skills`

### Choices (Line 876)
- All character creation choices
- **Current source**: Manually constructed from `class.Data`
- **Toolkit has**: Complete via `choices.GetClassRequirements()`

## The Mismatch

### What Toolkit Provides
```go
choices.GetClassRequirements(classes.Fighter, 1) returns:
- Skills: {Count: 2, Options: [...], Label: "Choose 2 skills"}
- Equipment: [{Choose: 1, Options: [...], Label: "Choose armor"}]
- FightingStyle: {Options: [...], Label: "Choose a fighting style"}
```

### What Proto Needs
```protobuf
ClassInfo {
  hit_die: "1d10"           // NOT in Requirements
  armor_proficiencies: [...]  // NOT in Requirements
  weapon_proficiencies: [...] // NOT in Requirements
  saving_throws: [...]        // NOT in Requirements
  choices: [...]              // YES in Requirements
}
```

## Decision

### Option 1: Hybrid Approach (Recommended)
Use toolkit for what it has (choices/requirements) and external API for what it doesn't (static class data).

```go
type ClassListItem struct {
    // Static data from external API or hardcoded
    ClassID      classes.Class
    HitDice      int
    Proficiencies struct {
        Armor   []string
        Weapons []string
        Tools   []string
    }
    SavingThrows []abilities.Ability
    
    // Choice requirements from toolkit
    Requirements *choices.Requirements
    
    // UI text from external API
    UIData       *external.ClassUIData
}
```

### Option 2: Full Toolkit
Add complete class data to toolkit (significant effort, duplicates dnd5e API).

### Option 3: Keep Current
Continue using external API for everything (misses toolkit's choice validation).

## Implementation Plan

### Phase 1: Define Static Class Data
Create a minimal set of class data that won't change:

```go
// internal/entities/dnd5e/class_static.go
var ClassStaticData = map[classes.Class]ClassStatic{
    classes.Fighter: {
        HitDice: 10,
        SavingThrows: []abilities.Ability{abilities.STR, abilities.CON},
        ArmorProficiencies: []string{"all armor", "shields"},
        WeaponProficiencies: []string{"simple weapons", "martial weapons"},
    },
    // ... other classes
}
```

### Phase 2: Update ListClasses
```go
func (o *Orchestrator) ListClasses(ctx context.Context, input *ListClassesInput) (*ListClassesOutput, error) {
    classList := make([]ClassListItem, 0)
    
    for _, classID := range classes.All {
        // Get static data (hardcoded or cached)
        staticData := ClassStaticData[classID]
        
        // Get requirements from toolkit
        requirements := choices.GetClassRequirements(classID, 1)
        
        // Get UI data from external API (descriptions only)
        uiData, _ := o.externalClient.GetClassUIData(ctx, string(classID))
        
        classList = append(classList, ClassListItem{
            ClassID:       classID,
            StaticData:    staticData,
            Requirements:  requirements,
            UIData:        uiData,
        })
    }
    
    return &ListClassesOutput{Classes: classList}, nil
}
```

### Phase 3: Update Proto Conversion
```go
func convertClassListItemToProto(item ClassListItem) *dnd5ev1alpha1.ClassInfo {
    info := &dnd5ev1alpha1.ClassInfo{
        Id:          string(item.ClassID),
        Name:        item.ClassID.String(), // or from static data
        Description: item.UIData.Description,
        HitDie:      fmt.Sprintf("1d%d", item.StaticData.HitDice),
        
        // From static data
        ArmorProficiencies:       item.StaticData.ArmorProficiencies,
        WeaponProficiencies:      item.StaticData.WeaponProficiencies,
        SavingThrowProficiencies: convertAbilitiesToStrings(item.StaticData.SavingThrows),
        
        // From requirements
        SkillChoicesCount: int32(item.Requirements.Skills.Count),
        AvailableSkills:   convertSkillsToStrings(item.Requirements.Skills.Options),
        
        // Convert requirements to proto choices
        Choices: convertRequirementsToProtoChoices(item.Requirements),
    }
    return info
}
```

## Consequences

### Positive
- Clear separation: toolkit owns choices, static data is separate, API provides text
- Leverages toolkit's validated choice system
- Reduces dependency on external API for mechanics
- Single source of truth for choices

### Negative
- Need to maintain static class data (but it rarely changes)
- More complex than current approach
- Requires careful mapping between Requirements and proto Choices

### Neutral
- External API still needed for descriptions/flavor text
- Proto structure remains unchanged

## Notes

The key insight is that toolkit's `Requirements` describes what needs to be chosen, while `class.Data` describes static properties. These are fundamentally different types of information that shouldn't be conflated.
