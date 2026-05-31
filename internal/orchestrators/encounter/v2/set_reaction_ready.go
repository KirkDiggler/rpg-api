package encounter

import (
	"context"
	"errors"
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// ErrSetReactionReadyRefused wraps a refusal from the toolkit's
// SetReactionReady verb. The verb rejects an empty charID, an empty reactionRef,
// or an entity not in the encounter (the SDK validates the charID against player
// + monster seats); in normal operation the handler pre-validates the envelope,
// so the entity-not-in-encounter case is the one that reaches here through the
// RPC path. The toolkit returns all of these as plain errors rather than a
// sentinel, so the orchestrator joins them with this sentinel and the handler
// maps it to codes.FailedPrecondition per pat-v2-status-code-mapping (mirrors
// ErrDoorVerbRefused on Interact).
var ErrSetReactionReadyRefused = errors.New("set reaction ready refused")

// SetReactionReadyInput carries the entity-typed SetReactionReady request. The
// handler builds it from the proto request after envelope validation, joining
// the proto Ref into the canonical "module:type:id" ReactionRef the toolkit's
// readiness map keys on (proto translation — stays handler-side).
type SetReactionReadyInput struct {
	// EncounterID identifies the encounter.
	EncounterID string

	// PlayerID is the authenticated player toggling readiness.
	PlayerID core.PlayerID

	// EntityID is the entity the player controls. load verifies the encounter
	// maps PlayerID -> EntityID and returns ErrEntityOwnershipMismatch otherwise
	// (a player may only toggle readiness on their own controlled entity). This
	// is also the charID handed to enc.SetReactionReady.
	EntityID core.EntityID

	// ReactionRef is the canonical "module:type:id" ref string for the reaction
	// (e.g. "dnd5e:conditions:shield"). The handler assembles it from the proto
	// Ref; the orchestrator passes it through to the toolkit as the readiness-map
	// key by reference, never inspecting which ref it is.
	ReactionRef string

	// Ready is the readiness value to set: true opts the entity in to the
	// reaction's trigger window, false opts it out.
	Ready bool
}

// SetReactionReadyOutput is the lean result of a readiness toggle. The write
// has no caller-private payload — clients refresh their readiness panel from
// the next encounter snapshot — so the output is intentionally empty, kept as a
// type for the Input/Output convention and future expansion.
type SetReactionReadyOutput struct{}

// SetReactionReady toggles per-entity reaction readiness: load the encounter
// (verifying membership + entity ownership), invoke the toolkit's
// SetReactionReady verb (which owns the readiness map and entity validation),
// and persist.
//
// No rules logic here: the orchestrator never decides which reactions exist or
// what readiness means — it routes the caller's ref + flag to the toolkit's
// readiness-map setter by reference and persists the snapshot. The toolkit owns
// the only rule-ish piece (validating the entity belongs to the encounter).
func (o *Orchestrator) SetReactionReady(
	ctx context.Context,
	in *SetReactionReadyInput,
) (*SetReactionReadyOutput, error) {
	if in == nil {
		return nil, errors.New("encounter orchestrator: SetReactionReadyInput is required")
	}

	enc, err := o.load(ctx, loadInput{
		EncounterID: in.EncounterID,
		PlayerID:    in.PlayerID,
		EntityID:    string(in.EntityID),
		// Readiness toggle doesn't touch combatant state, so no #689 cascade.
	})
	if err != nil {
		return nil, err
	}

	if readyErr := enc.SetReactionReady(in.EntityID, in.ReactionRef, in.Ready); readyErr != nil {
		// The toolkit returns plain fmt.Errorf for its refusals (entity not in
		// encounter, empty charID/ref — the latter pre-validated handler-side).
		// Join with the sentinel so the handler maps to FailedPrecondition
		// without string matching.
		return nil, fmt.Errorf("%w: %w", ErrSetReactionReadyRefused, readyErr)
	}

	if err := o.persist(ctx, enc, in.EncounterID); err != nil {
		return nil, err
	}

	return &SetReactionReadyOutput{}, nil
}
