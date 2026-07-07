---
name: rpg-api architecture overview
description: Request flow, layer rules, component map, and cross-repo boundaries for rpg-api
updated: 2026-05-02
confidence: high — verified by full code read-through of key files
---

# rpg-api architecture overview

rpg-api is a Go gRPC server that orchestrates game state for a multiplayer D&D 5e dungeon crawler. Its single mandate is *data orchestration*: load entities from repositories, pass them to rpg-toolkit for rule execution, persist the results, and publish events to connected clients. rpg-api never knows what Rage does, never calculates attack modifiers on its own, and never implements D&D rules. If game logic appears here, that is a defect — the missing helper belongs in rpg-toolkit.

## Request flow

```
gRPC client (rpg-dnd5e-web or Discord Activity)
    │
    ▼
gRPC interceptors  (cmd/server/server.go)
    auth.UnaryAuthInterceptor + auth.StreamAuthInterceptor
    extracts Discord token → validates → injects playerID into context
    │
    ▼
Handler  (internal/handlers/dnd5e/v1alpha1/<domain>/handler.go)
    proto → domain entity conversion
    input validation with status codes
    auth context extraction (auth.PlayerIDFromContext)
    delegate to orchestrator
    domain entity → proto conversion
    │
    ▼
Orchestrator  (internal/orchestrators/<domain>/orchestrator.go)
    business logic coordination using internal entity types ONLY
    composes repos + toolkit calls
    publishes encounter events via event processor
    │
    ├─→ Repository  (internal/repositories/<domain>/)
    │       Input/Output types on every method
    │       returns domain entities, never DB models
    │
    ├─→ rpg-toolkit  (github.com/KirkDiggler/rpg-toolkit/...)
    │       rule engine: combat resolution, initiative, abilities,
    │       monster turns, spatial placement, dungeon room generation
    │
    └─→ Event processor  (internal/processors/event/)
            persist to encounter log repo
            publish to Redis pub/sub channel
```

## Layer rules

### Handlers
- **Do:** proto ↔ entity conversion, input validation, auth context extraction.
- **Do not:** business logic, game rules, toolkit calls that are not conversions.
- One handler file per service version, one converter file per service.
- Accept proto request → call `auth.PlayerIDFromContext` → build input struct → call orchestrator → convert output → return proto response.

**Current violation in encounter handler:** `internal/handlers/dnd5e/v1alpha1/encounter/handler.go` imports `github.com/KirkDiggler/rpg-toolkit/tools/spatial` and hardcodes `spatial.GridTypeHex` and `spatial.HexOrientationPointyTop` at six call sites (lines 157–158, 359–360, 569–570, 757–758, 822–823, 905–906). A type assertion against `*toolkitchar.Data` also lives at line 765 with a TODO comment acknowledging it belongs in the orchestrator. These spatial values should come from the orchestrator's output, not be constructed in the handler.

### Orchestrators
- **Do:** coordinate data: load from repos, call toolkit, persist results, emit events.
- **Do not:** import or construct proto types. No `pb.` references. Speak internal entity types exclusively.
- Input/Output structs on every method (defined in `service.go` alongside the interface).

**Current violation in encounter orchestrator:** `internal/orchestrators/encounter/orchestrator.go` imports `pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"` and `apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"` at lines 11–12. There are 39 `pb.` references across the file including functions `buildRoomLayoutProto` (line 1097) and `buildRoomsMap` (line 1167) that construct proto messages inside the orchestrator, and `protoAbilityIDToRef` (line 2321) that converts `pb.CombatAbilityId` enums. These proto types contaminate the orchestrator's output structs. Fixing this requires moving `buildRoomLayoutProto` / `buildRoomsMap` to the encounter handler's converter layer and introducing internal `RoomLayout` entity types.

**Current violation in encounter service types:** `internal/orchestrators/encounter/service.go` imports `pb` at line 7 and uses `pb.MonsterType` in `MonsterCombatState` (line 459), `pb.CombatAbilityId` in `ActivateCombatAbilityInput` (line 511), and `pb.ActionId` in `ExecuteActionInput` (line 535). The Input/Output types themselves are polluted with proto enum types, meaning callers of the service interface must also import `pb` — the contamination propagates outward.

