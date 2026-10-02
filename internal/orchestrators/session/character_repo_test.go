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
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

const testWorldID = "world-123"

type CharacterRepositoryTestSuite struct {
	suite.Suite
	ctx      context.Context
	mockRepo *charactermock.MockRepository
	repo     sdk.CharacterRepository
}

func (s *CharacterRepositoryTestSuite) SetupTest() {
	s.ctx = worldcontext.With(context.Background(), worldcontext.Value{WorldID: testWorldID})
	ctrl := gomock.NewController(s.T())
	s.mockRepo = charactermock.NewMockRepository(ctrl)
	s.repo = sessionorch.NewCharacterRepository(s.mockRepo)
}

func TestCharacterRepositorySuite(t *testing.T) {
	suite.Run(t, new(CharacterRepositoryTestSuite))
}

func (s *CharacterRepositoryTestSuite) TestGetCharacter_ReturnsSDKData() {
	want := &tkcharacter.Data{ID: "char-1", Name: "Alice", PlayerID: "player-1"}
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{WorldID: testWorldID, ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{WorldID: testWorldID, Data: want}}, nil,
	)

	got, err := s.repo.GetCharacter(s.ctx, "char-1")
	s.Require().NoError(err)
	s.Same(want, got)
}

func (s *CharacterRepositoryTestSuite) TestGetCharacter_NotFound_TranslatesToSDKSentinel() {
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{WorldID: testWorldID, ID: "missing"}).Return(
		nil, apierr.NotFoundf("character with ID %s not found", "missing"),
	)

	_, err := s.repo.GetCharacter(s.ctx, "missing")
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdk.ErrNotFound)
}

func (s *CharacterRepositoryTestSuite) TestGetCharacter_StoredCharacterHasNoData_IsBadRepository() {
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{WorldID: testWorldID, ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{Data: nil}}, nil,
	)

	_, err := s.repo.GetCharacter(s.ctx, "char-1")
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdk.ErrBadRepository)
}

func (s *CharacterRepositoryTestSuite) TestGetCharacter_OtherFailure_PassesThrough() {
	boom := errors.New("redis is on fire")
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{WorldID: testWorldID, ID: "char-1"}).Return(nil, boom)

	_, err := s.repo.GetCharacter(s.ctx, "char-1")
	s.Require().Error(err)
	s.Require().ErrorIs(err, boom)
	s.Require().NotErrorIs(err, sdk.ErrNotFound)
}

// TestGetCharacter_MissingTrustedWorld_FailsClosed proves the SDK adapter never
// guesses a world: without a trusted world on the context there is no read.
func (s *CharacterRepositoryTestSuite) TestGetCharacter_MissingTrustedWorld_FailsClosed() {
	_, err := s.repo.GetCharacter(context.Background(), "char-1")
	s.Require().Error(err)
	s.Require().NotErrorIs(err, sdk.ErrNotFound)
	s.Require().NotErrorIs(err, sdk.ErrBadRepository)
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
	newData := &tkcharacter.Data{ID: "char-1", Name: "Alice", PlayerID: "player-1", HitPoints: 5, Appearance: appearance}

	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{WorldID: testWorldID, ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{
			WorldID: testWorldID,
			Data:    &tkcharacter.Data{ID: "char-1", PlayerID: "player-1", Name: "Before"},
		}}, nil,
	)
	s.mockRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, in characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			// Only the toolkit data is replaced; the stored world is reused.
			s.Equal(testWorldID, in.Character.WorldID)
			s.Same(newData, in.Character.Data)
			s.Same(appearance, in.Character.Data.Appearance)
			return &characterrepo.UpdateOutput{Character: in.Character}, nil
		},
	)

	err := s.repo.SaveCharacter(s.ctx, newData)
	s.Require().NoError(err)
}

// TestSaveCharacter_PreservesStoredWorld proves the adapter writes under the
// world the record was loaded from, never a world the caller could name.
func (s *CharacterRepositoryTestSuite) TestSaveCharacter_PreservesStoredWorld() {
	newData := &tkcharacter.Data{ID: "char-1", Name: "Alice", PlayerID: "player-1"}
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{WorldID: testWorldID, ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{WorldID: testWorldID, Data: newData}}, nil,
	)
	s.mockRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, in characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			s.Equal(testWorldID, in.Character.WorldID)
			return &characterrepo.UpdateOutput{Character: in.Character}, nil
		},
	)

	s.Require().NoError(s.repo.SaveCharacter(s.ctx, newData))
}

