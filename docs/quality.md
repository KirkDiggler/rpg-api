---
name: rpg-api quality scorecard
description: Per-component grade with rationale — a graded scorecard to update as the codebase evolves
updated: 2026-05-02
confidence: low-medium — first draft grades from code read-through and git history; needs Kirk's correction pass
---

# Quality Scorecard

Every component graded A–D. Grades reflect a holistic read of: API clarity,
test coverage, boundary discipline, known gaps, operational maturity.

This is a first draft. The grades are a starting point — expect them to change
as work lands and as the boundary violations are addressed.

## Handlers

### Encounter handler — C+

`internal/handlers/dnd5e/v1alpha1/encounter/handler.go` (1,116 lines)

The RPC shell is correctly structured: validate → call service → convert.
Auth context extraction works. The drag: `spatial.GridTypeHex` and
`spatial.HexOrientationPointyTop` are hardcoded six times, and a
`*toolkitchar.Data` type assertion lives here. Handlers should not import
toolkit packages — those types belong in the orchestrator output. Removing
the toolkit imports is a one-PR fix but it requires the orchestrator to stop
returning toolkit types in `CharacterData`. The `TODO` on `PlayerDisconnected`
at line 695 means disconnect events do not clean up encounter state.

### Character handler — C

`internal/handlers/dnd5e/v1alpha1/character/handler.go` (1,070 lines),
`converters.go` (3,132 lines)

Largest file surface in the repo. Conversion layer is necessarily big, but
the 20 TODO comments in converters.go — returning `SPELL_UNSPECIFIED`,
`TRAIT_UNSPECIFIED`, empty equipment data, no language enums — mean that the
character API silently returns stub data for real fields. One TODO in
`handler.go:765` explicitly names the wrong-layer smell: "handler should not
interact with toolkit, this belongs in the orchestrator." That smell is present
but not yet addressed. Test coverage exists via `list_equipment_test.go` and
`list_spells_test.go` but the converter surface is sparsely tested relative to
its size.

## Orchestrators

### Encounter orchestrator — C+

`internal/orchestrators/encounter/orchestrator.go` (5,577 lines)

The most critical file in the codebase and the most at-risk. Correct behavior
has been established through Round 1, but the file owns too much: dungeon
generation, room navigation, combat resolution, monster turns, entity-state
snapshots, event publishing, and coordinate transforms. The `StartCombat`
function is large enough to carry a `//nolint:gocyclo` suppression. Proto
types (`*pb.RoomLayout`, `*pb.EntityState`, `*pb.CombatStateProto`) are
constructed inside the orchestrator — 39 `pb.` references (verified by grep) — which
violates the handler→orchestrator boundary. The service.go Input/Output types also
embed `pb.MonsterType`, `pb.CombatAbilityId`, and `pb.ActionId` directly, propagating
proto contamination to all callers of the Service interface. The five coordinate-space bug fixes landed in three days
(2026-04-04 to 2026-04-06) are a signal that the transform logic needs a
canonical home. The `dungeon_mapper.go` hardcodes `ThemeDebugWalls` for crypt
with a TODO to revert.

### Character orchestrator — B-

`internal/orchestrators/character/orchestrator.go`

Smaller, more focused, and better tested than its encounter sibling. Delegates
to toolkit correctly for the most part. Spell list fetching at line 2828 has a
TODO about alive-check logic not being implemented. No proto leakage observed.
The `service.go` / `orchestrator.go` split within the package follows the
defined pattern. Grade would be higher with better alive-state handling and
completing the TODO at line 3352 (monster turns for entities acting before
current entity).

## Components

### Dungeon component — B-

`internal/components/dungeon/` (generator, room shapes, wall perimeter, door
spawning, hex coords, monster placement, theme tables)

The component is well-tested — integration tests use real toolkit generators,
unit tests cover shape selection, perimeter logic, placement validation, and
door spawning. The design is clean with internal adapters and toolkit wrappers.
The grade cannot be higher because **this component is in the wrong repository**.
Procedural dungeon generation (room shapes, wall placement, monster CR
budgets, hex-grid coordinates) is game logic — it belongs in rpg-toolkit under
the Boundary Rule. As long as it lives here, rpg-api violates its own mandate
of "data orchestration, never game logic." Moving it is a known planned task.

### Spawner component — B

`internal/components/spawner/`

