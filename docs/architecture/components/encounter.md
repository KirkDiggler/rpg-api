---
name: encounter (v1alpha2)
description: v1alpha2 encounter service — lightweight toolkit adapter, no orchestrator
updated: 2026-05-08
confidence: high — verified by reading handler.go, create.go, get.go, project.go, translate.go, translate_test.go, encounter_v2_test.go
---

# encounter (v1alpha2)

The v1alpha2 encounter service is the second-generation encounter handler. Unlike the v1alpha1 handler which delegates through a full orchestrator stack, v2 talks directly to the rpg-toolkit encounter SDK and a v2 repository. The handler is a thin wire adapter — no business logic, no orchestrator.

## Files

| File | Purpose |
|---|---|
| `internal/handlers/dnd5e/v2/encounter/handler.go` | Handler struct, constructor, MoveEntity, StreamEncounter |
| `internal/handlers/dnd5e/v2/encounter/create.go` | CreateEncounter RPC |
| `internal/handlers/dnd5e/v2/encounter/get.go` | GetEncounter RPC |
| `internal/handlers/dnd5e/v2/encounter/project.go` | ProjectFor helper — toolkit Data → proto Encounter |
| `internal/handlers/dnd5e/v2/encounter/translate.go` | Event translator — toolkit events → proto EncounterEvent |
| `internal/repositories/encounters/v2/repository.go` | Repository interface (Get, Save) |
| `internal/repositories/encounters/v2/in_memory.go` | In-memory implementation (JSON round-trip) |
| `internal/repositories/encounters/v2/redis.go` | Redis implementation |

## v2 RPCs

| RPC | Status | Wave |
|---|---|---|
| `CreateEncounter` | Implemented (#499) | 2.6 |
| `GetEncounter` | Implemented (#500) | 2.6 |
| `StreamEncounter` | Implemented (#496, replay #497) | 2.5 / 2.7 |
| `MoveEntity` | Implemented (#496) | 2.5 |

## CreateEncounter flow

1. Validate auth — `auth.GetPlayerID(ctx)` empty → `Unauthenticated`.
2. Validate request — `campaign_id` empty → `InvalidArgument`.
3. Construct `tkenc.New(uuid, broker)`, add creator as first player via `enc.AddPlayer`.
4. Persist via `h.encRepo.Save(ctx, enc.ToData())`.
5. Build proto response via `ProjectFor(data, viewer, broker, now)`.

## GetEncounter flow

1. Validate auth — `auth.GetPlayerID(ctx)` empty → `Unauthenticated`.
2. Validate request — `encounter_id` empty → `InvalidArgument`.
3. Load encounter via `h.encRepo.Get`; `ErrNotFound` → `NotFound`.
4. Authority check — `data.Players[core.PlayerID(playerID)]` lookup; not present → `PermissionDenied`. Mirrors `MoveEntity` exactly.
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

It is the single source of truth for toolkit Snapshot → proto Encounter translation. Called by `CreateEncounter` and `GetEncounter`; snapshot replay (#497) will reuse it without duplicating the projection logic.

### Entity visibility in ProjectFor (#500 broadening)

As of #500, `ProjectFor` includes all entities currently visible to the viewer — not just the viewer's own entity. Visibility is computed using `perception.VisibleHexesAt(viewer.Position, viewer.SightRange)` against each other player's current position in `data.Players`. Entities are sorted by player ID for deterministic wire output. This gives clients a real point-in-time snapshot of what the viewer can see.

## StreamEncounter flow (#497)

`StreamEncounter` uses a subscribe-before-snapshot ordering to guarantee no events are missed:

1. **Subscribe** to the broker first — broker holds events in a buffered channel.
2. **Load** encounter data from repo.
3. **Build projection** — `ProjectFor(data, viewer, broker, now)` once; result used for both the snapshot envelope and replay events.
4. **Send `SnapshotDelivered`** — inner `Encounter` field populated (non-nil, ID set).
5. **Send replay events** — `BuildReplayEvents(pbEncounter, snap, now)`:
   - One `EntityAppeared` per entity in `pbEncounter.Space.Entities` (includes viewer's own entity so the client can render its initial position).
   - One `GeometryRevealed` carrying all of `snap.RevealedHexes` (sorted deterministically by Q,R,S). Omitted if RevealedHexes is empty.
6. **Live forward loop** — drains broker's buffered channel; translates toolkit events to proto via `TranslateEvent`.

The broker's per-viewer projection (LoS-crossing semantics) guarantees no duplication: `EntityAppeared` fires only when an entity enters the viewer's sight range. An entity already visible from the replay will not re-fire `EntityAppeared` from a subsequent move that stays within the viewer's LoS.

### Replay builder location

`BuildReplayEvents` and `TranslateSnapshot` live in `translate.go` alongside the live-event translators. Both are exported so unit tests in the external `encounter_test` package can exercise them without an integration harness. `ProjectFor` is called once in `StreamEncounter` and its result is passed to both, keeping the projection logic in a single place.

## Architecture notes

- Handler imports no orchestrator — direct toolkit dependency is intentional for v2.
- `encounter.Broker` is process-scoped (one broker per server process, shared across all v2 RPCs).
- Repository follows the standard Input/Output interface pattern but `Get` returns `*encounter.Data` directly (toolkit's native type is the domain entity here).
- Entity ID in CreateEncounter defaults to player ID for the initial seat; future PRs will wire in character selection.
- Reconnect semantics (replay based on `last_seen_sequence`) are out of scope for #497; that is a separate future slice.
