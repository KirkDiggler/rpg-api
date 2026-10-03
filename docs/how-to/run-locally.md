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
| `RPG_WORLD_ACCESS_ENFORCEMENT` | `false` | Opts into world-role admission for gameplay unary calls and streams |
| `RPG_DEV_PERMISSION_LEVEL` | `none` | Explicit Dev role fixture when enforcement is enabled: `none`, `player`, `builder`, `admin` |
| `REDIS_ADDR` | `localhost:6379` | Redis address (check `cmd/server/server.go:mustRedisClient`) |

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

## Opt-in world-access testing

Normal local play and the existing Discord deployment retain their authenticated
behavior when `RPG_WORLD_ACCESS_ENFORCEMENT` is unset or `false`. Publishing the
code does not enable new role requirements. Existing private-resource checks,
composition membership/world checks, and WorldService owner/admin authorization
remain active; this is not an anonymous-access switch or a default-world bypass.

Enable enforcement only in the environment deliberately testing it. For a Dev
player who should play and author:

```bash
AUTH_DEV_MODE=true RPG_WORLD_ACCESS_ENFORCEMENT=true \
RPG_DEV_PERMISSION_LEVEL=builder go run ./cmd/server server
```

With enforcement enabled, missing Dev permissions grant nothing. Real Discord
credentials always use real membership and configured roles, even on a Dev
server. The target Discord server needs owner setup and an access walk before
its deployment opts in. Never enable `AUTH_DEV_MODE` in production.

The enforcement setting is read at startup; invalid values refuse startup.
It gates admission only, not storage migration or future world-owned schema
cutover. Do not enable unfinished storage changes through an ordinary release.

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
