---
name: character handler
description: gRPC handler for CharacterService — character creation, management, and data loading
updated: 2026-09-04
confidence: medium-high — #897 complete Appearance conversion/delegation and Data-owned persistence are verified by focused handler/converter tests and Docker-backed character integration
---

# character handler

The character handler is the gRPC adapter for `CharacterService`. It covers the full character creation lifecycle (draft → finalize), character management (equip/unequip), and data loading for the character creation UI (list races, classes, backgrounds, equipment, spells).

## Subclass choices and sheet proficiency ranks

`UpdateClass` preserves the IDs of subclass skill and language answers in toolkit
`ClassChoices.SubclassChoices`; base class skills keep their existing path.
Draft responses expose subclass answers in the class choice envelope so clients
can reload and resubmit them without losing their origin. Toolkit owns eligibility,
counts, and grants. Character responses retain languages and the provider's
`shared.Expert` rank through `Proficiencies.expertise_skills`, alongside the existing
skills list. This shared mapping also covers Bard/Rogue expertise.

The Knowledge creation integration test exercises save, reload, resubmit,
finalization, and returned languages, spells, and doubled skill ranks.

## Dwarf race tool choices (#728)

`UpdateRace` translates `CHOICE_CATEGORY_TOOLS` through the same canonical proto-to-toolkit tool converter used by class choices and passes the resulting selection IDs into `RaceChoices.Tools`. The toolkit remains responsible for validating Dwarf choice eligibility and completeness. Handler coverage pins Smith's Tools translation, while the character integration suite drives Dwarf race selection through `FinalizeDraft` to prevent a successful-but-discarded choice regression.

## Complete Appearance delegation (#897)

`UpdateAppearance` is creation-only: its request names a draft, never a finalized
character. The handler checks only the request envelope, converts the complete wire
Appearance through `internal/converters/customization`, delegates once, translates
toolkit errors, and converts the returned authoritative `DraftData`. It does not
validate provider refs, colors, roughness, defaults, or oneof semantics.

Appearance is nested in toolkit `character.Data`/`DraftData`. Draft reads,
`UpdateAppearance`, `FinalizeDraft`, `GetCharacter`, `ListCharacters`, and equipment
responses all project `Data.Appearance` directly. The converter preserves nil/empty
messages, style/none/malformed oneofs, optional scalar presence including zero, and
both outfit channels; deprecated string fields remain inert. Toolkit `Draft.SetAppearance`
owns refusal before the repository update.

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
matching detached View. Legacy Equip/Unequip convert the persisted entity's nested
`Data.Appearance`; they no longer call `GetCharacter` after a successful write. Internal strict projection
failures become generic INTERNAL `character data unavailable` at this boundary.

The owner-private `CharacterData` contract is translated only by
`internal/handlers/dnd5e/v2/character/character_data.go`: PlayerID, structured
class/race refs, equipment, level, HP, speed, structured feature/condition refs,
optional resource/source presence, and non-magical resources are copied from detached
values. The v1alpha1 handler does not grow a parallel converter for those fields.

The current branch pins proto generated commit `883dd221a6cdf724df8d5d993d897e0c8a3358ab`,
`rulebooks/dnd5e` v0.137.0, `rulebooks/dnd5e/session` v0.53.1, and
`rulebooks/dnd5e/resolution` v0.32.1. The session module is consumed directly by the
separate `SessionService` handler.

## Files

| File | Purpose |
|---|---|
| `handlers/dnd5e/v1alpha1/character/handler.go` | gRPC handler |
| `handlers/dnd5e/v1alpha1/character/converters.go` | Proto ↔ domain entity conversion |
| `handlers/dnd5e/v1alpha1/character/level_up.go` | The two advancement verbs: proto ↔ session SDK, nothing else |

## gRPC methods handled

- `CreateDraft` — starts a new character creation draft
- `GetDraft` / `ListDrafts` / `DeleteDraft` — draft lifecycle
- `GetRequirements` — returns pending choices for a draft
- `SetName` / `SetRace` / `SetClass` / `SetBackground` / `SetAbilityScores` — draft updates
- `SetAbilityScoresFromRolls` — assigns pre-rolled ability scores to draft
- `UpdateAppearance` — delegates complete creation-draft Appearance to the toolkit
- `ValidateDraft` / `FinalizeDraft` — validation and finalization
- `GetCharacter` / `ListCharacters` / `DeleteCharacter` — character CRUD
- `EquipItem` / `UnequipItem` — equipment slot management
- `ListRaces` / `ListClasses` / `ListBackgrounds` — UI data loading
- `ListEquipmentByType` / `ListSpells` — equipment and spell catalog
- `RollAbilityScores` — rolls 4d6-drop-lowest ability scores
- `StartDiceSession` — creates a dice session for ability score rolling
- `GetNextLevel` / `LevelUp` — advancement; see below

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

## Advancement calls the session SDK (rpg-project#452)

`GetNextLevel` and `LevelUp` are **pure translation**, and this is the one place
in this service that talks to the toolkit's session SDK besides the session
handler. Kirk's ruling: *"the API is dumb... we should not need an orchestrator
in API anymore and our level up should be contained in our session package"*
(design R6.1).

Each method binds the calling player, calls exactly ONE Manager verb, and
projects the answer. It loads no sheet, decides nothing, and orders no toolkit
steps. There is no character orchestrator involvement beyond the reads named
below.

The Manager arrives as `HandlerConfig.Sessions`, a two-method interface
declared at the point of use (`Sessions` in `level_up.go`) rather than the
SDK's concrete `*session.Manager` — the same shape and the same reason as the
session handler's own `Manager` interface: it is what lets these tests fake a
verb's outcome without a real Manager, Redis and roller behind them.
`*session.Manager` satisfies it structurally.

Three things stay on this side of the seam, because they are transport's:

1. **Ownership of the calling player.** The SDK is handed a character id and
   has no notion of who holds the connection. The gate answers NOT_FOUND, never
   PERMISSION_DENIED, so a foreign character is indistinguishable from a
   missing one.
2. **Refusing `HIT_POINT_METHOD_UNSPECIFIED`** before the SDK is touched.
   Rolled and averaged write different numbers into a record that is never
   corrected, so there is no defensible default. `MAX` is level-1 only and is
   not on the wire at all.
3. **Re-reading the sheet after a successful level.** The SDK returns no
   `character.Data` by its own boundary law — it reports that it saved — so the
   projected `Character` comes from `GetCharacter`, which also means the client
   is shown what is actually stored.

Error codes come from `handlers/dnd5e/sdkerr`, the one shared SDK translation
table (design rule 7), not from a second table here. The toolkit's own sentence
is preserved: it names the experience total and the threshold, or the spell
already known, and that is what the player reads.

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
