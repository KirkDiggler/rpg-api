---
name: rpg-api status
description: Where we are with rpg-api — active work, paused, known rough edges, per-subsystem confidence
updated: 2026-05-02
confidence: medium — first draft seeded from full code read-through, git history, and PR audit; needs Kirk's correction pass
---

# rpg-api: Where We Are

This is a living doc. Edit it in the same PR that invalidates a line. Don't let it rot.

## Active work

All six open PRs are coordinate-space fix branches feeding into the consolidated
Round 2 branch (#468). #468 itself is **failing CI** (test step, build passes) and
the current plan is to **abandon it and start fresh from main** — it picked up
merge debt from stacking fixes. None of these should be merged individually.

| PR | Branch | Status | Notes |
|----|--------|--------|-------|
| #459 | `fix/458-room-origins-monster-turns` | Open | `shiftRoomToAbsolute` wired to `DungeonStart` handler |
| #461 | `fix/open-door-monster-absolute-positions` | Open | Monster entity-map origin offset |
| #463 | `fix/462-room-layout-wall-absolute-positions` | Open | Wall coords translated to dungeon-space |
| #466 | `fix/open-door-room-data-persist` | Open | `OpenDoor` now persists merged `RoomData` |
| #467 | `fix/room2-missing-perimeter-walls-465` | Open | Horizontal perimeter wall Z-coord fix |
| #468 | `round2/multi-room-dungeon` | Open — CI FAILING | Consolidated branch; do not merge; start fresh |

## Recently landed (since Round 1, highlights)

- **Unified entity state** (PR #453, 2026-04-05) — `Entities` map added to
  `EncounterData`; `BuildEncounterStateData` snapshot builder; all events
  wire entity state through.
- **Entity state wired to events** (PR #454, 2026-04-05) — `AttackResolved`,
  `TurnEnded`, `FeatureActivated`, `MovementCompleted`, room reveal events all
  carry `EntityStateData`. Copilot feedback addressed post-merge.
- **Integration test: OpenDoor → RoomRevealed stream** (commit `176b501`) —
  first integration test proving the full event-stream pipeline for multi-room
  reveal.
- **Room layout protos wired through encounter state builder** (commit `87d015c`)
  — `RoomLayout` now returned in `GetEncounterState`.
- **Round 1: Monk kills monster** — all issues merged by 2026-04-05; combat
  loop, action economy, monster turns, event publishing all working end-to-end
  for a single-room dungeon.

## Paused / on hold

- **Round 2 consolidated branch (#468)** — failing CI; plan is to cherry-pick
  or re-apply the six fix PRs onto a fresh branch from main.
- **Debug-walls theme** — `dungeon_mapper.go:44` has a hardcoded
  `ThemeDebugWalls` where `ThemeCrypt` should be. Left in during wall-rendering
  UI testing; not reverted yet.
- **PlayerDisconnected** — `handler.go:695` has a `TODO` calling the orchestrator
  method; the streaming handler exits but no state cleanup fires.
- **Spell/trait enum conversions** — multiple `TODO` comments in
  `handlers/.../character/converters.go` where `SPELL_UNSPECIFIED` and
  `TRAIT_UNSPECIFIED` are returned because proto enums are not yet mapped.
- **REFACTOR_PLAN.md** (repo root) — equipment-choice business logic still lives
  in the handler/external client instead of the orchestrator. Flagged but
  deferred since early in the project.

## Known rough edges

### Boundary violations — proto types in the orchestrator

`internal/orchestrators/encounter/orchestrator.go` imports `pb` (the dnd5e proto
package) and `apiv1alpha1` directly and uses `*pb.RoomLayout`, `*pb.EntityState`,
`*pb.CombatStateProto` as return/local types (39 `pb.` references verified by grep).
This violates the handler→orchestrator boundary. The functions `buildRoomLayoutProto`
(line 1097) and `buildRoomsMap` (line 1167) construct proto messages inside the
orchestrator. No lint rule catches this today.

The violation also extends to `service.go` (line 7): the Service interface's Input/Output
types themselves embed `pb.MonsterType` (line 459), `pb.CombatAbilityId` (line 511),
and `pb.ActionId` (line 535) — meaning any caller of the interface must also import `pb`.

Affected files: `internal/orchestrators/encounter/orchestrator.go`,
`internal/orchestrators/encounter/service.go`

### Boundary violation — dungeon component belongs in toolkit

`internal/components/dungeon/` implements procedural dungeon generation
(room shapes, wall perimeters, door spawning, hex-grid layouts, monster placement).
This is game logic, not data orchestration. The CLAUDE.md boundary rule is explicit:
"If it's a game mechanic or calculation → rpg-toolkit." The component is correctly
identified as toolkit-bound in project memory but has not moved.

Affected path: `internal/components/dungeon/`

### Boundary violation — toolkit types leak into encounter handler

`internal/handlers/dnd5e/v1alpha1/encounter/handler.go` imports
`github.com/KirkDiggler/rpg-toolkit/tools/spatial` and
`github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character` directly.
`spatial.GridTypeHex` and `spatial.HexOrientationPointyTop` are hardcoded six
times in the handler. A type assertion on `*toolkitchar.Data` also lives there.
Handler should convert proto to domain types only.

Affected file: `internal/handlers/dnd5e/v1alpha1/encounter/handler.go`

### Coordinate-space fragility

Five separate fix commits between 2026-04-04 and 2026-04-06 all corrected the
same class of bug: room-local coordinates being treated as absolute dungeon-space
coordinates. The fixes are in `DungeonStart`, `OpenDoor`, `buildRoomLayoutProto`,
the entity map, and `mergeNewRoomMonsters`. The root problem is that coordinate
transformation is not enforced at a boundary — it's done ad-hoc in each call
site. Expect more of these until a single canonical transform function is
established and enforced.

Related PRs: #459, #461, #463, #466, #467

### Encounter + dungeon repositories are in-memory only

`internal/repositories/encounters/inmemory.go` and
`internal/repositories/dungeons/inmemory.go` are the only implementations.
State does not survive process restart. Character and draft repos have Redis
implementations; encounter and dungeon do not.

### Orchestrator size and complexity

`internal/orchestrators/encounter/orchestrator.go` is 5,577 lines with 70+
exported and unexported functions. `StartCombat` carries a `//nolint:gocyclo`
suppression. The file owns dungeon generation, combat resolution, monster turns,
room navigation, entity-state building, and event publishing. Splitting this into
focused sub-orchestrators (combat, dungeon, lobby) would improve testability.

### `fmt.Printf` log calls in production path

`internal/processors/event/processor.go:83` uses `fmt.Printf` to log publish
failures. No structured logging, no severity, no trace context. The same pattern
may exist elsewhere in the orchestrator.

### Handler TODO cluster in character handler/converters

`internal/handlers/dnd5e/v1alpha1/character/handler.go` has 8 TODO comments.
`internal/handlers/dnd5e/v1alpha1/character/converters.go` has 27 TODO
comments (verified by grep on docs/honest-status-snapshot branch — original draft said 20). Most are about incomplete proto enum mappings (spells, traits, tool
proficiencies, subraces, languages). Some responses return stub/zero values
today.

### Debug theme not restored

`internal/orchestrators/encounter/dungeon_mapper.go:44` routes the `"crypt"`
theme to `ThemeDebugWalls` with a TODO to revert. This means all crypt dungeons
render with debug walls in production.

## Per-subsystem confidence

See [quality.md](quality.md) for grade and rationale.

| Subsystem | Confidence |
|---|---|
| Encounter handler | Medium — clean RPC shell, but spatial/toolkit types leak in |
| Character handler | Medium — large converter surface with many TODO stubs |
| Encounter orchestrator | Medium-low — correct behavior but 5,577 lines, proto leakage, coordinate fragility |
| Character orchestrator | Medium-high — smaller, well-tested |
| Dungeon component | Medium — good tests, wrong repo; toolkit boundary violation |
| Spawner component | Medium — thin, functional |
| Event processor | Medium-high — clean interface, `fmt.Printf` in hot path |
| Redis publisher | Medium-high — works, serialization tested |
| Encounter repository (in-memory) | Low-medium — no persistence, data lost on restart |
| Dungeon repository (in-memory) | Low-medium — no persistence, data lost on restart |
| Character repository (Redis) | Medium-high — only persistent game-state store |
| Encounter log repository (in-memory) | Low — append-only design is correct but in-memory means no replay after restart |
| Integration test harness | Medium-high — good coverage of happy paths, Round 2 open-door test just added |
| Services layer (sandboxroom) | Low — sparse; most business logic lives in orchestrators |

### Lint — 65 pre-existing violations

`make pre-commit` fails on lint with 65 pre-existing violations as of 2026-05-02. All tests pass (`go test -short -race ./...` — 23 packages). The lint failures are in source code unrelated to the current docs branch. Categories:

- **goconst (42)** — magic string literals repeated 3+ times in dungeon toolkit, character and encounter converters
- **revive (5)** — `context.Context` not first param in test helpers, underscore in Go names
- **govet (4)** — error variable shadowing in harness and integration helpers
- **gocritic (3)** — sloppy reassignment patterns in encounter orchestrator
- **unconvert (3)** — unnecessary string() conversions in encounter orchestrator and character handler
- **unused (3)** — `convertEntityDoorsToProto`, `convertDungeonWallsToProto` in encounter converters; `protoActionIDToRef` in encounter orchestrator
- **staticcheck (1)** — `grpc.DialContext` deprecated in test harness
- **errcheck (2)** — unchecked errors in harness.Close()
- **misspell (1)** — "cancelled" → "canceled" in stream_entity_state_test.go
- **unparam (1)** — `handleRemainingChoices` always returns nil in helpers

The 3 unused functions (`convertEntityDoorsToProto`, `convertDungeonWallsToProto`, `protoActionIDToRef`) are the most interesting — they are dead code. `protoActionIDToRef` at line 2372 of orchestrator.go converts `pb.ActionId` but is never called. This is additional signal that the proto-to-ref conversion was moved inline without cleaning up.

## Upcoming work

- **Start fresh from main for Round 2** — cherry-pick or re-implement the
  six coordinate fixes; get CI green before merging.
- **Move dungeon component to rpg-toolkit** — tracked decision, not yet an
  open issue.
- **Redis implementations for encounter + dungeon repos** — required before
  any durability story.
- **Eliminate proto types from orchestrator** — introduce internal room/entity
  value types; move `buildRoomLayoutProto` and `buildRoomsMap` to the handler layer.
- **Character proto enum gaps** — spells, traits, subraces, languages all
  return stub values today.
- **PlayerDisconnected orchestrator hookup** — streaming handler exits cleanly
  but encounter state is not cleaned up.
- **Revert `ThemeDebugWalls` to `ThemeCrypt`** in `dungeon_mapper.go`.

## Related references

- [rpg-project/CLAUDE.md](../../rpg-project/CLAUDE.md) — cross-repo Boundary Rule
- [Project board #10](https://github.com/users/KirkDiggler/projects/10)
- [REFACTOR_PLAN.md](../REFACTOR_PLAN.md) — equipment-choice refactor deferred
- [docs/architecture/](architecture/) — existing architecture docs
