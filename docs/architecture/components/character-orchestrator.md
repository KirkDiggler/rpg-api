---
name: character orchestrator
description: Character creation, management, equipment, and data-loading orchestrator
updated: 2026-05-02
confidence: medium-high — verified by reading service.go and orchestrator.go
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
- **Character management:** equip/unequip items in toolkit equipment slots; get/list/delete characters.
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

## Known issues

### TODO at line 2765: alive-check not implemented

`orchestrator.go:2765` has a comment noting that "alive check logic" for spell list fetching is not implemented. Characters who are dead or unconscious may still have their spell list fetched without error.

### TODO at line 3282 (monster turns)

Actually in encounter orchestrator, not character. The character orchestrator is cleaner. However, the character orchestrator has a TODO at line ~3352 (based on quality.md notes) about "monster turns for entities acting before the current entity" — this may be a cross-reference in comments rather than a character-specific issue.

### Equipment choice logic in wrong layer

`REFACTOR_PLAN.md` (archived, originally at repo root) documents that equipment-choice business logic lives in the handler's external client interface rather than the orchestrator. This smell is present but deferred. The handler at `handler.go:765` has: `//TODO: handler should not interact with toolkit, this belongs in the orchestrator`. Toolkit type assertions in the character handler are the most visible symptom.

### No proto leakage (positive)

Unlike the encounter orchestrator, the character orchestrator does **not** import proto packages. Its Input/Output types in `service.go` use only toolkit types and local entity types. This is the correct pattern.
