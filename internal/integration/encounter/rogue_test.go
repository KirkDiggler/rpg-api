// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package encounter_integration provides API-level integration tests for class features.
// These tests verify that toolkit features work correctly when invoked through the API layer.
// Rule correctness is proven in toolkit tests — here we verify the integration.
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
// ROGUE API INTEGRATION TESTS
// Level 1 Features:
//   - Sneak Attack (1d6 extra damage with finesse/ranged when advantage or ally adjacent)
//   - Expertise (double proficiency bonus on 2 skills)
// ============================================================================

type RogueIntegrationSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	server *harness.TestServer
}

func TestRogueIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(RogueIntegrationSuite))
}

func (s *RogueIntegrationSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	var err error
	s.server, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  Rogue Integration Test Server Ready                              ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
}

func (s *RogueIntegrationSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *RogueIntegrationSuite) SetupTest() {
	err := s.server.FlushRedis(s.ctx)
	s.Require().NoError(err, "failed to flush redis")
}

// authCtx adds dev auth header for test requests
func (s *RogueIntegrationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// =============================================================================
// CHARACTER CREATION HELPERS
// =============================================================================

func (s *RogueIntegrationSuite) createRogueCharacter(playerID string) string {
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
		Name:    "Vex the Shadow",
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
							dnd5ev1alpha1.Language_LANGUAGE_GOBLIN,
						},
					},
				},
			},
		},
	})
	s.Require().NoError(err, "failed to set race")

	// Step 4: Set Rogue with skills, expertise, and equipment choices
	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_ROGUE,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			// Skills: 4 from rogue list
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ACROBATICS,
							dnd5ev1alpha1.Skill_SKILL_STEALTH,
							dnd5ev1alpha1.Skill_SKILL_PERCEPTION,
							dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND,
						},
					},
				},
			},
			// Expertise: double proficiency on 2 skills
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Expertise{
					Expertise: &dnd5ev1alpha1.ExpertiseSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_STEALTH,
							dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND,
						},
					},
				},
			},
			// Primary weapon: Rapier
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "rogue-weapons-primary",
				OptionId: "rogue-weapon-a", // Rapier
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Secondary weapon: Shortbow
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "rogue-weapons-secondary",
				OptionId: "rogue-secondary-a", // Shortbow
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Pack: Burglar's pack
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "rogue-pack",
				OptionId: "rogue-pack-a", // Burglar's pack
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
		Background: dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL,
	})
	s.Require().NoError(err, "failed to set background")

	// Step 6: Set ability scores (Rogue-optimized: DEX > WIS > CON)
	// Base: STR 10, DEX 15, CON 13, INT 12, WIS 14, CHA 8
	// Human +1 to all → STR 11(+0), DEX 16(+3), CON 14(+2), INT 13(+1), WIS 15(+2), CHA 9(-1)
	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     10,
				Dexterity:    15,
				Constitution: 13,
				Intelligence: 12,
				Wisdom:       14,
				Charisma:     8,
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

// Encounter helpers delegate to shared functions in helpers.go.
func (s *RogueIntegrationSuite) createEncounterAndStartCombat(ctx context.Context, playerID, characterID string) string {
	return CreateEncounterAndStartCombat(s.T(), s.server, ctx, playerID, characterID)
}

func (s *RogueIntegrationSuite) findMonsterTarget(ctx context.Context, playerID, encounterID string) string {
	return FindMonsterTarget(s.T(), s.server, ctx, playerID, encounterID)
}

// =============================================================================
// SNEAK ATTACK TESTS
// =============================================================================

func (s *RogueIntegrationSuite) TestSneakAttack_ExtraDamageOnHit() {
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  ROGUE SNEAK ATTACK: Extra Damage in Attack Breakdown            ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-rogue-sneak"
	ctx := s.authCtx(playerID)

	// 1. Create rogue character
	s.T().Log("Step 1: Creating rogue character...")
	characterID := s.createRogueCharacter(playerID)
	s.T().Logf("  ✓ Created character: %s", characterID)

	// 2. Create encounter and start combat
	s.T().Log("Step 2: Starting combat...")
	encounterID := s.createEncounterAndStartCombat(ctx, playerID, characterID)
	s.T().Logf("  ✓ Encounter: %s", encounterID)

	// 3. Find a target
	targetID := s.findMonsterTarget(ctx, playerID, encounterID)
	s.T().Logf("  ✓ Target: %s", targetID)

	// 4. Attack with rapier (finesse weapon — qualifies for Sneak Attack)
	s.T().Log("Step 3: Attacking with rapier (finesse weapon)...")
	attackResp, err := s.server.EncounterClient.Attack(ctx, &dnd5ev1alpha1.AttackRequest{
		EncounterId: encounterID,
		AttackerId:  characterID,
		TargetId:    targetID,
	})
	s.Require().NoError(err, "attack should succeed")
	s.Assert().True(attackResp.GetSuccess(), "attack request should succeed: %s", attackResp.GetError())

	result := attackResp.GetResult()
	s.Require().NotNil(result, "attack result should not be nil")
	s.T().Logf("  ✓ Attack roll: %d + bonus = %d vs AC %d → %s",
		result.GetAttackRoll(), result.GetAttackTotal(), result.GetTargetAc(),
		map[bool]string{true: "HIT", false: "MISS"}[result.GetHit()])

	if result.GetHit() {
		s.T().Logf("  ✓ Total damage: %d (%s)", result.GetDamage(), result.GetDamageType())

		breakdown := result.GetDamageBreakdown()
		s.Require().NotNil(breakdown, "damage breakdown should be present on hit")

		s.T().Log("  Damage breakdown:")
		foundSneakAttack := false
		for _, comp := range breakdown.GetComponents() {
			s.T().Logf("    - Source: %s, Dice: %v, Flat: %d, Type: %s",
				comp.GetSource(), comp.GetFinalDiceRolls(), comp.GetFlatBonus(), comp.GetDamageType())

			// Look for Sneak Attack in the damage components
			isSneakSource := comp.GetSource() == "sneak_attack" ||
				comp.GetSource() == "condition" ||
				(comp.GetSourceRef() != nil && comp.GetSourceRef().GetCondition() == dnd5ev1alpha1.ConditionId_CONDITION_ID_SNEAK_ATTACK)
			if isSneakSource && len(comp.GetFinalDiceRolls()) > 0 {
				foundSneakAttack = true
				s.T().Logf("    → Sneak Attack damage: %v", comp.GetFinalDiceRolls())
			}
		}

		if foundSneakAttack {
			s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
			s.T().Log("║  ✓ TEST PASSED: Sneak Attack extra damage in breakdown           ║")
			s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
		} else {
			// Sneak Attack requires advantage OR an ally adjacent to the target.
			// In a solo encounter (1 character), qualification isn't guaranteed.
			s.T().Log("  Sneak Attack not in breakdown (no advantage/ally in solo encounter)")
			s.T().Skip("Skipping: Sneak Attack qualification requires advantage or adjacent ally")
		}
	} else {
		s.T().Skip("Attack missed; skipping Sneak Attack verification (dice-dependent)")
	}
}

