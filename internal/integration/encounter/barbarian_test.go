// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package encounter_integration provides API-level integration tests for class features.
// These tests verify that toolkit features work correctly when invoked through the API layer.
// Rule correctness is proven in toolkit tests - here we verify the integration.
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
// BARBARIAN API INTEGRATION TESTS
// Level 1 Features:
//   - Rage (bonus action, +2 damage, B/P/S resistance)
//   - Unarmored Defense (AC = 10 + DEX + CON)
// ============================================================================

type BarbarianIntegrationSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	server *harness.TestServer
}

func TestBarbarianIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(BarbarianIntegrationSuite))
}

func (s *BarbarianIntegrationSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	var err error
	s.server, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  Integration Test Server Ready                                   ║")
	s.T().Log("║  • Real Redis via testcontainers                                 ║")
	s.T().Log("║  • In-process gRPC via bufconn                                   ║")
	s.T().Log("║  • Proto-generated clients                                       ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
}

func (s *BarbarianIntegrationSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *BarbarianIntegrationSuite) SetupTest() {
	// Flush Redis between tests for isolation
	err := s.server.FlushRedis(s.ctx)
	s.Require().NoError(err, "failed to flush redis")
}

// authCtx adds dev auth header for test requests
func (s *BarbarianIntegrationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// =============================================================================
// CHARACTER CREATION HELPERS
// =============================================================================

func (s *BarbarianIntegrationSuite) createBarbarianCharacter(playerID string) string {
	ctx := s.authCtx(playerID)

	// Step 1: Create character draft
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err, "failed to create draft")
	s.Require().NotNil(createResp.GetDraft(), "draft should not be nil")
	s.Require().NotEmpty(createResp.GetDraft().GetId(), "draft ID should not be empty")

	draftID := createResp.GetDraft().GetId()

	// Step 2: Set race (Human - no subrace needed)
	_, err = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
	})
	s.Require().NoError(err, "failed to set race")

	// Step 3: Set class (Barbarian) with required skill choices
	// Barbarians choose 2 skills from: Animal Handling, Athletics, Intimidation, Nature, Perception, Survival
	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_BARBARIAN,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
							dnd5ev1alpha1.Skill_SKILL_INTIMIDATION,
						},
					},
				},
			},
		},
	})
	s.Require().NoError(err, "failed to set class")

	// Step 4: Set ability scores directly (standard array: 15, 14, 13, 12, 10, 8)
	// Barbarian optimal: STR 15, CON 14, DEX 13, WIS 12, CHA 10, INT 8
	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     15,
				Constitution: 14,
				Dexterity:    13,
				Wisdom:       12,
				Charisma:     10,
				Intelligence: 8,
			},
		},
	})
	s.Require().NoError(err, "failed to set ability scores")

	// Step 6: Set character name
	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    "Grog the Mighty",
	})
	s.Require().NoError(err, "failed to set name")

	// Step 7: Finalize character
	finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err, "failed to finalize draft")
	s.Require().NotNil(finalizeResp.GetCharacter(), "character should not be nil")
	s.Require().NotEmpty(finalizeResp.GetCharacter().GetId(), "character ID should not be empty")

	return finalizeResp.GetCharacter().GetId()
}

// =============================================================================
// RAGE TESTS
// =============================================================================

