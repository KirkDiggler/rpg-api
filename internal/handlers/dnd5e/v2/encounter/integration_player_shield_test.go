package encounter_test

// integration_player_shield_test.go — Wave 2.11d closeout, partial #536.
//
// Ships ONE Shield integration test: TestPlayerShield_NotReady_NoPrompt.
// Proves the readiness gate: when Shield is Apply()'d to a spellcaster
// (gated on hasFirstLevelSpellSlot — unblocked by rpg-toolkit#660 LoadFromData
// fix landed in v0.58.1) but readiness is OFF (default), Shield's predicate
// does NOT publish a ReactionTriggerEvent. No PendingReactionPrompts gets
// persisted. The attack proceeds inline and hits.
//
// Two other originally-scoped Shield tests deferred to rpg-api#541 +
// rpg-toolkit#662 per closeout B12 finding:
//   - TestPlayerShield_ReadyAndPrompted_Blocks: blocked by SDK gap —
//     Encounter.CompleteTakeAction rejects NPC attackers, so the only Shield-
//     fires direction in PvE scope (NPC → player) can't be resumed.
//   - TestPlayerShield_ReadyAndSkipped_Hits: same B12 blocker.
//
// Shield predicate band (rulebooks/dnd5e/conditions/shield_spell.go):
//   - Target of the attack is the bearer (the wizard)
//   - WouldHit == true (attack total >= AC)
//   - Not nat-20
//   - TotalAttack < AC + 5 (so +5 AC would deflect)
//   - gamectx.IsReactionReady(wizard, Shield) == true
//
// The not-ready scenario gates on the last bullet: the predicate's first
// four checks all match (attack lands in band), but the readiness flag
// returns false → no trigger event published → no prompt → attack hits
// inline at the legacy single-phase path.
//
// Scenario:
//   - wendy (level-1 wizard, AC 12, HP 8) with SpellSlots[1].Max=2 to qualify
//     for Shield-Apply via applyReactionConditions.hasFirstLevelSpellSlot.
//     SpellSlots survive Get→LoadFromData lifecycle thanks to v0.58.1.
//   - goblin (NewGoblin DataJSON: AB +4, scimitar 1d6+2) attacks wendy.
//
// Deterministic roller: fixedRoller{val: 12} makes goblin's d20 = 12,
// total = 12 + 4 = 16. With wendy's AC 12: 16 >= 12 (hits), 16 < 17 (Shield
// WOULD deflect if ready). Squarely in the Shield-eligible band — so this
// test proves the readiness gate is the only thing stopping the prompt.
//
// Damage 1d6+2: Roll(6) = min(12,6) = 6; +2 = 8. Drops wendy 8 → 0.

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
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

const (
	shieldIntegEncID  = "enc-shield-integration"
	shieldPlayerWendy = "wendy"
	shieldEntityWendy = "char-wendy"
	shieldGoblinID    = "goblin-shield"

	// shieldFullHP is wendy's HP — chosen so unblocked = 0 (clear signal).
	shieldFullHP = 8

	// shieldRollVal: fixedRoller{val:12} → goblin d20=12 + AB 4 = 16.
	// Wendy AC 12: 16 >= 12 hits; 16 < 17 = AC + Shield's +5 — in-band.
	shieldRollVal = 12

	// shieldExpectedDamage: weapon 1d6+2 → Roll(6) = min(12,6)=6; +2 = 8.
	// Drops wendy from 8 → 0 if Shield doesn't block.
	shieldExpectedDamage = 8
)

// PlayerShieldIntegrationSuite tests the Shield readiness gate end-to-end
// through the v2 handler. Wave 2.11d ships this test; the prompted/skipped
// scenarios deferred to Wave 2.11e (rpg-api#541, blocked by rpg-toolkit#662).
type PlayerShieldIntegrationSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	charStore    *inMemoryCharStore // hoisted from sneak test
	broker       *tkenc.Broker
	repo         encountersv2.Repository
	handler      *v2encounter.Handler
	wendyCtx     context.Context
}

// TestPlayerShieldIntegrationSuite runs the Wave 2.11d Shield-gate suite.
func TestPlayerShieldIntegrationSuite(t *testing.T) {
	suite.Run(t, new(PlayerShieldIntegrationSuite))
}

// SetupTest constructs a fresh broker + repo + handler per test for isolation.
func (s *PlayerShieldIntegrationSuite) SetupTest() {
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
			Roller:        fixedRoller{val: shieldRollVal},
		},
	})
	s.Require().NoError(err)
	s.handler = h

	s.wendyCtx = auth.WithPlayerID(context.Background(), shieldPlayerWendy)
}

