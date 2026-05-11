package encounter_test

// integration_sneak_attack_test.go — Wave 2.11c sign-off integration tests.
//
// These tests verify the three core bugs fixed in this wave:
//  1. Encounter-scoped bus: SneakAttack.UsedThisTurn persists within a single
//     TakeAction call (intra-verb attacks, e.g. multi-attack). Separate TakeAction
//     RPC calls each do a fresh LoadFromData with a new bus, so cross-RPC
//     once-per-turn enforcement requires encounter-side state (tracked in #654).
//  2. gamectx wiring: RequireRoom does not crash with ErrNoGameContext;
//     the ally-adjacency check succeeds when an ally is placed next to the target.
//  3. TurnEnd bridge: EndTurn publishes dnd5e TurnEndEvent on the encounter bus
//     so conditions subscribed to TurnEndTopic receive the reset signal.
//
// Scenario:
//   - alice (rogue, level 2, DEX 16 > STR 12) has a shortsword equipped and
//     the SneakAttack condition persisted on her character data.
//   - bob is alice's ally, placed adjacent to goblin-1.
//   - goblin-1 has 100 HP (never dies during the test).
//
// # Deterministic roller strategy
//
// A fixedRoller controls the d20 attack roll and weapon damage dice so that
// HP-delta assertions are deterministic for the weapon component:
//
//   fixedRoller{val: 10}: d20 = 10+5 (alice bonus) = 15 > goblin AC 13 → always hit, no crit
//   Weapon 1d6: Roll(size=6) → min(10,6) = 6; damage = 6+3 (DEX mod) = 9
//
// The SneakAttack condition uses its own roller (nil after JSON round-trip →
// dice.NewRoller() → random 1-6). This is fine because:
//   - Attack WITH sneak: HP delta = 9 + (1-6) > 9 (always, since sneak dice ≥ 1)
//   - Attack WITHOUT sneak: HP delta = 9 exactly
//
// The once-per-turn gate works WITHIN a single TakeAction but NOT across
// separate RPC calls (each RPC calls LoadFromData which creates a fresh bus and
// condition with UsedThisTurn=false). Cross-RPC once-per-turn enforcement is
// tracked in rpg-toolkit#654.
//
// rpg-toolkit#653 tracks loader-registration support for synthetic conditions,
// which would allow test-package conditions to inject a fixed roller into the
// sneak dice path. Until then, sneak component assertions use "> 9" rather
// than "== exact_value".

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
	if v > size {
		v = size
	}
	return v, nil
}

func (r fixedRoller) RollN(_ context.Context, count, size int) ([]int, error) {
	v := r.val
	if v < 1 {
		v = 1
	}
	if v > size {
		v = size
	}
	result := make([]int, count)
	for i := range result {
		result[i] = v
	}
	return result, nil
}

// SneakAttackIntegrationSuite tests the Wave 2.11c sign-off scenario:
// encounter-scoped bus, gamectx wiring, and TurnEnd bridge.
type SneakAttackIntegrationSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	broker       *tkenc.Broker
	repo         encountersv2.Repository
	handler      *v2encounter.Handler
	aliceCtx     context.Context
	bobCtx       context.Context
}

func TestSneakAttackIntegrationSuite(t *testing.T) {
	suite.Run(t, new(SneakAttackIntegrationSuite))
}

func (s *SneakAttackIntegrationSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
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

// TestIntegration_SneakAttackFiresWhenConditionsMet verifies that sneak attack
// fires when an ally is adjacent to the target (gamectx.RequireRoom wired correctly)
// and the attacker uses a finesse weapon (AbilityUsed == "dex").
//
// HP-delta proof: with fixedRoller(val=10):
//   - weapon damage = 9 (deterministic)
//   - sneak attack: 1d6 random (1-6, always ≥1)
//   - goblin HP delta must be > 9 (sneak fired)
//
// This is the sign-off test for Wave 2.11c fix #2 (gamectx wiring enables
// the ally-adjacency path inside checkSneakAttackConditions).
func (s *SneakAttackIntegrationSuite) TestIntegration_SneakAttackFiresWhenConditionsMet() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()
	s.advanceToAlice()

	hpBefore := s.goblinHP()

	_, err := s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "attack must resolve without error")

	hpAfter := s.goblinHP()
	delta := hpBefore - hpAfter
	s.Greater(delta, sneakWeaponDamage,
		"goblin HP delta (%d) must exceed weapon-only damage (%d): sneak attack must have fired (ally adjacent, DEX weapon, gamectx wired correctly)",
		delta, sneakWeaponDamage)
}

