package character_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	external "github.com/KirkDiggler/rpg-api/internal/clients/external"
	externalmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/engine"
	enginemock "github.com/KirkDiggler/rpg-api/internal/engine/mock"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	characterorchestrator "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	characterrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	"github.com/KirkDiggler/rpg-api/internal/testutils/builders"
	"github.com/KirkDiggler/rpg-api/internal/testutils/mocks"
)

type OrchestratorTestSuite struct {
	suite.Suite
	ctrl               *gomock.Controller
	mockCharRepo       *characterrepomock.MockRepository
	mockDraftRepo      *draftrepomock.MockRepository
	mockEngine         *enginemock.MockEngine
	mockExternalClient *externalmock.MockClient
	mockDiceService    *dicemock.MockService
	orchestrator       *characterorchestrator.Orchestrator
	ctx                context.Context

	// Test data
	testDraftID     string
	testCharacterID string
	testPlayerID    string
	testSessionID   string
	testDraft       *dnd5e.CharacterDraft
	testCharacter   *dnd5e.Character
}

func (s *OrchestratorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = characterrepomock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftrepomock.NewMockRepository(s.ctrl)
	s.mockEngine = enginemock.NewMockEngine(s.ctrl)
	s.mockExternalClient = externalmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)

	cfg := &characterorchestrator.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		Engine:             s.mockEngine,
		ExternalClient:     s.mockExternalClient,
		DiceService:        s.mockDiceService,
	}

	orchestrator, err := characterorchestrator.New(cfg)
	s.Require().NoError(err)
	s.orchestrator = orchestrator

	s.ctx = context.Background()

	// Initialize base test IDs that won't change
	s.testDraftID = "draft-123"
	s.testCharacterID = "char-456"
	s.testPlayerID = "player-789"
	s.testSessionID = "session-012"
}

