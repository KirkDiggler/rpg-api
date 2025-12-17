// Package entities defines the core data structures for the RPG API
package entities

import "time"

// DungeonTheme defines the thematic style of a dungeon
type DungeonTheme int

const (
	DungeonThemeUnspecified DungeonTheme = iota
	DungeonThemeCrypt                    // Undead, structured stone, tombs
	DungeonThemeCave                     // Beasts, organic shapes, natural caverns
	DungeonThemeRuins                    // Mixed creatures, ancient structures
)

// DungeonDifficulty controls encounter CR scaling
type DungeonDifficulty int

const (
	DungeonDifficultyUnspecified DungeonDifficulty = iota
	DungeonDifficultyEasy                          // Lower CR encounters
	DungeonDifficultyMedium                        // Standard CR encounters
	DungeonDifficultyHard                          // Higher CR encounters
)

// DungeonLength determines the number of rooms
type DungeonLength int

const (
	DungeonLengthUnspecified DungeonLength = iota
	DungeonLengthShort                     // 3-4 rooms
	DungeonLengthMedium                    // 5-7 rooms
	DungeonLengthLong                      // 8-10 rooms
)

// DungeonState tracks the lifecycle of a dungeon run
type DungeonState int

const (
	DungeonStateUnspecified DungeonState = iota
	DungeonStateActive                   // Dungeon in progress
	DungeonStateVictorious               // Boss defeated, dungeon complete
	DungeonStateFailed                   // Party wiped (TPK)
	DungeonStateAbandoned                // Players left the dungeon
)

// DungeonRoom represents a room within a dungeon
type DungeonRoom struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`     // "entrance", "boss", "corridor", "chamber", etc.
	Width    int      `json:"width"`    // Room width in grid units
	Height   int      `json:"height"`   // Room height in grid units
	Theme    string   `json:"theme"`    // Visual theme for this room
	IsBoss   bool     `json:"is_boss"`  // Whether this is a boss room
	Monsters []string `json:"monsters"` // Monster IDs spawned in this room
}

// DungeonConnection represents a connection between two rooms
type DungeonConnection struct {
	ID           string    `json:"id"`
	FromRoomID   string    `json:"from_room_id"`
	ToRoomID     string    `json:"to_room_id"`
	FromPosition *Position `json:"from_position"` // Door position in source room
	ToPosition   *Position `json:"to_position"`   // Door position in destination room
	PhysicalHint string    `json:"physical_hint"` // "heavy stone door", "narrow crevice", etc.
}

// Dungeon represents a complete dungeon run with its state
type Dungeon struct {
	ID          string `json:"id"`
	EncounterID string `json:"encounter_id"` // Link to the associated encounter

	// Configuration
	Theme      DungeonTheme      `json:"theme"`
	Difficulty DungeonDifficulty `json:"difficulty"`
	Length     DungeonLength     `json:"length"`
	Seed       int64             `json:"seed"` // Random seed for reproducibility

	// Current state
	State DungeonState `json:"state"`

	// Map structure
	Rooms         map[string]*DungeonRoom `json:"rooms"`
	Connections   []*DungeonConnection    `json:"connections"`
	StartRoomID   string                  `json:"start_room_id"`
	BossRoomID    string                  `json:"boss_room_id"`
	CurrentRoomID string                  `json:"current_room_id"` // Room players are currently in

	// Exploration state
	RevealedRooms map[string]bool `json:"revealed_rooms"` // Room ID -> explored
	OpenDoors     map[string]bool `json:"open_doors"`     // Connection ID -> open

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
func (d *Dungeon) GetRoom(roomID string) *DungeonRoom {
	if d.Rooms == nil {
		return nil
	}
	return d.Rooms[roomID]
}

// GetConnectionsFromRoom returns all connections originating from a room
func (d *Dungeon) GetConnectionsFromRoom(roomID string) []*DungeonConnection {
	var result []*DungeonConnection
	for _, conn := range d.Connections {
		if conn.FromRoomID == roomID || conn.ToRoomID == roomID {
			result = append(result, conn)
		}
	}
	return result
}

// GetVisibleDoors returns connections from the current room that are visible (not yet open)
func (d *Dungeon) GetVisibleDoors() []*DungeonConnection {
	connections := d.GetConnectionsFromRoom(d.CurrentRoomID)
	var visible []*DungeonConnection
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
