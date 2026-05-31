package encounter_test

// integration_barbarian_rage_test.go — Wave 3 sign-off integration tests.
//
// These tests verify the ActivateFeature → Rage end-to-end chain:
//
//  1. ActivateFeature (Rage) persists the RagingCondition in bob's Conditions
//     and decrements RageCharges (2 → 1).
//  2. Raging bob attacks the goblin via TakeAction → DamageDealtEvent.Components
//     contains a Rage damage component (source: "dnd5e:conditions:raging").
//  3. Goblin attacks raging bob (NPC dispatch via EndTurn) → EntityDamagedEvent
//     amount is HALVED compared to a non-raging baseline (Rage physical resistance).
//
// The Phase 1 proof (rpg-api#577): the resistance test (3) was SKIPPED on PR #576
// because of the "modifier ID already exists" double-subscribe bug. The single
// load path (Runner.Run) makes this class of bug impossible. This test must PASS.
//
// Deterministic roller strategy
//
// fixedRoller{val:10} (from integration_sneak_attack_test.go):
//
//	bob attacks: d20 = min(10,20)=10 + attackBonus(5) = 15 ≥ goblin AC 15 → always hit, no crit.
//	Weapon 1d12: Roll(12) = min(10,12) = 10; damage = 10 + STR(+3) = 13, plus Rage +2 = 15.
//	Goblin attacks: d20=10, STR(-1)+prof(2)=1 → total=11 ≥ bob EffectiveAC(11) → hit.
//	Goblin damage (scimitar 1d6, Finesse → DEX +2): Roll(6)=min(10,6)=6; +2 → 8 per hit.
//	Non-raging bob: 8 damage. Raging bob: floor(8/2)=4 (Rage halves physical damage).

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	tkencevents "github.com/KirkDiggler/rpg-toolkit/encounter/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	dnd5eResources "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

const (
	rageIntegEncID = "enc-rage-integration"
	ragePlayerBob  = "bob"
	rageEntityBob  = "char-bob"
	rageGoblinID   = "goblin-rage"

	// goblin HP set high enough to survive raging-bob's attack so we can
	// observe the goblin's counter-attack on the same turn cycle.
	rageGoblinHP = 100

	// rageConditionSource matches what the toolkit emits in the
	// DamageComponent.Source field for the Rage condition.
	rageConditionSource = "dnd5e:conditions:raging"

	// rageChargesResourceRef is the canonical ref for rage_charges in the broker
	// ResourceChangedEvent (bare resource key, no module prefix).
	rageChargesResourceRef = "rage_charges"
)

// BarbarianRageIntegrationSuite tests the Wave 3 sign-off scenario end-to-end.
type BarbarianRageIntegrationSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	charStore    *inMemoryCharStore
	broker       *tkenc.Broker
	repo         encountersv2.Repository
	handler      *v2encounter.Handler
	bobCtx       context.Context
}

func TestBarbarianRageIntegrationSuite(t *testing.T) {
	suite.Run(t, new(BarbarianRageIntegrationSuite))
}

func (s *BarbarianRageIntegrationSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.charStore = newInMemoryCharStore()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	fixedNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h, err := v2encounter.New(&v2encounter.HandlerConfig{
		Broker: s.broker,
		Repo:   s.repo,
		Now:    func() time.Time { return fixedNow },
		CombatResolverConfig: &v2encounter.Dnd5eCombatResolverConfig{
			CharacterRepo: s.mockCharRepo,
			Roller:        fixedRoller{val: 10},
		},
	})
	s.Require().NoError(err)
	s.handler = h

	s.bobCtx = auth.WithPlayerID(context.Background(), ragePlayerBob)
}

func (s *BarbarianRageIntegrationSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestIntegration_ActivateRage_BrokerEmitsBothEvents verifies that after calling
// ActivateFeature(Rage), the broker emits:
//   - A ConditionAppliedEvent for the raging condition.
//   - A ResourceChangedEvent for rage_charges (2→1).
//
// This is the primary Wave 0 goal: broker events flow without rule logic in the
// handler. No tier table, no isAlreadyRaging, no charge math in rpg-api.
func (s *BarbarianRageIntegrationSuite) TestIntegration_ActivateRage_BrokerEmitsBothEvents() {
	bobData := s.buildBobBarbarianData()
	s.setupCharRepoMock(bobData)
	s.seedRageEncounter()

	sub, err := s.broker.Subscribe(
		encountercore.EncounterID(rageIntegEncID),
		encountercore.PlayerID(ragePlayerBob),
	)
	s.Require().NoError(err)
	defer sub.Close() //nolint:errcheck

	_, err = s.handler.ActivateFeature(s.bobCtx, &encounterv2pb.ActivateFeatureRequest{
		EncounterId: rageIntegEncID,
		CharacterId: rageEntityBob,
		FeatureRef:  &encounterv2pb.Ref{Module: "dnd5e", Type: "features", Id: "rage"},
	})
	s.Require().NoError(err, "ActivateFeature(Rage) must succeed via toolkit verb")

	condEvt, resEvt := s.drainForRageEvents(sub)
	s.Require().NotNil(condEvt,
		"broker must emit ConditionAppliedEvent (raging) when Rage is activated")
	s.Require().NotNil(resEvt,
		"broker must emit ResourceChangedEvent (rage_charges) when Rage is activated")

	// Validate ConditionAppliedEvent.
	s.Equal("char-bob", string(condEvt.TargetID))
	s.Equal("raging", condEvt.ConditionRef,
		"broker ConditionAppliedEvent.ConditionRef must be \"raging\" (toolkit ConditionType)")

	// Validate ResourceChangedEvent.
	s.Equal("char-bob", string(resEvt.EntityID))
	s.Equal(rageChargesResourceRef, resEvt.ResourceRef)
	s.Equal(1, resEvt.NewCurrent, "rage_charges must decrement from 2 to 1")
	s.Equal(2, resEvt.Max, "rage_charges max stays 2")
}

// TestIntegration_ActivateRage_PersistsConditionAndDecrementsCharges verifies
// that after calling ActivateFeature(Rage):
//   - Bob's Conditions JSON carries a RagingCondition blob.
//   - RageCharges decremented from 2 to 1.
func (s *BarbarianRageIntegrationSuite) TestIntegration_ActivateRage_PersistsConditionAndDecrementsCharges() {
	bobData := s.buildBobBarbarianData()
	s.setupCharRepoMock(bobData)
	s.seedRageEncounter()

	_, err := s.handler.ActivateFeature(s.bobCtx, &encounterv2pb.ActivateFeatureRequest{
		EncounterId: rageIntegEncID,
		CharacterId: rageEntityBob,
		FeatureRef:  &encounterv2pb.Ref{Module: "dnd5e", Type: "features", Id: "rage"},
	})
	s.Require().NoError(err, "ActivateFeature(Rage) must succeed")

	// Read bob's updated state from charStore.
	updated := s.charStore.get(rageEntityBob)
	s.Require().NotNil(updated, "bob must be in charStore after ActivateFeature")

	// Verify RagingCondition is persisted.
	s.True(s.hasRagingCondition(updated.Conditions),
		"bob's Conditions must carry a RagingCondition blob after ActivateFeature(Rage)")

	// Verify RageCharges decremented.
	rageRes, ok := updated.Resources[dnd5eResources.RageCharges]
	s.Require().True(ok, "RageCharges resource must still exist after activate")
	s.Equal(1, rageRes.Current,
		"RageCharges must decrement from 2 to 1 after ActivateFeature(Rage)")
}

// TestIntegration_RagingBobAttack_HasRageDamageComponent verifies that after
// activating Rage, raging bob's attack includes the Rage damage component in
// DamageDealtEvent.Components.
func (s *BarbarianRageIntegrationSuite) TestIntegration_RagingBobAttack_HasRageDamageComponent() {
	bobData := s.buildBobBarbarianData()
	s.setupCharRepoMock(bobData)
	s.seedRageEncounter()
	s.advanceToBob()

	// Activate Rage first.
	_, err := s.handler.ActivateFeature(s.bobCtx, &encounterv2pb.ActivateFeatureRequest{
		EncounterId: rageIntegEncID,
		CharacterId: rageEntityBob,
		FeatureRef:  &encounterv2pb.Ref{Module: "dnd5e", Type: "features", Id: "rage"},
	})
	s.Require().NoError(err, "ActivateFeature(Rage) must succeed before attack")

	// Subscribe before attack to capture DamageDealtEvent.
	sub, err := s.broker.Subscribe(
		encountercore.EncounterID(rageIntegEncID),
		encountercore.PlayerID(ragePlayerBob),
	)
	s.Require().NoError(err, "broker subscribe must succeed")
	defer sub.Close() //nolint:errcheck

	_, err = s.handler.TakeAction(s.bobCtx, s.attackBobVsGoblin())
	s.Require().NoError(err, "raging bob's attack must resolve without error")

	damageEvt := s.drainForDamageEvent(sub)
	s.Require().NotNil(damageEvt, "DamageDealtEvent must be published for bob's raging attack")
	s.Require().NotEmpty(damageEvt.Components, "DamageDealtEvent.Components must be non-empty")

	sources := make([]string, 0, len(damageEvt.Components))
	for _, c := range damageEvt.Components {
		sources = append(sources, c.Source)
	}
	s.Contains(sources, rageConditionSource,
		"damage breakdown must include a Rage component (source=%q); got %v",
		rageConditionSource, sources)
}

// TestIntegration_RageResistance_HalvesGoblinDamage verifies that Rage's physical
// resistance halves the damage raging-bob takes from the goblin's attack.
//
// This test was SKIPPED in PR #576 due to the "modifier ID already exists"
// double-subscribe bug. The single load path (Runner.Run → one LoadFromData)
// makes that class of bug impossible. This test MUST PASS.
//
// Strategy: run two separate encounters — one with Rage active, one without.
// Compare the HP delta from the goblin's attack in each scenario.
//
// Damage mechanics with fixedRoller{10}:
// Goblin scimitar (Finesse): Roll(6)=min(10,6)=6; DEX(+2) > STR(-1) → DEX used → 6+2=8.
// Bob's EffectiveAC: unarmored formula 10+DEX(+1)=11; roll=10, goblin bonus=1 → total=11 ≥ 11 → hit.
// Non-raging bob: 8 per goblin hit.
// Raging bob: floor(8/2) = 4 (Rage halves physical damage).
func (s *BarbarianRageIntegrationSuite) TestIntegration_RageResistance_HalvesGoblinDamage() {
	// --- Scenario 1: non-raging baseline ---
	baselineStore := newInMemoryCharStore()
	baselineBob := s.buildBobBarbarianData()
	baselineStore.set(baselineBob)

	baselineBobHP := s.goblinAttackBobHP(baselineStore, baselineBob, false)

	// --- Scenario 2: raging ---
	ragingStore := newInMemoryCharStore()
	ragingBob := s.buildBobBarbarianData()
	ragingStore.set(ragingBob)

	ragingBobHP := s.goblinAttackBobHP(ragingStore, ragingBob, true)

	baselineDelta := baselineBob.MaxHitPoints - baselineBobHP
	ragingDelta := ragingBob.MaxHitPoints - ragingBobHP

	s.Greater(baselineDelta, 0,
		"baseline: goblin must deal damage to non-raging bob (delta=%d)", baselineDelta)
	s.Greater(ragingDelta, 0,
		"raging: goblin must deal damage to raging bob (delta=%d)", ragingDelta)

	// Deterministic expected values with fixedRoller{10}.
	// Scimitar (Finesse): Roll(6)=min(10,6)=6; DEX(+2) > STR(-1) → DEX used → 6+2=8 per hit.
	// Raging: floor(8/2)=4 (Rage halves physical damage).
	const expectedBaselineDelta = 8
	const expectedRagingDelta = 4
	s.Equal(expectedBaselineDelta, baselineDelta,
		"non-raging baseline delta must be 8 (scimitar 1d6 + DEX +2) with fixedRoller{10}")
	s.Equal(expectedRagingDelta, ragingDelta,
		"raging delta must be floor(8/2)=4 (Rage halves physical damage) with fixedRoller{10}")

	// Rage resistance invariant: raging bob always takes less than non-raging.
	s.Less(ragingDelta, baselineDelta,
		"raging bob (delta=%d) must take less damage than non-raging bob (delta=%d); Rage resistance expected",
		ragingDelta, baselineDelta)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// goblinAttackBobHP runs a minimal scenario: seed encounter, optionally activate
// Rage, advance past bob's turn to trigger goblin's NPC attack via EndTurn, then
// return bob's current HP. Uses a fresh broker+repo per call to isolate scenarios.
func (s *BarbarianRageIntegrationSuite) goblinAttackBobHP(
	store *inMemoryCharStore,
	bobData *character.Data,
	withRage bool,
) int {
	ctrl := gomock.NewController(s.T())
	defer ctrl.Finish()

	mockRepo := charactermock.NewMockRepository(ctrl)
	broker := tkenc.NewBroker(tkenc.NewInMemoryTransport())
	repo := encountersv2.NewInMemory()

	// Wire mock to read/write from store.
	mockRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: rageEntityBob}).
		DoAndReturn(func(_ context.Context, _ characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			d := store.get(rageEntityBob)
			if d == nil {
				return nil, fmt.Errorf("bob not found in store")
			}
			return &characterrepo.GetOutput{Character: &entities.Character{Data: d}}, nil
		}).AnyTimes()
	mockRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			if input.Character != nil && input.Character.Data != nil {
				store.update(input.Character.Data)
			}
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		}).AnyTimes()
	mockRepo.EXPECT().
		Get(gomock.Any(), gomock.Not(characterrepo.GetInput{ID: rageEntityBob})).
		Return(nil, fmt.Errorf("not in test mock")).
		AnyTimes()

	h, err := v2encounter.New(&v2encounter.HandlerConfig{
		Broker: broker,
		Repo:   repo,
		Now:    func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		CombatResolverConfig: &v2encounter.Dnd5eCombatResolverConfig{
			CharacterRepo: mockRepo,
			Roller:        fixedRoller{val: 10},
		},
	})
	s.Require().NoError(err)

	bobCtx := auth.WithPlayerID(context.Background(), ragePlayerBob)

	// Seed encounter (unique ID per call to avoid cross-test contamination).
	encID := fmt.Sprintf("enc-rage-resistance-%v", withRage)
	enc := tkenc.New(context.Background(), encountercore.EncounterID(encID), broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    encountercore.PlayerID(ragePlayerBob),
		EntityID:    encountercore.EntityID(rageEntityBob),
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          bobData.HitPoints,
		MaxHP:       bobData.MaxHitPoints,
		AC:          bobData.ArmorClass,
		AttackBonus: 5,
		DamageDice:  "1d12+3",
		DamageType:  "slashing",
	}))

	// Build goblin with DataJSON so the Dnd5eCombatResolver can rehydrate
	// full ability scores for deterministic attack resolution.
	g := monster.NewGoblin(rageGoblinID)
	gData := g.ToData()
	gDataJSON, err := json.Marshal(gData)
	s.Require().NoError(err, "marshal goblin data for resistance test")
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID:          encountercore.EntityID(rageGoblinID),
		Position:    encountercore.Hex{Q: 1, R: 0, S: -1},
		HP:          rageGoblinHP,
		MaxHP:       rageGoblinHP,
		AC:          gData.ArmorClass,
		Speed:       6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
		DataJSON:    gDataJSON,
	}))
	s.Require().NoError(enc.SetMode(encountercore.ModeTurnBased))
	s.Require().NoError(repo.Save(context.Background(), enc.ToData()))

	// Advance to bob's turn.
	s.advanceToActor(h, broker, repo, encID, rageEntityBob, rageGoblinID)

	if withRage {
		_, err = h.ActivateFeature(bobCtx, &encounterv2pb.ActivateFeatureRequest{
			EncounterId: encID,
			CharacterId: rageEntityBob,
			FeatureRef:  &encounterv2pb.Ref{Module: "dnd5e", Type: "features", Id: "rage"},
		})
		s.Require().NoError(err, "ActivateFeature(Rage) must succeed")
	}

	// Bob attacks goblin, then ends turn — NPC dispatch will handle goblin's attack.
	_, err = h.TakeAction(bobCtx, &encounterv2pb.TakeActionRequest{
		EncounterId:   encID,
		ActorEntityId: rageEntityBob,
		ActionRef:     &encounterv2pb.Ref{Module: "dnd5e", Type: "action", Id: "attack"},
		Target: &encounterv2pb.ActionTarget{
			Kind: &encounterv2pb.ActionTarget_EntityId{EntityId: rageGoblinID},
		},
	})
	s.Require().NoError(err, "bob's attack must succeed")

	// End bob's turn — NPC dispatch will run the goblin's turn and attack bob.
	_, err = h.EndTurn(bobCtx, &encounterv2pb.EndTurnRequest{
		EncounterId: encID,
		EntityId:    rageEntityBob,
	})
	s.Require().NoError(err, "EndTurn must succeed (goblin NPC dispatched)")

	// Return bob's current HP from the encounter snapshot.
	data, err := repo.Get(context.Background(), encID)
	s.Require().NoError(err)
	pd, ok := data.Players[encountercore.PlayerID(ragePlayerBob)]
	s.Require().True(ok, "bob must be in encounter player data")
	return pd.HP
}

