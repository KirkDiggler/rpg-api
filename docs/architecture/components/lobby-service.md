---
name: lobby service
description: LobbyService v1alpha1 — party assembly (join refs, membership, ready flags, lifecycle) and the sole encounter-construction path
updated: 2026-08-21
confidence: medium — the party-assembly RPCs (CreateLobby/JoinLobby/SetReady/LeaveLobby/SetConnected/StreamLobby) below are still accurate as of rpg-project#227's rip-out; the StartEncounter/AbandonEncounter/GetMyActiveLobby internals and the "Known gaps"/"Verify" sections describing the OLD encounter stack are NOT — see the rpg-project#227 notes inline rather than a full rewrite
---

# lobby service

**Partially stale (rpg-project#227, 2026-08-21):** the old encounter stack
(`github.com/KirkDiggler/rpg-toolkit/encounter`, `internal/orchestrators/
encounter/v2` — see [`encounter.md`](./encounter.md)) this doc was written
against is deleted. `StartEncounter` now builds directly onto the
`rulebooks/dnd5e/session` SDK (`internal/orchestrators/lobby/
start_encounter_session_stack.go`, the sole implementation now — there is no
second stack to coexist with). `AuthoringService`/`internal/dungeonregistry`
and `LobbyService.ListDungeons` are also deleted (see
[`authoring.md`](./authoring.md)); `ListDungeons` is `Unimplemented`. The
sections below that describe old-stack internals (file names like
`start_encounter.go`/`crypt_monster_seed.go`, the `BuildCombatResolver`/
`BuildMovementResolver` exports, `tkenc.LoadFromData` in AbandonEncounter,
`devcombat`/`devseed`) are historical record of the pre-#227 shape, not
current code — not rewritten into new architecture prose here.

The lobby service is the party-assembly surface: N players join refs/ready-up/
host-migrate before a session exists at all. `StartEncounter` is the **only**
way a session comes into existence.

Design doc: `rpg-project/ideas/game-screen-rebuild/lobby-surface.md`. Umbrella:
KirkDiggler/rpg-project#81. Implementation issue: rpg-api#629.

## Boundary

The toolkit has **zero** lobby concept, and this component keeps it that way
(CLAUDE.md's Boundary Rule: "if it's data storage or API orchestration → rpg-api"). A
lobby's `Data`/`Member` types are rpg-api-owned entities (`internal/repositories/lobby`),
not a toolkit mirror. `StartEncounter`'s toolkit construction (`tkenc.New` +
`AddPlayer` per member) is data movement — building the toolkit's own encounter by
reference — the same "no boundary violation, it's data movement" classification
rpg-api#616 gave the single-caller version this replaces.

## Layers

```
internal/handlers/dnd5e/lobby/v1alpha1/   proto <-> input translation, sentinel->status mapping
internal/orchestrators/lobby/             load -> mutate -> persist -> publish core
internal/repositories/lobby/              Redis (+ in-memory) persistence
```

Layered from day one — Chapter-1 discipline, not the handler-package orchestration
shape rpg-api#616 flags on the old v2 encounter create/hydrate path.

### Handler (`internal/handlers/dnd5e/lobby/v1alpha1/`)

One file per RPC (`create_lobby.go`, `join_lobby.go`, `set_ready.go`,
`leave_lobby.go`, `start_encounter.go`, `stream_lobby.go`, `get_my_active_lobby.go`,
`abandon_encounter.go`), plus `handler.go` (Config/New only), `translate.go` (entity
<-> proto), and `status.go` (one shared `lobbyStatusError` switch covering every
sentinel — each RPC only ever returns a subset, so one exhaustive mapper beats six
near-duplicates).

`get_my_active_lobby.go` (rpg-api#653) is the resume-after-refresh lookup
(rpg-dnd5e-web#444): unlike every other RPC here, it takes no request fields —
identity comes entirely from `auth.GetPlayerID(ctx)`, matching `StreamLobby`'s
pattern (a client-supplied `player_id` would let a caller query another player's
lobby). "No active lobby" is a valid empty response, not an error — see the
Repository section below for the index it reads and the orchestrator's
`GetMyActiveLobby` doc comment for the STARTED-lobby liveness cross-check.

`abandon_encounter.go` (rpg-api#663) is the host-only escape hatch for a stuck or
unwanted STARTED encounter — Kirk's live `FREE_ROAM` deadlock (rpg-toolkit#483) with
no other way to end. Host-only (`ErrNotHost`, matching `StartEncounter`'s check via
`HostPlayerID`); `FailedPrecondition` (`ErrLobbyNotStarted`) if the lobby is still
`WAITING`; delegates entirely to `Encounter.End(EncounterEndedReasonAbandoned)`
(rpg-toolkit#797/#798) — the SAME terminal transition victory/TPK already use, not a
parallel end path. Resume-refusal after an abandon needs no cooperating write here:
`GetMyActiveLobby`'s existing liveness check (below) already zeroes its whole Output
the moment the encounter's persisted `Mode` reads `ModeEnded`, so a stuck encounter
just... stops being resumable, the instant `End` persists.

`StartEncounter` needs the SAME production combat/movement resolvers the v2
encounter handler builds (`Dnd5eCombatResolver` / `Dnd5eMovementResolver` —
rulebook-importing adapters). Rather than duplicating them, this package exports
`BuildCombatResolver` / `BuildMovementResolver` adapter funcs
(`handler.go`) that wrap `encounterhandlerv2.NewDnd5eCombatResolverForData` /
`NewDnd5eMovementResolverForData` into the orchestrator's builder shape — the wiring
layer (`cmd/server/server.go`, the integration harness) calls these once and hands the
result into `lobbyorch.Config`.

### Orchestrator (`internal/orchestrators/lobby/`)

Never imports proto. Sentinel errors (`errors.go`) mirror
`internal/orchestrators/encounter/v2`'s pattern — the handler maps them to gRPC codes,
not this package.

- **`keyed_mutex.go`** — a per-lobby-ID `sync.Mutex` gives `StartEncounter`'s "atomic
  member-set snapshot" guarantee (lobby-surface.md "Start/leave atomicity"): a racing
  `LeaveLobby` lands either before the snapshot or after (`ErrLobbyAlreadyStarted`).
  Sufficient because rpg-api is single-process — no Redis WATCH/MULTI needed for a
  guarantee that only has to hold within one process. Known tradeoff: per-key mutex
  entries are never evicted (a slow, usage-bounded leak — one entry per lobby ID ever
  minted, not a hot loop).
- **`broker.go`** — a lobby-scoped pub/sub for `StreamLobby`, deliberately simpler than
  the toolkit's `tkenc.Broker`: no per-viewer projection exists for a lobby roster (no
  line-of-sight concept), so every subscriber for a lobby ID gets an identical event
  stream.
- **`character.go`** — `resolveCharacter` (ownership validation + name enrichment for
  `CreateLobby`/`JoinLobby`) and `seedMemberCombatSnapshot` (generalizes the old
  handler-layer `seedPlayerHP`, rpg-api#612, from one caller to N ready members at
  `StartEncounter`; renamed + extended by rpg-api#634 to also seed AC honestly from the
  stored character — see the `memberCombatSnapshot` doc comment for why AttackBonus/
  DamageDice/DamageType are deliberately NOT seeded here).
- **`start_encounter.go`** — the lobby -> encounter seam. Builds a fresh
  `tkenc.Encounter` on the SAME broker/repo the v2 encounter service reads from, adds
  one player per ready member (HP + AC seeded, positions spread along a line — a
  placeholder; no spawn-point system exists yet), persists ONCE, flips the lobby to
  STARTED, and only then publishes `EncounterStarted`. **Persist-then-emit ordering is
  load-bearing**: a client reacting to the event must find the encounter already in the
  encounter repo — enforced by construction (the `Publish` call is textually after both
  `Save` calls), and proven by
  `TestStartEncounter_PublishesEncounterStarted_AfterPersist`. A lobby-created member's
  combat-readiness for TakeAction comes from hydration (the v2 encounter orchestrator's
  `characterData.Attach` cascade, keyed off the `EntityID` set here), not from a flat
  AttackBonus/DamageDice snapshot — see status.md's rpg-api#634 entry.
- **`abandon_encounter.go`** (rpg-api#663) — mirrors `start_encounter.go`'s shape in
  reverse: load lobby (host check), resolve `EncounterID`, `LoadFromData` on the SAME
  broker/resolvers, `enc.End(tkenc.EncounterEndedReasonAbandoned)`, persist. Unlike
  every other mutating RPC in this package, it never writes the lobby record itself —
  `CreateLobby` already unconditionally refreshes the caller's player->lobby index for
  the NEW lobby's own member set on every call, so a stale index entry pointing at an
  abandoned lobby is simply overwritten on the next `CreateLobby`, no explicit cleanup
  needed. No per-lobby or per-encounter lock: it never mutates the lobby (no lock
  needed there), and no per-encounter lock exists anywhere in this codebase today for
  the load-mutate-save cycle (the v2 encounter orchestrator's own combat verbs share
  the same unguarded exposure against each other) — not a new gap introduced here.

### Repository (`internal/repositories/lobby/`)

`Data`/`Member` are the canonical entity types (also used directly by the
orchestrator's `Event` payloads — one representation, no conversion layer). Redis
implementation uses `lobby:<id>` as the primary key and `lobby:joinref:<ref>` as a
secondary index (`JoinLobby` is the only RPC that addresses a lobby by ref instead of
ID), both refreshed with the same TTL on every `Save`, mirroring
`internal/repositories/encounters/v2/redis.go`. In-memory variant for tests, same
JSON-round-trip-on-every-call contract.

A third index, `player:<playerID>:lobby` (rpg-api#653), backs `GetByPlayerID` —
`GetMyActiveLobby`'s resume-after-refresh lookup. `Save` writes/refreshes this entry,
same TTL, same transaction, for every player currently in `data.Members` — one
active lobby per player, last write wins, no dual-membership tracking (nothing stops
a player being a member of two lobbies at once; the index just points at whichever
was `Save`d most recently). Because `Save` can only add or refresh entries from a
`Data` snapshot — it has no way to see who was removed — `LeaveLobby` calls the
repository's `ClearPlayerIndex` explicitly for the departing player; no other RPC
needs to.

## Contract edge cases (decided in the design, implemented here)

| Policy | Where enforced |
|---|---|
| `JoinLobby` idempotent, rebinds `character_id` | `join_lobby.go`'s `isRebind` branch |
| Late join on STARTED lobby → `FailedPrecondition` | `join_lobby.go` status check before character resolution |
| Host leaves → oldest remaining member becomes host | `leave_lobby.go`, keyed off `MemberOrder` |
| Disconnect ≠ `LeaveLobby` (presence only) | `SetConnected` (orchestrator) / `StreamLobby`'s subscribe+defer (handler) |
| Party cap (default 4) | `join_lobby.go`, `Config.PartyCap` |
| Abandoned `WAITING` lobbies expire, no reaper | Redis TTL on the repo, refreshed on every `Save` |
| `WAITING -> STARTED` terminal | Every mutating orchestrator method checks `Status == StatusStarted` first |
| `AbandonEncounter` host-only, `WAITING` lobby → `FailedPrecondition` | `abandon_encounter.go`'s `HostPlayerID` check + `ErrLobbyNotStarted` |
| Resumed encounter refuses further verbs after abandon | `GetMyActiveLobby`'s existing `Mode == ModeEnded` liveness check — no new code needed, see the RPC's own doc above |

## Known gaps

- **No `initial_mode` on `StartEncounter`** — the deleted `CreateEncounter` RPC could
  start an encounter directly in `TURN_BASED` mode; the new contract has no equivalent
  field, so every lobby-constructed encounter starts `FREE_ROAM`. No production caller
  depended on the old behavior (devseed writes straight to Redis); flagged as a design
  gap for a future RPC revision, not fixed here. `devseed --inject-combat`
  (`internal/pkg/devcombat`) fills this gap for local/MCP playtesting only — it loads an
  EXISTING lobby-started encounter, adds a monster, and flips TURN_BASED without a proto
  change (rpg-api#634).
- **Player spawn positions are still a line placeholder, now anchored at the dungeon
  entrance.** `StartEncounter` spreads members along a straight line (`Q += index`)
  starting from `SpaceData.Entrance` rather than a real spawn-point selection. **The
  dungeon is now an N-region chain selected by key** (rpg-api#688, generalizing
  rpg-api#676's two-chamber dungeon: `resolveDungeonSpec` maps a `DungeonKey` — the
  crypt key, or a named default — plus a seed to a `tkenc.DungeonParams`, and
  `enc.InitDungeon(...)` runs before the member loop. **rpg-api#694: for the crypt key,
  that `DungeonParams` is now `tkenc.CryptDungeonParams(seed, ...)`, called verbatim —
  rpg-api owns no region width/height/theme/obstacle literal of its own any more, only
  its two connector door IDs.** The crypt spec is 3 regions (entrance → corridor →
  boss) + 2 plain doors + a designated entrance cell + per-region archetype tags + an
  opaque `Theme` + physical set-piece `Obstacles` (obelisk/pillars/coffin/altar/
  statues, exact canonical `dnd5e:props:*` refs), all toolkit-generated by key —
  `chamberWidth`/`chamberHeight`/`chamberPattern`/`chamberDoorID`/`InitTwoChamberRoom`
  are gone from this file). **rpg-api#689 (2026-07-23) retired the 4-goblin
  composition**: exactly 1 deterministic-anchor skeleton in the entrance region, 0 in
  the corridor, exactly 1 deterministic-anchor non-wight skeleton-captain boss
  (rpg-toolkit#816) in the boss region — via `spawn.FixedPositions` only
  (`crypt_monster_seed.go`), no `PositionOracle` out-of-sight search anywhere in this
  path anymore. Concealment (when it happens) comes from real door/wall LoS geometry,
  not a placement predicate — see `docs/status.md`'s "Deterministic crypt monster
  composition" entry. Only the PLAYER placement within the entrance region is still the
  line placeholder; real player spawn-point selection is future work. Per-dungeon-key
  archetype tables (`monsterSeedSpecsByKey`) are additive for a future second named
  dungeon; CR budgeting, difficulty scaling, and monster pools remain out of scope
  (rpg-project#110's retro note records the deferral).
- **rpg-api#696 (out-of-sight goblin placement vs. the toolkit's smaller crypt
  dimensions) is CLOSED, not planned.** #696 was filed while implementing #694 (the
  toolkit dimensions alone raised the retired out-of-sight goblin search's failure
  rate to ~33% for a full party) — #689's deterministic `FixedPositions` composition,
  merged into this same branch, retires that search path ENTIRELY, structurally
  eliminating the "no valid position found" failure mode rather than tuning around it.
  See `docs/status.md`'s "Known rough edges" for the historical record.
- **`keyedMutex` never evicts** — see the orchestrator section above.
- **Discord-instance join_ref carrier is out of scope** — only the dev/playtest carrier
  (an opaque `join_ref` passed explicitly) ships here. The Discord Activity carrier
  (instance -> `join_ref` mapping via an interceptor) lands with Activity integration.

## Verify

`internal/integration/lobby_v1alpha1_test.go`'s
`TestPartyAssembles_FourPlayers_CreateJoinReadyStart` is the boarded "Party Assembles"
gate test: four dev-authenticated players create/join/ready/start and land in one `v2`
encounter, each with HP seeded from their bound character, verified both via direct
`EncRepoV2` inspection and via the pre-existing `StreamEncounter` snapshot path.
`TestJoinLobby_LateJoin_FailedPrecondition` covers the late-join edge case end-to-end.

`TestPartyAssembles_FourPlayers_ThenCombatEntry_AttackResolves` (same file, rpg-api#634)
builds on the same party-assembly flow and proves a lobby-created player can actually
fight: `devcombat.Inject` adds a goblin + flips TURN_BASED, and the active player's real
`TakeAction` RPC does not return `ErrNonCombatant` (fixed by rpg-toolkit encounter
v0.24.4, see status.md).

The integration harness (`internal/integration/harness/harness.go`) does spin up a real
Redis container (testcontainers) for the suite, but `LobbyRepo` and `EncRepoV2` are
wired to their **in-memory** variants there — mirroring how `BrokerV2`/`EncRepoV2`
were already in-memory in this harness before this PR. Only `CharacterRepo` (and the
other pre-existing Redis-backed repos: draft, dice session) exercise the real Redis
container in this test. The lobby repo's OWN Redis-backed persistence (the two-key
`lobby:`/`lobby:joinref:` write, TTL behavior) is covered separately by
`internal/repositories/lobby/redis_test.go` (via miniredis), not by this integration
suite.
