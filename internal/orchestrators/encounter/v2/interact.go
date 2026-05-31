package encounter

import (
	"context"
	"errors"
	"fmt"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// ErrTargetNotADoor means the Interact target id is not a door in this
// encounter. Handler maps to codes.NotFound. (Today only door interactions are
// wired; future interaction kinds — chests, levers, NPCs, traps — add their own
// lookups and classifications here.)
var ErrTargetNotADoor = errors.New("target entity is not a door, or door does not exist")

// ErrDoorVerbRefused wraps a state-dependent refusal from a toolkit door verb
// (AttemptUnlock / OpenDoor) that is not one of the recognized sentinels —
// e.g. player not in encounter at the verb level, door already open. The
// toolkit returns these as plain fmt.Errorf rather than sentinels, so the
// orchestrator wraps them in this sentinel and the handler maps it to
// codes.FailedPrecondition per pat-v2-status-code-mapping. The recognized
// sentinels (ErrDoorNotLocked, ErrPromptAlreadyPending) pass through unwrapped
// so the handler can map them distinctly.
var ErrDoorVerbRefused = errors.New("door interaction refused")

// InteractInput carries the entity-typed Interact request. The handler builds
// it from the proto request after envelope validation.
type InteractInput struct {
	// EncounterID identifies the encounter.
	EncounterID string

	// PlayerID is the authenticated player performing the interaction.
	PlayerID core.PlayerID

	// TargetEntityID is the entity being interacted with (today: a door).
	TargetEntityID core.EntityID
}

// InteractOutput is the lean result of an Interact. World changes flow as
// broker events (DoorOpened / HexRevealed), not in this output. The only
// caller-private payload is the skill-check prompt issued when the target door
// is locked.
type InteractOutput struct {
	// Prompt carries the toolkit's PromptIssued when the target door was locked
	// and AttemptUnlock issued a per-player skill-check prompt. The handler
	// translates it to the proto InputRequired{skill_check} wire shape.
	Prompt *tkenc.PromptIssued
}

// Interact routes a door interaction: load the encounter, classify the target
// door (locked vs unlocked), dispatch the matching toolkit verb (AttemptUnlock
// for locked, OpenDoor for unlocked), and persist.
//
// Door routing is the orchestrator's job by design, not a toolkit rule: the SDK
// does NOT gate OpenDoor on Locked (see encounter.DoorData docs — "Locked is the
// orchestrator's flag; orchestrators check Locked and route to AttemptUnlock").
// So reading door.Locked here is orchestration, not rules logic.
//
// Door state is read through the encounter's own synced snapshot (enc.ToData()),
// not a parallel *Data fetched alongside load — there is no public door accessor
// on *Encounter today (a toolkit gap worth a getter, but not blocking: ToData()
// is the sanctioned "ask the encounter for its state" surface and is the same
// snapshot persisted on Save).
func (o *Orchestrator) Interact(ctx context.Context, in *InteractInput) (*InteractOutput, error) {
	if in == nil {
		return nil, errors.New("encounter orchestrator: InteractInput is required")
	}

	enc, err := o.load(ctx, loadInput{
		EncounterID: in.EncounterID,
		PlayerID:    in.PlayerID,
		// Door interactions act on a target, not the player's own entity, so no
		// entity-ownership check. Membership is still verified by load.
	})
	if err != nil {
		return nil, err
	}

	// Classify the target via the encounter's synced snapshot. Only door
	// interactions are wired today; a non-door target is ErrTargetNotADoor.
	data := enc.ToData()
	door, ok := data.Doors[in.TargetEntityID]
	if !ok {
		return nil, ErrTargetNotADoor
	}

	// Locked-door branch: issue a per-player skill-check prompt. The prompt is
	// persisted on the encounter snapshot so the caller can resolve it later via
	// SubmitCheck; no broker event is emitted (prompts are caller-private).
	if door.Locked {
		issued, unlockErr := enc.AttemptUnlock(in.PlayerID, in.TargetEntityID)
		if unlockErr != nil {
			return nil, wrapDoorVerbErr("attempt unlock", unlockErr)
		}
		if err := o.persist(ctx, enc, in.EncounterID); err != nil {
			return nil, err
		}
		return &InteractOutput{Prompt: &issued}, nil
	}

	// Unlocked-door branch: dispatch directly to OpenDoor (publishes per-viewer
	// DoorOpened + GeometryRevealed events to the broker).
	if err := enc.OpenDoor(in.PlayerID, in.TargetEntityID); err != nil {
		return nil, wrapDoorVerbErr("open door", err)
	}
	if err := o.persist(ctx, enc, in.EncounterID); err != nil {
		return nil, err
	}
	return &InteractOutput{}, nil
}

// wrapDoorVerbErr classifies a toolkit door-verb error for the handler's status
// mapping. Recognized sentinels (ErrDoorNotLocked, ErrPromptAlreadyPending) are
// preserved (still wrapped with op context) so errors.Is keeps matching them in
// the handler. Everything else — the toolkit's plain fmt.Errorf state refusals —
// is additionally joined with ErrDoorVerbRefused so the handler maps it to
// FailedPrecondition without string matching.
func wrapDoorVerbErr(op string, err error) error {
	if errors.Is(err, tkenc.ErrDoorNotLocked) || errors.Is(err, tkenc.ErrPromptAlreadyPending) {
		return fmt.Errorf("%s: %w", op, err)
	}
	return fmt.Errorf("%s: %w: %w", op, ErrDoorVerbRefused, err)
}

// persist writes the encounter's synced snapshot back to the repo, checking the
// ToData write-back cascade for serialization errors first. A non-nil SyncErr
// means a held entity failed to re-serialize into its DataJSON, so the snapshot
// would carry stale state — surface it rather than persisting silently.
//
// For the door verbs no combatants are hydrated (no DataJSON attached), so the
// cascade is a no-op and SyncErr is nil; the check is correctness hygiene shared
// by every verb that lands on this orchestrator.
func (o *Orchestrator) persist(ctx context.Context, enc *tkenc.Encounter, encID string) error {
	out := enc.ToData()
	if syncErr := enc.SyncErr(); syncErr != nil {
		return fmt.Errorf("sync encounter state %q: %w", encID, syncErr)
	}
	if err := o.encRepo.Save(ctx, out); err != nil {
		return fmt.Errorf("save encounter %q: %w", encID, err)
	}
	return nil
}