// TestSaveCharacter_RefusesMissingRecord proves a save never creates a record:
// a party save for a character that does not exist in this world is NotFound.
func (s *CharacterRepositoryTestSuite) TestSaveCharacter_RefusesMissingRecord() {
	data := &tkcharacter.Data{ID: "missing", Name: "Ghost", PlayerID: "player-1"}
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{WorldID: testWorldID, ID: "missing"}).Return(
		nil, apierr.NotFoundf("character with ID %s not found", "missing"),
	)

	err := s.repo.SaveCharacter(s.ctx, data)
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdk.ErrNotFound)
	s.Require().NotErrorIs(err, sdk.ErrBadRepository)
}

// TestSaveCharacter_NotFound_TranslatesToSDKSentinel keeps the translation
// coverage for the SaveCharacter Get miss.
func (s *CharacterRepositoryTestSuite) TestSaveCharacter_NotFound_TranslatesToSDKSentinel() {
	data := &tkcharacter.Data{ID: "missing", Name: "Ghost", PlayerID: "player-1"}
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{WorldID: testWorldID, ID: "missing"}).Return(
		nil, apierr.NotFoundf("character with ID %s not found", "missing"),
	)

	err := s.repo.SaveCharacter(s.ctx, data)
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdk.ErrNotFound)
	s.Require().NotErrorIs(err, sdk.ErrBadRepository)
}

// TestSameWorldPartySaveAllowed proves the adapter does NOT restrict saves to
// the invoking player: the SDK may save another seated member's sheet in the
// same verified world, and this adapter has no caller-player notion at all.
func (s *CharacterRepositoryTestSuite) TestSameWorldPartySaveAllowed() {
	otherMember := &tkcharacter.Data{ID: "char-2", Name: "Bob", PlayerID: "player-2"}
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{WorldID: testWorldID, ID: "char-2"}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{
			WorldID: testWorldID,
			Data:    &tkcharacter.Data{ID: "char-2", PlayerID: "player-2", Name: "Before"},
		}}, nil,
	)
	s.mockRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, in characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			s.Equal(testWorldID, in.Character.WorldID)
			s.Equal("player-2", in.Character.Data.PlayerID)
			return &characterrepo.UpdateOutput{Character: in.Character}, nil
		},
	)

	s.Require().NoError(s.repo.SaveCharacter(s.ctx, otherMember))
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_UpdateFailure_PassesThrough() {
	data := &tkcharacter.Data{ID: "char-1", Name: "Alice", PlayerID: "player-1"}
	boom := errors.New("redis is on fire")
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{WorldID: testWorldID, ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{WorldID: testWorldID, Data: data}}, nil,
	)
	s.mockRepo.EXPECT().Update(s.ctx, gomock.Any()).Return(nil, boom)

	err := s.repo.SaveCharacter(s.ctx, data)
	s.Require().Error(err)
	s.Require().ErrorIs(err, boom)
	s.Require().NotErrorIs(err, sdk.ErrNotFound)
}

// TestSaveCharacter_MissingTrustedWorld_FailsClosed proves a save with no
// trusted world never reaches the repository at all.
func (s *CharacterRepositoryTestSuite) TestSaveCharacter_MissingTrustedWorld_FailsClosed() {
	data := &tkcharacter.Data{ID: "char-1", PlayerID: "player-1"}
	err := s.repo.SaveCharacter(context.Background(), data)
	s.Require().Error(err)
	s.Require().NotErrorIs(err, sdk.ErrNotFound)
	s.Require().NotErrorIs(err, sdk.ErrBadRepository)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_NilData_Errors() {
	err := s.repo.SaveCharacter(s.ctx, nil)
	s.Require().Error(err)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_EmptyID_Errors() {
	err := s.repo.SaveCharacter(s.ctx, &tkcharacter.Data{})
	s.Require().Error(err)
}
