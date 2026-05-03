---
name: run locally
description: How to start the rpg-api gRPC server locally with dev auth enabled
updated: 2026-05-02
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

## Auth in dev mode

With `AUTH_DEV_MODE=true`, all gRPC calls must include the header:
```
Authorization: Dev <your-player-id>
```

Example with `grpcurl`:
```bash
grpcurl -plaintext \
  -H "Authorization: Dev player-1" \
  -d '{"player_id": "player-1"}' \
  localhost:50051 \
  dnd5e.api.v1alpha1.EncounterService/CreateEncounter
```

## Using the encounter test client

`cmd/server/encounter_client.go` provides a simple test client. Check that file for usage examples.

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

Expected output includes `dnd5e.api.v1alpha1.CharacterService` and `dnd5e.api.v1alpha1.EncounterService`.
