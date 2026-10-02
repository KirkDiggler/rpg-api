---
name: run locally
description: How to start the rpg-api gRPC server locally with dev auth enabled
updated: 2026-10-02
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
| `RPG_DEV_WORLD_ID` | `test-world` | Default world for `Dev` auth (single local simulation world) |
| `RPG_DEV_WORLD_IDS` | unset | Optional comma-separated allowlist of canonical guild-shaped IDs a `Dev` request may select with `x-rpg-guild-id`. Honored only with `AUTH_DEV_MODE=true`; must include `RPG_DEV_WORLD_ID` |
| `REDIS_ADDR` | `localhost:6379` | Redis address (check `cmd/server/server.go:mustRedisClient`) |

## Auth in dev mode

With `AUTH_DEV_MODE=true`, all gRPC calls must include the header:
```
Authorization: Dev <your-player-id>
```

### Selecting between local simulation worlds

`RPG_DEV_WORLD_IDS` is an optional comma-separated allowlist of canonical
non-zero decimal guild-shaped IDs. It is honored only when `AUTH_DEV_MODE=true`
and the request uses the `Dev` scheme; a real `Discord` credential always
verifies real membership and never uses it. The existing `RPG_DEV_WORLD_ID`
(default `test-world`) is the default and must appear in the allowlist.

```bash
AUTH_DEV_MODE=true \
RPG_DEV_WORLD_ID=123456789012345678 \
RPG_DEV_WORLD_IDS=123456789012345678,223456789012345678 \
  go run ./cmd/server server
```

With the allowlist set, a `Dev` request may select an allowed world with the
existing `x-rpg-guild-id` header; omitting it uses the default. An unknown
selector is denied; a repeated or non-canonical selector is rejected. With the
allowlist unset, the fixed `RPG_DEV_WORLD_ID` behavior is preserved and any
selector is ignored. A malformed, empty, or duplicated allowlist entry, or a
list missing `RPG_DEV_WORLD_ID`, stops server startup rather than silently
broadening access.

```bash
# Same Dev player, two local worlds on one API/database
grpcurl -plaintext \
  -H "Authorization: Dev player-1" \
  -H "x-rpg-guild-id: 223456789012345678" \
  -d '{}' \
  localhost:50051 \
  dnd5e.api.v1alpha1.CharacterService/ListCharacters
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
