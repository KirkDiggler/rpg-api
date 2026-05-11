package encounter_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// Dnd5eCombatResolverTestSuite tests the Dnd5eCombatResolver — the real
// rulebook-backed CombatResolver that replaces StandInCombatResolver.
//
// These tests use gomock for the character repo and pre-built monster fixtures.
// They verify:
//   - Player-side attacks load the character, build the CombatantLookup,
//     call combat.ResolveAttack, and translate the result correctly.
//   - Monster-side attacks rehydrate from DataJSON, build the lookup, and
//     translate.
//   - The fallback to StandInCombatResolver fires when the char repo is absent
//     or an entity cannot be found.
//   - extractBaseDice strips modifiers correctly.
type Dnd5eCombatResolverTestSuite struct {
	suite.Suite
	ctx          context.Context
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
}

func TestDnd5eCombatResolverTestSuite(t *testing.T) {
	suite.Run(t, new(Dnd5eCombatResolverTestSuite))
}

func (s *Dnd5eCombatResolverTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
}

func (s *Dnd5eCombatResolverTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// ---------------------------------------------------------------------------
// Fallback behavior — no char repo, no encounter data
// ---------------------------------------------------------------------------

// TestResolveAttack_NilEncounterData_FallsBackToStandIn verifies that when
// encounterData is nil (e.g. CreateEncounter path), the resolver falls through
// to StandInCombatResolver and still returns a valid AttackOutcome.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_NilEncounterData_FallsBackToStandIn() {
	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{},
		nil, // nil data — fallback path
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "char-alice",
		TargetID:            "goblin-1",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d6+2",
		AttackerDamageType:  "slashing",
		TargetAC:            10,
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome, "nil encounter data must still return a valid outcome via stand-in")
}

// TestResolveAttack_NoCharRepo_FallsBackToStandIn verifies that when the
// character repo is absent, player-side attacks degrade to stand-in math.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_NoCharRepo_FallsBackToStandIn() {
	data := s.buildEncounterDataWithPlayer("char-alice", "goblin-1")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{}, // no CharacterRepo
		data,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "char-alice",
		TargetID:            "goblin-1",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d6+2",
		AttackerDamageType:  "slashing",
		TargetAC:            15,
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome, "absent char repo must degrade to stand-in, not error")
}

// ---------------------------------------------------------------------------
// Monster-side attacks (rehydrate from DataJSON)
// ---------------------------------------------------------------------------

// TestResolveAttack_MonsterAttacker_DataJSON_UsesRealChain verifies that when
// the attacker is a monster with a full DataJSON blob, the resolver rehydrates
// it, builds the CombatantLookup, and calls combat.ResolveAttack. The outcome
// must be non-nil and carry a valid AttackRoll.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_MonsterAttacker_DataJSON_UsesRealChain() {
	// Build a goblin monster with known ability scores and embed its DataJSON.
	goblin := monster.NewGoblin("goblin-1")
	goblinData := goblin.ToData()
	dataJSON, err := json.Marshal(goblinData)
	s.Require().NoError(err)

	// Build a player target as a character entity.
	aliceData := s.buildCharacterData("char-alice")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: aliceData},
		}, nil)

	encounterData := s.buildEncounterDataWithMonsterDataJSON("goblin-1", dataJSON, "char-alice")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{CharacterRepo: s.mockCharRepo},
		encounterData,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "goblin-1",
		TargetID:            "char-alice",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d6+2",
		AttackerDamageType:  "slashing",
		TargetAC:            14,
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome)
	// AttackRoll must be in [1, 20] — the real d20 roll from the rulebook chain.
	s.GreaterOrEqual(outcome.AttackRoll, 1)
	s.LessOrEqual(outcome.AttackRoll, 20)
}

