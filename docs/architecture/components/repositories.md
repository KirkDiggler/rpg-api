---
name: repositories
description: All data access components — interface definitions, implementations, and storage schemas
updated: 2026-07-13
confidence: high — verified by reading all remaining repository interface and implementation files
---

# repositories

**Updated 2026-07-13 (rpg-api#642):** the three in-memory-only, D-graded
repositories this doc used to describe (`encounters` v1 root, `dungeons`,
`encounterlog`) are deleted along with the v1alpha1 encounter stack they
served. See "Encounter repository (v2)" below for the replacement.

## Overview

| Repository | Interface | Implementation | Storage | Grade |
|---|---|---|---|---|
| `character` | `repository.go` | `redis.go` | Redis | B |
| `character_draft` | `repository.go` | `redis.go` | Redis | B- |
| `dice_session` | `repository.go` | `redis.go` | Redis | B- |
| `encounters/v2` | `repository.go` | `redis.go` + `in_memory.go` | Redis + in-memory | B |
| `lobby` | `repository.go` | `redis.go` + `in_memory.go` | Redis + in-memory | B |

~~`encounters` (v1 root)~~ / ~~`dungeons`~~ / ~~`encounterlog`~~ — all DELETED
(rpg-api#642).

## Character repository

**Path:** `repositories/character/`

Interface methods:
- `Get(ctx, *GetInput) (*GetOutput, error)` — by character ID
- `GetByPlayerID(ctx, *GetByPlayerIDInput) (*GetByPlayerIDOutput, error)`
- `List(ctx, *ListInput) (*ListOutput, error)` — by player ID
- `Save(ctx, *SaveInput) (*SaveOutput, error)`
- `Delete(ctx, *DeleteInput) (*DeleteOutput, error)`

**Storage:** `character:{id}` — JSON-serialized `character.Data` + `Appearance` (stored together).

Used by: character orchestrator, the v1alpha2 encounter path (character-data
hydration cascade), the lobby orchestrator.

The only repository exercised by integration tests with real Redis alongside
the v2 encounter and lobby repos.

## Character draft repository

**Path:** `repositories/character_draft/`

Interface methods: `Create`, `Get`, `List`, `Update`, `Delete`.

**Storage:** Redis keys per draft. Handles in-progress character creation state. Less tested than character repo — no integration tests that specifically exercise draft lifecycle.

## Dice session repository

**Path:** `repositories/dice_session/`

Narrow scope: tracks ability score dice rolls during character creation before they are assigned to a draft. Redis-backed, simple interface.

## Encounter repository (v2)

**Path:** `repositories/encounters/v2/`

Interface: `Get(ctx, *GetInput) (*GetOutput, error)`, `Save(ctx, *SaveInput) (*SaveOutput, error)` — operating on the rpg-toolkit encounter SDK's own `*encounter.Data` type directly (the repository does not define its own storage struct the way the deleted v1 repo did).

Two implementations: `in_memory.go` (JSON round-trip, used by tests and the
integration harness) and `redis.go` (24h TTL, wired into `cmd/server/server.go`
production wiring and the lobby orchestrator's `StartEncounter`). Unlike the
deleted v1 encounter repo, this one has a persistent backend from day one.

See [`encounter.md`](./encounter.md) for the v1alpha2 vertical this repo serves.

## Common patterns

All repositories:
- Input/Output types on every method (non-negotiable)
- Generated mocks in `mock/` subdirectory using `go:generate mockgen`
- Never return `(nil, nil)` — valid output or error
- Interface defined in `repository.go` alongside types

Mock packages follow `<domain>mock` naming (e.g., `encountermock`, `charactermock`).
