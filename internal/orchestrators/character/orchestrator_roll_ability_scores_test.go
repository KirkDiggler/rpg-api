package character_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	externalmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	dicesession "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

type OrchestratorRollAbilityScoresTestSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockDraft    *draftrepomock.MockRepository
	mockDice     *dicemock.MockService
	mockChar     *charactermock.MockRepository
	mockExternal *externalmock.MockClient
	mockIDGen    *idgenmock.MockGenerator
	orchestrator character.Service
}

func TestOrchestratorRollAbilityScoresTestSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorRollAbilityScoresTestSuite))
}

func (s *OrchestratorRollAbilityScoresTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockDraft = draftrepomock.NewMockRepository(s.ctrl)
	s.mockDice = dicemock.NewMockService(s.ctrl)
	s.mockChar = charactermock.NewMockRepository(s.ctrl)
	s.mockExternal = externalmock.NewMockClient(s.ctrl)
	s.mockIDGen = idgenmock.NewMockGenerator(s.ctrl)

	orchestrator, err := character.New(&character.Config{
		CharacterDraftRepo: s.mockDraft,
		DiceService:        s.mockDice,
		CharacterRepo:      s.mockChar,
		ExternalClient:     s.mockExternal,
		IDGenerator:        s.mockIDGen,
	})
	s.Require().NoError(err)
	s.orchestrator = orchestrator
}

func (s *OrchestratorRollAbilityScoresTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *OrchestratorRollAbilityScoresTestSuite) TestRollAbilityScores_Success() {
	ctx := context.Background()
	draftID := "draft-123"
	playerID := "player-456"

	// Mock draft exists
	mockDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Character",
	}
	s.mockDraft.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: mockDraft}, nil)

	// Mock dice rolling
	expiresAt := time.Now().Add(15 * time.Minute)
	mockSession := &dicesession.DiceSession{
		EntityID:  playerID,
		Context:   "ability_scores",
		ExpiresAt: expiresAt,
	}

	mockRolls := []*dicesession.DiceRoll{
		{
			RollID:      "roll_1",
			Total:       16,
			Description: "Ability Score 1 (4d6_drop_lowest)",
			Dice:        []int32{6, 5, 5},
			Dropped:     []int32{2},
		},
		{
			RollID:      "roll_2",
			Total:       14,
			Description: "Ability Score 2 (4d6_drop_lowest)",
			Dice:        []int32{5, 5, 4},
			Dropped:     []int32{3},
		},
		{
			RollID:      "roll_3",
			Total:       13,
			Description: "Ability Score 3 (4d6_drop_lowest)",
			Dice:        []int32{6, 4, 3},
			Dropped:     []int32{2},
		},
		{
			RollID:      "roll_4",
			Total:       12,
			Description: "Ability Score 4 (4d6_drop_lowest)",
			Dice:        []int32{5, 4, 3},
			Dropped:     []int32{1},
		},
		{
			RollID:      "roll_5",
			Total:       10,
			Description: "Ability Score 5 (4d6_drop_lowest)",
			Dice:        []int32{4, 3, 3},
			Dropped:     []int32{2},
		},
		{
			RollID:      "roll_6",
			Total:       15,
			Description: "Ability Score 6 (4d6_drop_lowest)",
			Dice:        []int32{6, 5, 4},
			Dropped:     []int32{3},
		},
	}

	s.mockDice.EXPECT().
		RollAbilityScores(ctx, &dice.RollAbilityScoresInput{
			EntityID: playerID,
			Method:   dice.MethodStandard,
		}).
		Return(&dice.RollAbilityScoresOutput{
			Rolls:   mockRolls,
			Session: mockSession,
		}, nil)

	// Call the method
	output, err := s.orchestrator.RollAbilityScores(ctx, &character.RollAbilityScoresInput{
		DraftID: draftID,
	})

	// Verify results
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().Len(output.Rolls, 6)
	s.Equal(playerID, output.SessionID)
	s.Equal(expiresAt, output.ExpiresAt)

	// Check first roll
	firstRoll := output.Rolls[0]
	s.Equal("roll_1", firstRoll.RollID)
	s.Equal(int32(16), firstRoll.Total)
	s.Equal([]int32{6, 5, 5}, firstRoll.Dice)
	s.Equal([]int32{2}, firstRoll.Dropped)
}

