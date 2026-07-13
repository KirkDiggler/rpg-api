---
name: rpg-api quality scorecard
description: Per-component grade with rationale — a graded scorecard to update as the codebase evolves
updated: 2026-07-13
confidence: medium — Wave 2.11e encounter v2 graded from shipped-code + integration test verification; older entries reflect 2026-05-02 snapshot pending refresh; rpg-api#636 note verified against passing tests; rpg-api#642 deletions verified against passing build/vet/test/lint
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

### Encounter v2 handler — B (Wave 2.11e update)

`internal/handlers/dnd5e/v2/encounter/` — the v1alpha2 encounter stack.
Since the #582 carve-out the RPCs delegate load → verb → persist to the v2
orchestrator (`internal/orchestrators/encounter/v2`), leaving the handler as
proto↔input + sentinel→status mapping; the resolvers below stay handler-side
(depguard-excluded) because they touch the rulebook. Wave 2.11e adds the
MovementResolver wiring, bringing OA-class reactions end-to-end for both
movement directions.

What's here as of Wave 2.11e:

- `Dnd5eCombatResolver` (`dnd5e_combat_resolver.go`, `dnd5e_combat_resolver_phased.go`)
  — implements both `tkenc.CombatResolver` and `tkenc.PhasedCombatResolver`.
  Cached attacker prep map (`pendingPhased`) lets phase 2 reuse the same
  loaded character + resolveCtx without creating duplicate condition
  subscribers.
- `Dnd5eMovementResolver` (`dnd5e_movement_resolver.go`) — implements
  `tkenc.MovementResolver`; delegates to `combat.MoveEntity` per hex step.
  Builds spatial room + combatant registry + gamectx per step so OA chain
  fires correctly (see `encounter.md` MovementResolver wiring section).