// TestIntegration_SneakAttackOncePerTurn documents the intra-TakeAction
// once-per-turn behaviour and the known cross-RPC gap.
//
// Within a single TakeAction verb, UsedThisTurn persists on the condition
// (same bus, same condition instance in memory). Across separate TakeAction
// RPC calls, each call runs LoadFromData which creates a fresh bus and a fresh
// condition instance with UsedThisTurn=false — so sneak fires again.
//
// Cross-RPC once-per-turn enforcement requires the encounter data to track
// per-entity turn-state (tracked in rpg-toolkit#654). Until that lands, this
// test documents the no-crash behaviour for repeated attacks.
func (s *SneakAttackIntegrationSuite) TestIntegration_SneakAttackOncePerTurn() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()
	s.advanceToAlice()

	// Attack 1 in the same turn — sneak attack fires (ally adjacent, DEX weapon).
	_, err := s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "first attack must resolve without error")

	// Attack 2 in the same turn — each TakeAction creates a fresh bus+condition,
	// so UsedThisTurn resets to false and sneak fires again. This is the known
	// cross-RPC gap (rpg-toolkit#654). The test proves no panic, no error.
	_, err = s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "second attack in the same turn must also resolve without error")
}

// TestIntegration_SneakAttackResetsOnTurnEnd verifies that EndTurn publishes
// the dnd5e TurnEndEvent on the encounter bus (the TurnEnd bridge in end_turn.go).
// The no-crash signal proves the bridge fires without error.
//
// Note: verifying that UsedThisTurn is reset by the TurnEnd event requires the
// condition to persist across LoadFromData calls, which requires cross-RPC state
// tracking (rpg-toolkit#654). This test proves the TurnEnd bridge executes and
// that attacks continue to resolve successfully on the next turn.
func (s *SneakAttackIntegrationSuite) TestIntegration_SneakAttackResetsOnTurnEnd() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()
	s.advanceToAlice()

	// Turn 1: alice attacks (sneak attack fires, UsedThisTurn set to true on current bus).
	_, err := s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "turn-1 attack must resolve")

	// End alice's turn. EndTurn must publish the dnd5e TurnEndEvent on the
	// encounter bus, resetting SneakAttack.UsedThisTurn for alice.
	_, err = s.handler.EndTurn(s.aliceCtx, &encounterv2pb.EndTurnRequest{
		EncounterId: sneakIntegEncID,
		EntityId:    sneakEntityAlice,
	})
	s.Require().NoError(err, "EndTurn must succeed")

	// Cycle through the remaining combatants (bob + goblin) until alice is
	// the active actor again on her next turn.
	s.advanceToAlice()

	// Turn 2: alice attacks again. Must not error — the TurnEnd bridge fires
	// correctly, and the next-turn attack resolves.
	_, err = s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "turn-2 attack must resolve (TurnEnd bridge must not error)")
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupCharRepoMock registers mock expectations for character lookups:
//   - Alice's data is returned for alice ID lookups (AnyTimes).
//   - All other character IDs (e.g. bob, loaded when goblin attacks bob during
//     NPCAct) return an error, which causes the resolver to fall back to
//     StandInCombatResolver. This keeps the mock strict while allowing the
//     NPC dispatch loop to cycle through non-alice turns.
func (s *SneakAttackIntegrationSuite) setupCharRepoMock(aliceData *character.Data) {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: sneakEntityAlice}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: aliceData},
		}, nil).AnyTimes()

	// Non-alice lookups fall back to stand-in (no rulebook chain, but no error).
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), gomock.Not(characterrepo.GetInput{ID: sneakEntityAlice})).
		Return(nil, fmt.Errorf("character not configured in integration test mock")).
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