func (s *OrchestratorRollAbilityScoresTestSuite) TestRollAbilityScores_MissingDraftID() {
	ctx := context.Background()

	output, err := s.orchestrator.RollAbilityScores(ctx, &character.RollAbilityScoresInput{
		DraftID: "",
	})

	s.Require().Error(err)
	s.Nil(output)
	s.True(errors.IsInvalidArgument(err))
	s.Contains(err.Error(), "draft ID is required")
}

func (s *OrchestratorRollAbilityScoresTestSuite) TestRollAbilityScores_DraftNotFound() {
	ctx := context.Background()
	draftID := "non-existent"

	s.mockDraft.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(nil, errors.NotFound("draft not found"))

	output, err := s.orchestrator.RollAbilityScores(ctx, &character.RollAbilityScoresInput{
		DraftID: draftID,
	})

	s.Require().Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "failed to get draft")
}

func (s *OrchestratorRollAbilityScoresTestSuite) TestRollAbilityScores_CustomMethod() {
	ctx := context.Background()
	draftID := "draft-789"
	playerID := "player-789"

	// Mock draft exists
	mockDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
	}
	s.mockDraft.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: mockDraft}, nil)

	// Mock dice rolling with custom method
	s.mockDice.EXPECT().
		RollAbilityScores(ctx, &dice.RollAbilityScoresInput{
			EntityID: playerID,
			Method:   dice.MethodClassic, // 3d6 method
		}).
		Return(&dice.RollAbilityScoresOutput{
			Rolls: []*dicesession.DiceRoll{
				{RollID: "roll_1", Total: 10, Dice: []int32{3, 3, 4}, Dropped: []int32{}},
				{RollID: "roll_2", Total: 11, Dice: []int32{4, 3, 4}, Dropped: []int32{}},
				{RollID: "roll_3", Total: 12, Dice: []int32{4, 4, 4}, Dropped: []int32{}},
				{RollID: "roll_4", Total: 9, Dice: []int32{3, 3, 3}, Dropped: []int32{}},
				{RollID: "roll_5", Total: 13, Dice: []int32{5, 4, 4}, Dropped: []int32{}},
				{RollID: "roll_6", Total: 8, Dice: []int32{2, 3, 3}, Dropped: []int32{}},
			},
			Session: &dicesession.DiceSession{
				ExpiresAt: time.Now().Add(15 * time.Minute),
			},
		}, nil)

	// Call with custom method
	output, err := s.orchestrator.RollAbilityScores(ctx, &character.RollAbilityScoresInput{
		DraftID: draftID,
		Method:  dice.MethodClassic,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().Len(output.Rolls, 6)

	// Verify no dropped dice for 3d6 method
	for _, roll := range output.Rolls {
		s.Empty(roll.Dropped)
	}
}

func (s *OrchestratorRollAbilityScoresTestSuite) TestRollAbilityScores_DiceServiceError() {
	ctx := context.Background()
	draftID := "draft-error"
	playerID := "player-error"

	// Mock draft exists
	s.mockDraft.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: &toolkitchar.DraftData{ID: draftID, PlayerID: playerID}}, nil)

	// Mock dice service error
	s.mockDice.EXPECT().
		RollAbilityScores(ctx, &dice.RollAbilityScoresInput{
			EntityID: playerID,
			Method:   dice.MethodStandard,
		}).
		Return(nil, errors.Internal("dice service error"))

	output, err := s.orchestrator.RollAbilityScores(ctx, &character.RollAbilityScoresInput{
		DraftID: draftID,
	})

	s.Require().Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "failed to roll ability scores")
}
