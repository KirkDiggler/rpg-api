---
name: character handler
description: gRPC handler for CharacterService — character creation, management, and data loading
updated: 2026-05-02
confidence: medium-high — verified by reading handler.go and converters.go
---

# character handler

The character handler is the gRPC adapter for `CharacterService`. It covers the full character creation lifecycle (draft → finalize), character management (equip/unequip), and data loading for the character creation UI (list races, classes, backgrounds, equipment, spells).

## Files

| File | Lines | Purpose |
|---|---|---|
| `handlers/dnd5e/v1alpha1/character/handler.go` | 1,070 | gRPC handler |
| `handlers/dnd5e/v1alpha1/character/converters.go` | 3,132 | Proto ↔ domain entity conversion |

`converters.go` is the largest file in the codebase by line count.

## gRPC methods handled

- `CreateDraft` — starts a new character creation draft
- `GetDraft` / `ListDrafts` / `DeleteDraft` — draft lifecycle
- `GetRequirements` — returns pending choices for a draft
- `SetName` / `SetRace` / `SetClass` / `SetBackground` / `SetAbilityScores` — draft updates
- `SetAbilityScoresFromRolls` — assigns pre-rolled ability scores to draft
- `SetAppearance` — sets cosmetic character appearance
- `ValidateDraft` / `FinalizeDraft` — validation and finalization
- `GetCharacter` / `ListCharacters` / `DeleteCharacter` — character CRUD
- `EquipItem` / `UnequipItem` — equipment slot management
- `ListRaces` / `ListClasses` / `ListBackgrounds` — UI data loading
- `ListEquipmentByType` / `ListSpells` — equipment and spell catalog
- `RollAbilityScores` — rolls 4d6-drop-lowest ability scores
- `StartDiceSession` — creates a dice session for ability score rolling

## Converter surface

`converters.go` at 3,132 lines is the conversion layer for the character domain. This size is expected given the breadth of the D&D 5e character model (races, classes, backgrounds, spells, equipment, traits, features, skills, proficiencies). However, the file has **27 TODO comments** indicating incomplete conversions.

### Known stub returns

- `SPELL_UNSPECIFIED` returned for all spell mappings (lines 344, 378) — spell enum not yet mapped
- `TRAIT_UNSPECIFIED` returned for all trait conversions (line 378)
- No language enum conversions — language fields return empty (line 1169)
- No subrace conversions — subrace data not mapped (line 782)
- Tool proficiency proto enums not mapped (line 899)
- Spell slot conversion not implemented (line 1182)
- Class resource conversion not implemented (line 1186)
- Equipment data incomplete for armor and tools (line 798)

These stubs silently return zero/unspecified values without errors, meaning the character API returns structurally valid but semantically incomplete data for these fields.

## Known issues

### Toolkit type assertion in handler

`handler.go:765` has a TODO acknowledging the smell:
```go
//TODO: handler should not interact with toolkit, this belongs in the orchestrator
if charData, ok := member.CharacterData.(*toolkitchar.Data); ok {
```

The character orchestrator returns `interface{}` for `CharacterData` in some output structs. The handler must type-assert against `*toolkitchar.Data` to convert it. This couples the handler to the toolkit type — the orchestrator should return a typed struct.

### Test coverage gap

`converters.go` (3,132 lines) has limited dedicated unit tests relative to its size. `list_equipment_test.go` and `list_spells_test.go` cover some paths. The 27 TODO stubs are a signal that the converter surface grew faster than its test coverage. A comprehensive converter test suite would catch stub regressions.

### Handler test files

`handler.go` has 8 TODO comments (lines 236, 294, 350, 765, 798, 836, 858) covering spell enum conversion, tool expertise, and pagination. These are handler-level gaps beyond the converter stubs.
