package sandbox_room

import (
	"time"

	roomcommon "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	sandboxcommon "github.com/KirkDiggler/rpg-api-protos/gen/go/sandbox/api/v1alpha1"
)

// GenerateRoomInput defines the request for generating a room
type GenerateRoomInput struct {
	Config *sandboxcommon.GenerativeRoomConfig
	Seed   int64
}

// GenerateRoomOutput defines the response for generating a room
type GenerateRoomOutput struct {
	RoomID   string
	Room     *roomcommon.Room
	Metrics  *sandboxcommon.GenerationMetrics
	Warnings []string
}

// BuildStaticRoomInput defines the request for building a static room
type BuildStaticRoomInput struct {
	Config *sandboxcommon.StaticRoomConfig
}

// BuildStaticRoomOutput defines the response for building a static room
type BuildStaticRoomOutput struct {
	RoomID string
	Room   *roomcommon.Room
}

// GetRoomInput defines the request for getting a room
type GetRoomInput struct {
	RoomID string
}

// GetRoomOutput defines the response for getting a room
type GetRoomOutput struct {
	Room *roomcommon.Room
}

// ListRoomsInput defines the request for listing rooms
type ListRoomsInput struct {
	PageSize     int32
	PageToken    int32
	RoomType     string
	CreatedAfter *time.Time
}

// ListRoomsOutput defines the response for listing rooms
type ListRoomsOutput struct {
	Rooms         []*roomcommon.Room
	NextPageToken int32
	TotalCount    int32
}

// DeleteRoomInput defines the request for deleting a room
type DeleteRoomInput struct {
	RoomID string
	Reason string // For audit trail
}

// DeleteRoomOutput defines the response for deleting a room
type DeleteRoomOutput struct {
	Success     bool
	HistoryID   string
	EntityCount int32
	WallCount   int32
}

// CheckLineOfSightInput defines the request for line of sight checks
type CheckLineOfSightInput struct {
	RoomID string
	From   *roomcommon.Position
	To     *roomcommon.Position
}

// CheckLineOfSightOutput defines the response for line of sight checks
type CheckLineOfSightOutput struct {
	IsClear          bool
	BlockedPositions []*roomcommon.Position
	PathPositions    []*roomcommon.Position
}

// FindPathInput defines the request for pathfinding
type FindPathInput struct {
	RoomID     string
	From       *roomcommon.Position
	To         *roomcommon.Position
	EntitySize *sandboxcommon.EntitySize
}

// FindPathOutput defines the response for pathfinding
type FindPathOutput struct {
	PathExists    bool
	Path          []*roomcommon.Position
	TotalDistance float64
}

// CalculateDistanceInput defines the request for distance calculation
type CalculateDistanceInput struct {
	RoomID string
	From   *roomcommon.Position
	To     *roomcommon.Position
}

// CalculateDistanceOutput defines the response for distance calculation
type CalculateDistanceOutput struct {
	Distance float64
	GridType roomcommon.GridType
}

// GetPositionsInRangeInput defines the request for getting positions in range
type GetPositionsInRangeInput struct {
	RoomID   string
	Center   *roomcommon.Position
	Range    float64
	GridType roomcommon.GridType
}

// GetPositionsInRangeOutput defines the response for getting positions in range
type GetPositionsInRangeOutput struct {
	Positions []*roomcommon.Position
	Count     int32
}

// PlaceEntityInput defines the request for placing an entity
type PlaceEntityInput struct {
	RoomID   string
	Entity   *roomcommon.Entity
	Position *roomcommon.Position
}

// PlaceEntityOutput defines the response for placing an entity
type PlaceEntityOutput struct {
	Success  bool
	EntityID string
	Reason   string
}

// MoveEntityInput defines the request for moving an entity
type MoveEntityInput struct {
	RoomID       string
	EntityID     string
	NewPosition  *roomcommon.Position
	ValidatePath bool
}

// MoveEntityOutput defines the response for moving an entity
type MoveEntityOutput struct {
	Success     bool
	OldPosition *roomcommon.Position
	NewPosition *roomcommon.Position
	Reason      string
}

// RemoveEntityInput defines the request for removing an entity
type RemoveEntityInput struct {
	RoomID   string
	EntityID string
}

// RemoveEntityOutput defines the response for removing an entity
type RemoveEntityOutput struct {
	Success       bool
	RemovedEntity *roomcommon.Entity
	Position      *roomcommon.Position
}

// GetEntitiesInRoomInput defines the request for getting entities in a room
type GetEntitiesInRoomInput struct {
	RoomID string
}

// GetEntitiesInRoomOutput defines the response for getting entities in a room
type GetEntitiesInRoomOutput struct {
	Entities []*roomcommon.EntityPlacement
	Count    int32
}
