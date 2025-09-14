# Proto to Toolkit Converter Implementation Plan

## Overview
This document outlines the conversion strategy for handling character draft data between proto messages and the toolkit domain models, specifically focusing on equipment and choice selections.

## Key Type Mappings

### SelectionID Handling
- **Toolkit**: `type SelectionID = string` (simple string alias)
- **Proto**: `string` field in EquipmentList
- **Conversion**: Direct cast is possible: `shared.SelectionID(protoString)`

### Equipment Selection Flow

#### Proto → Toolkit (CreateDraft)
```pseudo
// Proto input
message ChoiceData {
  selection: {
    equipment: {
      equipment: ["longsword", "chain-mail", "explorer-pack"]
    }
  }
}

// Conversion logic
func convertProtoChoiceToToolkit(protoChoice *ChoiceData) choices.ChoiceData {
  toolkitChoice := choices.ChoiceData{
    Category: convertProtoCategoryToToolkit(protoChoice.Category),
    Source:   convertProtoSourceToToolkit(protoChoice.Source),
    ChoiceID: choices.ChoiceID(protoChoice.ChoiceId),
  }
  
  // Handle equipment selection
  if equipment := protoChoice.GetEquipment(); equipment != nil {
    selections := make([]shared.SelectionID, len(equipment.Equipment))
    for i, equip := range equipment.Equipment {
      // Direct cast since SelectionID is string alias
      selections[i] = shared.SelectionID(equip)
    }
    toolkitChoice.EquipmentSelection = selections
  }
  
  return toolkitChoice
}
```

#### Toolkit → Proto (GetDraft response)
```pseudo
// Toolkit data
type ChoiceData struct {
  EquipmentSelection: []shared.SelectionID{"longsword", "chain-mail"}
}

// Conversion logic  
func convertToolkitChoiceToProto(choice choices.ChoiceData) *proto.ChoiceData {
  protoChoice := &proto.ChoiceData{
    Category: convertCategoryToProto(choice.Category),
    Source:   convertSourceToProto(choice.Source),
    ChoiceId: string(choice.ChoiceID),
  }
  
  // Handle equipment selection
  if len(choice.EquipmentSelection) > 0 {
    equipment := make([]string, len(choice.EquipmentSelection))
    for i, sel := range choice.EquipmentSelection {
      // Direct string conversion since SelectionID is string alias
      equipment[i] = string(sel)
    }
    protoChoice.Selection = &proto.ChoiceData_Equipment{
      Equipment: &proto.EquipmentList{
        Equipment: equipment,
      },
    }
  }
  
  return protoChoice
}
```

## Selection Type Conversions

### Simple String Conversions (Direct Cast Possible)
These can be directly cast between proto string and toolkit type:
- `EquipmentSelection`: `[]shared.SelectionID` ↔ `[]string`
- `TraitSelection`: `[]string` ↔ `[]string`
- `NameSelection`: `*string` ↔ `string`

### Enum Conversions (Need Mapping Functions)
These require enum converter functions:

#### Skills
- **Toolkit**: `skills.Skill` (string alias, values: "acrobatics", "animal-handling", etc.)
- **Proto**: `Skill` enum (SKILL_ACROBATICS, SKILL_ANIMAL_HANDLING, etc.)
- **Converter needed**: Map kebab-case strings to UPPER_SNAKE enum values

#### Languages  
- **Toolkit**: `languages.Language` (string alias, values: "common", "elvish", etc.)
- **Proto**: `Language` enum (LANGUAGE_COMMON, LANGUAGE_ELVISH, etc.)
- **Converter needed**: Map lowercase strings to UPPER_SNAKE enum values

#### Tools
- **Toolkit**: `proficiencies.Tool` (string alias, values: "thieves-tools", "smith-tools", etc.)
- **Proto**: `Tool` enum (TOOL_THIEVES_TOOLS, TOOL_SMITH_TOOLS, etc.)
- **Converter needed**: Map kebab-case strings to UPPER_SNAKE enum values

#### Fighting Styles
- **Toolkit**: `fightingstyles.FightingStyle` (string alias, values: "archery", "defense", etc.)
- **Proto**: Currently just `string` in proto (not enum)
- **Conversion**: Direct string cast works

#### Backgrounds
- **Toolkit**: `backgrounds.Background` (string alias, values: "acolyte", "criminal", etc.)
- **Proto**: `Background` enum (BACKGROUND_ACOLYTE, BACKGROUND_CRIMINAL, etc.)
- **Converter needed**: Map lowercase strings to UPPER_SNAKE enum values

### Complex Conversions

#### Ability Scores
- **Toolkit**: `shared.AbilityScores` (map[string]int with keys: "str", "dex", "con", "int", "wis", "cha")
- **Proto**: `AbilityScores` message with int32 fields (strength, dexterity, constitution, intelligence, wisdom, charisma)
- **Conversion**: Map between abbreviated keys and full field names

#### Spells (Future Consideration)
- **Toolkit**: `spells.Spell` (includes both spells and cantrips)
- **Proto**: Separate `SpellList` and `CantripList`
- **Challenge**: Need to check spell level to separate cantrips (level 0) from regular spells

## Implementation Priority

### Phase 1 (Current)
✅ Basic structure (Category, Source, ChoiceID)
✅ Equipment selections (string casting)
✅ Skills, Languages (partial - need enum converters)
✅ Fighting styles (string field)
✅ Backgrounds (partial - need enum converter)

### Phase 2 (Tomorrow)
- [ ] Add enum converter functions for Skills, Languages, Tools, Backgrounds
- [ ] Add Tool proficiency selection handling
- [ ] Add Expertise selection handling (reuse Skill converter)
- [ ] Add Trait selection handling (simple string array)

### Phase 3 (Future)
- [ ] Spell/Cantrip separation logic
- [ ] Validation of SelectionIDs against valid equipment
- [ ] Handle special equipment categories ("any-simple-weapon", "any-martial-weapon")

## Notes

### Why Direct Casting Works for SelectionID
Since `SelectionID` is defined as `type SelectionID = string` (not `type SelectionID string`), it's a pure type alias. This means:
- No runtime overhead
- Direct casting between `string` and `SelectionID` is safe
- No need for complex conversion logic

### Equipment ID Format
All equipment IDs follow kebab-case naming:
- Weapons: "longsword", "light-crossbow", "hand-axe"
- Armor: "chain-mail", "studded-leather", "half-plate"
- Packs: "explorer-pack", "burglar-pack", "scholar-pack"
- Categories: "any-simple-weapon", "any-martial-weapon"

### Validation Considerations
The converters should not validate if SelectionIDs are valid equipment - that's the toolkit's responsibility. The API layer just passes the strings through and lets the toolkit's `equipment.GetByID()` handle validation.