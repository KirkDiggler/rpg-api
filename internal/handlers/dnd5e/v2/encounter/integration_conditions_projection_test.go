package encounter_test

// integration_conditions_projection_test.go — rpg-api#651 sign-off integration
// tests: PlayerData.ActiveConditions / MonsterData.ActiveConditions
// (rpg-toolkit#754, filtered per rpg-toolkit#778) projected into
// Entity.status_effects at snapshot time, so a reconnecting client sees active
// battlefield statuses instead of nothing until the next live event.
//
// Three end-to-end proofs:
//  1. A character already raging (hydrated at load, not activated during
//     THIS test's observed window — no verb called) — a fresh StreamEncounter
//     connect's snapshot carries the rage status effect, and nothing further
//     arrives (no live event fires from the connect itself).
//  2. A Monk character carrying a permanently-granted MartialArts condition —
//     the toolkit's rpg-toolkit#778 filter excludes it from ActiveConditions,
//     so it never reaches status_effects. This is the whole provenance chain
//     end to end: a namespace filter in rpg-api could not make this
//     distinction (see rpg-toolkit#778's investigation trail); only the
//     toolkit's attachment-provenance-aware filter can.
//  3. Ending the encounter sweeps rage (rpg-toolkit#767/#752's
//     endCombatForPlayers, called from checkEncounterEnd) BEFORE the terminal
//     ToData() runs — a post-end snapshot must not show a stale rage badge.
//
// Fixture construction note: all three tests attach each character's
// DataJSON directly on tkenc.PlayerInput at AddPlayer time, then round-trip
// once through ToData -> json.Marshal -> json.Unmarshal -> LoadFromData
// before the FIRST repo.Save — mirroring the toolkit's own
// active_conditions_test.go's loadEncounterFromData helper exactly (its
// TestReconnect_PlayerCondition_VisibleInSnapshot_NoVerbCalled is this
// pattern's origin). This is deliberate, not incidental: ActiveConditions is
// computed by encounter.syncCombatantsToData, which only runs for entities
// currently held in e.combatants — and Encounter.ActivateFeature (the
// toolkit verb) deliberately does NOT hold the actor (rpg-api's orchestrator
// comment on WithCharacterData explains why: it would collide with
// ActivateFeature's own CharDataJSON self-load, the #684 class — tracked
// as the OPEN toolkit#691, out of scope here). So a character that only
// just activated Rage via ActivateFeature within this same test would NOT
// show up in ActiveConditions yet — a real, separate, already-tracked gap
// this suite deliberately routes around by building "already raging at
// load" fixtures instead, which is what "hydrated" means in the issue's own
// language anyway.

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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	dnd5eResources "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

const (
	condProjEncID       = "enc-conditions-projection"
	condProjPlayerRurik = "rurik"
	condProjEntityRurik = "char-rurik"
	condProjGoblinID    = "goblin-conditions-projection"

	// condProjGoblinKillableHP is under raging Rurik's deterministic attack
	// damage (15, same fixedRoller{val:10} math as integration_barbarian_rage_
	// test.go's header comment: 1d12 rolled-to-10 + STR(+3) + Rage(+2) = 15) so
	// one successful attack kills it outright, triggering checkEncounterEnd.
	condProjGoblinKillableHP = 10

	// condProjMonkPlayer / condProjMonkEntity are the Monk fixture (test 2):
	// carries a permanently-granted MartialArts condition that must never
	// badge, proving the toolkit's rpg-toolkit#778 filter end to end.
	condProjMonkPlayer = "charli"
	condProjMonkEntity = "char-charli"
)

// ConditionsProjectionIntegrationSuite exercises the same handler-level
// scaffolding as BarbarianRageIntegrationSuite (real broker, real in-memory
// encounter repo, real handler, mocked character repo backed by an
// inMemoryCharStore) — a fresh instance, not a shared one, so each test's
// fixtures don't leak into another's.
type ConditionsProjectionIntegrationSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	charStore    *inMemoryCharStore
	broker       *tkenc.Broker
	repo         encountersv2.Repository
	handler      *v2encounter.Handler
	rurikCtx     context.Context
}

func TestConditionsProjectionIntegrationSuite(t *testing.T) {
	suite.Run(t, new(ConditionsProjectionIntegrationSuite))
}

func (s *ConditionsProjectionIntegrationSuite) SetupTest() {
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

	s.rurikCtx = auth.WithPlayerID(context.Background(), condProjPlayerRurik)
}

func (s *ConditionsProjectionIntegrationSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestIntegration_RagingCharacter_ReconnectSnapshotCarriesStatusEffect_NoLiveEvent
// is the issue's headline proof: a character already raging when a client
// connects sees the rage badge in the connect snapshot. No verb is called in
// this test at all — rurik's RagingCondition is baked into his character
// fixture from the start, exactly matching what a reconnect after a server
// restart looks like (the character store has no memory of "how" the
// condition got there).
func (s *ConditionsProjectionIntegrationSuite) TestIntegration_RagingCharacter_ReconnectSnapshotCarriesStatusEffect_NoLiveEvent() {
	rurikData := s.buildRurikBarbarianData(true /* alreadyRaging */)
	s.setupCharRepoMock(rurikData)
	s.seedSoloRurikEncounter(rurikData)

	stream := newCapturingStream(s.rurikCtx)
	go func() {
		_ = s.handler.StreamEncounter(&encounterv2pb.StreamEncounterRequest{
			EncounterId: condProjEncID,
		}, stream)
	}()

	first := stream.WaitForSend(s.T(), 2*time.Second)
	snap := first.GetSnapshotDelivered()
	s.Require().NotNil(snap, "first envelope on connect must be SnapshotDelivered")

	var rurikEntity *encounterv2pb.Entity
	for _, e := range snap.GetEncounter().GetSpace().GetEntities() {
		if e.GetId() == condProjEntityRurik {
			rurikEntity = e
			break
		}
	}
	s.Require().NotNil(rurikEntity, "rurik must be in the connect snapshot")
	s.Require().Len(rurikEntity.GetStatusEffects(), 1,
		"a reconnecting client must see the already-active rage status effect in the snapshot")
	s.Equal("dnd5e", rurikEntity.GetStatusEffects()[0].GetSource().GetModule())
	s.Equal("conditions", rurikEntity.GetStatusEffects()[0].GetSource().GetType())
	s.Equal("raging", rurikEntity.GetStatusEffects()[0].GetSource().GetId())

	// Zero live events: nothing in this test does anything after the connect
	// that should produce a second envelope beyond the snapshot + its normal
	// replay set (EntityAppeared for rurik himself, GeometryRevealed).
	s.drainReplayThenAssertNoMore(stream)
}

// TestIntegration_MonkCharacter_MartialArtsDoesNotBadge is the provenance-chain
// proof: MartialArts is permanently granted at character construction
// (classes.Grant.Conditions, same mechanism as monster traits — see
// rpg-toolkit#778's investigation), never through ActivateFeature, so it must
// never appear as a status effect no matter how the character is hydrated.
// A ref-namespace filter in rpg-api could not make this distinction (both
// MartialArts and Raging are dnd5e:conditions:*); only the toolkit's
// attachment-provenance-aware ActiveConditions filter can, and this proves it
// actually does end to end — not just at the toolkit's own unit-test layer.
func (s *ConditionsProjectionIntegrationSuite) TestIntegration_MonkCharacter_MartialArtsDoesNotBadge() {
	charliData := s.buildCharliMonkDataWithMartialArts()
	s.setupCharliCharRepoMock(charliData)

	charliJSON, err := json.Marshal(charliData)
	s.Require().NoError(err, "marshal charli fixture")

	enc := tkenc.New(context.Background(), condProjEncID, s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   encountercore.PlayerID(condProjMonkPlayer),
		EntityID:   encountercore.EntityID(condProjMonkEntity),
		Position:   encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
		HP:         10,
		MaxHP:      10,
		AC:         15,
		DataJSON:   charliJSON,
	}))
	loaded := s.roundTripThroughLoadFromData(enc)
	s.Require().NoError(s.repo.Save(context.Background(), loaded.ToData()))

	monkCtx := auth.WithPlayerID(context.Background(), condProjMonkPlayer)
	stream := newCapturingStream(monkCtx)
	go func() {
		_ = s.handler.StreamEncounter(&encounterv2pb.StreamEncounterRequest{
			EncounterId: condProjEncID,
		}, stream)
	}()

	first := stream.WaitForSend(s.T(), 2*time.Second)
	snap := first.GetSnapshotDelivered()
	s.Require().NotNil(snap, "first envelope on connect must be SnapshotDelivered")

	var charliEntity *encounterv2pb.Entity
	for _, e := range snap.GetEncounter().GetSpace().GetEntities() {
		if e.GetId() == condProjMonkEntity {
			charliEntity = e
			break
		}
	}
	s.Require().NotNil(charliEntity, "charli must be in the connect snapshot")
	s.Require().Empty(charliEntity.GetStatusEffects(),
		"MartialArts is a permanent class-grant condition, never activated live — "+
			"it must never appear as a status effect, on any snapshot, ever")
}

