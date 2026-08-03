package encounter_test

// integration_sneak_attack_test.go — Wave 2.11c sign-off integration tests.
//
// These tests verify the three core bugs fixed in this wave:
//  1. Encounter-scoped bus: SneakAttack.UsedThisTurn persists within a single
//     TakeAction call (intra-verb attacks, e.g. multi-attack). With cross-RPC
//     persistence (saveAttackerConditionState + publishTurnEndAndPersistReset),
//     UsedThisTurn is now enforced across separate TakeAction RPC calls too.
//  2. gamectx wiring: RequireRoom does not crash with ErrNoGameContext;
//     the ally-adjacency check succeeds when an ally is placed next to the target.
//  3. TurnEnd bridge: EndTurn publishes dnd5e TurnEndEvent on the encounter bus
//     so conditions subscribed to TurnEndTopic receive the reset signal, and
//     the reset is persisted back to the character repo.
//
// Scenario:
//   - alice (rogue, level 2, DEX 16 > STR 12) has a shortsword equipped and
//     the SneakAttack condition persisted on her character data.
//   - bob is alice's ally, placed adjacent to goblin-1.
//   - goblin-1 has 100 HP (never dies during the test).
//   - initiative is fixed alice → bob → goblin. Bob's real EndTurn RPC drives
//     the goblin's real NPC turn before alice's next turn; the fixture never
//     advances an NPC directly through the SDK. That gives the fixture exactly
//     one deterministic goblin attack before alice's turn-two assertion.
//   - Alice and bob must both be adjacent to goblin-1 for this test's melee
//     Sneak Attack preconditions, so the NPC's closest-target tie remains a
//     production concern rather than something this fixture tries to change.
//
// # Deterministic roller strategy
//
// A fixedRoller controls the d20 attack roll and weapon damage dice so that
// HP-delta assertions are deterministic for the weapon component:
//
//	fixedRoller{val: 10}: d20 = 10+5 (alice bonus) = 15 > goblin AC 13 → always hit, no crit
//	Weapon 1d6: Roll(size=6) → min(10,6) = 6; damage = 6+3 (DEX mod) = 9
//
// The SneakAttack condition uses its own roller (nil after JSON round-trip →
// dice.NewRoller() → random 1-6). This is fine because:
//   - Attack WITH sneak: HP delta = 9 + (1-6) > 9 (always, since sneak dice ≥ 1)
//   - Attack WITHOUT sneak: HP delta = 9 exactly
//
// rpg-toolkit#653 tracks loader-registration support for synthetic conditions,
// which would allow test-package conditions to inject a fixed roller into the
// sneak dice path. Until then, sneak component assertions use "> 9" rather
// than "== exact_value".
//
// Both HandlerConfig.Roller and CombatResolverConfig.Roller receive this same
// fixed roller. The combat resolver controls Alice's and the stat-snapshot
// goblin's attack/damage rolls; the handler roller keeps encounter-internal
// rolls (such as automatic death saves) deterministic too. The goblin's 10+4
// attack hits Alice (AC 14) or Bob (AC 13) without a crit and deals 6+2=8.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

const (
	sneakIntegEncID  = "enc-sneak-integration"
	sneakPlayerAlice = "alice"
	sneakEntityAlice = "char-alice"
	sneakPlayerBob   = "bob"
	sneakEntityBob   = "char-bob"
	sneakGoblinID    = "goblin-sneak"

	// sneakWeaponDamage is the expected weapon-only damage from alice's shortsword
	// when fixedRoller{val:10} is used: Roll(size=6)=6, damage = 6+DEX_mod(+3) = 9.
	// Used as the baseline for sneak-fired assertions (HP delta must be > 9).
	sneakWeaponDamage = 9

	// sneakNPCDamage is the deterministic scripted goblin hit with the fixed roller:
	// 1d6 rolls 6, plus the fixture's +2 damage bonus.
	sneakNPCDamage = 8
)

