# Character Finalization Gaps Analysis

## Overview
This document identifies gaps in the character finalization process where data from race, class, and background combinations is not being properly populated to characters.

## Current State (What's Working)

### ✅ Successfully Populated Fields
1. **Basic Character Info**
   - Name, PlayerID, Level (set to 1)
   - RaceID, SubraceID, ClassID, BackgroundID

2. **Ability Scores**
   - Base scores from draft choices are transferred

3. **Hit Points**
   - Calculated from class hit dice + CON modifier
   - Minimum 1 HP enforced

4. **Physical Characteristics**
   - Speed from race data
   - Size from race data

5. **Languages**
   - Languages from race data (e.g., Common, Elvish for Elf)
   - Additional language choices from draft

6. **Skills**
   - Skills from class choices in draft
   - Properly marked as Proficient level

7. **Saving Throws**
   - Class-based saving throw proficiencies

8. **Weapon & Armor Proficiencies**
   - Weapon proficiencies from class
   - Armor proficiencies from class

9. **Equipment**
   - Equipment from choices in draft

10. **Fighting Style**
    - Fighting style choice is recorded in choices array

## Identified Gaps

### 🔴 Critical Gaps (Core Functionality Missing)

#### 1. Background Data Not Fetched
- **Issue**: `GetBackgroundData` is not implemented in external client
- **Impact**: No background skills, languages, tools, or equipment
- **Location**: `orchestrator.go:630-635` (commented out)
- **Required Updates**:
  - Implement `GetBackgroundData` in external client
  - Add background data structure to rpg-toolkit
  - Process background data in finalization

#### 2. Racial Traits Not Tracked
- **Issue**: Race data doesn't include traits like Darkvision, Keen Senses
- **Impact**: Missing racial abilities and features
- **Missing Fields in race.Data**:
  - Traits (e.g., Darkvision, Fey Ancestry, Lucky)
  - Skill proficiencies (e.g., Elf Perception)
  - Tool proficiency choices (e.g., Dwarf tools)
  - Cantrip grants (e.g., High Elf cantrip)

