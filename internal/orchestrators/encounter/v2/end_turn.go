package encounter

import (
	"context"
	"errors"
	"fmt"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// npcChainSafetyMargin is added to the initiative roster size to derive the
// per-call NPC dispatch cap. The cap (initiative_len + margin) lets the loop
// cycle past every combatant once and absorb a few extra rounds of pure-NPC
// initiative without deadlocking, while still bounding the worst case.
//
// Why "len(Initiative) + margin" instead of a fixed value: with a fixed cap
// of N, an all-NPC encounter (initiative larger than N — degenerate but
// possible if a future scenario seeds 20 monsters and 0 players) would exit
// the loop with an NPC still active. Since EndTurn requires a player-owned
// entity_id, the RPC would deadlock — players couldn't progress past the
// NPC chain.
//
// With the size-derived cap, the loop is guaranteed to either reach a player
// or exhaust the entire roster (in which case the encounter genuinely has no
// players and the deadlock is the encounter's own design, not the
// orchestrator's). We still surface ErrNPCChainExhausted if the cap is hit
// while isNPC is true (see the post-loop check below) so operators can spot
// misconfigured encounters rather than seeing them silently freeze.
const npcChainSafetyMargin = 4

// ErrNPCChainExhausted means the NPC dispatch loop hit its roster-derived cap
// with an NPC still active — the encounter has more consecutive NPC turns than
// the cap allows, which only happens when the encounter has no players in
// initiative (the cap-by-roster math reaches a player whenever one exists). The
// handler maps this to codes.FailedPrecondition so the misconfiguration shows
// up rather than the RPC silently freezing.
var ErrNPCChainExhausted = errors.New("npc dispatch loop exhausted with an npc still active")

// ErrNPCAct wraps a system-shaped failure from the toolkit's NPCAct verb
// (rehydration / bus / publish failure or an unexpected state mismatch). The
// handler maps it to codes.Internal. A paused-for-reaction NPCAct is NOT an
// error on this sentinel — it is handled in-loop (see EndTurn) and returns a
// successful pause.
var ErrNPCAct = errors.New("npc act failed")

// EndTurnInput carries the entity-typed EndTurn request. The handler builds it
// from the proto request after envelope validation (auth, empty-id). Membership
// + entity ownership are verified by load — a player may only end their own
// controlled entity's turn.
type EndTurnInput struct {
	// EncounterID identifies the encounter.
	EncounterID string

	// PlayerID is the authenticated player ending the turn. load verifies
	// membership and returns ErrPlayerNotInEncounter otherwise.
	PlayerID core.PlayerID

	// EntityID is the entity the player controls and whose turn is ending. load
	// verifies the encounter maps PlayerID -> EntityID and returns
	// ErrEntityOwnershipMismatch otherwise. The toolkit's EndTurn separately
	// enforces "is the active actor" via ErrNotYourTurn.
	EntityID core.EntityID
}

// EndTurnOutput is the lean result of an EndTurn. Initiative advancement and any
// NPC-turn outcomes (the dispatched NPCAct attack, condition resets) flow to
// clients as broker events, not in this output — so the empty output is
// intentional, matching the empty proto EndTurnResponse.
type EndTurnOutput struct{}

// EndTurn ends the active actor's turn, advances initiative, and — when the new
// active actor is an NPC — dispatches the toolkit's NPCAct verb on behalf of the
// server. The NPC dispatch loop continues cycling through any chain of
// consecutive NPC turns until the active actor is a player, the encounter ends,
// or the chain depth cap is reached.
//
// This is the orchestrator-side half of pat-v2-npc-turn-dispatch: the toolkit's
// EndTurn returns (newActiveID, isNPC, err) so the orchestrator knows whether to
// call NPCAct without re-querying state. After NPCAct returns, the orchestrator
// calls EndTurn again to advance the NPC's turn — NPCAct itself is
// single-purpose (resolve the NPC's action) and does not touch turn state.
//
// The turn-end reset is clean post-#689: enc.EndTurn(ctx, ...) emits the dnd5e
// TurnEndTopic on the encounter bus itself, so held conditions (e.g.
// SneakAttack) reset their per-turn state in place with no host-side publish and
// no character re-load. ToData persists the reset state back; the TurnEndTopic
// publish error is propagated by the SDK (it is the ONLY thing that resets
// per-turn state), so a failure surfaces here rather than being swallowed.
//
// Pause-for-reaction (Wave 2.11d): when a dispatched NPC attack pauses for a
// player reaction, the SDK has already persisted the pending reaction prompt +
// published InputRequiredDelivered from inside NPCAct. The orchestrator drops
// all-but-one prompt (single-reactor enforcement), marshals the cached
// PhasedAttackContext into the prompt's AttackContextJSON via the injected
// ReactionResume.MarshalAttackContext (so the orchestrator never touches the
// rulebook *combat.AttackContext shape), persists, and returns success — the
// player must respond (via SubmitReactionCheck) before the NPC's turn and the
// rest of the dispatch loop continue.
//
// INTERNAL: the pause resolves through the already-carved SubmitReactionCheck
// single-RPC; there is NO cross-RPC wire-pause built here (that is the deferred
// EndTurn concern — explicitly out of scope).
//
// Mode-gating, turn-violation, and no-combatants errors are surfaced by the
// toolkit UNWRAPPED so the handler maps each distinctly per
// pat-v2-status-code-mapping.
func (o *Orchestrator) EndTurn(ctx context.Context, in *EndTurnInput) (*EndTurnOutput, error) {
	if in == nil {
		return nil, errors.New("encounter orchestrator: EndTurnInput is required")
	}

	enc, err := o.load(ctx, loadInput{
		EncounterID: in.EncounterID,
		PlayerID:    in.PlayerID,
		EntityID:    string(in.EntityID),
		// #689: combat-capable verb — attach the player character blobs so the
		// hydration cascade holds each combatant; the held conditions reset on the
		// SDK's TurnEndTopic publish below — no separate character re-load.
		WithCharacterData: true,
	})
	if err != nil {
		return nil, err
	}

	// #689: EndTurn(ctx, ...) emits dnd5e TurnEndTopic on the encounter bus
	// itself, so held conditions reset their per-turn state in place. Toolkit
	// gate sentinels (ErrEncounterEnded / ErrNotTurnBased / ErrNotYourTurn /
	// ErrNoCombatants) surface UNWRAPPED so the handler maps each distinctly.
	newActive, isNPC, err := enc.EndTurn(ctx, in.EntityID)
	if err != nil {
		return nil, err
	}

	// NPC dispatch loop. After the toolkit's EndTurn advances initiative, if the
	// new active actor is an NPC the orchestrator runs its turn server-side and
	// ends it, then re-checks. The cap is initiative-size + margin so an all-NPC
	// roster cycles all the way through rather than freezing the RPC. The roster
	// is read from the synced snapshot (enc.ToData().Initiative) — no parallel
	// *Data handle, consistent with load's single-state rule.
	//
	// Wave 2.10: after each NPCAct / EndTurn cycle, if the encounter has
	// transitioned to ModeEnded (e.g., an NPC attack killed the last hostile —
	// not possible today since killEntity only runs in TakeAction's player-attack
	// path, but cheap insurance for future paths like faction-on-faction NPC
	// kills, environmental damage, etc.), break out of the loop. The post-loop
	// persist saves the terminal state and the RPC returns success — the
	// EncounterEnded event already fired through the broker, and the next combat
	// verb will get ErrEncounterEnded.
	npcChainCap := len(enc.ToData().Initiative) + npcChainSafetyMargin
	for depth := 0; isNPC && depth < npcChainCap; depth++ {
		if actErr := enc.NPCAct(ctx, newActive); actErr != nil {
			// Wave 2.11d: NPC turn paused for a player reaction. The encounter has
			// the pending reaction prompt + the InputRequiredDelivered event already
			// published BY THE SDK from inside NPCAct (encounter/npc.go at the
			// player-trigger loop). Drop all-but-one prompt, marshal the cached
			// PhasedAttackContext into the prompt's AttackContextJSON via the
			// injected adapter so SubmitReactionCheck can rebuild phase 2 on resume,
			// then persist and return success — the player must respond before the
			// NPC's turn (and the rest of the dispatch loop) continues.
			//
			// Wave 2.11d closeout (#538 B): the SDK-side publish-then-host-serialize
			// ordering creates a race window where a fast stream subscriber reloads
			// from repo BEFORE the serialize + Save below. The subscriber could see
			// either the old snapshot (no prompt) or a snapshot with a prompt but
			// AttackContextJSON=nil. The proper fix lives in the SDK: a
			// resolver-supplied serializer callback that lets the SDK populate
			// AttackContextJSON itself BEFORE publishing the event. Tracked in
			// rpg-toolkit#657 (implicit in rpg-toolkit#658 for movement-paused
			// reactions). We cannot fix this at the rpg-api layer without changing
			// the SDK contract — the publish happens inside NPCAct, before we return
			// here.
			if tkenc.IsNPCPausedForReaction(actErr) {
				// Single-reactor enforcement (Wave 2.11d closeout #538 C, Option 1):
				// if the SDK persisted multiple prompts for this pause, drop all but
				// the first. Orphan InputRequiredDelivered events already went out
				// from inside NPCAct; subscribers reloading from repo will find no
				// matching prompt and drop silently — acceptable under single-reactor
				// semantics. See TakeAction's persistPendingReactions for the design
				// rationale + rpg-api#540 for the proper aggregate-then-complete fix.
				o.enforceSingleReactor(enc)
				if serErr := o.serializeNPCPendingReactions(enc); serErr != nil {
					return nil, fmt.Errorf("serialize npc pending reactions %q: %w", in.EncounterID, serErr)
				}
				if persistErr := o.persistWithCharacterData(ctx, enc, in.EncounterID); persistErr != nil {
					return nil, persistErr
				}
				return &EndTurnOutput{}, nil
			}
			// NPCAct errors are system-shaped (rehydration / bus / publish failure
			// or unexpected state mismatch). Save what state we have first so the
			// player isn't stuck on the NPC's turn forever; the next manual EndTurn
			// picks up. Surface the wrapped sentinel so the handler maps to Internal.
			if persistErr := o.persistWithCharacterData(ctx, enc, in.EncounterID); persistErr != nil {
				return nil, fmt.Errorf("%w (%v) and persist failed: %w", ErrNPCAct, actErr, persistErr)
			}
			return nil, fmt.Errorf("%w %q: %w", ErrNPCAct, string(newActive), actErr)
		}
		// Post-NPCAct end-of-encounter check (Wave 2.10 guard).
		if enc.Mode() == core.ModeEnded {
			isNPC = false
			break
		}
		var endErr error
		// #689: enc.EndTurn(ctx, ...) emits the dnd5e TurnEndTopic on the encounter
		// bus itself for the NPC whose turn is ending — held conditions reset in
		// place, no host-side publish needed.
		newActive, isNPC, endErr = enc.EndTurn(ctx, newActive)
		if endErr != nil {
			// ErrEncounterEnded here means the NPC's action ended the encounter and
			// the subsequent EndTurn rejected the call. Treat as success — the
			// encounter has terminated, the events already fired, and we just need to
			// persist the terminal state. Fall through to the post-loop persist by
			// clearing isNPC.
			if errors.Is(endErr, tkenc.ErrEncounterEnded) {
				isNPC = false
				break
			}
			return nil, endErr
		}
		// Post-EndTurn end-of-encounter check (Wave 2.10 guard, defensive — EndTurn
		// doesn't mutate liveness today, but stays robust to future paths that fold
		// post-turn cleanup into EndTurn).
		if enc.Mode() == core.ModeEnded {
			isNPC = false
			break
		}
	}

	// Defensive: if the cap was reached while isNPC is still true, the encounter
	// has more consecutive NPCs than the cap allows. Persist what we have and
	// surface ErrNPCChainExhausted so the caller (and any operator watching logs)
	// sees the misconfiguration rather than a silent freeze. This shouldn't happen
	// with valid encounter setups; the cap-by-roster math above guarantees the
	// loop reaches a player whenever one exists.
	if isNPC {
		if persistErr := o.persistWithCharacterData(ctx, enc, in.EncounterID); persistErr != nil {
			return nil, persistErr
		}
		return nil, fmt.Errorf("%w: npc %q still active; encounter may have no players in initiative",
			ErrNPCChainExhausted, string(newActive))
	}

	if err := o.persistWithCharacterData(ctx, enc, in.EncounterID); err != nil {
		return nil, err
	}

	return &EndTurnOutput{}, nil
}

// enforceSingleReactor drops all but the first PendingReactionPrompt on the
// encounter. Used after the SDK's NPCAct path persists multiple reactor prompts
// during a single paused attack (npc.go's playerTriggers loop) —
// CompleteTakeAction is destructive per call, so multiple persisted prompts risk
// double-resolution.
//
// Wave 2.11d closeout (#538 C, Option 1): single-reactor enforcement.
// Wave 2.11e (rpg-api#540) replaces this with aggregate-then-complete once a
// second post-hit reaction ships (Counterspell, etc.). Until then,
// first-eligible-wins.
//
// Map iteration order in Go is unspecified, so "first" here means "any one" —
// acceptable for the Wave 2.11d Shield-only scope where the only multi-reactor
// case is two wizards both having Shield ready against the same target
// (1-target-per-attack scenario; both wouldn't both be the target of the same
// attack).
func (o *Orchestrator) enforceSingleReactor(enc *tkenc.Encounter) {
	prompts := enc.ToData().PendingReactionPrompts
	if len(prompts) <= 1 {
		return
	}
	var kept core.PlayerID
	for pid := range prompts {
		kept = pid
		break
	}
	for pid := range prompts {
		if pid != kept {
			enc.ClearPendingReactionPrompt(pid)
		}
	}
}

// serializeNPCPendingReactions marshals each in-process PhasedAttackContext
// (from NPC attacks that paused for a player reaction) into the corresponding
// PendingReactionPrompt.AttackContextJSON. Without this, the encounter snapshot
// would persist the prompt metadata but not the rulebook-specific AttackContext,
// so SubmitReactionCheck on the resume path could not rebuild phase 2.
//
// The single rulebook-touching piece — interpreting the opaque
// *combat.AttackContext payload — is delegated to the injected
// ReactionResume.MarshalAttackContext (the SAME func TakeAction's phase-1 pause
// uses), so the orchestrator never type-asserts the rulebook shape and stays
// free of the rulebooks/dnd5e/combat import. The marshal seam is only needed on
// this pause path, so it is required lazily here (not up-front in EndTurn) — a
// clear error if we reach the pause path without it beats a nil dereference.
func (o *Orchestrator) serializeNPCPendingReactions(enc *tkenc.Encounter) error {
	for pid, prompt := range enc.ToData().PendingReactionPrompts {
		if len(prompt.AttackContextJSON) > 0 {
			continue // already serialized (player-attack path filled this in)
		}
		phasedCtx := enc.PendingPhasedAttackContext(pid)
		if phasedCtx == nil {
			continue
		}
		// The marshal seam is only consulted here — when a prompt genuinely needs
		// its cached phased context serialized. Required lazily at the point of use
		// (not up-front) so a prompt that is already serialized or has no cached
		// context needs no marshal func wired, matching TakeAction's lazy-required
		// pattern. A clear error if we reach the marshal without it beats a nil
		// dereference.
		if o.reactionResume.MarshalAttackContext == nil {
			return errors.New("encounter orchestrator: ReactionResume.MarshalAttackContext is required to persist an npc reaction prompt")
		}
		ctxJSON, err := o.reactionResume.MarshalAttackContext(phasedCtx)
		if err != nil {
			return fmt.Errorf("marshal pending attack context: %w", err)
		}
		prompt.AttackContextJSON = ctxJSON
	}
	return nil
}
