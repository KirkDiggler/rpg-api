---
name: rpg-api data model
description: Entities, relationships, storage schemas, and known gaps in the data layer
updated: 2026-09-06
confidence: medium-high — #921 toolkit composition storage and Character/CharacterDraft Appearance storage are verified by focused Redis tests; the historical v1 encounter/dungeon model remains deleted
---

# rpg-api data model

**Stale-content notice (2026-07-13, rpg-api#642):** this doc originally described
six domain entities. Three of them — `Dungeon`, `Encounter`/`EncounterData`, and
`EncounterEvent` — belonged to the v1alpha1 encounter stack, which is now deleted
in full (registered service, 5,844-line orchestrator, entities, repos, publisher,
event processor — see `docs/status.md` "Active work"). The sections below for
those three entities are left in place but marked DELETED rather than removed
outright, so this doc keeps a record of what the old shape was. **A fresh
description of the v2 encounter data model (`rpg-toolkit`'s `encounter` package +
`internal/repositories/encounters/v2`) has not been written here — that's a
follow-up, not done in this PR.** Character, CharacterDraft, and DiceSession are
unaffected and their sections below are current.

## Entity overview (pre-#642 shape — Dungeon/EncounterData/EncounterEvent rows are historical)

```
Character ─────────────────── owned by character repo (Redis)
    │ referenced by ID in
    ▼
Player (embedded in EncounterData)
    │ M:M
    ▼
EncounterData ─────────────── DELETED (rpg-api#642) — owned by encounter repo (in-memory)
    │ 1:1
    ▼
Dungeon ────────────────────── DELETED (rpg-api#642) — owned by dungeon repo (in-memory)
    └── Rooms (map[string]*dungeon.Room)
    └── Connections ([]*environments.ConnectionEdge)
    └── RoomOrigins (map[string]dungeon.AbsolutePosition)

EncounterEvent ─────────────── DELETED (rpg-api#642) — owned by encounter log repo (in-memory, append-only)

CharacterDraft ─────────────── owned by character_draft repo (Redis)

DiceSession ────────────────── owned by dice_session repo (Redis)

composition.Data ───────────── toolkit-owned opaque JSON snapshot, scoped by WorldID (Redis)
```

The v2 encounter path's data model (encounter state owned by
`internal/repositories/encounters/v2`, Redis-backed with a 24h TTL) is not
diagrammed here yet — follow-up.

## Character (`entities/character.go` — wraps toolkit type)

The canonical character state is `*character.Data` from `rpg-toolkit/rulebooks/dnd5e/character`. rpg-api does not maintain a separate character struct — it stores and retrieves the toolkit type directly.

Key fields of `character.Data` (toolkit-owned):
- `ID`, `Name`, `ClassID`, `RaceID`, `Level`
- `HitPoints`, `MaxHitPoints`
- `AbilityScores` — STR/DEX/CON/INT/WIS/CHA
- `EquipmentSlots` — typed slot map (main hand, off hand, armor, etc.)
- `Conditions` — `[]json.RawMessage` (serialized condition objects from toolkit)
- `DeathSaveState` — successes/failures/stabilized/dead
- `Features` — class features (rage uses, second wind, etc.)

**Appearance** is `customization.Appearance` nested in toolkit `character.Data` and
`character.DraftData`. The API has no separate Appearance/Hair/Style entity; Redis
stores it as part of the thin `entities.Character`/`CharacterDraft` wrapper's nested
`Data` value. The shared converter preserves optional and malformed wire shape while
toolkit owns semantic validation.

**Storage:** Redis key `character:{id}` (verified via `repositories/character/redis.go`). No TTL observed — characters persist indefinitely.

## CharacterDraft (`entities/character_draft.go`)

In-progress character creation state. Wraps partial character choices before finalization.

Key fields:
- `ID`, `PlayerID`, `CreatedAt`, `UpdatedAt`
- `Name`, `RaceID`, `ClassID`, `BackgroundID`
- `AbilityScores` — pending assignment
- `ChoicesCompleted` — which selection steps are done

**Storage:** Redis-backed via `repositories/character_draft/redis.go`. Represents transient state during character creation flow.

## ~~Dungeon (`entities/dungeon.go`)~~ DELETED (rpg-api#642, 2026-07-13)

Represents a complete dungeon run with exploration state.

