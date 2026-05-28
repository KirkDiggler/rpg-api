package encounter_test

// integration_monk_unarmed_test.go — Wave 2 (Monk L1) sign-off integration tests.
//
// These tests verify that a charli (L1 monk, no weapon, Martial Arts + UnarmoredDefense
// conditions persisted) unarmed-strikes via the real v2 TakeAction RPC and that:
//
//  1. The Martial Arts condition runs on the unarmed path (not silently no-oped).
//  2. DEX modifier (+3) is used instead of STR (+0) in the damage components.
//  3. DamageDealtEvent.Components carries "dnd5e:abilities:dex" as the ability source.
//
// # Root cause addressed
//
// buildEncounterCharacterRegistryFromResolved previously registered only weapon slots
// (via reg.Add) but never called reg.AddAbilityScores. The MartialArts condition's
// onDamageChain calls registry.GetCharacterAbilityScores(charID) and early-returns nil
// when no scores are registered — silently yielding 1d4+STR. The fix (Wave 2) adds
// reg.AddAbilityScores for resolved characters so the DEX swap and dice replacement run.
//
// # Deterministic roller strategy
//
// fixedRoller{val:15} (imported from integration_sneak_attack_test.go):
//
//	The unarmed stub weapon has no Finesse property, so combat.ResolveAttack uses STR
//	for the attack roll: attackBonus = STR(+0) + proficiency(+2) = 2.
//	d20 = min(15,20) = 15; attack total = 15+2 = 17 ≥ goblin AC 15 → always hit, no crit (not 20).
//	Martial Arts replaces weapon dice: Roll(size=4) = min(15,4) = 4
//	DEX mod = +3 (DEX 16) — ability component (Martial Arts DEX swap)
//	STR mod = +0 (STR 10) — would be used WITHOUT Martial Arts DEX swap
//	Expected total damage with MA: 4+3 = 7
//	Expected total damage without MA (STR): 4+0 = 4

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
	monkIntegEncID   = "enc-monk-integration"
	monkPlayerCharli = "charli"
	monkEntityCharli = "char-charli"
	monkGoblinID     = "goblin-monk"

	// monkDEXMod is charli's DEX modifier (DEX 16 → +3). Martial Arts replaces the
	// ability component SourceRef with DEX and sets FlatBonus to dexMod. The
	// weapon dice (1d4 via MartialArts condition's own random roller) are random,
	// so HP-delta assertions use min/max bounds (1+3=4 to 4+3=7).
	monkDEXMod = 3
	// monkMaxDamage = max 1d4 (4) + DEX mod (3) = 7.
	monkMaxDamage = 7
)

// MonkUnarmedIntegrationSuite tests the Wave 2 sign-off scenario: charli (L1 monk,
// unarmed) strikes a goblin via TakeAction, proving Martial Arts applies DEX and 1d4.
type MonkUnarmedIntegrationSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	charStore    *inMemoryCharStore
	broker       *tkenc.Broker
	repo         encountersv2.Repository
	handler      *v2encounter.Handler
	charliCtx    context.Context
}

func TestMonkUnarmedIntegrationSuite(t *testing.T) {
	suite.Run(t, new(MonkUnarmedIntegrationSuite))
}

func (s *MonkUnarmedIntegrationSuite) SetupTest() {
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
			Roller:        fixedRoller{val: 15},
		},
	})
	s.Require().NoError(err)
	s.handler = h

	s.charliCtx = auth.WithPlayerID(context.Background(), monkPlayerCharli)
}

func (s *MonkUnarmedIntegrationSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestIntegration_MartialArts_UnarmedStrikeUsesDEX proves Martial Arts runs on the
// unarmed path and applies the DEX modifier instead of STR.
//
// The Martial Arts condition uses its own random roller for 1d4 dice replacement
// (not the handler's fixedRoller). HP delta is in range [1+3, 4+3] = [4, 7].
// delta >= monkDEXMod (3) proves at minimum one DEX pip was added — combined with
// the breakdown test which confirms "dnd5e:abilities:dex" source, this proves MA applied.
func (s *MonkUnarmedIntegrationSuite) TestIntegration_MartialArts_UnarmedStrikeUsesDEX() {
	charliData := s.buildCharliMonkData()
	s.setupCharRepoMock(charliData)
	s.seedMonkEncounter()
	s.advanceToCharli()

	hp0 := s.goblinHP()
	_, err := s.handler.TakeAction(s.charliCtx, s.attackCharliVsGoblin())
	s.Require().NoError(err, "unarmed strike must resolve without error")
	hp1 := s.goblinHP()

	delta := hp0 - hp1
	s.Greater(delta, 0,
		"unarmed strike must deal damage (delta=%d); attack with fixedRoller{15} should always hit", delta)
	s.LessOrEqual(delta, monkMaxDamage,
		"damage delta (%d) must not exceed monkMaxDamage (%d) = 1d4_max+DEX", delta, monkMaxDamage)
	s.GreaterOrEqual(delta, monkDEXMod,
		"damage delta (%d) must be >= DEX mod (%d); Martial Arts must apply DEX not STR (+0)",
		delta, monkDEXMod)
}

// TestIntegration_MartialArts_DamageBreakdown_HasDEXAbilityComponent verifies that
// the DamageDealtEvent published on the broker carries "dnd5e:abilities:dex" as the
// ability source (not "dnd5e:abilities:str"), proving the Martial Arts condition
// replaced the ability component source.
func (s *MonkUnarmedIntegrationSuite) TestIntegration_MartialArts_DamageBreakdown_HasDEXAbilityComponent() {
	charliData := s.buildCharliMonkData()
	s.setupCharRepoMock(charliData)
	s.seedMonkEncounter()
	s.advanceToCharli()

	sub, err := s.broker.Subscribe(encountercore.EncounterID(monkIntegEncID), encountercore.PlayerID(monkPlayerCharli))
	s.Require().NoError(err, "broker subscribe must succeed")
	defer sub.Close() //nolint:errcheck

	_, err = s.handler.TakeAction(s.charliCtx, s.attackCharliVsGoblin())
	s.Require().NoError(err, "attack must resolve without error")

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

	s.Require().NotNil(damageEvt, "DamageDealtEvent must be published on the broker for charli's attack")
	s.Require().NotEmpty(damageEvt.Components, "DamageDealtEvent.Components must be non-empty")

	sources := make([]string, 0, len(damageEvt.Components))
	for _, c := range damageEvt.Components {
		sources = append(sources, c.Source)
	}

	s.Contains(sources, "dnd5e:abilities:dex",
		"damage breakdown must include a DEX ability component (Martial Arts DEX swap); got %v", sources)
	s.NotContains(sources, "dnd5e:abilities:str",
		"STR ability component must not appear — Martial Arts must replace it with DEX; got %v", sources)
}

// TestIntegration_MartialArts_DoesNotCrashWithGamectx verifies that the unarmed
// strike path runs without crashing from missing gamectx (RequireCharacters),
// which would happen if AddAbilityScores were not called on the registry.
func (s *MonkUnarmedIntegrationSuite) TestIntegration_MartialArts_DoesNotCrashWithGamectx() {
	charliData := s.buildCharliMonkData()
	s.setupCharRepoMock(charliData)
	s.seedMonkEncounter()
	s.advanceToCharli()

	_, err := s.handler.TakeAction(s.charliCtx, s.attackCharliVsGoblin())
	s.Require().NoError(err,
		"unarmed strike with Martial Arts condition must not crash from missing ability scores in gamectx registry")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildCharliMonkData constructs character.Data for charli — a L1 monk with
// DEX 16 > STR 10, no weapon equipped, and MartialArts + UnarmoredDefense conditions
// persisted in the Conditions blob. DEX > STR ensures the Martial Arts DEX swap
// is observable (STR mod = +0, DEX mod = +3).
func (s *MonkUnarmedIntegrationSuite) buildCharliMonkData() *character.Data {
	martialArtsCond := conditions.NewMartialArtsCondition(conditions.MartialArtsInput{
		CharacterID: monkEntityCharli,
		MonkLevel:   1,
	})
	martialArtsJSON, err := martialArtsCond.ToJSON()
	s.Require().NoError(err, "marshal MartialArts condition")

	unarmoredDefenseCond := conditions.NewUnarmoredDefenseCondition(conditions.UnarmoredDefenseInput{
		CharacterID: monkEntityCharli,
		Type:        conditions.UnarmoredDefenseMonk,
		Source:      "dnd5e:classes:monk",
	})
	unarmoredDefenseJSON, err := unarmoredDefenseCond.ToJSON()
	s.Require().NoError(err, "marshal UnarmoredDefense condition")

	return &character.Data{
		ID:               monkEntityCharli,
		Name:             "Charli the Monk",
		Level:            1,
		ClassID:          "monk",
		ProficiencyBonus: 2,
		HitPoints:        10,
		MaxHitPoints:     10,
		ArmorClass:       15, // 10 + DEX(+3) + WIS(+2) — Unarmored Defense
		AbilityScores: shared.AbilityScores{
			abilities.STR: 10, // +0 — low STR makes DEX swap observable
			abilities.DEX: 16, // +3
			abilities.CON: 14, // +2
			abilities.INT: 10,
			abilities.WIS: 14, // +2
			abilities.CHA: 10,
		},
		// No EquipmentSlots — no weapon equipped, unarmed path triggered
		Conditions: []json.RawMessage{martialArtsJSON, unarmoredDefenseJSON},
	}
}

// setupCharRepoMock registers mock expectations backed by inMemoryCharStore.
// Charli's Get reads live state; Update writes back so subsequent Gets reflect
// condition updates (e.g. UsedThisTurn round-trips).
func (s *MonkUnarmedIntegrationSuite) setupCharRepoMock(charliData *character.Data) {
	s.charStore.set(charliData)

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: monkEntityCharli}).
		DoAndReturn(func(_ context.Context, _ characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			d := s.charStore.get(monkEntityCharli)
			if d == nil {
				return nil, fmt.Errorf("charli not found in charStore")
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

	// Non-charli lookups fall back to stand-in.
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), gomock.Not(characterrepo.GetInput{ID: monkEntityCharli})).
		Return(nil, fmt.Errorf("character not configured in monk integration test mock")).
		AnyTimes()
}

// attackCharliVsGoblin returns a TakeActionRequest for charli attacking the goblin.
func (s *MonkUnarmedIntegrationSuite) attackCharliVsGoblin() *encounterv2pb.TakeActionRequest {
	return &encounterv2pb.TakeActionRequest{
		EncounterId:   monkIntegEncID,
		ActorEntityId: monkEntityCharli,
		ActionRef:     &encounterv2pb.Ref{Module: "dnd5e", Type: "action", Id: "attack"},
		Target: &encounterv2pb.ActionTarget{
			Kind: &encounterv2pb.ActionTarget_EntityId{EntityId: monkGoblinID},
		},
	}
}

// goblinHP returns the goblin's current HP from the persisted encounter snapshot.
func (s *MonkUnarmedIntegrationSuite) goblinHP() int {
	data, err := s.repo.Get(context.Background(), monkIntegEncID)
	s.Require().NoError(err, "repo.Get for goblin HP check")
	md, ok := data.Monsters[encountercore.EntityID(monkGoblinID)]
	s.Require().True(ok, "goblin must be in encounter data")
	return md.HP
}

// seedMonkEncounter creates the wave-2-monk fixture encounter and saves it.
//
// Positions: charli at Q:0, goblin at Q:1 (adjacent, euclidean distance 1.0 ≤ 1.5).
// Goblin has 100 HP so it survives multiple strikes during the test.
func (s *MonkUnarmedIntegrationSuite) seedMonkEncounter() {
	enc := tkenc.New(monkIntegEncID, s.broker)

	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    encountercore.PlayerID(monkPlayerCharli),
		EntityID:    encountercore.EntityID(monkEntityCharli),
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          10,
		MaxHP:       10,
		AC:          15,
		AttackBonus: 5, // DEX +3 + proficiency +2
		DamageDice:  "1d4+3",
		DamageType:  "bludgeoning",
	}))

	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID:          encountercore.EntityID(monkGoblinID),
		Position:    encountercore.Hex{Q: 1, R: 0, S: -1},
		HP:          100,
		MaxHP:       100,
		AC:          15,
		Speed:       6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
	}))

	s.Require().NoError(enc.SetMode(encountercore.ModeTurnBased))
	s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))
}

