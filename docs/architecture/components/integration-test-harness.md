---
name: integration test harness
description: Full-stack test server wiring real Redis, in-process gRPC, and testcontainers
updated: 2026-08-28
confidence: medium-high — #852 registers SessionService and SessionPresentationService in the harness, exposes their clients, and proves two servers over one Redis; container-ownership/lifecycle mechanics remain accurate
---

# integration test harness

The integration test harness (`internal/integration/harness/`) wires the full
stack — real Redis via testcontainers, real orchestrators and repositories,
in-process gRPC via bufconn — and exercises complete game flows via
proto-level client calls.

**Updated 2026-08-28 (rpg-api#852):** `TestServer` still does not expose the
deleted v1alpha2 encounter stack (`EncounterClientV2`, `BrokerV2`, `EncRepoV2`),
but it now registers and exposes the live session stack: `SessionClient`,
`SessionPresentationClient`, `HealthClient`, and `SessionOrch`. Lobby launch,
SessionService access, and SessionPresentationService access share the same real
Session Manager, mirroring `cmd/server/server.go`. `internal/integration/sessionpresentation`
starts two TestServers over one Redis container and proves server B can authorize a
seated member from Session SDK state created by server A's real lobby launch path.

## Files

| File | Purpose |
|---|---|
| `integration/harness/harness.go` | TestServer wiring (`New`, `NewWithRedis`, `Close`) |
| `integration/harness/redis_container.go` | `RedisContainer` shared-fixture: `StartRedis`, `Terminate`, `Lease` (rpg-api#699) |
| `integration/harness/harness_test.go` | Smoke test: server starts, clients connect, Redis flush works |
| `integration/character/` | Character integration tests (own `TestMain`, own shared container) |
| `integration/session/` | New-stack (`rulebooks/dnd5e/session` SDK) integration tests |
| `integration/sessionpresentation/` | Cross-instance SessionPresentationService proof over shared Redis |

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
    ├── real orchestrators    ← same code as production (character, lobby, session presentation)
    ├── real repositories     ← character (Redis), lobby (in-memory)
    ├── SessionOrch           ← Redis-backed session.Manager, the sole
    │                            encounter-construction stack (rpg-project#227)
    └── health server         ← exact service-name checks for registered APIs
```

`TestServer.CharacterClient`, `.DiceClient`, `.LobbyClient`, `.SessionClient`,
`.SessionPresentationClient`, and `.HealthClient` are proto-generated gRPC clients
that connect to the in-process server. Tests call these clients exactly as the web
client would. Session roster reads and access checks use the real Session Manager
wired into `TestServer`; there is no `EncounterClientV2`
any more — the old v1alpha2 encounter service it connected to is deleted.

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

### Narrow full-stack coverage remains

#852 restores one full-stack path through this harness: `LobbyService` launch via
server A, `SessionPresentationService` streams on servers A and B, and
`SessionService.GetStory` before/after publish. Broader lobby/session full-stack
coverage is still narrower than the old deleted suite set; most gameplay acceptance
coverage remains in `internal/integration/session`, which uses its own local handler
harness rather than two bufconn servers.

## Value

The harness is what proves the full system works end-to-end — proto
serialization round-trips, handler <-> orchestrator wiring, and real Redis
operations that unit tests with mocks can't verify in isolation.

**If a change breaks integration tests, it breaks the game.**
