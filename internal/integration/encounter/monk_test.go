// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package encounter_integration provides API-level integration tests for class features.
// These tests verify that toolkit features work correctly when invoked through the API layer.
package encounter_integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
)

// ============================================================================
// MONK API INTEGRATION TESTS
// Level 1 Features:
//   - Martial Arts (DEX for attacks, d4 damage die, bonus unarmed strike)
//   - Unarmored Defense (AC = 10 + DEX + WIS)
// ============================================================================

type MonkIntegrationSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	server *harness.TestServer
}

func TestMonkIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(MonkIntegrationSuite))
}

func (s *MonkIntegrationSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	var err error
	s.server, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  Monk Integration Test Server Ready                              ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
}

func (s *MonkIntegrationSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *MonkIntegrationSuite) SetupTest() {
	err := s.server.FlushRedis(s.ctx)
	s.Require().NoError(err, "failed to flush redis")
}

// authCtx adds dev auth header for test requests
func (s *MonkIntegrationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// =============================================================================
// CHARACTER CREATION HELPERS
// =============================================================================

func (s *MonkIntegrationSuite) createMonkCharacter(playerID string) string {
	ctx := s.authCtx(playerID)

	// Step 1: Create character draft
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err, "failed to create draft")
	s.Require().NotNil(createResp.GetDraft(), "draft should not be nil")
	draftID := createResp.GetDraft().GetId()
	s.Require().NotEmpty(draftID, "draft ID should not be empty")

	// Step 2: Set name
	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    "Kira Shadowstep",
	})
	s.Require().NoError(err, "failed to set name")

	// Step 3: Set Human with language choice
	_, err = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				Selection: &dnd5ev1alpha1.ChoiceData_Languages{
					Languages: &dnd5ev1alpha1.LanguageSelection{
						Languages: []dnd5ev1alpha1.Language{
							dnd5ev1alpha1.Language_LANGUAGE_ELVISH,
						},
					},
				},
			},
		},
	})
	s.Require().NoError(err, "failed to set race")

	// Step 4: Set Monk with skills, equipment, and tool choices
	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_MONK,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			// Skills: Acrobatics, Stealth
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ACROBATICS,
							dnd5ev1alpha1.Skill_SKILL_STEALTH,
						},
					},
				},
			},
			// Primary weapon: Shortsword (monk weapon)
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "monk-weapons-primary",
				OptionId: "monk-weapon-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Pack: Dungeoneer's pack
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "monk-pack",
				OptionId: "monk-pack-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Tools: Calligrapher's supplies
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Tools{
					Tools: &dnd5ev1alpha1.ToolSelection{
						Tools: []dnd5ev1alpha1.Tool{
							dnd5ev1alpha1.Tool_TOOL_CALLIGRAPHER_SUPPLIES,
						},
					},
				},
			},
		},
	})
	s.Require().NoError(err, "failed to set class")

	// Step 5: Set background (Hermit)
	_, err = s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_HERMIT,
	})
	s.Require().NoError(err, "failed to set background")

	// Step 6: Set ability scores (Monk-optimized: DEX > WIS > CON)
	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     10,
				Dexterity:    16, // +3 modifier
				Constitution: 14, // +2 modifier
				Intelligence: 8,
				Wisdom:       15, // +2 modifier
				Charisma:     10,
			},
		},
	})
	s.Require().NoError(err, "failed to set ability scores")

	// Step 7: Finalize draft
	finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err, "failed to finalize draft")
	s.Require().NotNil(finalizeResp.GetCharacter(), "character should not be nil")
	s.Require().NotEmpty(finalizeResp.GetCharacter().GetId(), "character ID should not be empty")

	return finalizeResp.GetCharacter().GetId()
}

// =============================================================================
// TESTS: MARTIAL ARTS
// =============================================================================