Thin adapter around toolkit placement logic. Well-contained, single
responsibility. No known gaps. Lower-risk relative to dungeon component because
it delegates rather than implementing rules.

## Infrastructure

### Event processor — B+

`internal/processors/event/processor.go`

Clean two-step: persist to encounter log, then publish to Redis. Correct
semantics — publish failure is logged but does not fail the operation because
persistence is the source of truth. Interface is minimal and well-defined with
Input/Output types. Main drag: `fmt.Printf` on line 83 for publish failure
logging. No structured logging, no severity level, no trace context — this is
the hot path for every combat event and deserves a proper logger.

### Redis publisher — B

`internal/publishers/encounter/redis.go`

Proto serialization tested in `redis_test.go`. Interface is clean. No retry
on publish failure (consistent with processor's best-effort semantic, but
worth documenting). Works correctly for the current in-process use case.

## Repositories

### Encounter repository — D

`internal/repositories/encounters/inmemory.go`

In-memory only. No Redis implementation. All encounter state is lost on
process restart. The interface is correct and well-designed (Input/Output types,
proper mock), but without a persistent backend this is a development scaffold,
not a production store. The `copyEncounterData` deep-copy helper (added via
PR #453 fix) is a positive sign of data-integrity awareness, but it does not
address the durability gap.

### Dungeon repository — D

`internal/repositories/dungeons/inmemory.go`

Same situation as the encounter repository. In-memory only. Dungeon state
(room layouts, revealed rooms, open doors, monster positions) is lost on
restart. Redis implementation needed alongside the encounter repo.

### Encounter log repository — D

`internal/repositories/encounterlog/inmemory.go`

Append-only event log is the right design for event sourcing. But in-memory
means no replay, no late-join recovery after a server restart, and no audit
trail beyond the current process lifetime. The interface is correct; the
implementation is a stub.

### Character repository — B

`internal/repositories/character/redis.go`

The only repository with a persistent (Redis) implementation. Used by the
integration test harness. Tested with real Redis via the harness. Solid
foundation. No major gaps observed, though no evidence of TTL management or
stale-character cleanup.

### Character draft repository — B-

`internal/repositories/character_draft/redis.go`

Redis-backed. Handles in-progress character creation state. Less tested than
the character repo (no integration tests that specifically exercise draft
lifecycle). No known correctness gaps.

### Dice session repository — B-

`internal/repositories/dice_session/redis.go`

Redis-backed. Narrow scope. No observed gaps. Low risk.

## Testing

### Integration test harness — B+

`internal/integration/harness/harness.go` and
`internal/integration/encounter/`

The harness wires the full stack (real orchestrator, real repos, real toolkit)
and drives scenarios through the encounter flow. Class-specific test files
(barbarian, fighter, monk, rogue) cover class-specific combat paths. The
`open_door_test.go` and `stream_entity_state_test.go` were recently added to
cover Round 2 flows. The harness is the most valuable test asset in the repo —
it catches integration regressions that unit tests with mocks cannot. Gap:
Round 2 multi-room flows are partially covered but the failing CI on #468 means
the most recent additions have not been verified green on main yet.

### Unit tests — B-

66 test files for 101 source files (~65% file coverage by count). Test suites
use gomock correctly at interface boundaries. The encounter orchestrator test
files (`orchestrator_test.go`, `monster_turns_test.go`, `perception_test.go`,
`safe_placement_test.go`, `open_door_test.go`) cover key orchestration paths.
Main gap: the converter files (3,132 lines and 1,658 lines) have limited
dedicated unit tests relative to their size. The `build_room_layout_proto_test.go`
was added as a direct response to the coordinate-space bug cluster — good reflex,
but the pattern of writing tests after finding bugs rather than before is a risk
signal.

## Grade legend

- **A** — strong design, good tests, boundary-clean, no major known gaps
- **B** — works reliably; some known gaps or missing polish
- **C** — has known boundary violation, significant untested surface, or operational risk
- **D** — stub/scaffold implementation; would fail in a real deployment

## How this doc is meant to work

Grades are a first draft from 2026-05-02 based on code read-through and git
history. When you fix a gap named here, update the grade and leave a reason.
Don't just move a letter.

The intended evolution:

1. **Today** — human-curated grades from read-through
2. **Soon** — integration test coverage numbers give signal for repo and
   orchestrator grades
3. **Later** — CI coverage reports auto-flag when grades and coverage diverge