// advanceToActor cycles initiative until the target actor is active, ending
// NPC turns directly via SDK and player turns via handler.
func (s *BarbarianRageIntegrationSuite) advanceToActor(
	h *v2encounter.Handler,
	broker *tkenc.Broker,
	repo encountersv2.Repository,
	encID, targetEntityID, npcEntityID string,
) {
	bobCtx := auth.WithPlayerID(context.Background(), ragePlayerBob)
	for range 20 {
		data, err := repo.Get(context.Background(), encID)
		s.Require().NoError(err)
		s.Require().NotEmpty(data.Initiative, "initiative must be rolled")

		if data.ActiveIdx < 0 || data.ActiveIdx >= len(data.Initiative) {
			s.Fail("invalid ActiveIdx", data.ActiveIdx)
		}
		activeID := string(data.Initiative[data.ActiveIdx])

		if activeID == targetEntityID {
			return
		}

		if activeID == npcEntityID {
			enc, loadErr := tkenc.LoadFromData(context.Background(), data, broker)
			s.Require().NoError(loadErr)
			_, _, err = enc.EndTurn(context.Background(), encountercore.EntityID(activeID))
			s.Require().NoError(err)
			s.Require().NoError(repo.Save(context.Background(), enc.ToData()))
			continue
		}

		// Other player's turn — end via handler.
		_, err = h.EndTurn(bobCtx, &encounterv2pb.EndTurnRequest{
			EncounterId: encID,
			EntityId:    activeID,
		})
		s.Require().NoError(err, "EndTurn for actor %q must succeed", activeID)
	}
	s.Fail("advanceToActor: %q did not become active within 20 initiative steps", targetEntityID)
}

