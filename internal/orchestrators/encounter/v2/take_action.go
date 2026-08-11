package encounter

import (
	"context"
	"errors"
	"fmt"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	tkevents "github.com/KirkDiggler/rpg-toolkit/encounter/events"
)

// TakeActionInput carries the entity-typed TakeAction request. The handler
// builds it from the proto request after envelope validation (auth, empty-id,
// target-shape) and after loading + serializing the actor's character data
// (attached via the #689 cascade by load, not passed here).
type TakeActionInput struct {
	// EncounterID identifies the encounter.
	EncounterID string

	// PlayerID is the authenticated player taking the action. Membership +
	// entity ownership are verified by load.
	PlayerID core.PlayerID

	// ActorID is the entity the player controls and is acting with. load
	// verifies the encounter maps PlayerID -> ActorID and returns
	// ErrEntityOwnershipMismatch otherwise (a player may only act with their own
	// controlled entity). It is also the actor handed to the toolkit verb.
	ActorID core.EntityID

	// ActionRef is the canonical action reference (module / type / id) the
	// toolkit verb routes on (Wave 2.8: only id "attack" is wired; other refs
	// the toolkit refuses with ErrUnsupportedAction). The orchestrator passes it
	// through by reference, never inspecting which action it is.
	ActionRef tkenc.ActionRef

	// TargetEntityID is the entity-id target of the action. The handler
	// translates the proto oneof onto it (rpg-api#605): entity_id arm → that
	// entity; self arm → the actor's own entity id; unset oneof (untargeted
	// NONE, e.g. Dash) → empty. position / area arms are reserved for future
	// waves and rejected handler-side. The orchestrator passes it through to
	// the toolkit verb; whether the action requires a target is the toolkit's
	// call (an untargeted attack fails with ErrUnknownTarget).
	TargetEntityID core.EntityID
}

// TakeActionOutput is the lean result of a TakeAction. Outcome events
// (AttackResolved / DamageDealt on an inline resolution) flow to clients as
// broker events, not in this output. When a player reaction pauses the attack,
// the orchestrator persists the in-flight phase-1 state as a pending prompt and
// publishes InputRequiredDelivered to the reactor's stream; the empty output is
// intentional in both cases.
type TakeActionOutput struct{}

// TakeAction dispatches a player-initiated action against the encounter's
// turn-based combat verbs: load the encounter with the #689 hydration cascade
// (so the combat resolver reads the held attacker/defender — the #684
// double-subscribe cure), invoke the toolkit's two-phase TakeActionPhased verb,
// and persist.
//
// The toolkit owns ALL rules here: it gates mode / turn / combatant / target,
// runs the combat resolver's hit + damage chain (Rage's +2 damage component and
// physical resistance, Sneak Attack, etc.), and publishes the per-viewer
// AttackResolved / DamageDealt events. The orchestrator does no attack math — it
// loads, invokes the verb by reference, and persists.
//
// Phased reaction pause (Wave 2.11d): when phase 1 surfaces a readied player
// reaction (outcome.Reactions non-empty), NO outcome events fire yet. The
// orchestrator persists the in-flight phase-1 attack context as a pending
// reaction prompt on the snapshot, saves, THEN publishes InputRequiredDelivered
// to the reactor's stream. The save-before-publish ordering (rpg-api#538 A) is
// load-bearing: the stream translator reloads PendingReactionPrompts from the
// repo when it receives the event, so a fast subscriber must never see the
// pre-prompt snapshot. The reactor later resolves via SubmitReactionCheck.
//
// The single rulebook-touching piece — serializing the resolver's native
// attack-context payload into the opaque AttackContextJSON — is injected as
// ReactionResume.MarshalAttackContext (handler-side), so the orchestrator never
// type-asserts the rulebook shape.
func (o *Orchestrator) TakeAction(ctx context.Context, in *TakeActionInput) (*TakeActionOutput, error) {
	if in == nil {
		return nil, errors.New("encounter orchestrator: TakeActionInput is required")
	}

	// rpg-api#787: serialize the full load -> verb -> persist span per
	// encounter (see keyed_mutex.go). This is a single self-contained call —
	// the phase-1 pause below persists the pending prompt and returns; it
	// does not block waiting for the reaction, so this lock is held only for
	// this call's own duration, never across the later SubmitReactionCheck
	// resume RPC.
	unlock := o.encounterLocks.Lock(in.EncounterID)
	defer unlock()

	enc, err := o.load(ctx, loadInput{
		EncounterID: in.EncounterID,
		PlayerID:    in.PlayerID,
		EntityID:    string(in.ActorID),
		// #689: combat-capable verb — attach the player character blobs so the
		// resolver reads the held attacker/defender rather than re-loading them
		// mid-attack (the #684 double-subscribe cure).
		WithCharacterData: true,
	})
	if err != nil {
		return nil, err
	}

	outcome, takeErr := enc.TakeActionPhased(in.PlayerID, in.ActionRef, tkenc.ActionTarget{EntityID: in.TargetEntityID})
	if takeErr != nil {
		// The toolkit returns typed sentinels for its gates (ErrNotTurnBased,
		// ErrNotYourTurn, ErrUnsupportedAction, ErrNonCombatant, ErrUnknownTarget,
		// ErrEncounterEnded, ErrNoCombatants). Surface them unwrapped so the
		// handler maps each distinctly via errors.Is.
		return nil, takeErr
	}
	// Defensive nil check: TakeActionPhased must return non-nil outcome on
	// success (project rule: never return (nil, nil)). Guard so a future toolkit
	// regression produces a clean error rather than a nil dereference below.
	if outcome == nil {
		return nil, errors.New("encounter orchestrator: toolkit TakeActionPhased returned nil outcome with nil error")
	}

	// Phased reaction pause: persist the in-flight phase-1 state as a pending
	// prompt BEFORE saving so the snapshot carries it; collect reactor PlayerIDs
	// to notify AFTER the save commits (rpg-api#538 A save-before-publish).
	var promptsToPublish []core.PlayerID
	if len(outcome.Reactions) > 0 {
		pids, persistErr := o.persistPendingReactions(enc, outcome)
		if persistErr != nil {
			return nil, fmt.Errorf("persist pending reactions %q: %w", in.EncounterID, persistErr)
		}
		promptsToPublish = pids
	}

	if err := o.persistWithCharacterData(ctx, enc, in.EncounterID); err != nil {
		return nil, err
	}

	// Save committed — now safe to publish prompt events. Stream subscribers
	// that reload from the repo will see the persisted prompts.
	for _, pid := range promptsToPublish {
		if err := enc.PublishInputRequiredDelivered(pid, tkevents.PromptKindReaction); err != nil {
			return nil, fmt.Errorf("publish reaction prompt event %q: %w", in.EncounterID, err)
		}
	}

	return &TakeActionOutput{}, nil
}