func (s *BarbarianIntegrationSuite) TestRage_ActivateFeature_Success() {
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  BARBARIAN RAGE: Activate via gRPC API                           ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-barbarian-1"

	// 1. Create a barbarian character
	s.T().Log("Step 1: Creating barbarian character...")
	characterID := s.createBarbarianCharacter(playerID)
	s.T().Logf("  ✓ Created character: %s", characterID)

	// 2. Create an encounter
	s.T().Log("Step 2: Creating encounter...")
	ctx := s.authCtx(playerID)
	createResp, err := s.server.EncounterClient.CreateEncounter(ctx, &dnd5ev1alpha1.CreateEncounterRequest{
		CharacterIds: []string{characterID},
	})
	s.Require().NoError(err, "failed to create encounter")
	s.Require().NotEmpty(createResp.GetEncounterId(), "encounter ID should not be empty")
	s.T().Logf("  ✓ Created encounter: %s", createResp.GetEncounterId())
	s.T().Logf("  ✓ Join code: %s", createResp.GetJoinCode())

	encounterID := createResp.GetEncounterId()

	// 3. Set ready and start combat
	s.T().Log("Step 3: Starting combat...")
	_, err = s.server.EncounterClient.SetReady(ctx, &dnd5ev1alpha1.SetReadyRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
		IsReady:     true,
	})
	s.Require().NoError(err, "failed to set ready")

	startResp, err := s.server.EncounterClient.StartCombat(ctx, &dnd5ev1alpha1.StartCombatRequest{
		EncounterId: encounterID,
		Theme:       dnd5ev1alpha1.DungeonTheme_DUNGEON_THEME_CRYPT,
		Difficulty:  dnd5ev1alpha1.DungeonDifficulty_DUNGEON_DIFFICULTY_MEDIUM,
		Length:      dnd5ev1alpha1.DungeonLength_DUNGEON_LENGTH_SHORT,
	})
	s.Require().NoError(err, "failed to start combat")
	s.T().Logf("  ✓ Combat started, round: %d", startResp.GetCombatState().GetRound())

	// 4. Activate Rage
	s.T().Log("Step 4: Activating Rage...")
	activateResp, err := s.server.EncounterClient.ActivateFeature(ctx, &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: encounterID,
		CharacterId: characterID,
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE,
	})
	s.Require().NoError(err, "failed to activate rage")
	s.Assert().True(activateResp.GetSuccess(), "rage activation should succeed")
	s.T().Logf("  ✓ Rage activated: %s", activateResp.GetMessage())

	// 5. Verify character has Rage condition
	if activateResp.GetUpdatedCharacter() != nil {
		conditions := activateResp.GetUpdatedCharacter().GetActiveConditions()
		s.T().Logf("  ✓ Active conditions: %v", conditions)

		// Check for Rage condition (RAGING is the condition ID)
		hasRage := false
		for _, cond := range conditions {
			if cond.GetId() == dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING {
				hasRage = true
				break
			}
		}
		s.Assert().True(hasRage, "character should have Rage condition after activation")
	}

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  ✓ TEST PASSED: Barbarian Rage activates via gRPC                ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
}

func (s *BarbarianIntegrationSuite) TestRage_DamageBonus_AppearsInAttackResult() {
	s.T().Skip("TODO: Requires attack flow implementation")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  BARBARIAN RAGE: +2 Damage Bonus in Attack                       ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	// TODO:
	// 1. Create character with Rage ACTIVE (condition applied)
	// 2. Create encounter with monster target
	// 3. Call EncounterClient.Attack(ctx, &AttackRequest{...})
	// 4. Verify attack result breakdown includes Rage +2 damage
}

func (s *BarbarianIntegrationSuite) TestRage_Resistance_AppliedWhenTakingDamage() {
	s.T().Skip("TODO: Need monster attack flow - may require separate work")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  BARBARIAN RAGE: Resistance Halves B/P/S Damage                  ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	// TODO:
	// 1. Create character with Rage ACTIVE
	// 2. Simulate monster attacking character
	// 3. Verify damage is halved for B/P/S types
}

func (s *BarbarianIntegrationSuite) TestRage_EndsOnTurnEnd_NoCombatActivity() {
	s.T().Skip("TODO: Wire up end turn flow")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  BARBARIAN RAGE: Ends on Turn End Without Combat                 ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	// TODO:
	// 1. Create character with Rage ACTIVE
	// 2. Call EncounterClient.EndTurn without attacking
	// 3. Verify Rage condition is removed
}

// =============================================================================
// UNARMORED DEFENSE TESTS
// =============================================================================

func (s *BarbarianIntegrationSuite) TestBarbarianUnarmoredDefense_ACCalculation() {
	s.T().Skip("TODO: Need AC calculation API endpoint or verify through combat")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  BARBARIAN UNARMORED DEFENSE: AC = 10 + DEX + CON                ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	// TODO:
	// 1. Create unarmored barbarian (DEX 14, CON 16)
	// 2. Verify AC = 10 + 2 + 3 = 15
	// Note: May need to verify through monster attack targeting the barbarian
}
