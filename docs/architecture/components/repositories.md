---
name: repositories
description: All data access components — interface definitions, implementations, and storage schemas
updated: 2026-09-05
confidence: high — #921 composition persistence is verified through focused Redis repository tests; older repository notes retain stated caveats
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
| `composition` | `repository.go` | `redis.go` | Redis | B |
| `dice_session` | `repository.go` | `redis.go` | Redis | B- |
| ~~`encounters/v2`~~ | DELETED | DELETED | DELETED | n/a |
| `lobby` | `repository.go` | `redis.go` + `in_memory.go` | Redis + in-memory | B |
| `sessionpresentation` | `repository.go` | `redis.go` | Redis | B+ |

~~`encounters` (v1 root)~~ / ~~`dungeons`~~ / ~~`encounterlog`~~ — all DELETED
(rpg-api#642).

## Character repository

**Path:** `repositories/character/`

Interface methods use value Input and pointer Output types: `Create`, `Get`, `Update`,
`PatchEquipment`, `Delete`, `ListByPlayerID`, and `ListBySessionID`. `GetOutput` includes
an opaque version derived from the stored bytes. `PatchEquipmentInput` carries the
expected version/equipment plus only the proposed EquipmentSlots and cached ArmorClass.

**Storage:** `character:{id}` — JSON-serialized `entities.Character`, whose only
field is toolkit `character.Data` (including nested `Data.Appearance`), with
player/session index keys.

`PatchEquipment` uses Redis WATCH/MULTI. A stale equipment map returns ABORTED. A changed
version with unchanged equipment returns the latest entity without writing so the
orchestrator can strictly reproject; a successful transaction changes only the two
allowed fields and returns the actual patched entity. Miniredis tests cover concurrent
combat-state preservation and stale equipment refusal.

Used by: character, lobby, and session orchestration plus owner/public projections.

## Character draft repository

**Path:** `repositories/character_draft/`

Interface methods: `Create`, `Get`, `List`, `Update`, `Delete`.

**Storage:** Redis keys per draft. JSON stores the thin `entities.CharacterDraft`
wrapper around toolkit `character.DraftData`, including nested Appearance. Focused
Redis tests cover complete Appearance and present-zero optional values.

## Composition repository

**Path:** `repositories/composition/`

Guild-scoped Redis storage for stable composition definitions and immutable source
revisions. The typed contract creates a definition, appends a revision with an
expected-head CAS, gets one pinned revision, and lists current definition/head pairs in
definition-ID order. `ListDefinitions` deliberately fails closed if any indexed
definition or head is missing, unreadable, or corrupt; it never returns a silently
partial library.

One SHA-256 guild hash tag keeps the definition, revision, and index keys in one Redis
Cluster slot. Lua scripts preflight modeled type, identity, collision, index, and head
conflicts before writes; append also compares the exact definition/head bytes validated
by its read preflight, so those refusals are atomic. This is not an end-to-end
exactly-once acknowledgment guarantee: go-redis may retry a script after a lost reply
and report a collision/stale-head result even if the first execution committed. Before
any RPC exposure, the service integration must provide and test idempotent requests or
outcome reconciliation for that indeterminate result.

Stored source version 1 is read-validated with its version-1 semantics. Those semantics
must remain stable for immutable historical reads when future source versions are
introduced; new validation rules must be selected by the stored version rather than
retroactively tightened.

This is repository-only infrastructure. It is not wired into server DI or RPCs and adds
no guild authorization, dungeon/world integration, toolkit rule, rendering pipeline, or
user-visible readiness.

## Dice session repository

**Path:** `repositories/dice_session/`

Narrow scope: tracks ability score dice rolls during character creation before they are assigned to a draft. Redis-backed, simple interface.

## ~~Encounter repository (v2)~~ DELETED

`repositories/encounters/v2/` is deleted with the old v1alpha2 encounter stack.
Toolkit session state is now owned by `rulebooks/dnd5e/session.Manager`, wired by
`internal/orchestrators/session` over Redis-backed toolkit repositories.

## Session roster

The Session SDK owns roster state and public identity projection inside
`rulebooks/dnd5e/session.Manager`. `SessionService.GetRoster` and the shared
`sessionaccess.Access` gates call the manager's `Roster` method; rpg-api has no roster
repository or launch-time duplicate.

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