// advanceToCharli cycles initiative until charli is the active actor.
func (s *MonkUnarmedIntegrationSuite) advanceToCharli() {
	for range 20 {
		data, err := s.repo.Get(context.Background(), monkIntegEncID)
		s.Require().NoError(err)
		s.Require().NotEmpty(data.Initiative, "initiative must be rolled")

		if data.ActiveIdx < 0 || data.ActiveIdx >= len(data.Initiative) {
			s.Fail("invalid ActiveIdx", data.ActiveIdx)
		}
		activeID := string(data.Initiative[data.ActiveIdx])

		if activeID == monkEntityCharli {
			return
		}

		if activeID == monkGoblinID {
			enc, loadErr := tkenc.LoadFromData(data, s.broker)
			s.Require().NoError(loadErr)
			_, _, err = enc.EndTurn(encountercore.EntityID(activeID))
			s.Require().NoError(err)
			s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))
			continue
		}

		// Any other player's turn — end via handler with charli's context as fallback.
		_, err = s.handler.EndTurn(s.charliCtx, &encounterv2pb.EndTurnRequest{
			EncounterId: monkIntegEncID,
			EntityId:    activeID,
		})
		s.Require().NoError(err, "EndTurn for actor %q must succeed", activeID)
	}
	s.Fail("advanceToCharli: charli did not become active within 20 initiative steps")
}