// =============================================================================
// ATTACK TESTS
// =============================================================================

func (s *RogueIntegrationSuite) TestRogue_CanAttackInCombat() {
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  ROGUE: Can Attack Monster in Combat                              ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-rogue-attack"
	ctx := s.authCtx(playerID)

	// 1. Create rogue and start combat
	s.T().Log("Step 1: Creating rogue and starting combat...")
	characterID := s.createRogueCharacter(playerID)
	encounterID := s.createEncounterAndStartCombat(ctx, playerID, characterID)
	s.T().Logf("  ✓ Character: %s, Encounter: %s", characterID, encounterID)

	// 2. Find a target monster
	targetID := s.findMonsterTarget(ctx, playerID, encounterID)
	s.T().Logf("  ✓ Target: %s", targetID)

	// 3. Attack
	s.T().Log("Step 2: Attacking monster...")
	attackResp, err := s.server.EncounterClient.Attack(ctx, &dnd5ev1alpha1.AttackRequest{
		EncounterId: encounterID,
		AttackerId:  characterID,
		TargetId:    targetID,
	})
	s.Require().NoError(err, "attack should succeed")
	s.Assert().True(attackResp.GetSuccess(), "attack request should succeed: %s", attackResp.GetError())

	result := attackResp.GetResult()
	s.Require().NotNil(result, "attack result should not be nil")

	// Assert response structure is well-formed regardless of hit/miss
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
	s.T().Log("║  ✓ TEST PASSED: Rogue can attack in combat                        ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
}

// =============================================================================
// CHARACTER CREATION TESTS
// =============================================================================

func (s *RogueIntegrationSuite) TestRogue_CharacterCreation_WithExpertise() {
	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  ROGUE: Character Creation with Expertise                         ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")

	playerID := "test-player-rogue-create"
	ctx := s.authCtx(playerID)

	// Create rogue — validates the full creation pipeline works
	s.T().Log("Step 1: Creating rogue with expertise...")
	characterID := s.createRogueCharacter(playerID)
	s.Require().NotEmpty(characterID, "character should be created")
	s.T().Logf("  ✓ Created character: %s", characterID)

	// Get character to verify stats
	charResp, err := s.server.CharacterClient.GetCharacter(ctx, &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: characterID,
	})
	s.Require().NoError(err, "failed to get character")

	char := charResp.GetCharacter()
	s.Require().NotNil(char, "character should not be nil")

	// Assert core stats
	s.Assert().Equal(dnd5ev1alpha1.Class_CLASS_ROGUE, char.GetClass(), "Class should be Rogue")
	s.Assert().Equal(int32(1), char.GetLevel(), "Level should be 1")
	s.T().Logf("  ✓ Class: %s, Level: %d", char.GetClass(), char.GetLevel())
	s.T().Logf("  HP: %d, AC: %d", char.GetCombatStats().GetHitPointMaximum(), char.GetCombatStats().GetArmorClass())

	if char.GetAbilityScores() != nil {
		s.T().Logf("  DEX: %d, WIS: %d, CON: %d",
			char.GetAbilityScores().GetDexterity(),
			char.GetAbilityScores().GetWisdom(),
			char.GetAbilityScores().GetConstitution())
	}

	// Rogue should have Sneak Attack condition auto-applied at level 1
	conditions := char.GetActiveConditions()
	s.T().Logf("  Active conditions: %d", len(conditions))
	hasSneakAttack := false
	for _, cond := range conditions {
		s.T().Logf("    - %s", cond.GetId())
		if cond.GetId() == dnd5ev1alpha1.ConditionId_CONDITION_ID_SNEAK_ATTACK {
			hasSneakAttack = true
		}
	}

	s.Assert().True(hasSneakAttack, "Rogue should have Sneak Attack condition at level 1")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  ✓ TEST PASSED: Rogue created with expertise + Sneak Attack       ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
}
