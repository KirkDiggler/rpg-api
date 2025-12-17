package entities

import "github.com/KirkDiggler/rpg-toolkit/tools/spatial"

// CubeBounds represents a bounding area in cube coordinates.
// Used to track room boundaries in the merged coordinate space.
type CubeBounds struct {
	MinX, MinY, MinZ int
	MaxX, MaxY, MaxZ int
}

// Contains checks if a cube coordinate is within the bounds.
func (b CubeBounds) Contains(c spatial.CubeCoordinate) bool {
	return c.X >= b.MinX && c.X <= b.MaxX &&
		c.Y >= b.MinY && c.Y <= b.MaxY &&
		c.Z >= b.MinZ && c.Z <= b.MaxZ
}

// MergedGrid tracks a unified coordinate space as dungeon rooms connect.
// When doors open, connected rooms are placed adjacent to the existing combat space.
// All positions use cube coordinates (x + y + z = 0).
type MergedGrid struct {
	// Entities with cube coordinate positions
	Entities map[string]spatial.EntityCubePlacement

	// Blocked positions (obstacles, closed doors, entities)
	Blocked map[spatial.CubeCoordinate]bool

	// Room placement tracking
	RoomBounds  map[string]CubeBounds             // Room ID -> bounds in merged coords
	RoomOffsets map[string]spatial.CubeCoordinate // Room ID -> offset applied

	// Door positions in merged coords
	DoorPositions map[string]spatial.CubeCoordinate // Connection ID -> position
}

// NewMergedGrid creates a new empty merged grid.
func NewMergedGrid() *MergedGrid {
	return &MergedGrid{
		Entities:      make(map[string]spatial.EntityCubePlacement),
		Blocked:       make(map[spatial.CubeCoordinate]bool),
		RoomBounds:    make(map[string]CubeBounds),
		RoomOffsets:   make(map[string]spatial.CubeCoordinate),
		DoorPositions: make(map[string]spatial.CubeCoordinate),
	}
}

// AddRoom registers a room with its bounds and offset in the merged grid.
func (g *MergedGrid) AddRoom(roomID string, bounds CubeBounds, offset spatial.CubeCoordinate) {
	g.RoomBounds[roomID] = bounds
	g.RoomOffsets[roomID] = offset
}

// LocalToMerged transforms a local room coordinate to merged grid coordinates.
// Returns zero coordinate if the room is not found.
func (g *MergedGrid) LocalToMerged(roomID string, local spatial.CubeCoordinate) spatial.CubeCoordinate {
	offset, exists := g.RoomOffsets[roomID]
	if !exists {
		return spatial.CubeCoordinate{}
	}
	return local.Add(offset)
}

// MergedToLocal finds which room contains the merged coordinate and transforms it to local coordinates.
// Returns empty roomID if the coordinate is not in any known room.
func (g *MergedGrid) MergedToLocal(pos spatial.CubeCoordinate) (roomID string, local spatial.CubeCoordinate) {
	for id, offset := range g.RoomOffsets {
		// Calculate what the local coordinate would be
		localCandidate := pos.Subtract(offset)
		// Check if it falls within this room's bounds
		if g.RoomBounds[id].Contains(localCandidate) {
			return id, localCandidate
		}
	}
	return "", spatial.CubeCoordinate{}
}

// AddEntity adds an entity to the grid and marks its position as blocked.
func (g *MergedGrid) AddEntity(placement spatial.EntityCubePlacement) {
	g.Entities[placement.EntityID] = placement
	g.Blocked[placement.CubePosition] = true
}

// RemoveEntity removes an entity from the grid and unblocks its position.
func (g *MergedGrid) RemoveEntity(entityID string) {
	placement, exists := g.Entities[entityID]
	if !exists {
		return
	}
	delete(g.Blocked, placement.CubePosition)
	delete(g.Entities, entityID)
}

// MoveEntity moves an entity to a new position if it's not blocked.
// Returns true if the move succeeded, false if blocked or entity not found.
func (g *MergedGrid) MoveEntity(entityID string, newPos spatial.CubeCoordinate) bool {
	placement, exists := g.Entities[entityID]
	if !exists {
		return false
	}
	if g.IsBlocked(newPos) {
		return false
	}

	// Unblock old position
	delete(g.Blocked, placement.CubePosition)

	// Update entity position
	placement.CubePosition = newPos
	g.Entities[entityID] = placement

	// Block new position
	g.Blocked[newPos] = true

	return true
}

// GetEntityPosition returns the position of an entity.
// Returns false if the entity is not found.
func (g *MergedGrid) GetEntityPosition(entityID string) (spatial.CubeCoordinate, bool) {
	placement, exists := g.Entities[entityID]
	if !exists {
		return spatial.CubeCoordinate{}, false
	}
	return placement.CubePosition, true
}

// SetDoorPosition records a door's position in merged coordinates.
func (g *MergedGrid) SetDoorPosition(connectionID string, pos spatial.CubeCoordinate) {
	g.DoorPositions[connectionID] = pos
}

// OpenDoor removes a door from the blocked set.
func (g *MergedGrid) OpenDoor(connectionID string) {
	pos, exists := g.DoorPositions[connectionID]
	if !exists {
		return
	}
	delete(g.Blocked, pos)
}

// CloseDoor adds a door to the blocked set.
func (g *MergedGrid) CloseDoor(connectionID string) {
	pos, exists := g.DoorPositions[connectionID]
	if !exists {
		return
	}
	g.Blocked[pos] = true
}

// IsBlocked checks if a position is blocked by an entity, obstacle, or closed door.
func (g *MergedGrid) IsBlocked(pos spatial.CubeCoordinate) bool {
	return g.Blocked[pos]
}

// cubeDirections maps direction strings to cube coordinate deltas.
// For pointy-top hex orientation.
var cubeDirections = map[string]spatial.CubeCoordinate{
	"e":  {X: 1, Y: -1, Z: 0}, // east
	"ne": {X: 1, Y: 0, Z: -1}, // northeast
	"nw": {X: 0, Y: 1, Z: -1}, // northwest
	"w":  {X: -1, Y: 1, Z: 0}, // west
	"sw": {X: -1, Y: 0, Z: 1}, // southwest
	"se": {X: 0, Y: -1, Z: 1}, // southeast
}

// CalculateRoomOffset determines where to place a new room based on door alignment.
// doorInExisting: position of door in the existing room (merged coords)
// doorInNew: position of door in the new room (local coords)
// direction: which hex neighbor to use ("e", "ne", "nw", "w", "sw", "se")
// Returns the offset to apply to the new room so its door is adjacent to the existing door.
func CalculateRoomOffset(doorInExisting, doorInNew spatial.CubeCoordinate, direction string) spatial.CubeCoordinate {
	delta, exists := cubeDirections[direction]
	if !exists {
		return spatial.CubeCoordinate{}
	}

	// The new room's door should be at the neighbor position of the existing door
	targetPos := doorInExisting.Add(delta)

	// Offset = where we want the door to be - where it is locally
	return targetPos.Subtract(doorInNew)
}
