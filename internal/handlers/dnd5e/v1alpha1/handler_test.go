package v1alpha1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

type HandlerTestSuite struct {
	suite.Suite
	ctrl             *gomock.Controller
	mockCharService  *charactermock.MockService
	handler          *v1alpha1.Handler
	ctx              context.Context
}

func (s *HandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)
	s.ctx = context.Background()

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerTestSuite) TestGetDraft_Success() {
	draftID := "draft-123"
	
	// Mock orchestrator response
	s.mockCharService.EXPECT().
		GetDraft(s.ctx, &character.GetDraftInput{
			DraftID: draftID,
		}).
		Return(&character.GetDraftOutput{
			Draft: &toolkitchar.DraftData{
				ID:       draftID,
				PlayerID: "player-456",
				Name:     "Gandalf",
			},
		}, nil)

	// Call handler
	resp, err := s.handler.GetDraft(s.ctx, &dnd5ev1alpha1.GetDraftRequest{
		DraftId: draftID,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal(draftID, resp.Draft.Id)
	s.Equal("player-456", resp.Draft.PlayerId)
	s.Equal("Gandalf", resp.Draft.Name)
}

func (s *HandlerTestSuite) TestGetDraft_EmptyDraftID() {
	// Call handler with empty draft ID
	resp, err := s.handler.GetDraft(s.ctx, &dnd5ev1alpha1.GetDraftRequest{
		DraftId: "",
	})

	// Assert error
	s.Error(err)
	s.Nil(resp)
	
	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "draft_id is required")
}

func (s *HandlerTestSuite) TestGetDraft_NotFound() {
	draftID := "draft-notfound"
	
	// Mock orchestrator response
	s.mockCharService.EXPECT().
		GetDraft(s.ctx, &character.GetDraftInput{
			DraftID: draftID,
		}).
		Return(nil, errors.NotFound("draft not found"))

	// Call handler
	resp, err := s.handler.GetDraft(s.ctx, &dnd5ev1alpha1.GetDraftRequest{
		DraftId: draftID,
	})

	// Assert error
	s.Error(err)
	s.Nil(resp)
	
	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.NotFound, st.Code())
	s.Contains(st.Message(), "draft not found")
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}