// fixedRoller is a deterministic dice.Roller stub for integration tests.
// Roll always returns min(val, size); RollN returns []int{clamped, …} of length count.
// Named "fixed" not "mock" per project convention — hand-written stubs ≠ gomock doubles.
type fixedRoller struct{ val int }

func (r fixedRoller) Roll(_ context.Context, size int) (int, error) {
	v := r.val
	if v < 1 {
		v = 1
	}
	v = min(v, size)
	return v, nil
}

func (r fixedRoller) RollN(_ context.Context, count, size int) ([]int, error) {
	v := r.val
	if v < 1 {
		v = 1
	}
	v = min(v, size)
	result := make([]int, count)
	for i := range result {
		result[i] = v
	}
	return result, nil
}

// inMemoryCharStore is a test helper that backs the mockCharRepo with a
// live in-memory map. Named "Store" not "Repo" per project convention —
// hand-written test helpers ≠ gomock doubles.
//
// Get reads the current state (reflecting any prior Update write-backs).
// Update writes the updated character.Data back so the next Get reflects it.
type inMemoryCharStore struct {
	chars map[string]*character.Data
}

func newInMemoryCharStore() *inMemoryCharStore {
	return &inMemoryCharStore{chars: make(map[string]*character.Data)}
}

// set seeds initial character data (used in SetupTest).
func (s *inMemoryCharStore) set(data *character.Data) {
	s.chars[data.ID] = data
}

// get returns the current character data for the given ID, or nil if not found.
func (s *inMemoryCharStore) get(id string) *character.Data {
	return s.chars[id]
}

// update writes updated character data back to the store.
// Called by the mock's DoAndReturn for Update expectations.
func (s *inMemoryCharStore) update(data *character.Data) {
	if data == nil {
		return
	}
	s.chars[data.ID] = data
}

// SneakAttackIntegrationSuite tests the sign-off scenario for Sneak Attack across
// rogue levels: encounter-scoped bus, gamectx wiring, and TurnEnd bridge.
// rogueLevel controls the character level for alice (1 or 2); both yield 1d6 SA dice.
type SneakAttackIntegrationSuite struct {
	suite.Suite
	rogueLevel   int
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	charStore    *inMemoryCharStore
	broker       *tkenc.Broker
	repo         encountersv2.Repository
	handler      *v2encounter.Handler
	aliceCtx     context.Context
	bobCtx       context.Context
}

func TestSneakAttackIntegrationSuite(t *testing.T) {
	suite.Run(t, &SneakAttackIntegrationSuite{rogueLevel: 2})
}

// TestSneakAttackIntegrationL1Suite proves Sneak Attack works end-to-end for a
// level-1 rogue (Chapter 2 Wave 1 #552). calculateSneakAttackDice(1) = 1d6,
// same as L2, so all assertions are identical — only alice's Level and HP differ.
func TestSneakAttackIntegrationL1Suite(t *testing.T) {
	suite.Run(t, &SneakAttackIntegrationSuite{rogueLevel: 1})
}

func (s *SneakAttackIntegrationSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.charStore = newInMemoryCharStore()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	fixedNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// fixedRoller{val:10}: d20 attack = 10+5=15 > goblin AC 13 → always hit, never crit (not 20).
	// weapon 1d6 = Roll(6) = min(10,6) = 6; with DEX mod +3 → weapon damage = 9.
	h, err := v2encounter.New(&v2encounter.HandlerConfig{
		Broker: s.broker,
		Repo:   s.repo,
		Now:    func() time.Time { return fixedNow },
		CombatResolverConfig: &v2encounter.Dnd5eCombatResolverConfig{
			CharacterRepo: s.mockCharRepo,
			Roller:        fixedRoller{val: 10},
		},
		Roller: fixedRoller{val: 10},
	})
	s.Require().NoError(err)
	s.handler = h

	s.aliceCtx = auth.WithPlayerID(context.Background(), sneakPlayerAlice)
	s.bobCtx = auth.WithPlayerID(context.Background(), sneakPlayerBob)
}

