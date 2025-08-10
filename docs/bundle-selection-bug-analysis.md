# Bundle Selection Bug Analysis

## Current State (BROKEN)

### What the API Sends to Frontend
```json
{
  "optionType": {
    "case": "bundle",
    "value": {
      "items": [
        {
          "itemType": {
            "case": "concreteItem",
            "value": {
              "itemId": "choose-martial-weapons",
              "name": "choose-martial-weapons"
            }
          }
        },
        {
          "itemType": {
            "case": "concreteItem",
            "value": {
              "itemId": "shield",
              "name": "shield"
            }
          }
        }
      ]
    }
  }
}
```

### What Frontend Sends Back (WRONG)
```json
{
  "selection": {
    "case": "equipment",
    "value": {
      "items": ["warhammer"]  // MISSING THE SHIELD!
    }
  }
}
```

### Result
- Character gets: `warhammer`
- Character missing: `shield`

## Expected Behavior (CORRECT)

### What Frontend Should Send
```json
{
  "selection": {
    "case": "equipment",
    "value": {
      "items": [
        "bundle_0:0:warhammer",  // User's weapon choice at index 0
        "bundle_0:1:shield"      // Concrete shield at index 1
      ]
    }
  }
}
```

### How Backend Processes It
1. Receives: `["bundle_0:0:warhammer", "bundle_0:1:shield"]`
2. Unpacks using `unpackBundleItem()`:
   - `"bundle_0:0:warhammer"` → `"warhammer"`
   - `"bundle_0:1:shield"` → `"shield"`
3. Stores in character: `["warhammer", "shield"]`

## The Core Problem

The frontend `EquipmentChoice.tsx` component is not correctly handling bundles that contain both:
1. A choice placeholder (`choose-martial-weapons`)
2. Concrete items (`shield`)

When the user selects their weapon, the component should:
1. Replace the choice placeholder with the user's selection
2. Include ALL concrete items from the bundle
3. Format everything as bundle references

## Code Location

### Backend (WORKING)
- `/home/kirk/personal/rpg-api/internal/orchestrators/character/orchestrator.go`
- Function: `unpackBundleItem()` - correctly extracts items from bundle references

### Frontend (BROKEN)
- `/home/kirk/personal/rpg-dnd5e-web/src/components/choices/EquipmentChoice.tsx`
- Issue: Not including concrete items when processing bundles with choices

## Test to Verify Fix

1. Create a Fighter character
2. Select "Martial weapon and shield" bundle
3. Choose any martial weapon (e.g., longsword)
4. Check the network request payload
5. Should see: `["bundle_0:0:longsword", "bundle_0:1:shield"]`
6. Character equipment should show both items

## Fighting Style Issue

Separate issue: Fighting style selections are not persisting through draft updates.
- When selected: Saved correctly
- After navigating: Lost
- Needs investigation in backend draft update logic