// persistPendingReactions records the first eligible player-reaction trigger
// from the phased outcome as a PendingReactionPrompt on the encounter snapshot.
// Returns the list of reactor PlayerIDs whose prompts were persisted (0 or 1
// entries — see single-reactor enforcement note) for the caller to publish
// InputRequiredDelivered AFTER the snapshot save commits.
//
// The phase-1 attack context (opaque to the SDK; the resolver's native
// *combat.AttackContext under the hood) is serialized via the injected
// ReactionResume.MarshalAttackContext, keeping the rulebook type assertion
// handler-side. The orchestrator routes triggers to players by reference (entity
// -> controlling player) — orchestration, not rules.
//
// Single-reactor enforcement (rpg-api#538 C, Option 1): CompleteTakeAction is
// destructive (publishes AttackResolved + applies damage once per call), so
// persisting multiple reactor prompts on the same paused attack risks
// double-resolution. For the shipped scope (Shield is the only post-hit
// modifier; OA is fan-out as separate attacks, not modifier stacking) picking
// the first resolvable player trigger is correct. Multi-reactor aggregation is
// tracked as rpg-api#540.
func (o *Orchestrator) persistPendingReactions(
	enc *tkenc.Encounter,
	outcome *tkenc.TakeActionOutcome,
) ([]core.PlayerID, error) {
	if outcome.AttackContext == nil {
		return nil, errors.New("phased outcome has reactions but no attack context")
	}
	// The marshal seam is only needed on this pause path, so it is required
	// lazily here (not up-front in TakeAction) — non-phased callers that can
	// never produce reactions need not wire it. A clear error if we reach the
	// pause path without it beats a nil dereference.
	if o.reactionResume.MarshalAttackContext == nil {
		return nil, errors.New("encounter orchestrator: ReactionResume.MarshalAttackContext is required to persist a reaction prompt")
	}
	ctxJSON, err := o.reactionResume.MarshalAttackContext(outcome.AttackContext)
	if err != nil {
		return nil, fmt.Errorf("marshal attack context: %w", err)
	}

	// Single-reactor enforcement: pick the first trigger whose reactor resolves
	// to a player. Subsequent triggers are dropped (see rpg-api#540).
	for i := range outcome.Reactions {
		trig := outcome.Reactions[i]
		reactorPlayerID, ok := o.reactorPlayerID(enc, trig.ReactorID)
		if !ok {
			// SDK contract: only player reactors are surfaced (NPC triggers
			// auto-resolve inside TakeActionPhased). Missing playerID means data
			// inconsistency; skip and try the next trigger.
			continue
		}

		enc.SetPendingReactionPrompt(reactorPlayerID, &tkenc.PendingReactionPrompt{
			ReactorEntityID:   trig.ReactorID,
			ConditionRef:      trig.ConditionRef,
			TriggerKind:       trig.TriggerKind,
			SourceEntity:      trig.SourceEntity,
			AttackContextJSON: ctxJSON,
		})
		return []core.PlayerID{reactorPlayerID}, nil
	}

	// No eligible player-reactor trigger found.
	return []core.PlayerID{}, nil
}

// reactorPlayerID resolves the controlling player for a reactor entity through
// the encounter's synced snapshot (the same "ask the encounter for its state"
// surface Interact uses for doors). Returns false when the entity is not a
// player-controlled seat (e.g. a monster — should never happen on a surfaced
// reaction since NPC triggers auto-resolve inside TakeActionPhased, but checked
// defensively).
func (o *Orchestrator) reactorPlayerID(enc *tkenc.Encounter, reactorID core.EntityID) (core.PlayerID, bool) {
	for pid, pd := range enc.ToData().Players {
		if pd.EntityID == reactorID {
			return pid, true
		}
	}
	return "", false
}