Key fields:
```go
type Dungeon struct {
    ID          string
    EncounterID string  // links to the associated encounter

    // Toolkit connection graph
    Connections []*environments.ConnectionEdge
    StartRoomID string
    BossRoomID  string

    // Component room content (D&D 5e encounters, walls, features)
    Rooms       map[string]*dungeon.Room   // dungeon component type

    // Room positions in dungeon-space (canonical absolute cube coords)
    RoomOrigins map[string]dungeon.AbsolutePosition

    // Exploration state
    State         DungeonState    // Active/Victorious/Failed/Abandoned
    CurrentRoomID string
    RevealedRooms map[string]bool // room ID → explored
    OpenDoors     map[string]bool // connection ID → open

    Metrics: RoomsCleared, MonstersKilled, CreatedAt, CompletedAt
}
```

**Coordinate model (resolved in #471):** `dungeon.LocalPosition` and `dungeon.AbsolutePosition` are distinct integer cube types in `internal/components/dungeon/coords.go`. `dungeon.Module` (`module.go`) holds the per-room origin map and bridges local-to-absolute via `LocalToAbsolute(roomID, LocalPosition) AbsolutePosition`. All five transform sites that previously did inline casts (and produced bugs in PRs #459, #461, #463, #466, #467) now route through Module. The previous `dungeon.Position` (int) and `entities.Position` (float64) types were deleted.

**Storage (historical):** was in-memory only via `repositories/dungeons/inmemory.go` —
both the type and the repository are deleted.

## ~~Encounter / EncounterData (`entities/encounter.go` + `repositories/encounters/repository.go`)~~ DELETED (rpg-api#642, 2026-07-13)

The encounter is the central aggregate for a combat session. It owns player state, initiative, entity positions, and dungeon references.

`EncounterData` (repository storage type):
```go
type EncounterData struct {
    ID             string
    RoomData       interface{}  // spatial.RoomData — stored as interface{} (type fix pending)
    InitiativeData *initiative.TrackerData  // toolkit type
    InitiativeRolls []initiative.Roll
    MovementRemaining int32
    ActionEconomy  *entities.ActionEconomyState

    // DEPRECATED fields (migrating to Entities)
    Monsters    []*monster.Data
    CharacterHP map[string]int

    // Unified entity state map (current approach)
    Entities    map[string]*entities.EntityStateData

    // Multiplayer
    State    EncounterState  // waiting/active/paused/completed
    JoinCode string
    HostID   string
    Players  map[string]*Player

    LastEventID string  // ULID of most recent event (for load-then-stream sync)
    CreatedAt   time.Time
}
```

**DEPRECATED fields:** `Monsters []*monster.Data` and `CharacterHP map[string]int` are present but deprecated in favor of `Entities map[string]*EntityStateData`. Both migration paths are in use simultaneously — this is a debt item.

**RoomData as `interface{}`:** `RoomData interface{}` is a deliberate placeholder comment-noted in the repository as "Temporarily using interface{} until spatial is fixed." This type erasure makes the room data opaque to the repository and requires type assertions throughout the orchestrator.

**Storage (historical):** was in-memory only via `repositories/encounters/inmemory.go`
(the v1 root) — both the type and that repository are deleted. The surviving
`internal/repositories/encounters/v2/` is a different, Redis-backed store for the
v2 encounter path's own data shape (from `rpg-toolkit`'s `encounter` package),
not described here yet.

## ~~EntityStateData (`entities/entity_state.go`)~~ DELETED (rpg-api#642, 2026-07-13)

Unified per-entity state used in the `Entities` map. Holds everything needed to render an entity in the game UI.

```go
type EntityStateData struct {
    EntityID   string
    EntityType string   // "character", "monster", "obstacle"
    RoomID     string
    Position   *Position
    Size       int
    Appearance *Appearance  // nil for non-characters

    ToolkitData any   // *character.Data or *monster.Data or nil for obstacles
}
```

**Note (historical):** `entity_state.go` also contained `ToEntityStateProto` and
`ToEntityStateProtos` — functions that constructed proto messages inside the
entities package, a boundary violation this doc used to flag. Moot: the whole
file is deleted.

## ~~EncounterEvent (`entities/encounter_events.go`)~~ DELETED (rpg-api#642, 2026-07-13)

The event envelope for all encounter state changes. Uses a discriminated-union (oneof pattern): one struct, all event variants as optional fields.

```go
type EncounterEvent struct {
    ID          string      // ULID
    Type        EventType   // string constant
    EncounterID string
    Timestamp   time.Time

    // Only one of these is populated per event:
    PlayerJoined       *PlayerJoinedEvent
    PlayerLeft         *PlayerLeftEvent
    // ... 15 event types total
    RoomRevealed       *RoomRevealedEvent
}
```

Event types (15 total):
- **Player lifecycle:** `player_joined`, `player_left`, `player_ready`, `player_disconnected`, `player_reconnected`
- **Combat lifecycle:** `combat_started`, `combat_ended`, `combat_paused`, `combat_resumed`
- **Combat actions:** `movement_completed`, `attack_resolved`, `feature_activated`, `turn_ended`, `monster_turn_completed`
- **Dungeon lifecycle:** `dungeon_victory`, `dungeon_failure`, `room_revealed`

