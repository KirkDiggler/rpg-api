package encounter_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	dnd5eResources "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// ActivateFeature orchestrator tests (#582 step 4). These exercise the
// orchestrator's load → ActivateFeature verb → persist core directly, proving:
//   - the shared load preconditions (membership + entity ownership) classify as
//     the orchestrator sentinels the handler maps to gRPC codes;
//   - toolkit verb refusals are joined with ErrActivateFeatureRefused;
//   - the happy path applies the feature and returns the verb's UpdatedCharData
//     while persisting the encounter snapshot.
//
// #691 (toolkit, OPEN): ActivateFeature runs its load WITHOUT WithCharacterData
// — the verb self-loads the actor from CharDataJSON. The happy-path test below
// proves this self-load path works end-to-end (the Rage condition applies and
// the resource decrements) without a double-subscribe collision.

const (
	afEncID     = "enc-activate"
	afPlayerBob = "bob"
	afEntityBob = "char-bob"
	afRageRef   = "dnd5e:features:rage"
)

type ActivateFeatureSuite struct {
	suite.Suite

	ctx    context.Context
	broker *tkenc.Broker
	repo   encountersv2.Repository
	orch   *encounterorch.Orchestrator
}

func (s *ActivateFeatureSuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:                 s.broker,
		EncounterRepo:          s.repo,
		BuildCharacterResolver: constCharacterResolver(stubCharacterResolver{}),
		// ActivateFeature does not invoke the combat / movement resolvers (the
		// verb self-loads the actor), but the orchestrator requires the builders
		// to be present.
		BuildCombatResolver: func(_ *tkenc.Data) encounterorch.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)
	s.orch = orch
}

// seedBob persists an encounter with bob (player) controlling char-bob in
// turn-based mode so the toolkit verb sees a valid player seat.
func (s *ActivateFeatureSuite) seedBob(encID string) {
	enc := tkenc.New(s.ctx, core.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   afPlayerBob,
		EntityID:   afEntityBob,
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 4,
		HP:         14,
		MaxHP:      14,
		AC:         14,
	}))
	s.Require().NoError(enc.SetMode(core.ModeTurnBased))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))
}

// bobBarbarianJSON builds a minimal L1 barbarian character.Data blob with the
// Rage feature, RageCharges 2/2, and an in-combat ActionEconomy so the toolkit
// verb's ActivateAbility (which requires InCombat()) succeeds. Marshaled to the
// opaque CharDataJSON the orchestrator passes straight through.
func (s *ActivateFeatureSuite) bobBarbarianJSON() json.RawMessage {
	rageFeatureJSON := json.RawMessage(fmt.Sprintf(`{"ref":%q,"id":"rage","name":"Rage","level":1}`,
		refs.Features.Rage()))

	data := &character.Data{
		ID:               afEntityBob,
		Name:             "Bob the Barbarian",
		Level:            1,
		ClassID:          "barbarian",
		ProficiencyBonus: 2,
		HitPoints:        14,
		MaxHitPoints:     14,
		ArmorClass:       14,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 13,
			abilities.CON: 16,
			abilities.INT: 8,
			abilities.WIS: 12,
			abilities.CHA: 10,
		},
		Features: []json.RawMessage{rageFeatureJSON},
		Resources: map[coreResources.ResourceKey]character.RecoverableResourceData{
			dnd5eResources.RageCharges: {
				Current:   2,
				Maximum:   2,
				ResetType: coreResources.ResetLongRest,
			},
		},
		ActionEconomy: &character.ActionEconomyData{
			TurnNumber:            0,
			ActionsRemaining:      1,
			BonusActionsRemaining: 1,
			ReactionsRemaining:    1,
			MovementRemaining:     30,
		},
	}
	raw, err := json.Marshal(data)
	s.Require().NoError(err, "marshal bob barbarian data")
	return raw
}

func (s *ActivateFeatureSuite) activate(encID, actor, featureRef string, charJSON json.RawMessage) (*encounterorch.ActivateFeatureOutput, error) {
	return s.orch.ActivateFeature(s.ctx, &encounterorch.ActivateFeatureInput{
		EncounterID:  encID,
		PlayerID:     afPlayerBob,
		ActorID:      core.EntityID(actor),
		FeatureRef:   featureRef,
		CharDataJSON: charJSON,
	})
}

