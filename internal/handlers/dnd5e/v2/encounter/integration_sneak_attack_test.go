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

// TestIntegration_SneakAttackOncePerTurn verifies the cross-RPC once-per-turn
// enforcement via the inMemoryCharStore write-back:
//
//   - Attack 1: sneak fires (ally adjacent, DEX weapon) → HP delta > 9 (weapon 9 + sneak ≥ 1).
//   - Attack 2 (same turn, separate TakeAction RPC): UsedThisTurn=true persisted to
//     charStore by saveAttackerConditionState; next LoadFromData restores true →
//     sneak blocked → HP delta == 9 exactly (weapon-only).
//
// This proves cross-RPC once-per-turn enforcement works end-to-end with the
// toolkit PR #655 JSON persistence + saveAttackerConditionState write-back.
func (s *SneakAttackIntegrationSuite) TestIntegration_SneakAttackOncePerTurn() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()
	s.advanceToAlice()

	// Attack 1: sneak attack fires (ally adjacent, DEX weapon, UsedThisTurn=false).
	hp0 := s.goblinHP()
	_, err := s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "first attack must resolve without error")
	hp1 := s.goblinHP()

	delta1 := hp0 - hp1
	s.Greater(delta1, sneakWeaponDamage,
		"attack 1: HP delta (%d) must exceed weapon-only damage (%d) — sneak attack must fire "+
			"(ally adjacent, DEX finesse weapon, gamectx wired correctly)",
		delta1, sneakWeaponDamage)

	// Attack 2: sneak must be blocked by UsedThisTurn=true persisted across RPCs.
	// saveAttackerConditionState wrote UsedThisTurn=true to charStore after attack 1;
	// the next LoadFromData restores it from JSON (rpg-toolkit PR #655) → gate closed.
	_, err = s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "second attack must resolve without error")
	hp2 := s.goblinHP()

	delta2 := hp1 - hp2
	s.Equal(sneakWeaponDamage, delta2,
		"attack 2: HP delta (%d) must equal weapon-only damage (%d) — sneak must be blocked "+
			"(UsedThisTurn=true persisted to charStore across RPCs)",
		delta2, sneakWeaponDamage)
}

// TestIntegration_SneakAttackResetsOnTurnEnd verifies the full once-per-turn
// lifecycle across turn boundaries:
//
//   - Attack 1 (turn 1): sneak fires → HP delta > 9.
//   - Attack 2 (turn 1, same turn, separate RPC): sneak blocked (UsedThisTurn=true) → delta == 9.
//   - EndTurn + advance + Attack 3 (turn 2): sneak fires again → delta > 9.
//
// The EndTurn path: publishTurnEndAndPersistReset loads alice with the encounter
// bus (subscribing SneakAttack to the bus), fires TurnEndEvent (UsedThisTurn=false),
// and saves the updated character back to charStore.
func (s *SneakAttackIntegrationSuite) TestIntegration_SneakAttackResetsOnTurnEnd() {
	aliceData := s.buildAliceRogueData()
	s.setupCharRepoMock(aliceData)
	s.seedSneakEncounter()
	s.advanceToAlice()

	// Turn 1, Attack 1: sneak attack fires.
	hp0 := s.goblinHP()
	_, err := s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "turn-1 attack-1 must resolve")
	hp1 := s.goblinHP()

	delta1 := hp0 - hp1
	s.Greater(delta1, sneakWeaponDamage,
		"turn-1 attack-1: sneak must fire (delta=%d, weapon=%d)", delta1, sneakWeaponDamage)

	// Turn 1, Attack 2: sneak blocked (cross-RPC UsedThisTurn=true).
	_, err = s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "turn-1 attack-2 must resolve")
	hp2 := s.goblinHP()

	delta2 := hp1 - hp2
	s.Equal(sneakWeaponDamage, delta2,
		"turn-1 attack-2: sneak must be blocked (delta=%d, weapon=%d)", delta2, sneakWeaponDamage)

	// End alice's turn. publishTurnEndAndPersistReset loads alice with the
	// encounter bus, fires TurnEndEvent (SneakAttack.onTurnEnd sets UsedThisTurn=false),
	// and saves back to charStore.
	_, err = s.handler.EndTurn(s.aliceCtx, &encounterv2pb.EndTurnRequest{
		EncounterId: sneakIntegEncID,
		EntityId:    sneakEntityAlice,
	})
	s.Require().NoError(err, "EndTurn must succeed")

	// Cycle through remaining combatants until alice is active again on turn 2.
	s.advanceToAlice()

	// Turn 2, Attack 3: UsedThisTurn=false was persisted by EndTurn write-back;
	// next LoadFromData restores false → sneak fires again.
	_, err = s.handler.TakeAction(s.aliceCtx, s.attackAliceVsGoblin())
	s.Require().NoError(err, "turn-2 attack must resolve")
	hp3 := s.goblinHP()

	delta3 := hp2 - hp3
	s.Greater(delta3, sneakWeaponDamage,
		"turn-2 attack: sneak must fire again after EndTurn reset (delta=%d, weapon=%d)",
		delta3, sneakWeaponDamage)
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
				return nil, fmt.Errorf("alice not found in charStore")
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
	for range 20 {
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
			enc, loadErr := tkenc.LoadFromData(context.Background(), data, s.broker)
			s.Require().NoError(loadErr)
			_, _, err = enc.EndTurn(context.Background(), encountercore.EntityID(activeID))
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
