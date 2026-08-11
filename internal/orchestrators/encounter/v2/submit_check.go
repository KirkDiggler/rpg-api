package encounter

import (
	"context"
	"errors"
	"fmt"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// SubmitCheckInput carries the entity-typed SubmitCheck request. The handler
// builds it from the proto request after envelope validation (auth, empty-id,
// d20-range). The roll is the caller-supplied d20 value; modifier resolution is
// the toolkit's job via the wired CharacterResolver.
type SubmitCheckInput struct {
	// EncounterID identifies the encounter.
	EncounterID string

	// PlayerID is the authenticated player resolving their pending prompt.
	PlayerID core.PlayerID

	// EntityID is the entity the player controls. load verifies the encounter
	// maps PlayerID -> EntityID and returns ErrEntityOwnershipMismatch otherwise
	// (a caller may not resolve a prompt that is not theirs).
	EntityID string

	// Roll is the caller-supplied d20 value. The handler bounds it to [1,20]
	// before reaching here; the toolkit re-checks via ErrInvalidRoll.
	Roll int
}

// SubmitCheckOutput is the lean result of a skill-check resolution. World
// effects (a successful unlock dispatches OpenDoor inside the toolkit, which
// publishes DoorOpened + GeometryRevealed) flow as broker events, not in this
// output.
type SubmitCheckOutput struct {
	// Success is whether the resolved total met or beat the prompt DC.
	Success bool

	// Total is the resolved total (roll + ability modifier + tool-proficiency
	// bonus) the toolkit computed via the wired CharacterResolver.
	Total int
}

// Skill-check resolution sentinels. The toolkit returns these as typed
// sentinels (or plain fmt.Errorf for the dispatch-failed default); the
// orchestrator surfaces them unwrapped where the handler maps them distinctly,
// and wraps the unclassified dispatch failures behind ErrSubmitCheckDispatch.
var (
	// ErrSubmitCheckDispatch wraps a resolution-succeeded-but-side-effect-failed
	// error from enc.SubmitCheck (e.g. the downstream OpenDoor dispatch failed).
	// The toolkit returns these as plain fmt.Errorf rather than a sentinel; the
	// orchestrator joins them with this sentinel so the handler maps them to
	// codes.Internal without string matching. The recognized toolkit sentinels
	// (ErrNoPendingPrompt / ErrPromptKindMismatch / ErrInvalidRoll /
	// ErrUnsupportedPromptAction / ErrNoCharacterResolver / ErrOutOfRange) pass
	// through unwrapped.
	ErrSubmitCheckDispatch = errors.New("submit check dispatch failed")
)

// SubmitCheck resolves the caller's pending skill-check prompt against a
// supplied d20 roll: load the encounter (verifying membership + entity
// ownership), dispatch the toolkit's SubmitCheck verb (which owns modifier
// resolution and any success side-effect dispatch), and persist.
//
// The toolkit owns ALL rules here: it reads the pending prompt, applies the
// wired CharacterResolver's ability/tool modifiers to the roll, compares to the
// prompt DC, and on success dispatches the prompt's TriggeredAction (today only
// "open" on a locked door -> OpenDoor, which publishes per-viewer events). The
// orchestrator does no skill math and no door routing — it loads, invokes the
// verb, and persists.
func (o *Orchestrator) SubmitCheck(ctx context.Context, in *SubmitCheckInput) (*SubmitCheckOutput, error) {
	if in == nil {
		return nil, errors.New("encounter orchestrator: SubmitCheckInput is required")
	}

	// rpg-api#787: serialize the full load -> SubmitCheck -> persist span per
	// encounter (see keyed_mutex.go).
	unlock := o.encounterLocks.Lock(in.EncounterID)
	defer unlock()

	enc, err := o.load(ctx, loadInput{
		EncounterID: in.EncounterID,
		PlayerID:    in.PlayerID,
		EntityID:    in.EntityID,
	})
	if err != nil {
		return nil, err
	}

	result, submitErr := enc.SubmitCheck(in.PlayerID, in.Roll)
	if submitErr != nil {
		return nil, wrapSubmitCheckErr(submitErr)
	}

	if err := o.persist(ctx, enc, in.EncounterID); err != nil {
		return nil, err
	}

	return &SubmitCheckOutput{
		Success: result.Success,
		Total:   result.Total,
	}, nil
}

// wrapSubmitCheckErr classifies a toolkit SubmitCheck error for the handler's
// status mapping. Recognized sentinels are preserved (errors.Is keeps matching
// them in the handler). The unclassified "resolution proceeded but the side
// effect failed" cases — which the toolkit returns as plain fmt.Errorf — are
// joined with ErrSubmitCheckDispatch so the handler maps them to Internal
// without string matching.
//
// ErrOutOfRange (rpg-api#747, toolkit#864's gate-review blocker 2): dispatching
// a successful skill check's TriggeredAction re-checks reach before opening the
// door — a player who walked away between AttemptUnlock and SubmitCheck fails
// here, wrapping ErrOutOfRange. Recognized like the other toolkit sentinels
// (not folded into the generic ErrSubmitCheckDispatch bucket) so the handler
// can map it to FailedPrecondition instead of Internal — this is a
// state-dependent refusal (the actor moved), not a system-shaped dispatch
// failure.
func wrapSubmitCheckErr(err error) error {
	switch {
	case errors.Is(err, tkenc.ErrNoPendingPrompt),
		errors.Is(err, tkenc.ErrPromptKindMismatch),
		errors.Is(err, tkenc.ErrInvalidRoll),
		errors.Is(err, tkenc.ErrUnsupportedPromptAction),
		errors.Is(err, tkenc.ErrNoCharacterResolver),
		errors.Is(err, tkenc.ErrOutOfRange):
		return err
	}
	return fmt.Errorf("%w: %w", ErrSubmitCheckDispatch, err)
}
