package v1alpha1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
)

type HandlerExpertiseTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharService *charactermock.MockService
	handler         *v1alpha1.Handler
}

func TestHandlerExpertiseTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerExpertiseTestSuite))
}

func (s *HandlerExpertiseTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerExpertiseTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerExpertiseTestSuite) TestListClasses_RogueExpertiseConverted() {
	ctx := context.Background()

	// Create a mock Rogue with expertise
	mockRogue := &toolkitchar.StartingClass{
		ID:          "rogue",
		Name:        "Rogue",
		Description: "A scoundrel who uses stealth and trickery",
		Group:       classes.Rogue,
		Grants: &classes.AutomaticGrants{
			HitDice:             8,
			SavingThrows:        []abilities.Ability{abilities.DEX, abilities.INT},
			ArmorProficiencies:  []proficiencies.Armor{proficiencies.ArmorLight},
			WeaponProficiencies: []proficiencies.Weapon{proficiencies.WeaponSimple, proficiencies.WeaponHandCrossbow, proficiencies.WeaponLongsword, proficiencies.WeaponRapier, proficiencies.WeaponShortsword},
			ToolProficiencies:   []proficiencies.Tool{proficiencies.ToolThieves},
		},
		Requirements: &choices.Requirements{
			Skills: &choices.SkillRequirement{
				Count: 4,
				Label: "Choose 4 skills",
				Options: []skills.Skill{
					skills.Acrobatics,
					skills.Athletics,
					skills.Deception,
					skills.Insight,
					skills.Intimidation,
					skills.Investigation,
					skills.Perception,
					skills.Performance,
					skills.Persuasion,
					skills.SleightOfHand,
					skills.Stealth,
				},
			},
			Expertise: &choices.ExpertiseRequirement{
				Count: 2,
				Label: "Choose 2 skills or thieves' tools for expertise",
			},
			Equipment: []*choices.EquipmentRequirement{
				{
					Choose: 1,
					Label:  "(a) a rapier or (b) a shortsword",
					Options: []choices.EquipmentOption{
						{
							Label: "rapier",
							Items: []choices.ItemSpec{
								{Type: "weapon", ID: "rapier", Quantity: 1},
							},
						},
						{
							Label: "shortsword",
							Items: []choices.ItemSpec{
								{Type: "weapon", ID: "shortsword", Quantity: 1},
							},
						},
					},
				},
			},
		},
	}

	s.mockCharService.EXPECT().
		ListClasses(ctx, &character.ListClassesInput{}).
		Return(&character.ListClassesOutput{
			Classes: []*toolkitchar.StartingClass{
				mockRogue,
			},
			TotalSize: 1,
		}, nil)

	// Call the handler
	req := &dnd5ev1alpha1.ListClassesRequest{}
	resp, err := s.handler.ListClasses(ctx, req)

	// Verify response
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Classes, 1)

	rogue := resp.Classes[0]
	s.Equal("rogue", rogue.Id)
	s.Equal("Rogue", rogue.Name)
	s.Equal(dnd5ev1alpha1.Class_CLASS_ROGUE, rogue.Group)
	
	// Verify choices are populated including expertise
	s.Require().NotEmpty(rogue.Choices, "Rogue should have choices")
	
	// Find the expertise choice
	var expertiseChoice *dnd5ev1alpha1.Choice
	var skillChoice *dnd5ev1alpha1.Choice
	for _, choice := range rogue.Choices {
		if choice.ChoiceType == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE {
			expertiseChoice = choice
		}
		if choice.ChoiceType == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS {
			skillChoice = choice
		}
		s.T().Logf("Choice found: ID=%s, Type=%v, Count=%d, Description=%s",
			choice.Id, choice.ChoiceType, choice.ChooseCount, choice.Description)
	}
	
	// Verify expertise choice exists and is correct
	s.Require().NotNil(expertiseChoice, "Rogue should have expertise choice")
	s.Equal("class-expertise", expertiseChoice.Id)
	s.Equal(int32(2), expertiseChoice.ChooseCount, "Should choose 2 for expertise")
	s.Equal("Choose 2 skills or thieves' tools for expertise", expertiseChoice.Description)
	s.Equal(dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE, expertiseChoice.ChoiceType)
	
	// Also verify skill choice exists
	s.Require().NotNil(skillChoice, "Rogue should have skill choice")
	s.Equal("class-skills", skillChoice.Id)
	s.Equal(int32(4), skillChoice.ChooseCount, "Should choose 4 skills")
}

func (s *HandlerExpertiseTestSuite) TestListClasses_NonRogueNoExpertise() {
	ctx := context.Background()

	// Create a mock Fighter without expertise
	mockFighter := &toolkitchar.StartingClass{
		ID:          "fighter",
		Name:        "Fighter",
		Description: "A master of martial combat",
		Group:       classes.Fighter,
		Grants: &classes.AutomaticGrants{
			HitDice:             10,
			SavingThrows:        []abilities.Ability{abilities.STR, abilities.CON},
			ArmorProficiencies:  []proficiencies.Armor{proficiencies.ArmorLight, proficiencies.ArmorMedium, proficiencies.ArmorHeavy, proficiencies.ArmorShields},
			WeaponProficiencies: []proficiencies.Weapon{proficiencies.WeaponSimple, proficiencies.WeaponMartial},
		},
		Requirements: &choices.Requirements{
			Skills: &choices.SkillRequirement{
				Count: 2,
				Label: "Choose 2 skills",
				Options: []skills.Skill{
					skills.Acrobatics,
					skills.AnimalHandling,
					skills.Athletics,
					skills.History,
					skills.Insight,
					skills.Intimidation,
					skills.Perception,
					skills.Survival,
				},
			},
			// No expertise for Fighter
		},
	}

	s.mockCharService.EXPECT().
		ListClasses(ctx, &character.ListClassesInput{}).
		Return(&character.ListClassesOutput{
			Classes: []*toolkitchar.StartingClass{
				mockFighter,
			},
			TotalSize: 1,
		}, nil)

	// Call the handler
	req := &dnd5ev1alpha1.ListClassesRequest{}
	resp, err := s.handler.ListClasses(ctx, req)

	// Verify response
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Classes, 1)

	fighter := resp.Classes[0]
	
	// Verify Fighter has no expertise choice
	for _, choice := range fighter.Choices {
		s.NotEqual(dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE, choice.ChoiceType,
			"Fighter should not have expertise choice")
	}
}