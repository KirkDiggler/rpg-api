package character_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"errors"

	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type FinalizeDraftTestSuite struct {
	suite.Suite
	ctrl          *gomock.Controller
	mockCharRepo  *characterrepomock.MockRepository
	mockDraftRepo *draftrepomock.MockRepository
	orchestrator  character.Service
	ctx           context.Context
}

func (s *FinalizeDraftTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = characterrepomock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftrepomock.NewMockRepository(s.ctrl)
	s.ctx = context.Background()

	// For this test, we'll use a simplified setup
	// In a real test, you'd properly initialize the orchestrator with all dependencies
	// For now, we'll skip the actual orchestrator initialization since it requires many deps
}

func (s *FinalizeDraftTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TODO(#129): Enable this test after implementing full orchestrator constructor
// This test requires all orchestrator dependencies to be properly mocked
func (s *FinalizeDraftTestSuite) TestFinalizeDraft_Disabled() {
	testCases := []struct {
		name      string
		input     *character.FinalizeDraftInput
		setupMock func()
		validate  func(*character.FinalizeDraftOutput, error)
	}{
		{
			name: "success with complete draft",
			input: &character.FinalizeDraftInput{
				DraftID: "draft_123",
			},
			setupMock: func() {
				// Get draft from repository
				draftData := s.createTestDraftData()
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, gomock.Any()).
					Return(&draftrepo.GetOutput{
						Draft: draftData,
					}, nil)

				// Create character in repository
				s.mockCharRepo.EXPECT().
					Create(s.ctx, gomock.Any()).
					DoAndReturn(func(ctx context.Context, input characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
						// Validate the character data has expected fields
						s.NotNil(input.CharacterData)
						s.NotEmpty(input.CharacterData.ID)
						s.Equal("Test Hero", input.CharacterData.Name)
						s.Equal(1, input.CharacterData.Level)
						s.Equal("human", input.CharacterData.RaceID)
						s.Equal("fighter", input.CharacterData.ClassID)
						s.Equal("soldier", input.CharacterData.BackgroundID)

						return &characterrepo.CreateOutput{
							CharacterData: input.CharacterData,
						}, nil
					})

				// Delete draft after successful creation
				s.mockDraftRepo.EXPECT().
					Delete(s.ctx, gomock.Any()).
					Return(&draftrepo.DeleteOutput{}, nil)
			},
			validate: func(output *character.FinalizeDraftOutput, err error) {
				s.NoError(err)
				s.NotNil(output)
				s.NotNil(output.Character)
				s.True(output.DraftDeleted)

				// Validate converted character
				s.Equal("Test Hero", output.Character.Name)
				s.Equal(int32(1), output.Character.Level)
				s.Equal("human", output.Character.RaceID)
				s.Equal("fighter", output.Character.ClassID)
				s.Equal("soldier", output.Character.BackgroundID)
				s.Equal(int32(16), output.Character.AbilityScores.Strength)
			},
		},
		{
			name: "error when draft not found",
			input: &character.FinalizeDraftInput{
				DraftID: "draft_notfound",
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, gomock.Any()).
					Return(nil, errors.New("draft not found"))
			},
			validate: func(output *character.FinalizeDraftOutput, err error) {
				s.Error(err)
				s.Contains(err.Error(), "draft not found")
				s.Nil(output)
			},
		},
		{
			name: "continues when draft deletion fails",
			input: &character.FinalizeDraftInput{
				DraftID: "draft_123",
			},
			setupMock: func() {
				// Get draft
				draftData := s.createTestDraftData()
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, gomock.Any()).
					Return(&draftrepo.GetOutput{
						Draft: draftData,
					}, nil)

				// Create character successfully
				s.mockCharRepo.EXPECT().
					Create(s.ctx, gomock.Any()).
					Return(&characterrepo.CreateOutput{
						CharacterData: s.createTestCharacterData(),
					}, nil)

				// Delete draft fails but should not fail the operation
				s.mockDraftRepo.EXPECT().
					Delete(s.ctx, gomock.Any()).
					Return(nil, errors.New("delete failed"))
			},
			validate: func(output *character.FinalizeDraftOutput, err error) {
				// Should succeed despite draft deletion failure
				s.NoError(err)
				s.NotNil(output)
				s.NotNil(output.Character)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.FinalizeDraft(s.ctx, tc.input)
			tc.validate(output, err)
		})
	}
}

// Helper to create test draft data
func (s *FinalizeDraftTestSuite) createTestDraftData() *toolkitchar.DraftData {
	return &toolkitchar.DraftData{
		ID:       "draft_123",
		PlayerID: "player_123",
		Name:     "Test Hero",
		Choices: map[shared.ChoiceCategory]any{
			shared.ChoiceName: "Test Hero",
			shared.ChoiceRace: toolkitchar.RaceChoice{
				RaceID: "human",
			},
			shared.ChoiceClass:      "fighter",
			shared.ChoiceBackground: "soldier",
			shared.ChoiceAbilityScores: shared.AbilityScores{
				Strength:     16,
				Dexterity:    14,
				Constitution: 15,
				Intelligence: 10,
				Wisdom:       12,
				Charisma:     8,
			},
			shared.ChoiceSkills: []string{"athletics", "intimidation"},
		},
		ProgressFlags: toolkitchar.ProgressName |
			toolkitchar.ProgressRace |
			toolkitchar.ProgressClass |
			toolkitchar.ProgressBackground |
			toolkitchar.ProgressAbilityScores |
			toolkitchar.ProgressSkills,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// Helper to create test character data
func (s *FinalizeDraftTestSuite) createTestCharacterData() *toolkitchar.Data {
	now := time.Now()
	return &toolkitchar.Data{
		ID:           "char_generated",
		PlayerID:     "player_123",
		Name:         "Test Hero",
		Level:        1,
		Experience:   0,
		RaceID:       "human",
		ClassID:      "fighter",
		BackgroundID: "soldier",
		AbilityScores: shared.AbilityScores{
			Strength:     16,
			Dexterity:    14,
			Constitution: 15,
			Intelligence: 10,
			Wisdom:       12,
			Charisma:     8,
		},
		HitPoints:    12,
		MaxHitPoints: 12,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// TODO(#129): Enable this test suite after implementing full orchestrator constructor
func TestFinalizeDraftTestSuite_Disabled(t *testing.T) {
	// suite.Run(t, new(FinalizeDraftTestSuite))
	t.Skip("Skipping until orchestrator constructor is fully implemented")
}
