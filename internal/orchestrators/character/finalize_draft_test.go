package character_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	extmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	charrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charmock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

type FinalizeDraftOrchestratorTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	orchestrator    *character.Orchestrator
	mockCharRepo    *charmock.MockRepository
	mockDraftRepo   *draftmock.MockRepository
	mockExtClient   *extmock.MockClient
	mockDiceService *dicemock.MockService
	mockIDGen       *idgenmock.MockGenerator
	ctx             context.Context
}

func (s *FinalizeDraftOrchestratorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charmock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftmock.NewMockRepository(s.ctrl)
	s.mockExtClient = extmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)
	s.mockIDGen = idgenmock.NewMockGenerator(s.ctrl)
	s.ctx = context.Background()

	// Create orchestrator
	cfg := &character.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		ExternalClient:     s.mockExtClient,
		DiceService:        s.mockDiceService,
		IDGenerator:        s.mockIDGen,
	}
	orch, err := character.New(cfg)
	s.Require().NoError(err)
	s.orchestrator = orch
}

func (s *FinalizeDraftOrchestratorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_Success() {
	// Arrange
	draftID := "draft_123"

	// Mock ID generation
	s.mockIDGen.EXPECT().Generate().Return("char-123")

	completeDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Test Fighter",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: constants.RaceHuman,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassFighter,
		},
		BackgroundChoice: constants.BackgroundSoldier,
		AbilityScoreChoice: shared.AbilityScores{
			constants.STR: 16,
			constants.DEX: 14,
			constants.CON: 15,
			constants.INT: 10,
			constants.WIS: 12,
			constants.CHA: 8,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category:       shared.ChoiceSkills,
				Source:         shared.SourceClass,
				ChoiceID:       "fighter_skills",
				SkillSelection: []constants.Skill{constants.SkillAthletics, constants.SkillIntimidation},
			},
			{
				Category:          shared.ChoiceLanguages,
				Source:            shared.SourceRace,
				ChoiceID:          "human_languages",
				LanguageSelection: []constants.Language{constants.LanguageElvish},
			},
		},
	}

	// Mock the Get call
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: completeDraft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(constants.RaceHuman)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        constants.RaceHuman,
				Name:      "Human",
				Speed:     30,
				Size:      "Medium",
				Languages: []constants.Language{constants.LanguageCommon},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(constants.ClassFighter)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:                  constants.ClassFighter,
				Name:                "Fighter",
				HitDice:             10,
				SavingThrows:        []constants.Ability{constants.STR, constants.CON},
				WeaponProficiencies: []string{"simple", "martial"},
				ArmorProficiencies:  []string{"light", "medium", "heavy", "shields"},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(constants.BackgroundSoldier)).
		Return(&external.BackgroundData{
			ID:                 "soldier",
			Name:               "Soldier",
			SkillProficiencies: []string{"Athletics", "Intimidation"},
			Equipment:          []string{"Uniform", "Javelin"},
			Feature:            "Military Rank: You have military authority.",
		}, nil)

	// Mock character creation
	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Verify the character data
			s.Equal("char-123", input.CharacterData.ID)
			s.Equal("player_123", input.CharacterData.PlayerID)
			s.Equal("Test Fighter", input.CharacterData.Name)
			s.Equal(constants.RaceHuman, input.CharacterData.RaceID)
			s.Equal(constants.ClassFighter, input.CharacterData.ClassID)
			s.Equal(constants.BackgroundSoldier, input.CharacterData.BackgroundID)
			s.Equal(1, input.CharacterData.Level)

			// Hit points: 10 (max d10) + 2 (CON mod) = 12
			s.Equal(12, input.CharacterData.HitPoints)
			s.Equal(12, input.CharacterData.MaxHitPoints)

			// Speed from race
			s.Equal(30, input.CharacterData.Speed)
			s.Equal("Medium", input.CharacterData.Size)

			// Saving throws
			s.Equal(shared.Proficient, input.CharacterData.SavingThrows[constants.STR])
			s.Equal(shared.Proficient, input.CharacterData.SavingThrows[constants.CON])

			// Skills (both background and class-derived)
			s.Equal(shared.Proficient, input.CharacterData.Skills[constants.SkillAthletics])    // From background
			s.Equal(shared.Proficient, input.CharacterData.Skills[constants.SkillIntimidation]) // From background

			// Languages
			s.Contains(input.CharacterData.Languages, string(constants.LanguageCommon))
			s.Contains(input.CharacterData.Languages, string(constants.LanguageElvish))

			// Equipment (from background)
			s.Contains(input.CharacterData.Equipment, "Uniform")
			s.Contains(input.CharacterData.Equipment, "Javelin")

			// Class resources (Fighter should get Second Wind)
			s.Contains(input.CharacterData.ClassResources, "second_wind")
			resource := input.CharacterData.ClassResources["second_wind"]
			s.Equal("Second Wind", resource.Name)
			s.Equal(1, resource.Max)
			s.Equal(1, resource.Current)
			s.Equal("short_rest", resource.Resets)

			// Spell slots (Fighter should not have any)
			s.Empty(input.CharacterData.SpellSlots)

			return &charrepo.CreateOutput{CharacterData: input.CharacterData}, nil
		})

	// Mock draft deletion
	s.mockDraftRepo.EXPECT().
		Delete(gomock.Any(), draftrepo.DeleteInput{ID: draftID}).
		Return(&draftrepo.DeleteOutput{}, nil)

	// Act
	input := &character.FinalizeDraftInput{
		DraftID: draftID,
	}
	output, err := s.orchestrator.FinalizeDraft(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.Character)
	s.Equal("char-123", output.Character.ID)
	s.Equal("Test Fighter", output.Character.Name)
	s.True(output.DraftDeleted)
}

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_IncompleteDraft() {
	testCases := []struct {
		name          string
		draft         *toolkitchar.DraftData
		expectedError string
	}{
		{
			name: "Missing name",
			draft: &toolkitchar.DraftData{
				ID:       "draft_123",
				PlayerID: "player_123",
				RaceChoice: toolkitchar.RaceChoice{
					RaceID: constants.RaceHuman,
				},
				ClassChoice: toolkitchar.ClassChoice{
					ClassID: constants.ClassFighter,
				},
				BackgroundChoice: constants.BackgroundSoldier,
				AbilityScoreChoice: shared.AbilityScores{
					constants.STR: 16,
				},
			},
			expectedError: "draft is incomplete: name is required",
		},
		{
			name: "Missing race",
			draft: &toolkitchar.DraftData{
				ID:               "draft_123",
				PlayerID:         "player_123",
				Name:             "Test Character",
				BackgroundChoice: constants.BackgroundSoldier,
				ClassChoice: toolkitchar.ClassChoice{
					ClassID: constants.ClassFighter,
				},
				AbilityScoreChoice: shared.AbilityScores{
					constants.STR: 16,
				},
			},
			expectedError: "draft is incomplete: race is required",
		},
		{
			name: "Missing class",
			draft: &toolkitchar.DraftData{
				ID:       "draft_123",
				PlayerID: "player_123",
				Name:     "Test Character",
				RaceChoice: toolkitchar.RaceChoice{
					RaceID: constants.RaceHuman,
				},
				BackgroundChoice: constants.BackgroundSoldier,
				AbilityScoreChoice: shared.AbilityScores{
					constants.STR: 16,
				},
			},
			expectedError: "draft is incomplete: class is required",
		},
		{
			name: "Missing background",
			draft: &toolkitchar.DraftData{
				ID:       "draft_123",
				PlayerID: "player_123",
				Name:     "Test Character",
				RaceChoice: toolkitchar.RaceChoice{
					RaceID: constants.RaceHuman,
				},
				ClassChoice: toolkitchar.ClassChoice{
					ClassID: constants.ClassFighter,
				},
				AbilityScoreChoice: shared.AbilityScores{
					constants.STR: 16,
				},
			},
			expectedError: "draft is incomplete: background is required",
		},
		{
			name: "Missing ability scores",
			draft: &toolkitchar.DraftData{
				ID:       "draft_123",
				PlayerID: "player_123",
				Name:     "Test Character",
				RaceChoice: toolkitchar.RaceChoice{
					RaceID: constants.RaceHuman,
				},
				ClassChoice: toolkitchar.ClassChoice{
					ClassID: constants.ClassFighter,
				},
				BackgroundChoice: constants.BackgroundSoldier,
			},
			expectedError: "draft is incomplete: ability scores are required",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Mock the Get call
			s.mockDraftRepo.EXPECT().
				Get(gomock.Any(), draftrepo.GetInput{ID: tc.draft.ID}).
				Return(&draftrepo.GetOutput{Draft: tc.draft}, nil)

			// Act
			input := &character.FinalizeDraftInput{
				DraftID: tc.draft.ID,
			}
			output, err := s.orchestrator.FinalizeDraft(s.ctx, input)

			// Assert
			s.Require().Error(err)
			s.Nil(output)
			s.True(errors.IsInvalidArgument(err))
			s.Contains(err.Error(), tc.expectedError)
		})
	}
}

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_DraftNotFound() {
	// Arrange
	draftID := "non_existent"

	// Mock the Get call to return not found
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(nil, errors.NotFound("draft not found"))

	// Act
	input := &character.FinalizeDraftInput{
		DraftID: draftID,
	}
	output, err := s.orchestrator.FinalizeDraft(s.ctx, input)

	// Assert
	s.Require().Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "failed to get draft")
}

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_DraftDeleteFails() {
	// Arrange
	draftID := "draft_123"

	// Mock ID generation
	s.mockIDGen.EXPECT().Generate().Return("char-123")

	completeDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Test Fighter",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: constants.RaceHuman,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassFighter,
		},
		BackgroundChoice: constants.BackgroundSoldier,
		AbilityScoreChoice: shared.AbilityScores{
			constants.STR: 16,
			constants.DEX: 14,
			constants.CON: 15,
			constants.INT: 10,
			constants.WIS: 12,
			constants.CHA: 8,
		},
	}

	// Mock the Get call
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: completeDraft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(constants.RaceHuman)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:    constants.RaceHuman,
				Name:  "Human",
				Speed: 30,
				Size:  "Medium",
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(constants.ClassFighter)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           constants.ClassFighter,
				Name:         "Fighter",
				HitDice:      10,
				SavingThrows: []constants.Ability{constants.STR, constants.CON},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(constants.BackgroundSoldier)).
		Return(&external.BackgroundData{
			ID:                 "soldier",
			Name:               "Soldier",
			SkillProficiencies: []string{"Athletics", "Intimidation"},
			Equipment:          []string{"Uniform", "Javelin"},
			Feature:            "Military Rank: You have military authority.",
		}, nil)

	// Mock character creation
	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(&charrepo.CreateOutput{
			CharacterData: &toolkitchar.Data{
				ID:   "char-123",
				Name: "Test Fighter",
			},
		}, nil)

	// Mock draft deletion failure
	s.mockDraftRepo.EXPECT().
		Delete(gomock.Any(), draftrepo.DeleteInput{ID: draftID}).
		Return(nil, errors.Internal("failed to delete"))

	// Act
	input := &character.FinalizeDraftInput{
		DraftID: draftID,
	}
	output, err := s.orchestrator.FinalizeDraft(s.ctx, input)

	// Assert - should still succeed but flag draft not deleted
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.Character)
	s.Equal("char-123", output.Character.ID)
	s.False(output.DraftDeleted) // Draft deletion failed
}

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_BarbarianClassResources() {
	draftID := "draft_123"

	// Mock ID generation
	s.mockIDGen.EXPECT().Generate().Return("char-123")

	// Mock draft data for Barbarian
	draft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Test Barbarian",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: constants.RaceHuman,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassBarbarian,
		},
		BackgroundChoice: constants.BackgroundSoldier,
		AbilityScoreChoice: shared.AbilityScores{
			constants.STR: 16,
			constants.CON: 14,
			constants.CHA: 10,
		},
	}

	// Mock draft retrieval
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(constants.RaceHuman)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        constants.RaceHuman,
				Name:      "Human",
				Speed:     30,
				Size:      "Medium",
				Languages: []constants.Language{constants.LanguageCommon},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(constants.ClassBarbarian)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           constants.ClassBarbarian,
				Name:         "Barbarian",
				HitDice:      12,
				SavingThrows: []constants.Ability{constants.STR, constants.CON},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(constants.BackgroundSoldier)).
		Return(&external.BackgroundData{
			ID:                 "soldier",
			Name:               "Soldier",
			SkillProficiencies: []string{"Athletics", "Intimidation"},
		}, nil)

	// Mock character creation to verify Barbarian gets Rage
	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Verify class resources (Barbarian should get Rage)
			s.Contains(input.CharacterData.ClassResources, "rage")
			resource := input.CharacterData.ClassResources["rage"]
			s.Equal("Rage", resource.Name)
			s.Equal(2, resource.Max) // 2 rages at level 1
			s.Equal(2, resource.Current)
			s.Equal("long_rest", resource.Resets)

			// Spell slots (Barbarian should not have any)
			s.Empty(input.CharacterData.SpellSlots)

			return &charrepo.CreateOutput{CharacterData: input.CharacterData}, nil
		})

	// Mock draft deletion
	s.mockDraftRepo.EXPECT().
		Delete(gomock.Any(), draftrepo.DeleteInput{ID: draftID}).
		Return(&draftrepo.DeleteOutput{}, nil)

	// Act
	input := &character.FinalizeDraftInput{
		DraftID: draftID,
	}
	output, err := s.orchestrator.FinalizeDraft(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.True(output.DraftDeleted)
}

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_WizardSpellSlots() {
	draftID := "draft_123"

	// Mock ID generation
	s.mockIDGen.EXPECT().Generate().Return("char-123")

	// Mock draft data for Wizard
	draft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Test Wizard",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: constants.RaceHuman,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassWizard,
		},
		BackgroundChoice: constants.BackgroundSoldier,
		AbilityScoreChoice: shared.AbilityScores{
			constants.STR: 10,
			constants.CON: 14,
			constants.INT: 16,
		},
	}

	// Mock draft retrieval
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(constants.RaceHuman)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        constants.RaceHuman,
				Name:      "Human",
				Speed:     30,
				Size:      "Medium",
				Languages: []constants.Language{constants.LanguageCommon},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(constants.ClassWizard)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           constants.ClassWizard,
				Name:         "Wizard",
				HitDice:      6,
				SavingThrows: []constants.Ability{constants.INT, constants.WIS},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(constants.BackgroundSoldier)).
		Return(&external.BackgroundData{
			ID:                 "soldier",
			Name:               "Soldier",
			SkillProficiencies: []string{"Athletics", "Intimidation"},
		}, nil)

	// Mock character creation to verify Wizard gets spell slots
	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Class resources (Wizard has no class resources at level 1)
			s.Empty(input.CharacterData.ClassResources)

			// Spell slots (Wizard should get 2 first-level slots)
			s.Contains(input.CharacterData.SpellSlots, 1)
			slots := input.CharacterData.SpellSlots[1]
			s.Equal(2, slots.Max)  // 2 first-level slots at level 1
			s.Equal(0, slots.Used) // Unused initially

			return &charrepo.CreateOutput{CharacterData: input.CharacterData}, nil
		})

	// Mock draft deletion
	s.mockDraftRepo.EXPECT().
		Delete(gomock.Any(), draftrepo.DeleteInput{ID: draftID}).
		Return(&draftrepo.DeleteOutput{}, nil)

	// Act
	input := &character.FinalizeDraftInput{
		DraftID: draftID,
	}
	output, err := s.orchestrator.FinalizeDraft(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.True(output.DraftDeleted)
}

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_BardCharismaBasedResource() {
	draftID := "draft_123"

	// Mock ID generation
	s.mockIDGen.EXPECT().Generate().Return("char-123")

	// Mock draft data for Bard with high CHA
	draft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Test Bard",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: constants.RaceHuman,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassBard,
		},
		BackgroundChoice: constants.BackgroundSoldier,
		AbilityScoreChoice: shared.AbilityScores{
			constants.STR: 10,
			constants.CON: 14,
			constants.CHA: 16, // +3 modifier = 3 uses
		},
	}

	// Mock draft retrieval
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(constants.RaceHuman)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        constants.RaceHuman,
				Name:      "Human",
				Speed:     30,
				Size:      "Medium",
				Languages: []constants.Language{constants.LanguageCommon},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(constants.ClassBard)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           constants.ClassBard,
				Name:         "Bard",
				HitDice:      8,
				SavingThrows: []constants.Ability{constants.DEX, constants.CHA},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(constants.BackgroundSoldier)).
		Return(&external.BackgroundData{
			ID:                 "soldier",
			Name:               "Soldier",
			SkillProficiencies: []string{"Athletics", "Intimidation"},
		}, nil)

	// Mock character creation to verify Bard gets Bardic Inspiration and spell slots
	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Class resources (Bard should get Bardic Inspiration)
			s.Contains(input.CharacterData.ClassResources, "bardic_inspiration")
			resource := input.CharacterData.ClassResources["bardic_inspiration"]
			s.Equal("Bardic Inspiration", resource.Name)
			s.Equal(3, resource.Max) // CHA modifier (16 = +3)
			s.Equal(3, resource.Current)
			s.Equal("long_rest", resource.Resets)

			// Spell slots (Bard should get 2 first-level slots)
			s.Contains(input.CharacterData.SpellSlots, 1)
			slots := input.CharacterData.SpellSlots[1]
			s.Equal(2, slots.Max)
			s.Equal(0, slots.Used)

			return &charrepo.CreateOutput{CharacterData: input.CharacterData}, nil
		})

	// Mock draft deletion
	s.mockDraftRepo.EXPECT().
		Delete(gomock.Any(), draftrepo.DeleteInput{ID: draftID}).
		Return(&draftrepo.DeleteOutput{}, nil)

	// Act
	input := &character.FinalizeDraftInput{
		DraftID: draftID,
	}
	output, err := s.orchestrator.FinalizeDraft(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.True(output.DraftDeleted)
}

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_WarlockPactMagic() {
	draftID := "draft_123"

	// Mock ID generation
	s.mockIDGen.EXPECT().Generate().Return("char-123")

	// Mock draft data for Warlock
	draft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Test Warlock",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: constants.RaceHuman,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassWarlock,
		},
		BackgroundChoice: constants.BackgroundSoldier,
		AbilityScoreChoice: shared.AbilityScores{
			constants.STR: 10,
			constants.CON: 14,
			constants.CHA: 16,
		},
	}

	// Mock draft retrieval
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(constants.RaceHuman)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        constants.RaceHuman,
				Name:      "Human",
				Speed:     30,
				Size:      "Medium",
				Languages: []constants.Language{constants.LanguageCommon},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(constants.ClassWarlock)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           constants.ClassWarlock,
				Name:         "Warlock",
				HitDice:      8,
				SavingThrows: []constants.Ability{constants.WIS, constants.CHA},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(constants.BackgroundSoldier)).
		Return(&external.BackgroundData{
			ID:                 "soldier",
			Name:               "Soldier",
			SkillProficiencies: []string{"Athletics", "Intimidation"},
		}, nil)

	// Mock character creation to verify Warlock gets different spell slot pattern
	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Class resources (Warlock has no class resources at level 1)
			s.Empty(input.CharacterData.ClassResources)

			// Spell slots (Warlock should get 1 first-level slot due to Pact Magic)
			s.Contains(input.CharacterData.SpellSlots, 1)
			slots := input.CharacterData.SpellSlots[1]
			s.Equal(1, slots.Max) // 1 slot for Pact Magic
			s.Equal(0, slots.Used)

			return &charrepo.CreateOutput{CharacterData: input.CharacterData}, nil
		})

	// Mock draft deletion
	s.mockDraftRepo.EXPECT().
		Delete(gomock.Any(), draftrepo.DeleteInput{ID: draftID}).
		Return(&draftrepo.DeleteOutput{}, nil)

	// Act
	input := &character.FinalizeDraftInput{
		DraftID: draftID,
	}
	output, err := s.orchestrator.FinalizeDraft(s.ctx, input)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.True(output.DraftDeleted)
}

func TestFinalizeDraftOrchestratorTestSuite(t *testing.T) {
	suite.Run(t, new(FinalizeDraftOrchestratorTestSuite))
}
