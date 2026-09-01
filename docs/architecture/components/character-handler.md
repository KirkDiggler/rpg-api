---
name: character handler
description: gRPC handler for CharacterService — character creation, management, and data loading
updated: 2026-09-01
confidence: medium-high — #869 hair draft/finalization/Get/List mapping verified through focused handler tests and the Docker-backed character integration flow; older converter debt retains its stated caveats
---

# character handler

The character handler is the gRPC adapter for `CharacterService`. It covers the full character creation lifecycle (draft → finalize), character management (equip/unequip), and data loading for the character creation UI (list races, classes, backgrounds, equipment, spells).

## Dwarf race tool choices (#728)

`UpdateRace` translates `CHOICE_CATEGORY_TOOLS` through the same canonical proto-to-toolkit tool converter used by class choices and passes the resulting selection IDs into `RaceChoices.Tools`. The toolkit remains responsible for validating Dwarf choice eligibility and completeness. Handler coverage pins Smith's Tools translation, while the character integration suite drives Dwarf race selection through `FinalizeDraft` to prevent a successful-but-discarded choice regression.

## Production hair customization (#869)

`UpdateAppearance` is creation-only: its request names a draft, never a finalized
character. The handler validates the complete wire payload before invoking the
orchestrator, then maps the shared customization proto through
`internal/converters/customization`; it does not interpret provider refs.

Appearance is an API-owned envelope outside toolkit `character.Data`. Draft reads,
`UpdateAppearance`, `FinalizeDraft`, `GetCharacter`, and `ListCharacters` return the
same typed hair semantics: nil slot pointer means provider default, `none` is explicit,
`style_ref` is exact, and optional `color_srgb`/`roughness` preserve scalar presence
(including present zero). Finalization returns both the reconstructed toolkit character
and the persisted API-owned Appearance so the finalization response does not drop it.
A Docker `redis:7-alpine` integration test completes a Dwarf Fighter and proves the
exact payload across update, reload, finalization, and final character read; it also
proves invalid roughness is refused before repository mutation.

## Shared strict character application (#844)

The v1alpha1 `EquipItem`/`UnequipItem` methods do not retain a legacy mutation path. They
delegate to the same character orchestrator methods as the v1alpha2 owner-private
surface. That shared path strictly loads and attaches an isolated working character,
composes the complete post-mutation identity/equipment/status View **before** an atomic
repository equipment patch, and writes nothing if strict application or post-state
projection fails. The patch changes only EquipmentSlots and cached ArmorClass on the
latest record. This handler does not inspect feature/condition JSON, derive status, or
duplicate toolkit rules.

The orchestrator output carries both the actual persisted post-state entity and its
matching detached View. Legacy Equip/Unequip convert the entity directly with Appearance;
they no longer call `GetCharacter` after a successful write. Internal strict projection
failures become generic INTERNAL `character data unavailable` at this boundary.

The owner-private `CharacterData` contract is translated only by
`internal/handlers/dnd5e/v2/character/character_data.go`: PlayerID, structured
class/race refs, equipment, level, HP, speed, structured feature/condition refs,
optional resource/source presence, and non-magical resources are copied from detached
values. The v1alpha1 handler does not grow a parallel converter for those fields.

The current branch pins proto generated commit `4a54bd51df0e6459b2908d8f054978cb451416bc`,
`rulebooks/dnd5e` v0.124.0, `rulebooks/dnd5e/session` v0.41.0, and
`rulebooks/dnd5e/resolution` v0.25.0. The session module is consumed directly by the
separate `SessionService` handler.

## Files

| File | Purpose |
|---|---|
| `handlers/dnd5e/v1alpha1/character/handler.go` | gRPC handler |
| `handlers/dnd5e/v1alpha1/character/converters.go` | Proto ↔ domain entity conversion |

## gRPC methods handled

- `CreateDraft` — starts a new character creation draft
- `GetDraft` / `ListDrafts` / `DeleteDraft` — draft lifecycle
- `GetRequirements` — returns pending choices for a draft
- `SetName` / `SetRace` / `SetClass` / `SetBackground` / `SetAbilityScores` — draft updates
- `SetAbilityScoresFromRolls` — assigns pre-rolled ability scores to draft
- `UpdateAppearance` — validates and sets creation-draft hair appearance
- `ValidateDraft` / `FinalizeDraft` — validation and finalization
- `GetCharacter` / `ListCharacters` / `DeleteCharacter` — character CRUD
- `EquipItem` / `UnequipItem` — equipment slot management
- `ListRaces` / `ListClasses` / `ListBackgrounds` — UI data loading
- `ListEquipmentByType` / `ListSpells` — equipment and spell catalog
- `RollAbilityScores` — rolls 4d6-drop-lowest ability scores
- `StartDiceSession` — creates a dice session for ability score rolling

## Converter surface

`converters.go` is the conversion layer for the broad D&D 5e character domain (races, classes, backgrounds, spells, equipment, traits, features, skills, and proficiencies). Several conversions remain explicitly incomplete.

### Known stub returns

- `SPELL_UNSPECIFIED` returned for spell mappings — spell enum not yet mapped
- `TRAIT_UNSPECIFIED` returned for trait conversions
- No language enum conversions — language fields return empty
- No subrace conversions — subrace data not mapped
- Spell slot conversion not implemented
- Class resource conversion not implemented
- Equipment data incomplete for armor and tools

These stubs silently return zero/unspecified values without errors, meaning the character API returns structurally valid but semantically incomplete data for these fields.

## Known issues

### Toolkit type assertion in handler

`handler.go` has a TODO acknowledging the smell:
```go
//TODO: handler should not interact with toolkit, this belongs in the orchestrator
if charData, ok := member.CharacterData.(*toolkitchar.Data); ok {
```

The character orchestrator returns `interface{}` for `CharacterData` in some output structs. The handler must type-assert against `*toolkitchar.Data` to convert it. This couples the handler to the toolkit type — the orchestrator should return a typed struct.

### Test coverage gap

`converters.go` has limited dedicated unit tests relative to its breadth. `list_equipment_test.go` and `list_spells_test.go` cover some paths, but the explicitly incomplete conversions show that coverage has not kept pace with the surface. A comprehensive converter test suite would catch stub regressions.

### Handler test files

Remaining handler TODOs cover spell enum conversion, tool expertise, and pagination. These are handler-level gaps beyond the converter stubs.
