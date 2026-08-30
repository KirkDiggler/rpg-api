// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package character_integration provides integration tests for character creation.
// These tests verify the full character creation flow through the gRPC API.
package character_integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/armor"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
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

func (s *CharacterCreationSuite) TestCreateFighter_BroadCategoryLongbow() {
	s.createFighterWithMartialWeapon(
		s.authCtx("test-player-fighter-longbow"),
		"Sir Roland",
		dnd5ev1alpha1.Weapon_WEAPON_LONGBOW,
		map[string]int32{
			weapons.Longbow: 1,
			armor.Shield:    1,
		},
	)
}

func (s *CharacterCreationSuite) TestCreateFighter_BroadCategoryLongsword() {
	s.createFighterWithMartialWeapon(
		s.authCtx("test-player-fighter-longsword"),
		"Dame Elara",
		dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD,
		map[string]int32{
			weapons.Longsword: 1,
			armor.Shield:      1,
		},
	)
}

func (s *CharacterCreationSuite) createFighterWithMartialWeapon(
	ctx context.Context,
	name string,
	weapon dnd5ev1alpha1.Weapon,
	expectedInventory map[string]int32,
) {
	s.T().Logf("Creating Fighter with broad-category %s...", weapon)

	// Create draft
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()

	// Set name
	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    name,
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
			// Primary weapons: choose a concrete option from the advertised broad
			// martial category; toolkit owns selection eligibility.
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-weapons-primary",
				OptionId: "fighter-weapon-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{
						Items: []*dnd5ev1alpha1.EquipmentSelectionItem{
							{Equipment: &dnd5ev1alpha1.EquipmentSelectionItem_Weapon{Weapon: weapon}},
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
	s.Assert().Equal(name, char.GetName())
	s.Assert().Equal(dnd5ev1alpha1.Class_CLASS_FIGHTER, char.GetClass())
	s.assertInventoryCounts(char, expectedInventory)

	persisted, err := s.server.CharacterClient.GetCharacter(ctx, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: char.GetId()})
	s.Require().NoError(err)
	s.assertInventoryCounts(persisted.GetCharacter(), expectedInventory)
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

func (s *CharacterCreationSuite) TestCreateDwarfBarbarianWithToolChoice() {
	ctx := s.authCtx("test-player-dwarf-barbarian")

	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()

	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID, Name: "Dagna",
	})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_DWARF,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				ChoiceId: "dwarf-tools",
				Selection: &dnd5ev1alpha1.ChoiceData_Tools{
					Tools: &dnd5ev1alpha1.ToolSelection{
						Tools: []dnd5ev1alpha1.Tool{dnd5ev1alpha1.Tool_TOOL_SMITH_TOOLS},
					},
				},
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

	_, err = s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId: draftID, Background: dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER,
	})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{Strength: 15, Dexterity: 13, Constitution: 14, Intelligence: 8, Wisdom: 12, Charisma: 10},
		},
	})
	s.Require().NoError(err)

	finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{DraftId: draftID})
	s.Require().NoError(err)
	s.Require().NotNil(finalizeResp.GetCharacter())
	s.Equal(dnd5ev1alpha1.Race_RACE_DWARF, finalizeResp.GetCharacter().GetRace())
}

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

