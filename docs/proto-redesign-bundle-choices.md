# Proto Redesign: Support Choices in Bundles

## Problem Statement

Currently, when equipment choices include bundles like "leather armor and a martial weapon", we can't properly represent the martial weapon as a choice. The proto structure only allows `CountedItemReference` in bundles, forcing us to use the `category://` URI scheme hack.

## Current Structure (Problem)

```protobuf
message ItemBundle {
  repeated CountedItemReference items = 1;  // Can only contain concrete items!
}

message CountedItemReference {
  string item_id = 1;     // Forced to use "category://martial-weapons" hack
  string name = 2;
  int32 quantity = 3;
}
```

## Proposed Solution

### Option 1: Mixed Bundle Items (Recommended)

```protobuf
// Allow bundles to contain both concrete items and choices
message ItemBundle {
  repeated BundleItem items = 1;
}

message BundleItem {
  oneof item_type {
    CountedItemReference concrete_item = 1;  // For "leather armor", "20 arrows"
    NestedChoice choice_item = 2;            // For "a martial weapon"
  }
}

// No change to existing messages
message CountedItemReference {
  string item_id = 1;
  string name = 2;
  int32 quantity = 3;
}

message NestedChoice {
  Choice choice = 1;
}
```

**Benefits:**
- Strongly typed - no URI scheme hacks
- Consistent representation of choices everywhere
- UI knows exactly what needs user selection
- Backward compatible (old bundles still work)

**Example Usage:**
```json
{
  "bundle": {
    "items": [
      {
        "concrete_item": {
          "item_id": "leather-armor",
          "name": "Leather Armor",
          "quantity": 1
        }
      },
      {
        "choice_item": {
          "choice": {
            "id": "bundle_martial_weapon_choice",
            "description": "a martial weapon",
            "choose_count": 1,
            "choice_type": "CHOICE_TYPE_EQUIPMENT",
            "category_reference": {
              "category_id": "martial-weapons"
            }
          }
        }
      }
    ]
  }
}
```

### Option 2: Flatten to Choice Level

```protobuf
// Instead of bundle being a ChoiceOption, make it part of Choice
message Choice {
  string id = 1;
  string description = 2;
  int32 choose_count = 3;
  ChoiceType choice_type = 4;
  
  oneof option_set {
    ExplicitOptions explicit_options = 5;
    CategoryReference category_reference = 6;
    BundleOptions bundle_options = 7;  // NEW
  }
}

message BundleOptions {
  repeated BundleChoice choices = 1;
}

message BundleChoice {
  repeated BundleItem items = 1;
}
```

**Benefits:**
- Bundles become first-class choice types
- Clear that bundles can contain sub-choices
- More flexible for complex bundle scenarios

### Option 3: Simplify Everything

```protobuf
// Make all options support both items and sub-choices
message ChoiceOption {
  string id = 1;
  string name = 2;
  string description = 3;
  
  repeated OptionItem items = 4;       // What you get
  repeated Choice sub_choices = 5;      // What you still need to choose
}

message OptionItem {
  string item_id = 1;
  string name = 2;
  int32 quantity = 3;
}
```

**Benefits:**
- Uniform structure for all options
- Every option can be a mix of concrete items and choices
- Very flexible

## Recommendation

I recommend **Option 1 (Mixed Bundle Items)** because:

1. **Minimal Breaking Change**: Only affects bundle structure
2. **Clear Intent**: Oneof makes it obvious what's concrete vs needs selection
3. **Strongly Typed**: No string parsing or URI schemes
4. **Preserves Current Design**: Keeps the current choice/option hierarchy
5. **Easy Migration**: Can support both old and new formats during transition

## Migration Strategy

1. **Add to v1alpha1**: Add new `BundleItem` message and update `ItemBundle`
2. **Dual Support**: Parser can handle both old `CountedItemReference` arrays and new `BundleItem` arrays
3. **Gradual Migration**: Update rpg-api to emit new format
4. **Deprecation**: After clients update, deprecate old format

## Implementation Steps

1. Update proto definitions in rpg-api-protos
2. Regenerate Go and other language bindings
3. Update choice_parser.go to emit new structure
4. Update simple_choice_converter.go to handle new structure
5. Update documentation with new examples
6. Test with fighter equipment choices

## Example: Fighter Equipment Choice (After Fix)

```json
{
  "id": "fighter_equipment_1",
  "description": "(a) chain mail or (b) leather armor, longbow, and 20 arrows",
  "choose_count": 1,
  "choice_type": "CHOICE_TYPE_EQUIPMENT",
  "explicit_options": {
    "options": [
      {
        "counted_item": {
          "item_id": "chain-mail",
          "name": "Chain Mail",
          "quantity": 1
        }
      },
      {
        "bundle": {
          "items": [
            {
              "concrete_item": {
                "item_id": "leather-armor",
                "name": "Leather Armor",
                "quantity": 1
              }
            },
            {
              "concrete_item": {
                "item_id": "longbow",
                "name": "Longbow",
                "quantity": 1
              }
            },
            {
              "concrete_item": {
                "item_id": "arrow",
                "name": "Arrow",
                "quantity": 20
              }
            }
          ]
        }
      }
    ]
  }
}
```

And for the martial weapon choice:

```json
{
  "id": "fighter_equipment_2", 
  "description": "(a) a martial weapon and a shield or (b) two martial weapons",
  "choose_count": 1,
  "choice_type": "CHOICE_TYPE_EQUIPMENT",
  "explicit_options": {
    "options": [
      {
        "bundle": {
          "items": [
            {
              "choice_item": {
                "choice": {
                  "id": "martial_weapon_choice_1",
                  "description": "a martial weapon",
                  "choose_count": 1,
                  "choice_type": "CHOICE_TYPE_EQUIPMENT",
                  "category_reference": {
                    "category_id": "martial-weapons"
                  }
                }
              }
            },
            {
              "concrete_item": {
                "item_id": "shield",
                "name": "Shield",
                "quantity": 1
              }
            }
          ]
        }
      },
      {
        "nested_choice": {
          "choice": {
            "id": "two_martial_weapons",
            "description": "two martial weapons",
            "choose_count": 2,
            "choice_type": "CHOICE_TYPE_EQUIPMENT",
            "category_reference": {
              "category_id": "martial-weapons"
            }
          }
        }
      }
    ]
  }
}
```

Now both options use proper strongly-typed choices instead of URI scheme hacks!