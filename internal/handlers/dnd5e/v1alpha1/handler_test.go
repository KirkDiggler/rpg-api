package v1alpha1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type HandlerTestSuite struct {
	suite.Suite
	ctrl             *gomock.Controller
	mockCharService  *charactermock.MockService
	handler          *v1alpha1.Handler
	ctx              context.Context
	
	// Test data
	testDraftData    *toolkitchar.DraftData
	testDraftID      string
	testPlayerID     string
}

func (s *HandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)
	s.ctx = context.Background()

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler
	
	// Initialize test data
	s.setupTestData()
}

func (s *HandlerTestSuite) SetupSubTest() {
	// Reset test data to clean state for each subtest
	s.setupTestData()
}

func (s *HandlerTestSuite) setupTestData() {
	s.testDraftID = "draft-123"
	s.testPlayerID = "player-456"
	s.testDraftData = &toolkitchar.DraftData{
		ID:       s.testDraftID,
		PlayerID: s.testPlayerID,
		Name:     "Gandalf",
	}
}

func (s *HandlerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerTestSuite) TestGetDraft_Success() {
	// Mock orchestrator response
	s.mockCharService.EXPECT().
		GetDraft(s.ctx, &character.GetDraftInput{
			DraftID: s.testDraftID,
		}).
		Return(&character.GetDraftOutput{
			Draft: s.testDraftData,
		}, nil)

	// Call handler
	resp, err := s.handler.GetDraft(s.ctx, &dnd5ev1alpha1.GetDraftRequest{
		DraftId: s.testDraftID,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal(s.testDraftID, resp.Draft.Id)
	s.Equal(s.testPlayerID, resp.Draft.PlayerId)
	s.Equal(s.testDraftData.Name, resp.Draft.Name)
}

func (s *HandlerTestSuite) TestGetDraft_EmptyDraftID() {
	// Call handler with empty draft ID
	resp, err := s.handler.GetDraft(s.ctx, &dnd5ev1alpha1.GetDraftRequest{
		DraftId: "",
	})

	// Assert error
	s.Error(err)
	s.Nil(resp)
	
	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "draft_id is required")
}

func (s *HandlerTestSuite) TestGetDraft_NotFound() {
	draftID := "draft-notfound"
	
	// Mock orchestrator response
	s.mockCharService.EXPECT().
		GetDraft(s.ctx, &character.GetDraftInput{
			DraftID: draftID,
		}).
		Return(nil, errors.NotFound("draft not found"))

	// Call handler
	resp, err := s.handler.GetDraft(s.ctx, &dnd5ev1alpha1.GetDraftRequest{
		DraftId: draftID,
	})

	// Assert error
	s.Error(err)
	s.Nil(resp)
	
	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.NotFound, st.Code())
	s.Contains(st.Message(), "draft not found")
}

func (s *HandlerTestSuite) TestGetDraft_MultipleScenarios() {
	testCases := []struct {
		name         string
		modifyData   func()
		setupMock    func()
		expectError  bool
		expectedCode codes.Code
	}{
		{
			name: "with full name",
			modifyData: func() {
				s.testDraftData.Name = "Gandalf the Grey"
			},
			setupMock: func() {
				s.mockCharService.EXPECT().
					GetDraft(s.ctx, &character.GetDraftInput{
						DraftID: s.testDraftID,
					}).
					Return(&character.GetDraftOutput{
						Draft: s.testDraftData,
					}, nil)
			},
			expectError: false,
		},
		{
			name: "with empty name",
			modifyData: func() {
				s.testDraftData.Name = ""
			},
			setupMock: func() {
				s.mockCharService.EXPECT().
					GetDraft(s.ctx, &character.GetDraftInput{
						DraftID: s.testDraftID,
					}).
					Return(&character.GetDraftOutput{
						Draft: s.testDraftData,
					}, nil)
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// SetupSubTest() is called automatically here, resetting test data
			
			// Modify test data for this specific scenario
			tc.modifyData()
			
			// Setup mock expectations
			tc.setupMock()
			
			// Call handler
			resp, err := s.handler.GetDraft(s.ctx, &dnd5ev1alpha1.GetDraftRequest{
				DraftId: s.testDraftID,
			})
			
			// Assert
			if tc.expectError {
				s.Error(err)
				st, ok := status.FromError(err)
				s.True(ok)
				s.Equal(tc.expectedCode, st.Code())
			} else {
				s.NoError(err)
				s.NotNil(resp)
				s.NotNil(resp.Draft)
				s.Equal(s.testDraftData.Name, resp.Draft.Name)
			}
		})
	}
}

