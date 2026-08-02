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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/armor"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/tools"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

// CharacterCreationSuite tests character creation for all classes.
type CharacterCreationSuite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	server  *harness.TestServer
	release func()
}

func TestCharacterCreationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(CharacterCreationSuite))
}

func (s *CharacterCreationSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 3*time.Minute)

	// Lease this package's shared Redis container (rpg-api#699) — see
	// main_test.go. Released in TearDownTest. Each test still gets its
	// own fresh TestServer (gRPC server, repos, brokers, clients); only
	// the Redis container/connection is shared across tests.
	s.release = sharedRedis.Lease()

	var err error
	s.server, err = harness.NewWithRedis(s.ctx, nil, sharedRedis.Addr)
	s.Require().NoError(err, "failed to create test server")
	s.Require().NoError(s.server.FlushRedis(s.ctx), "failed to flush redis")
}

func (s *CharacterCreationSuite) TearDownTest() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.release != nil {
		s.release()
	}
}

func (s *CharacterCreationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

func (s *CharacterCreationSuite) assertInventoryCounts(char *dnd5ev1alpha1.Character, expected map[string]int32) {
	s.Require().NotNil(char)

	counts := make(map[string]int32, len(char.GetInventory()))
	for _, item := range char.GetInventory() {
		counts[item.GetItemId()] += item.GetQuantity()
	}

	for itemID, quantity := range expected {
		s.Require().Contains(counts, itemID, "expected %s in inventory", itemID)
		s.Equal(quantity, counts[itemID], "expected %d of %s in inventory", quantity, itemID)
	}
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

	// ListClasses must advertise Rogue's Expertise requirement - the client
	// otherwise never learns a Rogue needs this choice (rpg-api#625).
	listResp, err := s.server.CharacterClient.ListClasses(ctx, &dnd5ev1alpha1.ListClassesRequest{})
	s.Require().NoError(err)
	var rogueInfo *dnd5ev1alpha1.ClassInfo
	for _, classInfo := range listResp.GetClasses() {
		if classInfo.GetClassId() == dnd5ev1alpha1.Class_CLASS_ROGUE {
			rogueInfo = classInfo
			break
		}
	}
	s.Require().NotNil(rogueInfo, "ListClasses should include Rogue")
	var expertiseChoice *dnd5ev1alpha1.Choice
	for _, choice := range rogueInfo.GetChoices() {
		if choice.GetChoiceType() == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE {
			expertiseChoice = choice
			break
		}
	}
	s.Require().NotNil(expertiseChoice, "Rogue ClassInfo should advertise an Expertise choice")
	s.Assert().Equal(int32(2), expertiseChoice.GetChooseCount())
	s.Assert().NotEmpty(expertiseChoice.GetExpertiseOptions().GetAvailableSkills(),
		"Expertise choice should advertise available skills")

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

	// Rogue fixed starting equipment must reach both the finalize response and persisted API reads.
	s.assertInventoryCounts(char, map[string]int32{
		armor.Leather:      1,
		weapons.Dagger:     2,
		tools.ThievesTools: 1,
	})

	persisted, err := s.server.CharacterClient.GetCharacter(ctx, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: char.GetId()})
	s.Require().NoError(err)
	s.assertInventoryCounts(persisted.GetCharacter(), map[string]int32{
		armor.Leather:      1,
		weapons.Dagger:     2,
		tools.ThievesTools: 1,
	})
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
	s.Require().NotNil(finalizeResp.GetCharacter())

	char := finalizeResp.GetCharacter()
	s.T().Logf("✅ Barbarian created: %s", char.GetId())
	s.assertInventoryCounts(char, map[string]int32{
		weapons.Javelin: 4,
	})

	persisted, err := s.server.CharacterClient.GetCharacter(ctx, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: char.GetId()})
	s.Require().NoError(err)
	s.assertInventoryCounts(persisted.GetCharacter(), map[string]int32{
		weapons.Javelin: 4,
	})
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

func (s *CharacterCreationSuite) TestListClasses_PopulatesEquipmentDetail() {
	ctx := s.authCtx("test-list-classes-equipment-detail")

	listResp, err := s.server.CharacterClient.ListClasses(ctx, &dnd5ev1alpha1.ListClassesRequest{})
	s.Require().NoError(err)

	var fighterInfo *dnd5ev1alpha1.ClassInfo
	for _, classInfo := range listResp.GetClasses() {
		if classInfo.GetClassId() == dnd5ev1alpha1.Class_CLASS_FIGHTER {
			fighterInfo = classInfo
			break
		}
	}
	s.Require().NotNil(fighterInfo, "ListClasses should include Fighter")

	var armorChoice *dnd5ev1alpha1.Choice
	for _, choice := range fighterInfo.GetChoices() {
		if choice.GetId() == "fighter-armor" {
			armorChoice = choice
			break
		}
	}
	s.Require().NotNil(armorChoice, "Fighter should advertise an armor equipment choice")

	bundles := armorChoice.GetEquipmentOptions().GetBundles()
	s.Require().Len(bundles, 2, "fighter-armor should have 2 bundle options")

	// Bundle 0: chain mail (armor item) - assert equipment_detail is populated
	// over the real gRPC path, not just the converter unit.
	chainMailBundle := bundles[0]
	s.Require().Len(chainMailBundle.GetItems(), 1)
	chainMail := chainMailBundle.GetItems()[0]
	s.Assert().Equal("chain-mail", chainMail.GetSelectionId())
	s.Require().NotNil(chainMail.GetEquipmentDetail(),
		"chain mail EquipmentItem should carry equipment_detail over the wire")
	armorData := chainMail.GetEquipmentDetail().GetArmorData()
	s.Require().NotNil(armorData, "chain mail equipment_detail should carry armor data")
	s.Assert().Equal(int32(16), armorData.GetBaseAc())
	s.Assert().True(armorData.GetStealthDisadvantage())

	// Bundle 1: leather armor, longbow, arrows - assert the weapon item
	// (longbow) carries equipment_detail too.
	leatherBundle := bundles[1]
	s.Require().Len(leatherBundle.GetItems(), 3)
	longbow := leatherBundle.GetItems()[1]
	s.Assert().Equal("longbow", longbow.GetSelectionId())
	s.Require().NotNil(longbow.GetEquipmentDetail(),
		"longbow EquipmentItem should carry equipment_detail over the wire")
	weaponData := longbow.GetEquipmentDetail().GetWeaponData()
	s.Require().NotNil(weaponData, "longbow equipment_detail should carry weapon data")
	s.Assert().Equal("1d8", weaponData.GetDamageDice())
	s.Assert().Equal(dnd5ev1alpha1.DamageType_DAMAGE_TYPE_PIERCING, weaponData.GetDamageType())
	s.Assert().Equal(int32(150), weaponData.GetNormalRange())
	s.Assert().Equal(int32(600), weaponData.GetLongRange())
}