// TestIntegration_EncounterEnd_SweepsRageFromSnapshot proves the sweep
// interaction: checkEncounterEnd's endCombatForPlayers (rpg-toolkit#767/#752)
// calls ExitCombat for every HELD player BEFORE the terminal ToData() runs,
// so ActiveConditions (and therefore status_effects) must NOT show a stale
// rage badge once the encounter has ended. Rurik starts already-raging (the
// DataJSON fixture, same as test 1); killing the goblin runs through
// TakeAction — a genuinely combat-capable verb that DOES use the #689
// hydration cascade (unlike ActivateFeature), so checkEncounterEnd's
// e.heldCharacter(rurik) lookup succeeds and the sweep actually runs.
func (s *ConditionsProjectionIntegrationSuite) TestIntegration_EncounterEnd_SweepsRageFromSnapshot() {
	rurikData := s.buildRurikBarbarianData(true /* alreadyRaging */)
	s.setupCharRepoMock(rurikData)
	s.seedKillableGoblinEncounter(rurikData)
	s.advanceToRurik()

	// Sanity: rage is visible before the kill (proves the sweep, not an
	// absence of rage in the first place, is what clears it later).
	preKill, err := s.handler.GetEncounter(s.rurikCtx, &encounterv2pb.GetEncounterRequest{
		EncounterId: condProjEncID,
	})
	s.Require().NoError(err)
	s.Require().Len(s.entityStatusEffects(preKill.GetEncounter(), condProjEntityRurik), 1,
		"rage must be visible before the kill, or this test proves nothing about the sweep")

	// One raging attack (15 damage, deterministic with fixedRoller{val:10})
	// kills the 10-HP goblin, dropping monster count to zero and triggering
	// checkEncounterEnd's sweep.
	_, err = s.handler.TakeAction(s.rurikCtx, &encounterv2pb.TakeActionRequest{
		EncounterId:   condProjEncID,
		ActorEntityId: condProjEntityRurik,
		ActionRef:     &encounterv2pb.Ref{Module: "dnd5e", Type: "action", Id: "attack"},
		Target: &encounterv2pb.ActionTarget{
			Kind: &encounterv2pb.ActionTarget_EntityId{EntityId: condProjGoblinID},
		},
	})
	s.Require().NoError(err, "raging rurik's kill-shot attack must resolve without error")

	loaded, err := s.repo.Get(context.Background(), condProjEncID)
	s.Require().NoError(err)
	s.Require().Equal(encountercore.ModeEnded, loaded.Mode,
		"encounter must be ModeEnded once the sole monster is dead")

	postKill, err := s.handler.GetEncounter(s.rurikCtx, &encounterv2pb.GetEncounterRequest{
		EncounterId: condProjEncID,
	})
	s.Require().NoError(err)
	s.Require().Empty(s.entityStatusEffects(postKill.GetEncounter(), condProjEntityRurik),
		"the post-encounter-end snapshot must not carry the swept rage condition")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *ConditionsProjectionIntegrationSuite) entityStatusEffects(
	enc *encounterv2pb.Encounter, entityID string,
) []*encounterv2pb.StatusEffect {
	for _, e := range enc.GetSpace().GetEntities() {
		if e.GetId() == entityID {
			return e.GetStatusEffects()
		}
	}
	s.Failf("entity not found in snapshot", "id=%q", entityID)
	return nil
}

// roundTripThroughLoadFromData mirrors rpg-toolkit's own
// active_conditions_test.go ActiveConditionsSuite.loadEncounterFromData: a
// persist-then-reload cycle with no verb called in between, so the result
// reflects exactly what a client asking for a snapshot immediately after a
// fresh load would see. This is what actually populates ActiveConditions —
// see the file-level doc comment for why.
func (s *ConditionsProjectionIntegrationSuite) roundTripThroughLoadFromData(enc *tkenc.Encounter) *tkenc.Encounter {
	raw, err := json.Marshal(enc.ToData())
	s.Require().NoError(err)
	var data tkenc.Data
	s.Require().NoError(json.Unmarshal(raw, &data))
	loaded, err := tkenc.LoadFromData(context.Background(), &data, s.broker)
	s.Require().NoError(err)
	return loaded
}