func (s *SneakAttackIntegrationSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestIntegration_SneakAttackDoesNotCrashWithGamectx verifies that the gamectx
// wiring is correct: a rogue character with SneakAttack condition can attack
// without the chain crashing with ErrNoGameContext / ErrNoRoom.
//
// This is the primary sign-off test for Wave 2.11c fix #2 (gamectx wiring).
func (s *SneakAttackIntegrationSuite) TestIntegration_SneakAttackDoesNotCrashWithGamectx() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()
	s.advanceToAlice()

	_, err := s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "attack with SneakAttack condition must not crash from missing gamectx")
}

// TestIntegration_SneakAttackFiresOnTurnAttack verifies that alice's single
// turn attack fires sneak attack (ally adjacent, DEX finesse weapon, gamectx
// wired): HP delta > weapon-only damage.
//
// Note (rpg-api#598): the toolkit now enforces the action economy server-side —
// one action per turn — so a SECOND same-turn attack is rejected
// "insufficient action", not silently sneak-blocked. The pre-#598 version of
// this test fired two attacks in one turn to exercise the UsedThisTurn gate;
// that is no longer reachable for a single-attack rogue. The cross-turn
// UsedThisTurn reset is covered by TestIntegration_SneakAttackResetsOnTurnEnd.
func (s *SneakAttackIntegrationSuite) TestIntegration_SneakAttackFiresOnTurnAttack() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()
	s.advanceToAlice()

	// The turn's one attack: sneak fires (ally adjacent, DEX weapon, UsedThisTurn=false).
	hp0 := s.goblinHP()
	_, err := s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "attack must resolve without error")
	hp1 := s.goblinHP()

	delta1 := hp0 - hp1
	s.Greater(delta1, sneakWeaponDamage,
		"HP delta (%d) must exceed weapon-only damage (%d) — sneak attack must fire "+
			"(ally adjacent, DEX finesse weapon, gamectx wired correctly)",
		delta1, sneakWeaponDamage)

	// A second attack in the same turn is now rejected by the economy (one action
	// per turn) — proving the server-side enforcement (rpg-api#598).
	_, err = s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().Error(err, "second same-turn attack must be rejected — one action per turn")
	st, _ := status.FromError(err)
	s.Equal(codes.FailedPrecondition, st.Code(),
		"second same-turn attack must fail FailedPrecondition (insufficient action), got %v", st.Code())
}

// TestIntegration_SneakAttackResetsOnTurnEnd verifies the once-per-turn
// lifecycle across turn boundaries:
//
//   - Attack 1 (turn 1): sneak fires → HP delta > weapon-only.
//   - EndTurn + advance + Attack 2 (turn 2): sneak fires again → HP delta > weapon-only.
//
// The EndTurn path loads alice with the encounter bus (subscribing SneakAttack
// to the bus), fires TurnEndEvent (UsedThisTurn=false), re-seeds the next actor's
// economy, and saves the updated character back to charStore. A second attack
// WITHIN a turn is no longer possible (one action per turn — rpg-api#598), so the
// cross-turn reset is the lifecycle this test exercises.
func (s *SneakAttackIntegrationSuite) TestIntegration_SneakAttackResetsOnTurnEnd() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()
	s.advanceToAlice()

	// Turn 1 attack: sneak attack fires.
	hp0 := s.goblinHP()
	_, err := s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "turn-1 attack must resolve")
	hp1 := s.goblinHP()

	delta1 := hp0 - hp1
	s.Greater(delta1, sneakWeaponDamage,
		"turn-1 attack: sneak must fire (delta=%d, weapon=%d)", delta1, sneakWeaponDamage)

	// End alice's turn. EndTurn loads alice with the encounter bus, fires
	// TurnEndEvent (SneakAttack.onTurnEnd sets UsedThisTurn=false), and saves back
	// to charStore.
	_, err = s.handler.EndTurn(s.aliceCtx, &encounterv2pb.EndTurnRequest{
		EncounterId: sneakIntegEncID,
		EntityId:    sneakEntityAlice,
	})
	s.Require().NoError(err, "EndTurn must succeed")

	// Cycle through Bob's turn and the production NPC dispatch until alice is
	// active again on turn 2. Both valid equal-distance target choices must leave
	// Alice conscious, and exactly one deterministic NPC attack must occur.
	partyHPBeforeNPC := s.partyHP()
	s.advanceToAlice()
	s.Greater(s.aliceEncounterHP(), 0,
		"fixture must keep alice conscious before her turn-2 Sneak Attack assertion")
	s.Equal(sneakNPCDamage, partyHPBeforeNPC-s.partyHP(),
		"fixture progression must dispatch exactly one deterministic goblin attack")

	// Turn 2 attack: UsedThisTurn=false was persisted by EndTurn write-back, and
	// the new turn re-seeds the economy → sneak fires again.
	_, err = s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "turn-2 attack must resolve")
	hp2 := s.goblinHP()

	delta2 := hp1 - hp2
	s.Greater(delta2, sneakWeaponDamage,
		"turn-2 attack: sneak must fire again after EndTurn reset (delta=%d, weapon=%d)",
		delta2, sneakWeaponDamage)
}

