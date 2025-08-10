package v1alpha1_test

import (
	"context"
	"testing"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	externalmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
)

// FighterBundleIntegrationSuite tests the complete flow of fighter equipment selection
// with real Redis to ensure bundles are properly expanded
type FighterBundleIntegrationSuite struct {
	suite.Suite
	ctx            context.Context
	ctrl           *gomock.Controller
	miniRedis      *miniredis.Miniredis
	redisClient    redis.UniversalClient
	handler        *v1alpha1.Handler
	mockExternal   *externalmock.MockClient
	mockDice       *dicemock.MockService
	draftID        string
	playerID       string
}

func TestFighterBundleIntegrationSuite(t *testing.T) {
	suite.Run(t, new(FighterBundleIntegrationSuite))
}

func (s *FighterBundleIntegrationSuite) SetupTest() {
	s.ctx = context.Background()
	s.ctrl = gomock.NewController(s.T())
	
	// Setup Redis
	s.miniRedis = miniredis.RunT(s.T())
	s.redisClient = redis.NewClient(&redis.Options{
		Addr: s.miniRedis.Addr(),
	})
	
	// Setup mocks
	s.mockExternal = externalmock.NewMockClient(s.ctrl)
	s.mockDice = dicemock.NewMockService(s.ctrl)
	
	// Setup repositories with real Redis
	draftRepo := draftrepo.NewRedisRepository(s.redisClient, clock.New())
	
	// Setup orchestrator
	charOrchestrator, err := character.New(&character.Config{
		CharacterDraftRepo: draftRepo,
		ExternalClient:     s.mockExternal,
		DiceService:        s.mockDice,
		IDGenerator:        &idgen.Static{ID: "char_123"},
		DraftIDGenerator:   &idgen.Static{ID: "draft_123"},
	})
	s.Require().NoError(err)
	
	// Setup handler
	handlerCfg := &v1alpha1.Config{
		CharacterService: charOrchestrator,
	}
	s.Require().NoError(handlerCfg.Validate())
	
	s.handler = v1alpha1.NewHandler(handlerCfg)
	
	// Test data
	s.draftID = "draft_123"
	s.playerID = "player_123"
}

func (s *FighterBundleIntegrationSuite) TearDownTest() {
	s.ctrl.Finish()
	s.miniRedis.Close()
}

func (s *FighterBundleIntegrationSuite) TestFighterMartialWeaponPlusShieldBundle() {
	// This test verifies that selecting a fighter's "martial weapon + shield" bundle
	// correctly expands to include BOTH the chosen weapon AND the shield
	
	// Step 1: Create a draft
	createReq := &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId: s.playerID,
		Name:     "Test Fighter",
	}
	
	createResp, err := s.handler.CreateDraft(s.ctx, createReq)
	s.Require().NoError(err)
	s.Require().NotNil(createResp)
	s.draftID = createResp.Draft.Id
	
	// Step 2: Update with fighter class
	// Mock the external client to return fighter data with equipment choices
	s.mockExternal.EXPECT().
		GetClassData(s.ctx, "fighter").
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:        constants.ClassFighter,
				Name:      "Fighter",
				HitDice:   10,
				HitPointsPerLevel: 6,
				EquipmentChoices: []class.EquipmentChoice{
					{
						Choose:      1,
						Description: "Choose your equipment",
						Options: []class.EquipmentOption{
							{
								// Option 0: Chain mail
								Items: []class.EquipmentData{
									{ItemID: "chain-mail", Quantity: 1},
								},
							},
							{
								// Option 1: Leather armor, longbow, 20 arrows
								Items: []class.EquipmentData{
									{ItemID: "leather-armor", Quantity: 1},
									{ItemID: "longbow", Quantity: 1},
									{ItemID: "arrow", Quantity: 20},
								},
							},
						},
					},
					{
						Choose:      1,
						Description: "Choose weapons and shield",
						Options: []class.EquipmentOption{
							{
								// Option 0: Martial weapon + shield
								// The API should expand this properly
								Items: []class.EquipmentData{
									{ItemID: "choose-martial-weapons", Quantity: 1},
									{ItemID: "shield", Quantity: 1},
								},
							},
							{
								// Option 1: Two martial weapons
								Items: []class.EquipmentData{
									{ItemID: "choose-martial-weapons", Quantity: 1},
									{ItemID: "choose-martial-weapons", Quantity: 1},
								},
							},
						},
					},
				},
			},
		}, nil).AnyTimes()

	// Update draft with fighter class
	updateClassReq := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: s.draftID,
		ClassId: "fighter",
	}
	
	updateClassResp, err := s.handler.UpdateClass(s.ctx, updateClassReq)
	s.Require().NoError(err)
	s.Require().NotNil(updateClassResp)
	
	// Step 3: Make equipment selections
	// Select option 0 from first choice (chain mail)
	// Select option 0 from second choice (martial weapon + shield) with longsword
	
	// First equipment choice - just chain mail
	makeChoice1Req := &dnd5ev1alpha1.MakeChoiceRequest{
		DraftId: s.draftID,
		Choice: &dnd5ev1alpha1.ChoiceData{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId: "fighter_equipment_1",
			Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
				Equipment: &dnd5ev1alpha1.EquipmentList{
					Items: []string{"chain-mail"},
				},
			},
		},
	}
	
	_, err = s.handler.MakeChoice(s.ctx, makeChoice1Req)
	s.Require().NoError(err)
	
	// Second equipment choice - martial weapon + shield bundle
	// When we select longsword, the shield should be automatically included
	makeChoice2Req := &dnd5ev1alpha1.MakeChoiceRequest{
		DraftId: s.draftID,
		Choice: &dnd5ev1alpha1.ChoiceData{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId: "fighter_equipment_2",
			Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
				Equipment: &dnd5ev1alpha1.EquipmentList{
					// This is what the frontend sends - just the weapon choice
					// The backend should expand this to include the shield
					Items: []string{"bundle_0:0:longsword", "bundle_0:1:shield"},
				},
			},
		},
	}
	
	_, err = s.handler.MakeChoice(s.ctx, makeChoice2Req)
	s.Require().NoError(err)
	
	// Step 4: Get the draft and verify equipment
	getDraftReq := &dnd5ev1alpha1.GetDraftRequest{
		DraftId: s.draftID,
	}
	
	getDraftResp, err := s.handler.GetDraft(s.ctx, getDraftReq)
	s.Require().NoError(err)
	s.Require().NotNil(getDraftResp)
	s.Require().NotNil(getDraftResp.Draft)
	
	// Find the equipment choices
	var equipment []string
	for _, choice := range getDraftResp.Draft.Choices {
		if choice.Category == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT {
			if eqList := choice.GetEquipment(); eqList != nil {
				equipment = append(equipment, eqList.Items...)
			}
		}
	}
	
	// Verify that BOTH the weapon and shield are in the equipment
	s.T().Logf("Equipment found: %v", equipment)
	
	// The expected behavior is that the backend expands the bundle
	// So we should see both items
	hasLongsword := false
	hasShield := false
	hasChainMail := false
	
	for _, item := range equipment {
		// The backend should have unpacked these
		if item == "longsword" || item == "bundle_0:0:longsword" {
			hasLongsword = true
		}
		if item == "shield" || item == "bundle_0:1:shield" {
			hasShield = true
		}
		if item == "chain-mail" {
			hasChainMail = true
		}
	}
	
	s.True(hasChainMail, "Should have chain mail from first choice")
	s.True(hasLongsword, "Should have longsword from martial weapon choice")
	s.True(hasShield, "Should have shield from bundle - THIS IS THE BUG WE'RE TESTING")
}

