---
name: rpg-api quality scorecard
description: Per-component grade with rationale — a graded scorecard to update as the codebase evolves
updated: 2026-09-06
confidence: medium-high — #921 local-dev composition RPC integration is verified by focused handler/orchestrator, registration, and miniredis tests; #897 Appearance conversion/delegation remains verified by focused and Docker-backed integration tests
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
(`internal/handlers/dnd5e/session/v1alpha1/`), scored below.
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

### Session service handler — B+ (updated 2026-08-28)

`internal/handlers/dnd5e/session/v1alpha1/` is rpg-api's one gameplay interface
to the toolkit session SDK. Every member-naming verb applies the shared caller/member
gate before Manager dispatch; production and the harness now pass the same
`sessionaccess.Access` instance to SessionService and SessionPresentationService.
Conversion remains field-for-field with no rules or invented vocabulary, including
nested declarations/candidates and selectors. `GetRoster` delegates once to the Session
SDK and mechanically maps its public members and flat customization values, preserving
optional presence and explicit zero while keeping the shelf present-and-empty for
nil/default players and monsters; it projects no private sheet fields. `StreamEvents` is audience-filtered
best-effort live delivery; `GetStory` is persisted catch-up, and both use the same
typed-event converter. Unit and `internal/integration/session` suites cover ownership,
selectors, live fan-out, replay, and production declaration shapes. Held below A pending
production traffic.

### Session presentation handler — B+ (new, 2026-08-28)

`internal/handlers/dnd5e/sessionpresentation/v1alpha1/` adapts the presentation-only
`SessionPresentationService`. Both RPCs run the shared `CallerMemberSeated` gate before
service access, so caller ownership and Session SDK seating are identical to
SessionService. Mapping is proto ↔ proto-free domain structs plus a small status switch:
invalid plans become `InvalidArgument`, duplicate conflicting attempts become
`AlreadyExists`, and storage/subscription failures are sanitized as `Internal`. It does
not call toolkit/session APIs or inspect Story. Held at B+ rather than A because browser
consumption has not happened yet, even though unit tests and the cross-instance Redis
integration cover the contract.

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

