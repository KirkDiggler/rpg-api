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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
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

	// Mock fighter class with skill and equipment choices
	mockFighter := &class.Data{
		ID:                    constants.ClassFighter,
		Name:                  "Fighter",
		Description:           "A master of martial combat",
		HitDice:               10,
		SkillProficiencyCount: 2,
		SkillOptions: []constants.Skill{
			constants.SkillAcrobatics,
			constants.SkillAnimalHandling,
			constants.SkillAthletics,
			constants.SkillHistory,
			constants.SkillInsight,
			constants.SkillIntimidation,
			constants.SkillPerception,
			constants.SkillSurvival,
		},
		EquipmentChoices: []class.EquipmentChoiceData{
			{
				ID:          "fighter_primary_weapon",
				Description: "(a) chain mail or (b) leather armor, longbow, and 20 arrows",
				Choose:      1,
				Options: []class.EquipmentOption{
					{
						ID: "option_martial_weapon_shield",
						Items: []class.EquipmentBundleItem{
							{
								ConcreteItem: &class.EquipmentData{
									ItemID:   "any-martial-weapon",
									Quantity: 1,
								},
							},
							{
								ConcreteItem: &class.EquipmentData{
									ItemID:   "shield",
									Quantity: 1,
								},
							},
						},
					},
					{
						ID: "option_two_martial_weapons",
						Items: []class.EquipmentBundleItem{
							{
								ConcreteItem: &class.EquipmentData{
									ItemID:   "any-martial-weapon",
									Quantity: 2,
								},
							},
						},
					},
				},
			},
			{
				ID:          "fighter_ranged_weapon",
				Description: "(a) a light crossbow and 20 bolts or (b) two handaxes",
				Choose:      1,
				Options: []class.EquipmentOption{
					{
						ID: "option_light_crossbow",
						Items: []class.EquipmentBundleItem{
							{
								ConcreteItem: &class.EquipmentData{
									ItemID:   "light-crossbow",
									Quantity: 1,
								},
							},
							{
								ConcreteItem: &class.EquipmentData{
									ItemID:   "crossbow-bolt",
									Quantity: 20,
								},
							},
						},
					},
					{
						ID: "option_handaxe",
						Items: []class.EquipmentBundleItem{
							{
								ConcreteItem: &class.EquipmentData{
									ItemID:   "handaxe",
									Quantity: 2,
								},
							},
						},
					},
				},
			},
		},
		SavingThrows:        []constants.Ability{constants.STR, constants.CON},
		ArmorProficiencies:  []string{"light", "medium", "heavy", "shields"},
		WeaponProficiencies: []string{"simple", "martial"},
	}

	s.mockCharService.EXPECT().
		ListClasses(ctx, &character.ListClassesInput{}).
		Return(&character.ListClassesOutput{
			Classes: []character.ClassListItem{
				{
					ClassData: mockFighter,
					UIData:    nil,
				},
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
	s.Equal("fighter", fighter.Id)
	s.Equal("Fighter", fighter.Name)

	// Verify choices are populated
	s.Require().Len(fighter.Choices, 3, "Should have 3 choices: 1 skill choice and 2 equipment choices")

	// Check skill choice
	skillChoice := fighter.Choices[0]
	s.Equal("fighter_skills", skillChoice.Id)
	s.Equal("Choose 2 skills", skillChoice.Description)
	s.Equal(int32(2), skillChoice.ChooseCount)
	s.Equal(dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS, skillChoice.ChoiceType)

	// Verify skill options
	explicitOpts := skillChoice.GetExplicitOptions()
	s.Require().NotNil(explicitOpts)
	s.Len(explicitOpts.Options, 8, "Fighter should have 8 skill options")

	// Check first skill option
	firstSkill := explicitOpts.Options[0].GetItem()
	s.Require().NotNil(firstSkill)
	s.Equal("skill_acrobatics", firstSkill.ItemId)
	s.Equal("acrobatics", firstSkill.Name)

	// Check first equipment choice
	equipChoice1 := fighter.Choices[1]
	s.Equal("fighter_equipment_1", equipChoice1.Id)
	s.Equal("(a) chain mail or (b) leather armor, longbow, and 20 arrows", equipChoice1.Description)
	s.Equal(int32(1), equipChoice1.ChooseCount)
	s.Equal(dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT, equipChoice1.ChoiceType)

	// Verify equipment options
	equipOpts1 := equipChoice1.GetExplicitOptions()
	s.Require().NotNil(equipOpts1)
	s.Len(equipOpts1.Options, 2, "Should have 2 equipment options")

	// Check bundle option (martial weapon + shield)
	bundleOpt := equipOpts1.Options[0].GetBundle()
	s.Require().NotNil(bundleOpt)
	s.Len(bundleOpt.Items, 2, "Bundle should have 2 items")

	// Check first item in bundle
	firstBundleItem := bundleOpt.Items[0].GetConcreteItem()
	s.Require().NotNil(firstBundleItem)
	s.Equal("any-martial-weapon", firstBundleItem.ItemId)
	s.Equal(int32(1), firstBundleItem.Quantity)
}
