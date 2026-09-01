package session_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
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
	color := uint32(0x123456)
	roughness := float32(0.33)
	appearance := &entities.Appearance{Hair: &entities.HairCustomization{
		Scalp: &entities.StyleSelection{
			Kind:     entities.StyleSelectionKindStyle,
			StyleRef: "modular-fantasy-hero:hair:38",
		},
		FacialHair: &entities.StyleSelection{Kind: entities.StyleSelectionKindNone},
		ColorSRGB:  &color,
		Roughness:  &roughness,
	}}
	existing := &entities.Character{
		Data:       &tkcharacter.Data{ID: "char-1", Name: "Alice", HitPoints: 12},
		Appearance: appearance,
	}
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: existing}, nil,
	)
	s.mockRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, in characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			s.Same(newData, in.Character.Data)
			s.Same(existing.Appearance, in.Character.Appearance, "must preserve API-owned fields, not overwrite the whole entity")
			s.Equal("modular-fantasy-hero:hair:38", in.Character.Appearance.Hair.Scalp.StyleRef)
			s.Equal(entities.StyleSelectionKindNone, in.Character.Appearance.Hair.FacialHair.Kind)
			s.Equal(uint32(0x123456), *in.Character.Appearance.Hair.ColorSRGB)
			s.InDelta(0.33, *in.Character.Appearance.Hair.Roughness, 0.000001)
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

// The three below pin SaveCharacter's load failures. GetCharacter had a test
// for each of these shapes and SaveCharacter had none, which is how a panic
// and a mistranslated sentinel both survived in a file whose sibling method
// handles the identical cases correctly.

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_NotFound_TranslatesToSDKSentinel() {
	data := &tkcharacter.Data{ID: "missing", Name: "Ghost"}
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{ID: "missing"}).Return(
		nil, apierr.NotFoundf("character with ID %s not found", "missing"),
	)

	err := s.repo.SaveCharacter(s.ctx, data)
	s.Require().Error(err)
	// The point of the assertion: a vanished character must reach the Manager
	// as the repository contract's own sentinel, so a host can tell it from a
	// broken store. Before this it arrived as a wrapped store error.
	s.Require().ErrorIs(err, sdk.ErrNotFound)
	s.Require().NotErrorIs(err, sdk.ErrBadRepository)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_NilOutput_IsBadRepository() {
	data := &tkcharacter.Data{ID: "char-1", Name: "Alice"}
	// A successful Get handing back no output at all. This used to be
	// dereferenced unguarded -- the call PANICKED rather than returning
	// anything, so Update was never reached and no error was ever produced.
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{ID: "char-1"}).Return(nil, nil)

	err := s.repo.SaveCharacter(s.ctx, data)
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdk.ErrBadRepository)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_NilStoredCharacter_IsBadRepository() {
	data := &tkcharacter.Data{ID: "char-1", Name: "Alice"}
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{ID: "char-1"}).Return(
		&characterrepo.GetOutput{Character: nil}, nil,
	)

	// No Update is expected: writing here would persist a record whose
	// API-owned fields (Appearance) had been silently emptied, which is the
	// exact loss this method's load-then-merge exists to prevent. gomock fails
	// the test if Update is called at all.
	err := s.repo.SaveCharacter(s.ctx, data)
	s.Require().Error(err)
	s.Require().ErrorIs(err, sdk.ErrBadRepository)
}

func (s *CharacterRepositoryTestSuite) TestSaveCharacter_OtherLoadFailure_PassesThrough() {
	data := &tkcharacter.Data{ID: "char-1", Name: "Alice"}
	boom := errors.New("redis is on fire")
	s.mockRepo.EXPECT().Get(s.ctx, characterrepo.GetInput{ID: "char-1"}).Return(nil, boom)

	err := s.repo.SaveCharacter(s.ctx, data)
	s.Require().Error(err)
	s.Require().ErrorIs(err, boom)
	s.Require().NotErrorIs(err, sdk.ErrNotFound)
}
