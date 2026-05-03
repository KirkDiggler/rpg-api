# Dungeon Generation Design

## Overview

A procedural dungeon generation system that creates themed, multi-room dungeons with CR-scaled encounters. Designed as a modular component in rpg-api that can later be extracted to rpg-toolkit.

## Public API

```go
// Simple config-based API for users
result, err := dungeon.Generate(ctx, &GenerateInput{
    Theme:     ThemeCrypt,      // crypt, bandit_lair, cave, etc.
    Size:      SizeMedium,      // room dimensions (small/medium/large)
    Length:    4,               // number of rooms
    Layout:    LayoutBranching, // branching, linear, hub (default: branching)
    PartySize: 4,               // number of players
    TargetCR:  3,               // party level for encounter scaling
    Seed:      0,               // optional, for reproducibility
})
```

## Output Types

```go
type GenerateOutput struct {
    Dungeon *Dungeon
    Seed    int64    // actual seed used (for replay)
}

type Dungeon struct {
    ID          string
    Theme       Theme
    Rooms       []*Room
    Connections []*RoomConnection
    StartRoom   string
    BossRoom    string
}

type Room struct {
    ID          string
    Shape       *Shape
    Features    *FeatureLayout
    SpawnZones  []*Zone
    Encounter   *Encounter
}
```

## Architecture

### Directory Structure

```
internal/components/dungeon/
├── generator.go          # Public API, orchestrates layers
├── types.go              # Dungeon, Room, Shape, etc.
├── theme.go              # Theme definitions
│
├── shape/                # Layer 1: Shape generation
│   ├── generator.go      # ShapeGenerator interface + impl
│   ├── types.go          # Shape, Bounds, ShapeStyle
│   └── patterns.go       # L-shape, T-shape, organic algorithms
│
├── feature/              # Layer 2: Feature placement
│   ├── generator.go      # FeatureGenerator interface + impl
│   ├── types.go          # Feature, Zone, Obstacle
│   └── placer.go         # Placement algorithms
│
├── encounter/            # Layer 3: Monster composition
│   ├── generator.go      # EncounterGenerator interface + impl
│   ├── types.go          # Encounter, MonsterRole
│   ├── budget.go         # CR budget allocation
│   └── selector.go       # Role-based monster selection
│
└── layout/               # Room connection logic
    ├── generator.go      # LayoutGenerator interface + impl
    └── patterns.go       # Branching, linear, hub algorithms
```

### Layer Data Flow

```
GenerateInput
    ↓
LayoutGenerator  → decides room count, connections, main path
    ↓
ShapeGenerator   → creates shape per room (uses theme.ShapeStyle)
    ↓
FeatureGenerator → places obstacles, spawn zones (uses theme.FeatureRules)
    ↓
EncounterGenerator → fills monsters (uses theme.MonsterPool, CR budget)
    ↓
Dungeon (assembled output)
```

## Layer Interfaces

### Shape Generator

```go
type ShapeGenerator interface {
    Generate(input *ShapeInput) (*Shape, error)
}

type ShapeInput struct {
    Size   RoomSize
    Style  ShapeStyle    // from theme
    Seed   int64
}

type Shape struct {
    Bounds   []Position    // polygon vertices
    GridType GridType      // hex, square
    Width    int
    Height   int
    Area     int           // walkable cells
}

type ShapeStyle int
const (
    ShapeStyleStructured ShapeStyle = iota  // rectangles, L, T shapes
    ShapeStyleOrganic                        // caves, irregular
    ShapeStyleMixed
)
```

**Size dimensions:**
- Small: 10-15 x 10-15
- Medium: 15-25 x 15-20
- Large: 25-40 x 20-30

**Algorithms:**
- Structured: Compose rectangles, random chance for L/T wings
- Organic: Cellular automata, flood fill for connectivity

### Feature Generator

```go
type FeatureGenerator interface {
    Generate(input *FeatureInput) (*FeatureLayout, error)
}

type FeatureInput struct {
    Shape *Shape
    Rules FeatureRules    // from theme
    Seed  int64
}

type FeatureLayout struct {
    Obstacles  []Obstacle
    Terrain    []TerrainPatch
    SpawnZones []Zone
}

type Zone struct {
    ID       string
    Type     ZoneType    // PlayerSpawn, MonsterSpawn, Boss, Entrance, Exit
    Bounds   []Position
    Capacity int
}
```