**Current violation in entities package:** `internal/entities/entity_state.go` and `internal/entities/encounter_state_builder.go` import proto packages and contain full proto construction logic (`ToEntityStateProto`, `BuildEncounterStateData`, `CombatStateToProto`). The `entities` package should hold plain Go structs with no proto dependencies.

**Current violation in encounter_events.go:** `internal/entities/encounter_events.go` imports `dnd5ev1alpha1` at line 7 and embeds `*dnd5ev1alpha1.EncounterStateData`, `*dnd5ev1alpha1.EntityState`, and `*dnd5ev1alpha1.CombatState` as `json:"-"` fields in event structs (lines 109, 134, 147, 161, 177–178, 284–285, 300–301, 314, 323, 339). This is a layered contamination: proto types embedded in entity layer event structs.

### Repositories
- **Do:** data access abstraction with Input/Output types on every method.
- **Do not:** expose DB-layer models, return `(nil, nil)`.
- Interfaces defined in `repository.go` alongside Input/Output types.
- Implementations in same package (`redis.go`, `inmemory.go`).

**Current state:** Character, character_draft, and dice_session repositories have Redis implementations. **Encounter, dungeon, and encounter-log repositories are in-memory only** — state is lost on process restart. See `internal/repositories/encounters/inmemory.go`, `internal/repositories/dungeons/inmemory.go`, `internal/repositories/encounterlog/inmemory.go`.

### Components (`internal/components/`)
Components are local prototypes pending graduation to rpg-toolkit. They implement game logic that rightfully belongs in the rules engine but has not yet been extracted.

- `internal/components/dungeon/` — procedural dungeon generation (room shapes, wall perimeters, door spawning, hex-grid layouts, monster CR budgets, theme tables). **This is game logic in the wrong repository.** The Boundary Rule is explicit: game mechanics belong in rpg-toolkit.
- `internal/components/spawner/` — thin adapter around toolkit placement logic. Delegates rather than implements rules; lower risk.

## Component map

