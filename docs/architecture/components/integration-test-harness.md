---
name: integration test harness
description: Full-stack test server wiring real Redis, in-process gRPC, and testcontainers
updated: 2026-07-13
confidence: high — verified by reading harness.go and remaining integration test files
---

# integration test harness

The integration test harness (`internal/integration/harness/`) is the most valuable test asset in rpg-api. It wires the full stack — real Redis via testcontainers, real orchestrators and repositories, in-process gRPC via bufconn — and exercises complete game flows via proto-level client calls.

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
| `integration/harness/harness.go` | TestServer setup and teardown |
| `integration/character/` | Character integration tests |
| `integration/encounter_v2_test.go` | v1alpha2 encounter service tests |
| `integration/lobby_v1alpha1_test.go` | LobbyService tests, incl. combat-entry flows |

## Architecture

```
TestServer (harness.go)
    ├── testcontainers Redis  ← real Redis process in Docker
    ├── bufconn               ← in-process gRPC (no network)
    ├── real orchestrators    ← same code as production (character, lobby)
    ├── real repositories     ← character (Redis), encounters/v2 (in-memory), lobby (in-memory)
    └── real v2 encounter broker ← rpg-toolkit's tkenc.Broker, in-memory transport
```

`TestServer.CharacterClient`, `.EncounterClientV2`, `.DiceClient`, `.LobbyClient` — proto-generated gRPC clients that connect to the in-process server. Tests call these clients exactly as the web client would.

## TestServer lifecycle

```go
// Setup (per test file or suite)
ctx := context.Background()
server, err := harness.New(ctx, harness.DefaultConfig())
defer server.Close()

// Use clients
resp, err := server.EncounterClient.CreateEncounter(ctx, &proto.CreateEncounterRequest{...})
```

`Close()` terminates the gRPC server and the Redis container.

## Known gaps

### ~~Round 2 tests not yet green on main~~ MOOT (rpg-api#642, 2026-07-13)

The `open_door_test.go` and `stream_entity_state_test.go` this section
described were part of the deleted v1-only `internal/integration/encounter/`
suite. #468 and its stacked PRs are moot — see `docs/status.md`.

### testcontainers startup time

Each test suite that creates a new `TestServer` starts a fresh Redis container. Container startup adds ~2-5 seconds per test suite. Tests sharing a harness via `SetupSuite` amortize this cost; tests that create a harness per test case do not.

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
