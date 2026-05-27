---
name: rpg-api status
description: Where we are with rpg-api — active work, paused, known rough edges, per-subsystem confidence
updated: 2026-05-25
confidence: high — Wave 2.11e entries verified against shipped code and passing integration tests
---

# rpg-api: Where We Are

This is a living doc. Edit it in the same PR that invalidates a line. Don't let it rot.

## Active work

**Chapter 2 Wave 1 (rogue) — devseed fixture landed (2026-05-27)** — `--fixture=wave-1-rogue`
seeds alice (L1 rogue, 1d6 SneakAttack, HP 10) + bob (L1 barbarian) + goblin into
`enc:v2:dev-encounter`. No wendy in this fixture. Closes issue #551.

**Wave 2.11e rpg-api — PR open (2026-05-25)** — MovementResolver wiring: OA-class
reactions end-to-end in both movement directions (player retreats past NPC → NPC OA;
NPC retreats past player → player OA) plus Disengage suppression. Pins
`encounter@v0.14.0` + `rulebooks/dnd5e@v0.59.0`.

What landed on the rpg-api side (issue #539):

- `Dnd5eMovementResolver` (`dnd5e_movement_resolver.go`) — implements
  `tkenc.MovementResolver` via `combat.MoveEntity` per step. Builds
  `encounterHexRoom` + `CombatantRegistry` + `gamectx` per step so the OA
  chain (MovementChain → OpportunityAttackCondition → triggerOpportunityAttack →
  combat.ResolveAttack) runs with correct spatial + readiness context.
- `encounterHexRoom.MoveEntity` (`hex_room.go`) — write path added; mutates
  `data.Players[*].View.Position` / `data.Monsters[*].Position` per step so
  successive ResolveStep calls see updated positions.
- `encounterHexRoom.GetGrid()` returns `SquareGrid` (required by
  `OpportunityAttackCondition.isLeavingMyThreatRange`).
- `applyMonsterReactionConditions` (`reaction_conditions.go`) — wires OA
  condition on monsters at rehydration time (without this, NPC-direction OA
  never fires because the condition's bus subscription never installs).
- `HandlerConfig.MovementResolverConfig` + `buildMovementResolver` — wires the
  resolver at all 7 `LoadFromData` / `New` sites in the handler.
- 3 integration tests (`integration_movement_oa_test.go`):
  - `TestPlayerOA_OnNPCFleeing` — goblin retreats via `MoveNPCSteps`, alice's OA fires, goblin HP drops.
  - `TestNPCOA_OnPlayerFleeing` — alice retreats via `MoveEntity` RPC, goblin's OA fires, `DamageDealtEvent` verified.
  - `TestRogueDisengage_NoOAFiresOnMovement` — alice activates `BonusDisengage` on the encounter's bus, retreats, no OA fires. Documents cross-RPC persistence gap.

Known follow-ups (filed separately, out of Wave 2.11e scope):
- Disengage cross-RPC persistence (condition evaporates on LoadFromData per-RPC bus reconstruction).
- Player-pause reactions / Sentinel (rpg-toolkit#665).
- `v2 ActivateCombatAbility` RPC for Disengage.
- Weapon-aware OA attacker (`triggerOpportunityAttack` uses unarmed strike placeholder).

**Earlier active state (still relevant):**

All six open PRs are coordinate-space fix branches feeding into the consolidated
Round 2 branch (#468). #468 itself is **failing CI** (test step, build passes) and
the current plan is to **abandon it and start fresh from main** — it picked up
merge debt from stacking fixes. None of these should be merged individually.

| PR | Branch | Status | Notes |
|----|--------|--------|-------|
| #459 | `fix/458-room-origins-monster-turns` | Open | superseded by #471 coordinate-types refactor |
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
- **PlayerDisconnected** — the streaming handler exits but the `PlayerDisconnected`
  orchestrator method is never called, so no encounter state cleanup fires.
  (The TODO lives in `handler.go` in the stream-disconnect path.)
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

### Publish failures silently swallowed in event processor

`internal/processors/event/processor.go` discards the publish error entirely with
`_, _ = p.publisher.Publish(...)`. There is no logging, no alert, and no retry.
A comment in the code acknowledges this ("In production, might want to log, retry,
or queue failed publishes") but no instrumentation exists today. Every combat event
could silently fail to reach connected clients with no observable signal.
The same pattern may exist elsewhere in the orchestrator.

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

## v1alpha2 encounter service (Wave 2.5 slice 1 — PR forthcoming)

Walking skeleton wired through the rpg-toolkit `encounter` SDK. `MoveEntity` and
`StreamEncounter` implemented; all other RPCs return `codes.Unimplemented`. Per-viewer
event projection works end-to-end (verified by `internal/integration/encounter_v2_test.go`).
v1alpha1 movement path remains the primary path for the web; deletion deferred until
web migrates (slice 2: rpg-dnd5e-web#387).

Known gaps: rpg-toolkit#629 (LoS-loss events when entity moves out of viewer range — slice
1 wave goal uses mutual LoS so this doesn't bite). `SnapshotDelivered.encounter` proto
field is empty for slice 1; toolkit `Snapshot` shape will be mapped when slice 2 needs it.

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
| Event processor | Medium-high — clean interface, publish failures silently swallowed |
| Redis publisher | Medium-high — works, serialization tested |
| Encounter repository (in-memory) | Low-medium — no persistence, data lost on restart |
| Dungeon repository (in-memory) | Low-medium — no persistence, data lost on restart |
| Character repository (Redis) | Medium-high — only persistent game-state store |
| Encounter log repository (in-memory) | Low — append-only design is correct but in-memory means no replay after restart |
| Integration test harness | Medium-high — good coverage of happy paths, Round 2 open-door test just added |
| Services layer (sandboxroom) | Low — sparse; most business logic lives in orchestrators |

### Lint — 70 pre-existing violations

`make pre-commit` fails on lint with 70 pre-existing violations as of 2026-05-25. All tests pass. Wave 2.11e fixed 1 new violation introduced by the WIP (`buildCombatantRegistry` unparam). Categories:

- **goconst (44)** — magic string literals repeated 3+ times in dungeon toolkit, character and encounter converters
- **revive (5)** — `context.Context` not first param in test helpers, underscore in Go names
- **govet (4)** — error variable shadowing in harness and integration helpers
- **gocritic (3)** — sloppy reassignment patterns in encounter orchestrator
- **unconvert (3)** — unnecessary string() conversions in encounter orchestrator and character handler
- **unused (3)** — `convertEntityDoorsToProto`, `convertDungeonWallsToProto` in encounter converters; `protoActionIDToRef` in encounter orchestrator
- **staticcheck (4)** — `grpc.DialContext` deprecated in test harness; `combat.ResolveAttack` deprecated in two orchestrator sites
- **errcheck (2)** — unchecked errors in harness.Close()
- **misspell (1)** — "cancelled" → "canceled"
- **unparam (1)** — `handleRemainingChoices` always returns nil in helpers

The 3 unused functions (`convertEntityDoorsToProto`, `convertDungeonWallsToProto`, `protoActionIDToRef`) are dead code. `protoActionIDToRef` at line 2372 of orchestrator.go converts `pb.ActionId` but is never called. This is additional signal that the proto-to-ref conversion was moved inline without cleaning up.

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

- [rpg-project/CLAUDE.md](https://github.com/KirkDiggler/rpg-project/blob/main/CLAUDE.md) — cross-repo Boundary Rule
- [Project board #10](https://github.com/users/KirkDiggler/projects/10)
- [REFACTOR_PLAN.md](../REFACTOR_PLAN.md) — equipment-choice refactor deferred
- [docs/architecture/](architecture/) — existing architecture docs
