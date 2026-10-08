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
- **Character management:** equip/unequip through the session SDK's Equip/Unequip verbs (rpg-project#542 — see "Equipment" below); get/list/delete characters.
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

## Equipment (rpg-project#542)

`EquipItem`/`UnequipItem` are the one equip path shared by the v1alpha1 and v1alpha2
CharacterService handlers, and they hold no equipment rule. Each calls the session
SDK's `Equip`/`Unequip` verb through the required `Config.Equipment` capability
(`equipment.go`; `*sdk.Manager` in production), then projects armour class from the
record the verb saved (`EquipOutput.Character` → `resolution.ProjectCharacter`). Nothing
is read or written around the verb.

The verb owns everything the change means: it takes the guard the character's seat
decides (its own guard when unseated, the session's when seated), applies the
rulebook's change (occupancy, two-handed weapons, a swap), prices it on the member's
turn in a fight (stow = the action, draw = the object interaction), saves the record
through its one sheet store, tells the equip beat to everyone who perceives the
member and rechecks sight. That is why the equipment patch, its version check, its
retry loop and the appearance notifier are gone: each was a copy of something the
verb now owns.

The verb's refusals reach the client through the SDK's one translation table
(`internal/handlers/dnd5e/sdkerr`): `ErrNotYourTurn`, `ErrDowned`, `ErrCannotAfford`,
`ErrArmorInFight` → FAILED_PRECONDITION; `ErrBadEquip` → INVALID_ARGUMENT;
`ErrNoCharacter` → NOT_FOUND. A failure that maps to INTERNAL keeps the handler's
sanitized `character data unavailable` text. A projection failure after a successful
verb answers the call as failed with the change already durable.

`EquipItemOutput.PreviousItemID` (v1alpha1's `previously_equipped_item`) is the bare id
of the first item the verb put away, or empty.

## Armour class is a projection (rpg-project#538 slice 5)

rpg-api stores no armour class. Every response that carries one fills it from
`resolution.ProjectCharacter`, the toolkit door that installs the cast a monk's or
barbarian's Unarmored Defense reads (a monk answers 10 + DEX + WIS):

| verb | where the fold happens | output field |
|---|---|---|
| `FinalizeDraft` | the fold of the serialized sheet, BEFORE `Create`: a refusal saves nothing and the draft stands | `FinalizeDraftOutput.ArmorClass` |
| `GetCharacter` (also LevelUp's re-read) | `projectArmorClass` over the stored record | `GetCharacterOutput.ArmorClass` |
| `ListCharacters` | `projectArmorClasses`, one fold per listed record | `ListCharactersOutput.ArmorClasses` (keyed by character ID) |
| `EquipItem` / `UnequipItem` | `projectLoaded` over the record the SDK verb saved | `EquipItemOutput.ArmorClass` / `UnequipItemOutput.ArmorClass` |

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

Ownership gates never fold. The v1alpha1 and v1alpha2 `verifyCallerOwnsCharacter` and
`GetCharacterInventory` read `GetCharacterRecord`, the unprojected record. A fold before
the owner check would answer an unprojectable foreign sheet INTERNAL where a missing id
answers NOT_FOUND, which is the existence oracle rpg-api#815 closed. The integration test
`ownership_oracle_test.go` pins that GetNextLevel and GetCharacterData answer both cases
with the same NOT_FOUND sentence. LevelUp folds once, on its re-read.

Regression coverage in `equip_item_test.go` proves the verb is handed the request, its
refusal passes through with its sentinel, the response projects the saved record
(chain mail 16, a monk 10 + DEX + WIS), an unprojectable saved record is INTERNAL, and
that the orchestrator neither reads nor writes the character repository around the verb.

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
