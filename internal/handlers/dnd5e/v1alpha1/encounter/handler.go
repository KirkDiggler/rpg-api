// Package encounter handles the gRPC service interface for encounter management
package encounter

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
)

// HandlerConfig holds dependencies for the handler
type HandlerConfig struct {
	EncounterService encounter.Service
}

// Validate ensures all required dependencies are present
func (c *HandlerConfig) Validate() error {
	if c.EncounterService == nil {
		return errors.InvalidArgument("encounter service is required")
	}
	return nil
}

// Handler implements the D&D 5e Encounter gRPC service
type Handler struct {
	dnd5ev1alpha1.UnimplementedEncounterServiceServer
	encounterService encounter.Service
}

// New creates a new handler with the given configuration
func New(cfg *HandlerConfig) (*Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Handler{
		encounterService: cfg.EncounterService,
	}, nil
}

// Attack handles attack requests in combat
func (h *Handler) Attack(ctx context.Context, req *dnd5ev1alpha1.AttackRequest) (*dnd5ev1alpha1.AttackResponse, error) {
	// 1. Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetAttackerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "attacker_id is required")
	}
	if req.GetTargetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_id is required")
	}

	// 2. Create service input
	input := &encounter.ResolveAttackInput{
		EncounterID: req.GetEncounterId(),
		AttackerID:  req.GetAttackerId(),
		TargetID:    req.GetTargetId(),
		WeaponID:    req.GetWeaponId(), // Optional
	}

	// 3. Call service
	output, err := h.encounterService.ResolveAttack(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 4. Convert to proto response
	return &dnd5ev1alpha1.AttackResponse{
		Success: true,
		Result:  convertAttackResultToProto(output.Result),
		// TODO: Add combat_state, updated_room when available
	}, nil
}

// DungeonStart handles dungeon start requests
func (h *Handler) DungeonStart(
	ctx context.Context,
	req *dnd5ev1alpha1.DungeonStartRequest,
) (*dnd5ev1alpha1.DungeonStartResponse, error) {
	// 1. Create service input
	// Phase 2: Minimal - character_ids exist in proto but we're just creating encounter for now
	input := &encounter.CreateDungeonInput{
		// Phase 2: Just track that dungeon was started, ignore character_ids for now
		PlayerID: "", // No player tracking yet in Phase 2
	}

	// 2. Call service
	output, err := h.encounterService.CreateDungeon(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 3. Convert to proto response
	return &dnd5ev1alpha1.DungeonStartResponse{
		EncounterId: output.EncounterID,
		Room:        convertRoomDataToProto(output.Room),
		// TODO Phase 3: Add CombatState when combat initialization is implemented
	}, nil
}

// GetCombatState retrieves current combat state
func (h *Handler) GetCombatState(
	_ context.Context,
	_ *dnd5ev1alpha1.GetCombatStateRequest,
) (*dnd5ev1alpha1.GetCombatStateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetCombatState endpoint not yet implemented")
}

// MoveCharacter handles character movement
func (h *Handler) MoveCharacter(
	ctx context.Context,
	req *dnd5ev1alpha1.MoveCharacterRequest,
) (*dnd5ev1alpha1.MoveCharacterResponse, error) {
	// 1. Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetEntityId() == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}
	if len(req.GetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}

	// 2. Create service input - use last position in path as target
	lastPos := req.GetPath()[len(req.GetPath())-1]
	input := &encounter.MoveCharacterInput{
		EncounterID: req.GetEncounterId(),
		EntityID:    req.GetEntityId(),
		TargetPosition: &encounter.Position{
			X: float64(lastPos.GetX()),
			Y: float64(lastPos.GetY()),
		},
	}

	// 3. Call service
	output, err := h.encounterService.MoveCharacter(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 4. Convert to proto response
	response := &dnd5ev1alpha1.MoveCharacterResponse{
		Success:           output.Success,
		MovementRemaining: output.MovementRemaining,
	}

	// 5. Convert room data if present
	if output.UpdatedRoom != nil {
		response.UpdatedRoom = convertRoomDataToProto(output.UpdatedRoom)
	}

	return response, nil
}

// EndTurn handles turn ending
func (h *Handler) EndTurn(_ context.Context, _ *dnd5ev1alpha1.EndTurnRequest) (*dnd5ev1alpha1.EndTurnResponse, error) {
	return nil, status.Error(codes.Unimplemented, "EndTurn endpoint not yet implemented")
}
