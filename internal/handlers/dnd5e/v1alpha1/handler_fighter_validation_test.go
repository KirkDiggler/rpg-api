package v1alpha1_test

import (
	"context"
	"testing"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// FighterValidationTestSuite tests Fighter class validation
type FighterValidationTestSuite struct {
	suite.Suite
	client  dnd5ev1alpha1.CharacterServiceClient
	conn    *grpc.ClientConn
	ctx     context.Context
	draftID string
}

func TestFighterValidationSuite(t *testing.T) {
	suite.Run(t, new(FighterValidationTestSuite))
}

func (s *FighterValidationTestSuite) SetupSuite() {
	// Connect to test server
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	s.Require().NoError(err)
	s.conn = conn
	s.client = dnd5ev1alpha1.NewCharacterServiceClient(conn)
	s.ctx = context.Background()
}

func (s *FighterValidationTestSuite) TearDownSuite() {
	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *FighterValidationTestSuite) SetupTest() {
	// Create a fresh draft for each test
	resp, err := s.client.CreateDraft(s.ctx, &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId: "test-player",
	})
	s.Require().NoError(err)
	s.draftID = resp.Draft.Id
}

func (s *FighterValidationTestSuite) TestFighterWithoutAnyChoices() {
	// Update to Fighter class without any choices
	resp, err := s.client.UpdateClass(s.ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId:      s.draftID,
		Class:        dnd5ev1alpha1.Class_CLASS_FIGHTER,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{},
	})
	s.Require().NoError(err)
	
	// Should have warnings for all missing choices
	s.Require().Len(resp.Warnings, 1)
	warning := resp.Warnings[0]
	s.Equal("class_choices", warning.Field)
	s.Contains(warning.Message, "skill proficiencies")
	s.Contains(warning.Message, "fighting style")
	s.Contains(warning.Message, "starting armor")
	s.Contains(warning.Message, "primary martial weapon")
	s.Contains(warning.Message, "shield or second weapon")
	s.Contains(warning.Message, "ranged weapon")
	s.Contains(warning.Message, "equipment pack")
}

func (s *FighterValidationTestSuite) TestFighterWithOnlyFightingStyle() {
	// Update to Fighter class with only fighting style
	resp, err := s.client.UpdateClass(s.ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: s.draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_FIGHTER,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter_feature_fighting-style-choice",
				Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
					Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
					FightingStyle: "dueling",
				},
			},
		},
	})
	s.Require().NoError(err)
	
	// Should still have warnings for missing choices (but not fighting style)
	s.Require().Len(resp.Warnings, 1)
	warning := resp.Warnings[0]
	s.Contains(warning.Message, "skill proficiencies")
	s.NotContains(warning.Message, "fighting style") // This one is provided
	s.Contains(warning.Message, "starting armor")
}

func (s *FighterValidationTestSuite) TestFighterWithInvalidFightingStyle() {
	// Update to Fighter class with invalid fighting style
	resp, err := s.client.UpdateClass(s.ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: s.draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_FIGHTER,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category:      dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
				Source:        dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId:      "fighter_feature_fighting-style-choice",
				Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
					FightingStyle: "invalid-style",
				},
			},
		},
	})
	s.Require().NoError(err)
	
	// Should have warning about invalid fighting style
	hasInvalidStyleWarning := false
	for _, warning := range resp.Warnings {
		if warning.Field == "fighting_style" && s.Contains(warning.Message, "Invalid fighting style") {
			hasInvalidStyleWarning = true
			break
		}
	}
	s.True(hasInvalidStyleWarning, "Should have warning about invalid fighting style")
}

func (s *FighterValidationTestSuite) TestFighterWithSkillsAndFightingStyle() {
	// Update to Fighter class with skills and fighting style
	resp, err := s.client.UpdateClass(s.ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: s.draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_FIGHTER,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-skills",
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillList{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
							dnd5ev1alpha1.Skill_SKILL_PERCEPTION,
						},
					},
				},
			},
			{
				Category:      dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
				Source:        dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId:      "fighter_feature_fighting-style-choice",
				Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
					FightingStyle: "defense",
			},
		},
	})
	s.Require().NoError(err)
	
	// Should only have warnings for equipment choices now
	s.Require().Len(resp.Warnings, 1)
	warning := resp.Warnings[0]
	s.NotContains(warning.Message, "skill proficiencies") // Provided
	s.NotContains(warning.Message, "fighting style")      // Provided
	s.Contains(warning.Message, "starting armor")
	s.Contains(warning.Message, "primary martial weapon")
}

func (s *FighterValidationTestSuite) TestFighterFullyConfigured() {
	// Update to Fighter class with ALL required choices
	resp, err := s.client.UpdateClass(s.ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: s.draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_FIGHTER,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			// Skills
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-skills",
				Skills: &dnd5ev1alpha1.SkillList{
					Skills: []dnd5ev1alpha1.Skill{
						dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
						dnd5ev1alpha1.Skill_SKILL_INTIMIDATION,
					},
				},
			},
			// Fighting Style
			{
				Category:      dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
				Source:        dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId:      "fighter_feature_fighting-style-choice",
				Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
					FightingStyle: "great-weapon-fighting",
			},
			// Armor choice
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-armor-choice",
				Equipment: &dnd5ev1alpha1.EquipmentList{
					Items: []string{"chain-mail"},
				},
			},
			// Primary weapon
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-primary-weapon",
				Equipment: &dnd5ev1alpha1.EquipmentList{
					Items: []string{"greatsword"},
				},
			},
			// Secondary equipment (choosing second weapon instead of shield)
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-secondary-equipment",
				Equipment: &dnd5ev1alpha1.EquipmentList{
					Items: []string{"longsword"},
				},
			},
			// Ranged weapon
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-ranged-choice",
				Equipment: &dnd5ev1alpha1.EquipmentList{
					Items: []string{"crossbow-bolts"},
				},
			},
			// Pack
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-pack-choice",
				Equipment: &dnd5ev1alpha1.EquipmentList{
					Items: []string{"dungeoneers-pack"},
				},
			},
		},
	})
	s.Require().NoError(err)
	
	// Should have NO warnings when fully configured
	s.Empty(resp.Warnings, "Fully configured Fighter should have no validation warnings")
}