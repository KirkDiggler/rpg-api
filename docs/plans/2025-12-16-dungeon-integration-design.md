# Dungeon Integration Design

Wire the existing `internal/components/dungeon/` generator into the encounter flow for solo and multiplayer dungeon runs.

## Key Decisions

| Topic | Decision |
|-------|----------|
| Room reveal | Discovery moment - doors are visible, opening reveals room |
| Monster initiative | Join current order, continue from current position |
| Room state | All explored rooms stay active |
| Player config | Theme (enum) + Difficulty (enum) + Length (enum) |
| Data model | Separate Dungeon entity from Encounter |
| Client data | Progressive streaming - rooms sent as revealed |
| Movement | Merged grid - rooms stitch together when doors open |
| Solo vs multi | Solo skips lobby, both have config step |
| Victory | Boss kill, optional full clear |
| Failure | TPK ends dungeon (death saves later) |

## Data Model

### Philosophy: Proving Ground Pattern

The `internal/components/dungeon/` package serves as a **proving ground** for features that may graduate to rpg-toolkit:

1. **Use toolkit types where they exist** - Don't duplicate what toolkit provides
2. **Add what toolkit lacks here** - Exploration state, dungeon lifecycle, etc.
3. **Iterate fast, validate assumptions** - Confirm patterns work before moving to toolkit
4. **Graduate proven patterns** - Create explicit toolkit issues once we're confident

This keeps the toolkit clean while letting us move quickly.

### Type Layering

| Layer | Types Used | Source |
|-------|------------|--------|
| Entity | `entities.Dungeon` | rpg-api (wraps below) |
| Room content | `dungeon.Room` | rpg-api component (D&D 5e specific) |
| Connections | `environments.ConnectionEdge` | rpg-toolkit |
| Positions | `spatial.Position` | rpg-toolkit |
| Exploration state | `DungeonState`, `RevealedRooms`, etc. | rpg-api (proving ground) |

### New Dungeon Entity

Separate from Encounter. Dungeon owns the map and exploration state, Encounter owns combat state.

```go
import (
    "github.com/KirkDiggler/rpg-toolkit/tools/environments"
    "github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type Dungeon struct {
    ID          string
    EncounterID string
    Seed        int64

    // From toolkit - connection graph structure
    Connections []*environments.ConnectionEdge
    StartRoomID string
    BossRoomID  string

    // From component - room content with D&D 5e encounters
    Rooms map[string]*dungeon.Room

    // Exploration state (proving ground - may move to toolkit)
    State         DungeonState
    CurrentRoomID string
    RevealedRooms map[string]bool  // Room ID -> explored
    OpenDoors     map[string]bool  // Connection ID -> open

    // Merged grid state (proving ground)
    ActiveGrid  *MergedGrid
    RoomOffsets map[string]spatial.Position

    // Metrics
    RoomsCleared   int
    MonstersKilled int

    // Timestamps
    CreatedAt   time.Time
    CompletedAt *time.Time
}

type DungeonState int

const (
    DungeonStateActive DungeonState = iota
    DungeonStateVictorious
    DungeonStateFailed
    DungeonStateAbandoned
)
```

Note: Theme, Difficulty, and Length enums live in the component package, not the entity. They're input parameters for generation, not persisted dungeon state.

### Encounter Changes

Encounter stays focused on combat, references dungeon:

```go
type Encounter struct {
    // Existing fields...
    DungeonID string  // Links to Dungeon entity
}
```

### MergedGrid

Unified coordinate space as rooms connect:

```go
type MergedGrid struct {
    Width      int
    Height     int
    Entities   map[string]EntityPlacement
    Blocked    map[Position]bool
    RoomBounds map[string]Bounds
}
```

## Proto Changes

### New Enums

```protobuf
enum DungeonTheme {
  DUNGEON_THEME_UNSPECIFIED = 0;
  DUNGEON_THEME_CRYPT = 1;
  DUNGEON_THEME_CAVE = 2;
  DUNGEON_THEME_RUINS = 3;
}

enum DungeonDifficulty {
  DUNGEON_DIFFICULTY_UNSPECIFIED = 0;
  DUNGEON_DIFFICULTY_EASY = 1;
  DUNGEON_DIFFICULTY_MEDIUM = 2;
  DUNGEON_DIFFICULTY_HARD = 3;
}

enum DungeonLength {
  DUNGEON_LENGTH_UNSPECIFIED = 0;
  DUNGEON_LENGTH_SHORT = 1;   // 3-4 rooms
  DUNGEON_LENGTH_MEDIUM = 2;  // 5-7 rooms
  DUNGEON_LENGTH_LONG = 3;    // 8-10 rooms
}
```

### Updated DungeonStart

```protobuf
message DungeonStartRequest {
  repeated string character_ids = 1;
  DungeonTheme theme = 2;
  DungeonDifficulty difficulty = 3;
  DungeonLength length = 4;
}

message DungeonStartResponse {
  string encounter_id = 1;
  string dungeon_id = 2;
  Room room = 3;
  repeated DoorInfo doors = 4;
  CombatState combat_state = 5;
  repeated MonsterTurn monster_turns = 6;
}
```

### New Messages

