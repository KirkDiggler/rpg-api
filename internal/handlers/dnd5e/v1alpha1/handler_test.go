package v1alpha1_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api/gen/go/github.com/KirkDiggler/rpg-api/api/proto/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/services/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/services/character/mock"
)

type HandlerTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharService *charactermock.MockService
	handler         *v1alpha1.Handler
	ctx             context.Context

	// Test data - valid requests we can use across tests
	validCreateDraftReq     *dnd5ev1alpha1.CreateDraftRequest
	validGetDraftReq        *dnd5ev1alpha1.GetDraftRequest
	validListDraftsReq      *dnd5ev1alpha1.ListDraftsRequest
	validUpdateNameReq      *dnd5ev1alpha1.UpdateNameRequest
	validUpdateRaceReq      *dnd5ev1alpha1.UpdateRaceRequest
	validUpdateClassReq     *dnd5ev1alpha1.UpdateClassRequest
	validFinalizeDraftReq   *dnd5ev1alpha1.FinalizeDraftRequest
	validGetCharacterReq    *dnd5ev1alpha1.GetCharacterRequest
	validListCharactersReq  *dnd5ev1alpha1.ListCharactersRequest
	validDeleteCharacterReq *dnd5ev1alpha1.DeleteCharacterRequest

	// Common test IDs
	testPlayerID    string
	testSessionID   string
	testDraftID     string
	testCharacterID string

	// Expected entities for reuse
	expectedDraft            *dnd5e.CharacterDraft
	expectedCharacter        *dnd5e.Character
	expectedAbilityScores    *dnd5e.AbilityScores
	expectedCreationProgress *dnd5e.CreationProgress
}

func (s *HandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharService = charactermock.NewMockService(s.ctrl)

	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.mockCharService,
	})
	s.Require().NoError(err)
	s.handler = handler

	s.ctx = context.Background()

	// Initialize test IDs
	s.testPlayerID = "player-123"
	s.testSessionID = "session-456"
	s.testDraftID = "draft-789"
	s.testCharacterID = "char-101"

	// Initialize valid requests - these can be modified in specific tests
	s.validCreateDraftReq = &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId:  s.testPlayerID,
		SessionId: s.testSessionID,
		InitialData: &dnd5ev1alpha1.CharacterDraft{
			Name: "Gandalf the Grey",
		},
	}

	s.validGetDraftReq = &dnd5ev1alpha1.GetDraftRequest{
		DraftId: s.testDraftID,
	}

	s.validListDraftsReq = &dnd5ev1alpha1.ListDraftsRequest{
		PlayerId:  s.testPlayerID,
		SessionId: s.testSessionID,
		PageSize:  20,
		PageToken: "",
	}

	s.validUpdateNameReq = &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: s.testDraftID,
		Name:    "Gandalf the White",
	}

	s.validUpdateRaceReq = &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: s.testDraftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
	}

	s.validUpdateClassReq = &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: s.testDraftID,
		Class:   dnd5ev1alpha1.Class_CLASS_WIZARD,
	}

	s.validFinalizeDraftReq = &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: s.testDraftID,
	}

	s.validGetCharacterReq = &dnd5ev1alpha1.GetCharacterRequest{
		CharacterId: s.testCharacterID,
	}

	s.validListCharactersReq = &dnd5ev1alpha1.ListCharactersRequest{
		PlayerId:  s.testPlayerID,
		SessionId: s.testSessionID,
		PageSize:  20,
		PageToken: "",
	}

	s.validDeleteCharacterReq = &dnd5ev1alpha1.DeleteCharacterRequest{
		CharacterId: s.testCharacterID,
	}

	// Initialize expected entities for reuse
	s.expectedAbilityScores = &dnd5e.AbilityScores{
		Strength:     10,
		Dexterity:    14,
		Constitution: 12,
		Intelligence: 18,
		Wisdom:       15,
		Charisma:     13,
	}

	s.expectedCreationProgress = &dnd5e.CreationProgress{
		StepsCompleted:       dnd5e.ProgressStepName | dnd5e.ProgressStepRace | dnd5e.ProgressStepClass | dnd5e.ProgressStepBackground | dnd5e.ProgressStepAbilityScores,
		CompletionPercentage: 71,
		CurrentStep:          dnd5e.CreationStepAbilityScores,
	}

	s.expectedDraft = &dnd5e.CharacterDraft{
		ID:                  s.testDraftID,
		PlayerID:            s.testPlayerID,
		SessionID:           s.testSessionID,
		Name:                "Gandalf the Grey",
		RaceID:              dnd5e.RaceHuman,
		SubraceID:           "",
		ClassID:             dnd5e.ClassWizard,
		BackgroundID:        dnd5e.BackgroundSage,
		Alignment:           dnd5e.AlignmentLawfulGood,
		AbilityScores:       s.expectedAbilityScores,
		StartingSkillIDs:    []string{dnd5e.SkillArcana, dnd5e.SkillHistory},
		AdditionalLanguages: []string{dnd5e.LanguageElvish}, // Common is assumed
		Progress:            *s.expectedCreationProgress,
		CreatedAt:           1234567890,
		UpdatedAt:           1234567890,
	}

	s.expectedCharacter = &dnd5e.Character{
		ID:               s.testCharacterID,
		PlayerID:         s.testPlayerID,
		SessionID:        s.testSessionID,
		Name:             "Gandalf the White",
		Level:            20,
		RaceID:           dnd5e.RaceHuman,
		SubraceID:        "",
		ClassID:          dnd5e.ClassWizard,
		BackgroundID:     dnd5e.BackgroundSage,
		Alignment:        dnd5e.AlignmentLawfulGood,
		AbilityScores:    *s.expectedAbilityScores,
		CurrentHP:        100,
		TempHP:           0,
		ExperiencePoints: 355000,
		CreatedAt:        1234567890,
		UpdatedAt:        1234567890,
	}
}