// buildBobBarbarianData constructs character.Data for bob — a L1 barbarian with
// STR 16 (+3) / DEX 13 (+1) / CON 16 (+3), greataxe in main hand, Rage feature
// persisted, RageCharges 2/2, NOT raging at start.
func (s *BarbarianRageIntegrationSuite) buildBobBarbarianData() *character.Data {
	rageFeatureJSON := json.RawMessage(fmt.Sprintf(`{"ref":%q,"id":"rage","name":"Rage","level":1}`,
		refs.Features.Rage()))

	return &character.Data{
		ID:               rageEntityBob,
		Name:             "Bob the Barbarian",
		Level:            1,
		ClassID:          "barbarian",
		ProficiencyBonus: 2,
		HitPoints:        14,
		MaxHitPoints:     14,
		ArmorClass:       14,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, // +3 — barbarian primary
			abilities.DEX: 13, // +1
			abilities.CON: 16, // +3
			abilities.INT: 8,
			abilities.WIS: 12,
			abilities.CHA: 10,
		},
		EquipmentSlots: character.EquipmentSlots{
			character.SlotMainHand: "greataxe",
		},
		Inventory: []character.InventoryItemData{
			{Type: "weapon", ID: "greataxe", Quantity: 1},
		},
		Features: []json.RawMessage{rageFeatureJSON},
		Resources: map[coreResources.ResourceKey]character.RecoverableResourceData{
			dnd5eResources.RageCharges: {
				Current:   2,
				Maximum:   2,
				ResetType: coreResources.ResetLongRest,
			},
		},
	}
}

