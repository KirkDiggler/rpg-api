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
	_ context.Context,
	_ *dnd5ev1alpha1.DungeonStartRequest,
) (*dnd5ev1alpha1.DungeonStartResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DungeonStart endpoint not yet implemented")
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
	_ context.Context,
	_ *dnd5ev1alpha1.MoveCharacterRequest,
) (*dnd5ev1alpha1.MoveCharacterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "MoveCharacter endpoint not yet implemented")
}

// EndTurn handles turn ending
func (h *Handler) EndTurn(_ context.Context, _ *dnd5ev1alpha1.EndTurnRequest) (*dnd5ev1alpha1.EndTurnResponse, error) {
	return nil, status.Error(codes.Unimplemented, "EndTurn endpoint not yet implemented")
}
