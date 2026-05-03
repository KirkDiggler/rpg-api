---
name: encounter handler
description: gRPC handler for the EncounterService — validates, converts, delegates, streams events
updated: 2026-05-02
confidence: high — verified by reading handler.go and converters.go
---

# encounter handler

The encounter handler is the gRPC adapter for `EncounterService`. It translates proto requests to orchestrator inputs, delegates to the encounter orchestrator, and converts outputs back to proto responses. It also owns the server-streaming `StreamEncounterEvents` RPC that delivers real-time encounter events to clients.

## Files

| File | Lines | Purpose |
|---|---|---|
| `handlers/dnd5e/v1alpha1/encounter/handler.go` | 1,112 | gRPC handler |
| `handlers/dnd5e/v1alpha1/encounter/converters.go` | 1,658 | Proto ↔ domain entity conversion |

## gRPC methods handled

- `CreateEncounter` — creates multiplayer lobby
- `JoinEncounter` — joins by join code
- `SetReady` / `LeaveEncounter` — lobby management
- `StartCombat` — begins combat (host only)
- `CreateDungeon` — solo dungeon (legacy/test path)
- `GetEncounterState` — full snapshot for load-then-stream pattern
- `StreamEncounterEvents` — server streaming: subscribes, sends events as stream
- `ResolveAttack` — legacy single-step attack (pre-action economy)
- `MoveCharacter` — legacy movement
- `ActivateFeature` — activates class features (rage, second wind, etc.)
- `ActivateCombatAbility` — new action economy: activate ATTACK, DASH, DODGE, etc.
- `ExecuteAction` — new action economy: execute STRIKE, MOVE, etc.
- `EndTurn` — advances to next initiative turn
- `OpenDoor` — opens door and reveals new room
- `PlayerDisconnected` — TODO: not fully wired (see below)
- `GetEncounterHistory` — retrieves past events for replay

## Known boundary violations

### Toolkit spatial types hardcoded in handler (6 sites)

`handler.go` imports `github.com/KirkDiggler/rpg-toolkit/tools/spatial` and hardcodes:
```go
gridType := spatial.GridTypeHex              // lines 157, 359, 569, 757, 822, 905
hexOrientation := spatial.HexOrientationPointyTop  // lines 158, 360, 570, 758, 823, 906
```

These appear in `CreateEncounter`, `JoinEncounter`, `StartCombat`, `GetEncounterState`, `OpenDoor`, and `StreamEncounterEvents`. Handlers should not import toolkit spatial packages — the grid type is a domain concept that should come from the orchestrator's output (or be a constant in the dungeon component).

### Type assertion on `*toolkitchar.Data` in handler

`handler.go:765` contains:
```go
//TODO: handler should not interact with toolkit, this belongs in the orchestrator
if charData, ok := member.CharacterData.(*toolkitchar.Data); ok {
```

This is a type assertion against a toolkit type in the handler layer — a direct handler→toolkit dependency that violates the pattern. The orchestrator's `PartyMember.CharacterData` is typed as `interface{}`, which is why the handler must assert. The fix: the orchestrator should return a typed wrapper or character summary struct instead of `interface{}`.

### `PlayerDisconnected` TODO

`handler.go:691`:
```go
// TODO: Call PlayerDisconnected on the orchestrator
```

The streaming RPC handler handles client disconnect but does not call `orchestrator.PlayerDisconnected`. This means encounter state is not updated when a player's stream closes. The `PlayerDisconnected` orchestrator method exists and is implemented — the hook in the handler was never wired.

## Converter surface

`converters.go` (1,658 lines) owns all proto ↔ domain entity conversion for encounter types:
- `shiftRoomToAbsolute` — local → absolute position for rooms (the canonical handler-side transform)
- `shiftWallsByOrigin` / `shiftEntitiesByOrigin` — per-component shift helpers
- Entity type and size conversions
- Combat state, initiative entry, action economy conversions
- Monster state, attack result conversions
- Room data conversion from `spatial.RoomData` → proto `Room`

The `shiftRoomToAbsolute` function (line 68) is the primary coordinate transform in the codebase. It operates on proto `*dnd5ev1alpha1.Room` after the orchestrator returns room data. This is the correct layer for the transform (handler/converter), but the function only exists in proto space — there is no equivalent for entity-space transforms.

## Load-then-stream pattern

The encounter handler implements load-then-stream for new/reconnecting clients:
1. Client calls `GetEncounterState` — receives full snapshot with `LastEventID`.
2. Client calls `StreamEncounterEvents` — receives events where `id > LastEventID`.

`StreamEncounterEvents` subscribes to the Redis publisher, converts each `entities.EncounterEvent` to a proto `EncounterEvent`, and sends it on the stream. The conversion includes populating the proto-typed entity states from the event's `json:"-"` proto fields when available (the fast path), or reconstructing from entity data otherwise.

## Auth pattern

All handlers extract `playerID` via `auth.PlayerIDFromContext(ctx)` and pass it as a field in the orchestrator's input struct. No business logic in handlers.
