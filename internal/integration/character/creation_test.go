// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package character_integration provides integration tests for character creation.
// These tests verify the full character creation flow through the gRPC API.
package character_integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
)

// CharacterCreationSuite tests character creation for all classes.
type CharacterCreationSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	server *harness.TestServer
}

func TestCharacterCreationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(CharacterCreationSuite))
}

func (s *CharacterCreationSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 3*time.Minute)

	var err error
	s.server, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  Character Creation Integration Tests                            ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
}

func (s *CharacterCreationSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *CharacterCreationSuite) SetupTest() {
	err := s.server.FlushRedis(s.ctx)
	s.Require().NoError(err, "failed to flush redis")
}

func (s *CharacterCreationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// =============================================================================
// FIGHTER - Equipment + Fighting Style
// =============================================================================

func (s *CharacterCreationSuite) TestCreateFighter() {
	s.T().Log("Creating Fighter character...")
	ctx := s.authCtx("test-player-fighter")

	// Create draft
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()

	// Set name
	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    "Sir Roland",
	})
	s.Require().NoError(err)

	// Set Human with language
	_, err = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				Selection: &dnd5ev1alpha1.ChoiceData_Languages{
					Languages: &dnd5ev1alpha1.LanguageSelection{
						Languages: []dnd5ev1alpha1.Language{dnd5ev1alpha1.Language_LANGUAGE_DWARVISH},
					},
				},
			},
		},
	})
	s.Require().NoError(err)

	// Set Fighter with skills, fighting style, and equipment
	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_FIGHTER,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			// Skills (choose 2)
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
							dnd5ev1alpha1.Skill_SKILL_PERCEPTION,
						},
					},
				},
			},
			// Fighting Style
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
					FightingStyle: &dnd5ev1alpha1.FightingStyleSelection{
						Style: dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DEFENSE,
					},
				},
			},
			// Armor: Chain mail
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-armor",
				OptionId: "fighter-armor-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Primary weapons: Martial + Shield (need to specify which martial weapon)
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-weapons-primary",
				OptionId: "fighter-weapon-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{
						Items: []*dnd5ev1alpha1.EquipmentSelectionItem{
							{Equipment: &dnd5ev1alpha1.EquipmentSelectionItem_Weapon{Weapon: dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD}},
						},
					},
				},
			},
			// Secondary: Light crossbow
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-weapons-secondary",
				OptionId: "fighter-ranged-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Pack: Dungeoneer's
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-pack",
				OptionId: "fighter-pack-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
		},
	})
	s.Require().NoError(err)

	// Set background
	_, err = s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_SOLDIER,
	})
	s.Require().NoError(err)

	// Set ability scores (STR-focused)
	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength: 15, Dexterity: 13, Constitution: 14,
				Intelligence: 10, Wisdom: 12, Charisma: 8,
			},
		},
	})
	s.Require().NoError(err)

	// Check validation before finalize
	getDraftResp, err := s.server.CharacterClient.GetDraft(ctx, &dnd5ev1alpha1.GetDraftRequest{DraftId: draftID})
	s.Require().NoError(err)
	
	if issues := getDraftResp.GetDraft().GetValidation().GetIssues(); len(issues) > 0 {
		s.T().Log("Validation issues:")
		for _, issue := range issues {
			s.T().Logf("  • %s: %s", issue.GetField(), issue.GetMessage())
		}
		s.Fail("Fighter has validation issues")
		return
	}

	// Finalize
	finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err)
	s.Require().NotNil(finalizeResp.GetCharacter())

	char := finalizeResp.GetCharacter()
	s.T().Logf("✅ Fighter created: %s", char.GetId())
	s.Assert().Equal("Sir Roland", char.GetName())
	s.Assert().Equal(dnd5ev1alpha1.Class_CLASS_FIGHTER, char.GetClass())
}

// =============================================================================
// ROGUE - Skills (4) + Expertise
// =============================================================================