// TestIntegration_GamectxWiredForChainConditions verifies that gamectx.RequireRoom
// is callable from within the attack chain without crashing.
func (s *SneakAttackIntegrationSuite) TestIntegration_GamectxWiredForChainConditions() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()
	s.advanceToAlice()

	_, err := s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err,
		"attack chain must not return ErrNoGameContext / ErrNoRoom: gamectx.WithRoom must be wired")
}

// TestIntegration_ReactionReadinessExposedViaGamectx verifies that the
// encounter's ReactionReadiness map is correctly threaded into gamectx.
//
// Note: a stronger test would inject a synthetic condition that calls
// gamectx.IsReactionReady from inside its Apply handler and records the
// observed value. This is not currently possible because conditions.LoadJSON
// is a closed switch — only toolkit-registered condition refs can be
// reconstituted from character JSON blobs. rpg-toolkit#653 tracks adding a
// loader-registration extension point so test-package conditions can ride the
// snapshot lifecycle. Until then, this test verifies the data-shape (readiness
// map is seeded by the encounter SDK) plus the no-crash signal (TakeAction
// succeeds with the readiness map wired into gamectx via WithReactionReadiness).
func (s *SneakAttackIntegrationSuite) TestIntegration_ReactionReadinessExposedViaGamectx() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()

	// Verify that the encounter seeded OA readiness for alice (the encounter
	// SDK's AddPlayer seeds OA=true for combatants with DamageDice set).
	data, err := s.repo.Get(context.Background(), sneakIntegEncID)
	s.Require().NoError(err)
	aliceReadiness := data.ReactionReadiness[encountercore.EntityID(sneakEntityAlice)]
	s.Require().NotNil(aliceReadiness, "encounter must seed reaction readiness for alice")
	s.True(aliceReadiness[tkenc.OAReactionRef],
		"OA reaction must be ready-by-default for melee combatant alice")

	// Attack resolves without error — the readiness map flowed into gamectx.
	s.advanceToAlice()
	_, err = s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "attack must resolve with readiness map wired into gamectx")
}

// TestIntegration_SneakAttackDamageBreakdown_HasWeaponAndSneakComponents verifies
// that when alice (rogue) fires a sneak attack, the DamageDealtEvent published on
// the broker carries two Components: a "dnd5e:weapons:shortsword" component and a
// "dnd5e:features:sneak_attack" component. This proves the breakdown survives
// the full chain: combat resolver → AttackOutcome.Components → DamageDealtEvent.Components
// → broker JSON round-trip.
func (s *SneakAttackIntegrationSuite) TestIntegration_SneakAttackDamageBreakdown_HasWeaponAndSneakComponents() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()
	s.advanceToAlice()

	// Subscribe for alice before the attack so we capture the DamageDealtEvent.
	sub, err := s.broker.Subscribe(encountercore.EncounterID(sneakIntegEncID), encountercore.PlayerID(sneakPlayerAlice))
	s.Require().NoError(err, "broker subscribe must succeed")
	defer sub.Close() //nolint:errcheck

	_, err = s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "attack must resolve without error")

	// Drain events until we find the DamageDealtEvent (or timeout).
	var damageEvt *tkencevents.DamageDealtEvent
	timeout := time.After(2 * time.Second)