func (s *CharacterCreationSuite) TestListClasses_ProjectsMonkDescription() {
	ctx := s.authCtx("test-list-classes-monk-description")

	listResp, err := s.server.CharacterClient.ListClasses(ctx, &dnd5ev1alpha1.ListClassesRequest{})
	s.Require().NoError(err)

	for _, classInfo := range listResp.GetClasses() {
		if classInfo.GetClassId() == dnd5ev1alpha1.Class_CLASS_MONK {
			s.Assert().Equal(
				"A master of martial arts, harnessing inner power through discipline. "+
					"Monk weapons are shortswords and simple melee weapons without the Heavy or Two-Handed property",
				classInfo.GetDescription(),
			)
			return
		}
	}

	s.Fail("ListClasses should include Monk")
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

	// Bundle 1 is the legacy concrete path. Keep every published field in
	// order: category-choice conversion must not regress existing bundles.
	leatherBundle := bundles[1]
	legacyItems := leatherBundle.GetItems()
	s.Require().Len(legacyItems, 3)
	s.Assert().Equal([]string{"leather", "longbow", "arrows-20"}, equipmentSelectionIDs(legacyItems))
	s.Assert().Equal([]int32{1, 1, 1}, equipmentItemQuantities(legacyItems))

	leather := legacyItems[0]
	s.Assert().Equal(dnd5ev1alpha1.Armor_ARMOR_LEATHER, leather.GetArmor())
	s.Require().NotNil(leather.GetEquipmentDetail(), "leather should carry resolved detail over the wire")
	s.Assert().Equal("Leather Armor", leather.GetEquipmentDetail().GetName())
	leatherData := leather.GetEquipmentDetail().GetArmorData()
	s.Require().NotNil(leatherData)
	s.Assert().Equal(dnd5ev1alpha1.ArmorCategory_ARMOR_CATEGORY_LIGHT, leatherData.GetArmorCategory())
	s.Assert().Equal(int32(11), leatherData.GetBaseAc())
	s.Assert().True(leatherData.GetDexBonus())

	longbow := legacyItems[1]
	s.Assert().Equal(dnd5ev1alpha1.Weapon_WEAPON_LONGBOW, longbow.GetWeapon())
	s.Require().NotNil(longbow.GetEquipmentDetail(),
		"longbow EquipmentItem should carry equipment_detail over the wire")
	s.Assert().Equal("Longbow", longbow.GetEquipmentDetail().GetName())
	weaponData := longbow.GetEquipmentDetail().GetWeaponData()
	s.Require().NotNil(weaponData, "longbow equipment_detail should carry weapon data")
	s.Assert().Equal("1d8", weaponData.GetDamageDice())
	s.Assert().Equal(dnd5ev1alpha1.DamageType_DAMAGE_TYPE_PIERCING, weaponData.GetDamageType())
	s.Assert().Equal(int32(150), weaponData.GetNormalRange())
	s.Assert().Equal(int32(600), weaponData.GetLongRange())

	arrows := legacyItems[2]
	s.Assert().Nil(arrows.GetTypeHint(), "legacy ammunition type hint must remain absent")
	s.Require().NotNil(arrows.GetEquipmentDetail(), "arrows should carry resolved detail over the wire")
	s.Assert().Equal("Arrows (20)", arrows.GetEquipmentDetail().GetName())
	s.Assert().Equal(dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_ADVENTURING_GEAR,
		arrows.GetEquipmentDetail().GetCategory())
	s.Assert().Nil(arrows.GetEquipmentDetail().GetEquipmentData())
}