**Known proto contamination (historical, now moot):** several event structs
embedded `*dnd5ev1alpha1.EncounterStateData`, `*dnd5ev1alpha1.EntityState`, and
`*dnd5ev1alpha1.CombatState` as `json:"-"` fields — a boundary violation this doc
used to flag. Moot: the whole file is deleted.

**Storage (historical):** was append-only in memory via
`repositories/encounterlog/inmemory.go` — both the type and that repository are
deleted.

## ~~CombatState / ActionEconomyState (`entities/encounter.go`)~~ DELETED (rpg-api#642, 2026-07-13)

Local entity tracking combat turn state. These are separate from the toolkit's initiative tracker.

```go
type CombatState struct {
    EncounterID       string
    Round             int
    TurnOrder         []InitiativeEntry
    ActiveIndex       int
    MovementRemaining int32
    ActionEconomy     *ActionEconomyState
    CombatStarted     bool
    CombatEnded       bool
}

type ActionEconomyState struct {
    ActionsRemaining        int
    BonusActionsRemaining   int
    ReactionsRemaining      int
    AttacksRemaining        int
    OffHandAttacksRemaining int
    FlurryStrikesRemaining  int
    DisengageActive         bool
    DodgeActive             bool
}
```

`ActionEconomyState` has methods (`HasAction`, `UseAction`, `HasBonusAction`, etc.) — this is intentional behavior on entities (not game rules, just state tracking).

## Composition (`rpg-toolkit/world/composition.Data`)

The toolkit type is canonical: `ID` and `WorldID` identify a composition, and `JSON`
holds its opaque authoring payload. rpg-api does not duplicate or interpret that payload.
For the local-dev RPC, the orchestrator mints `ID` and the handler supplies its configured
WorldID after player/world checks; the repository remains a typed caller-supplied storage
contract. It stores a serialized `composition.Data` snapshot directly, with no API-owned
definition/revision/head model. Production guild-to-world mapping and rendering
integration do not exist here.

## DiceSession (repositories/dice_session)

Tracks in-progress ability score rolls for character creation. Redis-backed. Narrow scope; stores the rolls until assigned to a draft.

## Position types (canonical model, post-#471)

There are now two domain coordinate types, both integer cube coordinates with the invariant `X+Y+Z == 0`:

| Type | Location | Semantics |
|---|---|---|
| `dungeon.LocalPosition` | `components/dungeon/coords.go` | Coordinate inside a single room (origin (0,0,0) is the room's local origin) |
| `dungeon.AbsolutePosition` | `components/dungeon/coords.go` | Coordinate in dungeon-absolute space |
| `apiv1alpha1.Position` (proto) | generated | Wire format, `int32 X, Y, Z` |

`dungeon.Module` (`components/dungeon/module.go`) bridges the two via `LocalToAbsolute(roomID, LocalPosition) AbsolutePosition` and `AbsoluteToLocal(roomID, AbsolutePosition) LocalPosition`. Every transform site routes through Module — the local-vs-absolute distinction is enforced at the type level by the compiler. Proto conversion is a single 3-field cast (`int` → `int32`) at the handler boundary.

The old `entities.Position` (float64) and `dungeon.Position` (int) types — and the buggy ad-hoc casts between them that caused PRs #459, #461, #463, #466, #467 — were deleted in #471.

## Redis key schema (character repos)

Character repository (`repositories/character/redis.go`):
- `character:{id}` — JSON-serialized `entities.Character` wrapping toolkit `character.Data`

Character draft repository (`repositories/character_draft/redis.go`):
- `draft:{draftID}` — JSON-serialized `entities.CharacterDraft` wrapping toolkit `character.DraftData`
- `character_drafts:{playerID}` — set of draft IDs per player

Dice session repository (`repositories/dice_session/redis.go`):
- `dice_session:{playerID}:{sessionID}` — JSON-serialized session state

Composition repository (`repositories/composition/redis.go`):
- `composition:<WorldID>` — hash with composition ID fields and serialized toolkit
  `composition.Data` values; no TTL

~~Encounter events (publisher, `publishers/encounter/redis.go`):~~ DELETED
(rpg-api#642, 2026-07-13) — the v1 pub/sub publisher and the `EncounterEvent`
type it serialized are both gone. The v2 encounter path uses
`internal/repositories/encounters/v2/redis.go` (24h TTL, wired into
`cmd/server/server.go`) for its own state, and the toolkit's `tkenc.Broker`
for live event fan-out — neither documented here yet (follow-up).