drain:
	for {
		select {
		case evt, ok := <-sub.Events():
			if !ok {
				break drain
			}
			if d, ok := evt.(*tkencevents.DamageDealtEvent); ok {
				damageEvt = d
				break drain
			}
		case <-timeout:
			break drain
		}
	}

	s.Require().NotNil(damageEvt, "DamageDealtEvent must be published on the broker for alice's attack")
	s.Require().NotEmpty(damageEvt.Components,
		"DamageDealtEvent.Components must be non-empty when sneak attack fires")

	// Verify component sources. The dnd5e rulebook uses SourceRef.String() which
	// produces "dnd5e:weapons:<id>" for the weapon, "dnd5e:abilities:<id>" for the
	// ability modifier, and "dnd5e:features:sneak_attack" for the sneak attack dice.
	sources := make([]string, 0, len(damageEvt.Components))
	for _, c := range damageEvt.Components {
		sources = append(sources, c.Source)
	}
	s.Contains(sources, "dnd5e:weapons:shortsword",
		"damage breakdown must include a weapon component; got %v", sources)
	s.Contains(sources, "dnd5e:features:sneak_attack",
		"damage breakdown must include a sneak attack feature component; got %v", sources)

	// The weapon component carries only dice (6 from fixedRoller). The DEX modifier
	// is emitted as a separate "dnd5e:abilities:dex" component (amount=3).
	const weaponDiceOnly = 6 // fixedRoller{10}.Roll(6) = min(10,6) = 6
	for _, c := range damageEvt.Components {
		if c.Source == "dnd5e:weapons:shortsword" {
			s.Equal(weaponDiceOnly, c.Amount,
				"weapon component amount must equal %d (fixed dice roll)", weaponDiceOnly)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupCharRepoMock registers mock expectations backed by inMemoryCharStore:
//
//   - Get for alice uses DoAndReturn so it always reads the current charStore
//     state (reflecting prior Update write-backs from saveAttackerConditionState
//     and publishTurnEndAndPersistReset).
//   - Update uses DoAndReturn to write changes back to charStore (making the
//     next Get reflect the updated condition state).
//   - Non-alice Get lookups return an error so the resolver falls back to
//     StandInCombatResolver (NPC dispatch loop).
func (s *SneakAttackIntegrationSuite) setupCharRepoMock(aliceData *character.Data) {
	s.charStore.set(aliceData)

	// Alice Get: reads live charStore so each RPC sees the latest write-backs.
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: sneakEntityAlice}).
		DoAndReturn(func(_ context.Context, _ characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			d := s.charStore.get(sneakEntityAlice)
			if d == nil {
				return nil, apierr.NotFound("alice not found in charStore")
			}
			return &characterrepo.GetOutput{
				Character: &entities.Character{Data: d},
			}, nil
		}).AnyTimes()

	// Alice Update: writes updated data back to charStore.
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			if input.Character != nil && input.Character.Data != nil {
				s.charStore.update(input.Character.Data)
			}
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		}).AnyTimes()

	// Non-alice lookups fall back to stand-in: the real character repo returns
	// apierr.NotFound for an unknown id, which the seeding/attach paths treat as
	// "no stored character → stand-in seat" (NOT a fatal error). Using a generic
	// error here would mismatch the real contract and trip the NotFound-vs-real-
	// error distinction the seeding code relies on (Copilot review on #599).
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), gomock.Not(characterrepo.GetInput{ID: sneakEntityAlice})).
		Return(nil, apierr.NotFound("character not configured in integration test mock")).
		AnyTimes()
}

