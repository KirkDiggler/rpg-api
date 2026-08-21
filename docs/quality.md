---
name: rpg-api quality scorecard
description: Per-component grade with rationale — a graded scorecard to update as the codebase evolves
updated: 2026-07-21
confidence: medium — Wave 2.11e encounter v2 graded from shipped-code + integration test verification; older entries reflect 2026-05-02 snapshot pending refresh; rpg-api#636 note verified against passing tests; rpg-api#642 deletions verified against passing build/vet/test/lint; rpg-api#680 entries verified against passing unit + integration suite
---

# Quality Scorecard

Every component graded A–D. Grades reflect a holistic read of: API clarity,
test coverage, boundary discipline, known gaps, operational maturity.

This is a first draft. The grades are a starting point — expect them to change
as work lands and as the boundary violations are addressed.

## Handlers

### ~~Encounter handler — C+~~ DELETED (rpg-api#642, 2026-07-13)

`internal/handlers/dnd5e/v1alpha1/encounter/handler.go` — this file, its
converters, and the v1alpha1 EncounterService registration that served it are
gone. It was the audit-flagged live-but-legacy path (rpg-api's twin of
rpg-dnd5e-web#448's clean slate); the web made zero calls to it. See
`docs/status.md` "Active work" for the full deletion tally. The grade below
for the v2 encounter handler is now the only encounter handler grade.

### ~~Encounter v2 handler — B~~ DELETED (rpg-project#227, 2026-08-21)

`internal/handlers/dnd5e/v2/encounter/` — the v1alpha2 encounter stack this
section graded — is deleted in full (~44 files), along with its orchestrator
and repository (see below) and the `github.com/KirkDiggler/rpg-toolkit/
encounter` module it was built on. Replaced by the `rulebooks/dnd5e/session`
SDK, integrated directly into `LobbyService` — see `docs/architecture/
components/lobby-service.md`. There is no proto-level encounter handler grade
to give any more; gameplay verbs ride `SessionService`
(`internal/handlers/dnd5e/session/v1alpha1/`), not yet scored in this doc.
See `docs/status.md` "Active work" for the full deletion tally.

### Lobby handler — B+ (new, 2026-07-07)

`internal/handlers/dnd5e/lobby/v1alpha1/` — the LobbyService v1alpha1 stack
(rpg-api#629). Layered from day one: handler files are proto↔input translation
+ `lobbyStatusError` sentinel→status mapping only, one file per RPC, mirroring
the v2 encounter handler's post-#582 shape. `StreamLobby` mirrors
`StreamEncounter`'s subscribe-first/snapshot-first pattern. Grade held at B+
rather than A because the integration coverage is one gate test (4-player
create/join/ready/start) plus one edge case (late join) — the full edge-case
matrix (party cap, host migration on disconnect-vs-leave, idempotent rebind)
is unit-tested at the orchestrator layer but not yet re-proven at the wire
level.

### Session service handler — B (new, 2026-08-21)

`internal/handlers/dnd5e/session/v1alpha1/` — `SessionService`, rpg-api's one
interface to the toolkit's session SDK and, after rpg-api#801, the only
encounter surface it serves. Field-for-field translation (`convert.go` is the
whole of it), one file per verb, no rules and no invented vocabulary; the
converters are unit-tested per enum value and the projections are covered by
`internal/integration/session`. Held at B: rpg-api#803 — the verbs authenticate
but do not authorize the caller against session membership — is open.

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
its size. Its `EquipItem`/`UnequipItem` RPCs now delegate to the rules-correct
orchestrator method (rpg-api#680, see "Character orchestrator" below) — that
specific gap is closed even though the surrounding stub/TODO debt isn't.

### Character v2 handler — B (new, 2026-07-21)

`internal/handlers/dnd5e/v2/character/` (rpg-api#680) — the v1alpha2
`CharacterService`, out-of-encounter `EquipItem`/`UnequipItem` only. Deliberately
narrow: no `converters.go`, proto↔domain translation is small enough to stay
inline in `handler.go`. Delegates to the SAME orchestrator method the v1alpha1
handler uses and shares `BuildEquipmentCharacterData`
(`internal/handlers/dnd5e/v2/encounter/character_data.go`) with the encounter
snapshot path for its response composition — no independent logic to drift.
Gomock unit suite covers validation, delegation, and error→status mapping; the
end-to-end proof (real AC, occupancy visible across both surfaces) is the
integration suite in the encounter package (`encounter.md`). Held at B rather
than A pending production traffic and a genuine in-encounter equip path
(rpg-project#94) to prove the boundary holds under a second consumer.

## Orchestrators

### ~~Encounter orchestrator — C+~~ DELETED (rpg-api#642, 2026-07-13)

`internal/orchestrators/encounter/orchestrator.go` — the 5,844-line file this
grade described, plus its v1-only siblings (`dungeon_mapper.go`,
`monster_turns.go`, `perception.go`, `service.go`) — is deleted in full. This
was the most-flagged component in the repo: proto contamination (39 `pb.`
references), coordinate-space bugs, and a hardcoded debug theme all died with
it. The v2 orchestrator below is the only encounter orchestrator remaining and
was already graded on a materially different (and better) shape.

### Character orchestrator — B-

`internal/orchestrators/character/orchestrator.go`

Smaller, more focused, and better tested than its encounter sibling. Delegates
to toolkit correctly for the most part. Spell list fetching at line 2828 has a
TODO about alive-check logic not being implemented. No proto leakage observed.
The `service.go` / `orchestrator.go` split within the package follows the
defined pattern. Grade would be higher with better alive-state handling and
completing the TODO at line 3352 (monster turns for entities acting before
current entity).

**Update (rpg-api#680, 2026-07-21):** `EquipItem`/`UnequipItem` are rules-correct
now — load the runtime character, call the toolkit's `EquipItem`/`UnequipItem`
(occupancy, slot-compatibility), persist via a merge that deliberately avoids a
full `char.ToData()` overwrite (which is lossy for `BackgroundID`/`CreatedAt`/
non-registry inventory — see `character-orchestrator.md`'s Equipment section).
Grade held at B- rather than raised: the fix is real and well-tested, but the
alive-check and monster-turns TODOs that capped this grade are unrelated and
still open.

### Lobby orchestrator — B+ (new, 2026-07-07)

`internal/orchestrators/lobby/` — party assembly (join refs, membership,
ready flags, lifecycle) plus `StartEncounter`, the sole encounter-construction
path (subsumes the deleted v1alpha2 `CreateEncounter`). Never imports proto;
sentinel errors mirror `internal/orchestrators/encounter/v2`'s pattern. Atomic
member-set snapshot is a per-lobby `sync.Mutex` (`keyed_mutex.go`) — a
deliberate, documented tradeoff: rpg-api is single-process (same assumption
the v2 encounter broker wiring makes), so a Go-level lock is sufficient
without a Redis WATCH/MULTI transaction. One known leak: the mutex map never
evicts per-lobby entries — slow and usage-bounded (one UUID per lobby ever
created), not a hot-loop concern, called out as a follow-up rather than fixed
here. Full RPC-level unit coverage (create/join/rebind/ready/leave incl. host
migration/start incl. HP seeding and persist-then-emit ordering).

### Session orchestrator — B (new, 2026-08-21)

`internal/orchestrators/session/` — constructs the SDK `session.Manager` over
rpg-api's Redis-backed key-value repositories and the host's character
repository; owns no rules. Covered by its own suite and by
`internal/integration/session`. Held at B while rpg-api#800 (partial-failure
orphans) is unruled.

### sessionworld — B+ (new, 2026-08-21)

`internal/sessionworld/` — compiles the reference tomb through the toolkit's
`dungeonspec` and hands the lobby a world plus authored party seats. Its
defining test pins that placements are projected by the composition, not
copied: the entrance literal moved from `(1,-4)` to `(0,-3)` when
rpg-toolkit#1141 corrected the hex convention, with no code change here
(rpg-api#802). Below A only because it compiles exactly one world and
`ListDungeons` has no source of truth yet.

## Components

### ~~Dungeon component — B-~~ DELETED

`internal/components/dungeon/` no longer exists (already absent on `dev` before
rpg-api#801 — this entry had outlived the code). Dungeon geometry lives where
the old entry said it belonged: the toolkit's `rulebooks/dnd5e/encounter` and
its `dungeonspec` compiler, consumed through `internal/sessionworld`.

### ~~Spawner component — B~~ DELETED

`internal/components/spawner/` no longer exists (already absent on `dev` before
rpg-api#801). Monster placement is authored in the dungeon spec and compiled by
the toolkit.

## Infrastructure

### ~~Event processor — B+~~ DELETED (rpg-api#642, 2026-07-13)

`internal/processors/event/processor.go` is deleted. It had zero consumers
left once the v1 orchestrator and handler were gone — verified by grep before
deletion. The v2 path this note used to point to as the replacement is ALSO
deleted now (rpg-project#227, 2026-08-21 — see "Encounter v2 handler" above);
event delivery today is the `rulebooks/dnd5e/session` SDK's own event stream,
fanned out by `internal/orchestrators/session.Broker`, not graded in this doc.

### ~~Redis publisher — B~~ DELETED (rpg-api#642, 2026-07-13)

`internal/publishers/encounter/redis.go` is deleted alongside the event
processor that was its only real consumer.

## Repositories

### ~~Encounter repository (v1) — D~~ DELETED (rpg-api#642, 2026-07-13)

`internal/repositories/encounters/inmemory.go` (the v1 root, not the
`v2/` subdirectory) is deleted. See "Encounter repository v2" below for
its replacement, which is already Redis-backed.

### ~~Encounter repository v2 — B~~ DELETED (rpg-project#227, 2026-08-21)

`internal/repositories/encounters/v2/` (`redis.go`, `in_memory.go`) is deleted
along with the v2 encounter handler/orchestrator it backed (see "Encounter v2
handler" above). The lobby orchestrator's `StartEncounter` no longer persists
through a repository of its own — it builds directly onto the
`rulebooks/dnd5e/session` SDK's `sdk.Manager`, which owns its own session/
encounter persistence (`internal/orchestrators/session`'s Redis-backed
repositories), not graded in this doc.

### ~~Dungeon repository — D~~ DELETED (rpg-api#642, 2026-07-13)

`internal/repositories/dungeons/inmemory.go` is deleted. Dungeon state
(room layouts, revealed rooms, open doors, monster positions) was v1-only —
the v2 encounter path doesn't have a dungeon-state concept at this layer.

### ~~Encounter log repository — D~~ DELETED (rpg-api#642, 2026-07-13)

`internal/repositories/encounterlog/inmemory.go` is deleted. It had zero
consumers left once the v1 orchestrator, handler, and event processor were
gone.

### Lobby repository — B (new, 2026-07-07)

`internal/repositories/lobby/` — Redis-backed (`lobby:` prefix + a
`lobby:joinref:` secondary index for `GetByJoinRef`, 24h TTL refreshed on
every `Save`, mirroring `internal/repositories/encounters/v2/redis.go`'s
pattern) plus an in-memory variant for tests. Both round-trip through JSON on
every Save/Get like the v2 encounter repo. Grade held at B rather than higher
because it's brand new with no production traffic yet — the pattern is proven
but unexercised at scale.

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

### Integration test harness — B-

`internal/integration/harness/harness.go`

**Updated 2026-08-21 (rpg-project#227):** the harness no longer registers a
v1alpha2 EncounterService — that service is deleted along with the old
encounter stack (see "Encounter v2 handler" above). `TestServer` drops
`EncounterClientV2`/`BrokerV2`/`EncRepoV2`; `TestServer.SessionOrch` (a
miniredis-backed session orchestrator) is wired into the lobby orchestrator
instead, mirroring production. The six top-level `internal/integration/
*_test.go` suites that drove `LobbyService` through a real gRPC round-trip
via `EncounterClientV2`-backed assertions are deleted with it — see
`docs/architecture/components/integration-test-harness.md`. Grade dropped
from B+ to B-: the harness itself (container lifecycle, bufconn wiring,
CharacterService/DiceService/LobbyService registration) is intact and still
the most valuable test-infra piece in the repo, but it currently proves NO
full-stack, real-gRPC coverage of `LobbyService` or the session SDK — only
`internal/integration/character` and `internal/integration/session` (a
separate harness) exercise anything end-to-end today.

### Unit tests — B-

**Updated 2026-07-13 (#642):** the v1 orchestrator test files this section
named (`orchestrator_test.go`, `monster_turns_test.go`, `perception_test.go`,
`safe_placement_test.go`, `open_door_test.go`) are deleted along with the
5,844-line file they tested. File-count coverage numbers here predate the
deletion and need a fresh count in a future pass; not recomputed here since
`go test ./...` and `go vet ./...` both stayed green through every deletion
commit, which is the load-bearing signal for this PR. Main gap unaffected by
this deletion: the character converter files (3,132 lines and 1,658 lines)
have limited dedicated unit tests relative to their size.

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
