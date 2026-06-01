package encounter

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// EndTurn ends the active actor's turn, advances initiative, and — when the new
// active actor is an NPC — dispatches the toolkit's NPCAct verb server-side
// until a player is active, the encounter ends, or the NPC-chain cap is reached.
//
// The handler is thin (#582 step 7, the final verb carve): validate the
// envelope (auth, encounter_id, entity_id), build the entity-typed EndTurnInput,
// delegate the load → EndTurn verb (+ NPC dispatch loop + pause-for-reaction
// serialization) → persist cycle to the v2 orchestrator, and map the
// orchestrator's sentinel errors onto gRPC status codes. The empty
// EndTurnResponse is by proto design — initiative advancement and NPC-turn
// outcomes flow as events on StreamEncounter.
//
// NO rule logic lives here. The toolkit owns the turn-end reset (the #689
// TurnEndTopic publish that resets per-turn conditions) and all NPC combat
// resolution; the one rulebook-touching piece on the NPC pause-for-reaction path
// (marshaling the opaque *combat.AttackContext) lives in the injected
// ReactionResume adapter, so the orchestrator stays rulebook-free.
func (h *Handler) EndTurn(ctx context.Context, req *encounterv2pb.EndTurnRequest) (*encounterv2pb.EndTurnResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetEntityId() == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	_, err := h.orch.EndTurn(ctx, &encounterorch.EndTurnInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    core.PlayerID(playerID),
		EntityID:    core.EntityID(req.GetEntityId()),
	})
	if err != nil {
		return nil, endTurnStatusError(err)
	}

	return &encounterv2pb.EndTurnResponse{}, nil
}

// endTurnStatusError maps the orchestrator's EndTurn errors onto gRPC status
// codes per pat-v2-status-code-mapping. The orchestrator's load sentinels carry
// the auth classification; the toolkit EndTurn gate sentinels (surfaced
// unwrapped) are state-dependent → FailedPrecondition; ErrNPCChainExhausted is
// the no-players-in-initiative misconfiguration → FailedPrecondition; everything
// else (ErrNPCAct, persist / save failures) → Internal.
//
//   - ErrEncounterNotFound → NotFound
//   - ErrPlayerNotInEncounter / ErrEntityOwnershipMismatch → PermissionDenied
//   - tkenc.Err{EncounterEnded,NotTurnBased,NotYourTurn,NoCombatants} → FailedPrecondition
//   - ErrNPCChainExhausted → FailedPrecondition
//   - ErrNPCAct / unclassified failures → Internal
func endTurnStatusError(err error) error {
	switch {
	case errors.Is(err, encounterorch.ErrEncounterNotFound):
		return status.Error(codes.NotFound, "encounter not found")
	case errors.Is(err, encounterorch.ErrPlayerNotInEncounter):
		return status.Error(codes.PermissionDenied, "player is not in this encounter")
	case errors.Is(err, encounterorch.ErrEntityOwnershipMismatch):
		return status.Error(codes.PermissionDenied,
			"entity_id does not match player's controlled entity")
	case errors.Is(err, tkenc.ErrEncounterEnded):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNotTurnBased):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNotYourTurn):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNoCombatants):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, encounterorch.ErrNPCChainExhausted):
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Errorf(codes.Internal, "end turn: %v", err)
}
