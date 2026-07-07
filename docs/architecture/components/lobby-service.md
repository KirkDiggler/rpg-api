---
name: lobby service
description: LobbyService v1alpha1 — party assembly (join refs, membership, ready flags, lifecycle) and the sole encounter-construction path
updated: 2026-07-07
confidence: high — verified by reading the implementation and running the full RPC-level + orchestrator-level + repository-level test suites plus a real-Redis 4-player integration test
---

# lobby service

The lobby service is the party-assembly surface the v2 encounter stack never had: a
lobby lets N players join refs/ready-up/host-migrate before a `v1alpha2` encounter
exists at all. `StartEncounter` is where a lobby hands off to the encounter stack — it
is now the **only** way an encounter comes into existence (it subsumes the deleted
`EncounterService.CreateEncounter` RPC).

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
`leave_lobby.go`, `start_encounter.go`, `stream_lobby.go`), plus `handler.go`
(Config/New only), `translate.go` (entity <-> proto), and `status.go` (one shared
`lobbyStatusError` switch covering every sentinel — each RPC only ever returns a
subset, so one exhaustive mapper beats six near-duplicates).

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
  `CreateLobby`/`JoinLobby`) and `seedMemberHP` (generalizes the old
  handler-layer `seedPlayerHP`, rpg-api#612, from one caller to N ready members at
  `StartEncounter`).
- **`start_encounter.go`** — the lobby -> encounter seam. Builds a fresh
  `tkenc.Encounter` on the SAME broker/repo the v2 encounter service reads from, adds
  one player per ready member (HP seeded, positions spread along a line — a
  placeholder; no spawn-point system exists yet), persists ONCE, flips the lobby to
  STARTED, and only then publishes `EncounterStarted`. **Persist-then-emit ordering is
  load-bearing**: a client reacting to the event must find the encounter already in the
  encounter repo — enforced by construction (the `Publish` call is textually after both
  `Save` calls), and proven by
  `TestStartEncounter_PublishesEncounterStarted_AfterPersist`.

### Repository (`internal/repositories/lobby/`)

`Data`/`Member` are the canonical entity types (also used directly by the
orchestrator's `Event` payloads — one representation, no conversion layer). Redis
implementation uses `lobby:<id>` as the primary key and `lobby:joinref:<ref>` as a
secondary index (`JoinLobby` is the only RPC that addresses a lobby by ref instead of
ID), both refreshed with the same TTL on every `Save`, mirroring
`internal/repositories/encounters/v2/redis.go`. In-memory variant for tests, same
JSON-round-trip-on-every-call contract.

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

## Known gaps

- **No `initial_mode` on `StartEncounter`** — the deleted `CreateEncounter` RPC could
  start an encounter directly in `TURN_BASED` mode; the new contract has no equivalent
  field, so every lobby-constructed encounter starts `FREE_ROAM`. No production caller
  depended on the old behavior (devseed writes straight to Redis); flagged as a design
  gap for a future RPC revision, not fixed here.
- **Spawn positions are a placeholder** — `StartEncounter` spreads members along a
  straight line (`Q += index`) since no room/spawn-point system exists yet for a
  freshly-created lobby encounter. Real spawn selection is future work once room
  integration lands.
- **`keyedMutex` never evicts** — see the orchestrator section above.
- **Discord-instance join_ref carrier is out of scope** — only the dev/playtest carrier
  (an opaque `join_ref` passed explicitly) ships here. The Discord Activity carrier
  (instance -> `join_ref` mapping via an interceptor) lands with Activity integration.

## Verify

`internal/integration/lobby_v1alpha1_test.go`'s
`TestPartyAssembles_FourPlayers_CreateJoinReadyStart` is the boarded "Party Assembles"
gate test: four dev-authenticated players create/join/ready/start against a real Redis
container (testcontainers) and land in one `v2` encounter, each with HP seeded from
their bound character, verified both via direct `EncRepoV2` inspection and via the
pre-existing `StreamEncounter` snapshot path. `TestJoinLobby_LateJoin_FailedPrecondition`
covers the late-join edge case end-to-end.
