package encounter

import (
	"context"
	"errors"
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// ErrEmptyPath means the MoveEntity request carried no proposed path. The
// handler maps it to codes.InvalidArgument (the request shape is wrong, not the
// world state). It is checked AFTER load so the load preconditions (NotFound /
// PermissionDenied) keep their precedence over this argument-shaped error —
// matching the inline handler the carve replaces.
var ErrEmptyPath = errors.New("proposed path is required")

// ErrMoveRefused wraps a state-dependent refusal from the toolkit's Move verb
// (turn order, blocked path, out-of-range, entity not in encounter, etc.). The
// toolkit returns these as plain fmt.Errorf rather than typed sentinels, so the
// orchestrator joins them with this sentinel and the handler maps it to
// codes.FailedPrecondition per pat-v2-status-code-mapping (mirrors
// ErrDoorVerbRefused / ErrActivateFeatureRefused / ErrSetReactionReadyRefused on
// the other verbs). Empty-path is filtered before Move via ErrEmptyPath, so the
// only errors reaching here are genuinely state-dependent.
var ErrMoveRefused = errors.New("move refused")

// MoveEntityInput carries the entity-typed MoveEntity request. The handler
// builds it from the proto request after envelope validation (auth, empty
// encounter id) and converts the proto path positions into core.Hex (the
// proto↔entity conversion stays handler-side; the orchestrator speaks core.Hex).
type MoveEntityInput struct {
	// EncounterID identifies the encounter.
	EncounterID string

	// PlayerID is the authenticated player moving the entity. Membership +
	// entity ownership are verified by load.
	PlayerID core.PlayerID

	// EntityID is the entity the player controls and is moving. load verifies the
	// encounter maps PlayerID -> EntityID and returns ErrEntityOwnershipMismatch
	// otherwise (a player may only move their own controlled entity).
	EntityID core.EntityID

	// Path is the proposed movement path as toolkit hexes. The orchestrator
	// passes it through to the toolkit Move verb, which owns all movement rules
	// (per-step MovementChain, opportunity-attack triggers via the injected
	// movement resolver, path truncation when a step is prevented). Empty path is
	// classified as ErrEmptyPath after load.
	Path []core.Hex
}

// MoveEntityOutput is the lean result of a MoveEntity. World changes (the mover
// landing, opportunity attacks resolving against the mover) flow to clients as
// broker events published per-viewer by the toolkit verb, not in this output —
// so the empty output is intentional, matching the empty proto MoveEntityResponse.
type MoveEntityOutput struct{}

// MoveEntity moves a player's controlled entity along a proposed path: load the
// encounter with the #689 hydration cascade (so the movement resolver reads the
// held mover rather than re-loading it mid-move — the #684 double-subscribe
// cure), invoke the toolkit's Move verb, and persist.
//
// The toolkit owns ALL movement rules: per-step MovementChain dispatch, the
// opportunity-attack trigger/resolve path (a fleeing combatant leaving reach),
// and path truncation when a step is prevented (Disengage). The orchestrator
// does no movement math — it loads, invokes Move by reference, and persists.
//
// The opportunity-attack resolution rides the injected movement resolver
// (BuildMovementResolver), which is the tkenc↔rulebook translation seam kept
// handler-side (the resolver legitimately imports the dnd5e rulebook; the
// orchestrator stays rulebook-free). The resolver's lookup-only threatener load
// (loadCharacterForLookup) runs on a throwaway bus — NOT the encounter bus — so
// it is not a #684 source; it is carried faithfully as-is.
func (o *Orchestrator) MoveEntity(ctx context.Context, in *MoveEntityInput) (*MoveEntityOutput, error) {
	if in == nil {
		return nil, errors.New("encounter orchestrator: MoveEntityInput is required")
	}

	enc, err := o.load(ctx, loadInput{
		EncounterID: in.EncounterID,
		PlayerID:    in.PlayerID,
		EntityID:    string(in.EntityID),
		// #689: combat-capable verb — attach the player character blobs so the
		// movement resolver reads the held mover (and the held OA-condition
		// instances on the encounter bus fire) rather than re-loading them
		// mid-move (the #684 double-subscribe cure).
		WithCharacterData: true,
	})
	if err != nil {
		return nil, err
	}

	// Empty-path is the one genuinely argument-shaped error. Checked AFTER load so
	// the auth sentinels keep precedence (the inline handler validated emptiness
	// after the Get for the same reason). The toolkit Move also rejects it, but
	// classifying it here lets the handler return InvalidArgument distinctly from
	// the FailedPrecondition state refusals.
	if len(in.Path) == 0 {
		return nil, ErrEmptyPath
	}

	if moveErr := enc.Move(in.PlayerID, in.Path); moveErr != nil {
		// Toolkit Move errors are state-dependent (turn order, blocked path,
		// out-of-range, entity not in encounter, etc.) — join with the sentinel so
		// the handler maps to FailedPrecondition without string matching.
		return nil, fmt.Errorf("%w: %w", ErrMoveRefused, moveErr)
	}

	// rpg-api#659: enc.Move can trigger the toolkit's own FREE_ROAM->TURN_BASED
	// combat entry (checkCombatEntry, called inline at the end of Move) when the
	// mover's updated view newly sights a monster. SetMode rolls initiative and
	// announces the first turn (TurnStartedEvent) but never dispatches it — only
	// the orchestrator drives NPC turns (driveNPCChain, the same cycle EndTurn
	// uses below its own toolkit verb call). Without this, a monster winning
	// initiative on a sighted Move stalls forever: EndTurn requires the CURRENT
	// active actor's owning player to call it, and there is no such player.
	// Before this fix, the only thing that ever drove that first NPC turn was
	// the #636 connect-time self-heal (DriveStalledNPCTurn, called from
	// StreamEncounter/GetEncounter) — i.e. the encounter only advanced once some
	// client (re)connected. Mirroring EndTurn's call exactly makes the kick
	// deterministic on the mode transition itself, matching driveNPCChain's own
	// doc comment ("a future in-process combat-entry trigger can (and should)
	// call this... right after its own SetMode").
	//
	// active/isNPC are read AFTER Move so they reflect whatever Move actually
	// did: a non-combat-entry move (still FREE_ROAM, or TURN_BASED with a player
	// active) yields ActiveActor()=="" or a player id, so IsNPC is false and
	// driveNPCChain zero-iterates — behavior-preserving for every case except
	// the one this fixes. This also covers the encounter ending mid-Move (a
	// synchronous instant-death via an opportunity attack): ActiveActor()
	// returns "" whenever Mode != ModeTurnBased, which includes ModeEnded, so
	// driveNPCChain still zero-iterates and simply persists.
	//
	// driveNPCChain persists on every exit path (including zero-iteration), so
	// it replaces the direct persistWithCharacterData call below rather than
	// running alongside it — the same structure EndTurn uses.
	active := enc.ActiveActor()
	if err := o.driveNPCChain(ctx, enc, in.EncounterID, active, enc.IsNPC(active)); err != nil {
		return nil, err
	}

	return &MoveEntityOutput{}, nil
}
