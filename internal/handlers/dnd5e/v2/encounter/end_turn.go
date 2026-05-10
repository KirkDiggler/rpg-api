package encounter

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
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
// players and the deadlock is the encounter's own design, not the handler's).
// We still surface a defensive error if the cap is hit while isNPC is true
// (see the post-loop check below) so operators can spot misconfigured
// encounters rather than seeing them silently freeze.
const npcChainSafetyMargin = 4

// EndTurn ends the active actor's turn, advances initiative, and — when the
// new active actor is an NPC — dispatches the toolkit's NPCAct verb on
// behalf of the server. The NPC dispatch loop continues cycling through any
// chain of consecutive NPC turns until the active actor is a player or the
// chain depth cap is reached.
//
// This is the orchestrator-side half of pat-v2-npc-turn-dispatch: the
// toolkit's EndTurn returns (newActiveID, isNPC, err) so the handler knows
// whether to call NPCAct without re-querying state. After NPCAct returns,
// the handler must call EndTurn again to advance the NPC's turn — NPCAct
// itself is single-purpose (resolve the NPC's action) and does not touch
// turn state.
//
// Mode-gating, turn-violation, no-combatants errors are surfaced by the
// toolkit and mapped per pat-v2-status-code-mapping.
func (h *Handler) EndTurn(ctx context.Context, req *encounterv2pb.EndTurnRequest) (*encounterv2pb.EndTurnResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetEntityId() == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}

	data, err := h.encRepo.Get(ctx, req.GetEncounterId())
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal, "load encounter %q: %v", req.GetEncounterId(), err)
	}

	// Caller must be in the encounter and the entity_id must be the caller's
	// controlled entity (you can't end someone else's turn). The toolkit
	// also enforces "is the active actor" via ErrNotYourTurn.
	pd, ok := data.Players[core.PlayerID(playerID)]
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "player is not in this encounter")
	}
	if string(pd.EntityID) != req.GetEntityId() {
		return nil, status.Error(codes.PermissionDenied, "entity_id does not match player's controlled entity")
	}

	enc, err := tkenc.LoadFromData(data, h.broker, tkenc.WithCharacterResolver(h.resolver), tkenc.WithCombatResolver(h.combatResolver))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load from data %q: %v", req.GetEncounterId(), err)
	}

	newActive, isNPC, err := enc.EndTurn(core.EntityID(req.GetEntityId()))
	if err != nil {
		return nil, endTurnStatusError(err)
	}

	// NPC dispatch loop. After the toolkit's EndTurn advances initiative, if
	// the new active actor is an NPC the handler runs its turn server-side
	// and ends it, then re-checks. The cap is initiative-size + margin so an
	// all-NPC roster cycles all the way through rather than freezing the RPC.
	//
	// Wave 2.10: after each NPCAct / EndTurn cycle, if the encounter has
	// transitioned to ModeEnded (e.g., NPC's attack killed the last hostile
	// — not possible today since killEntity only runs in TakeAction's
	// player-attack path, but cheap insurance for future paths like
	// faction-on-faction NPC kills, environmental damage, etc.), break out
	// of the loop. The post-loop save persists the terminal state and the
	// RPC returns success — the EncounterEnded event already fired through
	// the broker, and the next combat verb will get ErrEncounterEnded.
	npcChainCap := len(data.Initiative) + npcChainSafetyMargin
	for depth := 0; isNPC && depth < npcChainCap; depth++ {
		if actErr := enc.NPCAct(ctx, newActive); actErr != nil {
			// NPCAct errors are surfaced as Internal — they indicate either a
			// rehydration / bus / publish failure (system-shaped) or an
			// unexpected state mismatch. Save what state we have first so the
			// player isn't stuck on the NPC's turn forever; the next manual
			// EndTurn picks up.
			if saveErr := h.encRepo.Save(ctx, enc.ToData()); saveErr != nil {
				return nil, status.Errorf(codes.Internal,
					"npc act failed (%v) and save failed (%v)", actErr, saveErr)
			}
			return nil, status.Errorf(codes.Internal, "npc act %q: %v", string(newActive), actErr)
		}
		// Post-NPCAct end-of-encounter check (Wave 2.10 guard).
		if enc.Mode() == core.ModeEnded {
			isNPC = false
			break
		}
		var endErr error
		newActive, isNPC, endErr = enc.EndTurn(newActive)
		if endErr != nil {
			// ErrEncounterEnded here means the NPC's action ended the
			// encounter and the subsequent EndTurn rejected the call.
			// Treat as success — the encounter has terminated, the events
			// already fired, and we just need to persist the terminal
			// state. Fall through to the post-loop save by clearing isNPC.
			if errors.Is(endErr, tkenc.ErrEncounterEnded) {
				isNPC = false
				break
			}
			return nil, endTurnStatusError(endErr)
		}
		// Post-EndTurn end-of-encounter check (Wave 2.10 guard, defensive
		// — EndTurn doesn't mutate liveness today, but stays robust to
		// future paths that fold post-turn cleanup into EndTurn).
		if enc.Mode() == core.ModeEnded {
			isNPC = false
			break
		}
	}

	// Defensive: if the cap was reached while isNPC is still true, the
	// encounter has more consecutive NPCs than the cap allows. Save what we
	// have and surface FailedPrecondition so the caller (and any operator
	// watching logs) sees the misconfiguration rather than a silent freeze.
	// This shouldn't happen with valid encounter setups; the cap-by-roster
	// math above guarantees the loop reaches a player whenever one exists.
	if isNPC {
		if saveErr := h.encRepo.Save(ctx, enc.ToData()); saveErr != nil {
			return nil, status.Errorf(codes.Internal,
				"npc dispatch loop exhausted and save failed: %v", saveErr)
		}
		return nil, status.Errorf(codes.FailedPrecondition,
			"npc dispatch loop exhausted with NPC %q still active; encounter may have no players in initiative",
			string(newActive))
	}

	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter %q: %v", req.GetEncounterId(), err)
	}

	return &encounterv2pb.EndTurnResponse{}, nil
}

// endTurnStatusError maps toolkit EndTurn sentinel errors onto gRPC status
// codes. All EndTurn errors are state-dependent → FailedPrecondition,
// except wrapping/internal failures which surface as Internal.
//
// Wave 2.10: ErrEncounterEnded is the terminal-state sentinel returned
// when EndTurn is called against an encounter whose mode is ModeEnded.
// State-dependent → FailedPrecondition.
func endTurnStatusError(err error) error {
	switch {
	case errors.Is(err, tkenc.ErrEncounterEnded):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNotTurnBased):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNotYourTurn):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNoCombatants):
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Errorf(codes.Internal, "end turn: %v", err)
}
