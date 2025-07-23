// Package entities provides core data structures for rpg-api.
package entities

import (
	"time"
)

// Room represents a generated tactical room
type Room struct {
	ID          string                 `json:"id"`
	EntityID    string                 `json:"entity_id"`    // Owner (following entity ownership pattern)
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Config      RoomConfig             `json:"config"`
	Dimensions  Dimensions             `json:"dimensions"`
	GridInfo    GridInformation        `json:"grid_info"`
	Properties  map[string]interface{} `json:"properties"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	ExpiresAt   time.Time              `json:"expires_at"`
}

// Entity represents an entity within a room
type Entity struct {
	ID         string                 `json:"id"`
	RoomID     string                 `json:"room_id"`
	Type       string                 `json:"type"`        // "wall", "door", "monster", "character"
	Position   Position               `json:"position"`
	Properties map[string]interface{} `json:"properties"`  // Size, material, health, etc.
	Tags       []string               `json:"tags"`        // "destructible", "blocking", "cover"
	State      EntityState            `json:"state"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// RoomConfig defines room generation parameters
type RoomConfig struct {
	Width       int32   `json:"width"`         // Room width in grid units
	Height      int32   `json:"height"`        // Room height in grid units
	Theme       string  `json:"theme"`         // "dungeon", "forest", "urban", etc.
	WallDensity float64 `json:"wall_density"`  // 0.0-1.0 wall coverage
	Pattern     string  `json:"pattern"`       // "empty", "random", "clustered"
	GridType    string  `json:"grid_type"`     // "square", "hex_pointy", "hex_flat", "gridless"
	GridSize    float64 `json:"grid_size"`     // 5.0 for D&D 5ft squares
}

// Position represents a 3D coordinate
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z,omitempty"` // Optional for 3D support
}

// EntityState represents entity gameplay state
type EntityState struct {
	BlocksMovement    bool  `json:"blocks_movement"`
	BlocksLineOfSight bool  `json:"blocks_line_of_sight"`
	Destroyed         bool  `json:"destroyed"`
	CurrentHP         int32 `json:"current_hp,omitempty"`
	MaxHP             int32 `json:"max_hp,omitempty"`
}

// Dimensions represents room dimensions
type Dimensions struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Depth  float64 `json:"depth,omitempty"`
}

// GridInformation contains grid system details
type GridInformation struct {
	Type string  `json:"type"` // "square", "hex_pointy", "hex_flat", "gridless"
	Size float64 `json:"size"` // Size of each grid cell
}