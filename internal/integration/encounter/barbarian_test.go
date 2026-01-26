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

	// Step 2: Set name
	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    "Grog the Mighty",
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
							dnd5ev1alpha1.Language_LANGUAGE_ORC,
						},
					},
				},
			},
		},
	})
	s.Require().NoError(err, "failed to set race")

	// Step 4: Set Barbarian with skills AND equipment choices
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
			// Primary weapon: Greataxe
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-weapons-primary",
				OptionId: "barbarian-weapon-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Secondary weapon: 2 handaxes
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-weapons-secondary",
				OptionId: "barbarian-secondary-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Pack: Explorer's pack
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-pack",
				OptionId: "barbarian-pack-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
		},
	})
	s.Require().NoError(err, "failed to set class")

	// Step 5: Set background
	_, err = s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER,
	})
	s.Require().NoError(err, "failed to set background")

	// Step 6: Set ability scores
	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     15,
				Dexterity:    13,
				Constitution: 14,
				Intelligence: 8,
				Wisdom:       12,
				Charisma:     10,
			},
		},
	})
	s.Require().NoError(err, "failed to set ability scores")

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
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  BARBARIAN RAGE: +2 Damage Bonus in Attack                       ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-barbarian-rage-dmg"
	ctx := s.authCtx(playerID)

	// 1. Create a barbarian character
	s.T().Log("Step 1: Creating barbarian character...")
	characterID := s.createBarbarianCharacter(playerID)
	s.T().Logf("  ✓ Created character: %s", characterID)

	// 2. Create an encounter
	s.T().Log("Step 2: Creating encounter...")
	createResp, err := s.server.EncounterClient.CreateEncounter(ctx, &dnd5ev1alpha1.CreateEncounterRequest{
		CharacterIds: []string{characterID},
	})
	s.Require().NoError(err, "failed to create encounter")
	encounterID := createResp.GetEncounterId()
	s.T().Logf("  ✓ Created encounter: %s", encounterID)

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

	// Find a monster to attack
	var targetID string
	for _, entity := range startResp.GetRoom().GetEntities() {
		if entity.GetEntityType() == dnd5ev1alpha1.EntityType_ENTITY_TYPE_MONSTER {
			targetID = entity.GetEntityId()
			s.T().Logf("  ✓ Found target monster: %s", targetID)
			break
		}
	}
	s.Require().NotEmpty(targetID, "should have a monster to attack")

	// 4. Activate Rage first
	s.T().Log("Step 4: Activating Rage...")
	activateResp, err := s.server.EncounterClient.ActivateFeature(ctx, &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: encounterID,
		CharacterId: characterID,
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE,
	})
	s.Require().NoError(err, "failed to activate rage")
	s.Assert().True(activateResp.GetSuccess(), "rage activation should succeed")
	s.T().Log("  ✓ Rage activated")

	// 5. Attack the monster while raging
	s.T().Log("Step 5: Attacking while raging...")
	attackResp, err := s.server.EncounterClient.Attack(ctx, &dnd5ev1alpha1.AttackRequest{
		EncounterId: encounterID,
		AttackerId:  characterID,
		TargetId:    targetID,
	})
	s.Require().NoError(err, "attack should succeed")
	s.Require().True(attackResp.GetSuccess(), "attack should succeed: %s", attackResp.GetError())

	result := attackResp.GetResult()
	s.Require().NotNil(result, "attack result should not be nil")
	s.T().Logf("  ✓ Attack roll: %d + bonus = %d vs AC %d", result.GetAttackRoll(), result.GetAttackTotal(), result.GetTargetAc())

	// 6. Verify damage breakdown includes Rage bonus
	if result.GetHit() {
		s.T().Logf("  ✓ Hit! Damage: %d (%s)", result.GetDamage(), result.GetDamageType())

		breakdown := result.GetDamageBreakdown()
		s.Require().NotNil(breakdown, "damage breakdown should be present on hit")
		s.T().Logf("  ✓ Breakdown total: %d", breakdown.GetTotalDamage())

		// Look for Rage component in breakdown
		var foundRageBonus bool
		for _, comp := range breakdown.GetComponents() {
			s.T().Logf("    - Component: source=%s, flat=%d, dice=%v, type=%s",
				comp.GetSource(), comp.GetFlatBonus(), comp.GetFinalDiceRolls(), comp.GetDamageType())

			// Rage adds +2 flat bonus at level 1
			// The Rage bonus appears as source="condition" with flat=2
			// Also check SourceRef.Condition for the typed reference
			isRageSource := comp.GetSource() == "rage" ||
				comp.GetSource() == "condition" ||
				(comp.GetSourceRef() != nil && comp.GetSourceRef().GetCondition() == dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING)
			if isRageSource && comp.GetFlatBonus() == 2 {
				foundRageBonus = true
			}
		}
		s.Assert().True(foundRageBonus, "damage breakdown should include Rage bonus component")

		s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
		s.T().Log("║  ✓ TEST PASSED: Rage +2 damage appears in attack breakdown       ║")
		s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
	} else {
		s.T().Log("  ⚠ Attack missed - cannot verify damage breakdown")
		s.T().Log("  Note: This test may need to be run multiple times or use a fixed roller")
		s.T().Skip("Attack missed - skipping damage verification")
	}
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
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  BARBARIAN RAGE: Ends on Turn End Without Combat                 ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-barbarian-rage-end"
	ctx := s.authCtx(playerID)

	// 1. Create a barbarian character
	s.T().Log("Step 1: Creating barbarian character...")
	characterID := s.createBarbarianCharacter(playerID)
	s.T().Logf("  ✓ Created character: %s", characterID)

	// 2. Create an encounter
	s.T().Log("Step 2: Creating encounter...")
	createResp, err := s.server.EncounterClient.CreateEncounter(ctx, &dnd5ev1alpha1.CreateEncounterRequest{
		CharacterIds: []string{characterID},
	})
	s.Require().NoError(err, "failed to create encounter")
	encounterID := createResp.GetEncounterId()
	s.T().Logf("  ✓ Created encounter: %s", encounterID)

	// 3. Set ready and start combat
	s.T().Log("Step 3: Starting combat...")
	_, err = s.server.EncounterClient.SetReady(ctx, &dnd5ev1alpha1.SetReadyRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
		IsReady:     true,
	})
	s.Require().NoError(err, "failed to set ready")

	_, err = s.server.EncounterClient.StartCombat(ctx, &dnd5ev1alpha1.StartCombatRequest{
		EncounterId: encounterID,
		Theme:       dnd5ev1alpha1.DungeonTheme_DUNGEON_THEME_CRYPT,
		Difficulty:  dnd5ev1alpha1.DungeonDifficulty_DUNGEON_DIFFICULTY_MEDIUM,
		Length:      dnd5ev1alpha1.DungeonLength_DUNGEON_LENGTH_SHORT,
	})
	s.Require().NoError(err, "failed to start combat")
	s.T().Log("  ✓ Combat started")

	// 4. Activate Rage
	s.T().Log("Step 4: Activating Rage...")
	activateResp, err := s.server.EncounterClient.ActivateFeature(ctx, &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: encounterID,
		CharacterId: characterID,
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE,
	})
	s.Require().NoError(err, "failed to activate rage")
	s.Assert().True(activateResp.GetSuccess(), "rage activation should succeed")

	// Verify Rage is active
	hasRageBefore := false
	if activateResp.GetUpdatedCharacter() != nil {
		for _, cond := range activateResp.GetUpdatedCharacter().GetActiveConditions() {
			if cond.GetId() == dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING {
				hasRageBefore = true
				break
			}
		}
	}
	s.Assert().True(hasRageBefore, "character should have Rage condition after activation")
	s.T().Log("  ✓ Rage activated and verified")

	// 5. End turn WITHOUT attacking or taking damage
	s.T().Log("Step 5: Ending turn without combat activity...")
	endTurnResp, err := s.server.EncounterClient.EndTurn(ctx, &dnd5ev1alpha1.EndTurnRequest{
		EncounterId: encounterID,
		EntityId:    characterID,
	})
	s.Require().NoError(err, "failed to end turn")
	s.T().Logf("  ✓ Turn ended, round: %d", endTurnResp.GetCombatState().GetRound())

	// 6. Get updated character state and verify Rage is removed
	stateResp, err := s.server.EncounterClient.GetEncounterState(ctx, &dnd5ev1alpha1.GetEncounterStateRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
	})
	s.Require().NoError(err, "failed to get encounter state")

	// Find our character in the party
	var charConditions []*dnd5ev1alpha1.Condition
	for _, member := range stateResp.GetParty() {
		if member.GetCharacter() != nil && member.GetCharacter().GetId() == characterID {
			charConditions = member.GetCharacter().GetActiveConditions()
			break
		}
	}

	// Verify Rage is no longer active
	hasRageAfter := false
	for _, cond := range charConditions {
		if cond.GetId() == dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING {
			hasRageAfter = true
			break
		}
	}

	s.Assert().False(hasRageAfter, "Rage should end when turn ends without attacking/taking damage")

	if !hasRageAfter {
		s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
		s.T().Log("║  ✓ TEST PASSED: Rage ends on turn end without combat activity    ║")
		s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
	} else {
		s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
		s.T().Log("║  ✗ TEST FAILED: Rage did not end (may need turn-end event wiring)║")
		s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
	}
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
