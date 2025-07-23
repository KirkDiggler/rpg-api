// Package room provides service interfaces for room generation and spatial queries.
package room

import (
	"context"
	"time"
	
	"github.com/KirkDiggler/rpg-api/internal/entities"
)

//go:generate mockgen -destination=mock/mock_service.go -package=roommock github.com/KirkDiggler/rpg-api/internal/services/room Service

// Service defines the room generation and spatial query interface
type Service interface {
	// Basic room generation
	GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error)
	GetRoom(ctx context.Context, input *GetRoomInput) (*GetRoomOutput, error)
	
	// Essential spatial queries
	QueryLineOfSight(ctx context.Context, input *QueryLineOfSightInput) (*QueryLineOfSightOutput, error)
	ValidateMovement(ctx context.Context, input *ValidateMovementInput) (*ValidateMovementOutput, error)  
	ValidateEntityPlacement(ctx context.Context, input *ValidateEntityPlacementInput) (*ValidateEntityPlacementOutput, error)
	QueryEntitiesInRange(ctx context.Context, input *QueryEntitiesInRangeInput) (*QueryEntitiesInRangeOutput, error)
}

// =============================================================================
// Service Input/Output Types
// =============================================================================

// GenerateRoomInput contains room generation parameters
type GenerateRoomInput struct {
	EntityID  string     `json:"entity_id"`  // Required: room owner
	Config    RoomConfig `json:"config"`     // Room generation configuration  
	Seed      int64      `json:"seed"`       // Required for reproducibility
	SessionID string     `json:"session_id,omitempty"`
	TTL       *int32     `json:"ttl,omitempty"`
}

// GenerateRoomOutput contains generated room result
type GenerateRoomOutput struct {
	Room *entities.Room `json:"room"`  // Uses internal entity types
}

// GetRoomInput contains room retrieval parameters
type GetRoomInput struct {
	RoomID   string `json:"room_id"`
	EntityID string `json:"entity_id"` // For ownership validation
}

// GetRoomOutput contains room details
type GetRoomOutput struct {
	Room     *entities.Room   `json:"room"`
	Entities []entities.Entity `json:"entities"`
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

// QueryLineOfSightInput contains line of sight query parameters
type QueryLineOfSightInput struct {
	RoomID      string   `json:"room_id"`
	FromX       float64  `json:"from_x"`
	FromY       float64  `json:"from_y"`
	ToX         float64  `json:"to_x"`
	ToY         float64  `json:"to_y"`
	EntitySize  float64  `json:"entity_size,omitempty"`  // Size for collision detection
	IgnoreTypes []string `json:"ignore_types,omitempty"` // Entity types to ignore
}

// QueryLineOfSightOutput contains line of sight results
type QueryLineOfSightOutput struct {
	HasLineOfSight   bool       `json:"has_line_of_sight"`
	BlockingEntityID *string    `json:"blocking_entity_id,omitempty"`
	BlockingPosition *Position  `json:"blocking_position,omitempty"`
	Distance         float64    `json:"distance"`        // Actual distance
	PathPositions    []Position `json:"path_positions"`  // LOS ray positions
}

// ValidateMovementInput contains movement validation parameters
type ValidateMovementInput struct {
	RoomID      string  `json:"room_id"`
	EntityID    string  `json:"entity_id"`          // Entity attempting movement
	FromX       float64 `json:"from_x"`
	FromY       float64 `json:"from_y"`
	ToX         float64 `json:"to_x"`
	ToY         float64 `json:"to_y"`
	EntitySize  float64 `json:"entity_size,omitempty"`
	MaxDistance float64 `json:"max_distance,omitempty"`
}

// ValidateMovementOutput contains movement validation results
type ValidateMovementOutput struct {
	IsValid          bool      `json:"is_valid"`
	BlockedBy        *string   `json:"blocked_by,omitempty"`     // Entity ID blocking path
	BlockingPosition *Position `json:"blocking_position,omitempty"`
	MovementCost     float64   `json:"movement_cost"`            // Cost in movement points
	ActualDistance   float64   `json:"actual_distance"`          // Calculated distance
}

// ValidateEntityPlacementInput contains entity placement parameters
type ValidateEntityPlacementInput struct {
	RoomID     string                 `json:"room_id"`
	EntityID   string                 `json:"entity_id,omitempty"`    // For updates
	EntityType string                 `json:"entity_type"`
	Position   Position               `json:"position"`
	Size       float64                `json:"size"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
}

// ValidateEntityPlacementOutput contains placement validation results
type ValidateEntityPlacementOutput struct {
	CanPlace           bool             `json:"can_place"`
	ConflictingIDs     []string         `json:"conflicting_ids"`     // Conflicting entity IDs
	SuggestedPositions []Position       `json:"suggested_positions"` // Alternative positions
	PlacementIssues    []PlacementIssue `json:"placement_issues"`
}

// QueryEntitiesInRangeInput contains range query parameters
type QueryEntitiesInRangeInput struct {
	RoomID          string   `json:"room_id"`
	CenterX         float64  `json:"center_x"`
	CenterY         float64  `json:"center_y"`
	Range           float64  `json:"range"`
	EntityTypes     []string `json:"entity_types,omitempty"`     // Filter by type
	Tags            []string `json:"tags,omitempty"`             // Filter by tags
	ExcludeEntityID string   `json:"exclude_entity_id,omitempty"` // Exclude specific entity
}

// QueryEntitiesInRangeOutput contains range query results
type QueryEntitiesInRangeOutput struct {
	Entities    []EntityResult `json:"entities"`
	TotalFound  int32          `json:"total_found"`
	QueryCenter Position       `json:"query_center"`
	QueryRange  float64        `json:"query_range"`
}

// =============================================================================
// Supporting Types
// =============================================================================

// Position represents a 3D coordinate
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z,omitempty"` // Optional for 3D support
}

// PlacementIssue represents an entity placement issue
type PlacementIssue struct {
	Type        string   `json:"type"`        // "collision", "out_of_bounds", "invalid_terrain"
	Description string   `json:"description"`
	Position    Position `json:"position"`
	Severity    string   `json:"severity"`    // "error", "warning", "info"
}

// EntityResult represents an entity found in a range query
type EntityResult struct {
	Entity      entities.Entity `json:"entity"`
	Distance    float64         `json:"distance"`     // Distance from query center
	Direction   float64         `json:"direction"`    // Angle from center (radians)
	RelativePos string          `json:"relative_pos"` // "north", "southeast", etc.
}