func (s *CharacterCreationSuite) TestListRaces_PopulatesTraits() {
	ctx := s.authCtx("test-list-races-traits")

	listResp, err := s.server.CharacterClient.ListRaces(ctx, &dnd5ev1alpha1.ListRacesRequest{})
	s.Require().NoError(err)

	var humanInfo, elfInfo, dwarfInfo *dnd5ev1alpha1.RaceInfo
	for _, raceInfo := range listResp.GetRaces() {
		switch raceInfo.GetRaceId() {
		case dnd5ev1alpha1.Race_RACE_HUMAN:
			humanInfo = raceInfo
		case dnd5ev1alpha1.Race_RACE_ELF:
			elfInfo = raceInfo
		case dnd5ev1alpha1.Race_RACE_DWARF:
			dwarfInfo = raceInfo
		}
	}
	s.Require().NotNil(humanInfo, "ListRaces should include Human")
	s.Require().NotNil(elfInfo, "ListRaces should include Elf")
	s.Require().NotNil(dwarfInfo, "ListRaces should include Dwarf")

	// Human has no toolkit traits - assert the real correct-empty behavior,
	// not just "doesn't crash".
	s.Assert().Empty(humanInfo.GetTraits(), "Human should have no racial traits on the wire")

	// Elf and Dwarf have real toolkit traits - assert they travel over the
	// real ListRaces RPC (not a converter-only unit test), with both name
	// and description populated.
	s.Require().NotEmpty(elfInfo.GetTraits(), "Elf should carry racial traits over the wire")
	elfNames := make([]string, 0, len(elfInfo.GetTraits()))
	for _, trait := range elfInfo.GetTraits() {
		elfNames = append(elfNames, trait.GetName())
		s.Assert().NotEmpty(trait.GetDescription(), "trait %q should carry a description over the wire", trait.GetName())
	}
	s.Assert().Contains(elfNames, "Darkvision")
	s.Assert().Contains(elfNames, "Fey Ancestry")

	s.Require().NotEmpty(dwarfInfo.GetTraits(), "Dwarf should carry racial traits over the wire")
	dwarfNames := make([]string, 0, len(dwarfInfo.GetTraits()))
	for _, trait := range dwarfInfo.GetTraits() {
		dwarfNames = append(dwarfNames, trait.GetName())
	}
	s.Assert().Contains(dwarfNames, "Stonecunning")
}

func (s *CharacterCreationSuite) TestCreateMonk() {
	s.T().Log("Creating Monk character...")
	ctx := s.authCtx("test-player-monk")

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

	// Set Monk with required choices:
	// - 2 skills (from Monk skill list)
	// - Weapon choice (shortsword or simple weapon)
	// - Pack choice (dungeoneer's or explorer's)
	// - Tool choice (artisan's tools or musical instrument)
	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_MONK,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			// Skills: Acrobatics and Stealth
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
			// Weapon: Shortsword
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "monk-weapons-primary",
				OptionId: "monk-weapon-a", // Shortsword
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Pack: Dungeoneer's pack
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "monk-pack",
				OptionId: "monk-pack-a", // Dungeoneer's pack
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Tools: Calligrapher's supplies (fits the Monk aesthetic)
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Tools{
					Tools: &dnd5ev1alpha1.ToolSelection{
						Tools: []dnd5ev1alpha1.Tool{dnd5ev1alpha1.Tool_TOOL_CALLIGRAPHER_SUPPLIES},
					},
				},
			},
		},
	})
	s.Require().NoError(err)

	// Set background
	_, err = s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_HERMIT,
	})
	s.Require().NoError(err)

	// Set ability scores (Monk-optimized: DEX > WIS > CON)
	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     10,
				Dexterity:    15,
				Constitution: 14,
				Intelligence: 8,
				Wisdom:       13,
				Charisma:     12,
			},
		},
	})
	s.Require().NoError(err)

	// Finalize
	finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err, "finalize should succeed")
	s.Require().NotNil(finalizeResp.GetCharacter())

	char := finalizeResp.GetCharacter()
	s.Assert().Equal("Shadow", char.GetName())
	s.Assert().Equal(dnd5ev1alpha1.Class_CLASS_MONK, char.GetClass())

	s.assertInventoryCounts(char, map[string]int32{
		weapons.Dart: 10,
	})

	persisted, err := s.server.CharacterClient.GetCharacter(ctx, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: char.GetId()})
	s.Require().NoError(err)
	s.assertInventoryCounts(persisted.GetCharacter(), map[string]int32{
		weapons.Dart: 10,
	})

	s.T().Logf("✅ Monk created: %s", char.GetId())
}