// TearDownTest closes the gomock controller (asserts all EXPECT()s satisfied).
func (s *PlayerShieldIntegrationSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestPlayerShield_NotReady_NoPrompt verifies the unreadied path: wendy
// is a spellcaster with Shield Apply()'d during character rehydration
// (qualified by hasFirstLevelSpellSlot — relies on rpg-toolkit#660's
// LoadFromData SpellSlots fix). She does NOT call SetReactionReady, so
// Shield's gamectx.IsReactionReady gate blocks the trigger publish entirely.
//
// Assertions:
//   - No PendingReactionPrompts populated (the readiness gate held)
//   - Goblin attack hits inline (legacy single-phase path)
//   - wendy HP drops by full weapon damage
//
// This is the one Shield path that ships end-to-end in Wave 2.11d. The two
// prompted paths (Ready+Take, Ready+Skip) need rpg-toolkit#662 (CompleteTakeAction
// symmetry for NPC attackers) which is deferred to Wave 2.11e per
// closeout B12 finding.
func (s *PlayerShieldIntegrationSuite) TestPlayerShield_NotReady_NoPrompt() {
	wendyData := s.buildWendyWizardData()
	s.setupCharRepoMock(wendyData)
	s.seedShieldEncounter()

	// Do NOT call SetReactionReady. Shield stays off (default per
	// applyReactionConditions — spell-cost reactions are opt-in).

	s.advanceToWendyActiveTurn()

	_, err := s.handler.EndTurn(s.wendyCtx, &encounterv2pb.EndTurnRequest{
		EncounterId: shieldIntegEncID,
		EntityId:    shieldEntityWendy,
	})
	s.Require().NoError(err, "EndTurn must succeed; goblin attack runs inline since Shield is unready")

	// No prompt should have been persisted — Shield's readiness gate held.
	data, err := s.repo.Get(context.Background(), shieldIntegEncID)
	s.Require().NoError(err)
	_, ok := data.PendingReactionPrompts[encountercore.PlayerID(shieldPlayerWendy)]
	s.False(ok,
		"PendingReactionPrompts must NOT contain wendy's prompt — Shield was unready, predicate gated")

	// Wendy took the full hit (no Shield to deflect).
	hpAfter := s.wendyHP()
	delta := shieldFullHP - hpAfter
	s.Equal(shieldExpectedDamage, delta,
		"wendy HP delta (%d) must equal weapon damage (%d) — Shield was unready so the goblin's 16 hits cleanly",
		delta, shieldExpectedDamage)
}

// ---------------------------------------------------------------------------
// Helpers — Shield-specific. Generic helpers (fixedRoller, inMemoryCharStore)
// live in integration_sneak_attack_test.go in the same package.
// ---------------------------------------------------------------------------

// setupCharRepoMock wires the mockCharRepo to read/write through inMemoryCharStore.
// Mirror of the sneak suite's setup but for wendy as the sole player character;
// non-wendy lookups return an error so the resolver falls back to StandIn.
func (s *PlayerShieldIntegrationSuite) setupCharRepoMock(wendyData *character.Data) {
	s.charStore.set(wendyData)

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: shieldEntityWendy}).
		DoAndReturn(func(_ context.Context, _ characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			d := s.charStore.get(shieldEntityWendy)
			if d == nil {
				return nil, fmt.Errorf("wendy not found in charStore")
			}
			return &characterrepo.GetOutput{
				Character: &entities.Character{Data: d},
			}, nil
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
		Get(gomock.Any(), gomock.Not(characterrepo.GetInput{ID: shieldEntityWendy})).
		Return(nil, fmt.Errorf("character not configured in integration test mock")).
		AnyTimes()
}

// wendyHP returns wendy's HP from the persisted encounter snapshot.
func (s *PlayerShieldIntegrationSuite) wendyHP() int {
	data, err := s.repo.Get(context.Background(), shieldIntegEncID)
	s.Require().NoError(err, "repo.Get for wendy HP check")
	pd, ok := data.Players[encountercore.PlayerID(shieldPlayerWendy)]
	s.Require().True(ok, "wendy must be in encounter player map")
	return pd.HP
}

// buildWendyWizardData constructs a level-1 wizard with Shield-eligible
// shape: SpellSlots[1].Max=2 → applyReactionConditions.hasFirstLevelSpellSlot
// returns true → ShieldSpellCondition.Apply() on every encounter rehydration.
// AC 12 keeps the Shield band test deterministic against fixedRoller{val:12}.
//
// SpellSlots round-trip cleanly through Data → LoadFromData → ToData thanks
// to rpg-toolkit#660 (landed v0.58.1). Before that fix, this field would
// silently drop on rehydration and Shield would never Apply.
func (s *PlayerShieldIntegrationSuite) buildWendyWizardData() *character.Data {
	return &character.Data{
		ID:               shieldEntityWendy,
		Name:             "Wendy the Wizard",
		Level:            1,
		ClassID:          "wizard",
		ProficiencyBonus: 2,
		HitPoints:        shieldFullHP,
		MaxHitPoints:     shieldFullHP,
		ArmorClass:       12,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 8,
			abilities.DEX: 14,
			abilities.CON: 12,
			abilities.INT: 16, // wizard primary
			abilities.WIS: 10,
			abilities.CHA: 10,
		},
		SpellSlots: map[int]character.SpellSlotData{
			1: {Max: 2, Used: 0},
		},
	}
}

