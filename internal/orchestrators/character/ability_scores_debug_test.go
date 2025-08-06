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
)

type AbilityScoresDebugTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	orchestrator    *character.Orchestrator
	mockCharRepo    *charmock.MockRepository
	mockDraftRepo   *draftmock.MockRepository
	mockExtClient   *extmock.MockClient
	mockDiceService *dicemock.MockService
	mockIDGen       *idgenmock.MockGenerator
}

func (s *AbilityScoresDebugTestSuite) SetupTest() {
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

func (s *AbilityScoresDebugTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *AbilityScoresDebugTestSuite) TestDebugEntityIDAndContext() {
	ctx := context.Background()

	// Test data matching what the web app is using
	draftID := "draft_2e56d910-44b1-480c-ba61-e5aa06894832"
	playerID := "test-player"

	s.T().Logf("Testing with draftID: %s, playerID: %s", draftID, playerID)

	// Create test draft
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

	// Step 1: Test RollAbilityScores
	s.T().Log("=== Testing RollAbilityScores ===")

	// Mock getting the draft
	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: testDraft}, nil)

	// Create expected dice session
	expiresAt := time.Now().Add(15 * time.Minute)
	expectedSession := &dicesession.DiceSession{
		EntityID:  playerID,         // Should use player ID
		Context:   "ability_scores", // Should use fixed context
		ExpiresAt: expiresAt,
		Rolls: []dicesession.DiceRoll{
			{RollID: "roll-_1754263196116218735_51cb35c2", Total: 16},
			{RollID: "roll-_1754263192857429962_e3fa154c", Total: 14},
			{RollID: "roll-_1754263196917499474_5df4060e", Total: 15},
			{RollID: "roll-_1754263195087745876_7568e804", Total: 12},
			{RollID: "roll-_1754263193879321903_53cbb854", Total: 13},
			{RollID: "roll-_1754263192789425709_789eceb9", Total: 10},
		},
	}

	// Mock dice rolling - verify the correct parameters
	s.mockDiceService.EXPECT().
		RollAbilityScores(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *dice.RollAbilityScoresInput) (*dice.RollAbilityScoresOutput, error) {
			s.T().Logf("RollAbilityScores called with EntityID: %s, Method: %s", input.EntityID, input.Method)
			s.Equal(playerID, input.EntityID, "EntityID should be player ID")
			s.Equal(dice.MethodStandard, input.Method)

			rolls := make([]*dicesession.DiceRoll, len(expectedSession.Rolls))
			for i := range expectedSession.Rolls {
				roll := expectedSession.Rolls[i]
				rolls[i] = &roll
			}

			return &dice.RollAbilityScoresOutput{
				Rolls:   rolls,
				Session: expectedSession,
			}, nil
		})

	// Execute RollAbilityScores
	rollOutput, err := s.orchestrator.RollAbilityScores(ctx, &character.RollAbilityScoresInput{
		DraftID: draftID,
	})
	s.Require().NoError(err)
	s.Equal(playerID, rollOutput.SessionID)

	// Step 2: Test UpdateAbilityScores
	s.T().Log("=== Testing UpdateAbilityScores ===")

	// Mock getting the draft again
	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: testDraft}, nil)

	// Mock getting the dice session - verify the correct parameters
	s.mockDiceService.EXPECT().
		GetRollSession(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *dice.GetRollSessionInput) (*dice.GetRollSessionOutput, error) {
			s.T().Logf("GetRollSession called with EntityID: %s, Context: %s", input.EntityID, input.Context)
			s.Equal(playerID, input.EntityID, "EntityID should be player ID")
			s.Equal("ability_scores", input.Context, "Context should be 'ability_scores'")

			return &dice.GetRollSessionOutput{
				Session: expectedSession,
			}, nil
		})

	// Mock clearing the session
	s.mockDiceService.EXPECT().
		ClearRollSession(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *dice.ClearRollSessionInput) (*dice.ClearRollSessionOutput, error) {
			s.T().Logf("ClearRollSession called with EntityID: %s, Context: %s", input.EntityID, input.Context)
			s.Equal(playerID, input.EntityID, "EntityID should be player ID")
			s.Equal("ability_scores", input.Context, "Context should be 'ability_scores'")
			return &dice.ClearRollSessionOutput{}, nil
		})

	// Mock updating the draft
	s.mockDraftRepo.EXPECT().
		Update(ctx, gomock.Any()).
		Return(&draftrepo.UpdateOutput{Draft: testDraft}, nil)

	// Execute UpdateAbilityScores with roll assignments
	_, err = s.orchestrator.UpdateAbilityScores(ctx, &character.UpdateAbilityScoresInput{
		DraftID: draftID,
		RollAssignments: &character.RollAssignments{
			StrengthRollID:     "roll-_1754263196116218735_51cb35c2",
			DexterityRollID:    "roll-_1754263192857429962_e3fa154c",
			ConstitutionRollID: "roll-_1754263196917499474_5df4060e",
			IntelligenceRollID: "roll-_1754263195087745876_7568e804",
			WisdomRollID:       "roll-_1754263193879321903_53cbb854",
			CharismaRollID:     "roll-_1754263192789425709_789eceb9",
		},
	})

	s.Require().NoError(err)

	s.T().Log("=== Summary ===")
	s.T().Logf("Both methods correctly use:")
	s.T().Logf("- EntityID: %s (player ID)", playerID)
	s.T().Logf("- Context: ability_scores")
}

func (s *AbilityScoresDebugTestSuite) TestSessionNotFoundScenario() {
	ctx := context.Background()
	draftID := "draft_test"
	playerID := "test-player"

	testDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Character",
	}

	// Mock getting the draft
	s.mockDraftRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: testDraft}, nil)

	// Mock dice session not found - this is what the user is seeing
	s.mockDiceService.EXPECT().
		GetRollSession(ctx, &dice.GetRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		}).
		Return(nil, errors.NotFound("dice session not found"))

	// Try to update ability scores
	_, err := s.orchestrator.UpdateAbilityScores(ctx, &character.UpdateAbilityScoresInput{
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
	s.Contains(err.Error(), "failed to get dice session for player test-player")
	s.Contains(err.Error(), "dice session not found")

	s.T().Log("Error matches what user is seeing in the logs")
}

func TestAbilityScoresDebugTestSuite(t *testing.T) {
	suite.Run(t, new(AbilityScoresDebugTestSuite))
}
