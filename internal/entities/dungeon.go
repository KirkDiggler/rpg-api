// Package entities defines the core data structures for the RPG API
package entities

import (
	"time"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
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
