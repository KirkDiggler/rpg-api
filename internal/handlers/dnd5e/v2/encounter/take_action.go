package encounter

// take_action.go — TakeAction handler.
//
// The handler is a thin envelope over the v2 orchestrator's TakeAction method
// (#582 step 5, retiring the inline load → verb → persist body). It owns exactly
// two things:
//  1. Validate the request envelope (auth, encounter_id, actor_entity_id,
//     action_ref, target shape) and convert proto → orchestrator Input.
//  2. Delegate the load → TakeActionPhased verb → persist (+ reaction-prompt
//     persist/publish) cycle to the orchestrator, mapping its toolkit sentinel
//     errors onto gRPC status codes.
//
// NO rule logic lives here. The toolkit's Encounter.TakeActionPhased owns all
// rule meaning (mode/turn/combatant gating, the attack hit + damage chain — Rage
// +2 + physical resistance, Sneak Attack, the reaction-trigger pause). rpg-api
// passes the action ref + target through by reference and persists the result.
//
// Wave 2.8 only wires the "attack" action ref; other refs the toolkit refuses
// with ErrUnsupportedAction (→ Unimplemented). Only entity-id targeting is wired
// (position / area / self oneofs are reserved for future waves and rejected here
// with InvalidArgument because the request shape is wrong, not the world state).

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

// TakeAction dispatches a player-initiated action against the encounter's
// turn-based combat verbs. The handler validates the envelope, converts the
// proto request to the orchestrator Input, delegates the load → TakeActionPhased
// → persist cycle to the v2 orchestrator, and maps the toolkit's sentinel errors
// onto gRPC status codes.
//
// Outcome events (AttackResolved / DamageDealt on hit) are published per-viewer
// through the broker by the toolkit verb itself; when a player reaction pauses
// the attack, the orchestrator persists the pending prompt and publishes
// InputRequiredDelivered to the reactor's stream. The empty TakeActionResponse
// is the design — the events deliver state changes.
func (h *Handler) TakeAction(ctx context.Context, req *encounterv2pb.TakeActionRequest) (*encounterv2pb.TakeActionResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetActorEntityId() == "" {
		return nil, status.Error(codes.InvalidArgument, "actor_entity_id is required")
	}
	if req.GetActionRef() == nil {
		return nil, status.Error(codes.InvalidArgument, "action_ref is required")
	}
	if req.GetTarget() == nil || req.GetTarget().GetKind() == nil {
		return nil, status.Error(codes.InvalidArgument, "target is required")
	}

	// Wave 2.8 only supports entity-id targeting. Other oneof variants are
	// reserved for future waves (position for AoEs, area for spells, self
	// for buffs). InvalidArgument because the request shape is wrong, not
	// the world state.
	entityID := req.GetTarget().GetEntityId()
	if entityID == "" {
		return nil, status.Error(codes.InvalidArgument, "target.entity_id is required (only entity-id targeting is supported in this wave)")
	}

	_, err := h.orch.TakeAction(ctx, &encounterorch.TakeActionInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    core.PlayerID(playerID),
		ActorID:     core.EntityID(req.GetActorEntityId()),
		ActionRef: tkenc.ActionRef{
			Module: req.GetActionRef().GetModule(),
			Type:   req.GetActionRef().GetType(),
			ID:     req.GetActionRef().GetId(),
		},
		TargetEntityID: core.EntityID(entityID),
	})
	if err != nil {
		return nil, takeActionStatusError(err)
	}

	return &encounterv2pb.TakeActionResponse{}, nil
}

// takeActionStatusError maps the orchestrator's TakeAction errors onto gRPC
// status codes per pat-v2-status-code-mapping. The orchestrator's load sentinels
// carry the auth classification; the toolkit verb sentinels (surfaced unwrapped)
// carry the mode / turn / combatant / target / terminal-state classifications.
//
//   - ErrEncounterNotFound → NotFound
//   - ErrPlayerNotInEncounter / ErrEntityOwnershipMismatch → PermissionDenied
//   - ErrUnsupportedAction → Unimplemented (only "attack" is wired this wave)
//   - ErrEncounterEnded / ErrNotTurnBased / ErrNotYourTurn / ErrNonCombatant /
//     ErrUnknownTarget / ErrNoCombatants → FailedPrecondition (state-dependent;
//     the toolkit overloads ErrUnknownTarget to also mean missing-monster, so it
//     maps to FailedPrecondition rather than InvalidArgument)
//   - load / save / unclassified failures → Internal
func takeActionStatusError(err error) error {
	switch {
	case errors.Is(err, encounterorch.ErrEncounterNotFound):
		return status.Error(codes.NotFound, "encounter not found")
	case errors.Is(err, encounterorch.ErrPlayerNotInEncounter):
		return status.Error(codes.PermissionDenied, "player is not in this encounter")
	case errors.Is(err, encounterorch.ErrEntityOwnershipMismatch):
		return status.Error(codes.PermissionDenied,
			"actor_entity_id does not match player's controlled entity")
	case errors.Is(err, tkenc.ErrUnsupportedAction):
		return status.Error(codes.Unimplemented, err.Error())
	case errors.Is(err, tkenc.ErrEncounterEnded),
		errors.Is(err, tkenc.ErrNotTurnBased),
		errors.Is(err, tkenc.ErrNotYourTurn),
		errors.Is(err, tkenc.ErrNonCombatant),
		errors.Is(err, tkenc.ErrUnknownTarget),
		errors.Is(err, tkenc.ErrNoCombatants):
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Errorf(codes.Internal, "take action: %v", err)
}