// attackAliceVsGoblin returns a fresh TakeActionRequest for alice attacking goblin.
func (s *SneakAttackIntegrationSuite) attackAliceVsGoblin() *encounterv2pb.TakeActionRequest {
	return &encounterv2pb.TakeActionRequest{
		EncounterId:   sneakIntegEncID,
		ActorEntityId: sneakEntityAlice,
		ActionRef:     &encounterv2pb.Ref{Module: "dnd5e", Type: "action", Id: "attack"},
		Target: &encounterv2pb.ActionTarget{
			Kind: &encounterv2pb.ActionTarget_EntityId{EntityId: sneakGoblinID},
		},
	}
}

// goblinHP returns the current HP of the goblin from the persisted encounter snapshot.
func (s *SneakAttackIntegrationSuite) goblinHP() int {
	data, err := s.repo.Get(context.Background(), sneakIntegEncID)
	s.Require().NoError(err, "repo.Get for goblin HP check")
	md, ok := data.Monsters[encountercore.EntityID(sneakGoblinID)]
	s.Require().True(ok, "goblin must be in encounter data")
	return md.HP
}

// aliceEncounterHP returns Alice's current HP from the persisted encounter snapshot.
func (s *SneakAttackIntegrationSuite) aliceEncounterHP() int {
	data, err := s.repo.Get(context.Background(), sneakIntegEncID)
	s.Require().NoError(err, "repo.Get for alice HP check")
	alice, ok := data.Players[encountercore.PlayerID(sneakPlayerAlice)]
	s.Require().True(ok, "alice must be in encounter data")
	return alice.HP
}

// partyHP returns Alice and Bob's total current HP from the persisted encounter
// snapshot, rather than their separate character-store records.
func (s *SneakAttackIntegrationSuite) partyHP() int {
	data, err := s.repo.Get(context.Background(), sneakIntegEncID)
	s.Require().NoError(err, "repo.Get for party HP check")
	alice, aliceOK := data.Players[encountercore.PlayerID(sneakPlayerAlice)]
	bob, bobOK := data.Players[encountercore.PlayerID(sneakPlayerBob)]
	s.Require().True(aliceOK, "alice must be in encounter data")
	s.Require().True(bobOK, "bob must be in encounter data")
	return alice.HP + bob.HP
}

// aliceHP returns hardcoded HP for alice based on s.rogueLevel.
// Values are test-fixture numbers, not strict 5e math.
func (s *SneakAttackIntegrationSuite) aliceHP() int {
	if s.rogueLevel == 1 {
		return 10
	}
	return 16
}

// buildAliceRogueData constructs a character.Data for alice — a rogue with
// DEX 16 > STR 12 (so finesse weapons use DEX), a shortsword in the main
// hand slot, and the SneakAttack condition persisted in her Conditions blob.
// Level and HP are driven by s.rogueLevel so L1 and L2 share this builder.
// calculateSneakAttackDice(level) = (level+1)/2: L1 → 1d6, L2 → 1d6.
func (s *SneakAttackIntegrationSuite) buildAliceRogueData() *character.Data {
	sneakCond := conditions.NewSneakAttackCondition(conditions.SneakAttackInput{
		CharacterID: sneakEntityAlice,
		Level:       s.rogueLevel,
	})
	sneakJSON, err := sneakCond.ToJSON()
	s.Require().NoError(err, "marshal SneakAttack condition")

	hp := s.aliceHP()

	return &character.Data{
		ID:               sneakEntityAlice,
		Name:             "Alice the Rogue",
		Level:            s.rogueLevel,
		ClassID:          "rogue",
		ProficiencyBonus: 2,
		HitPoints:        hp,
		MaxHitPoints:     hp,
		ArmorClass:       14,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 12, // +1 modifier
			abilities.DEX: 16, // +3 modifier — higher than STR, so finesse uses DEX
			abilities.CON: 14,
			abilities.INT: 12,
			abilities.WIS: 10,
			abilities.CHA: 10,
		},
		// Shortsword is a finesse weapon → combat.ResolveAttack will use DEX
		// ability (DEX mod 3 > STR mod 1). The SneakAttack condition checks
		// event.AbilityUsed == "dex" to determine eligibility.
		EquipmentSlots: character.EquipmentSlots{
			character.SlotMainHand: "shortsword",
		},
		Inventory: []character.InventoryItemData{
			{
				Type:     "weapon",
				ID:       "shortsword",
				Quantity: 1,
			},
		},
		Conditions: []json.RawMessage{sneakJSON},
	}
}

