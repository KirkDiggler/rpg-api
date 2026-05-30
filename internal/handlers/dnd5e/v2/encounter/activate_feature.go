package encounter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	dnd5eResources "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
)

const (
	featureModuleDnd5e  = "dnd5e"
	featureTypeFeatures = "features"
	featureIDRage       = "rage"

	// rageDamageBonusL1 is the flat damage bonus for a level-1 barbarian while
	// raging. Higher levels get more (Level 9 → +3, Level 16 → +4) but Wave 3
	// only wires L1 barbarians. The level stored on the character data is the
	// authoritative source; this constant is the L1 default used during the
	// ActivateFeature handler for first-wave Rage wiring.
	rageDamageBonusL1 = 2
)

// ActivateFeature applies a character feature (e.g. Rage) as an in-encounter
// action. The response is intentionally empty — state changes (condition applied,
// resource decremented) flow via the EncounterEvent stream so all subscribers
// observe the updated state without polling.
//
// Feature dispatch: the handler routes by feature_ref.id so adding a second
// feature (e.g. Second Wind in Wave 4) is a small switch-case addition, not a
// rewrite.
//
// Wave 3 wires ONLY Rage:
//  1. Validate the character has the Rage feature and available RageCharges.
//  2. Construct a RagingCondition (damage bonus + resistance) and apply it to
//     the encounter bus so all subsequent combat chain calls see the modifiers.
//  3. Persist the condition JSON into character.Data.Conditions and decrement
//     RageCharges (2 → 1).
//  4. Save the updated encounter (so the condition is wired into the bus on the
//     next LoadFromData) and character.
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

	// Route by feature_ref.id. Adding Second Wind or other features in future
	// waves is a new case here.
	switch fr.GetId() {
	case featureIDRage:
		return h.activateRage(ctx, req.GetEncounterId(), req.GetCharacterId(), playerID)
	default:
		return nil, status.Errorf(codes.Unimplemented, "feature %q is not implemented", fr.GetId())
	}
}

// activateRage applies the Rage condition to the character:
//  1. Load encounter + character.
//  2. Validate: not already raging; RageCharges > 0.
//  3. Construct + Apply RagingCondition on the encounter bus.
//  4. Persist the condition to character.Data.Conditions; decrement RageCharges.
//  5. Save encounter + character.
func (h *Handler) activateRage(ctx context.Context, encounterID, characterID, playerID string) (*encounterv2pb.ActivateFeatureResponse, error) {
	if h.combatResolverConfig.CharacterRepo == nil {
		return nil, status.Error(codes.Internal, "character repo not configured")
	}

	// Load encounter.
	data, err := h.encRepo.Get(ctx, encounterID)
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal, "load encounter %q: %v", encounterID, err)
	}

	// Verify player is in the encounter.
	if _, ok := data.Players[core.PlayerID(playerID)]; !ok {
		return nil, status.Error(codes.PermissionDenied, "player is not in this encounter")
	}

	// Load encounter with bus so RagingCondition's Apply() can subscribe.
	enc, err := tkenc.LoadFromData(data, h.broker,
		tkenc.WithCharacterResolver(h.resolver),
		tkenc.WithCombatResolver(h.buildCombatResolver(data)),
		tkenc.WithMovementResolver(h.buildMovementResolver(data)))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load encounter from data %q: %v", encounterID, err)
	}

	// Load character data.
	charOut, err := h.combatResolverConfig.CharacterRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load character %q: %v", characterID, err)
	}
	if charOut == nil || charOut.Character == nil || charOut.Character.Data == nil {
		return nil, status.Errorf(codes.NotFound, "character %q not found", characterID)
	}
	charData := charOut.Character.Data

	// Validate: not already raging.
	if isAlreadyRaging(charData) {
		return nil, status.Error(codes.FailedPrecondition, "character is already raging")
	}

	// Validate: rage charges available.
	rageRes, hasCharges := charData.Resources[dnd5eResources.RageCharges]
	if !hasCharges || rageRes.Current <= 0 {
		return nil, status.Error(codes.FailedPrecondition, "no rage charges remaining")
	}

	// Construct the RagingCondition and serialize to JSON for persistence.
	// We do NOT Apply() the condition to the bus here — the condition will be
	// rehydrated via character.LoadFromData on the next TakeAction/EndTurn RPC
	// when the combat resolver loads the character from the repo. Applying it
	// now would cause a "modifier ID already exists" error on that next Load
	// because the condition would be applied twice to the same bus.
	condOut, err := conditions.CreateFromRef(&conditions.CreateFromRefInput{
		Ref:         "dnd5e:conditions:raging",
		CharacterID: characterID,
		SourceRef:   "dnd5e:features:rage",
		Config:      json.RawMessage(fmt.Sprintf(`{"damage_bonus":%d,"level":%d}`, rageDamageBonusL1, charData.Level)),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create raging condition: %v", err)
	}

	// Persist: serialize the condition to JSON and append to character.Data.Conditions.
	condJSON, err := condOut.Condition.ToJSON()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "serialize raging condition: %v", err)
	}
	charData.Conditions = append(charData.Conditions, condJSON)

	// Decrement RageCharges (2 → 1 at L1).
	rageRes.Current--
	charData.Resources[dnd5eResources.RageCharges] = rageRes

	// Save updated character.
	_, err = h.combatResolverConfig.CharacterRepo.Update(ctx, characterrepo.UpdateInput{
		Character: &entities.Character{Data: charData},
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "save character %q: %v", characterID, err)
	}

	// Save updated encounter so the persisted RagingCondition condition JSON
	// rehydrates on the next LoadFromData (via character.LoadFromData in the
	// combat resolver).
	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter %q: %v", encounterID, err)
	}

	return &encounterv2pb.ActivateFeatureResponse{}, nil
}

// isAlreadyRaging checks character.Data.Conditions for a persisted RagingCondition.
// core.Ref marshals as a string "module:type:id" (custom MarshalJSON), so we
// peek at the "ref" field as a raw string and compare it to the canonical Rage
// condition ref string.
func isAlreadyRaging(data *tkcharacter.Data) bool {
	for _, blob := range data.Conditions {
		var peek struct {
			Ref string `json:"ref"`
		}
		if err := json.Unmarshal(blob, &peek); err != nil {
			continue
		}
		if peek.Ref == "dnd5e:conditions:raging" {
			return true
		}
	}
	return false
}
