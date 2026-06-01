---
name: encounter (v1alpha2)
description: v1alpha2 encounter service — lightweight toolkit adapter, no orchestrator
updated: 2026-05-25
confidence: high — verified by reading handler.go, dnd5e_movement_resolver.go, dnd5e_combat_resolver.go, hex_room.go, reaction_conditions.go, integration_movement_oa_test.go
---

# encounter (v1alpha2)

The v1alpha2 encounter service is the second-generation encounter handler. Unlike the v1alpha1 handler which delegates through a full orchestrator stack, v2 talks directly to the rpg-toolkit encounter SDK and a v2 repository. The handler is a thin wire adapter — no business logic, no orchestrator.

## Files

| File | Purpose |
|---|---|
| `internal/handlers/dnd5e/v2/encounter/handler.go` | Handler struct, constructor, MoveEntity (thin delegate → v2 orchestrator `move_entity.go`, #582 step 6), StreamEncounter |
| `internal/handlers/dnd5e/v2/encounter/create.go` | CreateEncounter RPC |
| `internal/handlers/dnd5e/v2/encounter/get.go` | GetEncounter RPC |
| `internal/handlers/dnd5e/v2/encounter/interact.go` | Interact RPC (Wave 2.7 — door interactions) |
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
| `Interact` | Implemented for doors (#504) | 2.7 |

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

## Interact flow (Wave 2.7)

1. Validate auth — `auth.GetPlayerID(ctx)` empty → `Unauthenticated`.
2. Validate request — `encounter_id` or `target_entity_id` empty → `InvalidArgument`.
3. Load encounter via `h.encRepo.Get`; `ErrNotFound` → `NotFound`.
4. Dispatch by target — Wave 2.7 only wires doors. `data.Doors[targetID]` lookup; not present → `NotFound` ("target entity is not a door, or door does not exist").
5. `encounter.LoadFromData(data, h.broker)` → rehydrate; error → `Internal`.
6. `enc.OpenDoor(playerID, doorID)` — toolkit publishes `DoorOpenedEvent` plus a parallel `HexRevealedEvent` for any viewer whose vision grew. Toolkit errors (player not in encounter, door already open) → `FailedPrecondition` per `pat-v2-status-code-mapping`.
7. `h.encRepo.Save(ctx, enc.ToData())` → `Internal` on error.
8. Return `&InteractResponse{}` (empty — door world changes flow as events). `InputRequired` is reserved for Wave 2.10 locked-door skill checks.

`interaction_kind` is plumbed through the proto for future routing (`examine`, `loot`, `disarm`) but unused in Wave 2.7. Future waves extend the dispatch with chests, levers, NPCs, traps.

### Cause/effect event split

The translator emits BOTH `DoorOpened` and `GeometryRevealed` as separate proto envelopes. The `DoorOpened.revealed_hexes`, `revealed_walls`, `removed_walls` fields are deliberately empty — the parallel `HexRevealedEvent` (translated to `GeometryRevealed`) carries the geometry delta. This mirrors the toolkit's cause/effect decomposition (see `rpg-toolkit/encounter/events/hex_revealed.go` for the rationale). Web composes the visual response from the two envelopes.

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

## MovementResolver wiring (Wave 2.11e #539)

`Dnd5eMovementResolver` implements the encounter SDK's `tkenc.MovementResolver` interface. It is wired into every `LoadFromData` + `New` call site via `encounter.WithMovementResolver(h.buildMovementResolver(data))`.

### Per-request lifecycle

`buildMovementResolver(data)` constructs a fresh `Dnd5eMovementResolver` bound to the current encounter's `*tkenc.Data`. This is intentional: the resolver needs the live entity positions to construct the spatial room for each step, and the encounter data is already loaded at each LoadFromData site.

### ResolveStep chain

For each atomic hex step the encounter SDK invokes `ResolveStep(input)`:

1. Classify the mover: `classifyEntityType(entityID)` checks `data.Players` and `data.Monsters` maps — "character" for players, "monster" for NPCs.
2. Build `encounterHexRoom` from current data positions. This room is both the read source for current positions and the write target for per-step mutations (`MoveEntity` on the room updates `data.Players[*].View.Position` or `data.Monsters[*].Position` in-place).
3. Build `CombatantRegistry` covering all players (loaded via `CharacterRepo`) and monsters (rehydrated from `DataJSON` via the combat resolver). Registered in context via `combat.WithCombatantLookup`.
4. Build `gamectx` context: `combat.WithRoom`, `gamectx.WithGameContext`, `gamectx.WithRoom`, `gamectx.WithReactionReadiness` (reaction readiness map from encounter data).
5. Call `combat.MoveEntity` with a single-hex path. The toolkit publishes `MovementChain`; condition subscribers fire:
   - `DisengagingCondition` (if Activate'd this turn) adds `OAPreventionSources`, suppressing the OA trigger.
   - `OpportunityAttackCondition` checks `isLeavingMyThreatRange` + `IsReactionReady` — if both match, publishes `ReactionTriggerEvent` and fires `triggerOpportunityAttack` inline → `combat.ResolveAttack` → damage applied.
6. Translate `MoveEntityResult → MovementStepResult`: `MovementStopped` → `Prevented`; `StopReason` → `PreventReason`.

### `encounterHexRoom` (`hex_room.go`)

Implements `spatial.Room` over the encounter's `*tkenc.Data`. Key methods:

- `GetEntityPosition` / `GetEntitiesInRange` / `GetAllEntities` — read current positions from `data.Players[*].View.Position` and `data.Monsters[*].Position`.
- `MoveEntity` — mutates positions in-place so successive `ResolveStep` calls see updated state within the same RPC.
- `GetGrid()` returns `SquareGrid` (Chebyshev distance). This is required because `OpportunityAttackCondition.isLeavingMyThreatRange` calls `room.GetGrid().Distance(...)`, and `HexGrid` expects offset coordinates while our positions carry axial values (Q→X, R→Y). SquareGrid + adjacent hexes (axial distance 1) gives correct D&D 5e adjacency.

### Reaction conditions (`reaction_conditions.go`)

`applyReactionConditions` and `applyMonsterReactionConditions` are called on every character/monster rehydration (inside the combat resolver at entity-load time). They wire:

- `OpportunityAttackCondition` on every melee combatant (character + monster). The condition's predicate gates on `IsReactionReady` — the encounter SDK's `seedOAReadiness` seeds this to `true` for every combatant with `DamageDice` set at `AddPlayer`/`AddMonster`.
- `ShieldSpellCondition` on characters with at least one 1st-level spell slot (heuristic gate).
- **Not applied**: `DisengagingCondition`. Its predicate has no "activated this turn" gate — applying universally would suppress OAs for everyone. Disengage must go through `combatabilities.Disengage.Activate` on the encounter's bus, which applies the condition itself. Cross-RPC persistence (condition applied in TakeAction, needed in MoveEntity) is a follow-up.

### Known gaps / deferred scope

| Gap | Filed as |
|---|---|
| Disengage cross-RPC persistence — condition applied in TakeAction evaporates when MoveEntity calls `LoadFromData` with a fresh bus | Future follow-up |
| Player-pause reactions (Sentinel, reactions-during-movement) | rpg-toolkit#665 |
| Weapon-aware OA attacker — `triggerOpportunityAttack` uses `weapons.UnarmedStrike` placeholder | toolkit#NEW (file separately) |
| `v2 ActivateCombatAbility` RPC for Disengage | Future wave |

## Architecture notes

- Handler imports no orchestrator — direct toolkit dependency is intentional for v2.
- `encounter.Broker` is process-scoped (one broker per server process, shared across all v2 RPCs).
- Repository follows the standard Input/Output interface pattern but `Get` returns `*encounter.Data` directly (toolkit's native type is the domain entity here).
- Entity ID in CreateEncounter defaults to player ID for the initial seat; future PRs will wire in character selection.
- Reconnect semantics (replay based on `last_seen_sequence`) are out of scope for #497; that is a separate future slice.
