package encounter_test

// integration_movement_oa_test.go — Wave 2.11e #539 sign-off integration tests.
//
// Verifies the MovementResolver wiring works end-to-end through the v2 handler:
//   - Player movement (real MoveEntity RPC) triggers MovementChain → NPC's OA
//     condition publishes a ReactionTriggerEvent → triggerOpportunityAttack
//     fires inline against the player → damage applied.
//   - NPC movement (driven via MoveNPCSteps for deterministic path) triggers
//     MovementChain → Player's OA condition publishes a ReactionTriggerEvent
//     → player's OA fires inline against the NPC → damage applied.
//
// Both tests drive the LOAD-BEARING flow per director brief on rpg-api#539:
// combat.MoveEntity → MovementChain publish → OA condition predicate →
// triggerOpportunityAttack → combat.ResolveAttack → damage event. They do
// NOT construct PhasedAttackContext or MovementStepResult directly — every
// chain step runs production code.
//
// NPC-OA-only scope (per #658 Q1=(b) signoff): no player-pause branch. All
// OAs auto-fire inline; the encounter SDK's trigger buffer catches the
// ReactionTriggerEvents for observability but does not partition or act
// on them. Player-pause for Sentinel-shape reactions deferred to
// rpg-toolkit#665.
//
// # Monster movement path (Scenario A — TestPlayerOA_OnNPCFleeing)
//
// Production NPC movement goes through NPCAct → monster.TakeTurn → AI
// chooses the path → applyNPCMovement → MovementResolver iteration. The
// AI typically moves TOWARD enemies (so the goblin would not flee
// adjacent fighter naturally), making it impractical to set up the
// retreat scenario through NPCAct's AI alone.
//
// We drive this scenario via Encounter.MoveNPCSteps (the public seam
// added in rpg-toolkit#672 PR review). MoveNPCSteps exercises the SAME
// applyNPCMovementSteps + iterateMovementStepsForEntity helpers that
// NPCAct uses for movement — the only thing bypassed is the AI's choice
// of where to move. The load-bearing chain (MovementChain → OA → attack)
// runs identically. This matches the director brief's intent ("drive
// the real flow") since the goal is to prove the chain wiring works,
// not to retest monster AI targeting.

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
	tkencevents "github.com/KirkDiggler/rpg-toolkit/encounter/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

const (
	movEncID         = "enc-movement-oa-integration"
	movPlayerAlice   = "alice"
	movEntityAlice   = "char-alice-fighter"
	movGoblinID      = "goblin-mov"
	movPlayerStartHP = 30
	movGoblinStartHP = 100 // big HP so OA damage doesn't kill it across tests
	// movRollVal = 18: d20=18 + ATK_BONUS → hits any reasonable AC; not nat-20
	// so no crit chain branching.
	movRollVal = 18
)

// MovementOAIntegrationSuite verifies the MovementResolver wiring on the v2
// handler stack: player movement triggers NPC OA, NPC movement triggers
// player OA.
type MovementOAIntegrationSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	charStore    *inMemoryCharStore
	broker       *tkenc.Broker
	repo         encountersv2.Repository
	handler      *v2encounter.Handler
	aliceCtx     context.Context
}

func TestMovementOAIntegrationSuite(t *testing.T) {
	suite.Run(t, new(MovementOAIntegrationSuite))
}

func (s *MovementOAIntegrationSuite) SetupTest() {
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
			Roller:        fixedRoller{val: movRollVal},
		},
		MovementResolverConfig: &v2encounter.Dnd5eMovementResolverConfig{
			CharacterRepo: s.mockCharRepo,
			Roller:        fixedRoller{val: movRollVal},
		},
	})
	s.Require().NoError(err)
	s.handler = h

	s.aliceCtx = auth.WithPlayerID(context.Background(), movPlayerAlice)
}

func (s *MovementOAIntegrationSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestPlayerOA_OnNPCFleeing verifies the wave-goal scenario: a player
// (fighter alice) has OA ready by default (seedOAReadiness fires when a
// melee combatant is added). When a goblin adjacent to alice retreats
// out of reach, alice's OpportunityAttackCondition predicate matches →
// publishes ReactionTriggerEvent → combat.MoveEntity's
// triggerOpportunityAttack fires inline → alice's OA attack hits the
// goblin → goblin takes damage.
//
// Drive path: enc.MoveNPCSteps (test seam from rpg-toolkit#672) with a
// retreat-direction path. See file-level docstring for why we don't
// drive through NPCAct here.
func (s *MovementOAIntegrationSuite) TestPlayerOA_OnNPCFleeing() {
	aliceData := s.buildAliceFighterData()
	s.setupCharRepoMock(aliceData)
	s.seedMovementEncounter()

	// Drive the NPC retreat through the MovementResolver chain. Load the
	// encounter via the test broker (matches what handler RPC does
	// internally), call MoveNPCSteps, save back.
	data, err := s.repo.Get(context.Background(), movEncID)
	s.Require().NoError(err)

	enc := s.loadEncForMovementTest(data)

	// Goblin retreats from (1,0,-1) → (2,0,-2). Alice is at (0,0,0); the
	// goblin starts within reach (distance 1) and ends outside reach
	// (distance 2). OA condition predicate should match on this step.
	err = enc.MoveNPCSteps(movGoblinID, []encountercore.Hex{
		{Q: 2, R: 0, S: -2},
	})
	s.Require().NoError(err, "MoveNPCSteps must succeed; OA fires inline against the goblin")

	s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))

	// Goblin should have taken damage from alice's OA. Reload and assert.
	finalData, err := s.repo.Get(context.Background(), movEncID)
	s.Require().NoError(err)

	gob := finalData.Monsters[encountercore.EntityID(movGoblinID)]
	s.Require().NotNil(gob, "goblin must still be in encounter")
	s.Less(gob.HP, movGoblinStartHP,
		"goblin HP must drop below starting HP (%d) — alice's OA hit (current HP: %d)",
		movGoblinStartHP, gob.HP)

	// Goblin landed at the retreat hex.
	s.Equal(encountercore.Hex{Q: 2, R: 0, S: -2}, gob.Position,
		"goblin should land at the retreat hex after the chain runs")
}