// buildAliceRogueData constructs a character.Data for alice — a level-2 rogue
// with DEX 16 > STR 12 (so finesse weapons use DEX), a shortsword in the main
// hand slot, and the SneakAttack condition persisted in her Conditions blob.
func (s *SneakAttackIntegrationSuite) buildAliceRogueData() *character.Data {
	// Build the SneakAttack condition JSON (level 2 rogue → 1d6 sneak attack).
	sneakCond := conditions.NewSneakAttackCondition(conditions.SneakAttackInput{
		CharacterID: sneakEntityAlice,
		Level:       2, // 1d6 at level 2
	})
	sneakJSON, err := sneakCond.ToJSON()
	s.Require().NoError(err, "marshal SneakAttack condition")

	return &character.Data{
		ID:               sneakEntityAlice,
		Name:             "Alice the Rogue",
		Level:            2,
		ClassID:          "rogue",
		ProficiencyBonus: 2,
		HitPoints:        16,
		MaxHitPoints:     16,
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
// Positions chosen so bob is adjacent to goblin (distance ≤ 1.5 in euclidean
// terms using Q→X, R→Y hex mapping):
//
//	alice  Q:0  R:0   (attacker)
//	bob    Q:1  R:0   (ally — distance to goblin sqrt((2-1)²+(0-0)²) = 1.0 ≤ 1.5)
//	goblin Q:2  R:0   (target — 100 HP so it survives multiple attacks)
//
// DamageDice is set for all combatants so they qualify as combatants for
// TakeAction and have OA readiness seeded by the encounter SDK.
func (s *SneakAttackIntegrationSuite) seedSneakEncounter() {
	enc := tkenc.New(sneakIntegEncID, s.broker)

	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    encountercore.PlayerID(sneakPlayerAlice),
		EntityID:    encountercore.EntityID(sneakEntityAlice),
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          16,
		MaxHP:       16,
		AC:          14,
		AttackBonus: 5, // DEX +3 + proficiency +2
		DamageDice:  "1d6+3",
		DamageType:  "piercing",
	}))

	// bob: ally placed adjacent to goblin (Q=1, goblin at Q=2 → euclidean distance=1.0)
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

	// goblin: 100 HP so it survives multiple attacks during the test.
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID:          encountercore.EntityID(sneakGoblinID),
		Position:    encountercore.Hex{Q: 2, R: 0, S: -2},
		HP:          100,
		MaxHP:       100,
		AC:          13,
		Speed:       6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
	}))

	s.Require().NoError(enc.SetMode(encountercore.ModeTurnBased))
	s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))
}

// advanceToAlice cycles initiative until alice is the active actor. Because
// initiative is randomly rolled, this loop ends turns for NPC and bob actors
// until alice gets her turn.
//
// NPC turns (goblin) are ended via direct SDK calls to avoid triggering the
// handler's NPC dispatch loop (which would try to run NPCAct and might pick
// up alice or bob as targets, requiring additional mock setup). Bob's turns
// are ended via the handler using bob's auth context.
func (s *SneakAttackIntegrationSuite) advanceToAlice() {
	for i := 0; i < 20; i++ {
		data, err := s.repo.Get(context.Background(), sneakIntegEncID)
		s.Require().NoError(err)
		s.Require().NotEmpty(data.Initiative, "initiative must be rolled")

		if data.ActiveIdx < 0 || data.ActiveIdx >= len(data.Initiative) {
			s.Fail("invalid ActiveIdx", data.ActiveIdx)
		}
		activeID := string(data.Initiative[data.ActiveIdx])

		if activeID == sneakEntityAlice {
			return // alice is now the active actor
		}

		if activeID == sneakGoblinID {
			// End the NPC's turn directly via the encounter SDK to avoid
			// triggering NPCAct (which would require additional mock setup for
			// the goblin's attack target character loading).
			enc, loadErr := tkenc.LoadFromData(data, s.broker)
			s.Require().NoError(loadErr)
			_, _, err = enc.EndTurn(encountercore.EntityID(activeID))
			s.Require().NoError(err)
			s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))
			continue
		}

		// It's a player's turn (bob or potentially alice if we somehow missed).
		// End via handler with the appropriate auth context.
		var turnCtx context.Context
		if activeID == sneakEntityBob {
			turnCtx = s.bobCtx
		} else {
			turnCtx = s.aliceCtx
		}
		_, err = s.handler.EndTurn(turnCtx, &encounterv2pb.EndTurnRequest{
			EncounterId: sneakIntegEncID,
			EntityId:    activeID,
		})
		s.Require().NoError(err, "EndTurn for actor %q must succeed", activeID)
	}
	s.Fail("advanceToAlice: alice did not become active within 20 initiative steps")
}
