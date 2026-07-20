---
name: encounter (v1alpha2)
description: v1alpha2 encounter service — thin handlers over a clean load→verb→persist orchestrator
updated: 2026-05-31
confidence: high — verified by reading handler.go, end_turn.go, orchestrators/encounter/v2/{orchestrator,load,end_turn,take_action,move_entity,submit_check_reaction}.go, reaction_resume.go, .golangci.yml, combat_handlers_test.go
---

# encounter (v1alpha2)

The v1alpha2 encounter service is the second-generation encounter vertical. As of the #582 carve (Chapter 1, Architecture Honesty) it is split into two layers:

- **Handlers** (`internal/handlers/dnd5e/v2/encounter/`) do proto ↔ entity conversion only: validate the envelope (auth, required ids), build the orchestrator's entity-typed Input, delegate, and map the orchestrator's sentinel errors onto gRPC status codes. No business logic, no load/persist.
- **Orchestrator** (`internal/orchestrators/encounter/v2/`) is the single `load → toolkit-verb → persist` core — one method per RPC, each doing exactly one load + one toolkit-verb dispatch + one persist. It speaks entity / toolkit types only and NEVER imports proto (`pb.*`). This replaced the retired handler-package `Runner` + the scattered inline verb bodies.

The orchestrator stays **rulebook-free**. All game complexity lives in the rpg-toolkit encounter SDK + the dnd5e rulebook, reached only through injected adapters (the combat / movement resolver builders and the `ReactionResume` seam) built handler-side. A depguard lint rule (`no-rulebook-internals-in-action-handlers`) denies `rulebooks/dnd5e/{conditions,resources,classes,weapons,armor,combat}` in both the guarded handler verb files and the orchestrator method files; the resolver-adapter files (which legitimately translate held entities through the rulebook) are excluded and not in the guarded set.

## Files

| File | Purpose |
|---|---|
| `internal/handlers/dnd5e/v2/encounter/handler.go` | Handler struct + constructor (wires the orchestrator with the injected resolver builders, `CharacterDataCascade`, and `ReactionResume` funcs), MoveEntity, StreamEncounter |
| `internal/handlers/dnd5e/v2/encounter/{create,get,interact,take_action,submit_check,submit_check_reaction,set_reaction_ready,activate_feature,end_turn}.go` | Thin verb handlers — proto↔entity + delegate to the orchestrator + sentinel→gRPC mapping |
| `internal/handlers/dnd5e/v2/encounter/reaction_resume.go` | Rulebook-touching adapter seam (excluded from depguard): marshal/decode the opaque `*combat.AttackContext`, build reaction modifiers (Shield +5 AC), one-shot-reaction predicate — injected into the orchestrator's `ReactionResume` |
| `internal/handlers/dnd5e/v2/encounter/{dnd5e_combat_resolver,dnd5e_combat_resolver_phased,dnd5e_movement_resolver,combatant,hydrate_players}.go` | Resolver / cascade adapters (excluded from depguard) — translate held entities through the rulebook |
| `internal/handlers/dnd5e/v2/encounter/project.go` | ProjectFor helper — toolkit Data → proto Encounter |
| `internal/handlers/dnd5e/v2/encounter/translate.go` | Event translator — toolkit events → proto EncounterEvent |
| `internal/orchestrators/encounter/v2/orchestrator.go` | Orchestrator struct + `Config` (Broker, repo, resolver builders, `CharacterDataCascade`, `ReactionResume`) + `New` |
| `internal/orchestrators/encounter/v2/load.go` | The single load core: Get → auth (membership + optional entity-ownership) → #689 character-data attach → `LoadFromData` with resolvers wired uniformly. Defines `ErrEncounterNotFound` / `ErrPlayerNotInEncounter` / `ErrEntityOwnershipMismatch` |
| `internal/orchestrators/encounter/v2/{interact,take_action,move_entity,submit_check,submit_check_reaction,set_reaction_ready,activate_feature,end_turn}.go` | One orchestrator method per RPC |
| `internal/repositories/encounters/v2/repository.go` | Repository interface (Get, Save) |
| `internal/repositories/encounters/v2/in_memory.go` | In-memory implementation (JSON round-trip) |
| `internal/repositories/encounters/v2/redis.go` | Redis implementation |

## v2 RPCs

