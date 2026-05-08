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

// maxNPCChainDepth caps the NPC dispatch loop to avoid runaway cycles when
// initiative is unexpectedly all-NPCs (shouldn't happen with at least one
// player, but defensive). After hitting the cap, the handler stops the loop
// and returns; the next manual EndTurn from any player picks up.
const maxNPCChainDepth = 16

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

	enc, err := tkenc.LoadFromData(data, h.broker)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load from data %q: %v", req.GetEncounterId(), err)
	}

	newActive, isNPC, err := enc.EndTurn(core.EntityID(req.GetEntityId()))
	if err != nil {
		return nil, endTurnStatusError(err)
	}

	// NPC dispatch loop. After the toolkit's EndTurn advances initiative, if
	// the new active actor is an NPC the handler runs its turn server-side
	// and ends it, then re-checks. The cap protects against the pathological
	// all-NPC initiative case.
	for depth := 0; isNPC && depth < maxNPCChainDepth; depth++ {
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
		var endErr error
		newActive, isNPC, endErr = enc.EndTurn(newActive)
		if endErr != nil {
			return nil, endTurnStatusError(endErr)
		}
	}

	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter %q: %v", req.GetEncounterId(), err)
	}

	return &encounterv2pb.EndTurnResponse{}, nil
}

// endTurnStatusError maps toolkit EndTurn sentinel errors onto gRPC status
// codes. All EndTurn errors are state-dependent → FailedPrecondition,
// except wrapping/internal failures which surface as Internal.
func endTurnStatusError(err error) error {
	switch {
	case errors.Is(err, tkenc.ErrNotTurnBased):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNotYourTurn):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, tkenc.ErrNoCombatants):
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Errorf(codes.Internal, "end turn: %v", err)
}