// seedShieldEncounter creates the fixture encounter. Wendy at Q:0,R:0;
// goblin at Q:2,R:0 (within scimitar reach after moveTowardEnemy in
// monster.TakeTurn).
//
// Goblin needs DataJSON so NPCAct goes through the rehydration +
// monster.TakeTurn → applyCapturedAttacks path. Without DataJSON, NPCAct
// falls back to npcActScripted which bypasses the phased path. We use
// monster.NewGoblin so the monster has realistic AbilityScores (a
// monster.New with zero AbilityScores produces a STR -5 modifier and can
// never hit anything).
func (s *PlayerShieldIntegrationSuite) seedShieldEncounter() {
	enc := tkenc.New(context.Background(), shieldIntegEncID, s.broker)

	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   encountercore.PlayerID(shieldPlayerWendy),
		EntityID:   encountercore.EntityID(shieldEntityWendy),
		Position:   encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
		HP:         shieldFullHP, MaxHP: shieldFullHP, AC: 12,
		AttackBonus: 0, // wizard isn't attacking; satisfies AddPlayer's combatant qualifier
		DamageDice:  "1d4",
		DamageType:  "force",
	}))

	goblin := monster.NewGoblin(shieldGoblinID)
	// Override HP so the goblin survives even if a test extends to multiple attacks.
	goblinData := goblin.ToData()
	goblinData.HitPoints = 100
	goblinData.MaxHitPoints = 100
	goblinDataJSON, err := json.Marshal(goblinData)
	s.Require().NoError(err, "marshal goblin data")

	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID:       encountercore.EntityID(shieldGoblinID),
		Position: encountercore.Hex{Q: 2, R: 0, S: -2},
		HP:       100, MaxHP: 100, AC: 15,
		Speed:       6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4, // matches scimitar action
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
		DataJSON:    goblinDataJSON,
	}))

	s.Require().NoError(enc.SetMode(encountercore.ModeTurnBased))
	s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))
}

// advanceToWendyActiveTurn walks initiative forward until wendy is the
// active actor. NPC turns encountered en route are force-ended via the SDK
// directly to avoid triggering NPCAct in this helper (the test method
// controls when the goblin's turn fires).
//
// Initiative is randomly rolled by SetMode, so this loop accepts either
// initial order. In practice ends in 1-2 iterations for a 2-combatant
// encounter.
func (s *PlayerShieldIntegrationSuite) advanceToWendyActiveTurn() {
	for i := 0; i < 4; i++ {
		data, err := s.repo.Get(context.Background(), shieldIntegEncID)
		s.Require().NoError(err)
		s.Require().NotEmpty(data.Initiative)
		if data.ActiveIdx < 0 || data.ActiveIdx >= len(data.Initiative) {
			s.Fail("invalid ActiveIdx", data.ActiveIdx)
		}
		active := string(data.Initiative[data.ActiveIdx])
		if active == shieldEntityWendy {
			return // caller's EndTurn will fire next, kicking off the goblin's NPCAct
		}
		// Active is the goblin — end its turn directly via SDK so we can
		// loop back to wendy first.
		enc, loadErr := tkenc.LoadFromData(context.Background(), data, s.broker)
		s.Require().NoError(loadErr)
		_, _, err = enc.EndTurn(context.Background(), encountercore.EntityID(active))
		s.Require().NoError(err)
		s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))
	}
	s.Fail("advanceToWendyActiveTurn: wendy did not become active within 4 turns")
}
