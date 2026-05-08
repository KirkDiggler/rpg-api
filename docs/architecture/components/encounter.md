---
name: encounter (v1alpha2)
description: v1alpha2 encounter service — lightweight toolkit adapter, no orchestrator
updated: 2026-05-07
confidence: high — verified by reading handler.go, create.go, project.go, translate.go
---

# encounter (v1alpha2)

The v1alpha2 encounter service is the second-generation encounter handler. Unlike the v1alpha1 handler which delegates through a full orchestrator stack, v2 talks directly to the rpg-toolkit encounter SDK and a v2 repository. The handler is a thin wire adapter — no business logic, no orchestrator.

## Files

| File | Purpose |
|---|---|
| `internal/handlers/dnd5e/v2/encounter/handler.go` | Handler struct, constructor, MoveEntity, StreamEncounter |
| `internal/handlers/dnd5e/v2/encounter/create.go` | CreateEncounter RPC |
| `internal/handlers/dnd5e/v2/encounter/project.go` | ProjectFor helper — toolkit Data → proto Encounter |
| `internal/handlers/dnd5e/v2/encounter/translate.go` | Event translator — toolkit events → proto EncounterEvent |
| `internal/repositories/encounters/v2/repository.go` | Repository interface (Get, Save) |
| `internal/repositories/encounters/v2/in_memory.go` | In-memory implementation (JSON round-trip) |
| `internal/repositories/encounters/v2/redis.go` | Redis implementation |

## v2 RPCs

| RPC | Status | Wave |
|---|---|---|
| `CreateEncounter` | Implemented (#499) | 2.6 |
| `GetEncounter` | Unimplemented | 2.6 (#500) |
| `StreamEncounter` | Implemented (#496) | 2.5 |
| `MoveEntity` | Implemented (#496) | 2.5 |

## CreateEncounter flow

1. Validate auth — `auth.GetPlayerID(ctx)` empty → `Unauthenticated`.
2. Validate request — `campaign_id` empty → `InvalidArgument`.
3. Construct `tkenc.New(uuid, broker)`, add creator as first player via `enc.AddPlayer`.
4. Persist via `h.encRepo.Save(ctx, enc.ToData())`.
5. Build proto response via `ProjectFor(data, viewer, broker, now)`.

## ProjectFor helper

`project.go` exposes a single exported function:

```go
func ProjectFor(
    data *tkenc.Data,
    viewer core.PlayerID,
    broker *tkenc.Broker,
    now time.Time,
) (*encounterv2pb.Encounter, error)
```

It is the single source of truth for toolkit Snapshot → proto Encounter translation. Called by `CreateEncounter` today; `GetEncounter` (#500) and snapshot replay (#497) will reuse it without duplicating the projection logic.

## Architecture notes

- Handler imports no orchestrator — direct toolkit dependency is intentional for v2.
- `encounter.Broker` is process-scoped (one broker per server process, shared across all v2 RPCs).
- Repository follows the standard Input/Output interface pattern but `Get` returns `*encounter.Data` directly (toolkit's native type is the domain entity here).
- Entity ID in CreateEncounter defaults to player ID for the initial seat; future PRs will wire in character selection.