#### 3. Tool Proficiencies Not Tracked
- **Issue**: No tool proficiency field in Proficiencies struct
- **Impact**: Can't track tool proficiencies from any source
- **Location**: `character.Data.Proficiencies` needs Tools field
- **Sources of tool proficiencies**:
  - Racial (Dwarf choice)
  - Background (most backgrounds)
  - Class (some classes like Rogue with thieves' tools)

### 🟡 Important Gaps (Features Incomplete)

#### 4. Class Features Not Applied
- **Issue**: Class features like Second Wind not initialized
- **Impact**: Missing class resources and abilities
- **Examples**:
  - Fighter: Second Wind, Action Surge
  - Barbarian: Rage uses
  - Monk: Ki points
  - Wizard: Arcane Recovery

#### 5. Subrace Features Not Applied
- **Issue**: Subrace-specific bonuses not processed
- **Impact**: Missing subrace benefits
- **Examples**:
  - Hill Dwarf: +1 HP per level
  - High Elf: Extra language, wizard cantrip
  - Lightfoot Halfling: Naturally Stealthy

#### 6. Expertise Not Tracked
- **Issue**: No way to distinguish expertise from proficiency
- **Impact**: Rogue/Bard expertise not properly represented
- **Potential Solution**: Add expertise level to ProficiencyLevel enum

#### 7. Special Languages Not Defined
- **Issue**: Thieves' Cant not in constants.Language
- **Impact**: Rogue's Thieves' Cant can't be added
- **Location**: Need to add to rpg-toolkit constants

### 🟠 Minor Gaps (Nice to Have)

#### 8. Starting Spell Slots Not Initialized
- **Issue**: SpellSlots map is empty for spellcasters
- **Impact**: Spellcasters don't have spell slots set
- **Required**: Calculate based on class and level

#### 9. Spell Lists Not Populated
- **Issue**: Known/prepared spells not tracked separately
- **Impact**: Can't distinguish between known and prepared spells
- **Required**: Separate fields for spell lists

#### 10. Feat System Not Implemented
- **Issue**: No feat selection or tracking
- **Impact**: Variant Human feat, future feat selection
- **Note**: May be intentionally deferred for later levels

## Required Updates by Project

### rpg-toolkit Updates Needed

1. **Add Background Data Structure**
   ```go
   type BackgroundData struct {
       ID                 constants.Background
       Name               string
       Skills             []constants.Skill     // 2 skills usually
       Languages          int                   // Number to choose
       ToolProficiencies  []string
       Equipment          []string
       Feature            string
   }
   ```

2. **Enhance Race Data**
   ```go
   type Data struct {
       // Existing fields...
       
       // Add these:
       Traits             []string
       SkillProficiencies []constants.Skill
       ToolProficiencyChoices []string  // Choose 1 from list
       ExtraLanguages     int           // Number of bonus languages
       Cantrips          []string       // Racial cantrips
   }
   ```

3. **Add Tool Proficiencies to Proficiencies**
   ```go
   type Proficiencies struct {
       Weapons []string
       Armor   []string
       Tools   []string  // NEW
   }
   ```

4. **Add Missing Constants**
   - `constants.LanguageThievesCant`
   - `shared.ChoiceToolProficiency`

5. **Add Expertise Level**
   ```go
   const (
       NotProficient ProficiencyLevel = 0
       Proficient    ProficiencyLevel = 1
       Expert        ProficiencyLevel = 2  // NEW
   )
   ```

### rpg-api Updates Needed

1. **Implement GetBackgroundData in External Client**
   - Add method to fetch background data
   - Map API response to BackgroundData struct

2. **Enhance Finalization Logic**
   - Process background skills (check for duplicates)
   - Process background languages
   - Process background tool proficiencies
   - Apply racial skill proficiencies
   - Apply subrace bonuses (e.g., Hill Dwarf HP)
   - Initialize spell slots for spellcasters
   - Initialize class resources

3. **Add Choice Processing**
   - Tool proficiency choices
   - Racial cantrip choices
   - Background equipment choices

### rpg-api-protos Updates Needed

1. **Add Background Service**
   - GetBackground RPC
   - ListBackgrounds RPC
   - Background message type

2. **Enhance Character Message**
   - Add tools field to proficiencies
   - Add spell_slots field
   - Add class_resources field
   - Add known_spells/prepared_spells fields

## Testing Strategy

### Integration Tests Needed

1. **Background Integration**
   - Verify skills from background don't duplicate class skills
   - Verify language count from background
   - Verify tool proficiencies from background

2. **Racial Features**
   - Verify racial skill proficiencies (Elf Perception)
   - Verify racial tool choices (Dwarf)
   - Verify subrace bonuses (Hill Dwarf HP)

3. **Class Features**
   - Verify class resources initialized
   - Verify spell slots for casters
   - Verify expertise for Rogues/Bards

4. **Edge Cases**
   - Duplicate skill proficiencies (background + class)
   - Duplicate languages
   - Missing optional choices

## Priority Recommendations

### Phase 1: Critical Infrastructure
1. Implement GetBackgroundData
2. Add tool proficiencies field
3. Add missing constants

### Phase 2: Core Features
1. Process background data in finalization
2. Enhance race data with traits
3. Initialize class resources

### Phase 3: Advanced Features
1. Expertise system
2. Spell slot initialization
3. Subrace-specific bonuses

### Phase 4: Future Enhancements
1. Feat system
2. Multiclassing support
3. Level-up processing

## Conclusion

The character finalization process has a solid foundation but needs enhancements to fully populate characters with all their starting abilities. The main gaps are:

1. **Background data integration** - Completely missing
2. **Racial traits and features** - Not tracked or applied
3. **Tool proficiencies** - No field to store them
4. **Class resources** - Not initialized

These gaps mean that finalized characters are missing important game mechanics that players expect to have at character creation. Addressing these gaps will require coordinated updates across rpg-toolkit, rpg-api, and potentially rpg-api-protos.
