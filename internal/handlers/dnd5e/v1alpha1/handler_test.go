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

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
)

type HandlerTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharService *charactermock.MockService
	handler         *v1alpha1.Handler
	ctx             context.Context

	// Test data - valid requests we can use across tests
	validCreateDraftReq         *dnd5ev1alpha1.CreateDraftRequest
	validGetDraftReq            *dnd5ev1alpha1.GetDraftRequest
	validListDraftsReq          *dnd5ev1alpha1.ListDraftsRequest
	validUpdateNameReq          *dnd5ev1alpha1.UpdateNameRequest
	validUpdateRaceReq          *dnd5ev1alpha1.UpdateRaceRequest
	validUpdateClassReq         *dnd5ev1alpha1.UpdateClassRequest
	validFinalizeDraftReq       *dnd5ev1alpha1.FinalizeDraftRequest
	validGetCharacterReq        *dnd5ev1alpha1.GetCharacterRequest
	validListCharactersReq      *dnd5ev1alpha1.ListCharactersRequest
	validDeleteCharacterReq     *dnd5ev1alpha1.DeleteCharacterRequest
	validListEquipmentByTypeReq *dnd5ev1alpha1.ListEquipmentByTypeRequest
	validListSpellsByLevelReq   *dnd5ev1alpha1.ListSpellsByLevelRequest

	// Equipment management requests
	validGetInventoryReq        *dnd5ev1alpha1.GetCharacterInventoryRequest
	validEquipItemReq           *dnd5ev1alpha1.EquipItemRequest
	validUnequipItemReq         *dnd5ev1alpha1.UnequipItemRequest
	validAddToInventoryReq      *dnd5ev1alpha1.AddToInventoryRequest
	validRemoveFromInventoryReq *dnd5ev1alpha1.RemoveFromInventoryRequest

	// Common test IDs
	testPlayerID    string
	testSessionID   string
	testDraftID     string
	testCharacterID string
	testItemID      string

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
	s.testItemID = "item-longsword"

	// Initialize valid requests - these can be modified in specific tests
	s.validCreateDraftReq = &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId:  s.testPlayerID,
		SessionId: s.testSessionID,
		InitialData: &dnd5ev1alpha1.CharacterDraftData{
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

	s.validListEquipmentByTypeReq = &dnd5ev1alpha1.ListEquipmentByTypeRequest{
		EquipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_MELEE_WEAPON,
		PageSize:      20,
		PageToken:     "",
	}

	s.validListSpellsByLevelReq = &dnd5ev1alpha1.ListSpellsByLevelRequest{
		Level:     0, // cantrips
		Class:     dnd5ev1alpha1.Class_CLASS_WIZARD,
		PageSize:  20,
		PageToken: "",
	}

	// Initialize equipment management requests
	s.validGetInventoryReq = &dnd5ev1alpha1.GetCharacterInventoryRequest{
		CharacterId: s.testCharacterID,
	}

	s.validEquipItemReq = &dnd5ev1alpha1.EquipItemRequest{
		CharacterId: s.testCharacterID,
		ItemId:      s.testItemID,
		Slot:        dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
	}

	s.validUnequipItemReq = &dnd5ev1alpha1.UnequipItemRequest{
		CharacterId: s.testCharacterID,
		Slot:        dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
	}

	s.validAddToInventoryReq = &dnd5ev1alpha1.AddToInventoryRequest{
		CharacterId: s.testCharacterID,
		Items: []*dnd5ev1alpha1.InventoryAddition{
			{
				ItemId:   s.testItemID,
				Quantity: 1,
			},
		},
	}

	s.validRemoveFromInventoryReq = &dnd5ev1alpha1.RemoveFromInventoryRequest{
		CharacterId: s.testCharacterID,
		ItemId:      s.testItemID,
		RemovalAmount: &dnd5ev1alpha1.RemoveFromInventoryRequest_Quantity{
			Quantity: 1,
		},
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
		StepsCompleted: dnd5e.ProgressStepName | dnd5e.ProgressStepRace |
			dnd5e.ProgressStepClass | dnd5e.ProgressStepBackground |
			dnd5e.ProgressStepAbilityScores,
		CompletionPercentage: 71,
		CurrentStep:          dnd5e.CreationStepAbilityScores,
	}

	s.expectedDraft = &dnd5e.CharacterDraft{
		ID:            s.testDraftID,
		PlayerID:      s.testPlayerID,
		SessionID:     s.testSessionID,
		Name:          "Gandalf the Grey",
		RaceID:        dnd5e.RaceHuman,
		SubraceID:     "",
		ClassID:       dnd5e.ClassWizard,
		BackgroundID:  dnd5e.BackgroundSage,
		Alignment:     dnd5e.AlignmentLawfulGood,
		AbilityScores: s.expectedAbilityScores,
		// Skills and languages are now handled through ChoiceSelections
		Progress:  *s.expectedCreationProgress,
		CreatedAt: 1234567890,
		UpdatedAt: 1234567890,
		// Include populated Info objects as orchestrator would provide them
		Race: &dnd5e.RaceInfo{
			ID:   dnd5e.RaceHuman,
			Name: "Human",
		},
		Class: &dnd5e.ClassInfo{
			ID:   dnd5e.ClassWizard,
			Name: "Wizard",
		},
		Background: &dnd5e.BackgroundInfo{
			ID:   dnd5e.BackgroundSage,
			Name: "Sage",
		},
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
			ID:            s.testDraftID,
			PlayerID:      s.testPlayerID,
			SessionID:     s.testSessionID,
			Name:          "Gandalf the Grey",
			RaceID:        dnd5e.RaceHuman,
			ClassID:       dnd5e.ClassWizard,
			BackgroundID:  dnd5e.BackgroundSage,
			AbilityScores: s.expectedAbilityScores,
			// Skills and languages are now handled through ChoiceSelections
			Progress:  *s.expectedCreationProgress,
			CreatedAt: 1234567890,
			UpdatedAt: 1234567890,
			// Include populated Info objects as orchestrator would provide them
			Race: &dnd5e.RaceInfo{
				ID:   dnd5e.RaceHuman,
				Name: "Human",
			},
			Class: &dnd5e.ClassInfo{
				ID:   dnd5e.ClassWizard,
				Name: "Wizard",
			},
			Background: &dnd5e.BackgroundInfo{
				ID:   dnd5e.BackgroundSage,
				Name: "Sage",
			},
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
		s.NotNil(resp.Draft.Race)
		s.Equal("Human", resp.Draft.Race.Name)
		s.NotNil(resp.Draft.Class)
		s.Equal("Wizard", resp.Draft.Class.Name)
		s.NotNil(resp.Draft.Background)
		s.Equal("Sage", resp.Draft.Background.Name)
		s.Equal(int32(10), resp.Draft.AbilityScores.Strength)
		s.Equal(int32(18), resp.Draft.AbilityScores.Intelligence)
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
	s.NotNil(resp.Draft.Race)
	s.Equal("Human", resp.Draft.Race.Name)
	s.NotNil(resp.Draft.Class)
	s.Equal("Wizard", resp.Draft.Class.Name)
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

func (s *HandlerTestSuite) TestUpdateRace_Basic() {
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
				Choices:   []dnd5e.ChoiceSelection{},
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
				Choices:   []dnd5e.ChoiceSelection{},
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

func (s *HandlerTestSuite) TestUpdateRace_WithChoices() {
	s.Run("with race choices", func() {
		req := &dnd5ev1alpha1.UpdateRaceRequest{
			DraftId: s.testDraftID,
			Race:    dnd5ev1alpha1.Race_RACE_HALF_ELF,
			RaceChoices: []*dnd5ev1alpha1.ChoiceSelection{
				{
					ChoiceId:     "half-elf-ability-score-increase",
					ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SKILL,
					Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
					SelectedKeys: []string{"deception", "insight"},
				},
				{
					ChoiceId:     "half-elf-languages",
					ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_LANGUAGE,
					Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
					SelectedKeys: []string{"elvish", "dwarvish"},
				},
			},
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID:       s.testDraftID,
			PlayerID: s.testPlayerID,
			RaceID:   dnd5e.RaceHalfElf,
			Race: &dnd5e.RaceInfo{
				ID:   dnd5e.RaceHalfElf,
				Name: "Half-Elf",
			},
		}

		s.mockCharService.EXPECT().
			UpdateRace(s.ctx, &character.UpdateRaceInput{
				DraftID:   s.testDraftID,
				RaceID:    dnd5e.RaceHalfElf,
				SubraceID: "",
				Choices: []dnd5e.ChoiceSelection{
					{
						ChoiceID:     "half-elf-ability-score-increase",
						ChoiceType:   dnd5e.ChoiceTypeSkill,
						Source:       dnd5e.ChoiceSourceRace,
						SelectedKeys: []string{"deception", "insight"},
					},
					{
						ChoiceID:     "half-elf-languages",
						ChoiceType:   dnd5e.ChoiceTypeLanguage,
						Source:       dnd5e.ChoiceSourceRace,
						SelectedKeys: []string{"elvish", "dwarvish"},
					},
				},
			}).
			Return(&character.UpdateRaceOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil).
			Times(1)

		resp, err := s.handler.UpdateRace(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
		s.Equal(s.testDraftID, resp.Draft.Id)
	})

	s.Run("with ability score choices", func() {
		req := &dnd5ev1alpha1.UpdateRaceRequest{
			DraftId: s.testDraftID,
			Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
			RaceChoices: []*dnd5ev1alpha1.ChoiceSelection{
				{
					ChoiceId:   "variant-human-ability-scores",
					ChoiceType: dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SKILL,
					Source:     dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
					AbilityScoreChoices: []*dnd5ev1alpha1.AbilityScoreChoice{
						{
							Ability: dnd5ev1alpha1.Ability_ABILITY_STRENGTH,
							Bonus:   1,
						},
						{
							Ability: dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION,
							Bonus:   1,
						},
					},
				},
			},
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID:       s.testDraftID,
			PlayerID: s.testPlayerID,
			RaceID:   dnd5e.RaceHuman,
			Race: &dnd5e.RaceInfo{
				ID:   dnd5e.RaceHuman,
				Name: "Human",
			},
		}

		s.mockCharService.EXPECT().
			UpdateRace(s.ctx, &character.UpdateRaceInput{
				DraftID:   s.testDraftID,
				RaceID:    dnd5e.RaceHuman,
				SubraceID: "",
				Choices: []dnd5e.ChoiceSelection{
					{
						ChoiceID:   "variant-human-ability-scores",
						ChoiceType: dnd5e.ChoiceTypeSkill,
						Source:     dnd5e.ChoiceSourceRace,
						AbilityScoreChoices: []dnd5e.AbilityScoreChoice{
							{
								Ability: dnd5e.AbilityStrength,
								Bonus:   1,
							},
							{
								Ability: dnd5e.AbilityConstitution,
								Bonus:   1,
							},
						},
					},
				},
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

	s.Run("with nil choice in array", func() {
		req := &dnd5ev1alpha1.UpdateRaceRequest{
			DraftId: s.testDraftID,
			Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
			RaceChoices: []*dnd5ev1alpha1.ChoiceSelection{
				{
					ChoiceId:     "human-skill",
					ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SKILL,
					Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
					SelectedKeys: []string{"athletics"},
				},
				nil, // This should be filtered out
				{
					ChoiceId:     "human-language",
					ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_LANGUAGE,
					Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
					SelectedKeys: []string{"orcish"},
				},
			},
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID:       s.testDraftID,
			PlayerID: s.testPlayerID,
			RaceID:   dnd5e.RaceHuman,
			Race: &dnd5e.RaceInfo{
				ID:   dnd5e.RaceHuman,
				Name: "Human",
			},
		}

		// Expect that nil choices are filtered out
		s.mockCharService.EXPECT().
			UpdateRace(s.ctx, &character.UpdateRaceInput{
				DraftID:   s.testDraftID,
				RaceID:    dnd5e.RaceHuman,
				SubraceID: "",
				Choices: []dnd5e.ChoiceSelection{
					{
						ChoiceID:     "human-skill",
						ChoiceType:   dnd5e.ChoiceTypeSkill,
						Source:       dnd5e.ChoiceSourceRace,
						SelectedKeys: []string{"athletics"},
					},
					{
						ChoiceID:     "human-language",
						ChoiceType:   dnd5e.ChoiceTypeLanguage,
						Source:       dnd5e.ChoiceSourceRace,
						SelectedKeys: []string{"orcish"},
					},
				},
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

	s.Run("with empty choices array", func() {
		req := &dnd5ev1alpha1.UpdateRaceRequest{
			DraftId:     s.testDraftID,
			Race:        dnd5ev1alpha1.Race_RACE_DWARF,
			RaceChoices: []*dnd5ev1alpha1.ChoiceSelection{},
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID:       s.testDraftID,
			PlayerID: s.testPlayerID,
			RaceID:   dnd5e.RaceDwarf,
			Race: &dnd5e.RaceInfo{
				ID:   dnd5e.RaceDwarf,
				Name: "Dwarf",
			},
		}

		s.mockCharService.EXPECT().
			UpdateRace(s.ctx, &character.UpdateRaceInput{
				DraftID:   s.testDraftID,
				RaceID:    dnd5e.RaceDwarf,
				SubraceID: "",
				Choices:   []dnd5e.ChoiceSelection{},
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
}

func (s *HandlerTestSuite) TestUpdateClass_Basic() {
	s.Run("with valid request", func() {
		expectedDraft := &dnd5e.CharacterDraft{
			ID:      s.testDraftID,
			ClassID: dnd5e.ClassWizard,
		}

		s.mockCharService.EXPECT().
			UpdateClass(s.ctx, &character.UpdateClassInput{
				DraftID: s.testDraftID,
				ClassID: dnd5e.ClassWizard,
				Choices: []dnd5e.ChoiceSelection{},
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

func (s *HandlerTestSuite) TestUpdateClass_WithChoices() {
	s.Run("with class choices", func() {
		req := &dnd5ev1alpha1.UpdateClassRequest{
			DraftId: s.testDraftID,
			Class:   dnd5ev1alpha1.Class_CLASS_FIGHTER,
			ClassChoices: []*dnd5ev1alpha1.ChoiceSelection{
				{
					ChoiceId:     "fighter-fighting-style",
					ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_FEAT,
					Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
					SelectedKeys: []string{"defense"},
				},
				{
					ChoiceId:     "fighter-starting-equipment",
					ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT,
					Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
					SelectedKeys: []string{"chain-mail", "longsword", "shield"},
				},
			},
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID:      s.testDraftID,
			ClassID: dnd5e.ClassFighter,
			Class: &dnd5e.ClassInfo{
				ID:   dnd5e.ClassFighter,
				Name: "Fighter",
			},
		}

		s.mockCharService.EXPECT().
			UpdateClass(s.ctx, &character.UpdateClassInput{
				DraftID: s.testDraftID,
				ClassID: dnd5e.ClassFighter,
				Choices: []dnd5e.ChoiceSelection{
					{
						ChoiceID:     "fighter-fighting-style",
						ChoiceType:   dnd5e.ChoiceTypeFeat,
						Source:       dnd5e.ChoiceSourceClass,
						SelectedKeys: []string{"defense"},
					},
					{
						ChoiceID:     "fighter-starting-equipment",
						ChoiceType:   dnd5e.ChoiceTypeEquipment,
						Source:       dnd5e.ChoiceSourceClass,
						SelectedKeys: []string{"chain-mail", "longsword", "shield"},
					},
				},
			}).
			Return(&character.UpdateClassOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.UpdateClass(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
		s.Equal(s.testDraftID, resp.Draft.Id)
		s.NotNil(resp.Draft.Class)
		s.Equal("Fighter", resp.Draft.Class.Name)
	})

	s.Run("with spellcasting class choices", func() {
		req := &dnd5ev1alpha1.UpdateClassRequest{
			DraftId: s.testDraftID,
			Class:   dnd5ev1alpha1.Class_CLASS_WIZARD,
			ClassChoices: []*dnd5ev1alpha1.ChoiceSelection{
				{
					ChoiceId:     "wizard-cantrips",
					ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SPELL,
					Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
					SelectedKeys: []string{"fire-bolt", "mage-hand", "prestidigitation"},
				},
				{
					ChoiceId:     "wizard-1st-level-spells",
					ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SPELL,
					Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
					SelectedKeys: []string{"magic-missile", "shield", "identify", "detect-magic", "sleep", "charm-person"},
				},
			},
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID:      s.testDraftID,
			ClassID: dnd5e.ClassWizard,
			Class: &dnd5e.ClassInfo{
				ID:   dnd5e.ClassWizard,
				Name: "Wizard",
			},
		}

		s.mockCharService.EXPECT().
			UpdateClass(s.ctx, &character.UpdateClassInput{
				DraftID: s.testDraftID,
				ClassID: dnd5e.ClassWizard,
				Choices: []dnd5e.ChoiceSelection{
					{
						ChoiceID:     "wizard-cantrips",
						ChoiceType:   dnd5e.ChoiceTypeSpell,
						Source:       dnd5e.ChoiceSourceClass,
						SelectedKeys: []string{"fire-bolt", "mage-hand", "prestidigitation"},
					},
					{
						ChoiceID:     "wizard-1st-level-spells",
						ChoiceType:   dnd5e.ChoiceTypeSpell,
						Source:       dnd5e.ChoiceSourceClass,
						SelectedKeys: []string{"magic-missile", "shield", "identify", "detect-magic", "sleep", "charm-person"},
					},
				},
			}).
			Return(&character.UpdateClassOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.UpdateClass(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
		s.Equal(s.testDraftID, resp.Draft.Id)
	})
}

func (s *HandlerTestSuite) TestUpdateBackground_Basic() {
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
				Choices:      []dnd5e.ChoiceSelection{},
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

func (s *HandlerTestSuite) TestUpdateBackground_WithChoices() {
	s.Run("with background choices", func() {
		req := &dnd5ev1alpha1.UpdateBackgroundRequest{
			DraftId:    s.testDraftID,
			Background: dnd5ev1alpha1.Background_BACKGROUND_FOLK_HERO,
			BackgroundChoices: []*dnd5ev1alpha1.ChoiceSelection{
				{
					ChoiceId:     "folk-hero-tool-proficiency",
					ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_TOOL,
					Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND,
					SelectedKeys: []string{"carpenters-tools"},
				},
				{
					ChoiceId:     "folk-hero-language",
					ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_LANGUAGE,
					Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND,
					SelectedKeys: []string{"dwarvish"},
				},
			},
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID:           s.testDraftID,
			BackgroundID: dnd5e.BackgroundFolkHero,
			Background: &dnd5e.BackgroundInfo{
				ID:   dnd5e.BackgroundFolkHero,
				Name: "Folk Hero",
			},
		}

		s.mockCharService.EXPECT().
			UpdateBackground(s.ctx, &character.UpdateBackgroundInput{
				DraftID:      s.testDraftID,
				BackgroundID: dnd5e.BackgroundFolkHero,
				Choices: []dnd5e.ChoiceSelection{
					{
						ChoiceID:     "folk-hero-tool-proficiency",
						ChoiceType:   dnd5e.ChoiceTypeTool,
						Source:       dnd5e.ChoiceSourceBackground,
						SelectedKeys: []string{"carpenters-tools"},
					},
					{
						ChoiceID:     "folk-hero-language",
						ChoiceType:   dnd5e.ChoiceTypeLanguage,
						Source:       dnd5e.ChoiceSourceBackground,
						SelectedKeys: []string{"dwarvish"},
					},
				},
			}).
			Return(&character.UpdateBackgroundOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.UpdateBackground(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
		s.Equal(s.testDraftID, resp.Draft.Id)
		s.NotNil(resp.Draft.Background)
		s.Equal("Folk Hero", resp.Draft.Background.Name)
	})

	s.Run("with multiple language choices", func() {
		req := &dnd5ev1alpha1.UpdateBackgroundRequest{
			DraftId:    s.testDraftID,
			Background: dnd5ev1alpha1.Background_BACKGROUND_SAGE,
			BackgroundChoices: []*dnd5ev1alpha1.ChoiceSelection{
				{
					ChoiceId:     "sage-languages",
					ChoiceType:   dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_LANGUAGE,
					Source:       dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND,
					SelectedKeys: []string{"draconic", "celestial"},
				},
			},
		}

		expectedDraft := &dnd5e.CharacterDraft{
			ID:           s.testDraftID,
			BackgroundID: dnd5e.BackgroundSage,
			Background: &dnd5e.BackgroundInfo{
				ID:   dnd5e.BackgroundSage,
				Name: "Sage",
			},
		}

		s.mockCharService.EXPECT().
			UpdateBackground(s.ctx, &character.UpdateBackgroundInput{
				DraftID:      s.testDraftID,
				BackgroundID: dnd5e.BackgroundSage,
				Choices: []dnd5e.ChoiceSelection{
					{
						ChoiceID:     "sage-languages",
						ChoiceType:   dnd5e.ChoiceTypeLanguage,
						Source:       dnd5e.ChoiceSourceBackground,
						SelectedKeys: []string{"draconic", "celestial"},
					},
				},
			}).
			Return(&character.UpdateBackgroundOutput{
				Draft:    expectedDraft,
				Warnings: []character.ValidationWarning{},
			}, nil)

		resp, err := s.handler.UpdateBackground(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Draft)
		s.Equal(s.testDraftID, resp.Draft.Id)
	})
}

func (s *HandlerTestSuite) TestUpdateAbilityScores() {
	s.Run("with valid ability scores", func() {
		req := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
			DraftId: s.testDraftID,
			ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
				AbilityScores: &dnd5ev1alpha1.AbilityScores{
					Strength:     15,
					Dexterity:    14,
					Constitution: 13,
					Intelligence: 12,
					Wisdom:       10,
					Charisma:     8,
				},
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
			// Skills are now handled through ChoiceSelections
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

//nolint:dupl // Test functions may have similar structure but test different functionality
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

//nolint:dupl // Test functions may have similar structure but test different functionality
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

// Equipment and spell listing tests

func (s *HandlerTestSuite) TestListEquipmentByType() {
	s.Run("with valid equipment type", func() {
		expectedEquipment := []*dnd5e.EquipmentInfo{
			{
				ID:          "dagger",
				Name:        "Dagger",
				Type:        "weapon",
				Category:    "simple-melee-weapon",
				Cost:        "2 gp",
				Weight:      "1 lb",
				Description: "A simple melee weapon",
				Properties:  []string{"finesse", "light", "thrown"},
			},
			{
				ID:          "club",
				Name:        "Club",
				Type:        "weapon",
				Category:    "simple-melee-weapon",
				Cost:        "1 sp",
				Weight:      "2 lbs",
				Description: "A simple melee weapon",
				Properties:  []string{"light"},
			},
		}

		s.mockCharService.EXPECT().
			ListEquipmentByType(s.ctx, &character.ListEquipmentByTypeInput{
				EquipmentType: "simple-melee-weapons",
				PageSize:      20,
				PageToken:     "",
			}).
			Return(&character.ListEquipmentByTypeOutput{
				Equipment:     expectedEquipment,
				NextPageToken: "next-token",
				TotalSize:     2,
			}, nil)

		resp, err := s.handler.ListEquipmentByType(s.ctx, s.validListEquipmentByTypeReq)

		s.NoError(err)
		s.NotNil(resp)
		s.Len(resp.Equipment, 2)
		s.Equal("next-token", resp.NextPageToken)
		s.Equal(int32(2), resp.TotalSize)

		// Check first equipment item conversion
		equipment := resp.Equipment[0]
		s.Equal("dagger", equipment.Id)
		s.Equal("Dagger", equipment.Name)
		s.Equal("simple-melee-weapon", equipment.Category)
		s.Equal("A simple melee weapon", equipment.Description)
		s.NotNil(equipment.Cost)
		s.Equal("gp", equipment.Cost.Unit)
		s.Equal(int32(2), equipment.Cost.Quantity) // Parsed from "2 gp"
		s.NotNil(equipment.Weight)
		s.Equal("lb", equipment.Weight.Unit)
		s.Equal(int32(1), equipment.Weight.Quantity) // Parsed from "1 lb"
	})

	s.Run("with different equipment types", func() {
		testCases := []struct {
			name          string
			equipmentType dnd5ev1alpha1.EquipmentType
			expectedType  string
		}{
			{
				name:          "simple_melee_weapon",
				equipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_MELEE_WEAPON,
				expectedType:  "simple-melee-weapons",
			},
			{
				name:          "martial_melee_weapon",
				equipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_MELEE_WEAPON,
				expectedType:  "martial-melee-weapons",
			},
			{
				name:          "simple_ranged_weapon",
				equipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_RANGED_WEAPON,
				expectedType:  "simple-ranged-weapons",
			},
			{
				name:          "martial_ranged_weapon",
				equipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_RANGED_WEAPON,
				expectedType:  "martial-ranged-weapons",
			},
			{
				name:          "light_armor",
				equipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_LIGHT_ARMOR,
				expectedType:  "light-armor",
			},
			{
				name:          "medium_armor",
				equipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MEDIUM_ARMOR,
				expectedType:  "medium-armor",
			},
			{
				name:          "heavy_armor",
				equipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_HEAVY_ARMOR,
				expectedType:  "heavy-armor",
			},
			{
				name:          "shield",
				equipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SHIELD,
				expectedType:  "shields",
			},
			{
				name:          "adventuring_gear",
				equipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_ADVENTURING_GEAR,
				expectedType:  "adventuring-gear",
			},
		}

		for _, tc := range testCases {
			s.Run(tc.name, func() {
				req := &dnd5ev1alpha1.ListEquipmentByTypeRequest{
					EquipmentType: tc.equipmentType,
					PageSize:      10,
					PageToken:     "",
				}

				s.mockCharService.EXPECT().
					ListEquipmentByType(s.ctx, &character.ListEquipmentByTypeInput{
						EquipmentType: tc.expectedType,
						PageSize:      10,
						PageToken:     "",
					}).
					Return(&character.ListEquipmentByTypeOutput{
						Equipment:     []*dnd5e.EquipmentInfo{},
						NextPageToken: "",
						TotalSize:     0,
					}, nil)

				resp, err := s.handler.ListEquipmentByType(s.ctx, req)

				s.NoError(err)
				s.NotNil(resp)
				s.Empty(resp.Equipment)
			})
		}
	})

	s.Run("with missing equipment type", func() {
		req := &dnd5ev1alpha1.ListEquipmentByTypeRequest{
			EquipmentType: dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_UNSPECIFIED,
			PageSize:      20,
		}

		resp, err := s.handler.ListEquipmentByType(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
		s.Contains(st.Message(), "equipment_type is required")
	})

	s.Run("when service returns error", func() {
		s.mockCharService.EXPECT().
			ListEquipmentByType(s.ctx, gomock.Any()).
			Return(nil, errors.New("database error"))

		resp, err := s.handler.ListEquipmentByType(s.ctx, s.validListEquipmentByTypeReq)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.Internal, st.Code())
		s.Contains(st.Message(), "database error")
	})
}

func (s *HandlerTestSuite) TestListSpellsByLevel() {
	s.Run("with valid spell level", func() {
		expectedSpells := []*dnd5e.SpellInfo{
			{
				ID:          "fire-bolt",
				Name:        "Fire Bolt",
				Level:       0,
				School:      "Evocation",
				CastingTime: "1 action",
				Range:       "120 feet",
				Components:  []string{"V", "S"},
				Duration:    "Instantaneous",
				Description: "A flaming bolt of energy",
				Classes:     []string{"sorcerer", "wizard"},
			},
			{
				ID:          "prestidigitation",
				Name:        "Prestidigitation",
				Level:       0,
				School:      "Transmutation",
				CastingTime: "1 action",
				Range:       "10 feet",
				Components:  []string{"V", "S"},
				Duration:    "Up to 1 hour",
				Description: "Minor magical effects",
				Classes:     []string{"bard", "sorcerer", "warlock", "wizard"},
			},
		}

		s.mockCharService.EXPECT().
			ListSpellsByLevel(s.ctx, &character.ListSpellsByLevelInput{
				Level:     0,
				ClassID:   "CLASS_WIZARD",
				PageSize:  20,
				PageToken: "",
			}).
			Return(&character.ListSpellsByLevelOutput{
				Spells:        expectedSpells,
				NextPageToken: "next-token",
				TotalSize:     2,
			}, nil)

		resp, err := s.handler.ListSpellsByLevel(s.ctx, s.validListSpellsByLevelReq)

		s.NoError(err)
		s.NotNil(resp)
		s.Len(resp.Spells, 2)
		s.Equal("next-token", resp.NextPageToken)
		s.Equal(int32(2), resp.TotalSize)

		// Check first spell conversion
		spell := resp.Spells[0]
		s.Equal("fire-bolt", spell.Id)
		s.Equal("Fire Bolt", spell.Name)
		s.Equal(int32(0), spell.Level)
		s.Equal("Evocation", spell.School)
		s.Equal("1 action", spell.CastingTime)
		s.Equal("120 feet", spell.Range)
		s.Equal("V, S", spell.Components) // Components are joined as string in proto
		s.Equal("Instantaneous", spell.Duration)
		s.Equal("A flaming bolt of energy", spell.Description)
		s.Equal([]string{"sorcerer", "wizard"}, spell.Classes)
	})

	s.Run("with different spell levels", func() {
		testCases := []int32{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

		for _, level := range testCases {
			s.Run(fmt.Sprintf("level_%d", level), func() {
				req := &dnd5ev1alpha1.ListSpellsByLevelRequest{
					Level:     level,
					Class:     dnd5ev1alpha1.Class_CLASS_WIZARD,
					PageSize:  10,
					PageToken: "",
				}

				s.mockCharService.EXPECT().
					ListSpellsByLevel(s.ctx, &character.ListSpellsByLevelInput{
						Level:     level,
						ClassID:   "CLASS_WIZARD",
						PageSize:  10,
						PageToken: "",
					}).
					Return(&character.ListSpellsByLevelOutput{
						Spells:        []*dnd5e.SpellInfo{},
						NextPageToken: "",
						TotalSize:     0,
					}, nil)

				resp, err := s.handler.ListSpellsByLevel(s.ctx, req)

				s.NoError(err)
				s.NotNil(resp)
				s.Empty(resp.Spells)
			})
		}
	})

	s.Run("without class filter", func() {
		req := &dnd5ev1alpha1.ListSpellsByLevelRequest{
			Level:     1,
			PageSize:  20,
			PageToken: "",
		}

		s.mockCharService.EXPECT().
			ListSpellsByLevel(s.ctx, &character.ListSpellsByLevelInput{
				Level:     1,
				ClassID:   "",
				PageSize:  20,
				PageToken: "",
			}).
			Return(&character.ListSpellsByLevelOutput{
				Spells:        []*dnd5e.SpellInfo{},
				NextPageToken: "",
				TotalSize:     0,
			}, nil)

		resp, err := s.handler.ListSpellsByLevel(s.ctx, req)

		s.NoError(err)
		s.NotNil(resp)
		s.Empty(resp.Spells)
	})

	s.Run("with invalid spell level", func() {
		req := &dnd5ev1alpha1.ListSpellsByLevelRequest{
			Level:    -1,
			PageSize: 20,
		}

		resp, err := s.handler.ListSpellsByLevel(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
		s.Contains(st.Message(), "level must be between 0 and 9")
	})

	s.Run("with spell level too high", func() {
		req := &dnd5ev1alpha1.ListSpellsByLevelRequest{
			Level:    10,
			PageSize: 20,
		}

		resp, err := s.handler.ListSpellsByLevel(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
		s.Contains(st.Message(), "level must be between 0 and 9")
	})

	s.Run("when service returns error", func() {
		s.mockCharService.EXPECT().
			ListSpellsByLevel(s.ctx, gomock.Any()).
			Return(nil, errors.New("database error"))

		resp, err := s.handler.ListSpellsByLevel(s.ctx, s.validListSpellsByLevelReq)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.Internal, st.Code())
		s.Contains(st.Message(), "database error")
	})
}

// Equipment management tests

func (s *HandlerTestSuite) TestGetCharacterInventory() {
	s.Run("with valid character_id", func() {
		expectedEquipment := &dnd5e.EquipmentSlots{
			MainHand: &dnd5e.InventoryItem{
				ItemID:   s.testItemID,
				Quantity: 1,
				Equipment: &dnd5e.EquipmentData{
					ID:   s.testItemID,
					Name: "Longsword",
					Type: "weapon",
				},
			},
		}
		expectedInventory := []dnd5e.InventoryItem{
			{
				ItemID:   "item-potion",
				Quantity: 3,
				Equipment: &dnd5e.EquipmentData{
					ID:        "item-potion",
					Name:      "Healing Potion",
					Type:      "potion",
					Stackable: true,
				},
			},
		}
		expectedEncumbrance := &dnd5e.EncumbranceInfo{
			CurrentWeight:    150,
			CarryingCapacity: 300,
			MaxCapacity:      600,
			Level:            dnd5e.EncumbranceLevelUnencumbered,
		}

		s.mockCharService.EXPECT().
			GetInventory(s.ctx, &character.GetInventoryInput{
				CharacterID: s.testCharacterID,
			}).
			Return(&character.GetInventoryOutput{
				EquipmentSlots:      expectedEquipment,
				Inventory:           expectedInventory,
				Encumbrance:         expectedEncumbrance,
				AttunementSlotsUsed: 1,
				AttunementSlotsMax:  3,
			}, nil)

		resp, err := s.handler.GetCharacterInventory(s.ctx, s.validGetInventoryReq)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.EquipmentSlots)
		s.NotNil(resp.EquipmentSlots.MainHand)
		s.Equal(s.testItemID, resp.EquipmentSlots.MainHand.ItemId)
		s.Len(resp.Inventory, 1)
		s.Equal("item-potion", resp.Inventory[0].ItemId)
		s.Equal(int32(3), resp.Inventory[0].Quantity)
		s.NotNil(resp.Encumbrance)
		s.Equal(int32(1), resp.AttunementSlotsUsed)
		s.Equal(int32(3), resp.AttunementSlotsMax)
	})

	s.Run("with missing character_id", func() {
		req := &dnd5ev1alpha1.GetCharacterInventoryRequest{}

		resp, err := s.handler.GetCharacterInventory(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

func (s *HandlerTestSuite) TestEquipItem() {
	s.Run("successful equip", func() {
		expectedCharacter := &dnd5e.Character{
			ID:       s.testCharacterID,
			Name:     "Test Character",
			PlayerID: s.testPlayerID,
			EquipmentSlots: &dnd5e.EquipmentSlots{
				MainHand: &dnd5e.InventoryItem{
					ItemID:   s.testItemID,
					Quantity: 1,
				},
			},
		}

		s.mockCharService.EXPECT().
			EquipItem(s.ctx, &character.EquipItemInput{
				CharacterID: s.testCharacterID,
				ItemID:      s.testItemID,
				Slot:        "main_hand",
			}).
			Return(&character.EquipItemOutput{
				Success:   true,
				Character: expectedCharacter,
			}, nil)

		resp, err := s.handler.EquipItem(s.ctx, s.validEquipItemReq)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Character)
		s.NotNil(resp.Character.EquipmentSlots)
		s.NotNil(resp.Character.EquipmentSlots.MainHand)
	})

	s.Run("with missing character_id", func() {
		req := &dnd5ev1alpha1.EquipItemRequest{
			ItemId: s.testItemID,
			Slot:   dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
		}

		resp, err := s.handler.EquipItem(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

func (s *HandlerTestSuite) TestUnequipItem() {
	s.Run("successful unequip", func() {
		expectedCharacter := &dnd5e.Character{
			ID:             s.testCharacterID,
			Name:           "Test Character",
			PlayerID:       s.testPlayerID,
			EquipmentSlots: &dnd5e.EquipmentSlots{
				// MainHand is now empty
			},
		}

		s.mockCharService.EXPECT().
			UnequipItem(s.ctx, &character.UnequipItemInput{
				CharacterID: s.testCharacterID,
				Slot:        "main_hand",
			}).
			Return(&character.UnequipItemOutput{
				Success:   true,
				Character: expectedCharacter,
			}, nil)

		resp, err := s.handler.UnequipItem(s.ctx, s.validUnequipItemReq)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Character)
	})

	s.Run("with missing character_id", func() {
		req := &dnd5ev1alpha1.UnequipItemRequest{
			Slot: dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
		}

		resp, err := s.handler.UnequipItem(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

func (s *HandlerTestSuite) TestAddToInventory() {
	s.Run("successful add single item", func() {
		expectedCharacter := &dnd5e.Character{
			ID:       s.testCharacterID,
			Name:     "Test Character",
			PlayerID: s.testPlayerID,
			Inventory: []dnd5e.InventoryItem{
				{
					ItemID:   s.testItemID,
					Quantity: 1,
				},
			},
		}

		s.mockCharService.EXPECT().
			AddToInventory(s.ctx, &character.AddToInventoryInput{
				CharacterID: s.testCharacterID,
				Items: []character.InventoryAddition{
					{
						Item: &dnd5e.InventoryItem{
							ItemID:   s.testItemID,
							Quantity: 1,
						},
						Source: "api",
					},
				},
			}).
			Return(&character.AddToInventoryOutput{
				Success:   true,
				Character: expectedCharacter,
				Errors:    []string{},
			}, nil)

		resp, err := s.handler.AddToInventory(s.ctx, s.validAddToInventoryReq)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Character)
		s.Len(resp.Character.Inventory, 1)
		s.Empty(resp.Errors)
	})

	s.Run("with missing character_id", func() {
		req := &dnd5ev1alpha1.AddToInventoryRequest{
			Items: []*dnd5ev1alpha1.InventoryAddition{
				{ItemId: s.testItemID, Quantity: 1},
			},
		}

		resp, err := s.handler.AddToInventory(s.ctx, req)

		s.Error(err)
		s.Nil(resp)
		st, ok := status.FromError(err)
		s.True(ok)
		s.Equal(codes.InvalidArgument, st.Code())
	})
}

func (s *HandlerTestSuite) TestRemoveFromInventory() {
	s.Run("successful remove with quantity", func() {
		expectedCharacter := &dnd5e.Character{
			ID:       s.testCharacterID,
			Name:     "Test Character",
			PlayerID: s.testPlayerID,
			Inventory: []dnd5e.InventoryItem{
				{
					ItemID:   s.testItemID,
					Quantity: 4, // Had 5, removed 1
				},
			},
		}

		s.mockCharService.EXPECT().
			RemoveFromInventory(s.ctx, &character.RemoveFromInventoryInput{
				CharacterID: s.testCharacterID,
				ItemID:      s.testItemID,
				Quantity:    1,
				RemoveAll:   false,
			}).
			Return(&character.RemoveFromInventoryOutput{
				Success:         true,
				Character:       expectedCharacter,
				QuantityRemoved: 1,
			}, nil)

		resp, err := s.handler.RemoveFromInventory(s.ctx, s.validRemoveFromInventoryReq)

		s.NoError(err)
		s.NotNil(resp)
		s.NotNil(resp.Character)
		s.Equal(int32(1), resp.QuantityRemoved)
	})

	s.Run("with missing character_id", func() {
		req := &dnd5ev1alpha1.RemoveFromInventoryRequest{
			ItemId: s.testItemID,
			RemovalAmount: &dnd5ev1alpha1.RemoveFromInventoryRequest_Quantity{
				Quantity: 1,
			},
		}

		resp, err := s.handler.RemoveFromInventory(s.ctx, req)

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