**Placement algorithm:**
1. Identify key positions (entrance, exit, center, corners)
2. Place spawn zones first (highest priority)
3. Place obstacles with constraints (spacing, don't block paths)
4. Apply terrain patches avoiding spawn zones

### Encounter Generator

```go
type EncounterGenerator interface {
    Generate(input *EncounterInput) (*Encounter, error)
}

type EncounterInput struct {
    Layout      *FeatureLayout
    MonsterPool []MonsterRef
    BossPool    []MonsterRef
    CRBudget    float64
    IsBossRoom  bool
    Seed        int64
}

type Encounter struct {
    Monsters []MonsterPlacement
    TotalCR  float64
}

type MonsterRole int
const (
    RoleMelee   MonsterRole = iota
    RoleRanged
    RoleSupport
    RoleBoss
)
```

**CR Budget allocation:**
- Reserve 40% of total CR for boss room
- Distribute remaining with slight escalation (later rooms harder)
- Per-room budget based on party size and target CR

**Role-based selection:**
- Low CR: mostly melee, maybe 1 ranged
- Medium CR: mix of melee + ranged
- High CR: full composition with support
- Boss rooms: boss from BossPool + minions

### Layout Generator

```go
type LayoutGenerator interface {
    Generate(input *LayoutInput) (*DungeonLayout, error)
}

type LayoutInput struct {
    Length     int
    LayoutType LayoutType
    Seed       int64
}

type DungeonLayout struct {
    Rooms       []RoomSlot
    Connections []RoomConnection
    StartRoom   int
    BossRoom    int
}

type RoomConnection struct {
    FromRoom     int
    ToRoom       int
    Type         ConnectionType  // door, stairs, passage
    IsMainPath   bool            // internal tracking
    PhysicalHint string          // "north door", "side passage"
}
```

**Branching algorithm (default):**
1. Create main path: Start → Room 1 → ... → Boss
2. Add side rooms based on length (1-3 branches)
3. Randomize physical placement - main path not always "forward"
4. Player discovers path through exploration

## Theme System

Themes bundle rules for all generators:

```go
type Theme struct {
    ID          string
    Name        string
    ShapeStyle  ShapeStyle
    Features    FeatureRules
    MonsterPool []MonsterRef
    BossPool    []MonsterRef
}

type FeatureRules struct {
    ObstacleChance float64
    ObstacleTypes  []ObstacleType
    TerrainChance  float64
    TerrainTypes   []TerrainType
}

type MonsterRef struct {
    ID   string        // references rulebook monster
    Role MonsterRole
    CR   float64
}
```

**Example themes:**

```go
var ThemeCrypt = Theme{
    ID:         "crypt",
    Name:       "Ancient Crypt",
    ShapeStyle: ShapeStyleStructured,
    Features: FeatureRules{
        ObstacleChance: 0.6,
        ObstacleTypes:  []ObstacleType{Pillar, Sarcophagus, Altar},
    },
    MonsterPool: []MonsterRef{
        {ID: "skeleton", Role: RoleMelee, CR: 0.25},
        {ID: "skeleton-archer", Role: RoleRanged, CR: 0.25},
        {ID: "zombie", Role: RoleMelee, CR: 0.25},
    },
    BossPool: []MonsterRef{
        {ID: "skeleton-captain", Role: RoleBoss, CR: 2},
    },
}

var ThemeCave = Theme{
    ID:         "cave",
    Name:       "Natural Cave",
    ShapeStyle: ShapeStyleOrganic,
    Features: FeatureRules{
        ObstacleChance: 0.4,
        ObstacleTypes:  []ObstacleType{Boulder, Stalagmite, Pool},
    },
    MonsterPool: []MonsterRef{
        {ID: "giant-rat", Role: RoleMelee, CR: 0.125},
        {ID: "giant-spider", Role: RoleMelee, CR: 0.5},
    },
    BossPool: []MonsterRef{
        {ID: "giant-wolf-spider", Role: RoleBoss, CR: 1},
    },
}
```

## Integration with Existing Code

### Encounter Orchestrator Usage

```go
func (o *Orchestrator) StartDungeon(ctx context.Context, input *StartDungeonInput) (*StartDungeonOutput, error) {
    dungeonOutput, err := o.dungeonGen.Generate(ctx, &dungeon.GenerateInput{
        Theme:     input.Theme,
        Size:      input.Size,
        Length:    input.Length,
        PartySize: len(input.CharacterIDs),
        TargetCR:  input.TargetCR,
    })

    firstRoom := dungeonOutput.Dungeon.Rooms[0]
    roomData := convertToSpatialRoom(firstRoom)

    // ... rest of encounter setup
}
```

### Spatial Module Conversion

```go
func convertToSpatialRoom(room *dungeon.Room) *spatial.RoomData {
    roomData := &spatial.RoomData{
        ID:       room.ID,
        Type:     "dungeon",
        Width:    room.Shape.Width,
        Height:   room.Shape.Height,
        GridType: room.Shape.GridType,
        Entities: make(map[string]spatial.EntityPlacement),
    }

    for _, obs := range room.Features.Obstacles {
        roomData.Entities[obs.ID] = spatial.EntityPlacement{
            EntityID:          obs.ID,
            EntityType:        "obstacle",
            Position:          obs.Position,
            BlocksMovement:    obs.BlocksMovement,
            BlocksLineOfSight: obs.BlocksLineOfSight,
        }
    }

    return roomData
}
```

## Migration Path to Toolkit

When ready to extract to `rpg-toolkit/tools/dungeon`:

1. Move `internal/components/dungeon/` → `rpg-toolkit/tools/dungeon/`
2. Replace monster ID strings with toolkit monster registry constants
3. `rpg-api` imports and calls toolkit dungeon generator
4. Same interfaces, different package location

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Generation approach | Procedural (not prefabs) | Building it right, infinite variety |
| Layer architecture | Composable with clear contracts | Testable, swappable implementations |
| Theme structure | Preset bundle | Cohesive, easy to add new themes |
| Size vs Length | Separate parameters | Independent control over room size and count |
| Room shapes | Theme-driven style | Crypts structured, caves organic |
| Layout default | Branching | Interesting exploration without complexity |
| CR distribution | Budget pool (40% boss) | Guarantees satisfying boss fight |
| Monster selection | Role-based | Tactically interesting encounters |
| Path visibility | Exploration-focused | Main path not visually obvious |
| Attrition | Ignored for now | Keep simple, tune after playtesting |

## Tech Debt

- Monster IDs are strings, will become constants from `rpg-toolkit/rulebooks/dnd5e/monsters`
- CR calculation simplified, may need D&D encounter math refinement
- Attrition not factored in, may need adjustment after playtesting

## Related Issues

- Issue #294: Rest System (complements dungeon generation)
- Issue #295: Dungeon Generation (this design addresses it)
