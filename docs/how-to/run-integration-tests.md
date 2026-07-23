---
name: run integration tests
description: How to run the integration test harness — requires Docker for testcontainers
updated: 2026-07-23
---

# How to run integration tests

Integration tests live in `internal/integration/`. They use `testcontainers-go` to spin up a real Redis instance, so Docker must be running.

## Prerequisites

- Docker running (`docker ps`)
- Go 1.24.1+

## Only one Docker-backed integration runner at a time

**Rule (rpg-api#699):** if more than one agent/worktree/session is working
against this repo's Docker daemon, only ONE of them should be exercising
`internal/integration/...` at a time. Two runners racing against the same
Docker daemon compete for port allocation and disk I/O and will confound
each other's timing evidence — the same daemon-contention effect that
originally inflated container churn in #699's baseline measurement. Use
targeted `-run <TestName>` while iterating (see below); reserve a full
`go test ./internal/integration/...` run for when you actually need the
whole-package signal, and don't run it back-to-back with another agent's
integration run.

## Run all tests (unit + integration)

```bash
cd rpg-api
go test -v -race ./...
```

This runs everything. Integration tests are not build-tagged, so they run with the full test suite.

## Run integration tests only

```bash
# v1alpha2 encounter service tests
go test -v -race ./internal/integration/ -run TestEncounterV2

# LobbyService tests (incl. combat-entry flows)
go test -v -race ./internal/integration/ -run TestLobby

# Character integration tests
go test -v -race ./internal/integration/character/...
```

**Updated 2026-07-13 (rpg-api#642):** the v1-only `internal/integration/encounter/`
suite (which this section used to point at, including `TestOpenDoor` and
`TestMonkKillsMonster`) is deleted along with the v1alpha1 EncounterService it
exercised. Encounter coverage now lives in `internal/integration/
encounter_v2_test.go` and `internal/integration/lobby_v1alpha1_test.go`.

## Redis container lifecycle (rpg-api#699)

Each of the three Go test binaries under `internal/integration/...` is its
own OS process, and each owns exactly **one** shared Redis testcontainer for
its process's whole run — not one container per test method:

| Package (process) | Suites | Container ownership |
|---|---|---|
| `internal/integration` | `DungeonCryptSuite`, `EncounterV2IntegrationSuite`, `CryptMonsterSeedRedisSuite`, `LobbyStartThenMoveSuite`, `LobbyV1alpha1IntegrationSuite` (28 test methods) | One shared container, started/terminated once in `main_test.go`'s `TestMain` |
| `internal/integration/character` | `CharacterCreationSuite` (7 test methods) | One shared container, started/terminated once in its own `main_test.go`'s `TestMain` |
| `internal/integration/harness` | `TestHarness_StartsAndConnects` (1 test method, standalone `harness.New` smoke test) | Its own single container — already minimal since it's one test |

Each package's `TestMain` calls `harness.StartRedis(ctx)` once before
`m.Run()` and `(*harness.RedisContainer).Terminate(ctx)` once after —
**do not** add a second `harness.New`/`StartRedis` call inside a suite in
`internal/integration` or `internal/integration/character`; every suite's
`SetupTest` should call `harness.NewWithRedis(ctx, cfg, sharedRedis.Addr)`
against the package's `sharedRedis` var instead. `sharedRedis.Lease()` /
the returned `release()` in `SetupTest`/`TearDownTest` is a
belt-and-suspenders guard, not a requirement to add `t.Parallel()` —
these suites are still expected to run serially.

Because these are three separate processes, `go test ./internal/integration/...`
starts and terminates **3** Redis containers total (one per process), not
1 — a single container object cannot be shared across OS process
boundaries.

There are 36 test methods total across the three packages (28 in
`internal/integration`, 7 in `internal/integration/character`, 1 in
`internal/integration/harness`), but the pre-#699 container count was
**not** "one per test method" uniformly: `internal/integration`'s 28 test
methods each started/terminated their own container (28 containers), but
`internal/integration/character`'s `CharacterCreationSuite` already used
`SetupSuite` (one `TestServer` shared across its 7 tests, 1 container) and
`internal/integration/harness` has always had exactly 1 test (1
container). The actual pre-#699 total was **30** containers, not 36 — down
to the 3 above after this change.

## What the harness wires

`internal/integration/harness/harness.go` builds a `TestServer` with:
- Real Redis client (via testcontainers, owned or borrowed — see below)
- In-process gRPC server via `bufconn` (no network overhead)
- Real orchestrators (character, lobby), real repositories
- The v2 encounter path's `tkenc.Broker` for live event fan-out
- `DevMode: true` — uses `Dev <player_id>` auth

Two constructors, split by **container ownership** (rpg-api#699):
- `harness.New(ctx, cfg)` — starts its own dedicated Redis container and
  owns it; `Close()` terminates that container. Use this for a standalone
  test/script that isn't part of a shared-fixture package (e.g.
  `internal/integration/harness/harness_test.go`'s own smoke test).
- `harness.NewWithRedis(ctx, cfg, redisAddr)` — wires the exact same
  gRPC server/repos/brokers/clients against an already-running Redis
  address instead of starting a container; `Close()` closes this
  TestServer's own Redis client connection but never terminates the
  container backing `redisAddr` — that's the caller's (a package
  `TestMain`'s) responsibility via `harness.StartRedis`/`RedisContainer.
  Terminate`. This is what every suite in `internal/integration` and
  `internal/integration/character` uses.

Exported on `TestServer` (updated 2026-07-13, rpg-api#642 — `EncounterClient`
and `EncounterPublisher` are gone with the v1alpha1 stack):
- `CharacterClient dnd5ev1alpha1.CharacterServiceClient`
- `EncounterClientV2 encounterv2pb.EncounterServiceClient`
- `LobbyClient lobbyv1alpha1pb.LobbyServiceClient`
- `DiceClient apiv1alpha1.DiceServiceClient`
- `CharacterRepo characterrepo.Repository` — for seeding test data
- `BrokerV2` / `EncRepoV2` — for seeding/inspecting v2 encounter state directly

## Writing new integration tests

Adding a test method to an existing suite in `internal/integration` or
`internal/integration/character`? Nothing to do beyond writing the test —
`SetupTest` already leases the package's shared container and gives you a
fresh `TestServer`.

Adding a brand-new suite to one of those packages? Follow the existing
pattern (e.g. `internal/integration/dungeon_crypt_test.go`'s
`SetupTest`/`TearDownTest`): lease `sharedRedis`, call
`harness.NewWithRedis(ctx, cfg, sharedRedis.Addr)`, `FlushRedis`, and
release the lease in `TearDownTest`. Do not call `harness.New` or
`harness.StartRedis` from inside a suite in these packages — that would
reintroduce one container per test method.

A standalone test outside those packages (or a throwaway script) can still
use the simple owning constructor:

```go
func TestMyScenario(t *testing.T) {
    ctx := context.Background()
    server, err := harness.New(ctx, harness.DefaultConfig())
    require.NoError(t, err)
    defer server.Close()

    // Seed a character
    charResp, err := server.CharacterClient.GetCharacter(ctx, &proto.GetCharacterRequest{...})

    // Drive the v2 encounter
    resp, err := server.EncounterClientV2.CreateEncounter(ctx, ...)
    require.NoError(t, err)
}
```

See `internal/integration/lobby_v1alpha1_test.go` for the current pattern of
seeding a party and driving a fight end-to-end via `LobbyClient` +
`EncounterClientV2`.

## Test timing

Before rpg-api#699, container create/health-check/teardown churn (one
Redis container per test method) dominated wall time: a full
`go test ./internal/integration/...` run measured ~283s across 33
container lifecycles (accepted baseline, #699's issue description). After
#699 (one shared container per process instead of per test method), a
`go test -v -race ./internal/integration/...` run measured **37.4s wall
clock** (`/usr/bin/time -v`, the single post-fix run) and started/
terminated exactly **3** Redis containers total — one per Go test
process (`internal/integration`, `internal/integration/character`,
`internal/integration/harness`), confirmed by each process's `TestMain`
STARTED/TERMINATED log lines and by counting `🐳 Creating container for
image redis:7-alpine` occurrences in the run's output. See
`docs/status.md` for the PR's full before/after writeup.

## Known gaps

~~Round 2 open-door and stream entity state tests...~~ MOOT (rpg-api#642,
2026-07-13): these tests belonged to the deleted v1-only
`internal/integration/encounter/` suite. See `docs/status.md` "Paused / on
hold" for the moot #459–#468 PR record.