**Update (rpg-api#897, 2026-09-03):** complete Appearance now converts field-for-field
to toolkit customization data. `UpdateAppearance` delegates once and returns the
service's complete DraftData without a second Get; malformed semantics reach
`Draft.SetAppearance`. Finalization/Get/List/equipment map nested `Data.Appearance`
naturally. Grade remains C because the older converter/TODO debt above is unchanged.

### Character v2 handler — B+ (updated 2026-08-25)

`internal/handlers/dnd5e/v2/character/` (rpg-api#680/#844) serves authenticated-owner
`GetCharacterData` plus out-of-encounter `EquipItem`/`UnequipItem`. Ownership precedes
private projection, missing/foreign NOT_FOUND responses are byte-identical, and all
three methods map the same detached orchestrator View through `BuildCharacterData`.
Writes return the actual repository-patched entity plus its precomposed matching View;
both handler versions answer from that same post-state with no fallible post-write reload.
The mapper includes PlayerID and structured class/race refs as well as equipment/status,
and strict failures use generic INTERNAL transport wording while retaining internal causes.
Focused gates cover malformed-data no-write behavior, projection-before-write, map
isolation, atomic-patch failure, post-state equality, all three RPC identity mappings,
complete level-3 Fighter status, optional presence, and representative four-build mapping.
Held below A pending production traffic and a second live consumer of the owner-private
composition.

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

The orchestrator delegates character rules to the toolkit and has no proto leakage.
Its `service.go` / `orchestrator.go` split follows the defined Input/Output pattern.
Verified remaining TODOs concern draft mutation access, background validation, error
logging, and pagination/class-filter placeholders; they are outside the owner-private
equipment path.

**Update (rpg-api#897, 2026-09-03):** Appearance now lives in toolkit
`character.Data`/`DraftData`; `SetAppearance` loads, calls `Draft.SetAppearance` once,
updates with `draft.ToData()`, and returns stored DraftData. Redis and session-save
paths use nested toolkit state without a sibling envelope. The grade remains B- because
older draft/catalog TODO debt below is unchanged.

**Update (rpg-api#680/#844, 2026-08-25):** `EquipItem`/`UnequipItem` strictly
load/attach, call the toolkit's rules-aware verbs, precompose complete post-views, and
persist only EquipmentSlots plus cached ArmorClass through the repository's atomic
optimistic patch. Unrelated concurrent character changes are reprojected and preserved;
stale equipment is aborted. The orchestrator returns the actual patched entity and the
matching detached View. Grade held at B- because older draft/catalog TODO debt elsewhere
in this large orchestrator is unchanged.

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

### Session presentation orchestrator — B+ (new, 2026-08-28)

`internal/orchestrators/sessionpresentation/` validates bounded dice throw drafts,
binds server-owned session/roller fields, encodes deterministic JSON payloads, and
delegates acceptance/fan-out to the repository. It is proto-free and toolkit-free:
`authority_seq` is only a carried reference and no Story/session Manager method is
called. Unit tests cover normalization, conflict mapping, malformed payload dropping,
close propagation, and validation limits. Held below A until live web traffic exercises
Pub/Sub behavior outside tests.

### sessionworld — B+ (new, 2026-08-21)

`internal/sessionworld/` — compiles one authored dungeon file (`Compile(raw)`)
through the toolkit's `dungeonspec` into a world plus authored party seats and
monster placements. It holds no content since rpg-api#806: the reference tomb
is `content/reference-tomb.yaml`, loaded by `internal/dungeons`. Its defining
test pins that placements are projected by the composition, not copied: the
entrance literal moved from `(1,-4)` to `(0,-3)` when rpg-toolkit#1141
corrected the hex convention, with no code change here (rpg-api#802). The borrowed-projection step (a throwaway encounter) is gone
with dungeonspec v2 (rpg-project#256): the one conversion is
`encounter.HexCellAt`, asked for, not reimplemented. Below A only until the
toolkit pins are real tags rather than pseudo-versions.

### Dungeon content registry + authoring — B+ (new, 2026-08-23)

`internal/dungeons/`, `internal/orchestrators/authoring/`,
`internal/handlers/dnd5e/authoring/v1alpha1/` (rpg-api#806). The registry's
contract — boot refuses a non-compiling file naming it, atomic temp+rename
writes, per-key serialization proven under `-race`, verbatim bytes back — and
the handler's transport rules (status for a malformed request, body for a file
that does not compile) are unit-tested; the lobby suite pins that a `Put`
dungeon starts and its `GetAtlas` is cell-for-cell the atlas `Put` answered.
Held below A until Kirk's walk and the toolkit tags. See
`docs/architecture/components/authoring-service.md`.

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

### Session presentation repository — B+ (new, 2026-08-28)

`internal/repositories/sessionpresentation/` accepts deterministic presentation payloads
with a Redis script: first writer stores and publishes, identical duplicate returns the
accepted payload, conflicting duplicate returns `ErrConflict`. Accepted keys live for two
minutes; Redis Pub/Sub streams are live-only and close with context/subscription shutdown.
Unit tests cover key hashing, TTL, duplicate/conflict behavior, fan-out, and close paths;
`internal/integration/sessionpresentation` proves two server instances over one Redis.
Held below A until web traffic exercises the live channel.

### Composition repository — B (new, 2026-09-06)

`internal/repositories/composition/` provides typed Create, Get, and List operations over
toolkit `world/composition.Data`. One Redis hash per world stores each composition under
its caller-supplied ID; HSETNX prevents overwrite, HGET/HGETALL serve reads, records do
not expire, and lists are sorted by ID. Miniredis tests cover round trips, same-ID world
isolation, duplicate refusal, absent/empty results, storage/decode errors, and snapshot
independence.

The published Create/Get/List wire contract now has a thin handler and orchestrator:
the handler requires the existing player context, matches the configured dev-only world,
and maps JSON strings to `json.RawMessage`; the orchestrator mints IDs before repository
Create. Registration is limited to `AUTH_DEV_MODE=true`, Create retains the separate
`RPG_AUTHORING_ENABLED=1` mutation gate, and reads remain available within dev mode.
Focused tests exercise the wire boundary through miniredis and prove non-dev absence.
Held at B because this local stub intentionally has no production guild-to-world mapping
or production traffic.

### Character repository — B+

`internal/repositories/character/redis.go` remains the durable character store. In
addition to CRUD/index operations it now exposes an Input/Output-typed atomic equipment
patch. Redis WATCH/MULTI guards the record, expected equipment rejects stale equip
writers with ABORTED, unrelated record revisions are returned without writing, and the
successful transaction changes only EquipmentSlots plus cached ArmorClass on the latest
entity. Miniredis regressions cover concurrent combat-state preservation and stale
expected-equipment refusal. Held below A because general full-record Update callers have
not been redesigned and TTL/stale-character lifecycle remains unchanged.

### Character draft repository — B-

`internal/repositories/character_draft/redis.go`

Redis-backed. Handles in-progress character creation state. #897 adds complete
nested toolkit Appearance JSON round trips, detached nested pointer assertions, and
present-zero optional scalar coverage. Broad repository lifecycle coverage remains
thinner than the character repository, so the grade does not change. No known
correctness gaps.

### Dice session repository — B-

`internal/repositories/dice_session/redis.go`

Redis-backed. Narrow scope. No observed gaps. Low risk.

## Testing

### Integration test harness — B+ (updated 2026-08-28)

`internal/integration/harness/harness.go`

The harness still omits the deleted v1alpha2 EncounterService, but #852 restores the
new-stack full-gRPC value and #869 uses its real `redis:7-alpine` character path to prove
hair update/reload/refusal/finalization/Get: `TestServer` exposes `SessionClient`,
`SessionPresentationClient`, and `HealthClient`; registers SessionService and
SessionPresentationService; and shares the real Session Manager with lobby launch and
both access gates. The new `internal/integration/sessionpresentation` package starts two
TestServers over one Redis container and proves real cross-instance seating plus Redis
Pub/Sub. Grade returns to B+ from B-; held below A pending wider lobby/session full-stack
suites and browser evidence.

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