func (s *HandlerTestSuite) TestCreateDraft_Success() {
	playerID := "player-123"
	draftID := "draft-456"
	
	// Mock orchestrator response
	s.mockCharService.EXPECT().
		CreateDraft(s.ctx, &character.CreateDraftInput{
			PlayerID: playerID,
		}).
		Return(&character.CreateDraftOutput{
			Draft: &toolkitchar.DraftData{
				ID:       draftID,
				PlayerID: playerID,
				Name:     "",
			},
		}, nil)

	// Call handler
	resp, err := s.handler.CreateDraft(s.ctx, &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId: playerID,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal(draftID, resp.Draft.Id)
	s.Equal(playerID, resp.Draft.PlayerId)
	s.Equal("", resp.Draft.Name)
}

func (s *HandlerTestSuite) TestCreateDraft_WithInitialData() {
	playerID := "player-789"
	draftID := "draft-012"
	draftName := "Gimli"
	
	// Mock orchestrator response with initial data
	s.mockCharService.EXPECT().
		CreateDraft(s.ctx, &character.CreateDraftInput{
			PlayerID: playerID,
			InitialData: &toolkitchar.DraftData{
				Name: draftName,
			},
		}).
		Return(&character.CreateDraftOutput{
			Draft: &toolkitchar.DraftData{
				ID:       draftID,
				PlayerID: playerID,
				Name:     draftName,
			},
		}, nil)

	// Call handler with initial data
	resp, err := s.handler.CreateDraft(s.ctx, &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId: playerID,
		InitialData: &dnd5ev1alpha1.CharacterDraftData{
			Name: draftName,
		},
	})

	// Assert response
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal(draftID, resp.Draft.Id)
	s.Equal(playerID, resp.Draft.PlayerId)
	s.Equal(draftName, resp.Draft.Name)
}

func (s *HandlerTestSuite) TestCreateDraft_WithSessionID() {
	playerID := "player-345"
	sessionID := "session-678"
	draftID := "draft-901"
	
	// Mock orchestrator response with session ID
	s.mockCharService.EXPECT().
		CreateDraft(s.ctx, &character.CreateDraftInput{
			PlayerID:  playerID,
			SessionID: sessionID,
		}).
		Return(&character.CreateDraftOutput{
			Draft: &toolkitchar.DraftData{
				ID:       draftID,
				PlayerID: playerID,
			},
		}, nil)

	// Call handler with session ID
	resp, err := s.handler.CreateDraft(s.ctx, &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId:  playerID,
		SessionId: sessionID,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal(draftID, resp.Draft.Id)
}

func (s *HandlerTestSuite) TestCreateDraft_EmptyPlayerID() {
	// Call handler with empty player ID
	resp, err := s.handler.CreateDraft(s.ctx, &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId: "",
	})

	// Assert error
	s.Error(err)
	s.Nil(resp)
	
	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
	s.Contains(st.Message(), "player_id is required")
}

func (s *HandlerTestSuite) TestCreateDraft_OrchestratorError() {
	playerID := "player-error"
	
	// Mock orchestrator error
	s.mockCharService.EXPECT().
		CreateDraft(s.ctx, &character.CreateDraftInput{
			PlayerID: playerID,
		}).
		Return(nil, errors.Internal("failed to create draft"))

	// Call handler
	resp, err := s.handler.CreateDraft(s.ctx, &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId: playerID,
	})

	// Assert error
	s.Error(err)
	s.Nil(resp)
	
	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.Internal, st.Code())
}

