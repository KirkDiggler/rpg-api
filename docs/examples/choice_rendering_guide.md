# Choice Rendering Guide for UI Applications

This guide shows how to render different types of D&D 5e choices returned by the rpg-api for character creation.

## Overview

The rpg-api returns choices in a standardized format that supports various D&D 5e character creation scenarios. Each choice has:

- `id`: Unique identifier for tracking selections
- `description`: Human-readable description for the UI
- `chooseCount`: How many options the user must select
- `choiceType`: The type of choice (equipment, skill, language, etc.)
- `optionSet`: Either explicit options or a category reference

## Choice Types and Rendering Examples

### 1. Skill Choices

**API Response Example:**
```json
{
  "id": "fighter_skills",
  "description": "Choose 2 skills",
  "chooseCount": 2,
  "choiceType": "CHOICE_TYPE_SKILL",
  "optionSet": {
    "case": "explicitOptions",
    "value": {
      "options": [
        {"optionType": {"case": "item", "value": {"itemId": "acrobatics", "name": "Acrobatics"}}},
        {"optionType": {"case": "item", "value": {"itemId": "animal-handling", "name": "Animal Handling"}}},
        {"optionType": {"case": "item", "value": {"itemId": "athletics", "name": "Athletics"}}},
        {"optionType": {"case": "item", "value": {"itemId": "intimidation", "name": "Intimidation"}}}
      ]
    }
  }
}
```

**UI Rendering:**
- Show checkboxes for each skill option
- Enforce exactly 2 selections
- Display skill names with descriptions if available
- Consider grouping by ability score (Strength skills, Dexterity skills, etc.)

### 2. Equipment Choices - Simple Options

**API Response Example:**
```json
{
  "id": "barbarian_equipment_1",
  "description": "(a) a greataxe or (b) any martial melee weapon",
  "chooseCount": 1,
  "choiceType": "CHOICE_TYPE_EQUIPMENT",
  "optionSet": {
    "case": "explicitOptions",
    "value": {
      "options": [
        {
          "optionType": {
            "case": "countedItem",
            "value": {"itemId": "greataxe", "name": "Greataxe", "quantity": 1}
          }
        },
        {
          "optionType": {
            "case": "nestedChoice",
            "value": {
              "choice": {
                "id": "nested_martial-weapons_any_martial_melee_weapon",
                "description": "any martial melee weapon",
                "chooseCount": 1,
                "choiceType": "CHOICE_TYPE_EQUIPMENT",
                "optionSet": {
                  "case": "categoryReference",
                  "value": {"categoryId": "martial-weapons", "excludeIds": []}
                }
              }
            }
          }
        }
      ]
    }
  }
}
```

**UI Rendering:**
- Radio buttons for mutually exclusive choices
- For nested choices, show "Choose from Martial Weapons" with a dropdown or modal
- Display item quantities clearly
- Show item descriptions/stats on hover or in expandable sections

### 3. Equipment Choices - Bundle Options

**API Response Example:**
```json
{
  "id": "fighter_equipment_2",
  "description": "(a) leather armor, longbow, and 20 arrows or (b) chain mail",
  "chooseCount": 1,
  "choiceType": "CHOICE_TYPE_EQUIPMENT",
  "optionSet": {
    "case": "explicitOptions",
    "value": {
      "options": [
        {
          "optionType": {
            "case": "bundle",
            "value": {
              "items": [
                {"itemId": "leather-armor", "name": "Leather Armor", "quantity": 1},
                {"itemId": "longbow", "name": "Longbow", "quantity": 1},
                {"itemId": "arrow", "name": "Arrow", "quantity": 20}
              ]
            }
          }
        },
        {
          "optionType": {
            "case": "countedItem",
            "value": {"itemId": "chain-mail", "name": "Chain Mail", "quantity": 1}
          }
        }
      ]
    }
  }
}
```

**UI Rendering:**
- Radio buttons for bundle vs. single item
- For bundles, show all items as a grouped list
- Clearly indicate quantities (especially for consumables like arrows)
- Consider showing total weight/value for bundles

### 4. Category Reference Choices

**API Response Example:**
```json
{
  "id": "wizard_cantrips",
  "description": "Choose 3 cantrips from the wizard spell list",
  "chooseCount": 3,
  "choiceType": "CHOICE_TYPE_SPELL",
  "optionSet": {
    "case": "categoryReference",
    "value": {
      "categoryId": "wizard-cantrips",
      "excludeIds": []
    }
  }
}
```

**UI Rendering:**
- Fetch items from the category using the ListSpellsByLevel API
- Filter: `level: 0, class: CLASS_WIZARD`
- Show as searchable/filterable list
- Enable multiple selection with count enforcement
- Consider spell school grouping and search functionality

### 5. Language Choices

**API Response Example:**
```json
{
  "id": "human_language_choice",
  "description": "Choose 1 additional language",
  "chooseCount": 1,
  "choiceType": "CHOICE_TYPE_LANGUAGE",
  "optionSet": {
    "case": "categoryReference",
    "value": {
      "categoryId": "additional_languages",
      "excludeIds": ["common"]
    }
  }
}
```

**UI Rendering:**
- Dropdown or searchable list
- Exclude languages already known (check excludeIds)
- Group by type (Standard vs. Exotic languages)

## Rendering Patterns by Choice Type

