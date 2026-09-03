---
name: rpg-api architecture overview
description: Request flow, layer rules, component map, and cross-repo boundaries for rpg-api
updated: 2026-09-04
confidence: medium-high — #897 refreshed character Appearance ownership, conversion, and storage boundaries; older component debt retains its stated caveats
---

# rpg-api architecture overview

rpg-api is a Go gRPC server that orchestrates game state for a multiplayer D&D 5e dungeon crawler. Its single mandate is *data orchestration*: load entities from repositories, pass them to rpg-toolkit for rule execution, persist the results, and publish events to connected clients. rpg-api never knows what Rage does, never calculates attack modifiers on its own, and never implements D&D rules. If game logic appears here, that is a defect — the missing helper belongs in rpg-toolkit.

## Request flow

**Updated 2026-08-28 (rpg-api#852):** the active game stack is `LobbyService`
launching into `SessionService`. `SessionPresentationService` follows the same
handler → orchestrator → repository layering for live-only dice choreography,
but deliberately stops at Redis Pub/Sub: it never calls the toolkit session
manager and never appends to Story.

```
gRPC client (rpg-dnd5e-web or Discord Activity)
    │
    ▼
gRPC interceptors  (cmd/server/server.go)
    auth.UnaryAuthInterceptor + auth.StreamAuthInterceptor
    extracts Discord token → validates → injects playerID into context
    │
    ▼
Handler  (internal/handlers/dnd5e/v1alpha1/<domain>/handler.go)
    proto → domain entity conversion
    input validation with status codes
    auth context extraction (auth.PlayerIDFromContext)
    delegate to orchestrator
    domain entity → proto conversion
    │
    ▼
Orchestrator  (internal/orchestrators/<domain>/orchestrator.go)
    business logic coordination using internal entity types ONLY
    composes repos + toolkit calls
    │
    ├─→ Repository  (internal/repositories/<domain>/)
    │       Input/Output types on every method
    │       returns domain entities, never DB models
    │
    └─→ rpg-toolkit  (github.com/KirkDiggler/rpg-toolkit/...)
            rule engine: combat resolution, initiative, abilities,
            character rules
```

## Layer rules

### Handlers
- **Do:** proto ↔ entity conversion, input validation, auth context extraction.
- **Do not:** business logic, game rules, toolkit calls that are not conversions.
- One handler file per service version, one converter file per service.
- Accept proto request → call `auth.PlayerIDFromContext` → build input struct → call orchestrator → convert output → return proto response.

~~**Current violation in encounter handler**~~ RESOLVED by deletion (rpg-api#642,
2026-07-13): `internal/handlers/dnd5e/v1alpha1/encounter/handler.go` — the file
this violation described — is deleted along with the rest of the v1alpha1
EncounterService.

### Orchestrators
- **Do:** coordinate data: load from repos, call toolkit, persist results, emit events.
- **Do not:** import or construct proto types. No `pb.` references. Speak internal entity types exclusively.
- Input/Output structs on every method (defined in `service.go` alongside the interface).

~~**Current violation in encounter orchestrator**~~ / ~~**encounter service
types**~~ / ~~**entities package**~~ / ~~**encounter_events.go**~~ ALL RESOLVED
by deletion (rpg-api#642, 2026-07-13): `internal/orchestrators/encounter/
orchestrator.go`, `service.go`, `internal/entities/entity_state.go`,
`internal/entities/encounter_state_builder.go`, and `internal/entities/
encounter_events.go` — every file these four violations named — are deleted.
The v2 orchestrator (`internal/orchestrators/encounter/v2/`) never imports
proto; see [`components/encounter.md`](./components/encounter.md).

### Repositories
- **Do:** data access abstraction with Input/Output types on every method.
- **Do not:** expose DB-layer models, return `(nil, nil)`.
- Interfaces defined in `repository.go` alongside Input/Output types.
- Implementations in same package (`redis.go`, `inmemory.go`).

**Updated 2026-07-13 (rpg-api#642):** Character, character_draft, and
dice_session repositories have Redis implementations, and so does
`internal/repositories/encounters/v2/` (24h TTL) and `internal/repositories/
lobby/`. The v1-only in-memory-only repositories this section used to flag —
`internal/repositories/encounters` (v1 root), `internal/repositories/dungeons`,
`internal/repositories/encounterlog` — are all deleted.

### Components (`internal/components/`)
Components are local prototypes pending graduation to rpg-toolkit. They implement game logic that rightfully belongs in the rules engine but has not yet been extracted.

- `internal/components/dungeon/` — procedural dungeon generation (room shapes, wall perimeters, door spawning, hex-grid layouts, monster CR budgets, theme tables). **This is game logic in the wrong repository.** The Boundary Rule is explicit: game mechanics belong in rpg-toolkit. **Updated 2026-07-13:** deliberately left untouched by #642 (Kirk decides relocation timing), but its only remaining caller anywhere in the codebase is `internal/components/spawner` — see the load-bearing finding in [`components/dungeon-component.md`](./components/dungeon-component.md): it now has zero production (handler/orchestrator) callers.
- `internal/components/spawner/` — thin adapter around toolkit placement logic. Delegates rather than implements rules; lower risk. Same "zero production callers" note applies.

## Component map

| Component | Path | Purpose | Notes |
|---|---|---|---|
| ~~Encounter v2 handler~~ | `internal/handlers/dnd5e/v2/encounter/` | DELETED | See `components/encounter.md` |
| Character handler | `internal/handlers/dnd5e/v1alpha1/character/` | gRPC ↔ character orchestrator | 20 TODO stubs in converters |
| Dice handler | `internal/handlers/api/v1alpha1/dice_handler.go` | gRPC ↔ dice orchestrator | Simple; no known issues |
| Session handler | `internal/handlers/dnd5e/session/v1alpha1/` | gRPC ↔ toolkit session manager | Gameplay verbs, Story, view, roster |
| Session presentation handler | `internal/handlers/dnd5e/sessionpresentation/v1alpha1/` | gRPC ↔ presentation orchestrator | Live-only shared dice plans; see `components/session-presentation.md` |
| ~~Encounter v2 orchestrator~~ | `internal/orchestrators/encounter/v2/` | DELETED | See `components/encounter.md` |
| Character orchestrator | `internal/orchestrators/character/` | Character creation, equipment | Smaller, well-tested |
| Dice orchestrator | `internal/orchestrators/dice/` | Dice rolling sessions | Thin; low risk |
| Dungeon component | `internal/components/dungeon/` | Procedural dungeon generation | Wrong repo (audit debt #5, untouched by #642); zero production callers as of #642 |
| Spawner component | `internal/components/spawner/` | Entity placement adapter | Thin; delegates to toolkit; zero production callers as of #642 |
| Auth | `internal/auth/` | Discord token validation + caching | Dev mode for local/tests |
| Entities | `internal/entities/` | Thin wrappers around toolkit Character/Data and DraftData | Proto-free; Appearance is nested in toolkit data |
| ~~Encounter repo v2~~ | `internal/repositories/encounters/v2/` | DELETED | See `components/encounter.md` |
| Character repo | `internal/repositories/character/` | Character persistence | Redis — only durable store predating the v2 vertical |
| Character draft repo | `internal/repositories/character_draft/` | Draft character state | Redis |
| Dice session repo | `internal/repositories/dice_session/` | Dice roll sessions | Redis |
| SandboxRoom service | `internal/services/sandboxroom/` | Room generation interface | Interface only; no implementation wired |
| Integration test harness | `internal/integration/harness/` | Full-stack test server with real Redis | Most valuable test asset |
| Lobby handler | `internal/handlers/dnd5e/lobby/v1alpha1/` | gRPC ↔ lobby orchestrator | New 2026-07-07 (rpg-api#629); layered from day one |
| Lobby orchestrator | `internal/orchestrators/lobby/` | Party assembly + sole encounter construction (`StartEncounter`) | New; see `docs/architecture/components/lobby-service.md` |
| Session presentation orchestrator | `internal/orchestrators/sessionpresentation/` | Validate/bind presentation plans | Proto-free; no toolkit calls |
| Lobby repo | `internal/repositories/lobby/` | Lobby persistence | Redis + in-memory |
| Session presentation repo | `internal/repositories/sessionpresentation/` | Accepted plan payloads + Redis Pub/Sub | Live-only, 2-minute duplicate/conflict keys |

~~Encounter handler (v1alpha1)~~ / ~~Encounter orchestrator (v1alpha1)~~ /
~~Event processor~~ / ~~Redis publisher~~ / ~~Encounter repo (v1)~~ / ~~Dungeon
repo~~ / ~~Encounter log repo~~ — all DELETED (rpg-api#642, 2026-07-13).

## Cross-repo boundaries

**What rpg-api accepts from clients (rpg-dnd5e-web):**
- References (IDs): character IDs, encounter/session IDs, join codes, dungeon IDs, connection IDs, presentation IDs.
- Intent: action requests with resource references (`weaponID`, `featureID`) and bounded presentation dice plans, never calculations or combat outcomes.
- Auth: Discord JWT tokens validated via Discord API.

**What rpg-api asks rpg-toolkit:**
- Combat resolution, initiative, monster turns, movement/opportunity attacks, Story, view, atlas, doors, and roster-adjacent session reads — via `rulebooks/dnd5e/session.Manager` from the session handler/lobby launch path.
- Character rules: ability score calculation, proficiency bonuses, spell slots — these come from toolkit types.
- Spatial: entity placement, movement validation, line-of-sight via the toolkit session/encounter packages.
- Nothing for SessionPresentationService: it validates presentation payload shape locally, stores/fans out through Redis, and never asks the toolkit to mutate or narrate game truth.
- ~~Dungeon room generation: room shapes, perimeter walls, feature layouts via `components/dungeon`~~ — component still exists (untouched by #642) but has zero production callers as of this PR; see `components/dungeon-component.md`.

**What rpg-api persists:**
- Thin `entities.Character`/`CharacterDraft` wrappers serialized to Redis; toolkit `Data`/`DraftData`, including nested Appearance, own character state.
- Toolkit session/encounter state owned by `rulebooks/dnd5e/session.Manager` — Redis-backed through `internal/orchestrators/session`, 24h TTL.
- Lobby state — Redis-backed via `internal/repositories/lobby`.
- Session roster — owned and projected by the toolkit `session.Manager`; SessionService and the shared access gate read it through the SDK.
- Session presentation plans — Redis-backed ephemeral duplicate/conflict keys (2-minute TTL) plus live Pub/Sub; not replayable Story.
- ~~`EncounterData` (local type)~~ / ~~`Dungeon` (local entity)~~ / ~~`EncounterEvent` (local entity)~~ — all DELETED (rpg-api#642); these were the v1alpha1 encounter stack's storage types.

**What rpg-api publishes:**
- `SessionService.StreamEvents` fans out the toolkit session SDK's live per-recipient events; `GetStory` is the persisted catch-up read.
- `SessionPresentationService.StreamDiceThrows` fans out bounded decorative dice plans through Redis Pub/Sub only. It is live-only and does not write Story.
- ~~Redis pub/sub channel `encounter:{id}:events`~~ — DELETED (rpg-api#642); this was the v1alpha1 `EncounterEvent` publisher.

## gRPC service versions

- `dnd5e.api.v1alpha1` — CharacterService only as of rpg-api#642 (2026-07-13):
  the v1alpha1 EncounterService is unregistered and deleted — see
  `docs/status.md` "Active work".
- `api.v1alpha1` — DiceService
- `dnd5e.api.session.v1alpha1` — SessionService, the gameplay/session SDK wire surface.
- `dnd5e.api.session.presentation.v1alpha1` — SessionPresentationService, live-only shared dice presentation; health service name matches this string exactly.
- ~~`dnd5e.api.v1alpha2.encounter`~~ — deleted with the old encounter stack.
- `dnd5e.api.lobby.v1alpha1` — LobbyService (new 2026-07-07, rpg-api#629). Own version
  clock, deliberately not tied to the encounter service's — see
  `docs/architecture/components/lobby-service.md`.

Proto definitions are in `rpg-api-protos` (separate repo). Compiled Go code pinned via `@generated` branch. This doc's service-version list otherwise predates the v2 encounter stack (last full refresh 2026-05-02) — treat the table above as current, the rest of this file as historical context pending a fuller refresh.

## Known architectural debt (in priority order)

Items 1–5 below (the entire v1alpha1 encounter stack's boundary debt) are
**RESOLVED by deletion, rpg-api#642, 2026-07-13** — the files that carried
these violations no longer exist. Left listed for the historical record.

1. ~~**Proto types in service interface**~~ (`orchestrators/encounter/service.go`) — DELETED.
2. ~~**Proto types in orchestrator**~~ (`orchestrators/encounter/orchestrator.go`) — DELETED.
3. ~~**Proto types in entities**~~ (`entities/entity_state.go`, `entities/encounter_state_builder.go`, `entities/encounter_events.go`) — DELETED.
4. ~~**Coordinate transform fragility**~~ — the orchestrator and handler converters with the ad-hoc fix sites are DELETED. If the v2 path develops coordinate bugs, that's fresh debt, not a continuation.
5. ~~**In-memory repositories**~~ — the v1 encounter/dungeon/encounter-log repos are DELETED; the surviving `encounters/v2` repo is Redis-backed from day one.
6. **Dungeon component in wrong repo** — `components/dungeon/` implements game logic that belongs in rpg-toolkit. Still open, deliberately untouched by #642 (Kirk decides relocation timing) — but now has zero production callers; see `components/dungeon-component.md`.