// drainReplayThenAssertNoMore drains the snapshot's normal replay set
// (EntityAppeared/GeometryRevealed — count varies, so drain by a short
// per-event timeout rather than a fixed count) then asserts nothing further
// arrives within a longer settle window.
func (s *ConditionsProjectionIntegrationSuite) drainReplayThenAssertNoMore(stream *capturingStream) {
	for {
		select {
		case <-stream.sent:
			continue // replay event — keep draining
		case <-time.After(200 * time.Millisecond):
			goto settled
		}
	}
settled:
	select {
	case extra := <-stream.sent:
		s.Failf("unexpected extra live envelope after snapshot+replay settled",
			"got %T", extra.GetEvent())
	case <-time.After(300 * time.Millisecond):
		// Expected: nothing more arrives.
	}
}

// ragingConditionBlob builds a RagingCondition JSON blob matching exactly
// what Encounter.ActivateFeature would have produced and persisted, for a
// character fixture that starts already-raging (no verb called).
func (s *ConditionsProjectionIntegrationSuite) ragingConditionBlob(characterID string) json.RawMessage {
	blob, err := json.Marshal(conditions.RagingData{
		Ref:         refs.Conditions.Raging(),
		CharacterID: characterID,
		DamageBonus: 2,
		Level:       1,
		Source:      "rage",
	})
	s.Require().NoError(err, "marshal RagingData fixture")
	return blob
}

// buildRurikBarbarianData mirrors integration_barbarian_rage_test.go's
// buildBobBarbarianData (same class, stats, and deterministic combat math) —
// a distinct fixture name/entity avoids any cross-suite ID confusion in test
// output, not because the fixtures differ. alreadyRaging attaches a
// RagingCondition blob directly (see the file-level doc comment for why this
// is the correct way to build a "hydrated, verb-free" raging fixture).
func (s *ConditionsProjectionIntegrationSuite) buildRurikBarbarianData(alreadyRaging bool) *character.Data {
	rageFeatureJSON := json.RawMessage(fmt.Sprintf(`{"ref":%q,"id":"rage","name":"Rage","level":1}`,
		refs.Features.Rage()))

	data := &character.Data{
		ID:               condProjEntityRurik,
		Name:             "Rurik the Barbarian",
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
	if alreadyRaging {
		data.Conditions = []json.RawMessage{s.ragingConditionBlob(condProjEntityRurik)}
	}
	return data
}

// buildCharliMonkDataWithMartialArts builds a Monk whose Conditions already
// carry a serialized MartialArtsCondition blob — exactly what a properly
// character-creation-granted Monk carries (classes.GetMonkGrants attaches it
// at level 1), built directly via the exported MartialArtsData shape rather
// than the full draft/finalize creation flow: the toolkit's rpg-toolkit#778
// filter matches on the literal Ref string regardless of how a condition
// blob was attached, so this is a faithful, minimal fixture for what it's
// testing (the filter), not a shortcut around it.
func (s *ConditionsProjectionIntegrationSuite) buildCharliMonkDataWithMartialArts() *character.Data {
	martialArtsBlob, err := json.Marshal(conditions.MartialArtsData{
		Ref:         refs.Conditions.MartialArts(),
		CharacterID: condProjMonkEntity,
		MonkLevel:   1,
	})
	s.Require().NoError(err, "marshal MartialArtsData fixture")

	return &character.Data{
		ID:               condProjMonkEntity,
		Name:             "Charli the Monk",
		Level:            1,
		ClassID:          "monk",
		ProficiencyBonus: 2,
		HitPoints:        10,
		MaxHitPoints:     10,
		ArmorClass:       15,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 10,
			abilities.DEX: 16, // +3
			abilities.CON: 13,
			abilities.INT: 10,
			abilities.WIS: 14, // +2 — unarmored defense
			abilities.CHA: 8,
		},
		Conditions: []json.RawMessage{martialArtsBlob},
	}
}

// setupCharRepoMock registers mock expectations for rurik, backed by
// inMemoryCharStore.
func (s *ConditionsProjectionIntegrationSuite) setupCharRepoMock(rurikData *character.Data) {
	s.charStore.set(rurikData)

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: condProjEntityRurik}).
		DoAndReturn(func(_ context.Context, _ characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			d := s.charStore.get(condProjEntityRurik)
			if d == nil {
				return nil, fmt.Errorf("rurik not found in charStore")
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

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), gomock.Not(characterrepo.GetInput{ID: condProjEntityRurik})).
		Return(nil, fmt.Errorf("character not configured in conditions-projection integration test mock")).
		AnyTimes()
}