func (s *OrchestratorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// SetupSubTest runs before each s.Run()
func (s *OrchestratorTestSuite) SetupSubTest() {
	// Reset test data to clean state for each subtest
	s.testDraft = builders.NewCharacterDraftBuilder().
		WithID(s.testDraftID).
		WithPlayerID(s.testPlayerID).
		WithSessionID(s.testSessionID).
		WithName("Gandalf").
		WithRace(dnd5e.RaceHuman).
		WithClass(dnd5e.ClassWizard).
		Build()

	s.testCharacter = &dnd5e.Character{
		ID:        s.testCharacterID,
		PlayerID:  s.testPlayerID,
		SessionID: s.testSessionID,
		Name:      "Gandalf",
		Level:     1,
		RaceID:    dnd5e.RaceHuman,
		ClassID:   dnd5e.ClassWizard,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
}

// Draft lifecycle tests

func (s *OrchestratorTestSuite) TestCreateDraft() {
	testCases := []struct {
		name      string
		input     *characterorchestrator.CreateDraftInput
		setupMock func()
		wantErr   bool
		errMsg    string
		validate  func(*characterorchestrator.CreateDraftOutput)
	}{
		{
			name: "successful creation with minimal data",
			input: &characterorchestrator.CreateDraftInput{
				PlayerID: s.testPlayerID,
			},
			setupMock: func() {
				// TODO(#30): Inject clock and ID generator into orchestrator to make tests deterministic
				// Currently using gomock.Any() because CreateDraft generates:
				// - ID using time.Now().UnixNano()
				// - Timestamps using time.Now().Unix()
				// - ExpiresAt using time.Now().Add(24 * time.Hour).Unix()
				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterDraftOutput{
						IsValid: true,
					}, nil)

				s.mockDraftRepo.EXPECT().
					Create(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.CreateInput) (*draftrepo.CreateOutput, error) {
						// Repository sets ID and timestamps
						draft := *input.Draft
						draft.ID = "generated-id"
						draft.CreatedAt = time.Now().Unix()
						draft.UpdatedAt = time.Now().Unix()
						return &draftrepo.CreateOutput{Draft: &draft}, nil
					})
			},
			wantErr: false,
			validate: func(output *characterorchestrator.CreateDraftOutput) {
				s.NotNil(output.Draft)
				s.Equal(s.testPlayerID, output.Draft.PlayerID)
				s.Equal("", output.Draft.SessionID)
				s.False(output.Draft.Progress.HasName())
				s.Equal(dnd5e.CreationStepName, output.Draft.Progress.CurrentStep)
				s.Equal(int32(0), output.Draft.Progress.CompletionPercentage)
			},
		},
		{
			name: "successful creation with initial data",
			input: &characterorchestrator.CreateDraftInput{
				PlayerID:  s.testPlayerID,
				SessionID: s.testSessionID,
				InitialData: &dnd5e.CharacterDraft{
					Name:    "Frodo",
					RaceID:  dnd5e.RaceHalfling,
					ClassID: dnd5e.ClassRogue,
				},
			},
			setupMock: func() {
				// TODO(#30): Need clock and ID generator injection for deterministic tests
				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterDraftOutput{
						IsValid: true,
					}, nil)

				s.mockDraftRepo.EXPECT().
					Create(s.ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, input draftrepo.CreateInput) (*draftrepo.CreateOutput, error) {
						// Repository sets ID and timestamps
						draft := *input.Draft
						draft.ID = "generated-id"
						draft.CreatedAt = time.Now().Unix()
						draft.UpdatedAt = time.Now().Unix()
						return &draftrepo.CreateOutput{Draft: &draft}, nil
					})
			},
			wantErr: false,
			validate: func(output *characterorchestrator.CreateDraftOutput) {
				s.NotNil(output.Draft)
				s.Equal("Frodo", output.Draft.Name)
				s.Equal(dnd5e.RaceHalfling, output.Draft.RaceID)
				s.Equal(dnd5e.ClassRogue, output.Draft.ClassID)
				s.True(output.Draft.Progress.HasName())
				s.True(output.Draft.Progress.HasRace())
				s.True(output.Draft.Progress.HasClass())
				s.Greater(output.Draft.Progress.CompletionPercentage, int32(0))
			},
		},
		{
			name:      "nil input",
			input:     nil,
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "input is required",
		},
		{
			name: "missing player ID",
			input: &characterorchestrator.CreateDraftInput{
				SessionID: s.testSessionID,
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "validation failed: playerID: is required",
		},
		{
			name: "repository error",
			input: &characterorchestrator.CreateDraftInput{
				PlayerID: s.testPlayerID,
			},
			setupMock: func() {
				// Expect engine validation
				s.mockEngine.EXPECT().
					ValidateCharacterDraft(s.ctx, gomock.Any()).
					Return(&engine.ValidateCharacterDraftOutput{
						IsValid: true,
					}, nil)

				s.mockDraftRepo.EXPECT().
					Create(s.ctx, gomock.Any()).
					Return(nil, errors.Internal("database error"))
			},
			wantErr: true,
			errMsg:  "failed to create draft",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.CreateDraft(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
				s.Nil(output)
			} else {
				s.NoError(err)
				s.NotNil(output)
				if tc.validate != nil {
					tc.validate(output)
				}
			}
		})
	}
}

