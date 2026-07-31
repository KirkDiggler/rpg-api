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
// Targeting mirrors the pushed menu's TargetKind contract (rpg-api#605):
// entity-id, self (translated to the actor's own entity id), and untargeted
// NONE (oneof unset) are accepted; position / area oneofs are reserved for
// future waves and rejected with InvalidArgument because the request shape is
// wrong, not the world state. Which refs require a target is the toolkit's
// call, never the handler's.

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
	if req.GetTarget() == nil {
		return nil, status.Error(codes.InvalidArgument, "target is required")
	}

	entityID, err := translateActionTarget(req.GetTarget(), req.GetActorEntityId())
	if err != nil {
		return nil, err
	}

	_, err = h.orch.TakeAction(ctx, &encounterorch.TakeActionInput{
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

// translateActionTarget maps the proto ActionTarget oneof onto the toolkit's
// entity-id target representation (encounter.ActionTarget carries only an
// EntityID). Pure shape translation — the handler never knows which refs need
// a target; the toolkit enforces that (e.g. an untargeted attack fails with
// ErrUnknownTarget → FailedPrecondition).
//
// The advertised TargetKind contract (rpg-api#605):
//   - entity_id arm → that entity (empty string is a malformed request)
//   - self arm (TARGET_KIND_SELF: Dodge/Hide/Disengage) → the actor's own
//     entity id
//   - oneof unset (TARGET_KIND_NONE: Dash — deliberately has no arm) → empty
//     id, the action is untargeted
//   - position / area arms → reserved for future waves (AoEs, spells), never
//     advertised, rejected request-shaped with InvalidArgument
func translateActionTarget(target *encounterv2pb.ActionTarget, actorEntityID string) (string, error) {
	switch kind := target.GetKind().(type) {
	case *encounterv2pb.ActionTarget_EntityId:
		if kind.EntityId == "" {
			return "", status.Error(codes.InvalidArgument, "target.entity_id must not be empty")
		}
		return kind.EntityId, nil
	case *encounterv2pb.ActionTarget_Self:
		return actorEntityID, nil
	case nil:
		// TARGET_KIND_NONE: an untargeted action sends an ActionTarget with
		// the oneof unset.
		return "", nil
	default:
		return "", status.Error(codes.InvalidArgument,
			"target kind is not supported (position/area targeting is reserved for future waves)")
	}
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
//   - ErrOutOfRange → FailedPrecondition (rpg-api#747: toolkit#864's range/reach
//     gate — target out of the actor's reach/range is a state-dependent refusal,
//     the same class as the other combat gates above, not a request defect)
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
		errors.Is(err, tkenc.ErrNoCombatants),
		// ErrActionUnaffordable: the toolkit enforces the action economy
		// server-side (rpg-api#598) — a character with no action/bonus-action
		// left is refused. State-dependent (the turn's economy), so
		// FailedPrecondition, not InvalidArgument.
		errors.Is(err, tkenc.ErrActionUnaffordable),
		// ErrActionDeferred: a ref the build surfaces in the menu but does not
		// resolve yet (e.g. move in Beat 1). State/scope-dependent refusal.
		errors.Is(err, tkenc.ErrActionDeferred),
		// ErrOutOfRange (rpg-api#747): the target is farther than the attack's
		// reach/range. Without this case it falls through to the generic
		// Internal below, which surfaces as an opaque error instead of the
		// actionable "target out of reach" message.
		errors.Is(err, tkenc.ErrOutOfRange):
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Errorf(codes.Internal, "take action: %v", err)
}