// setupCharRepoMock registers mock expectations backed by inMemoryCharStore
// for the shared broker/repo/handler in this suite.
func (s *BarbarianRageIntegrationSuite) setupCharRepoMock(bobData *character.Data) {
	s.charStore.set(bobData)

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: rageEntityBob}).
		DoAndReturn(func(_ context.Context, _ characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			d := s.charStore.get(rageEntityBob)
			if d == nil {
				return nil, fmt.Errorf("bob not found in charStore")
			}
			return &characterrepo.GetOutput{Character: &entities.Character{Data: d}}, nil
		}).AnyTimes()

	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			if input.Character != nil && input.Character.Data != nil {
				s.charStore.update(input.Character.Data)
			}
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		}).AnyTimes()

	// Non-bob lookups return error so stand-in resolver is used.
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), gomock.Not(characterrepo.GetInput{ID: rageEntityBob})).
		Return(nil, fmt.Errorf("character not configured in barbarian integration test mock")).
		AnyTimes()
}

// attackBobVsGoblin returns a TakeActionRequest for bob attacking the goblin.
func (s *BarbarianRageIntegrationSuite) attackBobVsGoblin() *encounterv2pb.TakeActionRequest {
	return &encounterv2pb.TakeActionRequest{
		EncounterId:   rageIntegEncID,
		ActorEntityId: rageEntityBob,
		ActionRef:     &encounterv2pb.Ref{Module: "dnd5e", Type: "action", Id: "attack"},
		Target: &encounterv2pb.ActionTarget{
			Kind: &encounterv2pb.ActionTarget_EntityId{EntityId: rageGoblinID},
		},
	}
}

