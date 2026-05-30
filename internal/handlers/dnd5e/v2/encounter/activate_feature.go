package encounter

// activate_feature.go — clean ActivateFeature handler (Wave 0, Task 5).
//
// The handler is a thin envelope over the toolkit's Encounter.ActivateFeature
// verb. It owns exactly four things:
//  1. Validate the request envelope (encounter_id, character_id, feature_ref).
//  2. Load the character from the repo + prepare CharDataJSON for the toolkit.
//  3. Load the encounter and run the verb (load→verb→persist pattern).
//  4. Persist the UpdatedCharData returned by the verb.
//
// NO rule logic lives here. The old activateRage / rageDamageBonusForLevel /
// isAlreadyRaging / charge-math / conditions.CreateFromRef calls are gone.
// The toolkit's Encounter.ActivateFeature owns all rule meaning; rpg-api
// passes the ref through and stores the result.
//
// NOTE: This handler follows the same load→verb→persist pattern as TakeAction
// and EndTurn (inline LoadFromData) rather than delegating to the Runner.
// The Runner is the preferred future pattern; ActivateFeature is currently
// implemented inline to match the existing TakeAction/EndTurn structure
// while the runner is being introduced incrementally.

import (
	"context"
	"encoding/json"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
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
// does not block activation). Turn 0 keeps this neutral (rpg-api does not track
// the actual turn number here).
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

	// Load encounter data. Read before LoadFromData so the ownership check
	// doesn't pay rehydration cost on auth-fail path.
	data, err := h.encRepo.Get(ctx, req.GetEncounterId())
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal, "load encounter %q: %v", req.GetEncounterId(), err)
	}

	pd, ok := data.Players[encountercore.PlayerID(playerID)]
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "player is not in this encounter")
	}
	if string(pd.EntityID) != req.GetCharacterId() {
		return nil, status.Error(codes.PermissionDenied, "character_id does not match player's controlled entity")
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
	// character data may not have ActionEconomy set (it's only serialized during
	// active turns). Injecting a minimal combat economy here is correct: this RPC
	// is by definition called during an active encounter.
	if charData.ActionEconomy == nil {
		aeCopy := *combatActionEconomy
		charData.ActionEconomy = &aeCopy
	}

	charJSON, err := json.Marshal(charData)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal character data: %v", err)
	}

	featureRef := fr.GetModule() + ":" + fr.GetType() + ":" + fr.GetId()

	// Load the encounter onto the broker bus — same pattern as TakeAction and EndTurn.
	enc, err := tkenc.LoadFromData(data, h.broker,
		tkenc.WithCharacterResolver(h.resolver),
		tkenc.WithCombatResolver(h.buildCombatResolver(data)),
		tkenc.WithMovementResolver(h.buildMovementResolver(data)))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load encounter from data %q: %v", req.GetEncounterId(), err)
	}

	// Invoke the toolkit verb. The verb publishes ConditionApplied and
	// ResourceChanged to the broker; StreamEncounter forwards them to clients.
	// The verb owns all rule logic (resource decrement, condition construction).
	out, err := enc.ActivateFeature(ctx, &tkenc.ActivateFeatureInput{
		ActorID:      encountercore.EntityID(req.GetCharacterId()),
		FeatureRef:   featureRef,
		CharDataJSON: json.RawMessage(charJSON),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "activate feature %q: %v", featureRef, err)
	}

	// Save the encounter state (sequence counter updated by broker publishes).
	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter %q: %v", req.GetEncounterId(), err)
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
