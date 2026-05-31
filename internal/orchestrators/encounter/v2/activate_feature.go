package encounter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// ErrActivateFeatureRefused wraps a refusal from the toolkit's ActivateFeature
// verb (e.g. the actor is not a player seat in this encounter, the feature ref
// is unknown, the resource is exhausted, the character is not in combat). The
// toolkit returns these as plain fmt.Errorf rather than typed sentinels, so the
// orchestrator joins them with this sentinel and the handler maps it to
// codes.FailedPrecondition per pat-v2-status-code-mapping (mirrors
// ErrDoorVerbRefused / ErrSetReactionReadyRefused on the other verbs).
var ErrActivateFeatureRefused = errors.New("activate feature refused")

// ActivateFeatureInput carries the entity-typed ActivateFeature request. The
// handler builds it from the proto request after envelope validation and after
// loading + serializing the actor's character data (the rule-ish serialization
// stays handler-side; see the #691 note on ActivateFeature).
type ActivateFeatureInput struct {
	// EncounterID identifies the encounter.
	EncounterID string

	// PlayerID is the authenticated player activating the feature. Membership is
	// verified by load.
	PlayerID core.PlayerID

	// ActorID is the entity the player controls and is activating the feature
	// for. load verifies the encounter maps PlayerID -> ActorID and returns
	// ErrEntityOwnershipMismatch otherwise (a player may only activate features
	// on their own controlled entity). It is also the ActorID handed to the
	// toolkit verb.
	ActorID core.EntityID

	// FeatureRef is the canonical "module:type:id" ref string for the feature
	// (e.g. "dnd5e:features:rage"). The handler assembles it from the proto Ref;
	// the orchestrator passes it through to the toolkit by reference, never
	// inspecting which feature it is.
	FeatureRef string

	// CharDataJSON is the opaque serialized dnd5e character.Data for the actor,
	// supplied by the handler. The orchestrator treats it as an opaque blob — it
	// never marshals/unmarshals or inspects the dnd5e character shape (that
	// rule-ish serialization is the handler/adapter's job). It is handed straight
	// to the toolkit verb, which self-loads the character from it. See the #691
	// note on ActivateFeature for why this self-load path stays, rather than the
	// shared #689 hydration cascade.
	CharDataJSON json.RawMessage
}

// ActivateFeatureOutput is the result of a feature activation. The world-visible
// effects (ConditionApplied, ResourceChanged) flow as broker events to clients;
// the only caller payload is the updated character serialization the toolkit
// verb returns, which the handler persists back to the character store. The
// orchestrator passes it through opaquely.
type ActivateFeatureOutput struct {
	// UpdatedCharData is the opaque serialized dnd5e character.Data after the
	// activation (resource decremented, condition appended), returned by the
	// toolkit verb. The handler persists it to the character store; the
	// orchestrator never inspects it.
	UpdatedCharData json.RawMessage
}

// ActivateFeature applies a character feature (e.g. Rage) as an in-encounter
// action: load the encounter (verifying membership + entity ownership), invoke
// the toolkit's ActivateFeature verb (which owns ALL rule meaning — resource
// cost, condition construction, the rage-tier table — and publishes
// ConditionApplied / ResourceChanged to the broker), and persist the encounter
// snapshot. The verb's UpdatedCharData is returned for the handler to flush to
// the character store.
//
// No rules logic here: the orchestrator routes the caller's feature ref + the
// opaque character blob to the toolkit verb by reference and persists. The
// toolkit owns the only rule-ish pieces (what the feature does, the resource
// cost, the condition it applies).
//
// #691 (toolkit, OPEN): ActivateFeature deliberately does NOT use the shared
// #689 hydration cascade (loadInput.WithCharacterData stays false). The toolkit
// verb self-loads the actor's character from CharDataJSON and subscribes it to
// the encounter bus to capture the condition it applies; attaching the same
// character via the cascade would hydrate a SECOND held instance onto the bus,
// colliding with the verb's self-load ("condition already applied" /
// "modifier ID already exists" — the exact #684 class). Folding this self-load
// onto the held combatant is tracked as toolkit#691; until it lands,
// ActivateFeature keeps its CharDataJSON self-load and skips the cascade. Do NOT
// "fix" this by attaching — that reintroduces the #691 collision.
func (o *Orchestrator) ActivateFeature(
	ctx context.Context,
	in *ActivateFeatureInput,
) (*ActivateFeatureOutput, error) {
	if in == nil {
		return nil, errors.New("encounter orchestrator: ActivateFeatureInput is required")
	}

	enc, err := o.load(ctx, loadInput{
		EncounterID: in.EncounterID,
		PlayerID:    in.PlayerID,
		EntityID:    string(in.ActorID),
		// #691: NO WithCharacterData here — the toolkit ActivateFeature verb
		// self-loads the actor from CharDataJSON; the cascade would collide with
		// that self-load. See the doc comment above.
	})
	if err != nil {
		return nil, err
	}

	out, verbErr := enc.ActivateFeature(ctx, &tkenc.ActivateFeatureInput{
		ActorID:      in.ActorID,
		FeatureRef:   in.FeatureRef,
		CharDataJSON: in.CharDataJSON,
	})
	if verbErr != nil {
		// The toolkit returns plain fmt.Errorf for its refusals (actor not a
		// player seat, unknown feature, resource exhausted, not in combat). Join
		// with the sentinel so the handler maps to FailedPrecondition without
		// string matching.
		return nil, fmt.Errorf("%w: %w", ErrActivateFeatureRefused, verbErr)
	}

	// Defensive nil check: enc.ActivateFeature must return non-nil output on
	// success (project rule: never return (nil, nil)), but guard here so a future
	// toolkit regression produces a clean error rather than a nil dereference
	// when the handler reads out.UpdatedCharData.
	if out == nil {
		return nil, errors.New("encounter orchestrator: toolkit ActivateFeature returned nil output with nil error")
	}

	if err := o.persist(ctx, enc, in.EncounterID); err != nil {
		return nil, err
	}

	return &ActivateFeatureOutput{UpdatedCharData: out.UpdatedCharData}, nil
}