func (s *MonkIntegrationSuite) TestMartialArts_CanAttackInCombat() {
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  MONK MARTIAL ARTS: Can Attack With Monk Weapon in Combat        ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-monk-bonus-strike"
	ctx := s.authCtx(playerID)

	// 1. Create monk character
	s.T().Log("Step 1: Creating monk character...")
	characterID := s.createMonkCharacter(playerID)
	s.T().Logf("  ✓ Created character: %s", characterID)

	// 2. Create encounter and start combat
	s.T().Log("Step 2: Starting combat...")
	encounterID := CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
	s.T().Logf("  ✓ Encounter: %s", encounterID)

	// 3. Find a target monster
	targetMonsterID := FindMonsterTarget(s.T(), s.server, ctx, playerID, encounterID)
	s.T().Logf("  ✓ Found monster target: %s", targetMonsterID)

	// 4. Attack with monk weapon
	s.T().Log("Step 4: Attacking with monk weapon (shortsword)...")
	attackResp, err := s.server.EncounterClient.Attack(ctx, &dnd5ev1alpha1.AttackRequest{
		EncounterId: encounterID,
		AttackerId:  characterID,
		TargetId:    targetMonsterID,
	})
	s.Require().NoError(err, "failed to resolve attack")
	s.Require().True(attackResp.GetSuccess(), "attack request should succeed: %s", attackResp.GetError())

	result := attackResp.GetResult()
	s.Require().NotNil(result, "attack result should not be nil")

	// Assert response structure regardless of hit/miss
	s.Assert().Greater(result.GetAttackRoll(), int32(0), "attack roll should be positive (1d20)")
	s.Assert().Greater(result.GetAttackTotal(), int32(0), "attack total should be positive (roll + bonus)")
	s.Assert().Greater(result.GetTargetAc(), int32(0), "target AC should be positive")

	s.T().Logf("  ✓ Attack roll: %d + bonus = %d vs AC %d → %s",
		result.GetAttackRoll(), result.GetAttackTotal(), result.GetTargetAc(),
		map[bool]string{true: "HIT", false: "MISS"}[result.GetHit()])

	if result.GetHit() {
		s.Assert().Greater(result.GetDamage(), int32(0), "damage should be positive on hit")
		s.Assert().NotEmpty(result.GetDamageType(), "damage type should be set on hit")
		s.T().Logf("  ✓ Damage: %d (%s)", result.GetDamage(), result.GetDamageType())

		breakdown := result.GetDamageBreakdown()
		s.Assert().NotNil(breakdown, "damage breakdown should be present on hit")
		if breakdown != nil {
			s.Assert().NotEmpty(breakdown.GetComponents(), "damage breakdown should have components")
			s.T().Log("  Damage breakdown:")
			for _, comp := range breakdown.GetComponents() {
				s.T().Logf("    - Source: %s, Dice: %v, Flat: %d, Type: %s",
					comp.GetSource(), comp.GetFinalDiceRolls(), comp.GetFlatBonus(), comp.GetDamageType())
			}
		}
	}

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  ✓ TEST PASSED: Monk can attack with monk weapon                 ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
}

func (s *MonkIntegrationSuite) TestMartialArts_DamageUsesMADie() {
	// Skip: This test cannot reliably verify martial arts die without a mocked roller.
	// The toolkit has unit tests for MartialArtsCondition that verify damage scaling.
	// TODO: Add unit tests at orchestrator level with injectable roller.
	s.T().Skip("Skipped: Needs mocked roller for deterministic damage verification")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  MONK MARTIAL ARTS: Unarmed Strike Uses Martial Arts Die         ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-monk-ma-die"
	ctx := s.authCtx(playerID)

	// 1. Create monk character
	s.T().Log("Step 1: Creating monk character...")
	characterID := s.createMonkCharacter(playerID)
	s.T().Logf("  ✓ Created character: %s", characterID)

	// 2. Create encounter and start combat
	encounterID := CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
	s.T().Log("  ✓ Combat started")

	// 3. Get monster target
	targetMonsterID := FindMonsterTarget(s.T(), s.server, ctx, playerID, encounterID)

	// 4. Attack (will use equipped monk weapon or unarmed)
	s.T().Log("Step 2: Attacking to verify damage breakdown...")
	attackResp, err := s.server.EncounterClient.Attack(ctx, &dnd5ev1alpha1.AttackRequest{
		EncounterId: encounterID,
		AttackerId:  characterID,
		TargetId:    targetMonsterID,
	})
	s.Require().NoError(err)

	result := attackResp.GetResult()
	if result.GetHit() {
		breakdown := result.GetDamageBreakdown()
		if breakdown != nil {
			s.T().Log("  Damage breakdown:")
			for _, comp := range breakdown.GetComponents() {
				s.T().Logf("    - Source: %s, Dice: %v, Flat: %d, Type: %s",
					comp.GetSource(), comp.GetFinalDiceRolls(), comp.GetFlatBonus(), comp.GetDamageType())
			}

			// At level 1, martial arts die is 1d4. Shortsword is 1d6.
			// If using shortsword, should see 1d6. If using unarmed with MA, could be 1d4.
			s.T().Logf("  Total damage: %d", result.GetDamage())
			s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
			s.T().Log("║  ✓ TEST PASSED: Attack resolved with damage breakdown            ║")
			s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
		} else {
			s.T().Log("  ⚠ No damage breakdown available")
		}
	} else {
		s.T().Log("  Attack missed - damage die verification skipped")
		s.T().Skip("Attack missed - cannot verify martial arts die")
	}
}

