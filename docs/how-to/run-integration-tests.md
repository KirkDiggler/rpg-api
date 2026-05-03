---
name: run integration tests
description: How to run the integration test harness — requires Docker for testcontainers
updated: 2026-05-02
---

# How to run integration tests

Integration tests live in `internal/integration/`. They use `testcontainers-go` to spin up a real Redis instance, so Docker must be running.

## Prerequisites

- Docker running (`docker ps`)
- Go 1.24.1+

## Run all tests (unit + integration)

```bash
cd /home/kirk/personal/rpg-api
go test -v -race ./...
```

This runs everything. Integration tests are not build-tagged, so they run with the full test suite. Each suite that creates a `TestServer` will start a Redis container.

## Run integration tests only

```bash
# Encounter integration tests
go test -v -race ./internal/integration/encounter/...

# Character integration tests
go test -v -race ./internal/integration/character/...
```

## Run a specific test

```bash
# Run the open door test
go test -v -race -run TestOpenDoor ./internal/integration/encounter/...

# Run the monk combat scenario
go test -v -race -run TestMonkKillsMonster ./internal/integration/encounter/...
```

## What the harness wires

`internal/integration/harness/harness.go` builds a `TestServer` with:
- Real Redis container (via testcontainers)
- In-process gRPC server via `bufconn` (no network overhead)
- Real orchestrators, real repositories
- Real event processor + Redis publisher
- `DevMode: true` — uses `Dev <player_id>` auth

Exported on `TestServer`:
- `CharacterClient dnd5ev1alpha1.CharacterServiceClient`
- `EncounterClient dnd5ev1alpha1.EncounterServiceClient`
- `DiceClient apiv1alpha1.DiceServiceClient`
- `CharacterRepo characterrepo.Repository` — for seeding test data
- `EncounterPublisher encounterpub.Publisher` — for subscribing to events in tests

## Writing new integration tests

```go
func TestMyScenario(t *testing.T) {
    ctx := context.Background()
    server, err := harness.New(ctx, harness.DefaultConfig())
    require.NoError(t, err)
    defer server.Close()

    // Seed a character
    charResp, err := server.CharacterClient.GetCharacter(ctx, &proto.GetCharacterRequest{...})

    // Drive the encounter
    resp, err := server.EncounterClient.CreateEncounter(ctx, ...)
    require.NoError(t, err)
}
```

See `internal/integration/encounter/helpers.go` for shared setup helpers (create character, create encounter, etc.).

## Test timing

Container startup is the bottleneck — typically 2–5 seconds per test suite. Tests that use `SetupSuite` share one container per suite (fast). Tests that create a harness per test case pay startup cost per test.

## Known gaps

- Round 2 open-door and stream entity state tests (`open_door_test.go`, `stream_entity_state_test.go`) were added during the failing #468 branch and have not been verified green on main. They should be green after the coordinate refactor lands.
