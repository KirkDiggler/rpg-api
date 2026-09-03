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

## Equipment (rpg-api#680/#844)

`EquipItem`/`UnequipItem` are the one rules-correct path shared by the v1alpha1 and
v1alpha2 CharacterService handlers. The toolkit owns item/slot validation, occupancy,
and effective AC. rpg-api coordinates strict application and persistence.

The method shape is:

1. `characterRepo.Get` returns the entity plus an opaque record version.
2. The orchestrator copies `character.Data`, clones the retained EquipmentSlots map,
   strictly calls `character.Load` + `character.Attach`, and requires complete detached
   identity, EquipmentView, and StatusView projections before mutation. PlayerID,
   ClassID, and RaceID are required. Malformed conditions, features, catalog items,
   resources, or status descriptors fail before any write.
3. The toolkit `EquipItem`/`UnequipItem` verb mutates the isolated sheet. The complete
   post-view is composed before persistence; there is no fallible projection afterward.
4. `characterRepo.PatchEquipment` receives only CharacterID, expected version, expected
   pre-mutation slots, post-mutation slots, and toolkit-computed cached ArmorClass. It
   never receives a full replacement entity from this path.
5. If an unrelated writer changed the record while equipment remained the same, the
   repository returns the newer entity without writing. The orchestrator strictly
   reapplies the operation to that entity and retries. If equipment itself changed, the
   repository returns ABORTED. On success it returns the actual patched entity.
6. The orchestrator returns that entity plus the matching precomposed View. Legacy
   conversion (including Appearance) and v1alpha2 CharacterData conversion therefore
   consume the same persisted post-state; neither handler performs a post-write Get.

### Atomic two-field persistence

The Redis implementation uses WATCH plus a transactional SET. It compares both the
expected equipment map and opaque version against the latest JSON record. The committed
record is decoded from the latest value and changes only:

- `EquipmentSlots`, cloned from the toolkit's post-mutation occupancy; and
- cached `ArmorClass`, copied from `EffectiveAC(ctx).Total`.

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

The current branch consumes `rulebooks/dnd5e` v0.136.0,
`rulebooks/dnd5e/session` v0.53.0, `rulebooks/dnd5e/resolution` v0.31.0, and
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
