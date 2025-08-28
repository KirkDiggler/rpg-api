package sandbox_room

//go:generate mockgen -destination=mock/mock_service.go -package=sandboxroommock github.com/KirkDiggler/rpg-api/internal/services/sandbox_room Service

import (
	"context"
)

// Service defines the interface for sandbox room operations
// Purpose: Provides room generation, spatial queries, and entity management for tactical gameplay
type Service interface {
	// Room Generation - Core room creation and management
	GenerateRoom(ctx context.Context, input *GenerateRoomInput) (*GenerateRoomOutput, error)
	BuildStaticRoom(ctx context.Context, input *BuildStaticRoomInput) (*BuildStaticRoomOutput, error)
	GetRoom(ctx context.Context, input *GetRoomInput) (*GetRoomOutput, error)
	ListRooms(ctx context.Context, input *ListRoomsInput) (*ListRoomsOutput, error)
	DeleteRoom(ctx context.Context, input *DeleteRoomInput) (*DeleteRoomOutput, error)

	// Spatial Queries - Pathfinding and line of sight for tactical gameplay
	CheckLineOfSight(ctx context.Context, input *CheckLineOfSightInput) (*CheckLineOfSightOutput, error)
	FindPath(ctx context.Context, input *FindPathInput) (*FindPathOutput, error)
	CalculateDistance(ctx context.Context, input *CalculateDistanceInput) (*CalculateDistanceOutput, error)
	GetPositionsInRange(ctx context.Context, input *GetPositionsInRangeInput) (*GetPositionsInRangeOutput, error)

	// Entity Management - Place and move entities within rooms
	PlaceEntity(ctx context.Context, input *PlaceEntityInput) (*PlaceEntityOutput, error)
	MoveEntity(ctx context.Context, input *MoveEntityInput) (*MoveEntityOutput, error)
	RemoveEntity(ctx context.Context, input *RemoveEntityInput) (*RemoveEntityOutput, error)
	GetEntitiesInRoom(ctx context.Context, input *GetEntitiesInRoomInput) (*GetEntitiesInRoomOutput, error)
}