func (s *FighterBundleIntegrationSuite) TestFightingStylePersistence() {
	// This test verifies that fighting style selections persist across draft updates
	
	// Create draft with fighter
	createReq := &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId: s.playerID,
		Name:     "Fighter with Style",
	}
	
	createResp, err := s.handler.CreateDraft(s.ctx, createReq)
	s.Require().NoError(err)
	s.draftID = createResp.Draft.Id
	
	// Mock fighter class data with fighting style
	s.mockExternal.EXPECT().
		GetClassData(s.ctx, "fighter").
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:                constants.ClassFighter,
				Name:              "Fighter",
				HitDice:           10,
				HitPointsPerLevel: 6,
				FightingStyles: []string{
					"archery",
					"defense",
					"dueling",
					"great-weapon-fighting",
					"protection",
					"two-weapon-fighting",
				},
			},
		}, nil).AnyTimes()
	
	// Update with fighter class
	updateClassReq := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: s.draftID,
		ClassId: "fighter",
	}
	
	_, err = s.handler.UpdateClass(s.ctx, updateClassReq)
	s.Require().NoError(err)
	
	// Select fighting style
	makeFightingStyleReq := &dnd5ev1alpha1.MakeChoiceRequest{
		DraftId: s.draftID,
		Choice: &dnd5ev1alpha1.ChoiceData{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId: "fighter_fighting_style",
			Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
				FightingStyle: "defense",
			},
		},
	}
	
	_, err = s.handler.MakeChoice(s.ctx, makeFightingStyleReq)
	s.Require().NoError(err)
	
	// Get draft and verify fighting style persisted
	getDraftReq := &dnd5ev1alpha1.GetDraftRequest{
		DraftId: s.draftID,
	}
	
	getDraftResp, err := s.handler.GetDraft(s.ctx, getDraftReq)
	s.Require().NoError(err)
	
	// Find fighting style choice
	var fightingStyle string
	for _, choice := range getDraftResp.Draft.Choices {
		if choice.Category == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE {
			fightingStyle = choice.GetFightingStyle()
			break
		}
	}
	
	s.Equal("defense", fightingStyle, "Fighting style should persist")
	
	// Now update the race (simulating navigating back and forth)
	s.mockExternal.EXPECT().
		GetRaceData(s.ctx, "human").
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID: constants.RaceHuman,
			},
		}, nil).AnyTimes()
	
	updateRaceReq := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: s.draftID,
		RaceId:  "human",
	}
	
	_, err = s.handler.UpdateRace(s.ctx, updateRaceReq)
	s.Require().NoError(err)
	
	// Get draft again and verify fighting style STILL persisted
	getDraftResp2, err := s.handler.GetDraft(s.ctx, getDraftReq)
	s.Require().NoError(err)
	
	// Find fighting style choice again
	fightingStyle = ""
	for _, choice := range getDraftResp2.Draft.Choices {
		if choice.Category == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE {
			fightingStyle = choice.GetFightingStyle()
			break
		}
	}
	
	s.Equal("defense", fightingStyle, "Fighting style should persist after race update - THIS TESTS PERSISTENCE BUG")
}