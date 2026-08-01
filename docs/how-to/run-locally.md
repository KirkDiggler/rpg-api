---
name: run locally
description: How to start the rpg-api gRPC server locally with dev auth enabled
updated: 2026-07-13
---

# How to run rpg-api locally

## Prerequisites

- Go 1.24.1+ (`go version`)
- Redis running locally on port 6379 (or via Docker)
- Discord token OR dev mode enabled (see below)

## Start Redis

```bash
# Docker (easiest)
docker run -d --name rpg-redis -p 6379:6379 redis:alpine

# Or via Homebrew on macOS
brew services start redis
```

## Build and run

```bash
cd /home/kirk/personal/rpg-api

# Run directly
AUTH_DEV_MODE=true go run ./cmd/server server

# Or build first
go build -o bin/rpg-api ./cmd/server
AUTH_DEV_MODE=true ./bin/rpg-api server
```

Default port: `50051`. Override with `--port <n>`.

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `AUTH_DEV_MODE` | `false` | Enables `Dev <player_id>` auth scheme (never in production) |
| `REDIS_ADDR` | `localhost:6379` | Redis address (check `cmd/server/server.go:mustRedisClient`) |
| `RPG_AUTHORING_ENABLED` | unset | Registers `AuthoringService` (`PutDungeon`) when set to any non-empty value. Unset = the service isn't registered at all — `grpcurl list` won't show it, and any call gets gRPC's own `Unimplemented`. Requires `RPG_CONTENT_DIR` also be set (construction-time failure otherwise). |
| `RPG_CONTENT_DIR` | unset | Directory of `*.yaml`/`*.yml` dungeon-spec overrides, layered over the embedded `internal/content/dungeons/*.yaml` set (already existed for content override). **New role**: when `RPG_AUTHORING_ENABLED` is set, this is also `PutDungeon`'s write-through target — required in that mode, not optional. |

## Auth in dev mode

With `AUTH_DEV_MODE=true`, all gRPC calls must include the header:
```
Authorization: Dev <your-player-id>
```

Example with `grpcurl`, using `LobbyService.CreateLobby` (updated 2026-07-13,
rpg-api#642 — the v1alpha1 `EncounterService` this example used to call is
deleted; `CreateLobby` is the current sole encounter-construction entry
point, per `docs/architecture/components/lobby-service.md`):
```bash
grpcurl -plaintext \
  -H "Authorization: Dev player-1" \
  -d '{"campaign_id": "campaign-1", "character_id": "char-1"}' \
  localhost:50051 \
  dnd5e.api.lobby.v1alpha1.LobbyService/CreateLobby
```

## Health check

```bash
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check
```

Health and gRPC reflection endpoints bypass auth.

## Verify server is up

```bash
# List services (no auth required)
grpcurl -plaintext localhost:50051 list
```

Expected output includes `dnd5e.api.v1alpha1.CharacterService`,
`api.v1alpha1.DiceService`, `dnd5e.api.v1alpha2.encounter.EncounterService`,
and `dnd5e.api.lobby.v1alpha1.LobbyService`. **Updated 2026-07-13
(rpg-api#642):** `dnd5e.api.v1alpha1.EncounterService` is no longer
registered — the v1alpha1 encounter stack is deleted.
