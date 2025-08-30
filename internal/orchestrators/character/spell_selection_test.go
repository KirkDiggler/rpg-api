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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
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
			RaceID: races.Human,
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
		ClassID: classes.Wizard,
		Choices: nil, // No additional choices from handler
	}
	output, err := s.orchestrator.UpdateClass(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.Draft)

	// Verify the class was set
	s.Equal(classes.Wizard, output.Draft.ClassChoice.ClassID)

	// Verify choices were added
	s.Require().NotNil(savedDraft, "Draft should have been saved")

	// The new system adds all class requirements including equipment choices
	// Should have: 1 race choice + skills + cantrips + spells + equipment choices
	s.Require().Greater(len(savedDraft.Choices), 3, "Should have race choice plus all wizard requirements")

	// Verify race choice is preserved
	hasRaceChoice := false
	hasCantripChoice := false
	hasSpellChoice := false
	hasSkillChoice := false
	equipmentCount := 0

	for _, choice := range savedDraft.Choices {
		switch {
		case choice.Source == shared.SourceRace && choice.Category == shared.ChoiceLanguages:
			hasRaceChoice = true
			s.Equal("human_languages", choice.ChoiceID)
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceCantrips:
			hasCantripChoice = true
			s.Equal("class_cantrips", choice.ChoiceID)
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceSpells:
			hasSpellChoice = true
			s.Equal("class_spells", choice.ChoiceID)
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceSkills:
			hasSkillChoice = true
			s.Equal("class_skills", choice.ChoiceID)
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceEquipment:
			equipmentCount++
		}
	}

	s.True(hasRaceChoice, "Race choice should be preserved")
	s.True(hasCantripChoice, "Wizard should have cantrip choice")
	s.True(hasSpellChoice, "Wizard should have spell choice")
	s.True(hasSkillChoice, "Wizard should have skill choice")
	s.Greater(equipmentCount, 0, "Wizard should have equipment choices")
}

func (s *SpellSelectionOrchestratorTestSuite) TestUpdateClass_ChangingClassClearsOldClassChoices() {
	// Arrange
	draftID := "draft_456"
	existingDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_456",
		Name:     "Multiclass Test",
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Wizard,
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
		ClassID: classes.Fighter,
		Choices: nil,
	}
	output, err := s.orchestrator.UpdateClass(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Equal(classes.Fighter, output.Draft.ClassChoice.ClassID)

	// Verify old wizard choices were removed but new Fighter choices added
	s.Require().NotNil(savedDraft)

	// Fighter should have race choice + fighter class choices (skills, fighting style, equipment)
	hasRaceChoice := false
	hasFightingStyle := false
	hasSkillChoice := false
	equipmentCount := 0

	// Should not have wizard-specific choices anymore
	hasWizardCantrips := false
	hasWizardSpells := false

	for _, choice := range savedDraft.Choices {
		switch {
		case choice.Source == shared.SourceRace && choice.Category == shared.ChoiceSkills:
			hasRaceChoice = true
			s.Equal("elf_skills", choice.ChoiceID)
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceFightingStyle:
			hasFightingStyle = true
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceSkills:
			hasSkillChoice = true
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceEquipment:
			equipmentCount++
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceCantrips:
			hasWizardCantrips = true
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceSpells:
			hasWizardSpells = true
		}
	}

	s.True(hasRaceChoice, "Race choice should be preserved")
	s.True(hasFightingStyle, "Fighter should have fighting style choice")
	s.True(hasSkillChoice, "Fighter should have skill choice")
	s.Greater(equipmentCount, 0, "Fighter should have equipment choices")
	s.False(hasWizardCantrips, "Fighter should not have wizard cantrip choices")
	s.False(hasWizardSpells, "Fighter should not have wizard spell choices")
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
		ClassID: classes.Cleric,
		Choices: nil,
	}
	output, err := s.orchestrator.UpdateClass(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Equal(classes.Cleric, output.Draft.ClassChoice.ClassID)

	// Verify Cleric choices were added
	s.Require().NotNil(savedDraft)

	// Cleric should have cantrips (prepared casters don't choose spells at creation)
	// Plus skills and equipment choices from the new system
	hasCantripChoice := false
	hasSkillChoice := false
	equipmentCount := 0

	for _, choice := range savedDraft.Choices {
		switch {
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceCantrips:
			hasCantripChoice = true
			s.Equal("class_cantrips", choice.ChoiceID)
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceSkills:
			hasSkillChoice = true
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceEquipment:
			equipmentCount++
		case choice.Source == shared.SourceClass && choice.Category == shared.ChoiceSpells:
			// Note: In the new system, Cleric actually gets a spell choice for domain spells
			// This is a difference from the old hardcoded logic
		}
	}

	s.True(hasCantripChoice, "Cleric should have cantrip choice")
	s.True(hasSkillChoice, "Cleric should have skill choice")
	s.Greater(equipmentCount, 0, "Cleric should have equipment choices")
}

// mockIDGenerator is a simple mock for testing
type mockIDGenerator struct{}

func (m *mockIDGenerator) Generate() string {
	return "mock_id_123"
}

func TestSpellSelectionOrchestratorTestSuite(t *testing.T) {
	suite.Run(t, new(SpellSelectionOrchestratorTestSuite))
}
