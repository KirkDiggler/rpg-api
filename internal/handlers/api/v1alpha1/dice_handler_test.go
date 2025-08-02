package v1alpha1_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/handlers/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	dicesession "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
)

type DiceHandlerTestSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockDice    *dicemock.MockService
	handler     *v1alpha1.DiceHandler
}

func TestDiceHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(DiceHandlerTestSuite))
}

func (s *DiceHandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockDice = dicemock.NewMockService(s.ctrl)

	handler, err := v1alpha1.NewDiceHandler(&v1alpha1.DiceHandlerConfig{
		DiceService: s.mockDice,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *DiceHandlerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *DiceHandlerTestSuite) TestRollDice_Success() {
	ctx := context.Background()
	entityID := "player_123"
	contextStr := "character_draft_456_abilities"
	notation := "4d6"

	// Mock dice service response
	mockRoll := &dicesession.DiceRoll{
		RollID:      "roll_abc",
		Notation:    notation,
		Dice:        []int32{6, 5, 4, 2},
		Total:       15, // 6+5+4 (dropped 2)
		Dropped:     []int32{2},
		Description: "Ability Score Roll",
		DiceTotal:   15,
		Modifier:    0,
	}

	mockSession := &dicesession.DiceSession{
		EntityID:  entityID,
		Context:   contextStr,
		Rolls:     []dicesession.DiceRoll{*mockRoll},
		ExpiresAt: time.Now().Add(15 * time.Minute),
		CreatedAt: time.Now(),
	}

	s.mockDice.EXPECT().
		RollDice(ctx, &dice.RollDiceInput{
			EntityID:    entityID,
			Context:     contextStr,
			Notation:    notation,
			Description: "",
		}).
		Return(&dice.RollDiceOutput{
			Roll:    mockRoll,
			Session: mockSession,
		}, nil)

	// Call handler
	req := &apiv1alpha1.RollDiceRequest{
		EntityId: entityID,
		Context:  contextStr,
		Notation: notation,
	}
	resp, err := s.handler.RollDice(ctx, req)

	// Verify response
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Rolls, 1)

	roll := resp.Rolls[0]
	s.Equal("roll_abc", roll.RollId)
	s.Equal(notation, roll.Notation)
	s.Equal([]int32{6, 5, 4, 2}, roll.Dice)
	s.Equal(int32(15), roll.Total)
	s.Equal([]int32{2}, roll.Dropped)
}

func (s *DiceHandlerTestSuite) TestRollDice_ValidationErrors() {
	ctx := context.Background()

	testCases := []struct {
		name     string
		req      *apiv1alpha1.RollDiceRequest
		errMsg   string
	}{
		{
			name: "missing entity_id",
			req: &apiv1alpha1.RollDiceRequest{
				Context:  "test",
				Notation: "4d6",
			},
			errMsg: "entity_id is required",
		},
		{
			name: "missing context",
			req: &apiv1alpha1.RollDiceRequest{
				EntityId: "player_123",
				Notation: "4d6",
			},
			errMsg: "context is required",
		},
		{
			name: "missing notation",
			req: &apiv1alpha1.RollDiceRequest{
				EntityId: "player_123",
				Context:  "test",
			},
			errMsg: "notation is required",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := s.handler.RollDice(ctx, tc.req)

			s.Require().Error(err)
			s.Nil(resp)

			st, ok := status.FromError(err)
			s.Require().True(ok)
			s.Equal(codes.InvalidArgument, st.Code())
			s.Contains(st.Message(), tc.errMsg)
		})
	}
}

func (s *DiceHandlerTestSuite) TestGetRollSession_Success() {
	ctx := context.Background()
	entityID := "player_123"
	contextStr := "character_draft_456_abilities"

	// Mock session with multiple rolls
	mockSession := &dicesession.DiceSession{
		EntityID: entityID,
		Context:  contextStr,
		Rolls: []dicesession.DiceRoll{
			{
				RollID:    "roll_1",
				Notation:  "4d6",
				Dice:      []int32{6, 5, 4, 3},
				Total:     15,
				Dropped:   []int32{3},
				DiceTotal: 15,
			},
			{
				RollID:    "roll_2",
				Notation:  "4d6",
				Dice:      []int32{5, 5, 3, 2},
				Total:     13,
				Dropped:   []int32{2},
				DiceTotal: 13,
			},
		},
		ExpiresAt: time.Now().Add(10 * time.Minute),
		CreatedAt: time.Now().Add(-5 * time.Minute),
	}

	s.mockDice.EXPECT().
		GetRollSession(ctx, &dice.GetRollSessionInput{
			EntityID: entityID,
			Context:  contextStr,
		}).
		Return(&dice.GetRollSessionOutput{
			Session: mockSession,
		}, nil)

	// Call handler
	req := &apiv1alpha1.GetRollSessionRequest{
		EntityId: entityID,
		Context:  contextStr,
	}
	resp, err := s.handler.GetRollSession(ctx, req)

	// Verify response
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Rolls, 2)
	s.Equal("roll_1", resp.Rolls[0].RollId)
	s.Equal("roll_2", resp.Rolls[1].RollId)
	s.Greater(resp.ExpiresAt, int64(0))
	s.Greater(resp.CreatedAt, int64(0))
}

func (s *DiceHandlerTestSuite) TestGetRollSession_NotFound() {
	ctx := context.Background()
	entityID := "player_123"
	contextStr := "non_existent"

	s.mockDice.EXPECT().
		GetRollSession(ctx, gomock.Any()).
		Return(nil, errors.NotFound("session not found"))

	req := &apiv1alpha1.GetRollSessionRequest{
		EntityId: entityID,
		Context:  contextStr,
	}
	resp, err := s.handler.GetRollSession(ctx, req)

	s.Require().Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.NotFound, st.Code())
}

func (s *DiceHandlerTestSuite) TestClearRollSession_Success() {
	ctx := context.Background()
	entityID := "player_123"
	contextStr := "character_draft_456_abilities"

	s.mockDice.EXPECT().
		ClearRollSession(ctx, &dice.ClearRollSessionInput{
			EntityID: entityID,
			Context:  contextStr,
		}).
		Return(&dice.ClearRollSessionOutput{
			RollsDeleted: 6,
		}, nil)

	req := &apiv1alpha1.ClearRollSessionRequest{
		EntityId: entityID,
		Context:  contextStr,
	}
	resp, err := s.handler.ClearRollSession(ctx, req)

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Equal("Roll session cleared successfully", resp.Message)
	s.Equal(int32(6), resp.RollsCleared)
}

func (s *DiceHandlerTestSuite) TestClearRollSession_ValidationErrors() {
	ctx := context.Background()

	testCases := []struct {
		name   string
		req    *apiv1alpha1.ClearRollSessionRequest
		errMsg string
	}{
		{
			name: "missing entity_id",
			req: &apiv1alpha1.ClearRollSessionRequest{
				Context: "test",
			},
			errMsg: "entity_id is required",
		},
		{
			name: "missing context",
			req: &apiv1alpha1.ClearRollSessionRequest{
				EntityId: "player_123",
			},
			errMsg: "context is required",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := s.handler.ClearRollSession(ctx, tc.req)

			s.Require().Error(err)
			s.Nil(resp)

			st, ok := status.FromError(err)
			s.Require().True(ok)
			s.Equal(codes.InvalidArgument, st.Code())
			s.Contains(st.Message(), tc.errMsg)
		})
	}
}