func (s *CharacterCreationSuite) TestListClasses_MapsResolvedCategoryChoiceOptions() {
	ctx := s.authCtx("test-list-classes-category-choice-options")

	response, err := s.server.CharacterClient.ListClasses(ctx, &dnd5ev1alpha1.ListClassesRequest{})
	s.Require().NoError(err)

	classesByID := make(map[dnd5ev1alpha1.Class]*dnd5ev1alpha1.ClassInfo, len(response.GetClasses()))
	for _, classInfo := range response.GetClasses() {
		classesByID[classInfo.GetClassId()] = classInfo
	}

	fighterMartial := categoryChoiceFromClass(s.T(), classesByID[dnd5ev1alpha1.Class_CLASS_FIGHTER], "fighter-weapons-primary", "fighter-weapon-a")
	assertCategoryWeaponOptions(
		s.T(),
		fighterMartial.GetOptions(),
		[]string{
			// "net" dropped: rpg-toolkit#1146 (composable attack damage
			// provider, dnd5e v0.97.0) removed the Net weapon from the
			// toolkit's registry entirely, so the real RPC this suite drives
			// no longer offers it as a martial-ranged option.
			"greatsword", "longsword", "rapier", "shortsword", "battleaxe", "flail", "glaive", "greataxe",
			"halberd", "lance", "maul", "morningstar", "pike", "scimitar", "trident", "war-pick",
			"warhammer", "whip", "heavy-crossbow", "longbow", "blowgun", "hand-crossbow",
		},
	)
	longbow := equipmentItemByID(s.T(), fighterMartial.GetOptions(), "longbow")
	s.Equal(dnd5ev1alpha1.Weapon_WEAPON_LONGBOW, longbow.GetWeapon())
	s.Equal(int32(1), longbow.GetQuantity())
	s.Require().NotNil(longbow.GetEquipmentDetail(), "resolved detail must survive the real RPC")
	s.Equal("Longbow", longbow.GetEquipmentDetail().GetName())
	s.Equal("1d8", longbow.GetEquipmentDetail().GetWeaponData().GetDamageDice())

	monkFixed := equipmentBundleFromClass(s.T(), classesByID[dnd5ev1alpha1.Class_CLASS_MONK], "monk-weapons-primary", "monk-weapon-a")
	s.Require().Empty(monkFixed.GetCategoryChoices(), "fixed Monk alternative must not become a category choice")
	s.Require().Len(monkFixed.GetItems(), 1)
	shortsword := monkFixed.GetItems()[0]
	s.Equal("shortsword", shortsword.GetSelectionId())
	s.Equal(int32(1), shortsword.GetQuantity())
	s.Equal(dnd5ev1alpha1.Weapon_WEAPON_SHORTSWORD, shortsword.GetWeapon())
	s.Require().NotNil(shortsword.GetEquipmentDetail())
	s.Equal("Shortsword", shortsword.GetEquipmentDetail().GetName())
	s.Equal(int32(10), shortsword.GetEquipmentDetail().GetCost().GetQuantity())
	s.Equal("gp", shortsword.GetEquipmentDetail().GetCost().GetUnit())

	monkSimple := categoryChoiceFromClass(s.T(), classesByID[dnd5ev1alpha1.Class_CLASS_MONK], "monk-weapons-primary", "monk-weapon-b")
	assertCategoryWeaponOptions(
		s.T(),
		monkSimple.GetOptions(),
		[]string{
			"club", "dagger", "handaxe", "javelin", "greatclub", "light-hammer", "mace", "quarterstaff",
			"sickle", "spear", "light-crossbow", "shortbow", "dart", "sling",
		},
	)
	monkIDs := equipmentSelectionIDs(monkSimple.GetOptions())
	s.Assert().NotContains(monkIDs, "shortsword", "shortsword remains the separate fixed Monk alternative")
	s.Assert().NotContains(monkIDs, "unarmed-strike", "toolkit special-weapon exclusion must survive the RPC")
}

func equipmentBundleFromClass(t *testing.T, classInfo *dnd5ev1alpha1.ClassInfo, choiceID, bundleID string) *dnd5ev1alpha1.EquipmentBundle {
	t.Helper()
	require.NotNil(t, classInfo)
	for _, choice := range classInfo.GetChoices() {
		if choice.GetId() != choiceID {
			continue
		}
		for _, bundle := range choice.GetEquipmentOptions().GetBundles() {
			if bundle.GetId() == bundleID {
				return bundle
			}
		}
	}
	require.Failf(t, "missing equipment bundle", "choice %q bundle %q", choiceID, bundleID)
	return nil
}

func categoryChoiceFromClass(t *testing.T, classInfo *dnd5ev1alpha1.ClassInfo, choiceID, bundleID string) *dnd5ev1alpha1.EquipmentCategoryChoice {
	t.Helper()
	require.NotNil(t, classInfo)
	for _, choice := range classInfo.GetChoices() {
		if choice.GetId() != choiceID {
			continue
		}
		for _, bundle := range choice.GetEquipmentOptions().GetBundles() {
			if bundle.GetId() == bundleID {
				require.Len(t, bundle.GetCategoryChoices(), 1)
				return bundle.GetCategoryChoices()[0]
			}
		}
	}
	require.Failf(t, "missing category choice", "choice %q bundle %q", choiceID, bundleID)
	return nil
}

func equipmentSelectionIDs(items []*dnd5ev1alpha1.EquipmentItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.GetSelectionId())
	}
	return ids
}

func equipmentItemQuantities(items []*dnd5ev1alpha1.EquipmentItem) []int32 {
	quantities := make([]int32, 0, len(items))
	for _, item := range items {
		quantities = append(quantities, item.GetQuantity())
	}
	return quantities
}

