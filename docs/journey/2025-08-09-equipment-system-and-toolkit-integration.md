# Equipment System Design and Toolkit Integration

## Current State

We've implemented a minimal equipment system in rpg-api for the combat demo with:
- Basic equipment slot management (equip/unequip)
- Inventory tracking
- Redis persistence for equipped items

## Copilot's Feedback

1. **Debug Logging**: Should use proper log levels (Debug vs Info)
2. **Helper Functions**: Need better structure for tracking equipped items
3. **Incomplete Slots**: Some equipment slots (Helm, Ring1, Ring2) are commented out

## Future rpg-toolkit Integration

### What Should Move to rpg-toolkit

The following equipment logic belongs in rpg-toolkit as game rules:

#### 1. Equipment Validation Rules
```go
// rpg-toolkit/rulebooks/dnd5e/equipment/validator.go
type EquipmentValidator interface {
    // Can this item be equipped in this slot?
    CanEquipToSlot(item Item, slot EquipmentSlot) error
    
    // Check class/race restrictions
    CanCharacterUseItem(character *Character, item Item) error
    
    // Check for conflicts (e.g., two-handed weapons)
    ValidateEquipmentSet(equipped map[EquipmentSlot]Item) []error
}
```

#### 2. Item Categories and Properties
```go
// rpg-toolkit/rulebooks/dnd5e/equipment/types.go
type ItemType int
const (
    ItemTypeWeapon ItemType = iota
    ItemTypeArmor
    ItemTypeShield
    // etc...
)

type WeaponProperties struct {
    Damage      string
    DamageType  DamageType
    Properties  []WeaponProperty // Finesse, Versatile, Two-Handed, etc.
    Range       *Range
}

type ArmorProperties struct {
    ArmorClass       int
    StealthDisadvantage bool
    StrengthRequired int
}
```

#### 3. Equipment Slot Rules
```go
// rpg-toolkit/rulebooks/dnd5e/equipment/slots.go
func GetValidSlotsForItem(item Item) []EquipmentSlot {
    // Logic to determine which slots an item can occupy
    // e.g., shields can only go in off-hand
    // two-handed weapons occupy both hand slots
}

func GetSlotConflicts(item Item, slot EquipmentSlot) []EquipmentSlot {
    // Returns slots that would need to be cleared
    // e.g., equipping a two-handed weapon clears off-hand
}
```

#### 4. Proficiency Checking
```go
// rpg-toolkit/rulebooks/dnd5e/character/proficiencies.go
func (c *Character) IsProficientWithItem(item Item) bool {
    // Check weapon/armor proficiencies
    // Consider class, race, and feat-based proficiencies
}
```

### What Stays in rpg-api

The following should remain in rpg-api as data/orchestration:

1. **Persistence Layer**
   - Storing which items are equipped where
   - Managing inventory quantities
   - Session state

2. **API Orchestration**
   - Coordinating between toolkit validation and storage
   - Transaction management
   - Error handling and response formatting

3. **External Data Integration**
   - Fetching item data from D&D 5e API
   - Converting between external and internal formats

## Implementation Plan

### Phase 1: Current (Minimal Demo)
✅ Basic slot management in rpg-api
✅ Simple string-based item storage
✅ No validation beyond "item exists"

### Phase 2: Enhanced Validation
- [ ] Move item type detection to toolkit
- [ ] Add proficiency checking via toolkit
- [ ] Implement slot conflict detection

### Phase 3: Full Equipment System
- [ ] Complete item property system in toolkit
- [ ] Add encumbrance calculations
- [ ] Implement attunement rules
- [ ] Add magical item effects

## Example Integration

Here's how rpg-api would use rpg-toolkit for equipment:

```go
// In rpg-api orchestrator
func (o *Orchestrator) EquipItem(ctx context.Context, input *EquipItemInput) (*EquipItemOutput, error) {
    // 1. Get character data
    char := o.getCharacter(input.CharacterID)
    
    // 2. Get item data
    item := o.getItem(input.ItemID)
    
    // 3. Use toolkit for validation
    validator := toolkit.NewEquipmentValidator()
    
    // Check if item can go in slot
    if err := validator.CanEquipToSlot(item, input.Slot); err != nil {
        return nil, errors.InvalidArgument(err.Error())
    }
    
    // Check proficiency
    if !char.IsProficientWithItem(item) {
        // Warning, but allow equipping
        warnings = append(warnings, "Not proficient with this item")
    }
    
    // Check for slot conflicts
    conflicts := toolkit.GetSlotConflicts(item, input.Slot)
    for _, conflictSlot := range conflicts {
        // Clear conflicting slots
        o.clearSlot(char.ID, conflictSlot)
    }
    
    // 4. Persist the change
    o.repository.SetEquipmentSlot(...)
    
    // 5. Return updated character
    return &EquipItemOutput{
        Character: char,
        Warnings: warnings,
    }, nil
}
```

## Design Principles

1. **Separation of Concerns**
   - rpg-toolkit: Game rules and mechanics
   - rpg-api: Data persistence and API orchestration

2. **Incremental Migration**
   - Start with simple validation in toolkit
   - Gradually move more rules as needed
   - Keep backward compatibility

3. **Testability**
   - Toolkit rules can be unit tested in isolation
   - API orchestration tests mock toolkit interfaces

## Notes on Current Implementation

The current implementation is intentionally minimal for the demo:
- Using strings for item IDs (should eventually be structured data)
- No validation beyond "item exists in inventory"
- Limited equipment slots implemented

This is acceptable for the demo but should be enhanced using toolkit for production use.