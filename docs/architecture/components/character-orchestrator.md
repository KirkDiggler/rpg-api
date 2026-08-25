---
name: character orchestrator
description: Character creation, management, equipment, and data-loading orchestrator
updated: 2026-08-25
confidence: high — #844 strict project-before-write path verified by focused no-write, post-projection, persistence-equality, handler, and lint gates
---

# character orchestrator

The character orchestrator handles character creation (draft lifecycle), character management (equip/unequip, finalize), and data loading for the character creation UI (list races, classes, backgrounds, equipment, spells).

## Files

| File | Lines | Purpose |
|---|---|---|
| `orchestrators/character/service.go` | ~200 | Service interface + all Input/Output types |
| `orchestrators/character/orchestrator.go` | 1,083 | Implementation |

## Purpose

- **Draft lifecycle:** create → update (name, race, class, background, ability scores, appearance) → validate → finalize → `character.Data` in Redis.
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
    ├── characterrepo.Repository         — get/save character.Data (Redis)
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
- `*entities.CharacterDraft` — in-progress creation state in Redis
- `*entities.Appearance` — cosmetic character data, stored alongside `character.Data`

## Equipment (rpg-api#680)

`EquipItem`/`UnequipItem` are the ONE rules-correct equip path — the v1alpha1
`CharacterService` and the new v1alpha2 `CharacterService`
(`internal/handlers/dnd5e/v2/character/`) both call these same two methods. Previously
they were a bare `charData.EquipmentSlots.Set/Clear(...)` — no rules, no occupancy, no
validation, no recompute. That's gone.

The method shape:

1. `characterRepo.Get` — load the persisted `*character.Data`.
2. Shallow-copy the loaded `character.Data`, clone its `EquipmentSlots` map, then
   strictly `character.Load(ctx, workingData)` + `character.Attach(ctx, char, bus)` and
   complete detached `EquipmentView` + `StatusView` projection before mutation.
   The toolkit retains and mutates the slots map, so this working copy keeps a
   cached/pointer-returning repository entity unchanged if projection or Update fails.
   Every unrelated map, slice, and opaque JSON value is preserved directly without a
   JSON round trip because Equip/Unequip do not write those containers. Malformed
   conditions, features, catalog items, resources, or unknown status descriptors fail
   the operation before any write; forgiving `LoadFromData` is not used here.
3. `char.EquipItem(slot, itemID)` / `char.UnequipItem(slot)` — the toolkit enforces
   occupancy here (rpg-toolkit#812, v0.67.0): a two-handed weapon claims `main_hand` and
   clears `off_hand`; equipping an occupied slot swaps the previous occupant back to
   inventory (never dropped); an incompatible slot now returns an `rpgerr` (mapped to
   `apierr.InvalidArgument` by `mapEquipError`) instead of silently succeeding.
4. Compose `mergedEquipmentData(ctx, workingData, char)` and the complete detached
   post-mutation View before repository Update, then return that already-composed
   View. The Update payload derives from the isolated working sheet; the repository's
   returned entity is untouched. There is no fallible reload/projection after a
   successful write. See below for why persistence is a merge, not `Data: char.ToData()`.

### Why persistence merges instead of overwriting (a real footgun)

`mergedEquipmentData` copies the isolated working `*character.Data` created in step 2
and refreshes only two fields from the post-mutation runtime character:

- **`EquipmentSlots`** — read via `char.ToData().EquipmentSlots`. Safe: it's a plain
  `map[InventorySlot]string`, no registry resolution involved, round-trips correctly.
- **`ArmorClass`** — set to `char.EffectiveAC(ctx).Total`, so the STORED int stays
  truthful. This matters because the encounter seat's combat AC (`lobby`'s
  `AddPlayer` seeding, `internal/orchestrators/lobby/character.go`) is seeded from this
  same stored int — leaving it stale after an equip would desync combat AC from the
  real display AC the wire now serves (see `docs/architecture/components/encounter.md`'s
  CharacterData projection). Baking `EffectiveAC` into the stored int here is safe
  specifically because this orchestrator method only ever runs on the out-of-encounter
  character sheet, where `Conditions`/`ActionEconomy` carry no live combat buffs
  (Shield, Mage Armor) to accidentally bake in — only permanent sources (armor, DEX,
  features like Unarmored Defense) are ever on the breakdown at that point. Deferred:
  rpg-api#684 (replace the cache with a direct `EffectiveAC` read everywhere, removing
  the desync risk this manages rather than eliminates) and rpg-api#681 (push a live
  update to an already-connected `StreamEncounter` client on an out-of-combat equip —
  no toolkit broker event supports this today).

Every OTHER field is deliberately left exactly as loaded, NOT persisted via a full
`char.ToData()` overwrite. `ToData()`/`LoadFromData()` is lossy for data the toolkit
runtime doesn't model on a round trip:

- `BackgroundID` and `CreatedAt` are never populated by `ToData()` — confirmed by
  reading `character.go`'s `ToData()`: no assignment exists for either. A full
  overwrite silently zeros them.
- API-owned appearance is not represented by `character.Data` or `Character.ToData()`.

The owner-private path now uses the toolkit's strict loader, so an inventory item outside
its catalog is rejected as INTERNAL with no Update rather than silently dropped or
preserved as an unprojectable row. For valid data, `Character.EquipItem`/`UnequipItem`
never touch `Inventory` (only `EquipmentSlots`), so merging those fields onto the
working `Data` preserves every non-equipment field and appearance exactly while the
repository-returned `Data` remains untouched.
Regression coverage: `TestEquipItem_PreservesNonEquipmentFields`,
`TestEquipItem_RejectsUnprojectableDataWithoutWriting`, the Equip/Unequip
post-projection and Update-failure isolation tests, and
`TestEquipItem_SyncsStoredArmorClass` (`internal/orchestrators/character/equip_item_test.go`).

**If this ever gets "simplified" back to `Data: char.ToData()`, the data loss returns
silently** — there's no compiler error, just a character quietly missing fields the
next time someone equips something.

## Production provider pins (#844)

The strict character loader/status projection used here is published in
`rulebooks/dnd5e` v0.100.0. The same API branch consumes the final session combat
providers at `rulebooks/dnd5e/session` v0.30.0 and
`rulebooks/dnd5e/resolution` v0.13.0, with proto generated v0.1.143 (`a7db07a`).
There are no local replaces or API-side rule substitutes: declaration availability,
reach, costs, selectors, character status, and resources all remain provider answers.

## Known issues

### TODO at line 2765: alive-check not implemented

`orchestrator.go:2765` has a comment noting that "alive check logic" for spell list fetching is not implemented. Characters who are dead or unconscious may still have their spell list fetched without error.

### TODO at line 3282 (monster turns)

Actually in encounter orchestrator, not character. The character orchestrator is cleaner. However, the character orchestrator has a TODO at line ~3352 (based on quality.md notes) about "monster turns for entities acting before the current entity" — this may be a cross-reference in comments rather than a character-specific issue.

### Equipment choice logic in wrong layer

`REFACTOR_PLAN.md` (archived, originally at repo root) documents that equipment-choice business logic lives in the handler's external client interface rather than the orchestrator. This smell is present but deferred. The handler at `handler.go:765` has: `//TODO: handler should not interact with toolkit, this belongs in the orchestrator`. Toolkit type assertions in the character handler are the most visible symptom.

### No proto leakage (positive)

Unlike the encounter orchestrator, the character orchestrator does **not** import proto packages. Its Input/Output types in `service.go` use only toolkit types and local entity types. This is the correct pattern.
