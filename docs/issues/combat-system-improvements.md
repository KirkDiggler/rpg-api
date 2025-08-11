# Combat System Improvements

## Overview
This document tracks issues and improvements needed for the combat system in rpg-api.
The combat package is currently located in `internal/toolkit/combat` as a prototype.

## Critical Issues (Fix Immediately)

### 1. Entity Type Detection Bug ✅ FIXED
**Status**: Fixed
**Problem**: Using string prefix "mon_" to detect monsters is fragile and incorrect
**Solution**: 
- Added TODO comments to pass entity type through call chain
- Removed hardcoded prefix check
- Falls back gracefully when character lookup fails

### 2. ParseDiceString Error Handling ✅ FIXED
**Status**: Fixed
**Problem**: Silent failures with strconv.Atoi, no error handling
**Solution**: 
- Added proper error returns to ParseDiceString
- Validates dice format and returns errors
- Added RollDiceString wrapper for simple cases
- Handles negative modifiers (e.g., "1d20-2")

### 3. Critical Hit Implementation ✅ FIXED
**Status**: Fixed
**Problem**: Simply doubling total damage is incorrect per D&D 5e rules
**Solution**: 
- Critical hits now double the DICE, not the total
- Modifiers are added only once (correct 5e behavior)
- Separate handling for weapon attacks vs spell attacks

## Important Issues (Fix Soon)

### 4. Missing Weapon Integration
**Status**: Open
**Problem**: Not using actual weapon data, everything is hardcoded
**Solution Started**: Created `weapon_integration.go` with:
- WeaponData and SpellData structures
- Provider interfaces for data sources
- Framework for finesse weapons and properties
**Next Steps**:
- Implement WeaponProvider using dnd5e-api data
- Wire up to actual attack resolution
- Handle versatile, thrown, and reach properties

### 5. Entity Type Storage
**Status**: Open
**Problem**: No reliable way to determine entity types
**Solutions Needed**:
1. Store entity type when adding to encounter
2. Pass entity type in AttackInput
3. Create EntityRegistry in encounter to track types
4. Use Entity.GetType() consistently

### 6. Spell Attack Implementation
**Status**: Open
**Problem**: Spell attacks incorrectly use INT for all casters
**Solutions Needed**:
- Detect spellcasting ability by class (WIS for Cleric/Druid, CHA for Sorcerer/Warlock, INT for Wizard)
- Implement spell attack bonus properly
- Handle cantrip scaling with character level

## Future Improvements (Track for Later)

### 7. Move Combat to rpg-toolkit
**Status**: Deferred
**When**: After combat system is mature and proven
**Why Wait**: Need to validate design with real usage first

### 8. Complex Mechanics Integration
**Status**: Planning
**Features to Add**:
- Advantage/Disadvantage (partially implemented)
- Saving throws
- Skill checks
- Conditions and status effects
- Area of effect attacks
- Reactions and bonus actions
- Concentration checks

### 9. Monster Data Service
**Status**: Not Started
**Need**: Proper monster stats instead of hardcoded defaults
**Options**:
- Create monster service in rpg-api
- Integrate with external D&D 5e API
- Store monster templates in database

### 10. Combat Logging and History
**Status**: Not Started
**Features**:
- Track all combat actions
- Generate combat narrative
- Support undo/replay
- Analytics and statistics

## Integration Points

### With dnd5e-api
- Weapon data (damage dice, properties)
- Spell data (damage, save DCs, ranges)
- Monster stats and abilities
- Equipment effects on AC and attacks

### With rpg-toolkit (future)
- Move combat resolution logic when mature
- Integrate with rules engine
- Support multiple game systems

### With rpg-dnd5e-web
- Real-time combat updates
- Visual combat feedback
- Turn notifications
- Damage animations

## Code Quality Improvements

### Testing
- Add comprehensive unit tests for combat resolver
- Test critical hit mechanics
- Test dice parsing edge cases
- Integration tests with mock data

### Documentation
- Document combat flow
- Explain damage calculation
- API documentation for combat endpoints
- Example combat scenarios

## Performance Considerations

### Current Issues
- No caching of character/monster stats
- Repeated database lookups
- No batch processing for multi-attacks

### Optimizations Needed
- Cache entity stats during encounter
- Batch database operations
- Pre-calculate common modifiers
- Optimize dice rolling algorithms

## Architecture Notes

### Current Structure
```
internal/toolkit/combat/
├── resolver.go         # Main combat logic
├── types.go           # Data structures
└── weapon_integration.go  # Integration framework (new)
```

### Proposed Structure (when moving to toolkit)
```
rpg-toolkit/
└── systems/
    └── dnd5e/
        └── combat/
            ├── attacks/
            ├── conditions/
            ├── damage/
            └── saves/
```

## Priority Matrix

| Issue | Impact | Effort | Priority |
|-------|--------|--------|----------|
| Entity Type Detection | High | Low | P0 ✅ |
| ParseDiceString Errors | High | Low | P0 ✅ |
| Critical Hits | High | Low | P0 ✅ |
| Weapon Integration | High | Medium | P1 |
| Entity Type Storage | High | Medium | P1 |
| Spell Attacks | Medium | Medium | P2 |
| Monster Service | Medium | High | P2 |
| Move to Toolkit | Low | High | P3 |
| Complex Mechanics | Medium | High | P3 |

## Next Actions

1. ✅ Fix critical issues (1-3) - DONE
2. Implement WeaponProvider interface
3. Create entity type registry
4. Add proper spell attack handling
5. Write comprehensive tests
6. Document combat system flow