# Door-Based Spawning Design

## Overview

Redesign player and monster spawning to be relative to door positions rather than room centers. Players spawn near the entrance door they came through; monsters spawn opposite them near the exit.

## Problem

Current spawning places entities in room centers regardless of door positions. This feels unnatural - players should enter through a door and spread out from there.

## Design

### Door Placement

**Entrance room (first room) has two doors:**
- **South wall**: Entrance door (leads outside - party can retreat/leave dungeon)
- **North wall**: Exit door (leads to next room)

Doors are placed at wall midpoints. Other rooms continue using existing connection-based door placement.

### Player Spawn Positions

Players spawn in a semi-circle formation near the entrance door (south wall):

```
Row 2:    [2] [1] [3]     <- Player 1 center, 2 left, 3 right
Row 1:  [4]   [D]   [5]   <- Player 4 left flank, 5 right flank
Wall:   ═══════════════   <- south wall with door D
```

**Fill order (symmetrical):**
1. Center back (directly behind door)
2. Left back
3. Right back
4. Left flank (beside door)
5. Right flank (beside door)
6+ Extend flanks outward along wall

**Position calculation:**
- Door position: `x = width/2, z = 1` (one row inside south wall)
- Position 1: `(door_x, door_z + 1)`
- Position 2: `(door_x - 1, door_z + 1)`
- Position 3: `(door_x + 1, door_z + 1)`
- Position 4: `(door_x - 1, door_z)`
- Position 5: `(door_x + 1, door_z)`

### Monster Spawn Positions

Monsters spawn near the exit door (north wall), creating a natural confrontation:

```
Wall:   ═══════════════   <- north wall with exit door
Row 1:  [M]   [D]   [M]   <- monsters flanking door
Row 2:    [M] [M] [M]     <- overflow row
```

Monsters fill symmetrically from door, mirroring player logic but on the opposite wall.

**Future consideration:** Monsters may eventually spawn spread throughout the room rather than clustered by the exit. The door-relative approach is a starting point.

### Non-Entrance Rooms

- Player spawns: Not applicable (players enter from previous room)
- Monster spawns: Keep existing center-based spawning (monsters are already in the room when door opens)

## Implementation

### Changes Required

1. **`layout.go`** - Add entrance connection for first room
   - Create connection from room 0 to `"outside"` or null marker
   - Direction: south (entrance is at bottom)

2. **`generator.go`** - Reorder generation flow
   - Current: shape → features (spawn zones) → connections
   - New: shape → connections/doors → features (spawn zones use door positions)
   - Pass door positions to feature generator

3. **`feature.go`** - Door-aware spawn zone generation
   - `generateSpawnZones` takes door positions as input
   - Entrance rooms: generate player spawns relative to south door
   - Calculate positions using door coordinates + offsets

### What Stays the Same

- Spawner component (receives zone positions, unchanged)
- Safe fallback logic (spiral search avoiding walls)
- Wall position filtering (removes positions on pillars)
- Perimeter wall generation
- Monster factory and placement flow

## Testing

### Test Cases

1. **Entrance room has two doors**
   - Assert door on south wall (entrance)
   - Assert door on north wall (exit)

2. **Player spawn positions near entrance**
   - All positions within 2 rows of south wall
   - Positions form semi-circle pattern

3. **Spawn order is symmetrical**
   - Position 1 at center
   - Positions 2/3 flank left/right of center
   - Positions 4/5 beside door

4. **Spawn positions avoid walls**
   - No positions on perimeter walls
   - No positions on internal walls (pillars)

5. **Monster spawns opposite players**
   - Monster zone near north wall
   - Clear separation from player spawn area

### Validation Approach

Extend existing `placement_validation_test.go`:
```go
func TestEntranceRoomSpawning() {
    // Generate dungeon
    // Find entrance room
    // Assert 2 doors (south + north)
    // Assert player spawn zone positions near south door
    // Assert monster spawn zone positions near north door
    // Assert no positions on walls
}
```

## Future Considerations

- Monster spawning may evolve to "spread throughout room" rather than door-clustered
- Larger parties (6+) extend spawn positions along wall
- Different room shapes may need adjusted spawn patterns
- Boss rooms may have special spawn arrangements
