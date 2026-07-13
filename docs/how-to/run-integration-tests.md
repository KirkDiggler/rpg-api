---
name: run integration tests
description: How to run the integration test harness — requires Docker for testcontainers
updated: 2026-07-13
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

## What the harness wires

`internal/integration/harness/harness.go` builds a `TestServer` with:
- Real Redis container (via testcontainers)
- In-process gRPC server via `bufconn` (no network overhead)
- Real orchestrators (character, lobby), real repositories
- The v2 encounter path's `tkenc.Broker` for live event fan-out
- `DevMode: true` — uses `Dev <player_id>` auth

Exported on `TestServer` (updated 2026-07-13, rpg-api#642 — `EncounterClient`
and `EncounterPublisher` are gone with the v1alpha1 stack):
- `CharacterClient dnd5ev1alpha1.CharacterServiceClient`
- `EncounterClientV2 encounterv2pb.EncounterServiceClient`
- `LobbyClient lobbyv1alpha1pb.LobbyServiceClient`
- `DiceClient apiv1alpha1.DiceServiceClient`
- `CharacterRepo characterrepo.Repository` — for seeding test data
- `BrokerV2` / `EncRepoV2` — for seeding/inspecting v2 encounter state directly

## Writing new integration tests

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

Container startup is the bottleneck — typically 2–5 seconds per test suite. Tests that use `SetupSuite` share one container per suite (fast). Tests that create a harness per test case pay startup cost per test.

## Known gaps

~~Round 2 open-door and stream entity state tests...~~ MOOT (rpg-api#642,
2026-07-13): these tests belonged to the deleted v1-only
`internal/integration/encounter/` suite. See `docs/status.md` "Paused / on
hold" for the moot #459–#468 PR record.
