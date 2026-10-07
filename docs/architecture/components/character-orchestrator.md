---
name: character orchestrator
description: Character creation, management, equipment, and data-loading orchestrator
updated: 2026-09-04
confidence: high — #897 adds complete toolkit-owned Appearance delegation/storage and Docker-backed integration evidence while retaining #844's strict equipment evidence
---

# character orchestrator

The character orchestrator handles character creation (draft lifecycle), character management (equip/unequip, finalize), and data loading for the character creation UI (list races, classes, backgrounds, equipment, spells).

## Files

| File | Purpose |
|---|---|
| `orchestrators/character/service.go` | Service interface + all Input/Output types |
| `orchestrators/character/orchestrator.go` | Implementation |

## Purpose

- **Draft lifecycle:** create → update (name, race, class, background, ability scores, Appearance) → validate → finalize → toolkit `character.Data`/`DraftData` in Redis.
- **Character management:** equip/unequip through the toolkit's rules engine (rpg-api#680 — see "Equipment" below, this used to be a bare data write); get/list/delete characters.
- **Data loading:** list races, classes, backgrounds, equipment by type, spells, ability scores — delegates to rpg-toolkit for actual data.

## Public interface

```go
type Service interface {
    // Draft lifecycle
    CreateDraft(ctx, *CreateDraftInput) (*CreateDraftOutput, error)
    GetDraft(ctx, *GetDraftInput) (*GetDraftOutput, error)
    ListDrafts(ctx, *ListDraftsInput) (*ListDraftsOutput, error)
    DeleteDraft(ctx, *DeleteDraftInput) (*DeleteDraftOutput, error)
    GetRequirements(ctx, *GetRequirementsInput) (*GetRequirementsOutput, error)
    SetName / SetRace / SetClass / SetBackground / SetAbilityScores
    SetAbilityScoresFromRolls / SetAppearance
    ValidateDraft / FinalizeDraft

    // Character operations
    GetCharacter / ListCharacters / DeleteCharacter
    EquipItem / UnequipItem


    // Data loading for UI
    ListRaces / ListClasses / ListBackgrounds / ListEquipmentByType
    RollAbilityScores / ListSpells
}
```

## Dependencies

```
Orchestrator
    ├── characterrepo.Repository         — CRUD plus atomic equipment patch (Redis)
    ├── characterdraftrepo.Repository     — get/save CharacterDraft (Redis)
    ├── dicesessionrepo.Repository        — read dice roll results
    ├── dice.Roller                        — ability score rolling
    ├── clock.Clock                        — timestamps (injectable for testing)
    └── rpg-toolkit packages:
         ├── character                     — character.Data type, FinalizeDraft
         ├── classes                       — class data, grants, starting equipment
         ├── races                         — race data, ability modifiers
         ├── backgrounds                   — background data, skill grants
         ├── abilities                     — ability score calculation
         └── choices                       — character creation choice resolution
```

## Internal data model

The orchestrator works with:
- `*character.Data` (toolkit type) — stored and loaded from Redis directly
- `*entities.CharacterDraft` — a storage wrapper around toolkit `DraftData`
- `customization.Appearance` nested in toolkit `Data`/`DraftData`

## Appearance lifecycle (#897)

The orchestrator's `SetAppearance` method is reachable only from the creation RPC
`UpdateAppearance` and accepts a draft ID plus toolkit `customization.Appearance`. It
loads `DraftData`, calls `Draft.SetAppearance` once, persists `draft.ToData()`, and
returns the repository's complete stored `DraftData`. Toolkit validation refuses
malformed semantic values before `Update`.

The Redis repositories serialize the toolkit data inside the thin API wrapper. Reload
and present-zero tests prove the complete Appearance shape, including outfit channels.
`GetCharacter`, `ListCharacters`, finalization, equipment patches, and Session SDK saves
carry `Data.Appearance` naturally; no sibling envelope or API-side preservation merge is
used.

## Advancement lives in the session SDK, not here (rpg-project#452)

**This orchestrator holds nothing about level-up, deliberately.** Kirk's ruling
after the wave walked: *"the API is dumb... we added the session package to act
as the SDK to the API. So we should not need an orchestrator in API anymore and
our level up should be contained in our session package."* (design R6.1/R6.2).

The first build put the verb here — load the sheet, call `Character.Advance`,
save — with a `Config.Roller` for rolled hit points and the next level computed
locally to look up grants. Every rule in it was the toolkit's even then; the
ORCHESTRATION of those rules was not, and that is what moved. `level_up.go`,
both service methods, their IO types and `Config.Roller` were deleted rather
than deprecated.

Advancement is now two verbs on the toolkit session Manager, called directly by
the v1alpha1 character handler. See `character-handler.md`. This orchestrator
is still involved in exactly one way: the handler reads through `GetCharacter`
to bind the calling player, and again after a successful level to project the
stored sheet, because the SDK returns no character by its own boundary law.

## Equipment (rpg-api#680/#844)

`EquipItem`/`UnequipItem` are the one rules-correct path shared by the v1alpha1 and
v1alpha2 CharacterService handlers. The toolkit owns item/slot validation, occupancy,
and effective AC. rpg-api coordinates strict application and persistence.

The method shape is:

