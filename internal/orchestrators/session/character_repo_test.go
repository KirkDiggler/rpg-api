package session_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	"github.com/stretchr/testify/suite"
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

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_MergesDataOntoExistingEntity() {
	newData := &tkcharacter.Data{ID: "char-1", Name: "Alice", HitPoints: 5}
	existing := &entities.Character{
		Data:       &tkcharacter.Data{ID: "char-1", Name: "Alice", HitPoints: 12},
		Appearance: &entities.Appearance{},
	}
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: existing}, nil,
	)
	s.mockRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, in characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			s.Same(newData, in.Character.Data)
			s.Same(existing.Appearance, in.Character.Appearance, "must preserve API-owned fields, not overwrite the whole entity")
			return &characterrepo.UpdateOutput{Character: in.Character}, nil
		},
	)

	err := s.repo.SaveCharacter(s.ctx, newData)
	s.Require().NoError(err)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_NilData_Errors() {
	err := s.repo.SaveCharacter(s.ctx, nil)
	s.Require().Error(err)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_EmptyID_Errors() {
	err := s.repo.SaveCharacter(s.ctx, &tkcharacter.Data{})
	s.Require().Error(err)
}
