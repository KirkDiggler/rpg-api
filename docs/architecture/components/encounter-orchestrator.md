---
name: encounter orchestrator
description: Central combat orchestrator — combat resolution, dungeon navigation, monster turns, entity state, event publishing
updated: 2026-05-02
confidence: high — verified by reading orchestrator.go, service.go, monster_turns.go, perception.go, dungeon_mapper.go
---

# encounter orchestrator

The encounter orchestrator is the largest and most critical component in rpg-api. It coordinates the full lifecycle of a multiplayer dungeon encounter: lobby creation, dungeon generation, combat resolution, monster turns, room navigation, entity state snapshots, and event publishing.

## Files

| File | Lines | Purpose |
|---|---|---|
| `orchestrators/encounter/orchestrator.go` | 5,577 | All orchestration logic |
| `orchestrators/encounter/service.go` | 563 | Service interface + Input/Output types |
| `orchestrators/encounter/monster_turns.go` | 521 | Monster turn execution |
| `orchestrators/encounter/perception.go` | 250 | Passive perception / monster AI targeting |
| `orchestrators/encounter/dungeon_mapper.go` | 90 | Maps orchestrator params → dungeon generator input |

**Total: ~7,001 lines across the encounter orchestrator package.**

## Purpose

Coordinates the full encounter lifecycle:
1. **Lobby** — create encounter, join by code, set ready, start combat
2. **Dungeon generation** — generate multi-room dungeon via component, place entities
3. **Combat** — initiative, action economy, attack resolution, feature activation, movement
4. **Monster turns** — AI-driven monster action selection and execution
5. **Room navigation** — open doors, reveal rooms, merge new monsters into initiative
6. **State snapshots** — build `EncounterStateData` proto for load-then-stream pattern
7. **Event publishing** — emit `EncounterEvent` per action via the event processor

## Public interface (`service.go`)

```go
type Service interface {
    ResolveAttack(ctx, *ResolveAttackInput) (*ResolveAttackOutput, error)
    CreateDungeon(ctx, *CreateDungeonInput) (*CreateDungeonOutput, error)
    MoveCharacter(ctx, *MoveCharacterInput) (*MoveCharacterOutput, error)
    EndTurn(ctx, *EndTurnInput) (*EndTurnOutput, error)
    ActivateFeature(ctx, *ActivateFeatureInput) (*ActivateFeatureOutput, error)
    OpenDoor(ctx, *OpenDoorInput) (*OpenDoorOutput, error)
    CreateEncounter(ctx, *CreateEncounterInput) (*CreateEncounterOutput, error)
    JoinEncounter(ctx, *JoinEncounterInput) (*JoinEncounterOutput, error)
    SetReady(ctx, *SetReadyInput) (*SetReadyOutput, error)
    StartCombat(ctx, *StartCombatInput) (*StartCombatOutput, error)
    LeaveEncounter(ctx, *LeaveEncounterInput) (*LeaveEncounterOutput, error)
    PlayerDisconnected(ctx, *PlayerDisconnectedInput) (*PlayerDisconnectedOutput, error)
    PlayerReconnected(ctx, *PlayerReconnectedInput) (*PlayerReconnectedOutput, error)
    GetEncounterState(ctx, *GetEncounterStateInput) (*GetEncounterStateOutput, error)
    GetEncounterHistory(ctx, *GetEncounterHistoryInput) (*GetEncounterHistoryOutput, error)
    ActivateCombatAbility(ctx, *ActivateCombatAbilityInput) (*ActivateCombatAbilityOutput, error)
    ExecuteAction(ctx, *ExecuteActionInput) (*ExecuteActionOutput, error)
}
```

## Internal data model

The orchestrator works with the following types:
- `entities.Dungeon` — dungeon graph + room origins + exploration state
- `encounterrepo.EncounterData` — full encounter state including entity map
- `entities.EntityStateData` — per-entity state (position, HP, toolkit data)
- `entities.CombatState` — initiative order, active turn, action economy
- `entities.EncounterEvent` — event envelope (emitted via event processor)

## Dependencies