// seedRageEncounter creates the wave-3-barbarian fixture encounter and saves it.
func (s *BarbarianRageIntegrationSuite) seedRageEncounter() {
	enc := tkenc.New(context.Background(), rageIntegEncID, s.broker)

	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    encountercore.PlayerID(ragePlayerBob),
		EntityID:    encountercore.EntityID(rageEntityBob),
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          14,
		MaxHP:       14,
		AC:          14,
		AttackBonus: 5,
		DamageDice:  "1d12+3",
		DamageType:  "slashing",
	}))

	s.Require().NoError(enc.AddMonster(s.goblinMonsterInput(rageGoblinID, encountercore.Hex{Q: 1, R: 0, S: -1}, rageGoblinHP)))
	s.Require().NoError(enc.SetMode(encountercore.ModeTurnBased))
	s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))
}

// goblinMonsterInput returns a MonsterInput for a standard goblin with DataJSON
// embedded from monster.NewGoblin so the Dnd5eCombatResolver can rehydrate the
// full ability scores for deterministic attack resolution.
func (s *BarbarianRageIntegrationSuite) goblinMonsterInput(id string, pos encountercore.Hex, hp int) tkenc.MonsterInput {
	g := monster.NewGoblin(id)
	gData := g.ToData()
	gDataJSON, err := json.Marshal(gData)
	s.Require().NoError(err, "marshal goblin data")

	return tkenc.MonsterInput{
		ID:          encountercore.EntityID(id),
		Position:    pos,
		HP:          hp,
		MaxHP:       hp,
		AC:          gData.ArmorClass,
		Speed:       6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
		DataJSON:    gDataJSON,
	}
}

// advanceToBob cycles initiative in the shared broker/repo until bob is active.
func (s *BarbarianRageIntegrationSuite) advanceToBob() {
	s.advanceToActor(s.handler, s.broker, s.repo, rageIntegEncID, rageEntityBob, rageGoblinID)
}

// drainForRageEvents drains broker events for up to 2 seconds looking for
// both a ConditionAppliedEvent (raging) and a ResourceChangedEvent (rage_charges).
func (s *BarbarianRageIntegrationSuite) drainForRageEvents(sub *tkenc.Subscription) (
	*tkencevents.ConditionAppliedEvent,
	*tkencevents.ResourceChangedEvent,
) {
	var condEvt *tkencevents.ConditionAppliedEvent
	var resEvt *tkencevents.ResourceChangedEvent

	timeout := time.After(2 * time.Second)
	for {
		if condEvt != nil && resEvt != nil {
			return condEvt, resEvt
		}
		select {
		case evt, ok := <-sub.Events():
			if !ok {
				return condEvt, resEvt
			}
			switch e := evt.(type) {
			case *tkencevents.ConditionAppliedEvent:
				if e.ConditionRef == "raging" {
					condEvt = e
				}
			case *tkencevents.ResourceChangedEvent:
				if e.ResourceRef == rageChargesResourceRef {
					resEvt = e
				}
			}
		case <-timeout:
			return condEvt, resEvt
		}
	}
}

// drainForDamageEvent drains broker events for up to 2 seconds looking for a
// DamageDealtEvent.
func (s *BarbarianRageIntegrationSuite) drainForDamageEvent(sub *tkenc.Subscription) *tkencevents.DamageDealtEvent {
	timeout := time.After(2 * time.Second)
	for {
		select {
		case evt, ok := <-sub.Events():
			if !ok {
				return nil
			}
			if d, ok := evt.(*tkencevents.DamageDealtEvent); ok {
				return d
			}
		case <-timeout:
			return nil
		}
	}
}

// hasRagingCondition inspects condition blobs for a RagingCondition ref.
func (s *BarbarianRageIntegrationSuite) hasRagingCondition(blobs []json.RawMessage) bool {
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
