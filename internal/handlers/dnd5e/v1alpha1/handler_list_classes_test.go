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

type HandlerListClassesTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharService *charactermock.MockService
	handler         *v1alpha1.Handler
}

func TestHandlerListClassesTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerListClassesTestSuite))
}

func (s *HandlerListClassesTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerListClassesTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerListClassesTestSuite) TestListClasses_WithChoices() {
	ctx := context.Background()

	// Create a mock fighter StartingClass with requirements
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
			Equipment: []*choices.EquipmentRequirement{
				{
					Choose: 1,
					Label:  "(a) chain mail or (b) leather armor, longbow, and 20 arrows",
					Options: []choices.EquipmentOption{
						{
							Label: "chain mail",
							Items: []choices.ItemSpec{
								{Type: "armor", ID: "chain-mail", Quantity: 1},
							},
						},
						{
							Label: "leather armor and ranged",
							Items: []choices.ItemSpec{
								{Type: "armor", ID: "leather", Quantity: 1},
								{Type: "weapon", ID: "longbow", Quantity: 1},
								{Type: "ammunition", ID: "arrow", Quantity: 20},
							},
						},
					},
				},
			},
		},
	}

	// Create mock Life Domain cleric with additional choices
	mockLifeDomain := &toolkitchar.StartingClass{
		ID:          "life-domain",
		Name:        "Life Domain",
		Description: "The Life domain focuses on healing",
		Group:       classes.Cleric,
		Grants: &classes.AutomaticGrants{
			HitDice:             8,
			SavingThrows:        []abilities.Ability{abilities.WIS, abilities.CHA},
			ArmorProficiencies:  []proficiencies.Armor{proficiencies.ArmorLight, proficiencies.ArmorMedium, proficiencies.ArmorShields},
			WeaponProficiencies: []proficiencies.Weapon{proficiencies.WeaponSimple},
		},
		Requirements: &choices.Requirements{
			Skills: &choices.SkillRequirement{
				Count: 2,
				Label: "Choose 2 skills from Cleric list",
				Options: []skills.Skill{
					skills.History,
					skills.Insight,
					skills.Medicine,
					skills.Persuasion,
					skills.Religion,
				},
			},
			Cantrips: &choices.SpellRequirement{
				Count: 3,
				Level: 0,
				Label: "Choose 3 Cleric cantrips",
			},
			Spells: &choices.SpellRequirement{
				Count: 2,
				Level: 1,
				Label: "Choose 2 1st-level Cleric spells",
			},
		},
	}

	s.mockCharService.EXPECT().
		ListClasses(ctx, &character.ListClassesInput{}).
		Return(&character.ListClassesOutput{
			Classes: []*toolkitchar.StartingClass{
				mockFighter,
				mockLifeDomain,
			},
			TotalSize: 2,
		}, nil)

	// Call the handler
	req := &dnd5ev1alpha1.ListClassesRequest{}
	resp, err := s.handler.ListClasses(ctx, req)

	// Verify response
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Classes, 2)

	// Check Fighter
	fighter := resp.Classes[0]
	s.Equal("fighter", fighter.Id)
	s.Equal("Fighter", fighter.Name)
	s.Equal(dnd5ev1alpha1.Class_CLASS_FIGHTER, fighter.Group)
	
	// Verify fighter choices are populated
	s.Require().Len(fighter.Choices, 2, "Fighter should have 2 choices: skills and equipment")
	
	// Check skill choice
	skillChoice := fighter.Choices[0]
	s.Equal("class-skills", skillChoice.Id)
	s.Equal(int32(2), skillChoice.ChooseCount)
	s.Equal(dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS, skillChoice.ChoiceType)
	
	// Check Life Domain
	lifeDomain := resp.Classes[1]
	s.Equal("life-domain", lifeDomain.Id)
	s.Equal("Life Domain", lifeDomain.Name)
	s.Equal(dnd5ev1alpha1.Class_CLASS_CLERIC, lifeDomain.Group, "Life Domain should have Cleric as group")
	
	// Verify Life Domain choices are populated
	s.Require().GreaterOrEqual(len(lifeDomain.Choices), 3, "Life Domain should have at least 3 choices: skills, cantrips, and spells")
	
	// Find cantrips choice
	var hasCantrips bool
	for _, choice := range lifeDomain.Choices {
		if choice.ChoiceType == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS {
			hasCantrips = true
			s.Equal(int32(3), choice.ChooseCount, "Should choose 3 cantrips")
			break
		}
	}
	s.True(hasCantrips, "Life Domain should have cantrips choice")
}

func (s *HandlerListClassesTestSuite) TestListClasses_Empty() {
	ctx := context.Background()

	s.mockCharService.EXPECT().
		ListClasses(ctx, &character.ListClassesInput{}).
		Return(&character.ListClassesOutput{
			Classes:   []*toolkitchar.StartingClass{},
			TotalSize: 0,
		}, nil)

	req := &dnd5ev1alpha1.ListClassesRequest{}
	resp, err := s.handler.ListClasses(ctx, req)

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Empty(resp.Classes)
	s.Equal(int32(0), resp.TotalSize)
}