```
Orchestrator
    ├── characterrepo.Repository     — loads character.Data for encounter entrants
    ├── encounterrepo.Repository     — reads/writes EncounterData (in-memory)
    ├── dungeonrepo.Repository       — reads/writes Dungeon entity (in-memory)
    ├── dungeon.Generator            — procedural dungeon generation (local component)
    ├── eventprocessor.Processor     — persist + publish encounter events
    ├── encounterlogrepo.Repository  — read event history for GetEncounterHistory
    ├── dice.Roller                  — dice rolls (optional, defaults to random)
    ├── spawner.Spawner              — entity placement in rooms (optional)
    ├── idgen.Generator (×3)         — encounter/dungeon/connection ID generation
    └── rpg-toolkit packages:
         ├── initiative              — roll and track initiative order
         ├── combat                  — attack resolution, damage, armor class
         ├── actions                 — ability activation (rage, second wind, etc.)
         ├── monster                 — monster stats, turn execution
         ├── monstertraits          — special monster abilities
         ├── environments            — connection graph (dungeon layout)
         ├── spatial                 — room data, entity placement, movement
         └── gamectx                 — game context for toolkit rule calls
```

## Known issues and debt

### CRITICAL: Proto types in orchestrator (architectural violation)

`orchestrator.go` imports:
```go
apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
```

There are **39 `pb.` references** including:
- `buildRoomLayoutProto` (line 1097) — constructs `*pb.RoomLayout` inside the orchestrator
- `buildRoomsMap` (line 1167) — builds `map[string]*pb.RoomLayout`
- `protoAbilityIDToRef` (line 2321) — converts `pb.CombatAbilityId` to toolkit `core.Ref`
- `protoActionIDToRef` (line 2372) — converts `pb.ActionId` to toolkit `core.Ref`
- Local variables `var attackerState, targetState *pb.EntityState` (line 538, 4764, 4929, 5096)
- `startRoomLayouts := map[string]*pb.RoomLayout{}` (line 3913)

The functions `buildRoomLayoutProto` and `buildRoomsMap` belong in the encounter handler's converter layer. The `protoAbilityIDToRef` and `protoActionIDToRef` functions map from proto enums to toolkit refs — they could be replaced by passing string refs from the handler instead of proto enums.

### CRITICAL: Proto types in service interface types

`service.go` line 7 imports `pb` and uses proto types directly in Input/Output structs:
- `MonsterCombatState.MonsterType pb.MonsterType` (line 459)
- `ActivateCombatAbilityInput.AbilityID pb.CombatAbilityId` (line 511)
- `ExecuteActionInput.ActionID pb.ActionId` (line 535)

This means callers of the `Service` interface (i.e., the handler) must also import `pb`, and any consumer of `ActivateCombatAbilityOutput` receives proto-typed fields. The fix is to use string IDs or local enum types in the service interface.

### Orchestrator size

At 5,577 lines with 70+ functions, `orchestrator.go` owns too much responsibility. Suggested decomposition:
- `combat.go` — `ResolveAttack`, `ActivateCombatAbility`, `ExecuteAction` (attack subtypes)
- `dungeon_nav.go` — `OpenDoor`, room reveal, entity map merging
- `lobby.go` — `CreateEncounter`, `JoinEncounter`, `SetReady`, `StartCombat`, player connect/disconnect
- `state.go` — `GetEncounterState`, `GetEncounterHistory`, entity state snapshot building

`StartCombat` carries `//nolint:gocyclo` (line 3592) — this is the most complex function, coordinating dungeon generation, initial entity placement, initiative rolling, monster turns, and event publishing. It is the first candidate for decomposition.

### Coordinate transform — no canonical function

Room-local coordinates (dungeon component types, integer cube coords) must be translated to dungeon-absolute positions. The current approach is five separate inline transform sites. Each is subtly different (some shift walls, some shift entities, some do both). The `shiftRoomToAbsolute` function in the encounter handler's converters.go is the closest to canonical but only operates on proto types.

The correct fix is:
1. A `toAbsolute(local entities.Position, origin dungeon.Position) entities.Position` function in `internal/pkg/` or `internal/entities/`.
2. All transform sites replaced with a single call.

### Debug theme not reverted

`dungeon_mapper.go:44` maps the `"crypt"` theme to `ThemeDebugWalls` with a TODO comment. All crypt dungeons currently render with debug walls. The fix is a one-line change but has not been reverted from wall UI testing.

### TODO: monster turns before current entity

`orchestrator.go:3282` has a TODO: "Implement monster turns for monsters that act before the current entity." When monsters are added to initiative mid-round (via `OpenDoor`), any monsters that rolled higher initiative than the current player should act immediately. This is currently not implemented.

### PlayerDisconnected not fully wired

`handlers/dnd5e/v1alpha1/encounter/handler.go:691` has a TODO: "Call PlayerDisconnected on the orchestrator." The streaming handler's disconnect path exits but does not notify the orchestrator. Encounter state is not cleaned up on disconnect.

### `interface{}` for room data

`EncounterData.RoomData interface{}` is explicitly documented as temporary. The orchestrator uses type assertions to access `*spatial.RoomData`. This makes the code brittle and disables type checking.
