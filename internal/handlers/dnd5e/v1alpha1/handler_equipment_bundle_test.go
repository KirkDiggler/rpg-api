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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// EquipmentBundleTestSuite tests equipment bundle handling
type EquipmentBundleTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharService *charactermock.MockService
	handler         *v1alpha1.Handler
	ctx             context.Context
}

func TestEquipmentBundleTestSuite(t *testing.T) {
	suite.Run(t, new(EquipmentBundleTestSuite))
}

func (s *EquipmentBundleTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)
	s.ctx = context.Background()

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *EquipmentBundleTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *EquipmentBundleTestSuite) TestUpdateClass_FighterEquipmentBundleProcessing() {
	// This test verifies that the handler correctly processes fighter equipment bundles
	// when receiving the selection from the frontend
	
	draftID := "draft_123"
	playerID := "player_123"
	
	// Create initial draft data
	initialDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Fighter",
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassFighter,
		},
		Choices: []toolkitchar.ChoiceData{},
	}
	
	// Mock the UpdateClass call - the orchestrator should save the choices
	s.mockCharService.EXPECT().
		UpdateClass(s.ctx, &character.UpdateClassInput{
			DraftID: draftID,
			ClassID: "fighter",
		}).
		Return(&character.UpdateClassOutput{
			Draft: &toolkitchar.DraftData{
				ID:       draftID,
				PlayerID: playerID,
				Name:     "Test Fighter",
				ClassChoice: toolkitchar.ClassChoice{
					ClassID: constants.ClassFighter,
				},
				Choices: []toolkitchar.ChoiceData{
					// After updating with fighter, these choices should be available
					{
						Category: shared.ChoiceEquipment,
						Source:   "class",
						Options: []string{
							"chain-mail",
							"leather-armor,longbow,arrow",
						},
					},
					{
						Category: shared.ChoiceEquipment,
						Source:   "class",
						Options: []string{
							"choose-martial-weapons,shield",  // This is the bundle we care about
							"choose-martial-weapons,choose-martial-weapons",
						},
					},
				},
			},
		}, nil)
	
	// Call UpdateClass
	updateReq := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class: &dnd5ev1alpha1.ClassReference{
			ClassId: "fighter",
		},
	}
	
	updateResp, err := s.handler.UpdateClass(s.ctx, updateReq)
	s.Require().NoError(err)
	s.Require().NotNil(updateResp)
	
	// Now test making the equipment selection with the bundle
	// When the frontend selects "martial weapon + shield" and chooses longsword,
	// it should send both items
	
	// Mock the UpdateChoices call - this is what should save the equipment
	s.mockCharService.EXPECT().
		UpdateChoices(s.ctx, &character.UpdateChoicesInput{
			DraftID: draftID,
			Choices: []toolkitchar.ChoiceData{
				{
					Category: shared.ChoiceEquipment,
					Source:   "class",
					EquipmentSelection: []string{
						"bundle_0:0:longsword",  // The chosen weapon
						"bundle_0:1:shield",     // The shield that comes with the bundle
					},
				},
			},
		}).
		Return(&character.UpdateChoicesOutput{
			Draft: &toolkitchar.DraftData{
				ID:       draftID,
				PlayerID: playerID,
				Name:     "Test Fighter",
				ClassChoice: toolkitchar.ClassChoice{
					ClassID: constants.ClassFighter,
				},
				Choices: []toolkitchar.ChoiceData{
					{
						Category: shared.ChoiceEquipment,
						Source:   "class",
						EquipmentSelection: []string{
							"bundle_0:0:longsword",
							"bundle_0:1:shield",
						},
					},
				},
			},
		}, nil)
	
	// Make the choice request as the frontend would
	choiceReq := &dnd5ev1alpha1.UpdateChoicesRequest{
		DraftId: draftID,
		Choices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter_equipment_2",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentList{
						Items: []string{
							"bundle_0:0:longsword",
							"bundle_0:1:shield",
						},
					},
				},
			},
		},
	}
	
	choiceResp, err := s.handler.UpdateChoices(s.ctx, choiceReq)
	s.Require().NoError(err)
	s.Require().NotNil(choiceResp)
	
	// Verify the response contains both items
	s.Require().Len(choiceResp.Draft.Choices, 1)
	equipment := choiceResp.Draft.Choices[0].GetEquipment()
	s.Require().NotNil(equipment)
	s.Require().Len(equipment.Items, 2)
	s.Contains(equipment.Items, "bundle_0:0:longsword")
	s.Contains(equipment.Items, "bundle_0:1:shield")
}