// =============================================================================
// TESTS: UNARMORED DEFENSE
// =============================================================================

func (s *MonkIntegrationSuite) TestUnarmoredDefense_ACCalculation() {
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  MONK UNARMORED DEFENSE: AC = 10 + DEX + WIS                      ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	// This test verifies at the character creation level that AC is calculated correctly.
	// Base scores: DEX 16, WIS 15. Human gives +1 to all = DEX 17 (+3), WIS 16 (+3)
	// AC should be 10 + 3 + 3 = 16

	playerID := "test-player-monk-unarmored"
	ctx := s.authCtx(playerID)

	s.T().Log("Step 1: Creating monk with DEX 17 (+3) and WIS 16 (+3) after Human bonus...")
	characterID := s.createMonkCharacter(playerID)
	s.T().Logf("  ✓ Created character: %s", characterID)

	// Get character to verify AC
	charResp, err := s.server.CharacterClient.GetCharacter(ctx, &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: characterID,
	})
	s.Require().NoError(err, "failed to get character")

	actualAC := charResp.GetCharacter().GetCombatStats().GetArmorClass()
	expectedAC := int32(16) // 10 + 3 (DEX 17) + 3 (WIS 16) - Human gives +1 to all

	s.T().Logf("  Character AC: %d (expected: %d)", actualAC, expectedAC)

	if actualAC == expectedAC {
		s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
		s.T().Log("║  ✓ TEST PASSED: Unarmored Defense AC = 10 + DEX + WIS            ║")
		s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
	} else {
		s.T().Logf("  ⚠ AC mismatch: got %d, expected %d", actualAC, expectedAC)
		s.Assert().Equal(expectedAC, actualAC, "Monk Unarmored Defense should be 10 + DEX + WIS")
	}
}

// =============================================================================
// KI FEATURE TESTS (Level 2)
// These tests create a level 1 Monk, patch to level 2 with Ki, then test
// feature activation through the API layer.
// =============================================================================

func (s *MonkIntegrationSuite) TestFlurryOfBlows_ActivateFeature_GrantsStrikes() {
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  MONK FLURRY OF BLOWS: Activate via gRPC API                     ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-monk-flurry"
	ctx := s.authCtx(playerID)

	// 1. Create monk and level up to 2 (gains Ki)
	s.T().Log("Step 1: Creating level 2 monk...")
	characterID := s.createMonkCharacter(playerID)
	LevelUpMonkToLevel2(s.T(), s.server, ctx, characterID)

	// 2. Start combat
	s.T().Log("Step 2: Starting combat...")
	encounterID := CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
	s.T().Logf("  ✓ Encounter: %s", encounterID)

	// 3. Activate Flurry of Blows
	s.T().Log("Step 3: Activating Flurry of Blows...")
	activateResp, err := s.server.EncounterClient.ActivateFeature(ctx, &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: encounterID,
		CharacterId: characterID,
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_FLURRY_OF_BLOWS,
	})
	s.Require().NoError(err, "failed to activate flurry of blows")
	s.Assert().True(activateResp.GetSuccess(), "flurry of blows should succeed: %s", activateResp.GetMessage())
	s.T().Logf("  ✓ Flurry of Blows activated: %s", activateResp.GetMessage())

	// 4. Check encounter state for flurry strikes in action economy
	stateResp, err := s.server.EncounterClient.GetEncounterState(ctx, &dnd5ev1alpha1.GetEncounterStateRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
	})
	s.Require().NoError(err, "failed to get encounter state")

	turnState := stateResp.GetCombatState().GetCurrentTurn()
	s.Require().NotNil(turnState, "turn state should not be nil")

	actionEconomy := turnState.GetActionEconomy()
	s.Require().NotNil(actionEconomy, "ActionEconomy must be present in GetEncounterState response")

	flurryStrikes := actionEconomy.GetFlurryStrikesRemaining()
	s.T().Logf("  ✓ Flurry strikes remaining: %d", flurryStrikes)
	s.Require().Equal(int32(2), flurryStrikes, "Flurry of Blows should grant 2 strikes")
}