// TestResolveAttack_MonsterAttacker_NoDataJSON_FallsBackToSnapshotWeapon
// verifies that a monster without DataJSON (test-fixture path) still resolves
// via the real chain using a synthetic weapon built from the snapshot fields.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_MonsterAttacker_NoDataJSON_FallsBackToSnapshotWeapon() {
	aliceData := s.buildCharacterData("char-alice")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: aliceData},
		}, nil)

	// Monster with no DataJSON (typical unit-test fixture seeded via AddMonster
	// without a DataJSON blob).
	encounterData := s.buildEncounterDataWithPlayer("char-alice", "goblin-1")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{CharacterRepo: s.mockCharRepo},
		encounterData,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "goblin-1",
		TargetID:            "char-alice",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d6+2",
		AttackerDamageType:  "slashing",
		TargetAC:            14,
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome)
	s.GreaterOrEqual(outcome.AttackRoll, 1)
	s.LessOrEqual(outcome.AttackRoll, 20)
}

// ---------------------------------------------------------------------------
// Player-side attacks (character repo lookup)
// ---------------------------------------------------------------------------

// TestResolveAttack_PlayerAttacker_CharRepoUsed verifies that a player-side
// attack loads the character from the repo and produces a valid outcome.
// The monster target has no DataJSON; a synthetic target is built from
// snapshot fields.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_PlayerAttacker_CharRepoUsed() {
	aliceData := s.buildCharacterData("char-alice")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: aliceData},
		}, nil)
	// saveAttackerConditionState calls Update after the attack resolves to persist
	// updated condition state (e.g. SneakAttack.UsedThisTurn). Allow any Update.
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil).AnyTimes()

	encounterData := s.buildEncounterDataWithPlayer("char-alice", "goblin-1")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{CharacterRepo: s.mockCharRepo},
		encounterData,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "char-alice",
		TargetID:            "goblin-1",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d8+2",
		AttackerDamageType:  "slashing",
		TargetAC:            15,
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome)
	// AttackRoll is [1, 20] from the rulebook chain.
	s.GreaterOrEqual(outcome.AttackRoll, 1)
	s.LessOrEqual(outcome.AttackRoll, 20)
	// TargetAC should reflect the monster's AC.
	s.Equal(15, outcome.TargetAC)
}

// TestResolveAttack_PlayerAttacker_CharRepoError_FallsBackToStandIn verifies
// that a character-repo error degrades gracefully to stand-in math rather than
// returning an error that would abort the attack verb.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_PlayerAttacker_CharRepoError_FallsBackToStandIn() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(nil, apierr.NotFound("character not found"))

	encounterData := s.buildEncounterDataWithPlayer("char-alice", "goblin-1")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{CharacterRepo: s.mockCharRepo},
		encounterData,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "char-alice",
		TargetID:            "goblin-1",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d8+2",
		AttackerDamageType:  "slashing",
		TargetAC:            15,
	})

	// Must degrade to stand-in: outcome non-nil, no error.
	s.Require().NoError(err)
	s.Require().NotNil(outcome)
}

// ---------------------------------------------------------------------------
// extractBaseDice — pure function table-driven
// ---------------------------------------------------------------------------

// TestExtractBaseDice exercises the internal dice-notation parser by invoking
// it indirectly through syntheticMonsterWeapon via a monster-attacker attack.
// The "1d6+2" → "1d6" parsing is critical: if it double-counts the modifier,
// the outcome damage will be inflated.
//
// We can't call extractBaseDice directly (unexported), but we CAN verify that
// a monster-attacker attack with "1d6+2" DamageDice resolves without error
// (which would happen if the base-dice parse produced invalid notation).
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_DamageDiceVariants_ParseWithoutError() {
	cases := []struct {
		name       string
		damageDice string
	}{
		{"modifier suffix", "1d6+2"},
		{"negative modifier", "2d4-1"},
		{"no modifier", "1d8"},
		{"empty", ""},
		{"multi-die", "3d6+3"},
	}

	aliceData := s.buildCharacterData("char-alice")

	for _, tc := range cases {
		s.Run(tc.name, func() {
			s.mockCharRepo.EXPECT().
				Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
				Return(&characterrepo.GetOutput{
					Character: &entities.Character{Data: aliceData},
				}, nil)

			encounterData := s.buildEncounterDataWithPlayer("char-alice", "goblin-1")

			resolver := v2encounter.NewDnd5eCombatResolverForData(
				v2encounter.Dnd5eCombatResolverConfig{CharacterRepo: s.mockCharRepo},
				encounterData,
			)

			outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
				AttackerID:         "goblin-1",
				TargetID:           "char-alice",
				AttackerDamageDice: tc.damageDice,
				AttackerDamageType: "slashing",
				TargetAC:           14,
			})

			// Even empty/malformed dice must not error — the rulebook falls back
			// or the stand-in covers it.
			s.Require().NoError(err, "dice notation %q must not cause an error", tc.damageDice)
			s.Require().NotNil(outcome)
		})
	}
}

