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
	v1alpha1 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type HandlerGetCharacterTestSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockService *charactermock.MockService
	handler     *v1alpha1.Handler
	ctx         context.Context
}

func TestHandlerGetCharacterTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerGetCharacterTestSuite))
}

func (s *HandlerGetCharacterTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = charactermock.NewMockService(s.ctrl)
	s.ctx = context.Background()

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerGetCharacterTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerGetCharacterTestSuite) TestGetCharacter_Success() {
	characterID := "char-123"
	
	// Mock character data
	mockCharacter := &toolkitchar.Data{
		ID:       characterID,
		PlayerID: "player-123",
		Name:     "Aragorn",
		Level:    5,
		RaceID:   constants.RaceHuman,
		ClassID:  constants.ClassRanger,
		HitPoints:    45,
		MaxHitPoints: 50,
		AbilityScores: shared.AbilityScores{
			constants.STR: 16,
			constants.DEX: 18,
			constants.CON: 14,
			constants.INT: 10,
			constants.WIS: 15,
			constants.CHA: 12,
		},
		Speed: 30,
		Size:  "Medium",
		Languages: []string{
			string(constants.LanguageCommon),
			string(constants.LanguageElvish),
		},
	}
	
	// Mock service call
	s.mockService.EXPECT().
		GetCharacter(s.ctx, &character.GetCharacterInput{
			CharacterID: characterID,
		}).
		Return(&character.GetCharacterOutput{
			Character: mockCharacter,
		}, nil)
	
	// Create request
	req := &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: characterID,
	}
	
	// Call handler
	resp, err := s.handler.GetCharacter(s.ctx, req)
	
	// Assert success
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Character)
	s.Equal(characterID, resp.Character.Id)
	s.Equal("Aragorn", resp.Character.Name)
	s.Equal(int32(5), resp.Character.Level)
	s.Equal(dnd5ev1alpha1.Race_RACE_HUMAN, resp.Character.Race)
	s.Equal(dnd5ev1alpha1.Class_CLASS_RANGER, resp.Character.Class)
	s.Equal(int32(45), resp.Character.CurrentHitPoints)
	
	// Check ability scores
	s.Equal(int32(16), resp.Character.AbilityScores.Strength)
	s.Equal(int32(18), resp.Character.AbilityScores.Dexterity)
	s.Equal(int32(14), resp.Character.AbilityScores.Constitution)
	s.Equal(int32(10), resp.Character.AbilityScores.Intelligence)
	s.Equal(int32(15), resp.Character.AbilityScores.Wisdom)
	s.Equal(int32(12), resp.Character.AbilityScores.Charisma)
	
	// Check combat stats
	s.Equal(int32(50), resp.Character.CombatStats.HitPointMaximum)
	s.Equal(int32(30), resp.Character.CombatStats.Speed)
	
	// Check languages
	s.Contains(resp.Character.Languages, dnd5ev1alpha1.Language_LANGUAGE_COMMON)
	s.Contains(resp.Character.Languages, dnd5ev1alpha1.Language_LANGUAGE_ELVISH)
}

func (s *HandlerGetCharacterTestSuite) TestGetCharacter_MissingCharacterID() {
	// Create request with empty character ID
	req := &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: "",
	}
	
	// Call handler
	resp, err := s.handler.GetCharacter(s.ctx, req)
	
	// Assert error
	s.Require().Error(err)
	s.Nil(resp)
	
	// Check gRPC status
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "character_id is required")
}

func (s *HandlerGetCharacterTestSuite) TestGetCharacter_NotFound() {
	characterID := "char-not-found"
	
	// Mock service call to return not found
	s.mockService.EXPECT().
		GetCharacter(s.ctx, &character.GetCharacterInput{
			CharacterID: characterID,
		}).
		Return(nil, errors.NotFoundf("character %s not found", characterID))
	
	// Create request
	req := &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: characterID,
	}
	
	// Call handler
	resp, err := s.handler.GetCharacter(s.ctx, req)
	
	// Assert error
	s.Require().Error(err)
	s.Nil(resp)
	
	// Check gRPC status
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.NotFound, st.Code())
	s.Contains(st.Message(), "character char-not-found not found")
}

func (s *HandlerGetCharacterTestSuite) TestGetCharacter_InternalError() {
	characterID := "char-123"
	
	// Mock service call to return internal error
	s.mockService.EXPECT().
		GetCharacter(s.ctx, &character.GetCharacterInput{
			CharacterID: characterID,
		}).
		Return(nil, errors.Internal("database connection failed"))
	
	// Create request
	req := &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: characterID,
	}
	
	// Call handler
	resp, err := s.handler.GetCharacter(s.ctx, req)
	
	// Assert error
	s.Require().Error(err)
	s.Nil(resp)
	
	// Check gRPC status
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.Internal, st.Code())
	s.Contains(st.Message(), "database connection failed")
}