// setupCharliCharRepoMock mirrors setupCharRepoMock for the Monk fixture (a
// separate test, no rurik in play — a dedicated mock keeps the two tests'
// character-store state fully isolated).
func (s *ConditionsProjectionIntegrationSuite) setupCharliCharRepoMock(charliData *character.Data) {
	s.charStore.set(charliData)

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: condProjMonkEntity}).
		DoAndReturn(func(_ context.Context, _ characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			d := s.charStore.get(condProjMonkEntity)
			if d == nil {
				return nil, fmt.Errorf("charli not found in charStore")
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

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), gomock.Not(characterrepo.GetInput{ID: condProjMonkEntity})).
		Return(nil, fmt.Errorf("character not configured in conditions-projection integration test mock")).
		AnyTimes()
}

// seedSoloRurikEncounter is test 1's fixture: rurik alone (no monster — the
// reconnect proof needs no combat, just a correctly-hydrated snapshot).
// rurikData's DataJSON is attached at AddPlayer time and round-tripped
// through LoadFromData before the first save, so ActiveConditions is
// correctly computed from the start (see the file-level doc comment).
func (s *ConditionsProjectionIntegrationSuite) seedSoloRurikEncounter(rurikData *character.Data) {
	rurikJSON, err := json.Marshal(rurikData)
	s.Require().NoError(err, "marshal rurik fixture")

	enc := tkenc.New(context.Background(), condProjEncID, s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    encountercore.PlayerID(condProjPlayerRurik),
		EntityID:    encountercore.EntityID(condProjEntityRurik),
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          14,
		MaxHP:       14,
		AC:          14,
		AttackBonus: 5,
		DamageDice:  "1d12+3",
		DamageType:  "slashing",
		DataJSON:    rurikJSON,
	}))
	loaded := s.roundTripThroughLoadFromData(enc)
	s.Require().NoError(s.repo.Save(context.Background(), loaded.ToData()))
}

// seedKillableGoblinEncounter is the sweep test's fixture: rurik
// (already-raging, DataJSON-attached + round-tripped) plus a goblin with HP
// under raging rurik's deterministic one-hit damage (see
// condProjGoblinKillableHP's doc).
func (s *ConditionsProjectionIntegrationSuite) seedKillableGoblinEncounter(rurikData *character.Data) {
	rurikJSON, err := json.Marshal(rurikData)
	s.Require().NoError(err, "marshal rurik fixture")

	enc := tkenc.New(context.Background(), condProjEncID, s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    encountercore.PlayerID(condProjPlayerRurik),
		EntityID:    encountercore.EntityID(condProjEntityRurik),
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          14,
		MaxHP:       14,
		AC:          14,
		AttackBonus: 5,
		DamageDice:  "1d12+3",
		DamageType:  "slashing",
		DataJSON:    rurikJSON,
	}))
	s.Require().NoError(enc.AddMonster(s.goblinMonsterInput(condProjGoblinID,
		encountercore.Hex{Q: 1, R: 0, S: -1}, condProjGoblinKillableHP)))
	// AddMonster inline-checks combat entry (rpg-toolkit#759): rurik (sight
	// range 10, no room) already sees the goblin, so the encounter
	// self-transitions to TURN_BASED here.
	loaded := s.roundTripThroughLoadFromData(enc)
	s.Require().NoError(s.repo.Save(context.Background(), loaded.ToData()))
}

// goblinMonsterInput mirrors integration_barbarian_rage_test.go's helper of
// the same name.
func (s *ConditionsProjectionIntegrationSuite) goblinMonsterInput(
	id string, pos encountercore.Hex, hp int,
) tkenc.MonsterInput {
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

// advanceToRurik cycles initiative until rurik is the active actor — a
// 2-combatant (rurik + one goblin) simplification of
// integration_barbarian_rage_test.go's advanceToActor (only ever needs to
// skip past the goblin's own turn, never a third player's).
func (s *ConditionsProjectionIntegrationSuite) advanceToRurik() {
	for range 20 {
		data, err := s.repo.Get(context.Background(), condProjEncID)
		s.Require().NoError(err)
		s.Require().NotEmpty(data.Initiative, "initiative must be rolled")
		s.Require().True(data.ActiveIdx >= 0 && data.ActiveIdx < len(data.Initiative), "ActiveIdx must be in range")

		activeID := string(data.Initiative[data.ActiveIdx])
		if activeID == condProjEntityRurik {
			return
		}

		// Only the goblin can be active otherwise (2-combatant encounter) —
		// end its turn directly via the toolkit (no handler RPC for NPC turns).
		enc, loadErr := tkenc.LoadFromData(context.Background(), data, s.broker)
		s.Require().NoError(loadErr)
		_, _, err = enc.EndTurn(context.Background(), encountercore.EntityID(activeID))
		s.Require().NoError(err)
		s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))
	}
	s.Fail("advanceToRurik: rurik did not become active within 20 initiative steps")
}
