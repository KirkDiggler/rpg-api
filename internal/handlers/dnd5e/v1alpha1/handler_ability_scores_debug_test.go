package v1alpha1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charmock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
)

type HandlerAbilityScoresDebugTestSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	handler     *v1alpha1.Handler
	mockService *charmock.MockService
	ctx         context.Context
}

func (s *HandlerAbilityScoresDebugTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = charmock.NewMockService(s.ctrl)
	s.ctx = context.Background()

	handler, err := v1alpha1.New(&v1alpha1.Config{
		CharacterService: s.mockService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerAbilityScoresDebugTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerAbilityScoresDebugTestSuite) TestRollAbilityScores_RequestStructure() {
	// Test what the web app is sending
	req := &dnd5ev1alpha1.RollAbilityScoresRequest{
		DraftId: "draft_2e56d910-44b1-480c-ba61-e5aa06894832",
		Method:  dnd5ev1alpha1.RollingMethod_ROLLING_METHOD_STANDARD,
	}

	s.T().Logf("RollAbilityScoresRequest: DraftId=%s, Method=%s", req.DraftId, req.Method)

	// Mock the service call
	s.mockService.EXPECT().
		RollAbilityScores(s.ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *character.RollAbilityScoresInput) (*character.RollAbilityScoresOutput, error) {
			s.T().Logf("Service RollAbilityScores called with DraftID=%s", input.DraftID)
			s.Equal("draft_2e56d910-44b1-480c-ba61-e5aa06894832", input.DraftID)
			
			return &character.RollAbilityScoresOutput{
				Rolls: []*character.AbilityScoreRoll{
					{RollID: "roll-_1754263196116218735_51cb35c2", Total: 16},
					{RollID: "roll-_1754263192857429962_e3fa154c", Total: 14},
					{RollID: "roll-_1754263196917499474_5df4060e", Total: 15},
					{RollID: "roll-_1754263195087745876_7568e804", Total: 12},
					{RollID: "roll-_1754263193879321903_53cbb854", Total: 13},
					{RollID: "roll-_1754263192789425709_789eceb9", Total: 10},
				},
				SessionID: "test-player",
			}, nil
		})

	resp, err := s.handler.RollAbilityScores(s.ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Len(resp.Rolls, 6)
	s.Equal("test-player", resp.SessionId)
}

func (s *HandlerAbilityScoresDebugTestSuite) TestUpdateAbilityScores_RequestStructure() {
	// Test what the web app is sending
	req := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: "draft_2e56d910-44b1-480c-ba61-e5aa06894832",
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments{
			RollAssignments: &dnd5ev1alpha1.RollAssignments{
				StrengthRollId:     "roll-_1754263196116218735_51cb35c2",
				DexterityRollId:    "roll-_1754263192857429962_e3fa154c",
				ConstitutionRollId: "roll-_1754263196917499474_5df4060e",
				IntelligenceRollId: "roll-_1754263195087745876_7568e804",
				WisdomRollId:       "roll-_1754263193879321903_53cbb854",
				CharismaRollId:     "roll-_1754263192789425709_789eceb9",
			},
		},
	}

	s.T().Logf("UpdateAbilityScoresRequest: DraftId=%s", req.DraftId)
	s.T().Logf("Roll assignments: STR=%s", req.GetRollAssignments().StrengthRollId)

	// Mock the service call - this should fail with session not found
	s.mockService.EXPECT().
		UpdateAbilityScores(s.ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *character.UpdateAbilityScoresInput) (*character.UpdateAbilityScoresOutput, error) {
			s.T().Logf("Service UpdateAbilityScores called with DraftID=%s", input.DraftID)
			s.Equal("draft_2e56d910-44b1-480c-ba61-e5aa06894832", input.DraftID)
			s.NotNil(input.RollAssignments)
			
			// Simulate the error the user is seeing
			return nil, status.Error(codes.NotFound, "failed to get dice session for player test-player: NOT_FOUND: failed to get dice session: NOT_FOUND: dice session not found")
		})

	resp, err := s.handler.UpdateAbilityScores(s.ctx, req)
	s.Require().Error(err)
	s.Nil(resp)
	
	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.NotFound, st.Code())
	s.Contains(st.Message(), "dice session not found")
}

func TestHandlerAbilityScoresDebugTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerAbilityScoresDebugTestSuite))
}