### Radio Buttons (chooseCount = 1)
```typescript
if (choice.chooseCount === 1) {
  return <RadioGroup options={choice.options} onChange={handleSelection} />;
}
```

### Checkboxes (chooseCount > 1)
```typescript
if (choice.chooseCount > 1) {
  return (
    <CheckboxGroup 
      options={choice.options} 
      maxSelections={choice.chooseCount}
      onChange={handleMultiSelection}
    />
  );
}
```

### Category Reference Resolution
```typescript
if (choice.optionSet.case === "categoryReference") {
  const { categoryId, excludeIds } = choice.optionSet.value;
  
  // Map category to API call
  const apiCall = getCategoryApiCall(categoryId, choice.choiceType);
  const items = await apiCall();
  
  // Filter excluded items
  const availableItems = items.filter(item => !excludeIds.includes(item.id));
  
  return <ItemSelector items={availableItems} maxSelections={choice.chooseCount} />;
}
```

## Category ID to API Call Mapping

```typescript
function getCategoryApiCall(categoryId: string, choiceType: ChoiceType) {
  switch (choiceType) {
    case "CHOICE_TYPE_SPELL":
      if (categoryId.endsWith("-cantrips")) {
        const className = categoryId.replace("-cantrips", "");
        return () => listSpellsByLevel({ level: 0, class: className });
      }
      // Handle other spell categories
      break;
      
    case "CHOICE_TYPE_EQUIPMENT":
      return () => listEquipmentByType({ equipmentType: categoryId });
      
    case "CHOICE_TYPE_LANGUAGE":
      // Languages might need a custom endpoint or predefined list
      return () => getLanguageList();
      
    default:
      throw new Error(`Unsupported choice type: ${choiceType}`);
  }
}
```

## Common Equipment Categories

| Category ID | API Call | Description |
|-------------|----------|-------------|
| `martial-weapons` | `ListEquipmentByType(MARTIAL_MELEE_WEAPON)` | Martial melee weapons |
| `simple-weapons` | `ListEquipmentByType(SIMPLE_MELEE_WEAPON)` | Simple melee weapons |
| `martial-ranged-weapons` | `ListEquipmentByType(MARTIAL_RANGED_WEAPON)` | Martial ranged weapons |
| `simple-ranged-weapons` | `ListEquipmentByType(SIMPLE_RANGED_WEAPON)` | Simple ranged weapons |
| `light-armor` | `ListEquipmentByType(LIGHT_ARMOR)` | Light armor |
| `medium-armor` | `ListEquipmentByType(MEDIUM_ARMOR)` | Medium armor |
| `heavy-armor` | `ListEquipmentByType(HEAVY_ARMOR)` | Heavy armor |
| `shields` | `ListEquipmentByType(SHIELD)` | Shields |
| `adventuring-gear` | `ListEquipmentByType(ADVENTURING_GEAR)` | General adventuring equipment |

## Error Handling

### Validation Errors
```typescript
interface ChoiceValidation {
  isValid: boolean;
  errors: string[];
  warnings: string[];
}

function validateChoice(choice: Choice, selections: string[]): ChoiceValidation {
  const errors: string[] = [];
  
  if (selections.length !== choice.chooseCount) {
    errors.push(`Must select exactly ${choice.chooseCount} option(s)`);
  }
  
  // Add more validation logic...
  
  return { isValid: errors.length === 0, errors, warnings: [] };
}
```

### API Error Handling
```typescript
try {
  const items = await getCategoryItems(categoryId);
  setAvailableOptions(items);
} catch (error) {
  console.error("Failed to load category items:", error);
  setErrorMessage("Unable to load available options. Please try again.");
}
```

## Best Practices

1. **Progressive Disclosure**: For nested choices, show summaries first, then expand details
2. **Search & Filter**: For large option lists (spells, equipment), provide search functionality
3. **Visual Grouping**: Group related options (by school, type, etc.)
4. **Validation Feedback**: Show real-time validation as users make selections
5. **Persistence**: Save partial selections as users navigate between choices
6. **Loading States**: Show spinners/skeletons while fetching category data
7. **Accessibility**: Ensure proper ARIA labels and keyboard navigation

## Example React Component Structure

```typescript
interface ChoiceRendererProps {
  choice: Choice;
  onSelectionChange: (choiceId: string, selectedIds: string[]) => void;
  currentSelections: string[];
}

export function ChoiceRenderer({ choice, onSelectionChange, currentSelections }: ChoiceRendererProps) {
  switch (choice.choiceType) {
    case "CHOICE_TYPE_SKILL":
      return <SkillChoice choice={choice} onSelectionChange={onSelectionChange} currentSelections={currentSelections} />;
    
    case "CHOICE_TYPE_EQUIPMENT":
      return <EquipmentChoice choice={choice} onSelectionChange={onSelectionChange} currentSelections={currentSelections} />;
    
    case "CHOICE_TYPE_SPELL":
      return <SpellChoice choice={choice} onSelectionChange={onSelectionChange} currentSelections={currentSelections} />;
    
    case "CHOICE_TYPE_LANGUAGE":
      return <LanguageChoice choice={choice} onSelectionChange={onSelectionChange} currentSelections={currentSelections} />;
    
    default:
      return <GenericChoice choice={choice} onSelectionChange={onSelectionChange} currentSelections={currentSelections} />;
  }
}
```

This structure provides a clean separation of concerns and makes it easy to customize rendering for each choice type while maintaining consistent behavior.