func (s *HandlerTestSuite) TestListDrafts_Success() {
	playerID := "player-123"
	draft1 := &toolkitchar.DraftData{
		ID:       "draft-1",
		PlayerID: playerID,
		Name:     "Gandalf",
	}

	// Mock orchestrator response
	s.mockCharService.EXPECT().
		ListDrafts(s.ctx, &character.ListDraftsInput{
			PlayerID: playerID,
		}).
		Return(&character.ListDraftsOutput{
			Drafts:        []*toolkitchar.DraftData{draft1},
			NextPageToken: "",
		}, nil)

	// Call handler
	resp, err := s.handler.ListDrafts(s.ctx, &dnd5ev1alpha1.ListDraftsRequest{
		PlayerId: playerID,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Drafts, 1)
	s.Equal("draft-1", resp.Drafts[0].Id)
	s.Equal("Gandalf", resp.Drafts[0].Name)
	s.Empty(resp.NextPageToken)
}

func (s *HandlerTestSuite) TestListDrafts_EmptyList() {
	playerID := "player-456"

	// Mock orchestrator response with empty list
	s.mockCharService.EXPECT().
		ListDrafts(s.ctx, &character.ListDraftsInput{
			PlayerID: playerID,
		}).
		Return(&character.ListDraftsOutput{
			Drafts:        []*toolkitchar.DraftData{},
			NextPageToken: "",
		}, nil)

	// Call handler
	resp, err := s.handler.ListDrafts(s.ctx, &dnd5ev1alpha1.ListDraftsRequest{
		PlayerId: playerID,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Drafts)
	s.Empty(resp.NextPageToken)
}

func (s *HandlerTestSuite) TestUpdateName_Success() {
	draftID := "draft-123"
	newName := "Gimli"
	updatedDraft := &toolkitchar.DraftData{
		ID:   draftID,
		Name: newName,
	}

	// Mock orchestrator response
	s.mockCharService.EXPECT().
		UpdateName(s.ctx, &character.UpdateNameInput{
			DraftID: draftID,
			Name:    newName,
		}).
		Return(&character.UpdateNameOutput{
			Draft:    updatedDraft,
			Warnings: []character.ValidationWarning{},
		}, nil)

	// Call handler
	resp, err := s.handler.UpdateName(s.ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    newName,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal(draftID, resp.Draft.Id)
	s.Equal(newName, resp.Draft.Name)
	s.Empty(resp.Warnings)
}

func (s *HandlerTestSuite) TestUpdateName_WithWarnings() {
	draftID := "draft-456"
	newName := "X"
	updatedDraft := &toolkitchar.DraftData{
		ID:   draftID,
		Name: newName,
	}

	// Mock orchestrator response with warnings
	s.mockCharService.EXPECT().
		UpdateName(s.ctx, &character.UpdateNameInput{
			DraftID: draftID,
			Name:    newName,
		}).
		Return(&character.UpdateNameOutput{
			Draft: updatedDraft,
			Warnings: []character.ValidationWarning{
				{
					Field:   "name",
					Message: "Name is very short",
					Type:    "warning",
				},
			},
		}, nil)

	// Call handler
	resp, err := s.handler.UpdateName(s.ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    newName,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal(newName, resp.Draft.Name)
	s.Len(resp.Warnings, 1)
	s.Equal("name", resp.Warnings[0].Field)
	s.Equal("Name is very short", resp.Warnings[0].Message)
}

func (s *HandlerTestSuite) TestUpdateRace_Success() {
	draftID := "draft-123"
	raceID := "RACE_DWARF"
	subraceID := "SUBRACE_MOUNTAIN_DWARF"
	updatedDraft := &toolkitchar.DraftData{
		ID:   draftID,
		Name: "Gimli",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID:    "RACE_DWARF",
			SubraceID: "SUBRACE_MOUNTAIN_DWARF",
		},
	}

	// Mock orchestrator response
	s.mockCharService.EXPECT().
		UpdateRace(s.ctx, &character.UpdateRaceInput{
			DraftID:   draftID,
			RaceID:    raceID,
			SubraceID: subraceID,
			Choices:   nil,
		}).
		Return(&character.UpdateRaceOutput{
			Draft:    updatedDraft,
			Warnings: []character.ValidationWarning{},
		}, nil)

	// Call handler
	resp, err := s.handler.UpdateRace(s.ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId:   draftID,
		Race:      dnd5ev1alpha1.Race_RACE_DWARF,
		Subrace:   dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF,
	})

	// Assert response
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal(draftID, resp.Draft.Id)
	s.Empty(resp.Warnings)
}

func (s *HandlerTestSuite) TestUpdateRace_WithChoices() {
	draftID := "draft-456"
	raceID := "RACE_HALF_ELF"
	updatedDraft := &toolkitchar.DraftData{
		ID:   draftID,
		Name: "Elrond",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: "RACE_HALF_ELF",
		},
		Choices: []toolkitchar.ChoiceData{
			{
				ChoiceID: "ability-increase-1",
				Category: shared.ChoiceAbilityScores,
				Source:   shared.SourceRace,
				AbilityScoreSelection: &shared.AbilityScores{
					"intelligence": 1,
				},
			},
		},
	}

	// Mock orchestrator response
	s.mockCharService.EXPECT().
		UpdateRace(s.ctx, &character.UpdateRaceInput{
			DraftID: draftID,
			RaceID:  raceID,
			Choices: []toolkitchar.ChoiceData{
				{
					ChoiceID: "ability-increase-1",
					Category: shared.ChoiceAbilityScores,
					Source:   shared.SourceRace,
					AbilityScoreSelection: &shared.AbilityScores{
						"intelligence": 1,
					},
				},
			},
		}).
		Return(&character.UpdateRaceOutput{
			Draft:    updatedDraft,
			Warnings: []character.ValidationWarning{},
		}, nil)

	// Call handler with choices
	resp, err := s.handler.UpdateRace(s.ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HALF_ELF,
		RaceChoices: []*dnd5ev1alpha1.ChoiceSelection{
			{
				ChoiceId:   "ability-increase-1",
				ChoiceType: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_ABILITY_SCORES,
				Source:     dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				AbilityScoreChoices: []*dnd5ev1alpha1.AbilityScoreChoice{
					{
						Ability: dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE,
						Bonus:   1,
					},
				},
			},
		},
	})

	// Assert response
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal(draftID, resp.Draft.Id)
}

func (s *HandlerTestSuite) TestUpdateRace_InvalidArgument() {
	// Mock orchestrator response
	s.mockCharService.EXPECT().
		UpdateRace(s.ctx, &character.UpdateRaceInput{
			DraftID: "",
			RaceID:  "RACE_HUMAN",
			Choices: nil,
		}).
		Return(nil, errors.InvalidArgument("draft ID is required"))

	// Call handler with empty draft ID
	resp, err := s.handler.UpdateRace(s.ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: "",
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
	})

	// Assert error
	s.Error(err)
	s.Nil(resp)
	
	st, ok := status.FromError(err)
	s.True(ok)
	s.Equal(codes.InvalidArgument, st.Code())
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}