func (s *CharacterCreationSuite) TestCreateRogue() {
	s.T().Log("Creating Rogue character...")
	ctx := s.authCtx("test-player-rogue")

	// Create draft
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()

	// Set name
	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    "Shadow",
	})
	s.Require().NoError(err)

	// Set Human with language choice
	_, err = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				Selection: &dnd5ev1alpha1.ChoiceData_Languages{
					Languages: &dnd5ev1alpha1.LanguageSelection{
						Languages: []dnd5ev1alpha1.Language{dnd5ev1alpha1.Language_LANGUAGE_ELVISH},
					},
				},
			},
		},
	})
	s.Require().NoError(err)

	// Set Rogue with skills (4) and expertise (2)
	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_ROGUE,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			// Skills (choose 4 from rogue list)
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
			// Expertise (choose 2 from proficient skills)
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "rogue-expertise-1",
				Selection: &dnd5ev1alpha1.ChoiceData_Expertise{
					Expertise: &dnd5ev1alpha1.ExpertiseSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_STEALTH,
							dnd5ev1alpha1.Skill_SKILL_PERCEPTION,
						},
					},
				},
			},
			// Primary weapon: Rapier
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "rogue-weapons-primary",
				OptionId: "rogue-weapon-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Secondary: Shortbow
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "rogue-weapons-secondary",
				OptionId: "rogue-secondary-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Pack: Burglar's
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "rogue-pack",
				OptionId: "rogue-pack-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
		},
	})
	s.Require().NoError(err)

	// Set background
	_, err = s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL,
	})
	s.Require().NoError(err)

	// Set ability scores (DEX-focused)
	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength: 8, Dexterity: 15, Constitution: 14,
				Intelligence: 12, Wisdom: 13, Charisma: 10,
			},
		},
	})
	s.Require().NoError(err)

	// Check validation
	getDraftResp, err := s.server.CharacterClient.GetDraft(ctx, &dnd5ev1alpha1.GetDraftRequest{DraftId: draftID})
	s.Require().NoError(err)
	
	if issues := getDraftResp.GetDraft().GetValidation().GetIssues(); len(issues) > 0 {
		s.T().Log("Validation issues:")
		for _, issue := range issues {
			s.T().Logf("  • %s: %s", issue.GetField(), issue.GetMessage())
		}
		s.Fail("Rogue has validation issues")
		return
	}

	// Finalize
	finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err)
	s.Require().NotNil(finalizeResp.GetCharacter())

	char := finalizeResp.GetCharacter()
	s.T().Logf("✅ Rogue created: %s", char.GetId())
	s.Assert().Equal("Shadow", char.GetName())
	s.Assert().Equal(dnd5ev1alpha1.Class_CLASS_ROGUE, char.GetClass())
}

// =============================================================================
// BARBARIAN - Already tested, include for completeness
// =============================================================================

func (s *CharacterCreationSuite) TestCreateBarbarian() {
	s.T().Log("Creating Barbarian character...")
	ctx := s.authCtx("test-player-barbarian")

	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()

	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID, Name: "Grog",
	})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
				Source:    dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				Selection: &dnd5ev1alpha1.ChoiceData_Languages{Languages: &dnd5ev1alpha1.LanguageSelection{Languages: []dnd5ev1alpha1.Language{dnd5ev1alpha1.Language_LANGUAGE_ORC}}},
			},
		},
	})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_BARBARIAN,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS, Source: dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, Selection: &dnd5ev1alpha1.ChoiceData_Skills{Skills: &dnd5ev1alpha1.SkillSelection{Skills: []dnd5ev1alpha1.Skill{dnd5ev1alpha1.Skill_SKILL_ATHLETICS, dnd5ev1alpha1.Skill_SKILL_INTIMIDATION}}}},
			{Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT, Source: dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, ChoiceId: "barbarian-weapons-primary", OptionId: "barbarian-weapon-a", Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{}}},
			{Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT, Source: dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, ChoiceId: "barbarian-weapons-secondary", OptionId: "barbarian-secondary-a", Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{}}},
			{Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT, Source: dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, ChoiceId: "barbarian-pack", OptionId: "barbarian-pack-a", Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{}}},
		},
	})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{DraftId: draftID, Background: dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId:     draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{AbilityScores: &dnd5ev1alpha1.AbilityScores{Strength: 15, Dexterity: 13, Constitution: 14, Intelligence: 8, Wisdom: 12, Charisma: 10}},
	})
	s.Require().NoError(err)

	finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{DraftId: draftID})
	s.Require().NoError(err)
	
	s.T().Logf("✅ Barbarian created: %s", finalizeResp.GetCharacter().GetId())
}

// =============================================================================
// VALIDATION TESTS - Find gaps in character creation
// =============================================================================

func (s *CharacterCreationSuite) TestValidation_MissingName() {
	ctx := s.authCtx("test-validation-name")

	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()

	// Set everything except name
	_, _ = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{DraftId: draftID, Race: dnd5ev1alpha1.Race_RACE_HUMAN})
	_, _ = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{DraftId: draftID, Class: dnd5ev1alpha1.Class_CLASS_FIGHTER})
	
	// Try to finalize without name
	_, err = s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{DraftId: draftID})
	s.Assert().Error(err, "should fail without name")
	s.T().Logf("✅ Correctly rejected: %v", err)
}

func (s *CharacterCreationSuite) TestValidation_MissingRace() {
	ctx := s.authCtx("test-validation-race")

	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()

	_, _ = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{DraftId: draftID, Name: "Test"})
	_, _ = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{DraftId: draftID, Class: dnd5ev1alpha1.Class_CLASS_FIGHTER})
	
	_, err = s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{DraftId: draftID})
	s.Assert().Error(err, "should fail without race")
	s.T().Logf("✅ Correctly rejected: %v", err)
}

func (s *CharacterCreationSuite) TestValidation_MissingClass() {
	ctx := s.authCtx("test-validation-class")

	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()

	_, _ = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{DraftId: draftID, Name: "Test"})
	_, _ = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{DraftId: draftID, Race: dnd5ev1alpha1.Race_RACE_HUMAN})
	
	_, err = s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{DraftId: draftID})
	s.Assert().Error(err, "should fail without class")
	s.T().Logf("✅ Correctly rejected: %v", err)
}