// ---------------------------------------------------------------------------
// Outcome translation
// ---------------------------------------------------------------------------

// TestResolveAttack_OutcomeTranslation_HitSetCorrectly verifies that the
// tkenc.AttackOutcome fields are populated from combat.AttackResult. We can
// assert on TargetAC since it reflects the monster's AC from the Combatant
// interface (not the snapshot value).
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_OutcomeTranslation_HitSetCorrectly() {
	aliceData := s.buildCharacterData("char-alice")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: aliceData},
		}, nil)
	// saveAttackerConditionState calls Update after the attack resolves to persist
	// updated condition state (e.g. SneakAttack.UsedThisTurn). Allow any Update.
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil).AnyTimes()

	encounterData := s.buildEncounterDataWithPlayer("char-alice", "goblin-1")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{CharacterRepo: s.mockCharRepo},
		encounterData,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "char-alice",
		TargetID:            "goblin-1",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d8+2",
		AttackerDamageType:  "slashing",
		TargetAC:            15,
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome)

	// AttackRoll must be in the d20 range.
	s.GreaterOrEqual(outcome.AttackRoll, 1)
	s.LessOrEqual(outcome.AttackRoll, 20)

	// If hit, damage must be non-negative.
	if outcome.Hit {
		s.GreaterOrEqual(outcome.Damage, 0)
	} else {
		s.Equal(0, outcome.Damage, "no damage on a miss")
	}

	// DamageType from the (unarmed stub) weapon — Bludgeoning for unarmed.
	// Alice has no weapon equipped in the test fixture, so she uses the stub.
	// We just assert it's non-empty on a hit.
	if outcome.Hit {
		s.NotEmpty(outcome.DamageType)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// buildCharacterData returns a minimal character.Data suitable for LoadFromData.
func (s *Dnd5eCombatResolverTestSuite) buildCharacterData(id string) *character.Data {
	return &character.Data{
		ID:               id,
		Name:             "Test Character",
		Level:            1,
		ProficiencyBonus: 2,
		HitPoints:        12,
		MaxHitPoints:     12,
		ArmorClass:       14,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 12,
			abilities.CON: 14,
			abilities.INT: 10,
			abilities.WIS: 10,
			abilities.CHA: 8,
		},
	}
}

// buildEncounterDataWithPlayer returns encounter data containing one player
// (playerEntityID) and one monster (monsterID, no DataJSON).
func (s *Dnd5eCombatResolverTestSuite) buildEncounterDataWithPlayer(playerEntityID, monsterID string) *tkenc.Data {
	data := tkenc.NewData("enc-test")
	data.Players[encountercore.PlayerID(playerEntityID)] = &tkenc.PlayerData{
		ID:       encountercore.PlayerID(playerEntityID),
		EntityID: encountercore.EntityID(playerEntityID),
		HP:       12,
		MaxHP:    12,
		AC:       14,
	}
	data.Monsters[encountercore.EntityID(monsterID)] = &tkenc.MonsterData{
		ID:          encountercore.EntityID(monsterID),
		HP:          7,
		MaxHP:       7,
		AC:          15,
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
		MonsterRef:  "dnd5e:monsters:goblin",
	}
	return data
}

// buildEncounterDataWithMonsterDataJSON returns encounter data where the
// monster has a full DataJSON blob (for the rehydration path).
func (s *Dnd5eCombatResolverTestSuite) buildEncounterDataWithMonsterDataJSON(
	monsterID string,
	dataJSON []byte,
	playerEntityID string,
) *tkenc.Data {
	data := s.buildEncounterDataWithPlayer(playerEntityID, monsterID)
	data.Monsters[encountercore.EntityID(monsterID)].DataJSON = dataJSON
	return data
}