// --- nil input ---

func (s *ActivateFeatureSuite) TestActivateFeature_NilInput_Error() {
	_, err := s.orch.ActivateFeature(s.ctx, nil)
	s.Require().Error(err)
}

// --- load preconditions ---

func (s *ActivateFeatureSuite) TestActivateFeature_MissingEncounter_ErrEncounterNotFound() {
	_, err := s.activate("nope", afEntityBob, afRageRef, s.bobBarbarianJSON())
	s.Require().ErrorIs(err, encounterorch.ErrEncounterNotFound)
}

func (s *ActivateFeatureSuite) TestActivateFeature_PlayerNotInEncounter_ErrPlayerNotInEncounter() {
	enc := tkenc.New(s.ctx, "enc-no-player", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "other", EntityID: "other-char", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.activate("enc-no-player", afEntityBob, afRageRef, s.bobBarbarianJSON())
	s.Require().ErrorIs(err, encounterorch.ErrPlayerNotInEncounter)
}

func (s *ActivateFeatureSuite) TestActivateFeature_EntityMismatch_ErrEntityOwnershipMismatch() {
	s.seedBob("enc-mismatch")
	_, err := s.activate("enc-mismatch", "char-someone-else", afRageRef, s.bobBarbarianJSON())
	s.Require().ErrorIs(err, encounterorch.ErrEntityOwnershipMismatch)
}

// --- verb refusal wrapped in the sentinel ---
//
// An unknown feature ref is a toolkit verb refusal (ActivateAbility can't find
// the feature). The toolkit returns a plain error; the orchestrator joins it
// with ErrActivateFeatureRefused so the handler maps to FailedPrecondition
// without string matching.
func (s *ActivateFeatureSuite) TestActivateFeature_UnknownFeature_ErrActivateFeatureRefused() {
	s.seedBob("enc-unknown-feature")
	_, err := s.activate("enc-unknown-feature", afEntityBob, "dnd5e:features:does-not-exist", s.bobBarbarianJSON())
	s.Require().ErrorIs(err, encounterorch.ErrActivateFeatureRefused)
}

// --- happy path: feature applies, UpdatedCharData returned, snapshot persisted ---
//
// Proves the #691 self-load path works end-to-end: the verb self-loads bob from
// CharDataJSON (the orchestrator's load did NOT attach it via the #689 cascade),
// applies Rage, decrements RageCharges 2 → 1, and returns the updated blob — all
// without a double-subscribe collision.
func (s *ActivateFeatureSuite) TestActivateFeature_Rage_AppliesAndReturnsUpdatedCharData() {
	s.seedBob(afEncID)

	out, err := s.activate(afEncID, afEntityBob, afRageRef, s.bobBarbarianJSON())
	s.Require().NoError(err, "ActivateFeature(Rage) must succeed via the toolkit verb self-load")
	s.Require().NotNil(out)
	s.Require().NotEmpty(out.UpdatedCharData, "verb must return the updated character serialization")

	// The orchestrator passes UpdatedCharData through opaquely; decode it here in
	// the test (handler-side concern) to assert the rule effects applied.
	var updated character.Data
	s.Require().NoError(json.Unmarshal(out.UpdatedCharData, &updated))

	rageRes, ok := updated.Resources[dnd5eResources.RageCharges]
	s.Require().True(ok, "RageCharges resource must still exist after activate")
	s.Require().Equal(1, rageRes.Current, "RageCharges must decrement from 2 to 1")

	s.Require().True(hasRagingCondition(updated.Conditions),
		"updated character data must carry a RagingCondition blob after ActivateFeature(Rage)")

	// The encounter snapshot must still be persisted (no error from persist).
	loaded, getErr := s.repo.Get(s.ctx, afEncID)
	s.Require().NoError(getErr)
	s.Require().NotNil(loaded)
}

// hasRagingCondition inspects condition blobs for a RagingCondition ref.
func hasRagingCondition(blobs []json.RawMessage) bool {
	for _, blob := range blobs {
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

func TestActivateFeatureSuite(t *testing.T) {
	suite.Run(t, new(ActivateFeatureSuite))
}
