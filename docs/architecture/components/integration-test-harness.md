---
name: integration test harness
description: Full-stack test server wiring real Redis, in-process gRPC, and testcontainers
updated: 2026-08-21
confidence: medium — the container-ownership/lifecycle sections below are still accurate as of rpg-project#227's rip-out; the per-service wiring details (EncounterClientV2, BrokerV2, EncRepoV2, the deleted encounter_v2_test.go/lobby_v1alpha1_test.go references) are NOT — see the rpg-project#227 note inline rather than a full rewrite
---

# integration test harness

**Partially stale (rpg-project#227, 2026-08-21):** `TestServer` no longer
exposes `EncounterClientV2`, `BrokerV2`, or `EncRepoV2` — the old v1alpha2
encounter service they wired (see [`encounter.md`](./encounter.md)) is
deleted. `TestServer.SessionOrch` (a miniredis-backed session orchestrator,
mirroring `cmd/server/server.go`'s production wiring) is wired into the lobby
orchestrator's `Config.SessionManager` instead. The six top-level
`internal/integration/*_test.go` suites this doc references below
(`dungeon_crypt_test.go`, `encounter_v2_test.go`,
`lobby_crypt_monster_seed_test.go`, `lobby_start_then_move_test.go`,
`lobby_v1alpha1_test.go`, `shared_fixture_regression_test.go`, plus
`main_test.go`) are deleted along with the fields they asserted through — new-
stack integration coverage lives in `internal/integration/session` instead.
The container-ownership/lifecycle mechanics below (`RedisContainer`,
`harness.New`/`NewWithRedis`, the shared-fixture pattern) are unaffected and
still accurate.

The integration test harness (`internal/integration/harness/`) is the most valuable test asset in rpg-api. It wires the full stack — real Redis via testcontainers, real orchestrators and repositories, in-process gRPC via bufconn — and exercises complete game flows via proto-level client calls.

**Updated 2026-07-23 (rpg-api#699):** container **ownership** is now split
from per-test **service wiring**. `harness.New` still starts and owns its
own dedicated Redis container (unchanged behavior for standalone callers).
`harness.NewWithRedis` wires the identical gRPC server/repos/brokers/
clients against an already-running Redis address without starting or
owning a container. Every suite in `internal/integration` and
`internal/integration/character` now shares one `harness.RedisContainer`
per test process (started/terminated once in that package's `TestMain`)
instead of starting a fresh container per test method — see
`docs/how-to/run-integration-tests.md` for the full pattern and the
one-Docker-runner-at-a-time rule this depends on.

**Updated 2026-07-13 (rpg-api#642):** the v1-only `internal/integration/encounter/`
suite (10 files: class-specific combat tests, `open_door_test.go`,
`stream_entity_state_test.go`, `helpers.go`, and siblings) is deleted along with
the v1alpha1 EncounterService it exercised. The harness's `EncounterClient`
field and matching v1 wiring (v1 orchestrator/repo/publisher/processor
construction) are removed. Encounter coverage now lives entirely in
`internal/integration/encounter_v2_test.go` and
`internal/integration/lobby_v1alpha1_test.go`.

## Files

| File | Purpose |
|---|---|
| `integration/harness/harness.go` | TestServer wiring (`New`, `NewWithRedis`, `Close`) |
| `integration/harness/redis_container.go` | `RedisContainer` shared-fixture: `StartRedis`, `Terminate`, `Lease` (rpg-api#699) |
| `integration/main_test.go` | `TestMain` owning the one shared container for this package's process |
| `integration/shared_fixture_regression_test.go` | Regression coverage that the shared fixture actually delivers per-test freshness + Redis state isolation (rpg-api#699) |
| `integration/character/main_test.go` | Same, for the separate `internal/integration/character` process |
| `integration/character/` | Character integration tests |
| `integration/encounter_v2_test.go` | v1alpha2 encounter service tests |
| `integration/lobby_v1alpha1_test.go` | LobbyService tests, incl. combat-entry flows |

## Architecture

```
RedisContainer (redis_container.go)         ← package/process-level fixture
    └── testcontainers Redis                 ← one real Redis process in Docker,
                                                started once by TestMain, terminated once

TestServer (harness.go), one fresh instance per test
    ├── Redis client (New: owns a container it started itself;
    │                 NewWithRedis: borrows a RedisContainer's Addr,
    │                 owns only its own client connection)
    ├── bufconn               ← in-process gRPC (no network)
    ├── real orchestrators    ← same code as production (character, lobby)
    ├── real repositories     ← character (Redis), encounters/v2 (in-memory), lobby (in-memory)
    └── real v2 encounter broker ← rpg-toolkit's tkenc.Broker, in-memory transport
```

`TestServer.CharacterClient`, `.EncounterClientV2`, `.DiceClient`, `.LobbyClient` — proto-generated gRPC clients that connect to the in-process server. Tests call these clients exactly as the web client would.

## TestServer lifecycle

Standalone (owns its own container):

```go
ctx := context.Background()
server, err := harness.New(ctx, harness.DefaultConfig())
defer server.Close()

resp, err := server.EncounterClientV2.CreateEncounter(ctx, &proto.CreateEncounterRequest{...})
```

`Close()` terminates the gRPC server and, because this `TestServer` owns
one, its Redis container.

Shared-fixture (the pattern every suite in `internal/integration` and
`internal/integration/character` uses — see that package's `TestMain`):

```go
// TestMain, once per process:
sharedRedis, _ = harness.StartRedis(ctx)
defer sharedRedis.Terminate(ctx) // or an explicit call after m.Run()

// SetupTest, once per test:
release := sharedRedis.Lease()
srv, _ := harness.NewWithRedis(ctx, cfg, sharedRedis.Addr)
srv.FlushRedis(ctx)

// TearDownTest, once per test:
srv.Close()   // closes gRPC server, brokers, this test's own Redis client —
              // never terminates sharedRedis's container
release()
```

## Known gaps

### ~~testcontainers startup time~~ FIXED (rpg-api#699, 2026-07-23)

Previously: each test suite that created a new `TestServer` via `harness.
New` started a fresh Redis container, adding ~2-5s of container create/
health-check/teardown churn per test *method* (not just per suite) for
every suite using `SetupTest` rather than `SetupSuite`. A full `go test ./internal/integration/...` run measured ~283s this way
(accepted baseline, 33 container lifecycles). Fixed by splitting
container ownership (`RedisContainer`) from per-test service wiring
(`NewWithRedis`): a post-fix `go test -v -race ./internal/integration/...`
run measured 37.4s wall clock and started/terminated exactly 3 containers
total (one per Go test process) — see
`docs/how-to/run-integration-tests.md` for the measured before/after and
the exact per-process container count.

### ~~Round 2 tests not yet green on main~~ MOOT (rpg-api#642, 2026-07-13)

The `open_door_test.go` and `stream_entity_state_test.go` this section
described were part of the deleted v1-only `internal/integration/encounter/`
suite. #468 and its stacked PRs are moot — see `docs/status.md`.

### v1alpha2 EncounterService character-store wiring (fixed rpg-api#634, 2026-07-11)

The v1alpha2 `EncounterService` handler (`ts.EncounterClientV2`, registered via
`encounterhandlerv2.New` in `wireServices`) was constructed with no
`CombatResolverConfig`/`MovementResolverConfig` — unlike `cmd/server/server.go`'s
production wiring, which passes `charRepo` through both. Without it, the v2 encounter
orchestrator's `characterData.Attach` hydration cascade (`#689`) silently never ran:
every combat-capable RPC (`TakeAction`, `EndTurn`, `MoveEntity`, `SubmitCheckReaction`)
fell back to the stat-snapshot stand-in resolver path regardless of what a test seeded.
This was invisible for years of tests because every existing combat fixture seeds
explicit `AttackBonus`/`DamageDice`/`DamageType` on `PlayerInput` directly (the
devseed-fixture pattern) — none needed real hydration. It became a real blocker for
testing a lobby-created player (which, by design, carries no flat combat snapshot — see
`internal/orchestrators/lobby/character.go`'s `seedMemberCombatSnapshot`), so it's fixed
now: `wireServices` passes `charRepo` through `CombatResolverConfig`/
`MovementResolverConfig` exactly as production does.

## Value

The harness is what proves the full system works end-to-end. Unit tests with mocks verify orchestrator logic in isolation. The harness verifies:
- Proto serialization/deserialization round-trips
- Handler ↔ orchestrator input/output wiring
- Real Redis operations (character persistence, pub/sub)
- Initiative order, combat state, event emission in sequence
- Class-specific combat paths (rage, flurry of blows, second wind, sneak attack)

**If a change breaks integration tests, it breaks the game.**
