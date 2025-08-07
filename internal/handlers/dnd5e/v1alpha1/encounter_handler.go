package v1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// EncounterHandlerConfig holds dependencies for the encounter handler
type EncounterHandlerConfig struct {
	EncounterService encounter.Service
}

// Validate ensures all required dependencies are present
func (c *EncounterHandlerConfig) Validate() error {
	if c.EncounterService == nil {
		return errors.InvalidArgument("encounter service is required")
	}
	return nil
}

// EncounterHandler implements the D&D 5e encounter gRPC service
type EncounterHandler struct {
	dnd5ev1alpha1.UnimplementedEncounterServiceServer
	encounterService encounter.Service
}

// NewEncounterHandler creates a new encounter handler with the given configuration
func NewEncounterHandler(cfg *EncounterHandlerConfig) (*EncounterHandler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &EncounterHandler{
		encounterService: cfg.EncounterService,
	}, nil
}

// DungeonStart creates a simple dungeon encounter for testing
func (h *EncounterHandler) DungeonStart(
	ctx context.Context,
	req *dnd5ev1alpha1.DungeonStartRequest,
) (*dnd5ev1alpha1.DungeonStartResponse, error) {
	// Create input for orchestrator
	input := &encounter.DungeonStartInput{
		CharacterIDs: req.GetCharacterIds(),
	}

	// Call orchestrator
	output, err := h.encounterService.DungeonStart(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert spatial RoomData to proto Room
	protoRoom := convertRoomDataToProto(output.RoomData)

	return &dnd5ev1alpha1.DungeonStartResponse{
		EncounterId: output.EncounterID,
		Room:        protoRoom,
	}, nil
}

// convertRoomDataToProto converts spatial.RoomData to proto Room message
// This provides direct field mapping as per the architecture principles
func convertRoomDataToProto(roomData *spatial.RoomData) *dnd5ev1alpha1.Room {
	// Convert grid type string to proto enum
	var gridType apiv1alpha1.GridType
	switch roomData.GridType {
	case "square":
		gridType = apiv1alpha1.GridType_GRID_TYPE_SQUARE
	case "hex":
		// Default to pointy-top (D&D 5e standard)
		// Our orchestrator creates pointy-top hex grids
		gridType = apiv1alpha1.GridType_GRID_TYPE_HEX_POINTY
	case "gridless":
		gridType = apiv1alpha1.GridType_GRID_TYPE_GRIDLESS
	default:
		gridType = apiv1alpha1.GridType_GRID_TYPE_SQUARE // Default fallback
	}

	// Convert entities to proto format
	var protoEntities []*dnd5ev1alpha1.EntityPlacement
	for _, placement := range roomData.Entities {
		protoEntities = append(protoEntities, &dnd5ev1alpha1.EntityPlacement{
			EntityId:   placement.EntityID,
			EntityType: placement.EntityType,
			Position: &apiv1alpha1.Position{
				X: placement.Position.X,
				Y: placement.Position.Y,
				// Note: spatial.Position has X,Y, proto Position has optional Z
			},
		})
	}

	return &dnd5ev1alpha1.Room{
		Id:       roomData.ID,
		Type:     roomData.Type,
		Width:    int32(roomData.Width),
		Height:   int32(roomData.Height),
		GridType: gridType,
		Entities: protoEntities,
	}
}