func (s *EquipmentBundleTestSuite) TestFightingStylePersistence() {
	// This test verifies that fighting style selections persist properly
	
	draftID := "draft_456"
	playerID := "player_456"
	
	// Initial draft with fighter class and fighting style choice
	draftWithFightingStyle := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Fighter",
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassFighter,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category:               shared.ChoiceFightingStyle,
				Source:                 "class",
				FightingStyleSelection: "defense",
			},
		},
	}
	
	// Mock getting the draft - should return the fighting style
	s.mockCharService.EXPECT().
		GetDraft(s.ctx, &character.GetDraftInput{
			DraftID: draftID,
		}).
		Return(&character.GetDraftOutput{
			Draft: draftWithFightingStyle,
		}, nil)
	
	// Get the draft
	getReq := &dnd5ev1alpha1.GetDraftRequest{
		DraftId: draftID,
	}
	
	getResp, err := s.handler.GetDraft(s.ctx, getReq)
	s.Require().NoError(err)
	s.Require().NotNil(getResp)
	
	// Verify fighting style is in the response
	var foundFightingStyle bool
	for _, choice := range getResp.Draft.Choices {
		if choice.Category == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE {
			s.Equal("defense", choice.GetFightingStyle())
			foundFightingStyle = true
			break
		}
	}
	s.True(foundFightingStyle, "Fighting style should be present in draft")
	
	// Now update something else (like race) and verify fighting style persists
	s.mockCharService.EXPECT().
		UpdateRace(s.ctx, &character.UpdateRaceInput{
			DraftID: draftID,
			RaceID:  "human",
		}).
		Return(&character.UpdateRaceOutput{
			Draft: &toolkitchar.DraftData{
				ID:       draftID,
				PlayerID: playerID,
				Name:     "Fighter",
				RaceChoice: toolkitchar.RaceChoice{
					RaceID: constants.RaceHuman,
				},
				ClassChoice: toolkitchar.ClassChoice{
					ClassID: constants.ClassFighter,
				},
				Choices: []toolkitchar.ChoiceData{
					{
						Category:               shared.ChoiceFightingStyle,
						Source:                 "class",
						FightingStyleSelection: "defense",  // Should still be here
					},
				},
			},
		}, nil)
	
	// Update race
	updateRaceReq := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race: &dnd5ev1alpha1.RaceReference{
			RaceId: "human",
		},
	}
	
	updateRaceResp, err := s.handler.UpdateRace(s.ctx, updateRaceReq)
	s.Require().NoError(err)
	s.Require().NotNil(updateRaceResp)
	
	// Verify fighting style still exists after race update
	foundFightingStyle = false
	for _, choice := range updateRaceResp.Draft.Choices {
		if choice.Category == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE {
			s.Equal("defense", choice.GetFightingStyle())
			foundFightingStyle = true
			break
		}
	}
	s.True(foundFightingStyle, "Fighting style should persist after race update")
}

func (s *EquipmentBundleTestSuite) TestBundleExpansionInFinalizeDraft() {
	// This test verifies that when a draft is finalized,
	// the bundle references are properly expanded into actual equipment
	
	draftID := "draft_789"
	playerID := "player_789"
	characterID := "char_789"
	
	// Draft with bundle equipment selections
	draftWithBundles := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Bundle Fighter",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: constants.RaceHuman,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassFighter,
		},
		BackgroundChoice: constants.BackgroundSoldier,
		AbilityScoreChoice: shared.AbilityScores{
			constants.STR: 16,
			constants.DEX: 14,
			constants.CON: 15,
			constants.INT: 10,
			constants.WIS: 12,
			constants.CHA: 8,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category: shared.ChoiceEquipment,
				Source:   "class",
				EquipmentSelection: []string{
					"bundle_0:0:longsword",
					"bundle_0:1:shield",
				},
			},
		},
	}
	
	// Mock the FinalizeDraft call
	// The orchestrator should expand the bundle references
	s.mockCharService.EXPECT().
		FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draftID,
		}).
		Return(&character.FinalizeDraftOutput{
			Character: &toolkitchar.Data{
				ID:           characterID,
				PlayerID:     playerID,
				Name:         "Bundle Fighter",
				RaceID:       constants.RaceHuman,
				ClassID:      constants.ClassFighter,
				BackgroundID: constants.BackgroundSoldier,
				Level:        1,
				Equipment: []string{
					"longsword",  // Expanded from bundle_0:0:longsword
					"shield",     // Expanded from bundle_0:1:shield
				},
			},
			DraftDeleted: true,
		}, nil)
	
	// Finalize the draft
	finalizeReq := &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	}
	
	finalizeResp, err := s.handler.FinalizeDraft(s.ctx, finalizeReq)
	s.Require().NoError(err)
	s.Require().NotNil(finalizeResp)
	s.Require().NotNil(finalizeResp.Character)
	
	// Verify equipment was properly expanded
	s.Require().Len(finalizeResp.Character.Equipment, 2)
	s.Contains(finalizeResp.Character.Equipment, "longsword")
	s.Contains(finalizeResp.Character.Equipment, "shield")
	
	// Verify no bundle references remain
	for _, item := range finalizeResp.Character.Equipment {
		s.NotContains(item, "bundle_", "Equipment should not contain bundle references after finalization")
	}
}