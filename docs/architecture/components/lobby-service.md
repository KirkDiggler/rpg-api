---
name: lobby service
description: LobbyService v1alpha1 — party assembly (join refs, membership, ready flags, lifecycle) and the sole session-construction path
updated: 2026-09-24
confidence: medium-high — the consumed session SDK surface is the six-method `SessionManager` interface with a generated mock, so the lobby's own unit suites script SDK outcomes without a playable session (rpg-api#1046); first-admission recovery ownership still verified through the retained real miniredis-backed `SessionStackSuite` Lobby StartEncounter and failure-order acceptance (rpg-api#882); dungeon_key routing + ListDungeons verified against unit + integration suites; rpg-api#1003 single-room launch verified through the real registry/SDK/miniredis suite (scene, cells, snapshot isolation, capacity refusal); the lobby has no browser-verified walkthrough on the new stack yet
---

# lobby service

The lobby service is the party-assembly surface: N players join refs/ready-up/
host-migrate before a session exists at all. `StartEncounter` is the **only**
way a session comes into existence — it builds directly onto the toolkit's
`rulebooks/dnd5e/session` SDK (`sdk.Manager`) through the six-method
`SessionManager` interface (below), the sole encounter-construction
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

Single-room authored content (rpg-api#1003) launches through this same path:
a v3 room Put into the registry starts like any dungeon, and every actor
stands on the authored axial cell — negative/odd rows included — through the
one conversion `sessionworld` performs. The launch writes the RESOLVED
dungeon key (after the default fallback) onto the session record, and it
reaches members as `GetAtlasResponse.dungeon_key`, which is how a client
fetches the room's appearance from the entry the world was compiled from
(rpg-project#479). Each run persists its compiled FIELD, so a room
republished after a launch reaches only future launches; the picture under
that key is mutable and a live session sees the new one, which design R1
rules acceptable pre-v1. The existing seat-capacity refusal (party bigger than the
derived seats) still fires before any session, world, character write or
`EncounterStarted` event. Browser proof of the authored room remains
pending.

## Boundary

The toolkit has **zero** lobby concept, and this component keeps it that way
(CLAUDE.md's Boundary Rule: "if it's data storage or API orchestration → rpg-api"). A
lobby's `Data`/`Member` types are rpg-api-owned entities (`internal/repositories/lobby`),
not a toolkit mirror. `StartEncounter`'s session construction
(`SessionManager.StartSession`, then `Spawn` per monster, then `Join` per ready
member, then the reference tomb's demo `PlaceNPC`, all against a world the
registry compiled through `internal/sessionworld`) is data movement — building the
toolkit's own session by reference, authoring no game rules. Session `Join`
owns first-admission recovery from its persisted `EverMembers` record and
persists the provider-authored result through the character repository adapter;
the lobby neither loads runtime D&D characters nor creates event buses.

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
stack arrives through the orchestrator, whose six-method `SessionManager`
interface (below) is the single handle onto it.

### Orchestrator (`internal/orchestrators/lobby/`)

Never imports proto. Sentinel errors live in `errors.go`; the handler maps them to
gRPC codes, not this package. `Config` requires a `LobbyRepo`, a `LobbyBroker`, a
`CharacterRepo`, the content registry (`Dungeons dungeons.Registry`), three ID
generators (`LobbyIDGenerator`, `JoinRefGenerator` and `EncounterIDGenerator`),
and a `SessionManager` — the single handle onto the session stack. There is no
encounter repository and no combat/movement resolver builder in this package.

- **`session_manager.go`** — the six-method `SessionManager` interface
  (`StartSession`, `Spawn`, `Join`, `PlaceNPC`, `Status`, `End`) declared at the
  point of use instead of depending on the concrete, unmockable `*sdk.Manager`.
  Its `//go:generate` directive produces `mock/mock_session_manager.go`
  (`lobbymock.MockSessionManager`) through the repository's gomock convention;
  the real `*sdk.Manager` satisfies the interface structurally (`var _
  SessionManager = (*sdk.Manager)(nil)`), so production wiring keeps passing the
  concrete manager — no adapter, no second manager, no provider-pin change. See
  "Test boundary" below for why this exists.
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
  before anything is written); then, in this exact order, `StartSession` on the
  entry's compiled world; `Spawn` every authored monster in authored order;
  `Join` every ready member in lobby `MemberOrder` at the authored party seats;
  on the default key only, `PlaceNPC` the demo vendor after the joins; flip the
  lobby to STARTED and `Save`; only then `Publish` `EncounterStarted`. **Every
  Spawn precedes every Join, and persist-then-emit ordering is load-bearing**: a
  client reacting to the event must find the session already started. Every verb
  is a separate load-act-save through `SessionManager`. StartSession failure
  precedes every Spawn and Join and therefore every character write. During
  Join, first-admission recovery is saved before projection and placement; a
  later failure intentionally
  leaves that valid between-runs transition durable and the SDK error carries its
  `SaveReport`. The lobby adds no all-member preflight, rollback, or rule branch.
- **`abandon_encounter.go`** (rpg-api#663) — load lobby, host check, `ErrLobbyNotStarted`
  guard, then `SessionManager.End(Session: EncounterID, Ending: sessionworld.EndingWithdrawn)`
  — the one external ending the tomb declares. It never writes the lobby record:
  `CreateLobby` refreshes the caller's player->lobby index on every call, so a stale
  entry pointing at an abandoned lobby is overwritten on the next create. The
  `withdrawn` ending for an administrative abandon is a product choice recorded on
  rpg-api#801, not a settled rule.
- **`get_my_active_lobby.go`** — reads the player->lobby index, then for a STARTED
  lobby cross-checks liveness with `SessionManager.Status`: `ErrNoSession`/`ErrNoEncounter`
  or `!Status.Open` both mean "no active lobby" (empty Output, not an error).

### Test boundary (rpg-api#1046)

The consumed SDK surface is an interface for one reason: so the lobby's own unit
suites can prove API behavior (storage, conversion, correct toolkit invocation)
rather than toolkit rules (rpg-project's test-ownership direction). Two kinds of
lobby test now coexist and must not be conflated:

- **Isolated** — `LobbySuite` and `StartContractSuite` (both `package lobby_test`)
  supply `lobbymock.MockSessionManager`, `dungeonsmock.MockRegistry`, the
  in-memory lobby repository and the broker. No miniredis, no shipped YAML, no
  session orchestrator, no playable character and no dice. The SDK mock is
  controller-isolated with no permissive `AnyTimes` defaults: an unauthorized or
  premature SDK call fails the test, and the launch-contract suite asserts the
  exact inputs and the required order — `StartSession` -> every `Spawn` -> every
  `Join` -> default-key `PlaceNPC` -> `Save` -> `Publish`.
- **Real-stack, retained pending #1049** — `SessionStackSuite` keeps its real
  miniredis-backed `session.Manager` and the real shipped content registry and
  stays active for the coverage this pilot did not map to an API-owned assertion:
  trade/sell/unpack, long-rest outcomes, reload, geometry and broader authored
  content. Those cases are retained, not ratified as a permanent API test suite;
  their assertion-level ownership mapping is rpg-api#1049. The four replaced gate
  cases (`NotHost`, `NotAllReady`, `LobbyNotFound`, `UnknownDungeonKeyIsRefused`)
  were removed only after each map to a `StartContractSuite` gate case; the
  disposition record is kept in [`docs/status.md`](../../status.md).

Pure converters (`arrival_test.go` and the social-approach spelling tests) are
unchanged and stay the API-owned spelling/presence proof.

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
| Resumed session refuses further verbs after abandon | `GetMyActiveLobby`'s liveness check (`SessionManager.Status`) — no new code needed, see the RPC's own doc above |
## Known gaps

- **No starting-in-combat path.** The old `--inject-combat` dev tooling (`devcombat`)
  is gone with the old stack. On the new stack a fight forms when a member sights a
  monster (session W4), so "start in TURN_BASED" is not a concept to port.
- **~~Arcade recovery is not ported~~ — closed by rpg-api#882.** The API-owned
  reset loop from rpg-api#828 is retired. Session v0.45.0 `Join` now owns a
  character's first-admission normal rest, persists it through the host adapter
  before projection/placement, and uses persisted `EverMembers` to prevent a
  reconnect or exit/rejoin from resting again. The stale action-economy sibling
  is narrower too: since session v0.24.1 the SDK clears a member's economy when
  their fight dissolves (rpg-toolkit#1222); the remaining leak is a session ended
  mid-fight via `End`/`Exit` (rpg-toolkit#1223).
- **Partial-failure orphans** — rpg-api#800. StartSession can still leave its
  documented encounter orphan if its later session save fails; later member
  Join/Spawn failures can still leave a partial session. A first-admission
  character save is separately intentional durable progress and is named in
  the Session SDK's `SaveError`/`SaveReport`, not rolled back by the lobby.
- **Authorization on the session verbs themselves** — rpg-api#803; the lobby's
  host check on `AbandonEncounter` is not mirrored by `SessionService.End`.
- **Authored monster facing reaches the carrier but not the session** — the
  compiled placement carries the authored `startingCell.facing` word
  (`sessionworld.Monster.Facing`, `internal/sessionworld/sessionworld.go:127`,
  filled by the compiler in the same file, rpg-toolkit#1899), but
  `StartEncounter`'s Spawn construction
  (`internal/orchestrators/lobby/start_encounter_session_stack.go`) never sets a
  facing, and it cannot: the consumed SDK's `SpawnInput`
  (`rulebooks/dnd5e/session` v0.109.0, `write.go`) has no `Facing` field. So a
  launched monster's authored direction is dropped at this seam. This is the
  separately-recorded provider/consumer gap the rpg-api#1046 design called for
  ("carried in sessionworld but is not currently forwarded by this launch",
  `docs/superpowers/specs/2026-09-24-lobby-sdk-test-boundary-design.md`):
  closing it needs a toolkit placement field, not a behavior change in this
  refactor.

## Verify

Two kinds of lobby test run here (see "Test boundary" above); do not conflate
them.

**Isolated orchestrator unit suites** (rpg-api#1046) — no miniredis, no shipped
YAML, no playable character; the SDK mock, the registry mock, the in-memory
lobby repository and the broker:

```sh
GOWORK=off go test ./internal/orchestrators/lobby -run 'TestLobbySuite|TestStartContractSuite' -count=1
```

`LobbySuite` (`lobby_test.go`) and `StartContractSuite`
(`start_encounter_contract_test.go`, `start_encounter_failure_test.go`) script
SDK outcomes and assert exact inputs, call ordering, refusal gates, and the one
lobby write without constructing a session.

**Retained real-stack orchestrator suite** — miniredis-backed `session.Manager`
plus the real shipped content registry:

```sh
GOWORK=off go test ./internal/orchestrators/lobby -run TestSessionStackSuite -count=1
```

`StartEncounter` really starts a session, and the suite carries the trade/sell/
unpack, rest-outcome, reload, geometry and broader authored-content coverage.
Retained active pending assertion-level ownership mapping (#1049); retention is
not ratification as a permanent API test. Orchestrator `AbandonEncounter` now
scripts `End` on the mock; a real `End` still rides the lobby handler suite,
which keeps its miniredis-backed `session.Manager` (translation and envelope
validation are its job), so not every lobby/API test is isolated yet:

```sh
GOWORK=off go test ./internal/handlers/dnd5e/lobby/... -count=1
```

- `go test ./internal/integration/...` — the harness wires the lobby orchestrator
  onto the same `sdk.Manager` the server uses (`harness.SessionOrch`).
- Live: four dev-authenticated players create/join/ready/start, then
  `SessionService.GetAtlas`/`GetStatus` on the returned `encounter_id` show the
  tomb with the party seated at its authored start. No browser walkthrough of the
  lobby on the new stack has been recorded yet — the game route still targets the
  old wire until the web cuts over.
