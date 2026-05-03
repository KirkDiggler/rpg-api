---
name: entities
description: Domain data structures — plain Go structs, but with a known proto contamination problem
updated: 2026-05-02
confidence: high — verified by reading all entity files
---

# entities

`internal/entities/` holds the domain data structures for rpg-api. Entities are the types that flow between layers: from handlers (after proto conversion) through orchestrators to repositories. In the ideal architecture, entities are plain Go structs with no external dependencies. In the current state, several entity files import proto packages — a known architectural violation.

## Files

| File | Purpose | Proto contamination? |
|---|---|---|
| `encounter.go` | CombatState, ActionEconomyState, InitiativeEntry, Player, Position, DoorInfo | No |
| `dungeon.go` | Dungeon entity with exploration state | No |
| `character.go` | Thin wrapper — mostly uses toolkit types | No |
| `character_draft.go` | In-progress character creation state | No |
| `appearance.go` | Cosmetic character appearance | No |
| `room.go` | Room entity (for sandboxroom service) | Imports `apiv1alpha1.GridType` |
| `merged_grid.go` | Merged grid for multi-room rendering | Check separately |
| `encounter_events.go` | Event envelope + all event structs | **Yes — embeds proto types** |
| `encounter_events_json.go` | Custom JSON marshaling for events | No (just encoding logic) |
| `entity_state.go` | EntityStateData + proto construction functions | **Yes — constructs proto messages** |
| `encounter_state_builder.go` | BuildEncounterStateData + CombatStateToProto | **Yes — constructs proto messages** |

## Clean entities

- `encounter.go` — pure Go structs (Position, CombatState, ActionEconomyState, etc.). `ActionEconomyState` has methods for tracking action consumption — this is state tracking, not game rules.
- `dungeon.go` — embeds toolkit and component types (`*environments.ConnectionEdge`, `*dungeon.Room`) but these are not proto types.
- `character.go`, `character_draft.go`, `appearance.go` — clean.

## Proto contamination

### `encounter_events.go`

Event structs embed proto types as `json:"-"` fields — not serialized but present in the struct:
```go
type PlayerReconnectedEvent struct {
    // ...
    EncounterStateData *dnd5ev1alpha1.EncounterStateData `json:"-"`
}

type MovementCompletedEvent struct {
    // ...
    UpdatedEntity    *dnd5ev1alpha1.EntityState `json:"-"`
    CombatStateProto *dnd5ev1alpha1.CombatState `json:"-"`
}
```

Nine event structs have these embedded proto fields. The intent is a fast path: the orchestrator pre-builds proto snapshots and attaches them to events, so the handler does not need to rebuild from entity state.

**The problem:** This couples the entity layer to proto types. The entity package now imports `rpg-api-protos`. Changing the proto schema requires touching entity code.

**Alternative approach (without proto contamination):** The orchestrator could attach proto payloads to a separate `protoPayloads` map keyed by event ID, which the handler reads after receiving the event. Or the handler could always reconstruct from entity data. The current approach is a pragmatic shortcut.

### `entity_state.go`

Contains `ToEntityStateProto` and `ToEntityStateProtos` — functions that construct `*dnd5ev1alpha1.EntityState` proto messages. These functions belong in the handler's converter layer, not in the entities package.

`entity_state.go` also imports:
- `apiv1alpha1` (for `apiv1alpha1.Position`)
- `dnd5ev1alpha1` (for EntityState, CharacterDetails, MonsterDetails, etc.)
- `toolkitchar`, `monster`, `classes`, `races` toolkit packages

This file is effectively a converter, not an entity definition.

### `encounter_state_builder.go`

Contains `BuildEncounterStateData` and `CombatStateToProto` — full proto construction functions. Same issue as `entity_state.go`.

`BuildEncounterStateDataInput.Rooms` is typed as `map[string]*dnd5ev1alpha1.RoomLayout` — a proto type in what should be a domain input struct.

### `room.go`

Imports `apiv1alpha1.GridType`:
```go
GridType roomcommon.GridType `json:"grid_type"`
```

`entities.Room` stores a proto enum directly on the entity struct. This is a minor contamination compared to the others, but it means any serialization of `entities.Room` to/from JSON would include the proto enum's integer value.

## Recommended path

The cleanest fix is to move all proto construction functions (`ToEntityStateProto`, `BuildEncounterStateData`, `CombatStateToProto`) to the handler converter layer, remove the `json:"-"` proto fields from event structs, and replace `apiv1alpha1.GridType` in `entities.Room` with a local string or int enum. This would make the entities package proto-free.

This is a larger refactor that should be done alongside the encounter orchestrator proto cleanup.