1. `characterRepo.Get` returns the entity plus an opaque record version.
2. The orchestrator copies `character.Data`, clones the retained EquipmentSlots map,
   strictly calls `character.Load` (no bus: equip, unequip and the character view no
   longer attach a sheet; `FinalizeDraft` still does, via `ToCharacter` and a lenient
   `LoadFromData`, until rpg-toolkit#1965 tier-2 F absorbs it), and
   requires complete detached identity, EquipmentView, and StatusView projections
   before mutation. The EquipmentView and the folded armour class come from
   `resolution.ProjectCharacter` over the sheet's record. PlayerID,
   ClassID, and RaceID are required. Malformed conditions, features, catalog items,
   resources, or status descriptors fail before any write.
3. The toolkit `EquipItem`/`UnequipItem` verb mutates the isolated sheet. The complete
   post-view and its folded armour class are composed before persistence; a refused
   fold refuses the equip (no fallback AC is written), and there is no fallible
   projection afterward.
4. `characterRepo.PatchEquipment` receives only CharacterID, expected version, expected
   pre-mutation slots, post-mutation slots, and (when the toolkit changed them) the
   post-equipment conditions. It never receives a full replacement entity or an armour
   class from this path.
5. If an unrelated writer changed the record while equipment remained the same, the
   repository returns the newer entity without writing. The orchestrator strictly
   reapplies the operation to that entity and retries. If equipment itself changed, the
   repository returns ABORTED. On success it returns the actual patched entity.
6. The orchestrator returns that entity, the matching precomposed View, and the
   post-state's folded armour class (`EquipItemOutput.ArmorClass`). Legacy conversion
   (including Appearance and `CombatStats.armor_class`) and v1alpha2 CharacterData
   conversion therefore consume the same post-state; neither handler performs a
   post-write Get.

### Atomic equipment persistence

The Redis implementation uses WATCH plus a transactional SET. It compares both the
expected equipment map and opaque version against the latest JSON record. The committed
record is decoded from the latest value and changes only:

- `EquipmentSlots`, cloned from the toolkit's post-mutation occupancy; and
- `Conditions`, only when the toolkit's post-equipment state differs.

No armour class is written: `character.Data` no longer has the field
(rpg-toolkit#1971), and an old record carrying `armor_class` loads with the key
ignored and is rewritten without it on its next save (rpg-project#538 R13; no
migration).

## Armour class is a projection (rpg-project#538 slice 5)

rpg-api stores no armour class. Every response that carries one fills it from
`resolution.ProjectCharacter`, the toolkit door that installs the cast a monk's or
barbarian's Unarmored Defense reads (a monk answers 10 + DEX + WIS):

| verb | where the fold happens | output field |
|---|---|---|
| `FinalizeDraft` | `projectArmorClass` over the record `Create` returned | `FinalizeDraftOutput.ArmorClass` |
| `GetCharacter` (also LevelUp's re-read and every ownership gate) | `projectArmorClass` over the stored record | `GetCharacterOutput.ArmorClass` |
| `ListCharacters` | `projectArmorClasses`, one fold per listed record | `ListCharactersOutput.ArmorClasses` (keyed by character ID) |
| `EquipItem` / `UnequipItem` | the post-state `projectLoaded` fold already composed before the patch | `EquipItemOutput.ArmorClass` / `UnequipItemOutput.ArmorClass` |

`projectArmorClass` (`view.go`) asks the door alone, not the full View: the status half
is a separate toolkit question with its own refusals, and a response that only carries
`CombatStats` must not fail on it. The door attaches strictly, so an unreadable sheet is
refused; a refusal fails the request as INTERNAL `character data unavailable` (R11).
`ListCharacters` fails whole on the first refused sheet, with the transport message
`character data unavailable: character "<id>"`; there is no per-row absence and no
fallback number. There is no cache (R7; a cached projection is R9's, deferred until a
measured cost asks). One fold is one strict attach plus the AC fold and the equipment
view's second AC fold, measured at roughly 30µs and 14KB per character, so a list of N
characters costs N of them.

Because `GetCharacter` folds, the v1alpha1 ownership gates (`verifyCallerOwnsCharacter`
on GetNextLevel/LevelUp) and `GetCharacterInventory` also refuse a sheet the door
refuses, and LevelUp folds twice (gate, then re-read).

HP, resources, conditions, action economy, inventory, identity, metadata, and nested
`Data.Appearance` come from the latest stored toolkit data and are not replaced by the
orchestrator's earlier snapshot. This also avoids the known lossiness of a full
`Character.ToData()` overwrite for non-round-tripped fields.

Regression coverage in `equip_item_test.go` proves pre/post strict projection, map
isolation, patch-only inputs, retry over an unrelated combat-state revision, stale patch
errors, and persisted entity/View agreement for Equip and Unequip. Repository miniredis
tests prove a concurrent combat update survives and stale expected equipment cannot
replace newer slots or other data.

## Production provider pins (#844)

The current branch consumes `rulebooks/dnd5e` v0.137.0,
`rulebooks/dnd5e/session` v0.53.1, `rulebooks/dnd5e/resolution` v0.32.1, and
proto generated commit `883dd221a6cdf724df8d5d993d897e0c8a3358ab`.
There are no local replaces or API-side rule substitutes: declaration availability,
reach, costs, selectors, character status, and resources all remain provider answers.

## Known issues

Verified remaining orchestrator TODOs are limited to draft state mutation access,
background validation, error logging, and pagination/class-filter placeholders. The
legacy handler still contains an explicit toolkit-boundary TODO in `handler.go`;
that is outside the strict owner-private equipment path documented here.

### No proto leakage (positive)

Unlike the encounter orchestrator, the character orchestrator does **not** import proto packages. Its Input/Output types in `service.go` use only toolkit types and local entity types. This is the correct pattern.
