package encounter

// activate_feature.go — ActivateFeature handler.
//
// The handler is a thin envelope over the v2 orchestrator's ActivateFeature
// method (#582 step 4, retiring the Runner). It owns exactly four things:
//  1. Validate the request envelope (encounter_id, character_id, feature_ref).
//  2. Load the character from the repo + prepare CharDataJSON for the toolkit
//     (the rule-ish serialization + in-combat ActionEconomy injection stays
//     handler-side, off the rulebook-free orchestrator).
//  3. Delegate the load → ActivateFeature verb → persist cycle to the
//     orchestrator, mapping its sentinel errors onto gRPC status codes.
//  4. Persist the UpdatedCharData the orchestrator returns from the verb.
//
// NO rule logic lives here. The toolkit's Encounter.ActivateFeature owns all
// rule meaning (resource cost, condition construction, tier table).
// rpg-api passes the ref through and stores the result.
//
// #691 (toolkit, OPEN): the orchestrator deliberately runs ActivateFeature's
// load WITHOUT the #689 hydration cascade, because the toolkit verb self-loads
// the actor from CharDataJSON; attaching the same character via the cascade
// would collide with that self-load. See the orchestrator's ActivateFeature
// doc comment. This handler keeps building CharDataJSON for that self-load.

import (
	"context"
	"encoding/json"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// ActivateFeature applies a character feature (e.g. Rage) as an in-encounter
// action. All rule logic — resource cost, condition construction, tier table —
// lives in the toolkit's Encounter.ActivateFeature verb, dispatched by the v2
// orchestrator. This handler validates the envelope, prepares the actor's
// CharDataJSON, delegates the load → verb → persist cycle to the orchestrator,
// and persists the updated character data the verb returns.
//
// State changes (ConditionApplied, ResourceChanged) flow to clients via the
// EncounterEvent stream through the broker; the empty response is intentional.
func (h *Handler) ActivateFeature(ctx context.Context, req *encounterv2pb.ActivateFeatureRequest) (*encounterv2pb.ActivateFeatureResponse, error) {
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
	fr := req.GetFeatureRef()
	if fr == nil || fr.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "feature_ref is required")
	}
	if fr.GetModule() == "" {
		return nil, status.Error(codes.InvalidArgument, "feature_ref.module is required")
	}
	if fr.GetType() == "" {
		return nil, status.Error(codes.InvalidArgument, "feature_ref.type is required")
	}

	if h.combatResolverConfig.CharacterRepo == nil {
		return nil, status.Error(codes.Internal, "character repo not configured")
	}

	// Seed the actor's turn economy in the character store if the encounter is
	// turn-based and the actor is the (unseeded) active actor (rpg-api#598). The
	// toolkit enforces the economy server-side and ActivateAbility requires
	// InCombat(); the v2 turn-based-entry sites flip to turn-based before any
	// character is held, so the first actor's stored ActionEconomy is nil. This
	// runs the toolkit's StartTurn for that actor and writes the seeded economy
	// to the store, so the Get below reads it back — replacing the former
	// rpg-api-authored ActionEconomy{1,1,1,30} injection (a North-Star Invariant 2
	// violation). ActivateFeature uses the #691 CharDataJSON self-load (not the
	// #689 cascade), so it seeds explicitly here rather than via the orchestrator
	// load path. No-op when not turn-based / not the active actor / already seeded.
	if encData, encErr := h.encRepo.Get(ctx, req.GetEncounterId()); encErr == nil && encData != nil {
		if seedErr := SeedActorTurnEconomyForData(
			ctx, encData, encountercore.EntityID(req.GetCharacterId()), h.combatResolverConfig.CharacterRepo,
		); seedErr != nil {
			return nil, status.Errorf(codes.Internal, "seed turn economy %q: %v", req.GetEncounterId(), seedErr)
		}
	}

	// Load the character from the repo. The character data is needed to build
	// the CharDataJSON the toolkit verb self-loads from (#691: the verb manages
	// the actor's character itself rather than via the #689 cascade).
	charOut, err := h.combatResolverConfig.CharacterRepo.Get(ctx, characterrepo.GetInput{ID: req.GetCharacterId()})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load character %q: %v", req.GetCharacterId(), err)
	}
	if charOut == nil || charOut.Character == nil || charOut.Character.Data == nil {
		return nil, status.Errorf(codes.NotFound, "character %q not found", req.GetCharacterId())
	}
	charData := charOut.Character.Data

	charJSON, err := json.Marshal(charData)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal character data: %v", err)
	}

	featureRef := fr.GetModule() + ":" + fr.GetType() + ":" + fr.GetId()

	// Delegate the load → ActivateFeature verb → persist cycle to the
	// orchestrator. The orchestrator does exactly one repo.Get + LoadFromData +
	// verb + repo.Save (the single-load guarantee that makes the modifier-ID
	// double-subscribe class structurally impossible), and returns the verb's
	// UpdatedCharData for this handler to flush to the character store.
	out, err := h.orch.ActivateFeature(ctx, &encounterorch.ActivateFeatureInput{
		EncounterID:  req.GetEncounterId(),
		PlayerID:     encountercore.PlayerID(playerID),
		ActorID:      encountercore.EntityID(req.GetCharacterId()),
		FeatureRef:   featureRef,
		CharDataJSON: json.RawMessage(charJSON),
	})
	if err != nil {
		return nil, activateFeatureStatusError(err)
	}

	// Persist the updated character data returned by the toolkit verb.
	// The verb owns the rule mutations (resource decrement, condition append);
	// rpg-api stores the result.
	var updatedData tkcharacter.Data
	if err := json.Unmarshal(out.UpdatedCharData, &updatedData); err != nil {
		return nil, status.Errorf(codes.Internal, "unmarshal updated character data: %v", err)
	}

	if _, err := h.combatResolverConfig.CharacterRepo.Update(ctx, characterrepo.UpdateInput{
		Character: &entities.Character{Data: &updatedData},
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "save character %q: %v", req.GetCharacterId(), err)
	}

	return &encounterv2pb.ActivateFeatureResponse{}, nil
}

// activateFeatureStatusError maps the orchestrator's ActivateFeature errors onto
// gRPC status codes per pat-v2-status-code-mapping. Orchestrator load sentinels
// carry the auth classification; the ErrActivateFeatureRefused wrapper carries
// the state-dependent verb refusal.
//
//   - ErrEncounterNotFound → NotFound
//   - ErrPlayerNotInEncounter / ErrEntityOwnershipMismatch → PermissionDenied
//   - ErrActivateFeatureRefused (toolkit verb refuses) → FailedPrecondition
//   - load / save / unclassified failures → Internal
func activateFeatureStatusError(err error) error {
	switch {
	case errors.Is(err, encounterorch.ErrEncounterNotFound):
		return status.Error(codes.NotFound, "encounter not found")
	case errors.Is(err, encounterorch.ErrPlayerNotInEncounter):
		return status.Error(codes.PermissionDenied, "player is not in this encounter")
	case errors.Is(err, encounterorch.ErrEntityOwnershipMismatch):
		return status.Error(codes.PermissionDenied,
			"character_id does not match player's controlled entity")
	case errors.Is(err, encounterorch.ErrActivateFeatureRefused):
		return status.Errorf(codes.FailedPrecondition, "%v", err)
	}
	return status.Errorf(codes.Internal, "activate feature: %v", err)
}
