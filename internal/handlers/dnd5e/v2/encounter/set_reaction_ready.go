package encounter

// Wave 2.11d — SetReactionReady RPC handler.
//
// Toggles per-character readiness for a specific reaction. Players opt in
// to spell-cost reactions (Shield) by setting ready=true; conditions only
// publish reaction triggers when the readiness flag is set, so without
// this RPC no Shield prompts ever fire.
//
// Free-cost reactions (OA) are seeded ready=true at AddPlayer time
// (encounter SDK Wave 2.11c); players use this RPC to override the default
// (e.g. "hold OA this turn").

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

// SetReactionReady handles the SetReactionReady RPC: validates the request,
// updates the encounter's reaction-readiness map for the named character,
// and persists the snapshot.
//
// Behavior contract:
//   - missing auth → Unauthenticated
//   - empty encounter_id / character_id / reaction_ref → InvalidArgument
//   - encounter not in repo → NotFound
//   - caller is not a member of the encounter → PermissionDenied
//   - character_id does not match caller's controlled entity → PermissionDenied
//   - SDK rejects the entity (not in encounter) → FailedPrecondition
//   - save failure → Internal
//
// On success returns an empty SetReactionReadyResponse — clients update
// their readiness panel from the next encounter snapshot or readiness-changed
// event (future).
func (h *Handler) SetReactionReady(
	ctx context.Context,
	req *encounterv2pb.SetReactionReadyRequest,
) (*encounterv2pb.SetReactionReadyResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetCharacterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}
	if req.GetReactionRef() == nil ||
		req.GetReactionRef().GetModule() == "" ||
		req.GetReactionRef().GetType() == "" ||
		req.GetReactionRef().GetId() == "" {
		return nil, status.Error(codes.InvalidArgument,
			"reaction_ref must have module, type, and id set")
	}

	data, err := h.encRepo.Get(ctx, req.GetEncounterId())
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal,
			"load encounter %q: %v", req.GetEncounterId(), err)
	}

	pd, ok := data.Players[core.PlayerID(playerID)]
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "player is not in this encounter")
	}
	if string(pd.EntityID) != req.GetCharacterId() {
		return nil, status.Error(codes.PermissionDenied,
			"character_id does not match player's controlled entity")
	}

	// #689: readiness toggle doesn't touch combatant state; no player DataJSON
	// attached, so the cascade skips hydration. ctx threaded for the new
	// LoadFromData signature.
	enc, err := tkenc.LoadFromData(ctx, data, h.broker,
		tkenc.WithCharacterResolver(h.resolver),
		tkenc.WithCombatResolver(h.buildCombatResolver(data)),
		tkenc.WithMovementResolver(h.buildMovementResolver(data)))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load from data %q: %v", req.GetEncounterId(), err)
	}

	refStr := refProtoToString(req.GetReactionRef())
	if err := enc.SetReactionReady(core.EntityID(req.GetCharacterId()), refStr, req.GetReady()); err != nil {
		// The SDK rejects unknown entities — surface as state-dependent.
		return nil, status.Errorf(codes.FailedPrecondition,
			"set reaction ready: %v", err)
	}

	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal,
			"save encounter %q after set-reaction-ready: %v", req.GetEncounterId(), err)
	}

	return &encounterv2pb.SetReactionReadyResponse{}, nil
}

// refProtoToString joins a proto Ref into the canonical "module:type:id"
// string the toolkit uses as the readiness-map key.
func refProtoToString(r *encounterv2pb.Ref) string {
	return r.GetModule() + ":" + r.GetType() + ":" + r.GetId()
}
