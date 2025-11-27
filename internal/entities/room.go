// Package entities defines the core data structures for the RPG API
package entities

import (
	"time"

	roomcommon "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// Room represents the persistent state of a room for tactical gameplay
// Purpose: Stores room data including spatial information and metadata
type Room struct {
	ID             string              `json:"id"`
	Type           string              `json:"type"`
	Width          int                 `json:"width"`
	Height         int                 `json:"height"`
	GridType       roomcommon.GridType `json:"grid_type"`
	GenerationSeed int64               `json:"generation_seed"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
	SpatialRoom    spatial.Room        `json:"-"`               // Toolkit room for queries - not serialized
	SerializedRoom []byte              `json:"serialized_room"` // JSON serialization for persistence
}

// RoomHistory represents historical metadata for deleted rooms
// Purpose: Audit trail for room deletion operations
type RoomHistory struct {
	ID              string              `json:"id"`
	OriginalID      string              `json:"original_id"`
	Type            string              `json:"type"`
	Width           int                 `json:"width"`
	Height          int                 `json:"height"`
	GridType        roomcommon.GridType `json:"grid_type"`
	GenerationSeed  int64               `json:"generation_seed"`
	EntityCount     int                 `json:"entity_count"`
	WallCount       int                 `json:"wall_count"`
	DeletedAt       time.Time           `json:"deleted_at"`
	DeleteReason    string              `json:"delete_reason"`
	CreatedAt       time.Time           `json:"created_at"`
	LifespanSeconds int64               `json:"lifespan_seconds"`
}

// BasicEntity represents a simple entity for spatial room integration
// Purpose: Minimal entity implementation for rpg-toolkit spatial compatibility
type BasicEntity struct {
	ID   string
	Type string
	Size int
}

// GetID returns the entity's unique identifier
func (e *BasicEntity) GetID() string {
	return e.ID
}

// GetType returns the entity's type
func (e *BasicEntity) GetType() string {
	return e.Type
}

// GetSize returns the entity's size for movement validation
func (e *BasicEntity) GetSize() int {
	return e.Size
}

// BlocksMovement returns whether this entity blocks movement
func (e *BasicEntity) BlocksMovement() bool {
	return e.Type == "wall"
}

// BlocksLineOfSight returns whether this entity blocks line of sight
func (e *BasicEntity) BlocksLineOfSight() bool {
	return e.Type == "wall"
}
