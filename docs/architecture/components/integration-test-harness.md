---
name: integration test harness
description: Full-stack test server wiring real Redis, in-process gRPC, and testcontainers
updated: 2026-08-21
confidence: medium — verified by reading the current harness.go after rpg-project#227's rip-out; container-ownership/lifecycle mechanics (rpg-api#699) unaffected by that rip-out and still accurate
---

# integration test harness

The integration test harness (`internal/integration/harness/`) wires the full
stack — real Redis via testcontainers, real orchestrators and repositories,
in-process gRPC via bufconn — and exercises complete game flows via
proto-level client calls.

**Updated 2026-08-21 (rpg-project#227):** `TestServer` no longer exposes
`EncounterClientV2`, `BrokerV2`, or `EncRepoV2` — the old v1alpha2 encounter
service they wired (`internal/handlers/dnd5e/v2/encounter`,
`internal/repositories/encounters/v2`) is deleted, see
[`encounter.md`](./encounter.md). `TestServer.SessionOrch` (a
miniredis-backed `*sessionorch.Orchestrator`) is wired into the lobby
orchestrator's `Config.SessionManager` instead, mirroring
`cmd/server/server.go`'s production wiring. The `internal/integration/`
top-level suites this doc used to describe below
(`dungeon_crypt_test.go`, `encounter_v2_test.go`,
`lobby_crypt_monster_seed_test.go`, `lobby_start_then_move_test.go`,
`lobby_v1alpha1_test.go`, `shared_fixture_regression_test.go`, and
`main_test.go`) are deleted along with the fields they asserted through —
new-stack integration coverage lives in `internal/integration/session`
instead; `internal/integration/character` is untouched.

## Files

| File | Purpose |
|---|---|
| `integration/harness/harness.go` | TestServer wiring (`New`, `NewWithRedis`, `Close`) |
| `integration/harness/redis_container.go` | `RedisContainer` shared-fixture: `StartRedis`, `Terminate`, `Lease` (rpg-api#699) |
| `integration/harness/harness_test.go` | Smoke test: server starts, clients connect, Redis flush works |
| `integration/character/` | Character integration tests (own `TestMain`, own shared container) |
| `integration/session/` | New-stack (`rulebooks/dnd5e/session` SDK) integration tests |

## Architecture

```
RedisContainer (redis_container.go)         ← package/process-level fixture
    └── testcontainers Redis                 ← one real Redis process in Docker,
                                                started once by a package's TestMain

TestServer (harness.go), one fresh instance per test
    ├── Redis client (New: owns a container it started itself;
    │                 NewWithRedis: borrows a RedisContainer's Addr,
    │                 owns only its own client connection)
    ├── bufconn               ← in-process gRPC (no network)
    ├── real orchestrators    ← same code as production (character, lobby)
    ├── real repositories     ← character (Redis), lobby (in-memory)
    └── SessionOrch           ← miniredis-backed session.Manager, the sole
                                 encounter-construction stack (rpg-project#227)
```

`TestServer.CharacterClient`, `.DiceClient`, `.LobbyClient` — proto-generated
gRPC clients that connect to the in-process server. Tests call these clients
exactly as the web client would. There is no `EncounterClientV2` any more —
the old v1alpha2 encounter service it connected to is deleted, and no
SessionService client is exposed here yet either (`TestServer.SessionOrch`
gives tests direct access to the `session.Manager` and `Broker` instead of a
proto client).

## TestServer lifecycle

Standalone (owns its own container):

```go
ctx := context.Background()
server, err := harness.New(ctx, harness.DefaultConfig())
defer server.Close()

resp, err := server.LobbyClient.CreateLobby(ctx, &lobbyv1alpha1.CreateLobbyRequest{...})
```

`Close()` terminates the gRPC server and, because this `TestServer` owns
one, its Redis container.

Shared-fixture (the pattern `internal/integration/character` uses — see that
package's `TestMain`):

```go
// TestMain, once per process:
sharedRedis, _ = harness.StartRedis(ctx)
defer sharedRedis.Terminate(ctx) // or an explicit call after m.Run()

// SetupTest, once per test:
release := sharedRedis.Lease()
srv, _ := harness.NewWithRedis(ctx, cfg, sharedRedis.Addr)
srv.FlushRedis(ctx)

// TearDownTest, once per test:
srv.Close()   // closes gRPC server, this test's own Redis client —
              // never terminates sharedRedis's container
release()
```

## Known gaps

### ~~testcontainers startup time~~ FIXED (rpg-api#699, 2026-07-23)

Splitting container ownership (`RedisContainer`) from per-test service wiring
(`NewWithRedis`) cut a full-package integration run from ~283s (33 container
lifecycles) to containers started/terminated once per Go test process — see
`docs/how-to/run-integration-tests.md` for the measured before/after.

### No proto-level LobbyService round-trip coverage since rpg-project#227

The six `internal/integration/*_test.go` suites that drove `LobbyService`
through this harness's real bufconn gRPC connection are deleted along with
the old v1alpha2 encounter service they asserted results through (see the
2026-08-21 note above). `internal/orchestrators/lobby` and
`internal/handlers/dnd5e/lobby/v1alpha1` still have their own suite-level
test coverage against the session SDK (see
[`lobby-service.md`](./lobby-service.md)'s "Verify" section) — what's missing
is specifically the full-stack, real-gRPC-round-trip layer this harness
exists to provide.

## Value

The harness is what proves the full system works end-to-end — proto
serialization round-trips, handler <-> orchestrator wiring, and real Redis
operations that unit tests with mocks can't verify in isolation.

**If a change breaks integration tests, it breaks the game.**
