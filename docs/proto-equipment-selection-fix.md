# Proto Equipment Selection Fix: Aligning with Toolkit's Requirements/Submissions Pattern

## Problem Statement

The current proto structure for equipment selections didn't properly handle multiple item selections from category choices (e.g., "choose 2 martial weapons"). The `EquipmentSelection` message used a `oneof` that only supported selecting ONE item, which broke the Requirements vs Submissions pattern from the toolkit.

## Root Cause Analysis

### Toolkit Pattern (Correct)
```go
// Requirements can ask for multiple selections
type EquipmentCategoryRequirement struct {
    Choose int  // How many to choose (e.g., 2)
    // ...
}

// Submissions store multiple values generically
type Submission struct {
    Values []shared.SelectionID  // Multiple selection IDs
    // ...
}
```

### Original Proto Pattern (Broken)
```proto
// Could only represent ONE equipment selection
message EquipmentSelection {
  oneof equipment {
    Weapon weapon = 1;
    Armor armor = 2;
    // ... only one item possible
  }
}
```

### The Gap
When a player chose from an `EquipmentCategoryChoice` like "choose 2 martial weapons", there was no way to represent the multiple selections in the proto.

## Solution

### 1. Fixed EquipmentSelection Structure
```proto
// Now supports multiple items from category choices
message EquipmentSelection {
  repeated EquipmentSelectionItem items = 1; // Multiple items can be selected
}

message EquipmentSelectionItem {
  oneof equipment {
    Weapon weapon = 1;
    Armor armor = 2;
    Tool tool = 3;
    Pack pack = 4;
    Ammunition ammunition = 5;
    string other_equipment_id = 6;
  }
  int32 quantity = 7;
}
```

### 2. Added Generic Selection Pattern
```proto
message ChoiceSubmission {
  string choice_id = 1;
  ChoiceCategory category = 2;
  ChoiceSource source = 3;
  string option_id = 4;
  
  // NEW: Generic selection IDs - mirrors toolkit's Values []shared.SelectionID
  repeated string selection_ids = 5; // Can handle multiple selections
  
  // DEPRECATED: Category-specific selections (backward compatibility)
  oneof selection { ... }
}
```

### 3. Updated ChoiceData for Consistency
```proto
message ChoiceData {
  // ... existing fields ...
  
  // NEW: Generic selection IDs - preferred approach
  repeated string selection_ids = 5;
  
  // DEPRECATED: Category-specific selection data
  oneof selection { ... }
}
```

## How It Solves the Problem

### Before (Broken)
```
Requirements: "choose 2 martial weapons"
Player selects: ["longsword", "battleaxe"]
Proto could only store: ONE weapon (broken!)
```

### After (Fixed)
```
Requirements: "choose 2 martial weapons" 
Player selects: ["longsword", "battleaxe"]

Option 1 - New generic approach:
selection_ids: ["longsword", "battleaxe"]

Option 2 - Enhanced equipment selection:
equipment: {
  items: [
    { weapon: WEAPON_LONGSWORD, quantity: 1 },
    { weapon: WEAPON_BATTLEAXE, quantity: 1 }
  ]
}
```

## Alignment with Toolkit Pattern

| Toolkit Concept | Proto Implementation | Alignment |
|-----------------|---------------------|-----------|
| `Requirements.Choose int` | `Choice.choose_count` | ✅ Already aligned |
| `EquipmentCategoryRequirement` | `EquipmentCategoryChoice` | ✅ Already aligned |
| `Submission.Values []SelectionID` | `ChoiceSubmission.selection_ids` | ✅ Now aligned |
| Multiple equipment selections | `EquipmentSelection.items` | ✅ Now supported |

## Migration Strategy

1. **Backward Compatibility**: Old category-specific selections remain available but deprecated
2. **Forward Compatibility**: New `selection_ids` field provides the generic toolkit-aligned approach
3. **Gradual Migration**: Services can gradually move from category-specific to generic selections

## Benefits

1. **Proper Category Choice Support**: Can now handle "choose 2 martial weapons" correctly
2. **Toolkit Alignment**: Proto structure now mirrors the toolkit's Requirements/Submissions pattern
3. **Generic Selection Pattern**: Single field (`selection_ids`) handles all selection types consistently
4. **Future-Proof**: Easy to add new choice categories without structural changes
5. **Backward Compatible**: Existing code continues to work

## Usage Examples

### Equipment Category Choice (New Capability)
```proto
# Requirements
choice: {
  id: "fighter-weapons"
  choose_count: 2
  choice_type: CHOICE_CATEGORY_EQUIPMENT
  equipment_options: {
    bundles: [{
      category_choices: [{
        choose: 2
        weapon_categories: [WEAPON_CATEGORY_MARTIAL]
        label: "Choose 2 martial weapons"
      }]
    }]
  }
}

# Submission
submission: {
  choice_id: "fighter-weapons"
  category: CHOICE_CATEGORY_EQUIPMENT
  selection_ids: ["longsword", "battleaxe"]  # Multiple selections now possible!
}
```

### Equipment Bundle Choice (Existing, Still Works)
```proto
# Submission for bundle selection
submission: {
  choice_id: "fighter-armor"
  category: CHOICE_CATEGORY_EQUIPMENT
  option_id: "chain-mail-bundle"
  selection_ids: ["chain-mail", "shield"]  # Items from the selected bundle
}
```

This fix ensures the proto structure properly supports the toolkit's Requirements vs Submissions pattern, especially for equipment category choices that require multiple selections.