func assertCategoryWeaponOptions(t *testing.T, actual []*dnd5ev1alpha1.EquipmentItem, expectedIDs []string) {
	t.Helper()
	require.Equal(t, expectedIDs, equipmentSelectionIDs(actual), "category option IDs and registry order")
	require.Len(t, actual, len(expectedIDs))
	for index, item := range actual {
		require.NotNil(t, item, "option %d", index)
		require.Equal(t, int32(1), item.GetQuantity(), "option %d quantity", index)
		require.NotEqual(t, dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED, item.GetWeapon(), "option %d type hint", index)
		require.NotNil(t, item.GetEquipmentDetail(), "option %d detail", index)
		require.NotNil(t, item.GetEquipmentDetail().GetWeaponData(), "option %d weapon detail", index)
		require.NotEmpty(t, item.GetEquipmentDetail().GetName(), "option %d detail name", index)
	}
}

func equipmentItemByID(t *testing.T, items []*dnd5ev1alpha1.EquipmentItem, selectionID string) *dnd5ev1alpha1.EquipmentItem {
	t.Helper()
	for _, item := range items {
		if item.GetSelectionId() == selectionID {
			return item
		}
	}
	require.Failf(t, "missing equipment item", "selection ID %q", selectionID)
	return nil
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

func (s *CharacterCreationSuite) TestCreateMonk_FixedShortsword() {
	ctx := s.authCtx("test-player-monk-shortsword")
	char := s.createMonkWithPrimaryWeapon(ctx, "monk-weapon-a", dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED)

	expectedInventory := map[string]int32{
		weapons.Dart:       10,
		weapons.Shortsword: 1,
	}
	s.assertInventoryCounts(char, expectedInventory)

	persisted, err := s.server.CharacterClient.GetCharacter(ctx, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: char.GetId()})
	s.Require().NoError(err)
	s.assertInventoryCounts(persisted.GetCharacter(), expectedInventory)
}

func (s *CharacterCreationSuite) TestCreateMonk_SimpleRangedCategorySelections() {
	testCases := []struct {
		name     string
		weapon   dnd5ev1alpha1.Weapon
		expected map[string]int32
	}{
		{
			name:   "club",
			weapon: dnd5ev1alpha1.Weapon_WEAPON_CLUB,
			expected: map[string]int32{
				weapons.Club: 1,
				weapons.Dart: 10,
			},
		},
		{
			name:   "dart",
			weapon: dnd5ev1alpha1.Weapon_WEAPON_DART,
			expected: map[string]int32{
				weapons.Dart: 11, // 10 class-granted darts + the selected category weapon
			},
		},
		{
			name:   "light-crossbow",
			weapon: dnd5ev1alpha1.Weapon_WEAPON_LIGHT_CROSSBOW,
			expected: map[string]int32{
				weapons.Dart:          10,
				weapons.LightCrossbow: 1,
			},
		},
	}

	for _, testCase := range testCases {
		s.Run(testCase.name, func() {
			ctx := s.authCtx("test-player-monk-" + testCase.name)
			char := s.createMonkWithPrimaryWeapon(ctx, "monk-weapon-b", testCase.weapon)
			s.assertInventoryCounts(char, testCase.expected)

			persisted, err := s.server.CharacterClient.GetCharacter(ctx, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: char.GetId()})
			s.Require().NoError(err)
			s.assertInventoryCounts(persisted.GetCharacter(), testCase.expected)
		})
	}
}

