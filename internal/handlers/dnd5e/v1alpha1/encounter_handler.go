package v1alpha1

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
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

	// Convert initiative data to proto CombatState
	protoCombatState := convertInitiativeDataToProto(
		output.EncounterID,
		output.InitiativeData,
		output.InitiativeRolls,
		output.CurrentTurn,
	)

	return &dnd5ev1alpha1.DungeonStartResponse{
		EncounterId: output.EncounterID,
		Room:        protoRoom,
		CombatState: protoCombatState,
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
		// Our orchestrator always creates pointy-top hex grids (D&D 5e standard)
		gridType = apiv1alpha1.GridType_GRID_TYPE_HEX_POINTY
	case "gridless":
		gridType = apiv1alpha1.GridType_GRID_TYPE_GRIDLESS
	default:
		// Default fallback for unknown grid types
		gridType = apiv1alpha1.GridType_GRID_TYPE_SQUARE
	}

	// Convert entities to proto format (map structure)
	protoEntities := make(map[string]*dnd5ev1alpha1.EntityPlacement)
	for entityID, placement := range roomData.Entities {
		protoEntities[entityID] = &dnd5ev1alpha1.EntityPlacement{
			EntityId:   placement.EntityID,
			EntityType: placement.EntityType,
			Position: &apiv1alpha1.Position{
				X: placement.Position.X,
				Y: placement.Position.Y,
				// Note: spatial.Position has X,Y, proto Position has optional Z
			},
			Size:              int32(placement.Size),
			BlocksMovement:    placement.BlocksMovement,
			BlocksLineOfSight: placement.BlocksLineOfSight,
		}
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

// convertInitiativeDataToProto converts toolkit initiative data to proto CombatState
func convertInitiativeDataToProto(
	encounterID string,
	initiativeData *initiative.TrackerData,
	initiativeRolls []initiative.Roll,
	currentTurn string,
) *dnd5ev1alpha1.CombatState {
	if initiativeData == nil {
		return nil
	}

	// Create a map from entity ID to roll data for quick lookup
	rollsByID := make(map[string]initiative.Roll)
	for _, roll := range initiativeRolls {
		rollsByID[roll.Entity.GetID()] = roll
	}

	// Convert turn order with actual initiative values
	var turnOrder []*dnd5ev1alpha1.InitiativeEntry
	for _, entity := range initiativeData.Order {
		entry := &dnd5ev1alpha1.InitiativeEntry{
			EntityId:   entity.ID,
			EntityType: entity.Type,
			HasActed:   false,
		}

		// Set actual initiative values if we have them
		if roll, ok := rollsByID[entity.ID]; ok {
			entry.Initiative = int32(roll.Total)
			entry.Modifier = int32(roll.Modifier)
		} else {
			// Fallback values if we don't have roll data
			entry.Initiative = 10
			entry.Modifier = 0
		}

		turnOrder = append(turnOrder, entry)
	}

	// Create current turn state
	var currentTurnState *dnd5ev1alpha1.TurnState
	if currentTurn != "" {
		currentTurnState = &dnd5ev1alpha1.TurnState{
			EntityId:          currentTurn,
			MovementUsed:      0,
			MovementMax:       30, // Standard movement for now
			ActionUsed:        false,
			BonusActionUsed:   false,
			ReactionAvailable: true,
			// Position will be set from room data
		}
	}

	return &dnd5ev1alpha1.CombatState{
		EncounterId:   encounterID,
		Round:         int32(initiativeData.Round),
		TurnOrder:     turnOrder,
		ActiveIndex:   int32(initiativeData.Current),
		CurrentTurn:   currentTurnState,
		CombatStarted: true,
		CombatEnded:   false,
	}
}

// GetCombatState retrieves current state (mainly for reconnection)
func (h *EncounterHandler) GetCombatState(
	ctx context.Context,
	req *dnd5ev1alpha1.GetCombatStateRequest,
) (*dnd5ev1alpha1.GetCombatStateResponse, error) {
	// For now, return unimplemented as we focus on the core gameplay flow
	// This will be implemented when we need reconnection support
	return nil, status.Error(codes.Unimplemented, "GetCombatState not yet implemented")
}

// MoveCharacter moves an entity to a new position
func (h *EncounterHandler) MoveCharacter(
	ctx context.Context,
	req *dnd5ev1alpha1.MoveCharacterRequest,
) (*dnd5ev1alpha1.MoveCharacterResponse, error) {
	// Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetEntityId() == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}
	if req.GetTargetPosition() == nil {
		return nil, status.Error(codes.InvalidArgument, "target_position is required")
	}

	// Call orchestrator
	moveInput := &encounter.MoveCharacterInput{
		EncounterID: req.GetEncounterId(),
		EntityID:    req.GetEntityId(),
		TargetPosition: spatial.Position{
			X: req.GetTargetPosition().GetX(),
			Y: req.GetTargetPosition().GetY(),
		},
	}

	moveOutput, err := h.encounterService.MoveCharacter(ctx, moveInput)
	if err != nil {
		// Convert specific errors to appropriate gRPC codes
		if errors.IsInvalidArgument(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		if errors.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert updated room data to proto
	var updatedRoom *dnd5ev1alpha1.Room
	if moveOutput.RoomData != nil {
		updatedRoom = convertRoomDataToProto(moveOutput.RoomData)
		slog.Info("Converted room data to proto",
			"room_id", updatedRoom.GetId(),
			"entities_count", len(updatedRoom.GetEntities()),
		)
	} else {
		slog.Warn("No room data in move output")
	}

	// Return response with movement details and updated room
	resp := &dnd5ev1alpha1.MoveCharacterResponse{
		Success:           moveOutput.Success,
		MovementRemaining: int32(moveOutput.MovementLeft),
		UpdatedRoom:       updatedRoom,
		// CombatState could be populated here if needed for turn state updates
	}
	
	slog.Info("Returning MoveCharacterResponse",
		"success", resp.Success,
		"movement_remaining", resp.MovementRemaining,
		"has_updated_room", resp.UpdatedRoom != nil,
	)
	
	return resp, nil
}

// EndTurn advances to the next turn
func (h *EncounterHandler) EndTurn(
	ctx context.Context,
	req *dnd5ev1alpha1.EndTurnRequest,
) (*dnd5ev1alpha1.EndTurnResponse, error) {
	// Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}

	// Call orchestrator to advance turn
	nextTurnInput := &encounter.NextTurnInput{
		EncounterID: req.GetEncounterId(),
	}

	nextTurnOutput, err := h.encounterService.NextTurn(ctx, nextTurnInput)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Get updated turn order to return full state
	turnOrderInput := &encounter.GetTurnOrderInput{
		EncounterID: req.GetEncounterId(),
	}

	turnOrderOutput, err := h.encounterService.GetTurnOrder(ctx, turnOrderInput)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert to proto CombatState
	protoCombatState := convertInitiativeDataToProto(
		req.GetEncounterId(),
		turnOrderOutput.InitiativeData,
		turnOrderOutput.InitiativeRolls,
		nextTurnOutput.CurrentTurn,
	)

	// For now, we don't return room data in EndTurn
	// In the future, we might include it if entities moved
	return &dnd5ev1alpha1.EndTurnResponse{
		CombatState: protoCombatState,
	}, nil
}

// Attack performs an attack action
func (h *EncounterHandler) Attack(
	ctx context.Context,
	req *dnd5ev1alpha1.AttackRequest,
) (*dnd5ev1alpha1.AttackResponse, error) {
	// For now, return unimplemented
	// Combat will be implemented last
	return nil, status.Error(codes.Unimplemented, "Attack not yet implemented")
}