func (s *OrchestratorTestSuite) TestGetDraft() {
	testCases := []struct {
		name      string
		input     *characterorchestrator.GetDraftInput
		setupMock func()
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful retrieval",
			input: &characterorchestrator.GetDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(s.testDraft)}, nil)

				// Expect hydration calls for race and class (since testDraft has these set)
				s.mockExternalClient.EXPECT().
					GetRaceData(s.ctx, dnd5e.RaceHuman).
					Return(&external.RaceData{
						ID:   dnd5e.RaceHuman,
						Name: "Human",
					}, nil)

				s.mockExternalClient.EXPECT().
					GetClassData(s.ctx, dnd5e.ClassWizard).
					Return(&external.ClassData{
						ID:   dnd5e.ClassWizard,
						Name: "Wizard",
					}, nil)
			},
			wantErr: false,
		},
		{
			name:      "nil input",
			input:     nil,
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "input is required",
		},
		{
			name: "missing draft ID",
			input: &characterorchestrator.GetDraftInput{
				DraftID: "",
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "validation failed: draftID: is required",
		},
		{
			name: "draft not found",
			input: &characterorchestrator.GetDraftInput{
				DraftID: "nonexistent",
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: "nonexistent"}).
					Return(nil, errors.NotFoundf("draft not found"))
			},
			wantErr: true,
			errMsg:  "failed to get draft",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.GetDraft(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
				s.Nil(output)
			} else {
				s.NoError(err)
				s.NotNil(output)
				s.NotNil(output.Draft)
				// Check that the draft was hydrated
				if output.Draft.RaceID != "" {
					s.NotNil(output.Draft.Race, "Race should be hydrated")
				}
				if output.Draft.ClassID != "" {
					s.NotNil(output.Draft.Class, "Class should be hydrated")
				}
			}
		})
	}
}

func (s *OrchestratorTestSuite) TestListDrafts() {
	testCases := []struct {
		name      string
		input     *characterorchestrator.ListDraftsInput
		setupMock func()
		wantErr   bool
		validate  func(*characterorchestrator.ListDraftsOutput)
	}{
		{
			name: "successful list - player has draft",
			input: &characterorchestrator.ListDraftsInput{
				PlayerID: s.testPlayerID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					GetByPlayerID(s.ctx, draftrepo.GetByPlayerIDInput{
						PlayerID: s.testPlayerID,
					}).
					Return(&draftrepo.GetByPlayerIDOutput{
						Draft: dnd5e.FromCharacterDraft(s.testDraft),
					}, nil)

				// Expect hydration calls
				s.mockExternalClient.EXPECT().
					GetRaceData(s.ctx, dnd5e.RaceHuman).
					Return(&external.RaceData{
						ID:   dnd5e.RaceHuman,
						Name: "Human",
					}, nil)

				s.mockExternalClient.EXPECT().
					GetClassData(s.ctx, dnd5e.ClassWizard).
					Return(&external.ClassData{
						ID:   dnd5e.ClassWizard,
						Name: "Wizard",
					}, nil)
			},
			wantErr: false,
			validate: func(output *characterorchestrator.ListDraftsOutput) {
				s.Len(output.Drafts, 1)
				s.Equal(s.testDraft.ID, output.Drafts[0].ID)
				s.Equal("", output.NextPageToken) // No pagination for single draft
				// Check hydration
				s.NotNil(output.Drafts[0].Race, "Race should be hydrated")
				s.NotNil(output.Drafts[0].Class, "Class should be hydrated")
			},
		},
		{
			name: "successful list - player has no draft",
			input: &characterorchestrator.ListDraftsInput{
				PlayerID: s.testPlayerID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					GetByPlayerID(s.ctx, draftrepo.GetByPlayerIDInput{
						PlayerID: s.testPlayerID,
					}).
					Return(nil, errors.NotFoundf("no draft found"))
			},
			wantErr: false,
			validate: func(output *characterorchestrator.ListDraftsOutput) {
				s.Empty(output.Drafts)
				s.Equal("", output.NextPageToken)
			},
		},
		{
			name: "error - no player ID provided",
			input: &characterorchestrator.ListDraftsInput{
				SessionID: s.testSessionID, // Only session ID
			},
			setupMock: func() {},
			wantErr:   true,
		},
		{
			name:      "nil input",
			input:     nil,
			setupMock: func() {},
			wantErr:   true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.ListDrafts(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
				s.NotNil(output)
				if tc.validate != nil {
					tc.validate(output)
				}
			}
		})
	}
}

