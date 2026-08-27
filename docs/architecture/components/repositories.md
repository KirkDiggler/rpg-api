---
name: repositories
description: All data access components — interface definitions, implementations, and storage schemas
updated: 2026-08-28
confidence: high — #852 verified Redis roster/presentation repository roles against code and cross-instance integration; older repository notes retain stated caveats
---

# repositories

**Updated 2026-07-13 (rpg-api#642):** the three in-memory-only, D-graded
repositories this doc used to describe (`encounters` v1 root, `dungeons`,
`encounterlog`) are deleted along with the v1alpha1 encounter stack they
served. The later v2 encounter repository is also deleted with rpg-project#227.

## Overview

| Repository | Interface | Implementation | Storage | Grade |
|---|---|---|---|---|
| `character` | `repository.go` | `redis.go` | Redis | B |
| `character_draft` | `repository.go` | `redis.go` | Redis | B- |
| `dice_session` | `repository.go` | `redis.go` | Redis | B- |
| ~~`encounters/v2`~~ | DELETED | DELETED | DELETED | n/a |
| `lobby` | `repository.go` | `redis.go` + `in_memory.go` | Redis + in-memory | B |
| `roster` | `repository.go` | `redis.go` + `in_memory.go` | Redis + in-memory | B+ |
| `sessionpresentation` | `repository.go` | `redis.go` | Redis | B+ |

~~`encounters` (v1 root)~~ / ~~`dungeons`~~ / ~~`encounterlog`~~ — all DELETED
(rpg-api#642).

## Character repository

**Path:** `repositories/character/`

Interface methods use value Input and pointer Output types: `Create`, `Get`, `Update`,
`PatchEquipment`, `Delete`, `ListByPlayerID`, and `ListBySessionID`. `GetOutput` includes
an opaque version derived from the stored bytes. `PatchEquipmentInput` carries the
expected version/equipment plus only the proposed EquipmentSlots and cached ArmorClass.

**Storage:** `character:{id}` — JSON-serialized `character.Data` + `Appearance` (stored
together), with player/session index keys.

`PatchEquipment` uses Redis WATCH/MULTI. A stale equipment map returns ABORTED. A changed
version with unchanged equipment returns the latest entity without writing so the
orchestrator can strictly reproject; a successful transaction changes only the two
allowed fields and returns the actual patched entity. Miniredis tests cover concurrent
combat-state preservation and stale equipment refusal.

Used by: character, lobby, and session orchestration plus owner/public projections.

## Character draft repository

**Path:** `repositories/character_draft/`

Interface methods: `Create`, `Get`, `List`, `Update`, `Delete`.

**Storage:** Redis keys per draft. Handles in-progress character creation state. Less tested than character repo — no integration tests that specifically exercise draft lifecycle.

## Dice session repository

**Path:** `repositories/dice_session/`

Narrow scope: tracks ability score dice rolls during character creation before they are assigned to a draft. Redis-backed, simple interface.

## ~~Encounter repository (v2)~~ DELETED

`repositories/encounters/v2/` is deleted with the old v1alpha2 encounter stack.
Toolkit session state is now owned by `rulebooks/dnd5e/session.Manager`, wired by
`internal/orchestrators/session` over Redis-backed toolkit repositories.

## Roster repository

**Path:** `repositories/roster/`

Stores the launch-written public membership row for a started session. Production and
`internal/integration/harness` use `NewRedis(client, 24*time.Hour)` so `SessionService`
and `SessionPresentationService` share the exact same seated-member gate across server
instances. The in-memory implementation remains for unit tests.

## Session presentation repository

**Path:** `repositories/sessionpresentation/`

Stores accepted presentation payloads under hashed session keys and fans them out over
Redis Pub/Sub. A Redis script makes `(session, presentation_id, attempt)` acceptance
atomic: first write publishes, identical duplicate returns the accepted payload, and a
different duplicate returns `ErrConflict`. Accepted keys have a two-minute TTL;
subscriptions are live-only and close on context/subscription shutdown.

## Common patterns

All repositories:
- Input/Output types on every method (non-negotiable)
- Generated mocks in `mock/` subdirectory using `go:generate mockgen`
- Never return `(nil, nil)` — valid output or error
- Interface defined in `repository.go` alongside types

Mock packages follow `<domain>mock` naming (e.g., `encountermock`, `charactermock`).
