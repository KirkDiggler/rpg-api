# Design: Using Toolkit Grants for Class Data

## Status
Proposed

## Context

The rpg-api currently fetches class data from the dnd5e API to populate basic class information like hit dice, saving throws, and proficiencies. However, this data is static D&D 5e rules that never change. The toolkit already has a "grants" pattern for races and backgrounds that provides automatic features.

## Decision

Add `classes/grants.go` to rpg-toolkit that provides all automatic class grants (things every member of that class gets, not choices).

## Implementation

### Toolkit Addition: classes/grants.go

```go
type AutomaticGrants struct {
    HitDice             int
    SavingThrows        []abilities.Ability  
    ArmorProficiencies  []proficiencies.Armor
    WeaponProficiencies []proficiencies.Weapon
    ToolProficiencies   []proficiencies.Tool
}

func GetAutomaticGrants(classID Class) AutomaticGrants
```

### Updated ListClasses in rpg-api

```go
func (o *Orchestrator) ListClasses(ctx context.Context, input *ListClassesInput) (*ListClassesOutput, error) {
    classList := make([]ClassListItem, 0)
    
    // Use toolkit's class list
    for classID := range classes.All {
        // Get automatic grants from toolkit (hit dice, proficiencies, etc.)
        grants := classes.GetAutomaticGrants(classID)
        
        // Get choice requirements from toolkit (skills to choose, equipment choices, etc.)
        requirements := choices.GetClassRequirements(classID, 1)
        
        // Optional: Get UI descriptions from external API if available
        var uiData *external.ClassUIData
        if o.externalClient != nil {
            uiData, _ = o.externalClient.GetClassUIData(ctx, string(classID))
        }
        
        classList = append(classList, ClassListItem{
            ClassID:      classID,
            Name:         string(classID), // or classID.String() if we add that
            Grants:       grants,
            Requirements: requirements,
            UIData:       uiData, // Optional flavor text
        })
    }
    
    return &ListClassesOutput{Classes: classList}, nil
}
```

### Handler Conversion to Proto

```go
func convertClassListItemToProto(item ClassListItem) *dnd5ev1alpha1.ClassInfo {
    info := &dnd5ev1alpha1.ClassInfo{
        Id:          string(item.ClassID),
        Name:        item.Name,
        Description: "", // From UIData if available
        
        // From Grants (automatic features)
        HitDie:                   fmt.Sprintf("1d%d", item.Grants.HitDice),
        ArmorProficiencies:       item.Grants.ArmorProficiencies,
        WeaponProficiencies:      item.Grants.WeaponProficiencies,
        ToolProficiencies:        item.Grants.ToolProficiencies,
        SavingThrowProficiencies: convertAbilitiesToStrings(item.Grants.SavingThrows),
        
        // From Requirements (player choices)
        SkillChoicesCount: int32(0), // Default
        AvailableSkills:   []string{},
    }
    
    // Add skill choices from requirements
    if item.Requirements != nil && item.Requirements.Skills != nil {
        info.SkillChoicesCount = int32(item.Requirements.Skills.Count)
        info.AvailableSkills = convertSkillsToStrings(item.Requirements.Skills.Options)
    }
    
    // Convert requirements to proto choices
    info.Choices = convertRequirementsToProtoChoices(item.Requirements)
    
    // Add UI data if available
    if item.UIData != nil {
        info.Description = item.UIData.Description
    }
    
    return info
}
```

## Benefits

1. **Single Source of Truth**: Toolkit owns all D&D 5e mechanics
2. **No External Dependencies**: Can run without dnd5e API
3. **Clear Separation**: 
   - Grants = automatic features everyone gets
   - Requirements = choices players must make
   - UIData = optional flavor text
4. **Cached by Design**: Static data in code, no API calls needed
5. **Type Safe**: Using toolkit enums throughout

## Migration Path

1. Add `grants.go` to toolkit (already created)
2. Update rpg-api ListClasses to use grants
3. Make external API optional for UI text only
4. Eventually move all static D&D data to toolkit

## Example Usage

```go
// Get everything about Fighter
grants := classes.GetAutomaticGrants(classes.Fighter)
requirements := choices.GetClassRequirements(classes.Fighter, 1)

// Fighter automatically gets:
// - 1d10 hit dice (from grants)
// - STR & CON saves (from grants)  
// - All armor & weapons (from grants)

// Fighter must choose:
// - 2 skills (from requirements)
// - Fighting style (from requirements)
// - Starting equipment (from requirements)
```

This completes the toolkit's ability to provide all mechanical data for character creation!