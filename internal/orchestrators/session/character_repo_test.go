package session_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type CharacterRepositoryTestSuite struct {
	suite.Suite
	ctx      context.Context
	mockRepo *charactermock.MockRepository
	repo     sdk.CharacterRepository
}

func (s *CharacterRepositoryTestSuite) SetupTest() {
	s.ctx = context.Background()
	ctrl := gomock.NewController(s.T())
	s.mockRepo = charactermock.NewMockRepository(ctrl)
	s.repo = sessionorch.NewCharacterRepository(s.mockRepo)
}

func TestCharacterRepositorySuite(t *testing.T) {
	suite.Run(t, new(CharacterRepositoryTestSuite))
}

func (s *CharacterRepositoryTestSuite) TestGetCharacter_ReturnsSDKData() {
	want := &tkcharacter.Data{ID: "char-1", Name: "Alice"}
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{Data: want}}, nil,
	)

	got, err := s.repo.GetCharacter(s.ctx, "char-1")
	s.Require().NoError(err)
	s.Same(want, got)
}

func (s *CharacterRepositoryTestSuite) TestGetCharacter_NotFound_TranslatesToSDKSentinel() {
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{ID: "missing"}).Return(
		nil, apierr.NotFoundf("character with ID %s not found", "missing"),
	)

	_, err := s.repo.GetCharacter(s.ctx, "missing")
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdk.ErrNotFound)
}

func (s *CharacterRepositoryTestSuite) TestGetCharacter_StoredCharacterHasNoData_IsBadRepository() {
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{Data: nil}}, nil,
	)

	_, err := s.repo.GetCharacter(s.ctx, "char-1")
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdk.ErrBadRepository)
}

func (s *CharacterRepositoryTestSuite) TestGetCharacter_OtherFailure_PassesThrough() {
	boom := errors.New("redis is on fire")
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{ID: "char-1"}).Return(nil, boom)

	_, err := s.repo.GetCharacter(s.ctx, "char-1")
	s.Require().Error(err)
	s.Require().ErrorIs(err, boom)
	s.Require().NotErrorIs(err, sdk.ErrNotFound)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_PersistsToolkitDataIncludingAppearance() {
	color := uint32(0x123456)
	roughness := float32(0.33)
	appearance := &customization.Appearance{Hair: &customization.HairCustomization{
		Scalp:      &customization.StyleSelection{Kind: customization.StyleSelectionStyle, StyleRef: "unknown:hair:38"},
		FacialHair: &customization.StyleSelection{Kind: customization.StyleSelectionNone},
		ColorSRGB:  &color,
		Roughness:  &roughness,
	}}
	newData := &tkcharacter.Data{ID: "char-1", Name: "Alice", HitPoints: 5, Appearance: appearance}

	s.mockRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, in characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			s.Same(newData, in.Character.Data)
			s.Same(appearance, in.Character.Data.Appearance)
			return &characterrepo.UpdateOutput{Character: in.Character}, nil
		},
	)

	err := s.repo.SaveCharacter(s.ctx, newData)
	s.Require().NoError(err)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_NotFound_TranslatesToSDKSentinel() {
	data := &tkcharacter.Data{ID: "missing", Name: "Ghost"}
	s.mockRepo.EXPECT().Update(s.ctx, gomock.Any()).Return(
		nil, apierr.NotFoundf("character with ID %s not found", "missing"),
	)

	err := s.repo.SaveCharacter(s.ctx, data)
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdk.ErrNotFound)
	s.Require().NotErrorIs(err, sdk.ErrBadRepository)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_UpdateFailure_PassesThrough() {
	data := &tkcharacter.Data{ID: "char-1", Name: "Alice"}
	boom := errors.New("redis is on fire")
	s.mockRepo.EXPECT().Update(s.ctx, gomock.Any()).Return(nil, boom)

	err := s.repo.SaveCharacter(s.ctx, data)
	s.Require().Error(err)
	s.Require().ErrorIs(err, boom)
	s.Require().NotErrorIs(err, sdk.ErrNotFound)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_NilData_Errors() {
	err := s.repo.SaveCharacter(s.ctx, nil)
	s.Require().Error(err)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_EmptyID_Errors() {
	err := s.repo.SaveCharacter(s.ctx, &tkcharacter.Data{})
	s.Require().Error(err)
}