// TestNPCOA_OnPlayerFleeing verifies the mirror direction: alice retreats
// past an adjacent goblin → goblin's OA fires inline → alice takes
// damage. Drives through the real handler.MoveEntity RPC path.
func (s *MovementOAIntegrationSuite) TestNPCOA_OnPlayerFleeing() {
	aliceData := s.buildAliceFighterData()
	s.setupCharRepoMock(aliceData)
	s.seedMovementEncounter()

	// Capture broker events to verify the OA fires end-to-end. We assert on
	// DamageDealtEvent firing (target=alice, source=goblin) rather than on
	// HP delta because triggerOpportunityAttack in the toolkit uses
	// weapons.UnarmedStrike as a placeholder (see
	// rulebooks/dnd5e/combat/movement.go:getAttackerMeleeWeapon TODO) — for
	// a goblin (STR 8, -1 mod) the unarmed strike damage rolls 1 + (-1) = 0.
	// The wiring proof is the DamageDealtEvent itself: it fires only when the
	// full chain (MovementChain → OA condition predicate → triggerOA →
	// combat.ResolveAttack → DamageReceivedEvent → applyMoveDamage) runs.
	//
	// Follow-up: weapon-aware OA attacker (toolkit#NEW — file separately).
	sub, subErr := s.broker.Subscribe(movEncID, encountercore.PlayerID(movPlayerAlice))
	s.Require().NoError(subErr)
	defer func() { _ = sub.Close() }()
	var damageEvents []*tkencevents.DamageDealtEvent
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for evt := range sub.Events() {
			if de, ok := evt.(*tkencevents.DamageDealtEvent); ok {
				damageEvents = append(damageEvents, de)
			}
		}
	}()

	// Alice retreats from (0,0,0) → (-1,0,1), away from the goblin at
	// (1,0,-1). She leaves the goblin's reach (distance 1 → 2). The
	// goblin's OpportunityAttackCondition predicate should match on this
	// step → trigger event published → triggerOpportunityAttack fires
	// inline against alice.
	_, err := s.handler.MoveEntity(s.aliceCtx, &encounterv2pb.MoveEntityRequest{
		EncounterId: movEncID,
		EntityId:    movEntityAlice,
		ProposedPath: []*encounterv2pb.Position{
			{X: -1, Y: 0, Z: 1},
		},
	})
	s.Require().NoError(err, "MoveEntity RPC must succeed; OA fires inline against alice")

	// Brief drain so broker forwards events to the subscription.
	time.Sleep(50 * time.Millisecond)
	_ = sub.Close()
	<-doneCh

	// Locate the OA damage event: target=alice, source=goblin. Without the
	// full chain wired, no DamageDealtEvent fires at all.
	var oaDamage *tkencevents.DamageDealtEvent
	for _, de := range damageEvents {
		if de.TargetID == encountercore.EntityID(movEntityAlice) &&
			de.SourceID == encountercore.EntityID(movGoblinID) {
			oaDamage = de
			break
		}
	}
	s.Require().NotNil(oaDamage,
		"OA's DamageDealtEvent (target=alice, source=goblin) must fire — proves the full chain wires up: MovementChain → OA predicate → triggerOpportunityAttack → applyMoveDamage")

	// Alice landed at the retreat hex.
	finalData, err := s.repo.Get(context.Background(), movEncID)
	s.Require().NoError(err)
	alice := finalData.Players[encountercore.PlayerID(movPlayerAlice)]
	s.Require().NotNil(alice, "alice must still be in encounter")
	s.Require().NotNil(alice.View)
	s.Equal(encountercore.Hex{Q: -1, R: 0, S: 1}, alice.View.Position,
		"alice should land at the retreat hex after the chain runs")
}

