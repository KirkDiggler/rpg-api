package character_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	extmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	charmock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type SpellSelectionOrchestratorTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	orchestrator    *character.Orchestrator
	mockCharRepo    *charmock.MockRepository
	mockDraftRepo   *draftmock.MockRepository
	mockExtClient   *extmock.MockClient
	mockDiceService *dicemock.MockService
	ctx             context.Context
}

func (s *SpellSelectionOrchestratorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charmock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftmock.NewMockRepository(s.ctrl)
	s.mockExtClient = extmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)
	s.ctx = context.Background()

	// Create orchestrator
	cfg := &character.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		ExternalClient:     s.mockExtClient,
		DiceService:        s.mockDiceService,
		IDGenerator:        &mockIDGenerator{},
		DraftIDGenerator:   &mockIDGenerator{},
	}
	orch, err := character.New(cfg)
	s.Require().NoError(err)
	s.orchestrator = orch
}

func (s *SpellSelectionOrchestratorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *SpellSelectionOrchestratorTestSuite) TestUpdateClass_WizardAddsSpellAndCantripChoices() {
	// Arrange
	draftID := "draft_123"
	existingDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Gandalf",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: constants.RaceHuman,
		},
		Choices: []toolkitchar.ChoiceData{
			// Existing race choice
			{
				Category: shared.ChoiceLanguages,
				Source:   shared.SourceRace,
				ChoiceID: "human_languages",
			},
		},
	}

	// Mock the Get call
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: existingDraft}, nil)

	// Mock the Update call and capture the draft
	var savedDraft *toolkitchar.DraftData
	s.mockDraftRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			savedDraft = input.Draft
			return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
		})

	// Act
	input := &character.UpdateClassInput{
		DraftID: draftID,
		ClassID: constants.ClassWizard,
		Choices: nil, // No additional choices from handler
	}
	output, err := s.orchestrator.UpdateClass(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.Draft)

	// Verify the class was set
	s.Equal(constants.ClassWizard, output.Draft.ClassChoice.ClassID)

	// Verify choices were added
	s.Require().NotNil(savedDraft, "Draft should have been saved")

	// Should have 3 choices: 1 race choice + 2 class choices (cantrips + spells)
	s.Require().Len(savedDraft.Choices, 3, "Should have race choice plus wizard spell choices")

	// Verify race choice is preserved
	hasRaceChoice := false
	hasCantripChoice := false
	hasSpellChoice := false

	for _, choice := range savedDraft.Choices {
		switch {
		case choice.Source == shared.SourceRace && choice.Category == shared.ChoiceLanguages:
			hasRaceChoice = true
			s.Equal("human_languages", choice.ChoiceID)
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceCantrips:
			hasCantripChoice = true
			s.Equal("wizard_cantrips", choice.ChoiceID)
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceSpells:
			hasSpellChoice = true
			s.Equal("wizard_spells", choice.ChoiceID)
		}
	}

	s.True(hasRaceChoice, "Race choice should be preserved")
	s.True(hasCantripChoice, "Wizard should have cantrip choice")
	s.True(hasSpellChoice, "Wizard should have spell choice")
}

func (s *SpellSelectionOrchestratorTestSuite) TestUpdateClass_ChangingClassClearsOldClassChoices() {
	// Arrange
	draftID := "draft_456"
	existingDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_456",
		Name:     "Multiclass Test",
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassWizard,
		},
		Choices: []toolkitchar.ChoiceData{
			// Existing wizard choices
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
			// Race choice
			{
				Category: shared.ChoiceSkills,
				Source:   shared.SourceRace,
				ChoiceID: "elf_skills",
			},
		},
	}

	// Mock the Get call
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: existingDraft}, nil)

	// Mock the Update call and capture the draft
	var savedDraft *toolkitchar.DraftData
	s.mockDraftRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			savedDraft = input.Draft
			return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
		})

	// Act - Change to Fighter (no spell choices)
	input := &character.UpdateClassInput{
		DraftID: draftID,
		ClassID: constants.ClassFighter,
		Choices: nil,
	}
	output, err := s.orchestrator.UpdateClass(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Equal(constants.ClassFighter, output.Draft.ClassChoice.ClassID)

	// Verify old wizard choices were removed
	s.Require().NotNil(savedDraft)
	s.Require().Len(savedDraft.Choices, 1, "Should only have race choice left")

	// Only race choice should remain
	choice := savedDraft.Choices[0]
	s.Equal(shared.SourceRace, choice.Source)
	s.Equal(shared.ChoiceSkills, choice.Category)
	s.Equal("elf_skills", choice.ChoiceID)
}

func (s *SpellSelectionOrchestratorTestSuite) TestUpdateClass_ClericOnlyGetsCantrips() {
	// Arrange
	draftID := "draft_789"
	existingDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_789",
		Name:     "Cleric Test",
		Choices:  []toolkitchar.ChoiceData{},
	}

	// Mock the Get call
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: existingDraft}, nil)

	// Mock the Update call and capture the draft
	var savedDraft *toolkitchar.DraftData
	s.mockDraftRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			savedDraft = input.Draft
			return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
		})

	// Act
	input := &character.UpdateClassInput{
		DraftID: draftID,
		ClassID: constants.ClassCleric,
		Choices: nil,
	}
	output, err := s.orchestrator.UpdateClass(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Equal(constants.ClassCleric, output.Draft.ClassChoice.ClassID)

	// Verify only cantrip choice was added (clerics prepare spells)
	s.Require().NotNil(savedDraft)
	s.Require().Len(savedDraft.Choices, 1, "Cleric should only have cantrip choice")

	choice := savedDraft.Choices[0]
	s.Equal(shared.SourceClass, choice.Source)
	s.Equal(shared.ChoiceCantrips, choice.Category)
	s.Equal("cleric_cantrips", choice.ChoiceID)
}

// mockIDGenerator is a simple mock for testing
type mockIDGenerator struct{}

func (m *mockIDGenerator) Generate() string {
	return "mock_id_123"
}

func TestSpellSelectionOrchestratorTestSuite(t *testing.T) {
	suite.Run(t, new(SpellSelectionOrchestratorTestSuite))
}
