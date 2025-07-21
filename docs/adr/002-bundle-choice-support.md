# ADR-002: Support Nested Choices in Equipment Bundles

## Status
Accepted

## Context

When converting D&D 5e equipment choices from the dnd5e-api to our proto format, we encountered an inconsistency in how category references are represented:

1. **Standalone choice** (e.g., "Choose 2 martial weapons"):
   - Properly represented as `NestedChoice` → `Choice` → `CategoryReference`
   - Clean, strongly typed structure

2. **Bundle with choice** (e.g., "Choose 1: martial weapon and shield"):
   - Forced to use `CountedItemReference` with `itemId: "category://martial-weapons"`
   - URI scheme hack because `ItemBundle` only supports `CountedItemReference`

This inconsistency creates confusion for UI developers who must handle the same logical concept (pick from category) differently based on context.

## Decision

We will extend the proto structure to properly support choices within bundles by:

1. Creating a new `BundleItem` message that can contain either:
   - `concrete_item`: A `CountedItemReference` for concrete items
   - `choice_item`: A `NestedChoice` for choices

2. Updating `ItemBundle` to use `repeated BundleItem` instead of `repeated CountedItemReference`

This change eliminates the need for URI scheme hacks and provides consistent, strongly-typed representation of choices regardless of context.

## Consequences

### Positive
- **Consistency**: Category references are always represented as proper choices
- **Type safety**: No string parsing or URI schemes needed
- **Clear intent**: Oneof makes it obvious what's concrete vs needs selection
- **Better UX**: UI developers have one pattern to handle choices

### Negative
- **Breaking change**: Clients must update to handle new structure
- **Migration effort**: Need to update both proto generation and parsing code
- **Backward compatibility**: Must support both formats during transition

### Neutral
- Proto message size slightly increases due to additional nesting
- More complex proto structure, but more accurate domain modeling

## Implementation

1. Update `character.proto` to add `BundleItem` message
2. Regenerate proto bindings for all languages
3. Update choice parser to emit new structure
4. Update converters to handle new structure
5. Document the change for UI developers

## Example

Before (URI scheme hack):
```json
{
  "bundle": {
    "items": [
      {"item_id": "category://martial-weapons", "name": "a martial weapon", "quantity": 1},
      {"item_id": "shield", "name": "Shield", "quantity": 1}
    ]
  }
}
```

After (proper structure):
```json
{
  "bundle": {
    "items": [
      {
        "choice_item": {
          "choice": {
            "id": "martial_weapon_choice",
            "description": "a martial weapon",
            "choose_count": 1,
            "choice_type": "CHOICE_TYPE_EQUIPMENT",
            "category_reference": {"category_id": "martial-weapons"}
          }
        }
      },
      {
        "concrete_item": {"item_id": "shield", "name": "Shield", "quantity": 1}
      }
    ]
  }
}
```

## References
- Proto change: `/rpg-api-protos/dnd5e/api/v1alpha1/character.proto`
- Design doc: `/docs/proto-redesign-bundle-choices.md`
- Related issue: Fighter equipment choices showing category references as regular items