// Disengage suppression test is intentionally NOT included in this slice.
//
// The full production path is: handler.TakeAction(Disengage) →
// combatabilities.Disengage.Activate → conditions.DisengagingCondition.Apply
// on the encounter's dnd5e bus. The condition subscribes to MovementChain
// and adds OAPreventionSources when the owner moves.
//
// But the encounter SDK creates a FRESH dnd5e bus per LoadFromData (see
// encounter.go:193: `bus: dnd5events.NewEventBus()`). The TakeAction RPC
// returns, the bus is torn down, and the subsequent MoveEntity RPC builds
// a new encounter with a new bus — the DisengagingCondition is gone.
//
// Cross-RPC persistence is a separate design problem (filed as follow-up;
// see comment in reaction_conditions.go on why universal Apply isn't a
// fix — the condition has no "activated this turn" gate so universal
// application would suppress OAs forever). Until that lands, an
// integration test here can only exercise the OA-fires direction, not
// the OA-suppressed direction.
//
// Toolkit-side: combatabilities.Disengage.Activate + DisengagingCondition
// suppression are covered by rulebooks/dnd5e/combatabilities tests
// (#666) and conditions/disengaging_test.go — both verify the condition
// gates OAPreventionSources correctly when present.

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// loadEncForMovementTest constructs an Encounter from data using the same
// resolver wiring as the production handler. Used by TestPlayerOA_OnNPCFleeing
// to drive MoveNPCSteps without going through the handler RPC layer.
func (s *MovementOAIntegrationSuite) loadEncForMovementTest(data *tkenc.Data) *tkenc.Encounter {
	movResolver := v2encounter.NewDnd5eMovementResolverForData(
		v2encounter.Dnd5eMovementResolverConfig{
			CharacterRepo: s.mockCharRepo,
			Roller:        fixedRoller{val: movRollVal},
		},
		data,
	)
	combResolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{
			CharacterRepo: s.mockCharRepo,
			Roller:        fixedRoller{val: movRollVal},
		},
		data,
	)
	enc, err := tkenc.LoadFromData(data, s.broker,
		tkenc.WithCombatResolver(combResolver),
		tkenc.WithMovementResolver(movResolver))
	s.Require().NoError(err)
	return enc
}

// setupCharRepoMock wires the mockCharRepo to read/write through inMemoryCharStore.
func (s *MovementOAIntegrationSuite) setupCharRepoMock(aliceData *character.Data) {
	s.charStore.set(aliceData)

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: movEntityAlice}).
		DoAndReturn(func(_ context.Context, _ characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			d := s.charStore.get(movEntityAlice)
			if d == nil {
				return nil, fmt.Errorf("alice not found in charStore")
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
		Get(gomock.Any(), gomock.Not(characterrepo.GetInput{ID: movEntityAlice})).
		Return(nil, fmt.Errorf("character not configured in integration test mock")).
		AnyTimes()
}

// buildAliceFighterData constructs a level-1 fighter — STR 14 (+2), DEX 14,
// AC 16, HP 30. The fighter has OA default-ready via seedOAReadiness when
// added as a combatant (DamageDice non-empty).
func (s *MovementOAIntegrationSuite) buildAliceFighterData() *character.Data {
	return &character.Data{
		ID:               movEntityAlice,
		Name:             "Alice the Fighter",
		Level:            1,
		ClassID:          "fighter",
		ProficiencyBonus: 2,
		HitPoints:        movPlayerStartHP,
		MaxHitPoints:     movPlayerStartHP,
		ArmorClass:       16,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 14,
			abilities.DEX: 14,
			abilities.CON: 14,
			abilities.INT: 10,
			abilities.WIS: 10,
			abilities.CHA: 10,
		},
	}
}

// seedMovementEncounter creates the fixture encounter: alice at (0,0,0)
// adjacent to goblin at (1,0,-1). Both are combatants (have melee
// DamageDice) so they get default-on OA readiness from seedOAReadiness.
//
// Goblin uses monster.NewGoblin DataJSON so NPCAct would have a real AI
// blob — though TestPlayerOA_OnNPCFleeing uses MoveNPCSteps directly
// (not NPCAct) per file-level docstring rationale.
func (s *MovementOAIntegrationSuite) seedMovementEncounter() {
	enc := tkenc.New(movEncID, s.broker)

	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   encountercore.PlayerID(movPlayerAlice),
		EntityID:   encountercore.EntityID(movEntityAlice),
		Position:   encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
		HP:         movPlayerStartHP, MaxHP: movPlayerStartHP, AC: 16,
		AttackBonus: 4, // STR 14 (+2) + proficiency 2
		DamageDice:  "1d8+2",
		DamageType:  "slashing",
	}))

	goblin := monster.NewGoblin(movGoblinID)
	goblinData := goblin.ToData()
	goblinData.HitPoints = movGoblinStartHP
	goblinData.MaxHitPoints = movGoblinStartHP
	goblinDataJSON, err := json.Marshal(goblinData)
	s.Require().NoError(err, "marshal goblin data")

	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID:          encountercore.EntityID(movGoblinID),
		Position:    encountercore.Hex{Q: 1, R: 0, S: -1},
		HP:          movGoblinStartHP, MaxHP: movGoblinStartHP, AC: 15,
		Speed:       6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
		DataJSON:    goblinDataJSON,
	}))

	s.Require().NoError(enc.SetMode(encountercore.ModeTurnBased))
	s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))
}