| Component | Path | Purpose | Notes |
|---|---|---|---|
| Encounter handler | `internal/handlers/dnd5e/v1alpha1/encounter/` | gRPC ↔ encounter orchestrator | toolkit spatial imports leak in |
| Character handler | `internal/handlers/dnd5e/v1alpha1/character/` | gRPC ↔ character orchestrator | 20 TODO stubs in converters |
| Dice handler | `internal/handlers/api/v1alpha1/dice_handler.go` | gRPC ↔ dice orchestrator | Simple; no known issues |
| Encounter orchestrator | `internal/orchestrators/encounter/` | Combat, dungeon, lobby, room nav | 5,577 lines; proto leakage |
| Character orchestrator | `internal/orchestrators/character/` | Character creation, equipment | Smaller, well-tested |
| Dice orchestrator | `internal/orchestrators/dice/` | Dice rolling sessions | Thin; low risk |
| Dungeon component | `internal/components/dungeon/` | Procedural dungeon generation | Wrong repo; toolkit boundary violation |
| Spawner component | `internal/components/spawner/` | Entity placement adapter | Thin; delegates to toolkit |
| Event processor | `internal/processors/event/` | Persist + publish encounter events | `fmt.Printf` in hot path |
| Redis publisher | `internal/publishers/encounter/` | Redis pub/sub for encounter events | Tested; no retry logic |
| Auth | `internal/auth/` | Discord token validation + caching | Dev mode for local/tests |
| Entities | `internal/entities/` | Domain structs | Proto leakage in entity_state.go |
| Encounter repo | `internal/repositories/encounters/` | Encounter persistence | In-memory only |
| Dungeon repo | `internal/repositories/dungeons/` | Dungeon persistence | In-memory only |
| Encounter log repo | `internal/repositories/encounterlog/` | Event log (append-only) | In-memory only |
| Character repo | `internal/repositories/character/` | Character persistence | Redis — only durable store |
| Character draft repo | `internal/repositories/character_draft/` | Draft character state | Redis |
| Dice session repo | `internal/repositories/dice_session/` | Dice roll sessions | Redis |
| SandboxRoom service | `internal/services/sandboxroom/` | Room generation interface | Interface only; no implementation wired |
| Integration test harness | `internal/integration/harness/` | Full-stack test server with real Redis | Most valuable test asset |
| Lobby handler | `internal/handlers/dnd5e/lobby/v1alpha1/` | gRPC ↔ lobby orchestrator | New 2026-07-07 (rpg-api#629); layered from day one |
| Lobby orchestrator | `internal/orchestrators/lobby/` | Party assembly + sole encounter construction (`StartEncounter`) | New; see `docs/architecture/components/lobby-service.md` |
| Lobby repo | `internal/repositories/lobby/` | Lobby persistence | Redis + in-memory |

## Cross-repo boundaries

**What rpg-api accepts from clients (rpg-dnd5e-web):**
- References (IDs): character IDs, encounter IDs, join codes, dungeon IDs, connection IDs.
- Intent: action requests with resource references (`weaponID`, `featureID`), never calculations.
- Auth: Discord JWT tokens validated via Discord API.

**What rpg-api asks rpg-toolkit:**
- Combat resolution: `actions.CheckAndGrantOffHandStrike`, `combat.ResolveAttack`, monster turns.
- Initiative: `initiative.Roll`, `initiative.Tracker`.
- Character rules: ability score calculation, proficiency bonuses, spell slots — these come from toolkit types.
- Spatial: entity placement, movement validation, line-of-sight via `tools/spatial` and `tools/environments`.
- Dungeon room generation: room shapes, perimeter walls, feature layouts via `components/dungeon` (pending move to toolkit).

**What rpg-api persists:**
- `character.Data` (toolkit type) serialized to Redis — character state is owned by the toolkit type.
- `EncounterData` (local type) in memory — initiative, room data, entity map, player roster, combat state.
- `Dungeon` (local entity) in memory — connection graph, room origins, revealed rooms, open doors.
- `EncounterEvent` (local entity) in memory — append-only event log per encounter.

**What rpg-api publishes:**
- Redis pub/sub channel `encounter:{id}:events` — JSON-serialized `EncounterEvent` structs.
- One channel per active encounter; clients subscribe for real-time state updates.

## gRPC service versions

- `dnd5e.api.v1alpha1` — CharacterService + EncounterService (primary services)
- `api.v1alpha1` — DiceService
- `dnd5e.api.v1alpha2.encounter` — EncounterService (v2, `MoveEntity`/`StreamEncounter`/
  combat verbs). `CreateEncounter` is deleted from this service — construction now
  happens exclusively through `LobbyService.StartEncounter` (below).
- `dnd5e.api.lobby.v1alpha1` — LobbyService (new 2026-07-07, rpg-api#629). Own version
  clock, deliberately not tied to the encounter service's — see
  `docs/architecture/components/lobby-service.md`.

Proto definitions are in `rpg-api-protos` (separate repo). Compiled Go code pinned via `@generated` branch. This doc's service-version list otherwise predates the v2 encounter stack (last full refresh 2026-05-02) — treat the table above as current, the rest of this file as historical context pending a fuller refresh.

## Known architectural debt (in priority order)

1. **Proto types in service interface** (`orchestrators/encounter/service.go` lines 459, 511, 535) — the Input/Output types themselves embed `pb.MonsterType`, `pb.CombatAbilityId`, `pb.ActionId`.
2. **Proto types in orchestrator** (`orchestrators/encounter/orchestrator.go`, 39 `pb.` refs) — builds proto messages inside the orchestrator.
3. **Proto types in entities** (`entities/entity_state.go`, `entities/encounter_state_builder.go`, `entities/encounter_events.go`) — proto construction and proto-typed fields in the domain entity layer.
4. **Coordinate transform fragility** — no canonical local-to-absolute transform function; five ad-hoc fix sites across orchestrator and handler converters.
5. **In-memory repositories** — encounter, dungeon, and encounter-log repos lose state on restart.
6. **Dungeon component in wrong repo** — `components/dungeon/` implements game logic that belongs in rpg-toolkit.