func (s *OrchestratorTestSuite) TestDeleteDraft() {
	testCases := []struct {
		name      string
		input     *characterorchestrator.DeleteDraftInput
		setupMock func()
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful deletion",
			input: &characterorchestrator.DeleteDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Delete(s.ctx, draftrepo.DeleteInput{ID: s.testDraftID}).
					Return(&draftrepo.DeleteOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name:      "nil input",
			input:     nil,
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "input is required",
		},
		{
			name: "missing draft ID",
			input: &characterorchestrator.DeleteDraftInput{
				DraftID: "",
			},
			setupMock: func() {},
			wantErr:   true,
			errMsg:    "validation failed: draftID: is required",
		},
		{
			name: "repository error",
			input: &characterorchestrator.DeleteDraftInput{
				DraftID: s.testDraftID,
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Delete(s.ctx, draftrepo.DeleteInput{ID: s.testDraftID}).
					Return(nil, errors.Internal("database error"))
			},
			wantErr: true,
			errMsg:  "failed to delete draft",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.DeleteDraft(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
			} else {
				s.NoError(err)
				s.NotNil(output)
				s.Contains(output.Message, "deleted successfully")
			}
		})
	}
}

func (s *OrchestratorTestSuite) TestUpdateBackgroundWithChoices() {
	testCases := []struct {
		name      string
		input     *characterorchestrator.UpdateBackgroundInput
		setupMock func()
		wantErr   bool
		errMsg    string
	}{
		{
			name: "success - with choices",
			input: &characterorchestrator.UpdateBackgroundInput{
				DraftID:      s.testDraftID,
				BackgroundID: "soldier",
				Choices: []dnd5e.ChoiceSelection{
					{
						ChoiceID:     "language-choice",
						ChoiceType:   dnd5e.ChoiceTypeLanguage,
						Source:       dnd5e.ChoiceSourceBackground,
						SelectedKeys: []string{"orcish"},
					},
				},
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, draftrepo.GetInput{ID: s.testDraftID}).
					Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(s.testDraft)}, nil)

				s.mockEngine.EXPECT().
					ValidateBackgroundChoice(s.ctx, &engine.ValidateBackgroundChoiceInput{
						BackgroundID: "soldier",
					}).
					Return(&engine.ValidateBackgroundChoiceOutput{
						IsValid: true,
					}, nil)

				s.mockDraftRepo.EXPECT().
					Update(s.ctx, gomock.Any()).
					DoAndReturn(func(ctx context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
						// Verify choices were added
						s.Equal("soldier", input.Draft.BackgroundID)
						foundBackgroundChoice := false
						for _, choice := range input.Draft.ChoiceSelections {
							if choice.Source == dnd5e.ChoiceSourceBackground {
								foundBackgroundChoice = true
								s.Equal("language-choice", choice.ChoiceID)
							}
						}
						s.True(foundBackgroundChoice, "Background choice should be added")

						return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
					})

				// Expect hydration of existing race and class too
				mocks.ExpectDraftHydration(s.ctx, s.mockExternalClient, s.testDraft)

				s.mockExternalClient.EXPECT().
					GetBackgroundData(s.ctx, "soldier").
					Return(&external.BackgroundData{
						ID:   "soldier",
						Name: "Soldier",
					}, nil)
			},
			wantErr: false,
		},
		{
			name: "error - invalid background",
			input: &characterorchestrator.UpdateBackgroundInput{
				DraftID:      s.testDraftID,
				BackgroundID: "invalid",
			},
			setupMock: func() {
				s.mockDraftRepo.EXPECT().
					Get(s.ctx, gomock.Any()).
					Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(s.testDraft)}, nil)

				s.mockEngine.EXPECT().
					ValidateBackgroundChoice(s.ctx, gomock.Any()).
					Return(&engine.ValidateBackgroundChoiceOutput{
						IsValid: false,
					}, nil)
			},
			wantErr: true,
			errMsg:  "invalid background choice",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			tc.setupMock()

			output, err := s.orchestrator.UpdateBackground(s.ctx, tc.input)

			if tc.wantErr {
				s.Error(err)
				s.Contains(err.Error(), tc.errMsg)
			} else {
				s.NoError(err)
				s.NotNil(output)
			}
		})
	}
}

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}
