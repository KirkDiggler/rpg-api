---
name: lobby service
description: LobbyService v1alpha1 — party assembly (join refs, membership, ready flags, lifecycle) and the sole session-construction path
updated: 2026-08-23
confidence: medium — rewritten 2026-08-21 for the post-rip-out shape (rpg-api#801); dungeon_key routing + ListDungeons added 2026-08-23 (rpg-api#806), verified against the passing unit + integration suites; the lobby has no browser-verified walkthrough on the new stack yet
---

# lobby service

The lobby service is the party-assembly surface: N players join refs/ready-up/
host-migrate before a session exists at all. `StartEncounter` is the **only**
way a session comes into existence — it builds directly onto the toolkit's
`rulebooks/dnd5e/session` SDK (`sdk.Manager`), the sole encounter-construction
stack since rpg-project#227 removed the old `github.com/KirkDiggler/
rpg-toolkit/encounter` module and everything built on it (see
[`encounter.md`](./encounter.md), [`authoring.md`](./authoring.md)).
The world it starts in comes from the content registry (`internal/dungeons`,
rpg-api#806): `StartEncounterRequest.dungeon_key` picks a registered dungeon
(empty → `reference-tomb`, unknown → `NotFound`), and
`LobbyService.ListDungeons` answers from the same registry, ungated — see
[`authoring-service.md`](./authoring-service.md).

Design doc: `rpg-project/ideas/game-screen-rebuild/lobby-surface.md`. Umbrella:
KirkDiggler/rpg-project#81. Implementation issue: rpg-api#629.

## Boundary

The toolkit has **zero** lobby concept, and this component keeps it that way
(CLAUDE.md's Boundary Rule: "if it's data storage or API orchestration → rpg-api"). A
lobby's `Data`/`Member` types are rpg-api-owned entities (`internal/repositories/lobby`),
not a toolkit mirror. `StartEncounter`'s session construction (`sdk.Manager.StartSession`
+ `Join` per member + `Spawn` per monster, all against a world the registry
compiled through `internal/sessionworld`) is data movement — building the
toolkit's own session by reference, authoring no game rules.

## Layers

```
internal/handlers/dnd5e/lobby/v1alpha1/   proto <-> input translation, sentinel->status mapping
internal/orchestrators/lobby/             load -> mutate -> persist -> publish core
internal/repositories/lobby/              Redis (+ in-memory) persistence
```

Layered from day one — Chapter-1 discipline.
### Handler (`internal/handlers/dnd5e/lobby/v1alpha1/`)

One file per RPC (`create_lobby.go`, `join_lobby.go`, `set_ready.go`,
`leave_lobby.go`, `start_encounter.go`, `stream_lobby.go`, `get_my_active_lobby.go`,
`abandon_encounter.go`, `list_dungeons.go`), plus `handler.go` (Config/New only),
`translate.go` (entity <-> proto), and `status.go` (one shared `lobbyStatusError`
switch covering every sentinel — each RPC only ever returns a subset, so one
exhaustive mapper beats six near-duplicates). `ListDungeons` answers from
`Config.Dungeons` (`registry.List`) and is deliberately **not** behind the
authoring gate: it reads content and mutates nothing, and the picker
(rpg-project#131) needs it with authoring off.

`get_my_active_lobby.go` (rpg-api#653) is the resume-after-refresh lookup
(rpg-dnd5e-web#444): unlike every other RPC here, it takes no request fields —
identity comes entirely from `auth.GetPlayerID(ctx)`, matching `StreamLobby`'s
pattern (a client-supplied `player_id` would let a caller query another player's
lobby). "No active lobby" is a valid empty response, not an error — see the
Repository section below for the index it reads and the orchestrator's
`GetMyActiveLobby` doc comment for the STARTED-lobby liveness cross-check.

`abandon_encounter.go` (rpg-api#663) is the host-only escape hatch for a stuck or
unwanted STARTED encounter. Host-only (`ErrNotHost`, matching `StartEncounter`'s
check via `HostPlayerID`); `FailedPrecondition` (`ErrLobbyNotStarted`) if the lobby
is still `WAITING`; delegates to the orchestrator, which ends the session through
the SDK (below). Resume-refusal after an abandon needs no cooperating write:
`GetMyActiveLobby`'s liveness check zeroes its whole Output the moment the
session's `Status.Open` reads false.

The handler imports no toolkit runtime. Everything it needs from the session
stack arrives through the orchestrator's `sdk.Manager`.

### Orchestrator (`internal/orchestrators/lobby/`)

Never imports proto. Sentinel errors live in `errors.go`; the handler maps them to
gRPC codes, not this package. `Config` requires a `LobbyRepo`, `LobbyBroker`,
`CharacterRepo`, the two ID generators, and a `SessionManager` (`sdk.Manager`) — the
single handle onto the session stack. There is no encounter repository, no
combat/movement resolver builder, and no dungeon registry in this package any more.

- **`keyed_mutex.go`** — a per-lobby-ID `sync.Mutex` gives `StartEncounter`'s "atomic
  member-set snapshot" guarantee (lobby-surface.md "Start/leave atomicity"): a racing
  `LeaveLobby` lands either before the snapshot or after (`ErrLobbyAlreadyStarted`).
  Sufficient because rpg-api is single-process — no Redis WATCH/MULTI needed for a
  guarantee that only has to hold within one process. Known tradeoff: per-key mutex
  entries are never evicted (a slow, usage-bounded leak — one entry per lobby ID ever
  minted, not a hot loop).
- **`broker.go`** — a lobby-scoped pub/sub for `StreamLobby`. No per-viewer projection
  exists for a lobby roster (no line-of-sight concept), so every subscriber for a
  lobby ID gets an identical event stream.
- **`character.go`** — `resolveCharacter`: ownership validation + name enrichment for
  `CreateLobby`/`JoinLobby`. HP/AC are no longer seeded here — the session stack's
  `Join` loads the character through the host's character repository (session
  v0.3.0) and the composition derives combat state itself.
- **`start_encounter_session_stack.go`** — the lobby -> session seam, and the only
  way a session comes into existence. Under the per-lobby lock: snapshot the ready
  members; resolve `DungeonKey` against `Config.Dungeons` (empty →
  `dungeons.DefaultKey`, unknown → `ErrDungeonNotFound` → `NotFound`, refused
  before anything is written); `StartSession` on the entry's compiled world;
  `Join` one member per ready player at the dungeon's authored party seats;
  `Spawn` the authored monsters; flip the lobby to STARTED and `Save`; only then
  `Publish` `EncounterStarted`. **Persist-then-emit ordering is load-bearing**: a
  client reacting to the event must find the session already started. Every verb
  is a separate load-act-save through `sdk.Manager`; there is no rollback on
  partial failure — that contract is rpg-api#800's to rule on.
- **`abandon_encounter.go`** (rpg-api#663) — load lobby, host check, `ErrLobbyNotStarted`
  guard, then `sdk.Manager.End(Session: EncounterID, Ending: sessionworld.EndingWithdrawn)`
  — the one external ending the tomb declares. It never writes the lobby record:
  `CreateLobby` refreshes the caller's player->lobby index on every call, so a stale
  entry pointing at an abandoned lobby is overwritten on the next create. The
  `withdrawn` ending for an administrative abandon is a product choice recorded on
  rpg-api#801, not a settled rule.
- **`get_my_active_lobby.go`** — reads the player->lobby index, then for a STARTED
  lobby cross-checks liveness with `sdk.Manager.Status`: `ErrNoSession`/`ErrNoEncounter`
  or `!Status.Open` both mean "no active lobby" (empty Output, not an error).

### Repository (`internal/repositories/lobby/`)

`Data`/`Member` are the canonical entity types (also used directly by the
orchestrator's `Event` payloads — one representation, no conversion layer). Redis
implementation uses `lobby:<id>` as the primary key and `lobby:joinref:<ref>` as a
secondary index (`JoinLobby` is the only RPC that addresses a lobby by ref instead of
ID), both refreshed with the same TTL on every `Save`. In-memory variant for tests,
same JSON-round-trip-on-every-call contract.

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
| Resumed session refuses further verbs after abandon | `GetMyActiveLobby`'s liveness check (`sdk.Manager.Status`) — no new code needed, see the RPC's own doc above |
## Known gaps

- **No starting-in-combat path.** The old `--inject-combat` dev tooling (`devcombat`)
  is gone with the old stack. On the new stack a fight forms when a member sights a
  monster (session W4), so "start in TURN_BASED" is not a concept to port.
- **Arcade recovery is not ported.** The old `StartEncounter` restored a character
  for a new encounter and cleared a stale in-combat action economy before seating.
  The SDK's `Join` contract does not do this; whether it belongs in the toolkit's
  `Join` or in an explicit rpg-api call is open (recorded on rpg-api#801).
- **Partial-failure orphans** — rpg-api#800.
- **Authorization on the session verbs themselves** — rpg-api#803; the lobby's
  host check on `AbandonEncounter` is not mirrored by `SessionService.End`.

## Verify

- `go test ./internal/orchestrators/lobby/ ./internal/handlers/dnd5e/lobby/...` —
  fixtures run on a miniredis-backed `session.Manager`, so `StartEncounter` really
  starts a session and `AbandonEncounter` really ends one.
- `go test ./internal/integration/...` — the harness wires the lobby orchestrator
  onto the same `sdk.Manager` the server uses (`harness.SessionOrch`).
- Live: four dev-authenticated players create/join/ready/start, then
  `SessionService.GetAtlas`/`GetStatus` on the returned `encounter_id` show the
  tomb with the party seated at its authored start. No browser walkthrough of the
  lobby on the new stack has been recorded yet — the game route still targets the
  old wire until the web cuts over.