// seedSneakEncounter creates the integration test fixture encounter and saves it.
//
// Positions (cube hex coordinates, hexDistance = (|dQ|+|dR|+|dS|)/2 — the
// toolkit's real metric, not the euclidean approximation this comment used
// to claim): goblin sits at a common neighbor of both alice and bob, so
// BOTH are within 1 hex of it — alice needs that for her own attack
// (rpg-toolkit#864's range/reach gate: TakeAction now requires the
// attacker within weapon reach, 1 hex by default for her un-hydrated-reach
// shortsword), and bob still needs it for Sneak Attack's ally-adjacent-to-
// target rule:
//
//	alice  Q:0  R:0  S:0   (attacker)
//	bob    Q:1  R:0  S:-1  (ally — hexDistance to goblin = 1)
//	goblin Q:1  R:-1 S:0   (target, 100 HP so it survives multiple attacks;
//	                        hexDistance to alice = 1, to bob = 1)
//
// DamageDice is set for all combatants so they qualify as combatants for
// TakeAction and have OA readiness seeded by the encounter SDK.
func (s *SneakAttackIntegrationSuite) seedSneakEncounter() {
	enc := tkenc.New(context.Background(), sneakIntegEncID, s.broker)

	aliceHP := s.aliceHP()
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    encountercore.PlayerID(sneakPlayerAlice),
		EntityID:    encountercore.EntityID(sneakEntityAlice),
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          aliceHP,
		MaxHP:       aliceHP,
		AC:          14,
		AttackBonus: 5, // DEX +3 + proficiency +2
		DamageDice:  "1d6+3",
		DamageType:  "piercing",
	}))

	// bob: ally placed adjacent to goblin (hexDistance = 1) for Sneak
	// Attack's ally-adjacent-to-target rule.
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   encountercore.PlayerID(sneakPlayerBob),
		EntityID:   encountercore.EntityID(sneakEntityBob),
		Position:   encountercore.Hex{Q: 1, R: 0, S: -1},
		SightRange: 10,
		HP:         12,
		MaxHP:      12,
		AC:         13,
		DamageDice: "1d8+2",
		DamageType: "slashing",
	}))

	// goblin: 100 HP so it survives multiple attacks during the test. A
	// common neighbor of alice and bob (see seedSneakEncounter's doc
	// comment) so alice's own attack is within reach too
	// (rpg-toolkit#864). No DataJSON is supplied: the toolkit's scripted NPC
	// path therefore uses the deterministic stat-snapshot resolver regardless
	// of which equal-distance player map iteration selects.
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID:          encountercore.EntityID(sneakGoblinID),
		Position:    encountercore.Hex{Q: 1, R: -1, S: 0},
		HP:          100,
		MaxHP:       100,
		AC:          13,
		Speed:       6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
	}))

	// AddMonster inline-checks combat entry (rpg-toolkit#759): alice (sight
	// range 10, no room) already sees the goblin, so the encounter
	// self-transitions to TURN_BASED here. An explicit SetMode would now be
	// redundant and error ("mode is already TURN_BASED").
	//
	// The test fixture fixes initiative after that production transition. This
	// makes the reset scenario alice → bob → goblin → alice:
	// bob's handler EndTurn owns the goblin dispatch and its subsequent
	// turn-start work, instead of the test bypassing it with a direct SDK call.
	data := enc.ToData()
	data.Initiative = []encountercore.EntityID{
		sneakEntityAlice,
		sneakEntityBob,
		sneakGoblinID,
	}
	data.ActiveIdx = 0
	data.Round = 1
	s.Require().NoError(s.repo.Save(context.Background(), data))
}

