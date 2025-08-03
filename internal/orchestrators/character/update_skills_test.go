package character_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	extmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	charmock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type UpdateSkillsOrchestratorTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	orchestrator    *character.Orchestrator
	mockCharRepo    *charmock.MockRepository
	mockDraftRepo   *draftmock.MockRepository
	mockExtClient   *extmock.MockClient
	mockDiceService *dicemock.MockService
	ctx             context.Context
}

func (s *UpdateSkillsOrchestratorTestSuite) SetupTest() {
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
	}
	orch, err := character.New(cfg)
	s.Require().NoError(err)
	s.orchestrator = orch
}

func (s *UpdateSkillsOrchestratorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *UpdateSkillsOrchestratorTestSuite) TestUpdateSkills_Success() {
	// Arrange
	draftID := "draft_123"
	existingDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Test Fighter",
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassFighter,
		},
		Choices: []toolkitchar.ChoiceData{
			// Existing language choice
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
	input := &character.UpdateSkillsInput{
		DraftID:  draftID,
		SkillIDs: []string{string(constants.SkillAthletics), string(constants.SkillIntimidation)},
	}
	output, err := s.orchestrator.UpdateSkills(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.Draft)

	// Verify the draft was saved with skill choices
	s.Require().NotNil(savedDraft)
	s.Require().Len(savedDraft.Choices, 2, "Should have language choice and skill choice")

	// Find the skill choice
	var skillChoice *toolkitchar.ChoiceData
	for _, choice := range savedDraft.Choices {
		if choice.Category == shared.ChoiceSkills {
			skillChoice = &choice
			break
		}
	}

	s.Require().NotNil(skillChoice, "Skill choice should be present")
	s.Equal(shared.SourceClass, skillChoice.Source)
	s.Equal("class_skills", skillChoice.ChoiceID)
	s.Require().Len(skillChoice.SkillSelection, 2)
	s.Contains(skillChoice.SkillSelection, constants.SkillAthletics)
	s.Contains(skillChoice.SkillSelection, constants.SkillIntimidation)
}

func (s *UpdateSkillsOrchestratorTestSuite) TestUpdateSkills_ReplacesExistingSkills() {
	// Arrange
	draftID := "draft_456"
	existingDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_456",
		Name:     "Test Fighter",
		Choices: []toolkitchar.ChoiceData{
			// Existing skill choice
			{
				Category:       shared.ChoiceSkills,
				Source:         shared.SourceClass,
				ChoiceID:       "class_skills",
				SkillSelection: []constants.Skill{constants.SkillPerception},
			},
			// Other choice
			{
				Category: shared.ChoiceLanguages,
				Source:   shared.SourceRace,
				ChoiceID: "race_languages",
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

	// Act - Replace with new skills
	input := &character.UpdateSkillsInput{
		DraftID:  draftID,
		SkillIDs: []string{string(constants.SkillAthletics), string(constants.SkillAcrobatics)},
	}
	output, err := s.orchestrator.UpdateSkills(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)

	// Verify old skills were replaced
	s.Require().NotNil(savedDraft)
	s.Require().Len(savedDraft.Choices, 2, "Should still have 2 choices")

	// Find the skill choice
	var skillChoice *toolkitchar.ChoiceData
	for _, choice := range savedDraft.Choices {
		if choice.Category == shared.ChoiceSkills {
			skillChoice = &choice
			break
		}
	}

	s.Require().NotNil(skillChoice)
	s.Require().Len(skillChoice.SkillSelection, 2)
	s.Contains(skillChoice.SkillSelection, constants.SkillAthletics)
	s.Contains(skillChoice.SkillSelection, constants.SkillAcrobatics)
	s.NotContains(skillChoice.SkillSelection, constants.SkillPerception, "Old skill should be replaced")
}

func (s *UpdateSkillsOrchestratorTestSuite) TestUpdateSkills_MissingDraftID() {
	// Act
	input := &character.UpdateSkillsInput{
		DraftID:  "",
		SkillIDs: []string{string(constants.SkillAthletics)},
	}
	output, err := s.orchestrator.UpdateSkills(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Nil(output)
	s.True(errors.IsInvalidArgument(err))
	s.Contains(err.Error(), "draft ID is required")
}

func (s *UpdateSkillsOrchestratorTestSuite) TestUpdateSkills_NoSkills() {
	// Act
	input := &character.UpdateSkillsInput{
		DraftID:  "draft_123",
		SkillIDs: []string{},
	}
	output, err := s.orchestrator.UpdateSkills(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Nil(output)
	s.True(errors.IsInvalidArgument(err))
	s.Contains(err.Error(), "at least one skill must be selected")
}

func (s *UpdateSkillsOrchestratorTestSuite) TestUpdateSkills_DraftNotFound() {
	// Arrange
	draftID := "non_existent"

	// Mock the Get call to return not found
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(nil, errors.NotFound("draft not found"))

	// Act
	input := &character.UpdateSkillsInput{
		DraftID:  draftID,
		SkillIDs: []string{string(constants.SkillAthletics)},
	}
	output, err := s.orchestrator.UpdateSkills(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "failed to get draft")
}

func TestUpdateSkillsOrchestratorTestSuite(t *testing.T) {
	suite.Run(t, new(UpdateSkillsOrchestratorTestSuite))
}