| RPC | Status | Wave |
|---|---|---|
| `CreateEncounter` | Implemented (#499) | 2.6 |
| `GetEncounter` | Implemented (#500) | 2.6 |
| `StreamEncounter` | Implemented (#496, replay #497) | 2.5 / 2.7 |
| `MoveEntity` | Implemented; orchestrator carve #582 step 6 | 2.5 |
| `Interact` | Implemented for doors (#504); orchestrator carve #582 | 2.7 |
| `TakeAction` | Implemented (two-phase); orchestrator carve #582 step 5 | 2.8 / 2.11d |
| `SubmitCheck` | Implemented (skill check + take_reaction branch); orchestrator carve #582 | 2.9 / 2.11d |
| `SetReactionReady` | Implemented; orchestrator carve #582 | 2.11d |
| `ActivateFeature` | Implemented; orchestrator carve #582 step 4 | 2.x |
| `EndTurn` | Implemented; orchestrator carve #582 step 7 (final verb) | 2.10 / 2.11d |

## Orchestrator method anatomy (the `load → verb → persist` shape)

Every orchestrator method follows the same skeleton (see `move_entity.go`, `take_action.go`, `end_turn.go`):

1. Nil-guard the `*Input` (never `(nil, nil)`).
2. `o.load(ctx, loadInput{...})` — Get + auth + (combat-capable verbs) the #689 character-data attach + `LoadFromData`. Auth runs before rehydration so the auth-fail path pays no rehydration cost. Returns ONLY the encounter (the synced state); there is no parallel `*Data` handle to drift.
3. Invoke the toolkit verb on the encounter. Toolkit gate sentinels (`ErrEncounterEnded`, `ErrNotTurnBased`, `ErrNotYourTurn`, `ErrNoCombatants`, `ErrUnsupportedAction`, `ErrUnknownTarget`, …) surface **unwrapped** so the handler maps each distinctly via `errors.Is`. State-dependent refusals that the toolkit returns as plain errors are joined with a verb-specific sentinel (e.g. `ErrMoveRefused`).
4. `o.persistWithCharacterData(ctx, enc, encID)` — `ToData` + `SyncErr` check + (combat-capable) flush the cascaded player `DataJSON` back to the character store + `Save`.

### EndTurn specifics (#582 step 7)

`Orchestrator.EndTurn` carries three concerns, all kept rulebook-free:

1. **NPC-dispatch loop** — after the toolkit `enc.EndTurn` advances initiative, if the new active actor is an NPC the orchestrator runs `enc.NPCAct` + `enc.EndTurn`-the-NPC server-side until a player is active, the encounter ends (`enc.Mode() == ModeEnded`), or the roster-derived cap (`len(enc.ToData().Initiative) + npcChainSafetyMargin`) is hit. The cap is read from the synced snapshot, not a side `*Data`.
2. **Turn-end reset** — clean post-#689: `enc.EndTurn(ctx, …)` emits the dnd5e `TurnEndTopic` on the encounter bus itself, so held conditions (SneakAttack, etc.) reset their per-turn state in place — no host-side publish, no character re-load.
3. **Pause-for-reaction bookkeeping** — when a dispatched NPC attack pauses (`IsNPCPausedForReaction`), the SDK has already persisted the pending prompt + published `InputRequiredDelivered` from inside `NPCAct`. The orchestrator drops all-but-one prompt (`enforceSingleReactor`, single-reactor enforcement per #538 C) and marshals the cached `*combat.AttackContext` into the prompt's `AttackContextJSON` via the **injected `ReactionResume.MarshalAttackContext`** (`serializeNPCPendingReactions`) — the SAME seam TakeAction phase-1 uses — then persists and returns success.

`ErrNPCChainExhausted` (no players in initiative) and `ErrNPCAct` (system-shaped NPCAct failure) are the orchestrator-specific sentinels; the handler maps the former to `FailedPrecondition` and the latter to `Internal`. `ErrNPCChainExhausted` is a defensive guard — unreachable through the player-gated entrypoint (the first `EndTurn` requires the player to be active, so the player is always in initiative and the loop re-reaches them on wrap).

### Reaction resume seam (`reaction_resume.go`)

The two-phase attack (TakeAction phase-1 pause → SubmitReactionCheck phase-2 resume, and the NPC-pause serialization) needs the rulebook's opaque `*combat.AttackContext` marshaled/decoded and the rule-ish reaction magnitudes (Shield = +5 AC). These live in `reaction_resume.go` (handler-side, depguard-excluded) and are injected into the orchestrator as the `ReactionResume` struct: `MarshalAttackContext`, `DecodeAttackContext`, `BuildReactionModifiers`, `IsOneShotReaction`. The orchestrator never type-asserts the rulebook payload — it calls the injected funcs.

**Deferred:** the cross-RPC reaction *wire-pause* is NOT built. NPC reaction pauses resolve through the existing single-RPC `SubmitReactionCheck`; the reaction step stays internal.

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
    ctx context.Context,
    data *tkenc.Data,
    viewer core.PlayerID,
    broker *tkenc.Broker,
    charRepo characterrepo.Repository,
    now time.Time,
) (*encounterv2pb.Encounter, error)
```

It is the single source of truth for toolkit Snapshot → proto Encounter translation. Called by `CreateEncounter` and `GetEncounter`; snapshot replay (#497) will reuse it without duplicating the projection logic.

### Entity visibility in ProjectFor (#500 broadening)

As of #500, `ProjectFor` includes all entities currently visible to the viewer — not just the viewer's own entity. Visibility is computed using `perception.VisibleHexesAt(viewer.Position, viewer.SightRange)` against each other player's current position in `data.Players`. Entities are sorted by player ID for deterministic wire output. This gives clients a real point-in-time snapshot of what the viewer can see.

### Wall + door projection (The Dungeon wave 2 Slice 2, rpg-api#676)

`Space.Walls` carries TWO shapes now, both whole-room/unconditional (not LoS-gated like `Hexes`/`Entities` — wave 1's "no per-viewer wall reveal yet" still holds): `wallsToProto(data.Space)` projects persisted solid/window segments (`Start == End` degenerate hexes), and `doorWallsToProto(data)` separately projects every entry in `data.Doors` as a `WALL_KIND_DOOR_CLOSED`/`WALL_KIND_DOOR_OPEN` `Wall` carrying its entity id (`rpg-api-protos#186`'s additive `Wall.id` — the web's click→`Interact(id)` bridge). A door's `From` is its own cell; `To` is its passage-edge neighbor (`doorPassageNeighbor`, found via `perception.HexNeighbors` + `SpaceData.RegionAt` — never `Start == End` for a door, which is the 6-door-pair render-multiplicity bug the design doc's §Q2 calls out). `DoorData` stays the single source of door truth (position, open/locked state); the wall is purely its projected geometry.

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

### Reaction conditions (post-#689: toolkit-cascade owned)

The standalone `reaction_conditions.go` is **gone**. rpg-api no longer constructs `conditions.New*` for player OA/Shield: the #689 hydration cascade holds each combatant's `*character.Character` (with its conditions already applied) on the encounter bus exactly once per RPC, so player reaction conditions ride that held instance rather than being re-applied handler-side. This was the cure for the #684 double-subscribe class. The cascade is plumbed via `CharacterDataCascade` (attach before `LoadFromData`, persist after `ToData`), injected into the orchestrator handler-side.

The one survivor is `applyMonsterReactionConditions` (in `dnd5e_movement_resolver.go`, a depguard-excluded adapter), which wires `OpportunityAttackCondition` on monsters at rehydration time — without it, NPC-direction OA never fires because the condition's bus subscription never installs. Its predicate gates on `IsReactionReady`, which the encounter SDK's `seedOAReadiness` seeds to `true` for every combatant with `DamageDice` set at `AddPlayer`/`AddMonster`.

**Not applied**: `DisengagingCondition`. Its predicate has no "activated this turn" gate — applying universally would suppress OAs for everyone. Disengage must go through `combatabilities.Disengage.Activate` on the encounter's bus, which applies the condition itself. Cross-RPC persistence (condition applied in TakeAction, needed in MoveEntity) is a follow-up.

### Known gaps / deferred scope

| Gap | Filed as |
|---|---|
| Disengage cross-RPC persistence — condition applied in TakeAction evaporates when MoveEntity calls `LoadFromData` with a fresh bus | Future follow-up |
| Player-pause reactions (Sentinel, reactions-during-movement) | rpg-toolkit#665 |
| Weapon-aware OA attacker — `triggerOpportunityAttack` uses `weapons.UnarmedStrike` placeholder | toolkit#NEW (file separately) |
| `v2 ActivateCombatAbility` RPC for Disengage | Future wave |

## Architecture notes

- As of #582 the action-verb handlers delegate to the `internal/orchestrators/encounter/v2` orchestrator (one `load → verb → persist` method per RPC); the handler-package `Runner` + inline verb bodies are retired. `CreateEncounter` / `GetEncounter` / `StreamEncounter` still touch the repo + `ProjectFor` directly (read/projection paths, not action verbs). The orchestrator imports the toolkit but NEVER proto; the handlers own all proto↔entity conversion.
- The orchestrator stays rulebook-free — rulebook-touching work (resolver translation, the `*combat.AttackContext` marshal/decode, reaction magnitudes, the #689 character-blob cascade) lives in handler-side adapter funcs injected via `Config`, and depguard enforces no `rulebooks/dnd5e/{conditions,resources,classes,weapons,armor,combat}` import in the guarded handler + orchestrator files.
- `encounter.Broker` is process-scoped (one broker per server process, shared across all v2 RPCs).
- Repository follows the standard Input/Output interface pattern but `Get` returns `*encounter.Data` directly (toolkit's native type is the domain entity here).
- Entity ID in CreateEncounter defaults to player ID for the initial seat; future PRs will wire in character selection.
- Reconnect semantics (replay based on `last_seen_sequence`) are out of scope for #497; that is a separate future slice.
