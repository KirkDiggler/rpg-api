package encounter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	tkevents "github.com/KirkDiggler/rpg-toolkit/encounter/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
)

// TakeAction dispatches a player-initiated action against the encounter's
// turn-based combat verbs. Wave 2.8 only wires the "attack" action ref;
// other refs return Unimplemented.
//
// The handler is a thin shim over the toolkit's Encounter.TakeAction verb:
// load → load-from-data → call verb → save. Mode-gating, turn-violation,
// non-combatant, and unknown-target errors are all surfaced by the toolkit
// as sentinel errors (encounter.ErrNotTurnBased, ErrNotYourTurn,
// ErrUnsupportedAction, ErrNonCombatant, ErrUnknownTarget); the handler
// maps each onto the corresponding gRPC status code.
//
// Outcome events (AttackResolvedEvent + DamageDealtEvent on hit) are
// published per-viewer through the broker by the toolkit verb itself; the
// handler does not need to translate or echo them in the response. Empty
// TakeActionResponse is the design — the events deliver state changes.
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

	data, err := h.encRepo.Get(ctx, req.GetEncounterId())
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal, "load encounter %q: %v", req.GetEncounterId(), err)
	}

	// Caller must be in the encounter and the actor entity must be the
	// caller's controlled entity. Read directly from data BEFORE LoadFromData
	// so we don't pay rehydration cost on the auth-fail path.
	pd, ok := data.Players[core.PlayerID(playerID)]
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "player is not in this encounter")
	}
	if string(pd.EntityID) != req.GetActorEntityId() {
		return nil, status.Error(codes.PermissionDenied, "actor_entity_id does not match player's controlled entity")
	}

	enc, err := tkenc.LoadFromData(data, h.broker, tkenc.WithCharacterResolver(h.resolver), tkenc.WithCombatResolver(h.buildCombatResolver(data)))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load from data %q: %v", req.GetEncounterId(), err)
	}

	actionRef := tkenc.ActionRef{
		Module: req.GetActionRef().GetModule(),
		Type:   req.GetActionRef().GetType(),
		ID:     req.GetActionRef().GetId(),
	}
	target := tkenc.ActionTarget{EntityID: core.EntityID(entityID)}

	// Wave 2.11d: use the phased verb so reaction prompts can surface to
	// player reactors. Resolved=true → no readied reactions matched, behaves
	// as the legacy single-phase path. Reactions populated → persist the
	// in-flight phase-1 state as a pending prompt + push prompt event onto
	// the reactor's per-viewer stream.
	outcome, takeErr := enc.TakeActionPhased(core.PlayerID(playerID), actionRef, target)
	if takeErr != nil {
		return nil, takeActionStatusError(takeErr)
	}

	if outcome != nil && len(outcome.Reactions) > 0 {
		if err := h.persistPendingReactions(enc, outcome); err != nil {
			return nil, status.Errorf(codes.Internal, "persist pending reactions: %v", err)
		}
	}

	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter %q: %v", req.GetEncounterId(), err)
	}

	return &encounterv2pb.TakeActionResponse{}, nil
}

// persistPendingReactions records each player-reaction trigger from the
// phased outcome as a PendingReactionPrompt on the encounter Data and
// emits an InputRequiredDelivered event to each reactor's per-viewer
// stream.
//
// The PhasedAttackContext (opaque to the SDK; *combat.AttackContext under
// the hood) is JSON-marshaled and stored on the prompt so SubmitCheck +
// CompleteTakeAction can rehydrate it on the resume path. This is the
// load-bearing reason combat.AttackContext was refactored to be pure data
// in Wave 2.11d (no eventBus/roller fields).
func (h *Handler) persistPendingReactions(enc *tkenc.Encounter, outcome *tkenc.TakeActionOutcome) error {
	if outcome.AttackContext == nil {
		return errors.New("phased outcome has reactions but no attack context")
	}
	rulebookCtx, ok := outcome.AttackContext.Rulebook.(*combat.AttackContext)
	if !ok {
		return fmt.Errorf("AttackContext.Rulebook is %T, expected *combat.AttackContext", outcome.AttackContext.Rulebook)
	}
	ctxJSON, err := json.Marshal(rulebookCtx)
	if err != nil {
		return fmt.Errorf("marshal attack context: %w", err)
	}

	for _, trig := range outcome.Reactions {
		reactorPlayerID, ok := h.reactorPlayerID(enc, trig.ReactorID)
		if !ok {
			// SDK contract: only player reactors are surfaced (NPC triggers
			// auto-resolve inside TakeActionPhased). Missing playerID means
			// data inconsistency; skip rather than fail the attack.
			continue
		}

		enc.SetPendingReactionPrompt(reactorPlayerID, &tkenc.PendingReactionPrompt{
			ReactorEntityID:   trig.ReactorID,
			ConditionRef:      trig.ConditionRef,
			TriggerKind:       trig.TriggerKind,
			SourceEntity:      trig.SourceEntity,
			AttackContextJSON: ctxJSON,
		})

		if err := enc.PublishInputRequiredDelivered(reactorPlayerID, tkevents.PromptKindReaction); err != nil {
			return fmt.Errorf("publish reaction prompt event: %w", err)
		}
	}
	return nil
}

// reactorPlayerID resolves the controlling player ID for a reactor entity.
// Returns false when the entity is not a player-controlled entity (e.g.
// monster — should never happen on a surfaced reaction since NPC triggers
// auto-resolve inside TakeActionPhased, but checked defensively).
func (h *Handler) reactorPlayerID(enc *tkenc.Encounter, reactorID core.EntityID) (core.PlayerID, bool) {
	for pid, pd := range enc.ToData().Players {
		if pd.EntityID == reactorID {
			return pid, true
		}
	}
	return "", false
}

// takeActionStatusError maps toolkit TakeAction sentinel errors onto gRPC
// status codes per pat-v2-status-code-mapping (FailedPrecondition for
// state-dependent failures, Unimplemented for unsupported actions,
// InvalidArgument for unknown targets that are syntactically valid but
// don't exist in the encounter — though the toolkit overloads
// ErrUnknownTarget to also mean state-dependent missing-monster, so we
// map it to FailedPrecondition).
//
// Wave 2.10: ErrEncounterEnded is the terminal-state sentinel returned
// when verbs are called against an encounter whose mode is ModeEnded.
// State-dependent → FailedPrecondition.
func takeActionStatusError(err error) error {
	switch {
	case errors.Is(err, tkenc.ErrEncounterEnded):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNotTurnBased):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNotYourTurn):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNonCombatant):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrUnsupportedAction):
		return status.Error(codes.Unimplemented, err.Error())
	case errors.Is(err, tkenc.ErrUnknownTarget):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNoCombatants):
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Errorf(codes.Internal, "take action: %v", err)
}
