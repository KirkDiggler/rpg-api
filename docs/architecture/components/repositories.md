---
name: repositories
description: All data access components — interface definitions, implementations, and storage schemas
updated: 2026-05-02
confidence: high — verified by reading all repository interface and implementation files
---

# repositories

rpg-api has six repositories. Three are Redis-backed (persistent); three are in-memory only (state lost on restart). All follow the Input/Output type pattern with generated mocks.

## Overview

| Repository | Interface | Implementation | Storage | Grade |
|---|---|---|---|---|
| `character` | `repository.go` | `redis.go` | Redis | B |
| `character_draft` | `repository.go` | `redis.go` | Redis | B- |
| `dice_session` | `repository.go` | `redis.go` | Redis | B- |
| `encounters` | `repository.go` | `inmemory.go` | In-memory | D |
| `dungeons` | `repository.go` | `inmemory.go` | In-memory | D |
| `encounterlog` | `repository.go` | `inmemory.go` | In-memory | D |

## Character repository

**Path:** `repositories/character/`

Interface methods:
- `Get(ctx, *GetInput) (*GetOutput, error)` — by character ID
- `GetByPlayerID(ctx, *GetByPlayerIDInput) (*GetByPlayerIDOutput, error)`
- `List(ctx, *ListInput) (*ListOutput, error)` — by player ID
- `Save(ctx, *SaveInput) (*SaveOutput, error)`
- `Delete(ctx, *DeleteInput) (*DeleteOutput, error)`

**Storage:** `character:{id}` — JSON-serialized `character.Data` + `Appearance` (stored together).

Used by: character orchestrator, encounter orchestrator (to load characters for dungeon entry).

The only repository exercised by integration tests with real Redis.

## Character draft repository

**Path:** `repositories/character_draft/`

Interface methods: `Create`, `Get`, `List`, `Update`, `Delete`.

**Storage:** Redis keys per draft. Handles in-progress character creation state. Less tested than character repo — no integration tests that specifically exercise draft lifecycle.

## Dice session repository

**Path:** `repositories/dice_session/`

Narrow scope: tracks ability score dice rolls during character creation before they are assigned to a draft. Redis-backed, simple interface.

## Encounter repository (in-memory)

**Path:** `repositories/encounters/`

Interface methods:
- `Save(ctx, *SaveInput) (*SaveOutput, error)` — full encounter save
- `Get(ctx, *GetInput) (*GetOutput, error)` — by ID
- `GetByJoinCode(ctx, *GetByJoinCodeInput) (*GetOutput, error)` — by lobby join code
- `Update(ctx, *UpdateInput) (*UpdateOutput, error)` — partial update (nil = no change)
- `Delete(ctx, *DeleteInput) (*DeleteOutput, error)`

`EncounterData` stores:
- Room data as `interface{}` (type fix pending)
- `*initiative.TrackerData` (toolkit type)
- `map[string]*entities.EntityStateData` — unified entity map
- DEPRECATED: `[]*monster.Data`, `map[string]int` (CharacterHP) — both still present during migration
- Multiplayer fields: State, JoinCode, HostID, Players map

`UpdateInput` uses pointer fields for optional updates — `nil` means "don't change this field." This is the correct pattern for partial updates.

**`copyEncounterData` deep copy** (inmemory.go): Added after PR #453 fix for data integrity — the in-memory store returns a deep copy to prevent callers from mutating stored state via the returned pointer.

**Gap:** No Redis implementation. All encounter state is process-local. A Redis implementation requires solving the `interface{}` RoomData serialization problem first.

## Dungeon repository (in-memory)

**Path:** `repositories/dungeons/`

Interface methods:
- `Save(ctx, *SaveInput) (*SaveOutput, error)`
- `Get(ctx, *GetInput) (*GetOutput, error)` — by dungeon ID
- `GetByEncounterID(ctx, *GetByEncounterIDInput) (*GetOutput, error)`
- `Update(ctx, *UpdateInput) (*UpdateOutput, error)`
- `Delete(ctx, *DeleteInput) (*DeleteOutput, error)`

`DungeonData` stores the full `entities.Dungeon`, including the toolkit-typed `Connections []*environments.ConnectionEdge` and component-typed `Rooms map[string]*dungeon.Room`.

**Gap:** No Redis implementation. Serializing `entities.Dungeon` to Redis requires solving the mixed type problem (dungeon.Room contains spatial.RoomData which contains an interface graph).

## Encounter log repository (in-memory)

**Path:** `repositories/encounterlog/`

Interface methods:
- `Append(ctx, *AppendInput) (*AppendOutput, error)` — appends event, assigns ULID
- `GetHistory(ctx, *GetHistoryInput) (*GetHistoryOutput, error)` — retrieves events in order

Append-only event log. The `AppendOutput.EventID` is a ULID — sortable by time, globally unique. The ULID becomes the `EncounterEvent.ID` and the encounter's `LastEventID` for load-then-stream sync.

**Gap:** No Redis implementation. Events are lost on restart. This means:
- No replay for clients that join after a restart.
- No audit trail beyond the current process.
- Late joiners who call `GetEncounterHistory` after a restart get an empty log.

## Common patterns

All repositories:
- Input/Output types on every method (non-negotiable)
- Generated mocks in `mock/` subdirectory using `go:generate mockgen`
- Never return `(nil, nil)` — valid output or error
- Interface defined in `repository.go` alongside types

Mock packages follow `<domain>mock` naming (e.g., `encountermock`, `charactermock`).