func (s *HandlerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// Draft lifecycle tests

func (s *HandlerTestSuite) TestCreateDraft() {
	s.Run("with valid request", func() {
		// Setup expected service call with explicit matching
		expectedInput := &character.CreateDraftInput{
			PlayerID:  s.testPlayerID,
			SessionID: s.testSessionID,
			InitialData: &dnd5e.CharacterDraft{
				Name: "Gandalf the Grey",
			},
		}

		// Use a copy of the expected draft with specific values for this test
		expectedDraft := &dnd5e.CharacterDraft{
			ID:                  s.testDraftID,
			PlayerID:            s.testPlayerID,
			SessionID:           s.testSessionID,
			Name:                "Gandalf the Grey",
			RaceID:              dnd5e.RaceHuman,
			ClassID:             dnd5e.ClassWizard,
			BackgroundID:        dnd5e.BackgroundSage,
			AbilityScores:       s.expectedAbilityScores,
			StartingSkillIDs:    []string{dnd5e.SkillArcana, dnd5e.SkillHistory},
			AdditionalLanguages: []string{dnd5e.LanguageElvish},
			Progress:            *s.expectedCreationProgress,
			CreatedAt:           1234567890,
			UpdatedAt:           1234567890,
		}

		s.mockCharService.EXPECT().
			CreateDraft(s.ctx, expectedInput).
			Return(&character.CreateDraftOutput{
				Draft: expectedDraft,
			}, nil)

		resp, err := s.handler.CreateDraft(s.ctx, s.validCreateDraftReq)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
		s.Equal(s.testDraftID, resp.Draft.Id)
		s.Equal("Gandalf the Grey", resp.Draft.Name)
		// Check more fields to ensure entity conversion works
		s.Equal(dnd5ev1alpha1.Race_RACE_HUMAN, resp.Draft.Race)
		s.Equal(dnd5ev1alpha1.Class_CLASS_WIZARD, resp.Draft.Class)
		s.Equal(dnd5ev1alpha1.Background_BACKGROUND_SAGE, resp.Draft.Background)
		s.Equal(int32(10), resp.Draft.AbilityScores.Strength)
		s.Equal(int32(18), resp.Draft.AbilityScores.Intelligence)
		s.Len(resp.Draft.StartingSkills, 2)
	})

	s.Run("with minimal request", func() {
		req := &dnd5ev1alpha1.CreateDraftRequest{
			PlayerId: s.testPlayerID,
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID:       s.testDraftID,
			PlayerID: s.testPlayerID,
		}

		s.mockCharService.EXPECT().
			CreateDraft(s.ctx, gomock.Any()).
			Return(&character.CreateDraftOutput{
				Draft: expectedDraft,
			}, nil)

		resp, err := s.handler.CreateDraft(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
		s.Equal(s.testDraftID, resp.Draft.Id)
	})

	s.Run("with missing player ID", func() {
		req := &dnd5ev1alpha1.CreateDraftRequest{
			SessionId: s.testSessionID,
		}

		resp, err := s.handler.CreateDraft(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
		s.Contains(st.Message(), "player_id is required")
	})

	s.Run("when service returns error", func() {
		s.mockCharService.EXPECT().
			CreateDraft(s.ctx, gomock.Any()).
			Return(nil, errors.New("database error"))

		resp, err := s.handler.CreateDraft(s.ctx, s.validCreateDraftReq)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.Internal, st.Code())
		s.Contains(st.Message(), "database error")
	})
}

func (s *HandlerTestSuite) TestGetDraft() {
	// Use the complete expected draft from suite
	s.mockCharService.EXPECT().
		GetDraft(s.ctx, &character.GetDraftInput{
			DraftID: s.testDraftID,
		}).
		Return(&character.GetDraftOutput{
			Draft: s.expectedDraft,
		}, nil)

	resp, err := s.handler.GetDraft(s.ctx, s.validGetDraftReq)

	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal(s.testDraftID, resp.Draft.Id)
	s.Equal("Gandalf the Grey", resp.Draft.Name)
	s.Equal(dnd5ev1alpha1.Race_RACE_HUMAN, resp.Draft.Race)
	s.Equal(dnd5ev1alpha1.Class_CLASS_WIZARD, resp.Draft.Class)
	s.NotNil(resp.Draft.AbilityScores)
	s.Equal(int32(18), resp.Draft.AbilityScores.Intelligence)
	s.NotNil(resp.Draft.Progress)
	s.Equal(dnd5ev1alpha1.CreationStep_CREATION_STEP_ABILITY_SCORES, resp.Draft.Progress.CurrentStep)
}

func (s *HandlerTestSuite) TestListDrafts() {
	s.Run("with valid request", func() {
		expectedDrafts := []*dnd5e.CharacterDraft{
			{
				ID:        "draft-1",
				PlayerID:  s.testPlayerID,
				SessionID: s.testSessionID,
				Name:      "Character 1",
			},
			{
				ID:        "draft-2",
				PlayerID:  s.testPlayerID,
				SessionID: s.testSessionID,
				Name:      "Character 2",
			},
		}

		s.mockCharService.EXPECT().
			ListDrafts(s.ctx, &character.ListDraftsInput{
				PlayerID:  s.testPlayerID,
				SessionID: s.testSessionID,
				PageSize:  20,
				PageToken: "",
			}).
			Return(&character.ListDraftsOutput{
				Drafts:        expectedDrafts,
				NextPageToken: "next-token",
			}, nil)

		resp, err := s.handler.ListDrafts(s.ctx, s.validListDraftsReq)

		s.NoError(err)
		s.NotNil(resp)
		s.Len(resp.Drafts, 2)
		s.Equal("next-token", resp.NextPageToken)
	})

	s.Run("with different page sizes", func() {
		testCases := []int32{1, 10, 50, 100}

		for _, pageSize := range testCases {
			s.Run(fmt.Sprintf("page_size_%d", pageSize), func() {
				req := &dnd5ev1alpha1.ListDraftsRequest{
					PlayerId: s.testPlayerID,
					PageSize: pageSize,
				}

				s.mockCharService.EXPECT().
					ListDrafts(s.ctx, &character.ListDraftsInput{
						PlayerID:  s.testPlayerID,
						PageSize:  pageSize,
						PageToken: "",
					}).
					Return(&character.ListDraftsOutput{
						Drafts: []*dnd5e.CharacterDraft{},
					}, nil)

				resp, err := s.handler.ListDrafts(s.ctx, req)

				s.NoError(err)
				s.NotNil(resp)
			})
		}
	})
}

// Section update tests

func (s *HandlerTestSuite) TestUpdateName() {
	s.Run("with valid request", func() {
		expectedDraft := &dnd5e.CharacterDraft{
			ID:       s.testDraftID,
			PlayerID: s.testPlayerID,
			Name:     "Gandalf the White",
		}

		s.mockCharService.EXPECT().
			UpdateName(s.ctx, &character.UpdateNameInput{
				DraftID: s.testDraftID,
				Name:    "Gandalf the White",
			}).
			Return(&character.UpdateNameOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.UpdateName(s.ctx, s.validUpdateNameReq)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
		s.Equal(s.testDraftID, resp.Draft.Id)
		s.Equal("Gandalf the White", resp.Draft.Name)
	})

	s.Run("with missing draft_id", func() {
		req := &dnd5ev1alpha1.UpdateNameRequest{
			Name: "Some Name",
		}

		resp, err := s.handler.UpdateName(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
		s.Contains(st.Message(), "draft_id is required")
	})

	s.Run("with missing name", func() {
		req := &dnd5ev1alpha1.UpdateNameRequest{
			DraftId: s.testDraftID,
		}

		resp, err := s.handler.UpdateName(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
		s.Contains(st.Message(), "name is required")
	})
}

func (s *HandlerTestSuite) TestUpdateRace() {
	s.Run("with valid race", func() {
		expectedDraft := &dnd5e.CharacterDraft{
			ID:       s.testDraftID,
			PlayerID: s.testPlayerID,
			RaceID:   dnd5e.RaceHuman,
		}

		s.mockCharService.EXPECT().
			UpdateRace(s.ctx, &character.UpdateRaceInput{
				DraftID:   s.testDraftID,
				RaceID:    dnd5e.RaceHuman,
				SubraceID: "",
			}).
			Return(&character.UpdateRaceOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.UpdateRace(s.ctx, s.validUpdateRaceReq)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
		s.Equal(s.testDraftID, resp.Draft.Id)
	})

	s.Run("with subrace", func() {
		req := &dnd5ev1alpha1.UpdateRaceRequest{
			DraftId: s.testDraftID,
			Race:    dnd5ev1alpha1.Race_RACE_ELF,
			Subrace: dnd5ev1alpha1.Subrace_SUBRACE_HIGH_ELF,
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID:        s.testDraftID,
			PlayerID:  s.testPlayerID,
			RaceID:    dnd5e.RaceElf,
			SubraceID: dnd5e.SubraceHighElf,
		}

		s.mockCharService.EXPECT().
			UpdateRace(s.ctx, &character.UpdateRaceInput{
				DraftID:   s.testDraftID,
				RaceID:    dnd5e.RaceElf,
				SubraceID: dnd5e.SubraceHighElf,
			}).
			Return(&character.UpdateRaceOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.UpdateRace(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
		s.Equal(s.testDraftID, resp.Draft.Id)
	})

	s.Run("with missing draft_id", func() {
		req := &dnd5ev1alpha1.UpdateRaceRequest{
			Race: dnd5ev1alpha1.Race_RACE_HUMAN,
		}

		resp, err := s.handler.UpdateRace(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
		s.Contains(st.Message(), "draft_id is required")
	})
}

func (s *HandlerTestSuite) TestUpdateClass() {
	s.Run("with valid request", func() {
		expectedDraft := &dnd5e.CharacterDraft{
			ID:      s.testDraftID,
			ClassID: dnd5e.ClassWizard,
		}

		s.mockCharService.EXPECT().
			UpdateClass(s.ctx, &character.UpdateClassInput{
				DraftID: s.testDraftID,
				ClassID: dnd5e.ClassWizard,
			}).
			Return(&character.UpdateClassOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.UpdateClass(s.ctx, s.validUpdateClassReq)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
	})

	s.Run("with missing draft_id", func() {
		req := &dnd5ev1alpha1.UpdateClassRequest{
			Class: dnd5ev1alpha1.Class_CLASS_WIZARD,
		}

		resp, err := s.handler.UpdateClass(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

func (s *HandlerTestSuite) TestUpdateBackground() {
	s.Run("with valid background", func() {
		req := &dnd5ev1alpha1.UpdateBackgroundRequest{
			DraftId:    s.testDraftID,
			Background: dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE,
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID:           s.testDraftID,
			BackgroundID: dnd5e.BackgroundAcolyte,
		}

		s.mockCharService.EXPECT().
			UpdateBackground(s.ctx, &character.UpdateBackgroundInput{
				DraftID:      s.testDraftID,
				BackgroundID: dnd5e.BackgroundAcolyte,
			}).
			Return(&character.UpdateBackgroundOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.UpdateBackground(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
	})

	s.Run("with missing draft_id", func() {
		req := &dnd5ev1alpha1.UpdateBackgroundRequest{
			Background: dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE,
		}

		resp, err := s.handler.UpdateBackground(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

func (s *HandlerTestSuite) TestUpdateAbilityScores() {
	s.Run("with valid ability scores", func() {
		req := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
			DraftId: s.testDraftID,
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     15,
				Dexterity:    14,
				Constitution: 13,
				Intelligence: 12,
				Wisdom:       10,
				Charisma:     8,
			},
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID: s.testDraftID,
			AbilityScores: &dnd5e.AbilityScores{
				Strength:     15,
				Dexterity:    14,
				Constitution: 13,
				Intelligence: 12,
				Wisdom:       10,
				Charisma:     8,
			},
		}

		s.mockCharService.EXPECT().
			UpdateAbilityScores(s.ctx, &character.UpdateAbilityScoresInput{
				DraftID: s.testDraftID,
				AbilityScores: dnd5e.AbilityScores{
					Strength:     15,
					Dexterity:    14,
					Constitution: 13,
					Intelligence: 12,
					Wisdom:       10,
					Charisma:     8,
				},
			}).
			Return(&character.UpdateAbilityScoresOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.UpdateAbilityScores(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
	})

	s.Run("with missing ability_scores", func() {
		req := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
			DraftId: s.testDraftID,
		}

		resp, err := s.handler.UpdateAbilityScores(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

func (s *HandlerTestSuite) TestUpdateSkills() {
	s.Run("with valid skills", func() {
		req := &dnd5ev1alpha1.UpdateSkillsRequest{
			DraftId: s.testDraftID,
			Skills: []dnd5ev1alpha1.Skill{
				dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
				dnd5ev1alpha1.Skill_SKILL_PERCEPTION,
			},
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID: s.testDraftID,
			StartingSkillIDs: []string{
				dnd5e.SkillAthletics,
				dnd5e.SkillPerception,
			},
		}

		s.mockCharService.EXPECT().
			UpdateSkills(s.ctx, &character.UpdateSkillsInput{
				DraftID: s.testDraftID,
				SkillIDs: []string{
					dnd5e.SkillAthletics,
					dnd5e.SkillPerception,
				},
			}).
			Return(&character.UpdateSkillsOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.UpdateSkills(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
	})

	s.Run("with missing draft_id", func() {
		req := &dnd5ev1alpha1.UpdateSkillsRequest{
			Skills: []dnd5ev1alpha1.Skill{dnd5ev1alpha1.Skill_SKILL_ATHLETICS},
		}

		resp, err := s.handler.UpdateSkills(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

func (s *HandlerTestSuite) TestValidateDraft() {
	s.Run("with valid draft", func() {
		req := &dnd5ev1alpha1.ValidateDraftRequest{
			DraftId: s.testDraftID,
		}

		s.mockCharService.EXPECT().
			ValidateDraft(s.ctx, &character.ValidateDraftInput{
				DraftID: s.testDraftID,
			}).
			Return(&character.ValidateDraftOutput{
				IsValid:  true,
				Errors:   []character.ValidationError{},
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.ValidateDraft(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.True(resp.IsValid)
		s.Empty(resp.Errors)
		s.Empty(resp.Warnings)
	})

	s.Run("with missing draft_id", func() {
		req := &dnd5ev1alpha1.ValidateDraftRequest{}

		resp, err := s.handler.ValidateDraft(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

func (s *HandlerTestSuite) TestDeleteDraft() {
	s.Run("with valid draft_id", func() {
		req := &dnd5ev1alpha1.DeleteDraftRequest{
			DraftId: s.testDraftID,
		}

		s.mockCharService.EXPECT().
			DeleteDraft(s.ctx, &character.DeleteDraftInput{
				DraftID: s.testDraftID,
			}).
			Return(&character.DeleteDraftOutput{
				Message: "Draft deleted successfully",
			}, nil)

		resp, err := s.handler.DeleteDraft(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.Contains(resp.Message, "deleted successfully")
	})

	s.Run("with missing draft_id", func() {
		req := &dnd5ev1alpha1.DeleteDraftRequest{}

		resp, err := s.handler.DeleteDraft(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

// Finalization tests

func (s *HandlerTestSuite) TestFinalizeDraft() {
	s.Run("with valid request", func() {
		expectedCharacter := &dnd5e.Character{
			ID:       s.testCharacterID,
			PlayerID: s.testPlayerID,
			Name:     "Gandalf",
		}

		s.mockCharService.EXPECT().
			FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
				DraftID: s.testDraftID,
			}).
			Return(&character.FinalizeDraftOutput{
				Character: expectedCharacter,
			}, nil)

		resp, err := s.handler.FinalizeDraft(s.ctx, s.validFinalizeDraftReq)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Character)
		s.Equal(s.testCharacterID, resp.Character.Id)
	})

	s.Run("with missing draft_id", func() {
		req := &dnd5ev1alpha1.FinalizeDraftRequest{}

		resp, err := s.handler.FinalizeDraft(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

// Character operation tests

func (s *HandlerTestSuite) TestGetCharacter() {
	s.Run("with valid character_id", func() {
		expectedCharacter := &dnd5e.Character{
			ID:       s.testCharacterID,
			PlayerID: s.testPlayerID,
			Name:     "Gandalf",
		}

		s.mockCharService.EXPECT().
			GetCharacter(s.ctx, &character.GetCharacterInput{
				CharacterID: s.testCharacterID,
			}).
			Return(&character.GetCharacterOutput{
				Character: expectedCharacter,
			}, nil)

		resp, err := s.handler.GetCharacter(s.ctx, s.validGetCharacterReq)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Character)
		s.Equal(s.testCharacterID, resp.Character.Id)
	})

	s.Run("with missing character_id", func() {
		req := &dnd5ev1alpha1.GetCharacterRequest{}

		resp, err := s.handler.GetCharacter(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

func (s *HandlerTestSuite) TestListCharacters() {
	s.Run("with all filters", func() {
		expectedCharacters := []*dnd5e.Character{
			{
				ID:        "char-1",
				PlayerID:  s.testPlayerID,
				SessionID: s.testSessionID,
				Name:      "Character 1",
			},
			{
				ID:        "char-2",
				PlayerID:  s.testPlayerID,
				SessionID: s.testSessionID,
				Name:      "Character 2",
			},
		}

		s.mockCharService.EXPECT().
			ListCharacters(s.ctx, &character.ListCharactersInput{
				PlayerID:  s.testPlayerID,
				SessionID: s.testSessionID,
				PageSize:  20,
				PageToken: "",
			}).
			Return(&character.ListCharactersOutput{
				Characters:    expectedCharacters,
				NextPageToken: "next-token",
			}, nil)

		resp, err := s.handler.ListCharacters(s.ctx, s.validListCharactersReq)

		s.NoError(err)
		s.NotNil(resp)
		s.Len(resp.Characters, 2)
		s.Equal("next-token", resp.NextPageToken)
	})

	s.Run("with only player filter", func() {
		req := &dnd5ev1alpha1.ListCharactersRequest{
			PlayerId: s.testPlayerID,
			PageSize: 20,
		}

		s.mockCharService.EXPECT().
			ListCharacters(s.ctx, &character.ListCharactersInput{
				PlayerID:  s.testPlayerID,
				SessionID: "",
				PageSize:  20,
				PageToken: "",
			}).
			Return(&character.ListCharactersOutput{
				Characters:    []*dnd5e.Character{},
				NextPageToken: "",
			}, nil)

		resp, err := s.handler.ListCharacters(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
	})

	s.Run("with only session filter", func() {
		req := &dnd5ev1alpha1.ListCharactersRequest{
			SessionId: s.testSessionID,
			PageSize:  20,
		}

		s.mockCharService.EXPECT().
			ListCharacters(s.ctx, &character.ListCharactersInput{
				PlayerID:  "",
				SessionID: s.testSessionID,
				PageSize:  20,
				PageToken: "",
			}).
			Return(&character.ListCharactersOutput{
				Characters:    []*dnd5e.Character{},
				NextPageToken: "",
			}, nil)

		resp, err := s.handler.ListCharacters(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
	})
}

func (s *HandlerTestSuite) TestDeleteCharacter() {
	s.Run("with valid character_id", func() {
		s.mockCharService.EXPECT().
			DeleteCharacter(s.ctx, &character.DeleteCharacterInput{
				CharacterID: s.testCharacterID,
			}).
			Return(&character.DeleteCharacterOutput{}, nil)

		resp, err := s.handler.DeleteCharacter(s.ctx, s.validDeleteCharacterReq)

		s.NoError(err)
		s.NotNil(resp)
		s.Contains(resp.Message, "deleted successfully")
	})

	s.Run("with missing character_id", func() {
		req := &dnd5ev1alpha1.DeleteCharacterRequest{}

		resp, err := s.handler.DeleteCharacter(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}