// advanceToAlice moves through only player-owned turns until Alice is active.
// The fixture fixes initiative as alice → bob → goblin, so the only progression
// this helper needs is Bob's real EndTurn RPC. That RPC drives the adjacent
// goblin through the production NPC chain and starts Alice's next turn; the
// test must never bypass that behavior with a direct SDK EndTurn call.
func (s *SneakAttackIntegrationSuite) advanceToAlice() {
	data, err := s.repo.Get(context.Background(), sneakIntegEncID)
	s.Require().NoError(err)
	s.Require().NotEmpty(data.Initiative, "fixture initiative must be configured")
	s.Require().GreaterOrEqual(data.ActiveIdx, 0, "fixture ActiveIdx must be non-negative")
	s.Require().Less(data.ActiveIdx, len(data.Initiative), "fixture ActiveIdx must be in range")
	if data.Initiative[data.ActiveIdx] == sneakEntityAlice {
		return
	}
	s.Require().Equal(encountercore.EntityID(sneakEntityBob), data.Initiative[data.ActiveIdx],
		"fixture must route the goblin through bob's production EndTurn RPC")

	_, err = s.handler.EndTurn(s.bobCtx, &encounterv2pb.EndTurnRequest{
		EncounterId: sneakIntegEncID,
		EntityId:    sneakEntityBob,
	})
	s.Require().NoError(err, "EndTurn for bob must drive the NPC chain")

	data, err = s.repo.Get(context.Background(), sneakIntegEncID)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(data.ActiveIdx, 0, "fixture ActiveIdx must be non-negative")
	s.Require().Less(data.ActiveIdx, len(data.Initiative), "fixture ActiveIdx must be in range")
	s.Require().Equal(encountercore.EntityID(sneakEntityAlice), data.Initiative[data.ActiveIdx],
		"Bob's EndTurn must advance through the goblin to alice")
}

// TestIntegration_TurnStartSnapshot_MenuIsPrivateToActiveActor proves the #601
// audience gate (Copilot #602): the turn-start snapshot projects the active
// actor's economy + menu ONLY into the controlling player's snapshot, never into
// another player's view. During alice's turn, alice sees her menu; bob (a
// non-controlling viewer) gets the menu-less initiative-only TurnState, so one
// player's action options/economy never leak to another (Inv 6 audience-seam,
// matching the live TurnStateChanged push).
func (s *SneakAttackIntegrationSuite) TestIntegration_TurnStartSnapshot_MenuIsPrivateToActiveActor() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()
	s.advanceToAlice()

	// Alice (the active actor's controller) sees her own menu + economy.
	aliceResp, err := s.handler.GetEncounter(s.aliceCtx, &encounterv2pb.GetEncounterRequest{
		EncounterId: sneakIntegEncID,
	})
	s.Require().NoError(err)
	aliceTS := aliceResp.GetEncounter().GetTurnState()
	s.Require().NotNil(aliceTS)
	s.Require().Equal(sneakEntityAlice, aliceTS.GetActiveEntityId())
	s.Require().NotNil(aliceTS.GetEconomy(), "alice (active actor) must see her own economy")
	s.Require().NotEmpty(aliceTS.GetAvailableActions(), "alice must see her own menu")

	// Bob (a non-controlling viewer) gets the same turn structure (initiative /
	// active / round) but NOT alice's private menu/economy.
	bobResp, err := s.handler.GetEncounter(s.bobCtx, &encounterv2pb.GetEncounterRequest{
		EncounterId: sneakIntegEncID,
	})
	s.Require().NoError(err)
	bobTS := bobResp.GetEncounter().GetTurnState()
	s.Require().NotNil(bobTS, "bob still sees the turn structure")
	s.Require().Equal(sneakEntityAlice, bobTS.GetActiveEntityId(), "bob sees whose turn it is")
	s.Require().NotEmpty(bobTS.GetInitiativeOrder(), "bob sees the initiative roster")
	s.Require().Nil(bobTS.GetEconomy(), "bob must NOT see alice's economy (private to active actor)")
	s.Require().Empty(bobTS.GetAvailableActions(), "bob must NOT see alice's menu (private to active actor)")
}