func (s *CharacterCreationSuite) TestCreateMonk_RejectsLiveAndPersistedUnarmedStrikeCategorySelection() {
	ctx := s.authCtx("test-player-monk-unarmed-strike")

	// A live request must fail before the invalid selection is saved.
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	invalidChoices := monkClassChoices("monk-weapon-b", dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED)
	invalidChoices[1].GetEquipment().Items = []*dnd5ev1alpha1.EquipmentSelectionItem{{
		Equipment: &dnd5ev1alpha1.EquipmentSelectionItem_OtherEquipmentId{OtherEquipmentId: weapons.UnarmedStrike},
	}}
	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId:      createResp.GetDraft().GetId(),
		Class:        dnd5ev1alpha1.Class_CLASS_MONK,
		ClassChoices: invalidChoices,
	})
	s.Require().Error(err)
	s.ErrorContains(err, "Invalid equipment choice 'unarmed-strike'")

	// Seed the serialized shape accepted by v0.70.0, then prove the real
	// FinalizeDraft RPC revalidates persisted selections before creating a character.
	draftID := s.createMonkDraftWithPrimaryWeapon(ctx, "monk-weapon-b", dnd5ev1alpha1.Weapon_WEAPON_CLUB)
	draftRepo := s.newDraftRepository()
	stored, err := draftRepo.Get(ctx, characterdraft.GetInput{ID: draftID})
	s.Require().NoError(err)
	mutated := false
	for i := range stored.Draft.Data.Choices {
		choice := &stored.Draft.Data.Choices[i]
		if choice.ChoiceID == "monk-weapons-primary" && choice.OptionID == "monk-weapon-b" {
			choice.EquipmentSelection = []shared.SelectionID{weapons.UnarmedStrike}
			mutated = true
			break
		}
	}
	s.Require().True(mutated, "valid Monk category selection must be persisted before the regression mutation")
	_, err = draftRepo.Update(ctx, characterdraft.UpdateInput{Draft: stored.Draft})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{DraftId: draftID})
	s.Require().Error(err)
	s.ErrorContains(err, "unarmed-strike")
}

func (s *CharacterCreationSuite) newDraftRepository() characterdraft.Repository {
	client, err := redisclient.NewClient(sharedRedis.Addr, nil)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = client.Close() })

	repo, err := characterdraft.NewRedis(&characterdraft.Config{
		Client:      client,
		Clock:       clock.New(),
		IDGenerator: idgen.NewPrefixed("test-draft-"),
	})
	s.Require().NoError(err)
	return repo
}

func (s *CharacterCreationSuite) createMonkWithPrimaryWeapon(
	ctx context.Context,
	optionID string,
	weapon dnd5ev1alpha1.Weapon,
) *dnd5ev1alpha1.Character {
	draftID := s.createMonkDraftWithPrimaryWeapon(ctx, optionID, weapon)

	finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err, "finalize should succeed")
	s.Require().NotNil(finalizeResp.GetCharacter())

	char := finalizeResp.GetCharacter()
	s.Assert().Equal("Shadow", char.GetName())
	s.Assert().Equal(dnd5ev1alpha1.Class_CLASS_MONK, char.GetClass())
	s.T().Logf("✅ Monk created: %s", char.GetId())
	return char
}

func (s *CharacterCreationSuite) createMonkDraftWithPrimaryWeapon(
	ctx context.Context,
	optionID string,
	weapon dnd5ev1alpha1.Weapon,
) string {
	s.T().Logf("Creating Monk with primary weapon option %s...", optionID)

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

	// Set Monk with required choices.
	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId:      draftID,
		Class:        dnd5ev1alpha1.Class_CLASS_MONK,
		ClassChoices: monkClassChoices(optionID, weapon),
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

	return draftID
}

func monkClassChoices(optionID string, weapon dnd5ev1alpha1.Weapon) []*dnd5ev1alpha1.ChoiceData {
	weaponSelection := &dnd5ev1alpha1.EquipmentSelection{}
	if weapon != dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED {
		weaponSelection.Items = []*dnd5ev1alpha1.EquipmentSelectionItem{
			{Equipment: &dnd5ev1alpha1.EquipmentSelectionItem_Weapon{Weapon: weapon}},
		}
	}

	return []*dnd5ev1alpha1.ChoiceData{
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			Selection: &dnd5ev1alpha1.ChoiceData_Skills{Skills: &dnd5ev1alpha1.SkillSelection{
				Skills: []dnd5ev1alpha1.Skill{dnd5ev1alpha1.Skill_SKILL_ACROBATICS, dnd5ev1alpha1.Skill_SKILL_STEALTH},
			}},
		},
		{
			Category:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
			Source:    dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId:  "monk-weapons-primary",
			OptionId:  optionID,
			Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: weaponSelection},
		},
		{
			Category:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
			Source:    dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId:  "monk-pack",
			OptionId:  "monk-pack-a",
			Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{}},
		},
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			Selection: &dnd5ev1alpha1.ChoiceData_Tools{Tools: &dnd5ev1alpha1.ToolSelection{
				Tools: []dnd5ev1alpha1.Tool{dnd5ev1alpha1.Tool_TOOL_CALLIGRAPHER_SUPPLIES},
			}},
		},
	}
}
