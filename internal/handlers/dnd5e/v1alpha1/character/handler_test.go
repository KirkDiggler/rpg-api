package character

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

type HandlerTestSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockService *charactermock.MockService
	handler     *Handler
	ctx         context.Context
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
		Return(nil, apierr.NotFound("character not found"))

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
		Return(nil, apierr.InvalidArgument("invalid character ID format"))

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
		Return(nil, apierr.Internal("database error"))

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

func (s *HandlerTestSuite) TestGetCharacter_IncludesEquipmentSlots() {
	characterID := "char-123"
	req := &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: characterID,
	}

	// Build character data with equipment slots populated
	charData := &toolkitchar.Data{
		ID:    characterID,
		Name:  "Test Fighter",
		Level: 1,
		EquipmentSlots: toolkitchar.EquipmentSlots{
			toolkitchar.SlotMainHand: "item-longsword",
			toolkitchar.SlotArmor:    "item-chainmail",
			toolkitchar.SlotOffHand:  "item-shield",
		},
	}

	s.mockService.EXPECT().
		GetCharacter(s.ctx, &character.GetCharacterInput{
			CharacterID: characterID,
		}).
		Return(&character.GetCharacterOutput{
			Character: &entities.Character{
				Data: charData,
			},
		}, nil)

	resp, err := s.handler.GetCharacter(s.ctx, req)

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Character)
	s.Require().NotNil(resp.Character.EquipmentSlots, "EquipmentSlots must be present in GetCharacter response")

	// Verify individual slots
	s.Require().NotNil(resp.Character.EquipmentSlots.MainHand)
	s.Equal("item-longsword", resp.Character.EquipmentSlots.MainHand.ItemId)
	s.Require().NotNil(resp.Character.EquipmentSlots.Armor)
	s.Equal("item-chainmail", resp.Character.EquipmentSlots.Armor.ItemId)
	s.Require().NotNil(resp.Character.EquipmentSlots.OffHand)
	s.Equal("item-shield", resp.Character.EquipmentSlots.OffHand.ItemId)
}

func (s *HandlerTestSuite) TestGetCharacter_EmptyCharacterId() {
	req := &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: "",
	}

	resp, err := s.handler.GetCharacter(s.ctx, req)

	s.Error(err)
	s.Nil(resp)
}

func (s *HandlerTestSuite) TestGetCharacter_ServiceError() {
	characterID := "char-404"
	req := &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: characterID,
	}

	s.mockService.EXPECT().
		GetCharacter(s.ctx, &character.GetCharacterInput{
			CharacterID: characterID,
		}).
		Return(nil, apierr.NotFound("character not found"))

	resp, err := s.handler.GetCharacter(s.ctx, req)

	s.Error(err)
	s.Nil(resp)
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}
