package character

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
)

type HandlerTestSuite struct {
	suite.Suite
	ctrl             *gomock.Controller
	mockService      *charactermock.MockService
	handler          *Handler
	ctx              context.Context
}

func (s *HandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = charactermock.NewMockService(s.ctrl)
	s.ctx = context.Background()

	config := &HandlerConfig{
		CharacterService: s.mockService,
	}

	var err error
	s.handler, err = NewHandler(config)
	s.Require().NoError(err)
}

func (s *HandlerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerTestSuite) TestDeleteCharacter_Success() {
	characterID := "char-123"
	req := &dnd5ev1alpha1.DeleteCharacterRequest{
		CharacterId: characterID,
	}

	// Mock the service call
	s.mockService.EXPECT().
		DeleteCharacter(s.ctx, &character.DeleteCharacterInput{
			CharacterID: characterID,
		}).
		Return(&character.DeleteCharacterOutput{}, nil)

	// Call the handler
	resp, err := s.handler.DeleteCharacter(s.ctx, req)

	// Assert success
	s.NoError(err)
	s.NotNil(resp)
}

func (s *HandlerTestSuite) TestDeleteCharacter_InvalidRequest() {
	testCases := []struct {
		name string
		req  *dnd5ev1alpha1.DeleteCharacterRequest
		code codes.Code
		msg  string
	}{
		{
			name: "nil request",
			req:  nil,
			code: codes.InvalidArgument,
			msg:  "request is required",
		},
		{
			name: "empty character ID",
			req:  &dnd5ev1alpha1.DeleteCharacterRequest{CharacterId: ""},
			code: codes.InvalidArgument,
			msg:  "character_id is required",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := s.handler.DeleteCharacter(s.ctx, tc.req)

			s.Error(err)
			s.Nil(resp)

			st, ok := status.FromError(err)
			s.True(ok)
			s.Equal(tc.code, st.Code())
			s.Equal(tc.msg, st.Message())
		})
	}
}

func (s *HandlerTestSuite) TestDeleteCharacter_NotFound() {
	characterID := "char-404"
	req := &dnd5ev1alpha1.DeleteCharacterRequest{
		CharacterId: characterID,
	}

	// Mock the service to return not found
	s.mockService.EXPECT().
		DeleteCharacter(s.ctx, &character.DeleteCharacterInput{
			CharacterID: characterID,
		}).
		Return(nil, errors.NotFound("character not found"))

	// Call the handler
	resp, err := s.handler.DeleteCharacter(s.ctx, req)

	// Assert not found error
	s.Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.NotFound, st.Code())
	s.Equal("character not found", st.Message())
}

func (s *HandlerTestSuite) TestDeleteCharacter_InvalidArgument() {
	characterID := "char-123"
	req := &dnd5ev1alpha1.DeleteCharacterRequest{
		CharacterId: characterID,
	}

	// Mock the service to return invalid argument
	s.mockService.EXPECT().
		DeleteCharacter(s.ctx, &character.DeleteCharacterInput{
			CharacterID: characterID,
		}).
		Return(nil, errors.InvalidArgument("invalid character ID format"))

	// Call the handler
	resp, err := s.handler.DeleteCharacter(s.ctx, req)

	// Assert invalid argument error
	s.Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "invalid character ID format")
}

func (s *HandlerTestSuite) TestDeleteCharacter_InternalError() {
	characterID := "char-123"
	req := &dnd5ev1alpha1.DeleteCharacterRequest{
		CharacterId: characterID,
	}

	// Mock the service to return internal error
	s.mockService.EXPECT().
		DeleteCharacter(s.ctx, &character.DeleteCharacterInput{
			CharacterID: characterID,
		}).
		Return(nil, errors.Internal("database error"))

	// Call the handler
	resp, err := s.handler.DeleteCharacter(s.ctx, req)

	// Assert internal error
	s.Error(err)
	s.Nil(resp)

	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.Internal, st.Code())
	s.Equal("failed to delete character", st.Message())
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}