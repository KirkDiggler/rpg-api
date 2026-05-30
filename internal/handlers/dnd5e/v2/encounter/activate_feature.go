package encounter

// activate_feature.go — ActivateFeature handler.
//
// The handler is a thin envelope over the toolkit's Encounter.ActivateFeature
// verb. It owns exactly four things:
//  1. Validate the request envelope (encounter_id, character_id, feature_ref).
//  2. Load the character from the repo + prepare CharDataJSON for the toolkit.
//  3. Load the encounter via the Runner (load→verb→persist pattern).
//  4. Persist the UpdatedCharData returned by the verb.
//
// NO rule logic lives here. The toolkit's Encounter.ActivateFeature owns all
// rule meaning (resource cost, condition construction, tier table).
// rpg-api passes the ref through and stores the result.
//
// The Runner is the load path: each call to Runner.Run calls repo.Get +
// tkenc.LoadFromData + verb + repo.Save exactly once. The single load path
// makes the "modifier ID already exists" double-subscribe class impossible.

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// combatActionEconomy is the minimal in-combat action economy injected when the
// character's stored data does not have ActionEconomy set. The toolkit's
// character.ActivateAbility requires InCombat()==true; since an ActivateFeature
// RPC by definition occurs during an active encounter, the character IS in combat
// even if the serialized data predates the current turn's StartTurn call.
//
// Values: 1 action + 1 bonus action (both available so Rage's bonus-action cost
// does not block activation). Turn 0 keeps this neutral.
var combatActionEconomy = &tkcharacter.ActionEconomyData{
	TurnNumber:            0,
	ActionsRemaining:      1,
	BonusActionsRemaining: 1,
	ReactionsRemaining:    1,
	MovementRemaining:     30,
}

// ActivateFeature applies a character feature (e.g. Rage) as an in-encounter
// action. All rule logic — resource cost, condition construction, tier table —
// lives in the toolkit's Encounter.ActivateFeature verb. This handler passes
// the feature ref through and persists the updated character data.
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

	// Load the character from the repo. The character data is needed to build
	// the CharDataJSON the toolkit verb requires.
	charOut, err := h.combatResolverConfig.CharacterRepo.Get(ctx, characterrepo.GetInput{ID: req.GetCharacterId()})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load character %q: %v", req.GetCharacterId(), err)
	}
	if charOut == nil || charOut.Character == nil || charOut.Character.Data == nil {
		return nil, status.Errorf(codes.NotFound, "character %q not found", req.GetCharacterId())
	}
	charData := charOut.Character.Data

	// Ensure InCombat() == true when passing charData to the toolkit verb.
	// ActivateAbility requires the character to be in combat, but the stored
	// character data may not have ActionEconomy set. Injecting a minimal
	// combat economy here is correct: this RPC is by definition called during
	// an active encounter.
	if charData.ActionEconomy == nil {
		aeCopy := *combatActionEconomy
		charData.ActionEconomy = &aeCopy
	}

	charJSON, err := json.Marshal(charData)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal character data: %v", err)
	}

	featureRef := fr.GetModule() + ":" + fr.GetType() + ":" + fr.GetId()

	// Use the Runner for the load→verb→persist cycle. The Runner calls
	// repo.Get + LoadFromData + verb + repo.Save exactly once. This single
	// load path is the fix: no scattered loaders means the modifier-ID
	// double-subscribe class is structurally impossible.
	var out *tkenc.ActivateFeatureOutput
	if runErr := h.runner.Run(ctx, &EncounterRunnerInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    encountercore.PlayerID(playerID),
		EntityID:    req.GetCharacterId(),
	}, func(enc *tkenc.Encounter, _ *tkenc.Data) error {
		var verbErr error
		out, verbErr = enc.ActivateFeature(ctx, &tkenc.ActivateFeatureInput{
			ActorID:      encountercore.EntityID(req.GetCharacterId()),
			FeatureRef:   featureRef,
			CharDataJSON: json.RawMessage(charJSON),
		})
		return verbErr
	}); runErr != nil {
		// Runner.Run returns gRPC status errors for all non-verb failures
		// (NotFound, PermissionDenied, Internal). Propagate them unchanged
		// so clients receive the correct code. errors.Is against the raw
		// ErrNotFound sentinel is intentionally omitted — the runner already
		// converts it to status.Error(codes.NotFound) before returning.
		return nil, runErr
	}

	// Persist the updated character data returned by the toolkit verb.
	// The verb owns the rule mutations (resource decrement, condition append);
	// rpg-api stores the result.
	//
	// Defensive nil check: enc.ActivateFeature must return non-nil output on
	// success (project rule: never return (nil, nil)), but guard here so a
	// future toolkit regression produces a clean codes.Internal rather than
	// a nil-pointer panic.
	if out == nil {
		return nil, status.Error(codes.Internal, "toolkit ActivateFeature returned nil output with nil error")
	}
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
