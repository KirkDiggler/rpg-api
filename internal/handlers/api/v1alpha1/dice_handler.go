// Package v1alpha1 handles the generic API grpc service interface
package v1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/api/v1alpha1"
)

// DiceHandlerConfig holds dependencies for the dice handler
type DiceHandlerConfig struct {
	DiceService dice.Service
}

// Validate ensures all required dependencies are present
func (c *DiceHandlerConfig) Validate() error {
	if c.DiceService == nil {
		return errors.InvalidArgument("dice service is required")
	}
	return nil
}

// DiceHandler implements the generic dice gRPC service
type DiceHandler struct {
	apiv1alpha1.UnimplementedDiceServiceServer
	diceService dice.Service
}

// NewDiceHandler creates a new dice handler with the given configuration
func NewDiceHandler(cfg *DiceHandlerConfig) (*DiceHandler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &DiceHandler{
		diceService: cfg.DiceService,
	}, nil
}

// RollDice rolls dice using the specified notation and stores the result in a session
func (h *DiceHandler) RollDice(
	ctx context.Context,
	req *apiv1alpha1.RollDiceRequest,
) (*apiv1alpha1.RollDiceResponse, error) {
	if req.EntityId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("entity_id is required"))
	}
	if req.Context == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("context is required"))
	}
	if req.Notation == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("notation is required"))
	}

	// Use the dice service to roll dice
	diceInput := &dice.RollDiceInput{
		EntityID:    req.EntityId,
		Context:     req.Context,
		Notation:    req.Notation,
		Description: req.ModifierDescription,
	}

	diceOutput, err := h.diceService.RollDice(ctx, diceInput)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	// Convert all session rolls to proto format
	rolls := make([]*apiv1alpha1.DiceRoll, 0, len(diceOutput.Session.Rolls))
	for _, sessionRoll := range diceOutput.Session.Rolls {
		rolls = append(rolls, &apiv1alpha1.DiceRoll{
			RollId:      sessionRoll.RollID,
			Notation:    sessionRoll.Notation,
			Dice:        sessionRoll.Dice,
			Total:       sessionRoll.Total,
			Dropped:     sessionRoll.Dropped,
			Description: sessionRoll.Description,
			DiceTotal:   sessionRoll.DiceTotal,
			Modifier:    sessionRoll.Modifier,
		})
	}

	return &apiv1alpha1.RollDiceResponse{
		Rolls:     rolls,
		ExpiresAt: diceOutput.Session.ExpiresAt.Unix(),
	}, nil
}

// GetRollSession retrieves an existing dice roll session
func (h *DiceHandler) GetRollSession(
	ctx context.Context,
	req *apiv1alpha1.GetRollSessionRequest,
) (*apiv1alpha1.GetRollSessionResponse, error) {
	if req.EntityId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("entity_id is required"))
	}
	if req.Context == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("context is required"))
	}

	// Use the dice service to get the session
	diceInput := &dice.GetRollSessionInput{
		EntityID: req.EntityId,
		Context:  req.Context,
	}

	diceOutput, err := h.diceService.GetRollSession(ctx, diceInput)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	// Convert session rolls to proto format
	rolls := make([]*apiv1alpha1.DiceRoll, 0, len(diceOutput.Session.Rolls))
	for _, sessionRoll := range diceOutput.Session.Rolls {
		rolls = append(rolls, &apiv1alpha1.DiceRoll{
			RollId:      sessionRoll.RollID,
			Notation:    sessionRoll.Notation,
			Dice:        sessionRoll.Dice,
			Total:       sessionRoll.Total,
			Dropped:     sessionRoll.Dropped,
			Description: sessionRoll.Description,
			DiceTotal:   sessionRoll.DiceTotal,
			Modifier:    sessionRoll.Modifier,
		})
	}

	return &apiv1alpha1.GetRollSessionResponse{
		Rolls:     rolls,
		ExpiresAt: diceOutput.Session.ExpiresAt.Unix(),
		CreatedAt: diceOutput.Session.CreatedAt.Unix(),
	}, nil
}

// ClearRollSession removes a dice roll session
func (h *DiceHandler) ClearRollSession(
	ctx context.Context,
	req *apiv1alpha1.ClearRollSessionRequest,
) (*apiv1alpha1.ClearRollSessionResponse, error) {
	if req.EntityId == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("entity_id is required"))
	}
	if req.Context == "" {
		return nil, errors.ToGRPCError(errors.InvalidArgument("context is required"))
	}

	// Use the dice service to clear the session
	diceInput := &dice.ClearRollSessionInput{
		EntityID: req.EntityId,
		Context:  req.Context,
	}

	diceOutput, err := h.diceService.ClearRollSession(ctx, diceInput)
	if err != nil {
		return nil, errors.ToGRPCError(err)
	}

	return &apiv1alpha1.ClearRollSessionResponse{
		Message:      "Roll session cleared successfully",
		RollsCleared: diceOutput.RollsDeleted,
	}, nil
}
