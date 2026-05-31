package encounter

import (
	"context"
	"errors"
	"fmt"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// ErrNoPendingReactionPrompt means the caller has no pending reaction prompt to
// resolve. Handler maps to codes.FailedPrecondition.
var ErrNoPendingReactionPrompt = errors.New("no pending reaction prompt for player")

// SubmitReactionCheckInput carries the entity-typed SubmitCheck{take_reaction}
// request. The handler builds it from the proto request after envelope
// validation. There is no d20 roll on this branch — a reaction prompt is a
// take/skip choice, not a check.
type SubmitReactionCheckInput struct {
	// EncounterID identifies the encounter.
	EncounterID string

	// PlayerID is the authenticated player resolving their reaction prompt.
	PlayerID core.PlayerID

	// EntityID is the entity the player controls. load verifies the encounter
	// maps PlayerID -> EntityID and returns ErrEntityOwnershipMismatch otherwise.
	EntityID string

	// Take is the caller's take/skip choice. When true, the orchestrator builds
	// the prompt's ReactionModifiers (via the injected BuildReactionModifiers)
	// and runs phase 2 with them; when false, phase 2 runs with no modifiers.
	Take bool
}

// SubmitReactionCheckOutput is the lean result of a reaction resolution. The
// phase-2 attack outcome flows as broker events (AttackResolved / DamageDealt);
// this output only signals that phase 2 ran and echoes the take/skip choice for
// client telemetry.
type SubmitReactionCheckOutput struct {
	// TookReaction echoes the caller's take/skip choice (the handler maps it to
	// the wire Total: 1 = took, 0 = skipped).
	TookReaction bool
}

// shieldRef is the canonical Shield spell condition ref. Used only to apply the
// one-shot Shield readiness reset after a taken Shield reaction — this is
// orchestration of readiness state (a flag the toolkit exposes), NOT a rules
// computation: the AC magnitude lives in the injected BuildReactionModifiers,
// handler-side.
const shieldRef = "dnd5e:spells:shield"

// SubmitReactionCheck resolves the caller's pending reaction prompt: load the
// encounter with the #689 cascade, decode the persisted phase-1 attack context,
// run the toolkit's CompleteTakeAction (phase 2) with the take/skip modifiers,
// clear the prompt, and persist (flushing player character data back to the
// store).
//
// The toolkit owns the phase-2 attack resolution (re-evaluating the hit with
// the reactor's modifiers and publishing AttackResolved / DamageDealt). The
// orchestrator loads, decodes the opaque context via the injected decoder,
// builds modifiers via the injected builder, invokes the verb, and persists —
// the rule-ish pieces (the AttackContext shape, the Shield +5 magnitude) live
// in the injected funcs, handler-side, not here.
//
// INTERNAL single-RPC: this resolves a prompt that is already persisted on the
// snapshot (set by a prior TakeAction phase-1 / NPCAct pause). There is NO
// cross-RPC wire-pause built here — that is the deferred EndTurn concern.
func (o *Orchestrator) SubmitReactionCheck(
	ctx context.Context,
	in *SubmitReactionCheckInput,
) (*SubmitReactionCheckOutput, error) {
	if in == nil {
		return nil, errors.New("encounter orchestrator: SubmitReactionCheckInput is required")
	}
	if o.reactionResume.DecodeAttackContext == nil || o.reactionResume.BuildReactionModifiers == nil {
		return nil, errors.New("encounter orchestrator: ReactionResume funcs are required for SubmitReactionCheck")
	}

	enc, err := o.load(ctx, loadInput{
		EncounterID:       in.EncounterID,
		PlayerID:          in.PlayerID,
		EntityID:          in.EntityID,
		WithCharacterData: true,
	})
	if err != nil {
		return nil, err
	}

	// Read the pending reaction prompt through the encounter's synced snapshot —
	// the same "ask the encounter for its state" surface Interact uses for doors.
	prompt, ok := enc.ToData().PendingReactionPrompts[in.PlayerID]
	if !ok || prompt == nil {
		return nil, ErrNoPendingReactionPrompt
	}

	phasedCtx, err := o.reactionResume.DecodeAttackContext(prompt.AttackContextJSON)
	if err != nil {
		return nil, fmt.Errorf("decode attack context %q: %w", in.EncounterID, err)
	}

	modifiers := o.reactionResume.BuildReactionModifiers(prompt, in.Take)

	if err := enc.CompleteTakeAction(phasedCtx, modifiers); err != nil {
		return nil, fmt.Errorf("complete take action %q: %w", in.EncounterID, err)
	}

	// One-shot Shield: when the player takes a Shield reaction, clear the
	// readiness flag so the caster must re-ready before the next attack. Free
	// reactions (OA) stay ready until explicitly toggled off. This is readiness
	// bookkeeping (a toolkit-exposed flag), not a rules computation. Non-fatal —
	// the reaction already fired.
	if in.Take && prompt.ConditionRef == shieldRef {
		_ = enc.SetReactionReady(prompt.ReactorEntityID, prompt.ConditionRef, false)
	}

	enc.ClearPendingReactionPrompt(in.PlayerID)

	if err := o.persistWithCharacterData(ctx, enc, in.EncounterID); err != nil {
		return nil, err
	}

	return &SubmitReactionCheckOutput{TookReaction: in.Take}, nil
}

// persistWithCharacterData is the combat-capable persist path: ToData +
// SyncErr check (shared by persist) + flush the cascaded player character
// DataJSON back to the authoritative character store BEFORE saving the
// encounter snapshot (so the next RPC re-attaches fresh state and the snapshot
// does not carry a soon-stale char blob). No-op flush when no Persist func is
// wired.
func (o *Orchestrator) persistWithCharacterData(ctx context.Context, enc *tkenc.Encounter, encID string) error {
	out := enc.ToData()
	if syncErr := enc.SyncErr(); syncErr != nil {
		return fmt.Errorf("sync encounter state %q: %w", encID, syncErr)
	}
	if o.characterData.Persist != nil {
		if err := o.characterData.Persist(ctx, out); err != nil {
			return fmt.Errorf("persist character data %q: %w", encID, err)
		}
	}
	if err := o.encRepo.Save(ctx, out); err != nil {
		return fmt.Errorf("save encounter %q: %w", encID, err)
	}
	return nil
}
