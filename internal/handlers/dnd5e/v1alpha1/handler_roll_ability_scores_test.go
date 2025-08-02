package v1alpha1_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
)

type HandlerRollAbilityScoresTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharService *charactermock.MockService
	handler         *v1alpha1.Handler
}

func TestHandlerRollAbilityScoresTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerRollAbilityScoresTestSuite))
}

func (s *HandlerRollAbilityScoresTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerRollAbilityScoresTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerRollAbilityScoresTestSuite) TestRollAbilityScores_Success() {
	ctx := context.Background()
	draftID := "draft-123"

	// Mock response from orchestrator
	mockRolls := []*character.AbilityScoreRoll{
		{
			RollID:      "roll_1",
			Total:       16,
			Description: "Ability Score 1 (4d6_drop_lowest)",
			Dice:        []int32{6, 5, 5, 2},
			Dropped:     []int32{2},
		},
		{
			RollID:      "roll_2",
			Total:       14,
			Description: "Ability Score 2 (4d6_drop_lowest)",
			Dice:        []int32{5, 5, 4, 3},
			Dropped:     []int32{3},
		},
		{
			RollID:      "roll_3",
			Total:       13,
			Description: "Ability Score 3 (4d6_drop_lowest)",
			Dice:        []int32{6, 4, 3, 2},
			Dropped:     []int32{2},
		},
		{
			RollID:      "roll_4",
			Total:       12,
			Description: "Ability Score 4 (4d6_drop_lowest)",
			Dice:        []int32{5, 4, 3, 1},
			Dropped:     []int32{1},
		},
		{
			RollID:      "roll_5",
			Total:       10,
			Description: "Ability Score 5 (4d6_drop_lowest)",
			Dice:        []int32{4, 3, 3, 2},
			Dropped:     []int32{2},
		},
		{
			RollID:      "roll_6",
			Total:       15,
			Description: "Ability Score 6 (4d6_drop_lowest)",
			Dice:        []int32{6, 5, 4, 3},
			Dropped:     []int32{3},
		},
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	s.mockCharService.EXPECT().
		RollAbilityScores(ctx, &character.RollAbilityScoresInput{
			DraftID: draftID,
		}).
		Return(&character.RollAbilityScoresOutput{
			Rolls:     mockRolls,
			SessionID: "char_draft_" + draftID,
			ExpiresAt: expiresAt,
		}, nil)

	// Call the handler
	req := &dnd5ev1alpha1.RollAbilityScoresRequest{
		DraftId: draftID,
	}
	resp, err := s.handler.RollAbilityScores(ctx, req)

	// Verify response
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Rolls, 6)

	// Check first roll details
	firstRoll := resp.Rolls[0]
	s.Equal("roll_1", firstRoll.RollId)
	s.Equal(int32(16), firstRoll.Total)
	s.Equal([]int32{6, 5, 5, 2}, firstRoll.Dice)
	s.Equal(int32(2), firstRoll.Dropped)
	s.Equal("Ability Score 1 (4d6_drop_lowest)", firstRoll.Notation)

	// Check expires at
	s.Equal(expiresAt.Unix(), resp.ExpiresAt)
}

func (s *HandlerRollAbilityScoresTestSuite) TestRollAbilityScores_MissingDraftID() {
	ctx := context.Background()

	req := &dnd5ev1alpha1.RollAbilityScoresRequest{
		DraftId: "", // Missing draft ID
	}

	resp, err := s.handler.RollAbilityScores(ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "draft_id is required")
}

func (s *HandlerRollAbilityScoresTestSuite) TestRollAbilityScores_DraftNotFound() {
	ctx := context.Background()
	draftID := "non-existent"

	s.mockCharService.EXPECT().
		RollAbilityScores(ctx, &character.RollAbilityScoresInput{
			DraftID: draftID,
		}).
		Return(nil, errors.NotFound("draft not found"))

	req := &dnd5ev1alpha1.RollAbilityScoresRequest{
		DraftId: draftID,
	}
	resp, err := s.handler.RollAbilityScores(ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.NotFound, st.Code())
}

func (s *HandlerRollAbilityScoresTestSuite) TestRollAbilityScores_EmptyDropped() {
	// Test case where a roll has no dropped dice (e.g., 3d6 method)
	ctx := context.Background()
	draftID := "draft-456"

	mockRolls := []*character.AbilityScoreRoll{
		{
			RollID:      "roll_1",
			Total:       12,
			Description: "Ability Score 1 (3d6)",
			Dice:        []int32{4, 4, 4},
			Dropped:     []int32{}, // No dropped dice
		},
	}

	// Only return 1 roll for simplicity in this test
	for i := 2; i <= 6; i++ {
		mockRolls = append(mockRolls, &character.AbilityScoreRoll{
			RollID:      "roll_" + string(rune('0'+i)),
			Total:       10,
			Description: "Ability Score " + string(rune('0'+i)) + " (3d6)",
			Dice:        []int32{3, 3, 4},
			Dropped:     []int32{},
		})
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	s.mockCharService.EXPECT().
		RollAbilityScores(ctx, gomock.Any()).
		Return(&character.RollAbilityScoresOutput{
			Rolls:     mockRolls,
			SessionID: "char_draft_" + draftID,
			ExpiresAt: expiresAt,
		}, nil)

	req := &dnd5ev1alpha1.RollAbilityScoresRequest{
		DraftId: draftID,
	}
	resp, err := s.handler.RollAbilityScores(ctx, req)

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	
	// Check that dropped is 0 when no dice were dropped
	firstRoll := resp.Rolls[0]
	s.Equal(int32(0), firstRoll.Dropped)
}