func (s *MonkIntegrationSuite) TestPatientDefense_ActivateFeature_Success() {
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  MONK PATIENT DEFENSE: Activate via gRPC API                      ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-monk-patient"
	ctx := s.authCtx(playerID)

	// 1. Create monk and level up
	s.T().Log("Step 1: Creating level 2 monk...")
	characterID := s.createMonkCharacter(playerID)
	LevelUpMonkToLevel2(s.T(), s.server, ctx, characterID)

	// 2. Start combat
	s.T().Log("Step 2: Starting combat...")
	encounterID := CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
	s.T().Logf("  ✓ Encounter: %s", encounterID)

	// 3. Activate Patient Defense
	s.T().Log("Step 3: Activating Patient Defense...")
	activateResp, err := s.server.EncounterClient.ActivateFeature(ctx, &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: encounterID,
		CharacterId: characterID,
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_PATIENT_DEFENSE,
	})
	s.Require().NoError(err, "failed to activate patient defense")
	s.Require().True(activateResp.GetSuccess(), "patient defense should succeed: %s", activateResp.GetMessage())
	s.T().Logf("  ✓ Patient Defense activated: %s", activateResp.GetMessage())

	// 4. Verify DodgeActive in encounter state
	s.T().Log("Step 4: Verifying DodgeActive in encounter state...")
	stateResp, err := s.server.EncounterClient.GetEncounterState(ctx, &dnd5ev1alpha1.GetEncounterStateRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
	})
	s.Require().NoError(err, "failed to get encounter state")

	actionEconomy := stateResp.GetCombatState().GetCurrentTurn().GetActionEconomy()
	s.Require().NotNil(actionEconomy, "ActionEconomy must be present after Patient Defense")
	// DodgeActive/DisengageActive are now tracked as conditions on the character,
	// not as flags on the proto ActionEconomy. Verify bonus action was consumed instead.
	s.Require().False(actionEconomy.GetBonusActionAvailable(), "bonus action should be consumed after Patient Defense")
	s.T().Log("  ✓ Bonus action consumed confirmed in encounter state")
}

func (s *MonkIntegrationSuite) TestStepOfTheWind_ActivateFeature_Success() {
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  MONK STEP OF THE WIND: Activate via gRPC API                     ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-monk-sotw"
	ctx := s.authCtx(playerID)

	// 1. Create monk and level up
	s.T().Log("Step 1: Creating level 2 monk...")
	characterID := s.createMonkCharacter(playerID)
	LevelUpMonkToLevel2(s.T(), s.server, ctx, characterID)

	// 2. Start combat
	s.T().Log("Step 2: Starting combat...")
	encounterID := CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
	s.T().Logf("  ✓ Encounter: %s", encounterID)

	// 3. Activate Step of the Wind
	s.T().Log("Step 3: Activating Step of the Wind...")
	activateResp, err := s.server.EncounterClient.ActivateFeature(ctx, &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: encounterID,
		CharacterId: characterID,
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_STEP_OF_THE_WIND,
	})
	s.Require().NoError(err, "failed to activate step of the wind")
	s.Require().True(activateResp.GetSuccess(), "step of the wind should succeed: %s", activateResp.GetMessage())
	s.T().Logf("  ✓ Step of the Wind activated: %s", activateResp.GetMessage())

	// 4. Verify DisengageActive in encounter state
	s.T().Log("Step 4: Verifying DisengageActive in encounter state...")
	stateResp, err := s.server.EncounterClient.GetEncounterState(ctx, &dnd5ev1alpha1.GetEncounterStateRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
	})
	s.Require().NoError(err, "failed to get encounter state")

	actionEconomy := stateResp.GetCombatState().GetCurrentTurn().GetActionEconomy()
	s.Require().NotNil(actionEconomy, "ActionEconomy must be present after Step of the Wind")
	// DisengageActive is now tracked as a condition on the character,
	// not as a flag on the proto ActionEconomy. Verify bonus action was consumed instead.
	s.Require().False(actionEconomy.GetBonusActionAvailable(), "bonus action should be consumed after Step of the Wind")
	s.T().Log("  ✓ Bonus action consumed confirmed in encounter state")
}

