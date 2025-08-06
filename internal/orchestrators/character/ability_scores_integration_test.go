package character_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	extmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	charmock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	dicesession "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type AbilityScoresIntegrationTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	orchestrator    *character.Orchestrator
	mockCharRepo    *charmock.MockRepository
	mockDraftRepo   *draftmock.MockRepository
	mockExtClient   *extmock.MockClient
	mockDiceService *dicemock.MockService
	mockIDGen       *idgenmock.MockGenerator
}

func (s *AbilityScoresIntegrationTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charmock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftmock.NewMockRepository(s.ctrl)
	s.mockExtClient = extmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)
	s.mockIDGen = idgenmock.NewMockGenerator(s.ctrl)

	// Create orchestrator
	mockDraftIDGen := idgenmock.NewMockGenerator(s.ctrl)
	cfg := &character.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		ExternalClient:     s.mockExtClient,
		DiceService:        s.mockDiceService,
		IDGenerator:        s.mockIDGen,
		DraftIDGenerator:   mockDraftIDGen,
	}
	orchestrator, err := character.New(cfg)
	s.Require().NoError(err)
	s.orchestrator = orchestrator
}

func (s *AbilityScoresIntegrationTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *AbilityScoresIntegrationTestSuite) TestRollAndAssignAbilityScores_FullFlow() {
	ctx := context.Background()
	draftID := "draft_test_123"
	playerID := "test-player"

	// Create a draft for testing
	testDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Character",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: constants.RaceHuman,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassFighter,
		},
		BackgroundChoice: constants.BackgroundSoldier,
	}

	// Step 1: Roll ability scores
	// Mock getting the draft
	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: testDraft}, nil)

	// Mock dice rolling - simulate what the dice service would return
	expiresAt := time.Now().Add(15 * time.Minute)
	mockSession := &dicesession.DiceSession{
		EntityID:  playerID,
		Context:   "ability_scores",
		ExpiresAt: expiresAt,
		Rolls: []dicesession.DiceRoll{
			{
				RollID:      "roll_str_123",
				Total:       16,
				Description: "Ability Score 1 (4d6_drop_lowest)",
				Dice:        []int32{6, 5, 5},
				Dropped:     []int32{2},
			},
			{
				RollID:      "roll_dex_123",
				Total:       14,
				Description: "Ability Score 2 (4d6_drop_lowest)",
				Dice:        []int32{5, 5, 4},
				Dropped:     []int32{3},
			},
			{
				RollID:      "roll_con_123",
				Total:       15,
				Description: "Ability Score 3 (4d6_drop_lowest)",
				Dice:        []int32{6, 5, 4},
				Dropped:     []int32{2},
			},
			{
				RollID:      "roll_int_123",
				Total:       12,
				Description: "Ability Score 4 (4d6_drop_lowest)",
				Dice:        []int32{5, 4, 3},
				Dropped:     []int32{1},
			},
			{
				RollID:      "roll_wis_123",
				Total:       13,
				Description: "Ability Score 5 (4d6_drop_lowest)",
				Dice:        []int32{6, 4, 3},
				Dropped:     []int32{2},
			},
			{
				RollID:      "roll_cha_123",
				Total:       10,
				Description: "Ability Score 6 (4d6_drop_lowest)",
				Dice:        []int32{4, 3, 3},
				Dropped:     []int32{2},
			},
		},
	}

	// Convert to pointer slice for the mock
	mockRolls := make([]*dicesession.DiceRoll, len(mockSession.Rolls))
	for i := range mockSession.Rolls {
		roll := mockSession.Rolls[i] // Copy the roll
		mockRolls[i] = &roll
	}

	s.mockDiceService.EXPECT().
		RollAbilityScores(ctx, &dice.RollAbilityScoresInput{
			EntityID: playerID,
			Method:   dice.MethodStandard,
		}).
		Return(&dice.RollAbilityScoresOutput{
			Rolls:   mockRolls,
			Session: mockSession,
		}, nil)

	// Execute roll ability scores
	rollOutput, err := s.orchestrator.RollAbilityScores(ctx, &character.RollAbilityScoresInput{
		DraftID: draftID,
	})
	s.Require().NoError(err)
	s.Require().NotNil(rollOutput)
	s.Require().Len(rollOutput.Rolls, 6)

	// Step 2: Assign the rolls to abilities
	// Mock getting the draft again
	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: testDraft}, nil)

	// Mock getting the dice session - this is where the issue might be
	s.mockDiceService.EXPECT().
		GetRollSession(ctx, &dice.GetRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		}).
		Return(&dice.GetRollSessionOutput{
			Session: mockSession,
		}, nil)

	// Mock clearing the session after use
	s.mockDiceService.EXPECT().
		ClearRollSession(ctx, &dice.ClearRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		}).
		Return(&dice.ClearRollSessionOutput{}, nil)

	// Mock updating the draft with ability scores
	updatedDraft := *testDraft // Copy the draft
	updatedDraft.AbilityScoreChoice = shared.AbilityScores{
		constants.STR: 16,
		constants.DEX: 14,
		constants.CON: 15,
		constants.INT: 12,
		constants.WIS: 13,
		constants.CHA: 10,
	}

	s.mockDraftRepo.EXPECT().
		Update(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			// Verify the ability scores were set correctly
			s.Equal(16, input.Draft.AbilityScoreChoice[constants.STR])
			s.Equal(14, input.Draft.AbilityScoreChoice[constants.DEX])
			s.Equal(15, input.Draft.AbilityScoreChoice[constants.CON])
			s.Equal(12, input.Draft.AbilityScoreChoice[constants.INT])
			s.Equal(13, input.Draft.AbilityScoreChoice[constants.WIS])
			s.Equal(10, input.Draft.AbilityScoreChoice[constants.CHA])
			return &draftrepo.UpdateOutput{Draft: &updatedDraft}, nil
		})

	// Execute update ability scores with roll assignments
	updateOutput, err := s.orchestrator.UpdateAbilityScores(ctx, &character.UpdateAbilityScoresInput{
		DraftID: draftID,
		RollAssignments: &character.RollAssignments{
			StrengthRollID:     "roll_str_123",
			DexterityRollID:    "roll_dex_123",
			ConstitutionRollID: "roll_con_123",
			IntelligenceRollID: "roll_int_123",
			WisdomRollID:       "roll_wis_123",
			CharismaRollID:     "roll_cha_123",
		},
	})

	s.Require().NoError(err)
	s.Require().NotNil(updateOutput)
	s.Require().NotNil(updateOutput.Draft)
	s.Equal(16, updateOutput.Draft.AbilityScoreChoice[constants.STR])
	s.Equal(14, updateOutput.Draft.AbilityScoreChoice[constants.DEX])
	s.Equal(15, updateOutput.Draft.AbilityScoreChoice[constants.CON])
	s.Equal(12, updateOutput.Draft.AbilityScoreChoice[constants.INT])
	s.Equal(13, updateOutput.Draft.AbilityScoreChoice[constants.WIS])
	s.Equal(10, updateOutput.Draft.AbilityScoreChoice[constants.CHA])
}

func (s *AbilityScoresIntegrationTestSuite) TestUpdateAbilityScores_SessionNotFound() {
	ctx := context.Background()
	draftID := "draft_no_session"
	playerID := "player_no_session"

	// Mock getting the draft
	testDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Character",
	}

	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: testDraft}, nil)

	// Mock dice session not found
	s.mockDiceService.EXPECT().
		GetRollSession(ctx, &dice.GetRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		}).
		Return(nil, errors.NotFound("dice session not found"))

	// Try to update ability scores without rolling first
	output, err := s.orchestrator.UpdateAbilityScores(ctx, &character.UpdateAbilityScoresInput{
		DraftID: draftID,
		RollAssignments: &character.RollAssignments{
			StrengthRollID:     "roll_1",
			DexterityRollID:    "roll_2",
			ConstitutionRollID: "roll_3",
			IntelligenceRollID: "roll_4",
			WisdomRollID:       "roll_5",
			CharismaRollID:     "roll_6",
		},
	})

	s.Require().Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "failed to get dice session")
}

func TestAbilityScoresIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(AbilityScoresIntegrationTestSuite))
}