- `TakeAction` (`take_action.go`) — RPC for player-initiated combat actions.
  Any menu ref routes through the toolkit dispatch (encounter v0.20+); the
  handler translates the target oneof per the advertised TargetKind contract —
  entity_id, self (→ actor's own id), unset oneof (untargeted NONE) — and
  rejects reserved position/area (#605). Carved onto the v2 orchestrator
  (`internal/orchestrators/encounter/v2/take_action.go`, #582 step 5): the
  handler is now proto↔input + sentinel→status mapping; the orchestrator owns
  load (with the #689 hydration cascade so the resolver reads the held
  attacker/defender — the #684 double-subscribe cure) → `enc.TakeActionPhased`
  → persist, and persists `PendingReactionPrompts` + publishes
  `InputRequiredDelivered` (save-before-publish, #538 A) when phase 1 surfaces a
  player reactor. The single rulebook-touching piece — marshaling the resolver's
  native `*combat.AttackContext` into the opaque `AttackContextJSON` — is the
  injected `ReactionResume.MarshalAttackContext` (the phase-1 counterpart to the
  SubmitCheck-side decode), kept in `reaction_resume.go` so the orchestrator
  stays rulebook-free. Joins `Interact`/`SubmitCheck`/`SetReactionReady`/
  `ActivateFeature` on the orchestrator's single-`load` core.
- `MoveEntity` (`handler.go`) — RPC for moving a player's controlled entity
  along a proposed path. Carved onto the v2 orchestrator
  (`internal/orchestrators/encounter/v2/move_entity.go`, #582 step 6): the
  handler is now proto↔input (proto positions → `core.Hex`) + sentinel→status
  mapping; the orchestrator owns load (with the #689 hydration cascade so the
  movement resolver reads the held mover — the #684 double-subscribe cure) →
  `enc.Move` → `persistWithCharacterData`. Empty-path is classified as
  `ErrEmptyPath` after load so the auth sentinels keep precedence; toolkit Move
  refusals join `ErrMoveRefused` → `FailedPrecondition`. The opportunity-attack
  resolution rides the injected `BuildMovementResolver` (the rulebook-importing
  `Dnd5eMovementResolver` translation seam stays handler-side); its lookup-only
  threatener load runs on a throwaway bus (not the encounter bus), so it is not a
  #684 source and is carried as-is. Joins the rest of the verbs on the
  orchestrator's single-`load` core.
- `SubmitCheck` (`submit_check.go` + `submit_check_reaction.go`) — dispatches
  to the reaction branch when the caller's pending prompt is a reaction;
  unmarshals the persisted `AttackContextJSON` back into `combat.AttackContext`
  and feeds it into `CompleteTakeAction` with the chosen modifiers.
- `SetReactionReady` (`set_reaction_ready.go`) — RPC for per-character
  per-condition readiness toggle. Carved onto the v2 orchestrator
  (`internal/orchestrators/encounter/v2/set_reaction_ready.go`, #582 step 3):
  the handler is now proto↔input + sentinel→status mapping; the orchestrator
  owns load → `enc.SetReactionReady` → persist. Joins `Interact` (#582 step 1)
  and `SubmitCheck` (#582 step 2) on the orchestrator's single-`load` core.
- `ActivateFeature` (`activate_feature.go`) — RPC for in-encounter feature
  activation (Rage et al.). Carved onto the v2 orchestrator
  (`internal/orchestrators/encounter/v2/activate_feature.go`, #582 step 4): the
  handler keeps building `CharDataJSON` (the rule-ish character serialization +
  in-combat `ActionEconomy` injection stays handler-side) and persists the
  verb's `UpdatedCharData`; the orchestrator owns load → `enc.ActivateFeature` →
  persist. This step also **retired the handler-package `Runner`** — it was the
  Runner's only caller, so `runner.go` (and its `buildCombatResolver` /
  `buildMovementResolver`, duplicates of the orchestrator's `load` resolver
  wiring) was deleted. The orchestrator's `load` is now the single load core for
  every carved verb. #691 (toolkit, OPEN): ActivateFeature's load deliberately
  does NOT use the #689 hydration cascade (`WithCharacterData` stays false) — the
  toolkit verb self-loads the actor from `CharDataJSON`; attaching it via the
  cascade would reintroduce the #684-class double-subscribe collision.
- `EndTurn` (`end_turn.go`) — handles `IsNPCPausedForReaction(err)`;
  `serializePendingPhasedAttacks` marshals the live `*PhasedAttackContext`
  into `AttackContextJSON` before snapshot (honors the encounter SDK's
  HOST CONTRACT documented in `persistNPCPendingReactions`). rpg-api#636: its
  NPC dispatch loop is now the shared `driveNPCChain`, also used by
  `DriveStalledNPCTurn` (`drive_npc.go`) — the "combat-entry kick" that
  `StreamEncounter`/`GetEncounter` call (best-effort, error-swallowed) at every
  connect so a TURN_BASED encounter that starts with an NPC active (no
  preceding `EndTurn` to chain off of) doesn't stall forever. Single-flighted
  per encounter ID (`keyed_mutex.go`) against concurrent connects.
- `applyReactionConditions` + `applyMonsterReactionConditions`
  (`reaction_conditions.go`) — wires OA on every character and monster +
  Shield on spellcasters. Both applied at every rehydration; idempotent.
- Translator (`translate.go`) — the `ReactionPrompt` oneof variant on
  `InputRequired` + `InputRequiredDelivered` event publication.
- `encounterHexRoom` (`hex_room.go`) — `spatial.Room` adapter over
  encounter data; `MoveEntity` write path added for per-step position mutation.

Test coverage:

- 5 Wave 2.11c sneak-attack regression tests (cross-RPC bus, gamectx,
  TurnEnd reset, once-per-turn enforcement).
- 3 Wave 2.11e movement OA integration tests (`integration_movement_oa_test.go`):
  player OA on NPC fleeing (via `MoveNPCSteps`); NPC OA on player fleeing
  (via `MoveEntity` RPC); Disengage suppression (on same bus).
- Full integration suites pass.
- Dedicated player Shield + OA tests: `integration_player_shield_test.go`
  (Wave 2.11d #536 partial — wired in same wave).

Grade remains B: surface is well-shaped, resolver wiring is tested end-to-end.
Gaps keeping it below A: Disengage cross-RPC persistence (condition evaporates
between TakeAction and MoveEntity due to per-LoadFromData bus reconstruction),
player-pause reactions (toolkit#665 future), and the HOST CONTRACT around
`AttackContextJSON` serialization is documented but not yet structurally
enforced from the SDK side (rpg-toolkit#657).

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

### ~~Event processor — B+~~ DELETED (rpg-api#642, 2026-07-13)

`internal/processors/event/processor.go` is deleted. It had zero consumers
left once the v1 orchestrator and handler were gone — verified by grep before
deletion. The v2 path publishes through the toolkit's own `tkenc.Broker`
instead, a different mechanism with its own (untracked-here) grade.

### ~~Redis publisher — B~~ DELETED (rpg-api#642, 2026-07-13)

`internal/publishers/encounter/redis.go` is deleted alongside the event
processor that was its only real consumer.

## Repositories

### ~~Encounter repository (v1) — D~~ DELETED (rpg-api#642, 2026-07-13)

`internal/repositories/encounters/inmemory.go` (the v1 root, not the
`v2/` subdirectory) is deleted. See "Encounter repository v2" below for
its replacement, which is already Redis-backed.

### Encounter repository v2 — B

`internal/repositories/encounters/v2/` (`redis.go`, `in_memory.go`)

Unlike the deleted v1 repo, this one has both a Redis implementation (wired
into production in `cmd/server/server.go` with a 24h TTL) and an in-memory
variant for tests, both covered by their own test files. This is the
repository the v2 encounter path and the lobby orchestrator's `StartEncounter`
actually use. Held at B rather than A pending a closer read of TTL/eviction
behavior under real traffic — not graded from a fresh deep read in this pass,
just confirmed to exist, compile, and pass its own tests.

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

### Integration test harness — B+

`internal/integration/harness/harness.go`

**Updated 2026-07-13 (#642):** the v1-only `internal/integration/encounter/`
suite (barbarian/fighter/monk/rogue class tests, `open_door_test.go`,
`stream_entity_state_test.go`, and siblings — 10 files) that exercised the now-deleted
v1alpha1 EncounterService is deleted along with it. The harness itself is trimmed
of matching v1 wiring (no more `EncounterClient`, v1 orchestrator/repo/publisher
construction) but keeps registering CharacterService, DiceService, the v1alpha2
EncounterService, and LobbyService. Coverage for the encounter flow now lives in
`internal/integration/encounter_v2_test.go` and `internal/integration/
lobby_v1alpha1_test.go`, both unaffected by this deletion and already passing.
Grade held at B+: the harness remains the most valuable test asset in the repo,
now scoped to the paths that are actually live.

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