func (s *MonkIntegrationSuite) TestKiExhaustion_CannotActivateWhenDepleted() {
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  MONK KI: Cannot Activate Features When Ki Depleted               ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-monk-ki-exhaust"
	ctx := s.authCtx(playerID)

	// 1. Create monk and level up (2 Ki points at level 2)
	s.T().Log("Step 1: Creating level 2 monk (2 Ki points)...")
	characterID := s.createMonkCharacter(playerID)
	LevelUpMonkToLevel2(s.T(), s.server, ctx, characterID)

	// 2. Start combat
	s.T().Log("Step 2: Starting combat...")
	encounterID := CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
	s.T().Logf("  ✓ Encounter: %s", encounterID)

	// 3. Turn 1: Activate Flurry of Blows (1/2 Ki used, bonus action consumed)
	s.T().Log("Step 3: Turn 1 — Flurry of Blows (1 Ki)...")
	resp1, err := s.server.EncounterClient.ActivateFeature(ctx, &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: encounterID,
		CharacterId: characterID,
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_FLURRY_OF_BLOWS,
	})
	s.Require().NoError(err, "first activation should not error")
	s.Require().True(resp1.GetSuccess(), "first activation should succeed")
	s.T().Log("  ✓ First activation succeeded (1/2 Ki used)")

	// 4. End turn — cycles through monster turn(s), comes back to player with fresh action economy
	s.T().Log("Step 4: Ending turn (monster takes turn, action economy resets)...")
	_, err = s.server.EncounterClient.EndTurn(ctx, &dnd5ev1alpha1.EndTurnRequest{
		EncounterId: encounterID,
		EntityId:    characterID,
	})
	s.Require().NoError(err, "end turn should not error")
	s.T().Log("  ✓ Turn ended, new turn started")

	// 5. Turn 2: Activate Patient Defense (2/2 Ki used, bonus action consumed)
	s.T().Log("Step 5: Turn 2 — Patient Defense (1 Ki)...")
	resp2, err := s.server.EncounterClient.ActivateFeature(ctx, &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: encounterID,
		CharacterId: characterID,
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_PATIENT_DEFENSE,
	})
	s.Require().NoError(err, "second activation should not error")
	s.Require().True(resp2.GetSuccess(), "second activation should succeed (Ki 2/2 used)")
	s.T().Log("  ✓ Second activation succeeded (2/2 Ki used)")

	// 6. End turn again for fresh action economy
	s.T().Log("Step 6: Ending turn...")
	_, err = s.server.EncounterClient.EndTurn(ctx, &dnd5ev1alpha1.EndTurnRequest{
		EncounterId: encounterID,
		EntityId:    characterID,
	})
	s.Require().NoError(err, "end turn should not error")
	s.T().Log("  ✓ Turn ended")

	// 7. Turn 3: Try Step of the Wind — should fail (0 Ki remaining, bonus action is available)
	s.T().Log("Step 7: Turn 3 — Step of the Wind (should fail, no Ki)...")
	resp3, err := s.server.EncounterClient.ActivateFeature(ctx, &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: encounterID,
		CharacterId: characterID,
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_STEP_OF_THE_WIND,
	})

	// The API must signal failure: either a gRPC error OR success=false.
	// Both are acceptable — but silent success is not.
	if err != nil {
		s.T().Logf("  ✓ Third activation returned gRPC error (expected): %v", err)
	} else {
		s.Require().NotNil(resp3, "response must not be nil when err is nil")
		s.Require().False(resp3.GetSuccess(), "third activation must fail when Ki is depleted")
		s.T().Logf("  ✓ Third activation returned success=false: %s", resp3.GetMessage())
	}
}
