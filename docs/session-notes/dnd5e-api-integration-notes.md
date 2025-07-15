# D&D 5e API Integration Notes

**Date**: July 14, 2025  
**Context**: Understanding the external data source for race/class validation (#36)

## Architecture Overview

```
rpg-api → dnd5e-api (our wrapper) → D&D 5e SRD API (5e-bits/dnd5eapi.co)
```

## D&D 5e SRD API (External Source)

**Base URL**: `https://www.dnd5eapi.co/api/`  
**Documentation**: https://5e-bits.github.io/docs/  
**Features**: Comprehensive D&D 5e System Reference Document data

### Key Endpoints for Our Use

#### Races
- **List**: `/api/races` - Returns all available races (9 total)
- **Detail**: `/api/races/{race-index}` - Full race data

**Race Data Structure** (Example: Elf):
```json
{
  "index": "elf",
  "name": "Elf", 
  "speed": 30,
  "ability_bonuses": [{"ability_score": {"index": "dex"}, "bonus": 2}],
  "size": "Medium",
  "age": "although elves reach physical maturity...",
  "alignment": "Elves lean strongly toward...",
  "language_desc": "You can speak, read, and write Common and Elvish...",
  "traits": [
    {"index": "darkvision", "name": "Darkvision"},
    {"index": "fey-ancestry", "name": "Fey Ancestry"},
    {"index": "trance", "name": "Trance"}
  ],
  "proficiencies": [{"index": "skill-perception", "name": "Skill: Perception"}],
  "languages": [
    {"index": "common", "name": "Common"},
    {"index": "elvish", "name": "Elvish"}
  ],
  "subraces": [{"index": "high-elf", "name": "High Elf", "url": "/api/subraces/high-elf"}]
}
```

#### Classes  
- **List**: `/api/classes` - Returns all available classes
- **Detail**: `/api/classes/{class-index}` - Full class data

**Class Data Structure** (Example: Wizard):
```json
{
  "index": "wizard",
  "name": "Wizard",
  "hit_die": 6,
  "spellcasting_ability": {"index": "int", "name": "INT"},
  "multi_classing": {"prerequisites": [{"ability_score": {"index": "int"}, "minimum_score": 13}]},
  "proficiencies": [
    {"index": "daggers", "name": "Daggers"},
    {"index": "darts", "name": "Darts"}
  ],
  "saving_throws": [
    {"index": "int", "name": "INT"},
    {"index": "wis", "name": "WIS"}
  ],
  "proficiency_choices": [{
    "desc": "Choose two from Arcana, History, Insight...",
    "choose": 2,
    "from": {
      "option_set_type": "options_array",
      "options": [
        {"option_type": "reference", "item": {"index": "skill-arcana"}},
        {"option_type": "reference", "item": {"index": "skill-history"}}
      ]
    }
  }],
  "starting_equipment": [...],
  "spellcasting": {...}
}
```

#### Other Useful Endpoints
- `/api/ability-scores` - Ability score definitions
- `/api/skills` - All skills with ability score mappings
- `/api/proficiencies` - All proficiency types
- `/api/subraces/{subrace-index}` - Subrace details

## Our Implementation Strategy

### 1. **dnd5e-api Service** (Wrapper with Caching)
- Wraps the external API with caching layer
- Provides clean interface for rpg-api
- Handles rate limiting, error handling, retries

### 2. **rpg-api Integration** 
- Uses `internal/clients/external.Client` interface
- Fetches data through dnd5e-api for validation
- Caches frequently accessed data (races, classes)

### 3. **Engine Validation Logic**
For **Race Validation**:
```go
func (a *Adapter) ValidateRaceChoice(ctx context.Context, input *engine.ValidateRaceChoiceInput) (*engine.ValidateRaceChoiceOutput, error) {
    // 1. Fetch race data from external client
    raceData, err := a.externalClient.GetRaceData(ctx, input.RaceID)
    
    // 2. Validate subrace belongs to race (if provided)
    if input.SubraceID != "" {
        // Check subraces array
    }
    
    // 3. Return traits and ability modifiers
    return &engine.ValidateRaceChoiceOutput{
        IsValid: true,
        RaceTraits: extractTraitNames(raceData.Traits),
        AbilityMods: convertAbilityBonuses(raceData.AbilityBonuses),
    }, nil
}
```

For **Class Validation**:
```go
func (a *Adapter) ValidateClassChoice(ctx context.Context, input *engine.ValidateClassChoiceInput) (*engine.ValidateClassChoiceOutput, error) {
    // 1. Fetch class data from external client
    classData, err := a.externalClient.GetClassData(ctx, input.ClassID)
    
    // 2. Check ability score prerequisites
    if !meetsPrerequisites(input.AbilityScores, classData.MultiClassing.Prerequisites) {
        return &engine.ValidateClassChoiceOutput{
            IsValid: false,
            Errors: []engine.ValidationError{{
                Field: "ability_scores",
                Message: "Does not meet class prerequisites",
                Code: "INSUFFICIENT_ABILITY_SCORES",
            }},
        }
    }
    
    // 3. Return class features
    return &engine.ValidateClassChoiceOutput{
        IsValid: true,
        HitDice: fmt.Sprintf("1d%d", classData.HitDie),
        PrimaryAbility: classData.SpellcastingAbility.Index,
        SavingThrows: extractSavingThrows(classData.SavingThrows),
        SkillChoicesCount: classData.ProficiencyChoices[0].Choose,
        AvailableSkills: extractSkillOptions(classData.ProficiencyChoices),
    }, nil
}
```

## Next Steps for Issue #36

1. **Update engine adapter constructor** to accept external client dependency
2. **Implement validation logic** using external data (placeholder for now)
3. **Create comprehensive tests** with mocked external client responses  
4. **Document missing external client implementation** as separate issue

## Data Mapping Notes

- **Ability Score Keys**: API uses "str", "dex", "con", "int", "wis", "cha"
- **Hit Dice**: API returns integer (6), we need "1d6" format
- **Prerequisites**: API has structured prerequisites with minimum scores
- **Traits**: API returns array with index/name, we need just names
- **Skills**: API uses skill indexes like "skill-perception", we may need clean names

## Caching Strategy

Given this is relatively static data, aggressive caching makes sense:
- **Races/Classes**: Cache for hours/days (rarely change)
- **Individual lookups**: Cache by ID with TTL
- **Bulk operations**: Cache list endpoints

This rich external API gives us everything needed for comprehensive D&D 5e validation!
