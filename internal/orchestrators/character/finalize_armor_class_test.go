package character

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// completeDraftFixture is a real complete dwarf fighter draft, captured from
// the integration suite's gRPC creation flow (completeDwarfFighterDraft).
func (s *OrchestratorTestSuite) completeDraftFixture() *character.DraftData {
	raw, err := os.ReadFile("testdata/complete_dwarf_fighter_draft.json")
	s.Require().NoError(err)
	var draft character.DraftData
	s.Require().NoError(json.Unmarshal(raw, &draft))
	return &draft
}

func (s *OrchestratorTestSuite) expectCompleteDraft() *character.DraftData {
	draft := s.completeDraftFixture()
	s.mockDraftRepo.EXPECT().
		Get(s.ctx, characterdraft.GetInput{ID: s.testDraftID}).
		Return(&characterdraft.GetOutput{Draft: &entities.CharacterDraft{Data: draft}}, nil)
	s.mockIDGen.EXPECT().Generate().Return(s.testCharacterID)
	return draft
}

// TestFinalizeDraft_AFoldRefusalSavesNothingAndKeepsTheDraft: the armour
// class of the sheet about to be saved is folded before anything is written,
// so a refusal leaves no unreadable character and the draft standing for a
// retry. The absent Create and Delete expectations are the no-write proof:
// gomock fails the test on either call.
func (s *OrchestratorTestSuite) TestFinalizeDraft_AFoldRefusalSavesNothingAndKeepsTheDraft() {
	s.expectCompleteDraft()
	refusal := errors.New("door refused the sheet")
	var folded *character.Data
	s.orchestrator.foldAC = func(_ context.Context, in *projectArmorClassInput) (*projectArmorClassOutput, error) {
		folded = in.Data
		return nil, refusal
	}

	out, err := s.orchestrator.FinalizeDraft(s.ctx, &FinalizeDraftInput{DraftID: s.testDraftID})

	s.Require().Error(err)
	s.Nil(out)
	s.ErrorIs(err, refusal)
	s.True(apierr.IsInternal(err), "%v", err)
	s.Require().NotNil(folded, "the fold was asked")
	s.Equal(s.testCharacterID, folded.ID, "of the sheet that would have been saved")
}

// TestFinalizeDraft_ReturnsTheFoldOfTheSavedSheet: the response's armour class
// is the door's fold of exactly the record handed to Create, and that record
// carries no armour class.
func (s *OrchestratorTestSuite) TestFinalizeDraft_ReturnsTheFoldOfTheSavedSheet() {
	s.expectCompleteDraft()
	var saved *character.Data
	gomock.InOrder(
		s.mockCharacterRepo.EXPECT().
			Create(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, in characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
				saved = in.Character.Data
				return &characterrepo.CreateOutput{Character: in.Character}, nil
			}),
		s.mockDraftRepo.EXPECT().
			Delete(s.ctx, characterdraft.DeleteInput{ID: s.testDraftID}).
			Return(&characterdraft.DeleteOutput{}, nil),
	)

	out, err := s.orchestrator.FinalizeDraft(s.ctx, &FinalizeDraftInput{DraftID: s.testDraftID})

	s.Require().NoError(err)
	s.Require().NotNil(out.ArmorClass)
	s.Require().NotNil(saved)
	want, err := projectArmorClass(s.ctx, &projectArmorClassInput{Data: saved})
	s.Require().NoError(err)
	s.Equal(want.ArmorClass.Total, out.ArmorClass.Total)
	written, err := json.Marshal(saved)
	s.Require().NoError(err)
	s.NotContains(string(written), "armor_class")
}
