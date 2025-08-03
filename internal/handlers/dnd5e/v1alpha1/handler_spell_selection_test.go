package v1alpha1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	v1alpha1 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type SpellSelectionTestSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockService *charactermock.MockService
	handler     *v1alpha1.Handler
	ctx         context.Context
}

func (s *SpellSelectionTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = charactermock.NewMockService(s.ctrl)
	s.ctx = context.Background()

	// Create handler with mock service
	handlerCfg := &v1alpha1.HandlerConfig{
		CharacterService: s.mockService,
	}
	handler, err := v1alpha1.NewHandler(handlerCfg)
	s.Require().NoError(err)
	s.handler = handler
}

func (s *SpellSelectionTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *SpellSelectionTestSuite) TestUpdateClass_WizardAddsSpellChoices() {
	// Arrange
	draftID := "draft_wizard_test"
	playerID := "player_123"

	// Create a draft that will be returned after update
	updatedDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Wizard",
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassWizard,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category: shared.ChoiceCantrips,
				Source:   shared.SourceClass,
				ChoiceID: "wizard_cantrips",
			},
			{
				Category: shared.ChoiceSpells,
				Source:   shared.SourceClass,
				ChoiceID: "wizard_spells",
			},
		},
	}

	// Expect the UpdateClass call
	s.mockService.EXPECT().
		UpdateClass(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input *character.UpdateClassInput) (*character.UpdateClassOutput, error) {
			// Verify the input
			s.Equal(draftID, input.DraftID)
			s.Equal(constants.ClassWizard, input.ClassID)

			return &character.UpdateClassOutput{
				Draft:    updatedDraft,
				Warnings: nil,
			}, nil
		})

	// Act
	req := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_WIZARD,
	}
	resp, err := s.handler.UpdateClass(s.ctx, req)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Draft)

	// Check that choices were converted properly
	s.Require().Len(resp.Draft.Choices, 2, "Should have 2 choices (cantrips and spells)")

	// Find and verify cantrip choice
	var hasCantrips, hasSpells bool
	for _, choice := range resp.Draft.Choices {
		switch choice.Category {
		case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS:
			hasCantrips = true
			s.Equal(dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, choice.Source)
			s.Equal("wizard_cantrips", choice.ChoiceId)
		case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS:
			hasSpells = true
			s.Equal(dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, choice.Source)
			s.Equal("wizard_spells", choice.ChoiceId)
		}
	}

	s.True(hasCantrips, "Should have cantrips choice")
	s.True(hasSpells, "Should have spells choice")
}

func (s *SpellSelectionTestSuite) TestUpdateClass_SorcererAddsSpellChoices() {
	// Arrange
	draftID := "draft_sorcerer_test"
	playerID := "player_456"

	// Create a draft that will be returned after update
	updatedDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Sorcerer",
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassSorcerer,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category: shared.ChoiceCantrips,
				Source:   shared.SourceClass,
				ChoiceID: "sorcerer_cantrips",
			},
			{
				Category: shared.ChoiceSpells,
				Source:   shared.SourceClass,
				ChoiceID: "sorcerer_spells",
			},
		},
	}

	// Expect the UpdateClass call
	s.mockService.EXPECT().
		UpdateClass(gomock.Any(), gomock.Any()).
		Return(&character.UpdateClassOutput{
			Draft:    updatedDraft,
			Warnings: nil,
		}, nil)

	// Act
	req := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_SORCERER,
	}
	resp, err := s.handler.UpdateClass(s.ctx, req)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Draft)
	s.Require().Len(resp.Draft.Choices, 2, "Sorcerer should have 2 choices")
}

func (s *SpellSelectionTestSuite) TestUpdateClass_ClericOnlyAddsCantrips() {
	// Arrange
	draftID := "draft_cleric_test"
	playerID := "player_789"

	// Create a draft that will be returned after update
	// Clerics prepare spells, so they only get cantrip choices
	updatedDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Cleric",
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassCleric,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category: shared.ChoiceCantrips,
				Source:   shared.SourceClass,
				ChoiceID: "cleric_cantrips",
			},
		},
	}

	// Expect the UpdateClass call
	s.mockService.EXPECT().
		UpdateClass(gomock.Any(), gomock.Any()).
		Return(&character.UpdateClassOutput{
			Draft:    updatedDraft,
			Warnings: nil,
		}, nil)

	// Act
	req := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_CLERIC,
	}
	resp, err := s.handler.UpdateClass(s.ctx, req)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Draft)
	s.Require().Len(resp.Draft.Choices, 1, "Cleric should only have cantrip choice")

	// Verify it's a cantrip choice
	choice := resp.Draft.Choices[0]
	s.Equal(dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS, choice.Category)
	s.Equal(dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS, choice.Source)
	s.Equal("cleric_cantrips", choice.ChoiceId)
}

func (s *SpellSelectionTestSuite) TestUpdateClass_FighterNoSpellChoices() {
	// Arrange
	draftID := "draft_fighter_test"
	playerID := "player_999"

	// Fighter should not get spell choices
	updatedDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: playerID,
		Name:     "Test Fighter",
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassFighter,
		},
		Choices: []toolkitchar.ChoiceData{}, // No spell choices
	}

	// Expect the UpdateClass call
	s.mockService.EXPECT().
		UpdateClass(gomock.Any(), gomock.Any()).
		Return(&character.UpdateClassOutput{
			Draft:    updatedDraft,
			Warnings: nil,
		}, nil)

	// Act
	req := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_FIGHTER,
	}
	resp, err := s.handler.UpdateClass(s.ctx, req)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Draft)
	s.Empty(resp.Draft.Choices, "Fighter should have no spell choices")
}

func TestSpellSelectionTestSuite(t *testing.T) {
	suite.Run(t, new(SpellSelectionTestSuite))
}