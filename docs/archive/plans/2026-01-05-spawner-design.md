# Spawner Component Design

**Date**: 2026-01-05
**Status**: Approved for implementation

## Problem

The dungeon component has cascading issues with entity spawning:
- Monsters spawning on walls
- Monsters spawning on character positions
- Spawn zones not being found (falling back to hardcoded positions)
- Coordinate conversion inconsistencies

Root cause: Dual representation of spawn zones violates "pick ONE way to represent data" principle.

## Phase 1: Clean Up Spawn Zone Representation

### Current State (Broken)

Spawn zones stored in TWO places:
- `Room.Features.SpawnZones []Zone` - where data is created
- `Room.SpawnZones []*Zone` - where orchestrator reads

Generator duplicates data between them, but sync breaks during JSON serialization.

### Target State

Single source of truth: `Room.SpawnZones []*Zone`

### Changes

1. **`types.go`** - Remove `SpawnZones` from `FeatureLayout`
   ```go
   type FeatureLayout struct {
       Obstacles []Obstacle
       Terrain   []TerrainPatch
       // SpawnZones removed - lives on Room directly
   }
   ```

2. **`generator.go`** - Assign zones directly to Room (already does this)

3. **`toolkit/feature.go`** - Update `FeatureOutput` structure
   ```go
   type FeatureOutput struct {
       Features *FeatureLayout  // obstacles, terrain only
       Walls    []WallSegment
       Zones    []Zone          // separate from Features
   }
   ```

4. **Orchestrator** - Already reads from `Room.SpawnZones`, no change needed

---

## Phase 2: Spawner Component

### Location

`/internal/components/spawner/` - designed for extraction to rpg-toolkit

### Core Interface

```go
package spawner

type Spawner interface {
    Spawn(input *SpawnInput) (*SpawnOutput, error)
}

type SpawnInput struct {
    Width       int
    Height      int
    Walls       []WallSegment
    Occupied    []CubePosition
    Zones       []SpawnZone
    Entities    []EntityToSpawn
}

type SpawnOutput struct {
    Placements []EntityPlacement
    Errors     []PlacementError
}
```

### Supporting Types

```go
type CubePosition struct {
    X, Y, Z int
}

type WallSegment struct {
    Start, End     CubePosition
    BlocksMovement bool
}

type SpawnZone struct {
    ID       string
    Type     ZoneType
    Bounds   []CubePosition
    Capacity int
}

type ZoneType string
const (
    ZoneTypePlayerSpawn  ZoneType = "player_spawn"
    ZoneTypeMonsterSpawn ZoneType = "monster_spawn"
    ZoneTypeBoss         ZoneType = "boss"
)

type EntityToSpawn struct {
    ID            string
    Type          EntityType
    PreferredZone ZoneType
    Size          int
}

type EntityPlacement struct {
    EntityID string
    Position CubePosition
    ZoneID   string
}

type PlacementError struct {
    EntityID string
    Reason   string
}
```

### Algorithm

```go
func (s *DefaultSpawner) Spawn(input *SpawnInput) (*SpawnOutput, error) {
    // 1. Build blocked position set (walls + occupied)
    blocked := s.buildBlockedSet(input)

    // 2. Filter zone positions to only valid (unblocked) ones
    availableByZone := s.filterAvailablePositions(input.Zones, blocked)

    // 3. Place each entity
    for _, entity := range input.Entities {
        position, zoneID, err := s.placeEntity(entity, availableByZone)
        // ... handle placement or error
        // Mark position as occupied after placement
    }

    return &SpawnOutput{...}, nil
}
```

### Design Principles

- **Spatial correctness first** - avoid walls, avoid overlaps, respect zones
- **Modular for future enhancement** - tactical placement can be added later
- **Zero rpg-api dependencies** - ready for extraction to toolkit
- **Stateless** - input room state, output placements

---

## Implementation Order

1. Phase 1: Clean up dual representation (~30 min)
2. Phase 2: Build spawner component (~1-2 hours)
3. Integration: Wire spawner into dungeon generation
4. Remove fallback positions from orchestrator