```protobuf
message DoorInfo {
  string connection_id = 1;
  Position position = 2;
  string physical_hint = 3;  // "iron door", "crumbling archway"
  bool is_open = 4;
}

message OpenDoorRequest {
  string encounter_id = 1;
  string connection_id = 2;
}

message OpenDoorResponse {
  Room revealed_room = 1;
  Position room_offset = 2;
  repeated DoorInfo new_doors = 3;
  repeated MonsterInfo monsters = 4;
  CombatState combat_state = 5;
}
```

### New Events

```protobuf
message RoomRevealedEvent {
  Room room = 1;
  Position offset = 2;
  repeated DoorInfo doors = 3;
  repeated MonsterInfo monsters = 4;
}

message DungeonVictoryEvent {
  int32 rooms_cleared = 1;
  int32 monsters_killed = 2;
}

message DungeonFailureEvent {
  string reason = 1;  // "tpk"
}
```

## Service Layer Changes

### New Dungeon Repository

```go
// internal/repositories/dungeons/repository.go
type Repository interface {
    Save(ctx context.Context, dungeon *Dungeon) error
    Get(ctx context.Context, id string) (*Dungeon, error)
    Update(ctx context.Context, id string, updates *UpdateInput) error
}
```

In-memory implementation to start, same pattern as encounters.

### Encounter Orchestrator Config

```go
type Config struct {
    CharacterRepo   characterrepo.Repository
    EncounterRepo   encounterrepo.Repository
    DungeonRepo     dungeonrepo.Repository    // NEW
    DungeonGen      *dungeon.Generator        // NEW
    Publisher       encounterpub.Publisher
}
```

### New Service Methods

```go
type Service interface {
    // Existing methods...

    OpenDoor(ctx context.Context, input *OpenDoorInput) (*OpenDoorOutput, error)
    ExitDungeon(ctx context.Context, input *ExitDungeonInput) (*ExitDungeonOutput, error)
}
```

## Flows

### Dungeon Start (Solo)

1. Client calls `DungeonStart` with character_ids + theme + difficulty + length
2. Server maps enums to generation params
3. Server calls `DungeonGen.Generate()` with mapped params
4. Server creates `Dungeon` entity (all rooms unexplored, all doors closed)
5. Server creates `Encounter` entity linked to dungeon
6. Server initializes `MergedGrid` with start room at origin
7. Server places characters at start room spawn zone
8. Server spawns start room monsters, rolls initiative
9. Server streams `DungeonStartedEvent` with start room + doors
10. Combat begins

### Dungeon Start (Multiplayer)

1. Host calls `CreateEncounter` with theme + difficulty + length (stored, not generated yet)
2. Players join via code, select characters
3. Host calls `StartCombat` when all ready
4. Server generates dungeon (same as solo steps 3-9)
5. Streams to all players

### Door Opening

1. Player uses action: `OpenDoor` with connection_id
2. Server validates: door exists, closed, player adjacent
3. Server marks connection open in `Dungeon.OpenDoors`
4. Server marks room revealed in `Dungeon.RevealedRooms`
5. Server calculates room offset for grid merge
6. Server expands `MergedGrid`, transforms room entities
7. Server removes door from blocked positions
8. Server creates live monsters from room's placements
9. Server rolls initiative for new monsters
10. Server inserts into existing initiative order
11. Server streams `RoomRevealedEvent`
12. Player's turn ends (opening was their action)

### Grid Merge Algorithm

1. Get door positions in both rooms (local coords)
2. Get Room A's offset in merged grid
3. Calculate Room B offset so doors are adjacent
4. Expand grid bounds if needed
5. Transform Room B entities by offset, add to merged grid
6. Mark door position as passable

### Victory

1. Monster dies
2. Check: was it the boss? (in BossRoomID + RoleBoss)
3. If yes: stream `DungeonVictoryEvent`, set state to VICTORIOUS
4. Combat continues if monsters remain
5. Players can keep exploring or call `ExitDungeon`

### Failure (TPK)

1. Character drops to 0 HP, mark as DOWN
2. Check: are all characters DOWN?
3. If yes: stream `DungeonFailureEvent`, set state to FAILED
4. Encounter ends, no further actions

## Component Mapping

Map player choices to generator params:

```go
func mapToGeneratorInput(theme DungeonTheme, diff DungeonDifficulty, length DungeonLength, partySize int) *dungeon.GenerateInput {
    return &dungeon.GenerateInput{
        Theme:     themeToInternal(theme),
        Size:      dungeon.RoomSizeMedium,
        Length:    lengthToRoomCount(length),  // short=4, medium=6, long=9
        Layout:    dungeon.LayoutBranching,
        PartySize: partySize,
        TargetCR:  difficultyToCR(diff, partySize),
    }
}
```

## Future Considerations

- **Death saves**: Replace TPK with proper 0 HP mechanics (backlog item API 5/5)
- **Retreat**: Allow party to flee back to start and exit
- **Room effects**: Traps, environmental hazards per room

### Proving Ground → Toolkit Graduation

As patterns are validated here, create toolkit issues for:

- **Exploration state**: `RevealedRooms`, `OpenDoors` tracking could become toolkit features
- **MergedGrid**: Multi-room coordinate merging could move to `tools/spatial`
- **DungeonState lifecycle**: Active/Victory/Failed state machine may be generally useful
