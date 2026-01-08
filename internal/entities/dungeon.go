// Package entities defines the core data structures for the RPG API
package entities

import (
	"time"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// DungeonState tracks the lifecycle of a dungeon run (proving ground - may move to toolkit)
type DungeonState int

const (
	DungeonStateUnspecified DungeonState = iota
	DungeonStateActive                   // Dungeon in progress
	DungeonStateVictorious               // Boss defeated, dungeon complete
	DungeonStateFailed                   // Party wiped (TPK)
	DungeonStateAbandoned                // Players left the dungeon
)

// Dungeon represents a complete dungeon run with its state.
// It wraps toolkit types (ConnectionEdge) and component types (Room) with exploration state.
type Dungeon struct {
	ID          string `json:"id"`
	EncounterID string `json:"encounter_id"` // Link to the associated encounter
	Seed        int64  `json:"seed"`         // Random seed for reproducibility

	// From toolkit - connection graph structure
	Connections []*environments.ConnectionEdge `json:"connections"`
	StartRoomID string                         `json:"start_room_id"`
	BossRoomID  string                         `json:"boss_room_id"`

	// From component - room content with D&D 5e encounters
	Rooms map[string]*dungeon.Room `json:"rooms"`

	// Unified coordinate system - room positions in dungeon-absolute coordinates
	// Enables pathfinding and movement across the entire dungeon
	RoomPositions map[string]spatial.CubeCoordinate `json:"room_positions"`

	// Exploration state (proving ground - may move to toolkit)
	State         DungeonState    `json:"state"`
	CurrentRoomID string          `json:"current_room_id"` // Room players are currently in
	RevealedRooms map[string]bool `json:"revealed_rooms"`  // Room ID -> explored
	OpenDoors     map[string]bool `json:"open_doors"`      // Connection ID -> open

	// Metrics
	RoomsCleared   int `json:"rooms_cleared"`
	MonstersKilled int `json:"monsters_killed"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// IsRoomRevealed returns whether a room has been revealed to players
func (d *Dungeon) IsRoomRevealed(roomID string) bool {
	if d.RevealedRooms == nil {
		return false
	}
	return d.RevealedRooms[roomID]
}

// IsDoorOpen returns whether a door/connection has been opened
func (d *Dungeon) IsDoorOpen(connectionID string) bool {
	if d.OpenDoors == nil {
		return false
	}
	return d.OpenDoors[connectionID]
}

// RevealRoom marks a room as revealed
func (d *Dungeon) RevealRoom(roomID string) {
	if d.RevealedRooms == nil {
		d.RevealedRooms = make(map[string]bool)
	}
	d.RevealedRooms[roomID] = true
}

// OpenDoor marks a connection as open
func (d *Dungeon) OpenDoor(connectionID string) {
	if d.OpenDoors == nil {
		d.OpenDoors = make(map[string]bool)
	}
	d.OpenDoors[connectionID] = true
}

// GetRoom returns a room by ID
func (d *Dungeon) GetRoom(roomID string) *dungeon.Room {
	if d.Rooms == nil {
		return nil
	}
	return d.Rooms[roomID]
}

// GetConnectionsFromRoom returns all connections originating from a room
func (d *Dungeon) GetConnectionsFromRoom(roomID string) []*environments.ConnectionEdge {
	var result []*environments.ConnectionEdge
	for _, conn := range d.Connections {
		if conn.FromRoomID == roomID || conn.ToRoomID == roomID {
			result = append(result, conn)
		}
	}
	return result
}

// GetVisibleDoors returns connections from the current room that lead to unrevealed rooms
func (d *Dungeon) GetVisibleDoors() []*environments.ConnectionEdge {
	connections := d.GetConnectionsFromRoom(d.CurrentRoomID)
	var visible []*environments.ConnectionEdge
	for _, conn := range connections {
		// Door is visible if the room it leads to hasn't been revealed yet
		targetRoomID := conn.ToRoomID
		if conn.ToRoomID == d.CurrentRoomID {
			targetRoomID = conn.FromRoomID
		}
		if !d.IsRoomRevealed(targetRoomID) {
			visible = append(visible, conn)
		}
	}
	return visible
}

// IsBossRoom returns true if the given room ID is the boss room
func (d *Dungeon) IsBossRoom(roomID string) bool {
	if d.BossRoomID == "" {
		return false
	}
	return d.BossRoomID == roomID
}

// MarkVictory sets the dungeon state to victorious and records completion time
func (d *Dungeon) MarkVictory() {
	d.State = DungeonStateVictorious
	now := time.Now()
	d.CompletedAt = &now
}

// MarkFailed sets the dungeon state to failed and records completion time
func (d *Dungeon) MarkFailed() {
	d.State = DungeonStateFailed
	now := time.Now()
	d.CompletedAt = &now
}

// GetRoomPosition returns a room's origin in dungeon-absolute coordinates.
// Returns zero coordinate and false if room not found.
func (d *Dungeon) GetRoomPosition(roomID string) (spatial.CubeCoordinate, bool) {
	if d.RoomPositions == nil {
		return spatial.CubeCoordinate{}, false
	}
	pos, ok := d.RoomPositions[roomID]
	return pos, ok
}

// ToAbsolute converts room-local coordinates to dungeon-absolute coordinates.
// Returns zero coordinate if room not found.
func (d *Dungeon) ToAbsolute(roomID string, local spatial.CubeCoordinate) spatial.CubeCoordinate {
	roomPos, ok := d.GetRoomPosition(roomID)
	if !ok {
		return spatial.CubeCoordinate{}
	}
	return spatial.CubeCoordinate{
		X: roomPos.X + local.X,
		Y: roomPos.Y + local.Y,
		Z: roomPos.Z + local.Z,
	}
}

// ToLocal converts dungeon-absolute coordinates to room-local coordinates.
// Returns zero coordinate if room not found.
func (d *Dungeon) ToLocal(roomID string, absolute spatial.CubeCoordinate) spatial.CubeCoordinate {
	roomPos, ok := d.GetRoomPosition(roomID)
	if !ok {
		return spatial.CubeCoordinate{}
	}
	return spatial.CubeCoordinate{
		X: absolute.X - roomPos.X,
		Y: absolute.Y - roomPos.Y,
		Z: absolute.Z - roomPos.Z,
	}
}

// GetAbsoluteWalls returns the walls for a room in dungeon-absolute coordinates.
// Returns nil if room not found.
func (d *Dungeon) GetAbsoluteWalls(roomID string) []dungeon.WallSegment {
	room := d.Rooms[roomID]
	if room == nil || len(room.Walls) == 0 {
		return nil
	}

	origin, ok := d.GetRoomPosition(roomID)
	if !ok {
		return nil
	}

	walls := make([]dungeon.WallSegment, len(room.Walls))
	for i, wall := range room.Walls {
		walls[i] = dungeon.WallSegment{
			ID:   wall.ID,
			Type: wall.Type,
			Start: dungeon.Position{
				X: wall.Start.X + origin.X,
				Y: wall.Start.Y + origin.Y,
				Z: wall.Start.Z + origin.Z,
			},
			End: dungeon.Position{
				X: wall.End.X + origin.X,
				Y: wall.End.Y + origin.Y,
				Z: wall.End.Z + origin.Z,
			},
			BlocksMovement:    wall.BlocksMovement,
			BlocksLineOfSight: wall.BlocksLineOfSight,
		}
	}
	return walls
}

// CalculateRoomPositions computes dungeon-absolute positions for all rooms.
// Uses BFS from the start room, positioning each room so doors align.
func (d *Dungeon) CalculateRoomPositions() {
	if len(d.Rooms) == 0 {
		return
	}

	d.RoomPositions = make(map[string]spatial.CubeCoordinate)

	// Start room at origin
	d.RoomPositions[d.StartRoomID] = spatial.CubeCoordinate{X: 0, Y: 0, Z: 0}

	// BFS to place remaining rooms
	placed := map[string]bool{d.StartRoomID: true}
	queue := []string{d.StartRoomID}

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		currentPos := d.RoomPositions[currentID]
		currentRoom := d.Rooms[currentID]
		if currentRoom == nil {
			continue
		}

		// Find all connections from this room
		for _, conn := range d.Connections {
			neighborID, currentDoorPos, neighborDoorPos, _, found := d.getUnplacedNeighborDoorPositions(
				conn, currentID, currentRoom, placed,
			)
			if !found {
				continue
			}

			neighborRoom := d.Rooms[neighborID]
			if neighborRoom == nil {
				continue
			}

			// Calculate neighbor's position so doors align (same absolute position)
			// neighborPos + neighborDoorPos = currentPos + currentDoorPos
			// neighborPos = currentPos + currentDoorPos - neighborDoorPos
			d.RoomPositions[neighborID] = spatial.CubeCoordinate{
				X: currentPos.X + currentDoorPos.X - neighborDoorPos.X,
				Y: currentPos.Y + currentDoorPos.Y - neighborDoorPos.Y,
				Z: currentPos.Z + currentDoorPos.Z - neighborDoorPos.Z,
			}

			placed[neighborID] = true
			queue = append(queue, neighborID)
		}
	}

	// Handle any disconnected rooms (shouldn't happen in valid dungeons)
	offset := 100
	for roomID := range d.Rooms {
		if !placed[roomID] {
			d.RoomPositions[roomID] = spatial.CubeCoordinate{X: offset, Y: 0, Z: -offset}
			offset += 100
		}
	}
}

// getUnplacedNeighborDoorPositions finds an unplaced neighbor and returns door positions.
// Uses the connection's FromPosition/ToPosition which already contain the door coordinates
// in each room's local coordinate system. Also returns the direction from current to neighbor.
func (d *Dungeon) getUnplacedNeighborDoorPositions(
	conn *environments.ConnectionEdge,
	currentID string,
	_ *dungeon.Room,
	placed map[string]bool,
) (neighborID string, currentDoorPos, neighborDoorPos spatial.CubeCoordinate, direction string, found bool) {
	switch {
	case conn.FromRoomID == currentID && !placed[conn.ToRoomID]:
		neighborID = conn.ToRoomID
		if d.Rooms[neighborID] == nil {
			return "", spatial.CubeCoordinate{}, spatial.CubeCoordinate{}, "", false
		}
		// Current room is "From", neighbor is "To"
		// Use the pre-computed door positions from the connection
		// Direction is parsed from connection type
		return neighborID, conn.FromPosition, conn.ToPosition, parseDirection(conn.Type), true

	case conn.ToRoomID == currentID && !placed[conn.FromRoomID] && conn.Bidirectional:
		neighborID = conn.FromRoomID
		if d.Rooms[neighborID] == nil {
			return "", spatial.CubeCoordinate{}, spatial.CubeCoordinate{}, "", false
		}
		// Current room is "To", neighbor is "From"
		// Use the pre-computed door positions from the connection (swapped)
		// Direction is opposite of connection type
		return neighborID, conn.ToPosition, conn.FromPosition, oppositeDirection(parseDirection(conn.Type)), true
	}

	return "", spatial.CubeCoordinate{}, spatial.CubeCoordinate{}, "", false
}

// oppositeDirection returns the opposite cardinal direction
func oppositeDirection(dir string) string {
	switch dir {
	case "north":
		return "south"
	case "south":
		return "north"
	case "east":
		return "west"
	case "west":
		return "east"
	case "northeast":
		return "southwest"
	case "southwest":
		return "northeast"
	case "northwest":
		return "southeast"
	case "southeast":
		return "northwest"
	default:
		return dir
	}
}

// findDoorPosition finds the door position for a room based on connection type.
// Connection type encodes "direction|hint" (e.g., "north|heavy wooden door").
// The _ parameter (isFrom) is reserved for future use to handle opposite directions.
func (d *Dungeon) findDoorPosition(room *dungeon.Room, connType string, _ bool) spatial.CubeCoordinate {
	if room == nil || room.Shape == nil {
		return spatial.CubeCoordinate{}
	}

	// Parse direction from connection type (format: "direction|hint")
	direction := connType
	for i, c := range connType {
		if c == '|' {
			direction = connType[:i]
			break
		}
	}

	// Find matching connection point by direction
	for _, cp := range room.Shape.ConnectionPoints {
		if cp.Direction == direction || cp.Name == direction {
			// ConnectionPoint.Position is already in cube coordinates (created via offsetToCube)
			// so we just copy the values directly
			return spatial.CubeCoordinate{
				X: cp.Position.X,
				Y: cp.Position.Y,
				Z: cp.Position.Z,
			}
		}
	}

	// Fallback: use room center edge based on direction
	// Width/Height are in offset coordinate terms (columns/rows)
	// Convert to cube coordinates: X = col, Z = row, Y = -X - Z
	width := room.Shape.Width
	height := room.Shape.Height
	midX := width / 2
	midZ := height / 2

	switch direction {
	case "north":
		// North edge: middle column, top row (row = height-1)
		x, z := midX, height-1
		return spatial.CubeCoordinate{X: x, Y: -x - z, Z: z}
	case "south":
		// South edge: middle column, bottom row (row = 0)
		x, z := midX, 0
		return spatial.CubeCoordinate{X: x, Y: -x - z, Z: z}
	case "east":
		// East edge: rightmost column (col = width-1), middle row
		x, z := width-1, midZ
		return spatial.CubeCoordinate{X: x, Y: -x - z, Z: z}
	case "west":
		// West edge: leftmost column (col = 0), middle row
		x, z := 0, midZ
		return spatial.CubeCoordinate{X: x, Y: -x - z, Z: z}
	default:
		// Default to center
		x, z := midX, midZ
		return spatial.CubeCoordinate{X: x, Y: -x - z, Z: z}
	}
}

// directionToCubeOffset returns a unit cube coordinate offset for a cardinal direction.
// This is used to add 1 hex of separation between rooms when positioning them.
func directionToCubeOffset(direction string) spatial.CubeCoordinate {
	// Hex cube coordinate offsets for 6 directions (pointy-top orientation)
	// Reference: https://www.redblobgames.com/grids/hexagons/#neighbors-cube
	switch direction {
	case "east":
		return spatial.CubeCoordinate{X: 1, Y: -1, Z: 0}
	case "west":
		return spatial.CubeCoordinate{X: -1, Y: 1, Z: 0}
	case "northeast":
		return spatial.CubeCoordinate{X: 1, Y: 0, Z: -1}
	case "northwest":
		return spatial.CubeCoordinate{X: 0, Y: 1, Z: -1}
	case "southeast":
		return spatial.CubeCoordinate{X: 0, Y: -1, Z: 1}
	case "southwest":
		return spatial.CubeCoordinate{X: -1, Y: 0, Z: 1}
	case "north":
		// For 4-direction systems, north maps to northwest
		return spatial.CubeCoordinate{X: 0, Y: 1, Z: -1}
	case "south":
		// For 4-direction systems, south maps to southeast
		return spatial.CubeCoordinate{X: 0, Y: -1, Z: 1}
	default:
		// No offset for unknown direction
		return spatial.CubeCoordinate{}
	}
}

// parseDirection extracts direction from connection type (format: "direction|hint")
func parseDirection(connType string) string {
	for i, c := range connType {
		if c == '|' {
			return connType[:i]
		}
	}
	return connType
}

// GetRoomAt finds which room contains a dungeon-absolute coordinate.
// Returns empty string and false if no room contains the coordinate.
func (d *Dungeon) GetRoomAt(coord spatial.CubeCoordinate) (string, bool) {
	for roomID, roomPos := range d.RoomPositions {
		room := d.Rooms[roomID]
		if room == nil {
			continue
		}

		// Convert to room-local coordinates
		local := spatial.CubeCoordinate{
			X: coord.X - roomPos.X,
			Y: coord.Y - roomPos.Y,
			Z: coord.Z - roomPos.Z,
		}

		// Check if within room bounds
		// Room dimensions are in grid units, cube coords use x for width, z for height
		if local.X >= 0 && local.X < room.Shape.Width &&
			local.Z >= -room.Shape.Height+1 && local.Z <= 0 {
			return roomID, true
		}
	}
	return "", false
}
