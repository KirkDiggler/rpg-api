// Package v1alpha1 handles the generic API grpc service interface
package v1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
)

// DiceHandlerConfig holds dependencies for the dice handler
type DiceHandlerConfig struct {
	DiceService dice.Service
}

// Validate ensures all required dependencies are present
func (c *DiceHandlerConfig) Validate() error {
	if c.DiceService == nil {
		return apierr.InvalidArgument("dice service is required")
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

// trustedWorld reads the world installed by the auth/role boundary. A missing
// world fails closed rather than letting dice state land in an unnamed world.
func trustedWorld(ctx context.Context) (string, error) {
	world, ok := worldcontext.Get(ctx)
	if !ok || world.WorldID == "" {
		return "", apierr.FailedPrecondition("trusted world context is required")
	}
	return world.WorldID, nil
}

// creationEntityID binds the ability_scores roll session to the authenticated
// player. The measured creation consumer sends entity_id = playerId, and a
// client-supplied entity id is never authority: a request naming a different
// entity for creation is refused rather than written under that name.
func creationEntityID(ctx context.Context, requested string) (string, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return "", apierr.Unauthenticated("player not authenticated")
	}
	if requested != playerID {
		return "", apierr.PermissionDenied("ability_scores rolls belong to the authenticated player")
	}
	return playerID, nil
}

// RollDice rolls dice using the specified notation and stores the result in a session
func (h *DiceHandler) RollDice(
	ctx context.Context,
	req *apiv1alpha1.RollDiceRequest,
) (*apiv1alpha1.RollDiceResponse, error) {
	if req.EntityId == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("entity_id is required"))
	}
	if req.Context == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("context is required"))
	}
	if req.Notation == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("notation is required"))
	}

	worldID, err := trustedWorld(ctx)
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}

	entityID := req.EntityId
	if req.Context == dice.ContextAbilityScores {
		entityID, err = creationEntityID(ctx, req.EntityId)
		if err != nil {
			return nil, apierr.ToGRPCError(err)
		}
	}

	// Use the dice service to roll dice
	diceInput := &dice.RollDiceInput{
		WorldID:     worldID,
		EntityID:    entityID,
		Context:     req.Context,
		Notation:    req.Notation,
		Description: req.ModifierDescription,
	}

	diceOutput, err := h.diceService.RollDice(ctx, diceInput)
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}

	// Convert all session rolls to proto format
	rolls := make([]*apiv1alpha1.DiceRoll, 0, len(diceOutput.Session.Rolls))
	for _, sessionRoll := range diceOutput.Session.Rolls {
		rolls = append(rolls, &apiv1alpha1.DiceRoll{
			RollId:      sessionRoll.RollID,
			Notation:    sessionRoll.Notation,
			Dice:        intToInt32s(sessionRoll.Dice),
			Total:       int32(sessionRoll.Total),
			Dropped:     intToInt32s(sessionRoll.Dropped),
			Description: sessionRoll.Description,
			DiceTotal:   int32(sessionRoll.DiceTotal),
			Modifier:    int32(sessionRoll.Modifier),
		})
	}

	return &apiv1alpha1.RollDiceResponse{
		Rolls:     rolls,
		ExpiresAt: diceOutput.Session.ExpiresAt.Unix(),
	}, nil
}

func intToInt32s(ints []int) []int32 {
	int32s := make([]int32, len(ints))
	for i, v := range ints {
		int32s[i] = int32(v)
	}
	return int32s
}

// GetRollSession retrieves an existing dice roll session
func (h *DiceHandler) GetRollSession(
	ctx context.Context,
	req *apiv1alpha1.GetRollSessionRequest,
) (*apiv1alpha1.GetRollSessionResponse, error) {
	if req.EntityId == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("entity_id is required"))
	}
	if req.Context == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("context is required"))
	}

	worldID, err := trustedWorld(ctx)
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}

	entityID := req.EntityId
	if req.Context == dice.ContextAbilityScores {
		entityID, err = creationEntityID(ctx, req.EntityId)
		if err != nil {
			return nil, apierr.ToGRPCError(err)
		}
	}

	// Use the dice service to get the session
	// AutoCreate for ability_scores so UI doesn't need to click roll button
	diceInput := &dice.GetRollSessionInput{
		WorldID:    worldID,
		EntityID:   entityID,
		Context:    req.Context,
		AutoCreate: req.Context == dice.ContextAbilityScores,
	}

	diceOutput, err := h.diceService.GetRollSession(ctx, diceInput)
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}

	// Convert session rolls to proto format
	rolls := make([]*apiv1alpha1.DiceRoll, 0, len(diceOutput.Session.Rolls))
	for _, sessionRoll := range diceOutput.Session.Rolls {
		rolls = append(rolls, &apiv1alpha1.DiceRoll{
			RollId:      sessionRoll.RollID,
			Notation:    sessionRoll.Notation,
			Dice:        intToInt32s(sessionRoll.Dice),
			Total:       int32(sessionRoll.Total),
			Dropped:     intToInt32s(sessionRoll.Dropped),
			Description: sessionRoll.Description,
			DiceTotal:   int32(sessionRoll.DiceTotal),
			Modifier:    int32(sessionRoll.Modifier),
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
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("entity_id is required"))
	}
	if req.Context == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("context is required"))
	}

	worldID, err := trustedWorld(ctx)
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}

	entityID := req.EntityId
	if req.Context == dice.ContextAbilityScores {
		entityID, err = creationEntityID(ctx, req.EntityId)
		if err != nil {
			return nil, apierr.ToGRPCError(err)
		}
	}

	// Use the dice service to clear the session
	diceInput := &dice.ClearRollSessionInput{
		WorldID:  worldID,
		EntityID: entityID,
		Context:  req.Context,
	}

	diceOutput, err := h.diceService.ClearRollSession(ctx, diceInput)
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}

	return &apiv1alpha1.ClearRollSessionResponse{
		Message:      "Roll session cleared successfully",
		RollsCleared: int32(diceOutput.RollsDeleted),
	}, nil
}
