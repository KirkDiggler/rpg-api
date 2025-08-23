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
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/languages"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
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
	mockDraftIDGen  *idgenmock.MockGenerator
	ctx             context.Context
}

func (s *FinalizeDraftOrchestratorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charmock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftmock.NewMockRepository(s.ctrl)
	s.mockExtClient = extmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)
	s.mockIDGen = idgenmock.NewMockGenerator(s.ctrl)
	s.mockDraftIDGen = idgenmock.NewMockGenerator(s.ctrl)
	s.ctx = context.Background()

	// Create orchestrator
	cfg := &character.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		ExternalClient:     s.mockExtClient,
		DiceService:        s.mockDiceService,
		IDGenerator:        s.mockIDGen,
		DraftIDGenerator:   s.mockDraftIDGen,
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
			RaceID: races.Human,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Fighter,
		},
		BackgroundChoice: backgrounds.Soldier,
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 14,
			abilities.CON: 15,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category:       shared.ChoiceSkills,
				Source:         shared.SourceClass,
				ChoiceID:       "fighter_skills",
				SkillSelection: []skills.Skill{skills.Athletics, skills.Intimidation},
			},
			{
				Category:          shared.ChoiceLanguages,
				Source:            shared.SourceRace,
				ChoiceID:          "human_languages",
				LanguageSelection: []languages.Language{languages.Elvish},
			},
		},
	}

	// Mock the Get call
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: completeDraft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(races.Human)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        races.Human,
				Name:      "Human",
				Speed:     30,
				Size:      "Medium",
				Languages: []languages.Language{languages.Common},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(classes.Fighter)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:                  classes.Fighter,
				Name:                "Fighter",
				HitDice:             10,
				SavingThrows:        []abilities.Ability{abilities.STR, abilities.CON},
				WeaponProficiencies: []string{"simple", "martial"},
				ArmorProficiencies:  []string{"light", "medium", "heavy", "shields"},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(backgrounds.Soldier)).
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
			s.Equal(races.Human, input.CharacterData.RaceID)
			s.Equal(classes.Fighter, input.CharacterData.ClassID)
			s.Equal(backgrounds.Soldier, input.CharacterData.BackgroundID)
			s.Equal(1, input.CharacterData.Level)

			// Hit points: 10 (max d10) + 2 (CON mod) = 12
			s.Equal(12, input.CharacterData.HitPoints)
			s.Equal(12, input.CharacterData.MaxHitPoints)

			// Speed from race
			s.Equal(30, input.CharacterData.Speed)
			s.Equal("Medium", input.CharacterData.Size)

			// Saving throws
			s.Equal(shared.Proficient, input.CharacterData.SavingThrows[abilities.STR])
			s.Equal(shared.Proficient, input.CharacterData.SavingThrows[abilities.CON])

			// Skills (both background and class-derived)
			s.Equal(shared.Proficient, input.CharacterData.Skills[skills.Athletics])    // From background
			s.Equal(shared.Proficient, input.CharacterData.Skills[skills.Intimidation]) // From background

			// Languages
			s.Contains(input.CharacterData.Languages, string(languages.Common))
			s.Contains(input.CharacterData.Languages, string(languages.Elvish))

			// Equipment (from background)
			s.Contains(input.CharacterData.Equipment, "Uniform")
			s.Contains(input.CharacterData.Equipment, "Javelin")

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

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_SuccessWithoutBackground() {
	// Test that finalization works without a background (optional)
	draftID := "draft_no_bg_123"

	// Mock ID generation
	s.mockIDGen.EXPECT().Generate().Return("char-no-bg-123")

	// Mock draft data WITHOUT background
	draft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Test Fighter No BG",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: races.Human,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Fighter,
		},
		// NO BackgroundChoice - it's optional now
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 12,
			abilities.CON: 14,
			abilities.INT: 10,
			abilities.WIS: 13,
			abilities.CHA: 8,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category:       shared.ChoiceSkills,
				Source:         shared.SourceClass,
				ChoiceID:       "fighter_skills",
				SkillSelection: []skills.Skill{skills.Athletics, skills.Intimidation},
			},
		},
	}

	// Mock the Get call
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(races.Human)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        races.Human,
				Name:      "Human",
				Speed:     30,
				Size:      "Medium",
				Languages: []languages.Language{languages.Common, languages.Elvish},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(classes.Fighter)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:                  classes.Fighter,
				Name:                "Fighter",
				HitDice:             10,
				SavingThrows:        []abilities.Ability{abilities.STR, abilities.CON},
				WeaponProficiencies: []string{"simple", "martial"},
				ArmorProficiencies:  []string{"light", "medium", "heavy", "shields"},
			},
		}, nil)

	// NO GetBackgroundData call since background is not provided

	// Mock character creation
	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Verify character is created without background data
			s.Equal("char-no-bg-123", input.CharacterData.ID)
			s.Equal("player_123", input.CharacterData.PlayerID)
			s.Equal("Test Fighter No BG", input.CharacterData.Name)

			// Verify HP calculation still works
			s.Equal(12, input.CharacterData.MaxHitPoints) // 10 (hit dice) + 2 (CON mod)

			// Skills should still be set from choices
			s.Equal(shared.Proficient, input.CharacterData.Skills[skills.Athletics])
			s.Equal(shared.Proficient, input.CharacterData.Skills[skills.Intimidation])

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
	s.Equal("char-no-bg-123", output.Character.ID)
	s.Equal("Test Fighter No BG", output.Character.Name)
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
					RaceID: races.Human,
				},
				ClassChoice: toolkitchar.ClassChoice{
					ClassID: classes.Fighter,
				},
				BackgroundChoice: backgrounds.Soldier,
				AbilityScoreChoice: shared.AbilityScores{
					abilities.STR: 16,
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
				BackgroundChoice: backgrounds.Soldier,
				ClassChoice: toolkitchar.ClassChoice{
					ClassID: classes.Fighter,
				},
				AbilityScoreChoice: shared.AbilityScores{
					abilities.STR: 16,
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
					RaceID: races.Human,
				},
				BackgroundChoice: backgrounds.Soldier,
				AbilityScoreChoice: shared.AbilityScores{
					abilities.STR: 16,
				},
			},
			expectedError: "draft is incomplete: class is required",
		},
		// Background is now optional - commenting out this test
		// {
		// 	name: "Missing background",
		// 	draft: &toolkitchar.DraftData{
		// 		ID:       "draft_123",
		// 		PlayerID: "player_123",
		// 		Name:     "Test Character",
		// 		RaceChoice: toolkitchar.RaceChoice{
		// 			RaceID: races.Human,
		// 		},
		// 		ClassChoice: toolkitchar.ClassChoice{
		// 			ClassID: classes.Fighter,
		// 		},
		// 		AbilityScoreChoice: shared.AbilityScores{
		// 			abilities.STR: 16,
		// 		},
		// 	},
		// 	expectedError: "draft is incomplete: background is required",
		// },
		{
			name: "Missing ability scores",
			draft: &toolkitchar.DraftData{
				ID:       "draft_123",
				PlayerID: "player_123",
				Name:     "Test Character",
				RaceChoice: toolkitchar.RaceChoice{
					RaceID: races.Human,
				},
				ClassChoice: toolkitchar.ClassChoice{
					ClassID: classes.Fighter,
				},
				BackgroundChoice: backgrounds.Soldier,
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
			RaceID: races.Human,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Fighter,
		},
		BackgroundChoice: backgrounds.Soldier,
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 14,
			abilities.CON: 15,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
	}

	// Mock the Get call
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: completeDraft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(races.Human)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:    races.Human,
				Name:  "Human",
				Speed: 30,
				Size:  "Medium",
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(classes.Fighter)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           classes.Fighter,
				Name:         "Fighter",
				HitDice:      10,
				SavingThrows: []abilities.Ability{abilities.STR, abilities.CON},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(backgrounds.Soldier)).
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

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_ElfWithRacialTraits() {
	// Arrange
	draftID := "draft_elf_123"

	// Mock ID generation
	s.mockIDGen.EXPECT().Generate().Return("char-elf-123")

	elfDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Test Elf Ranger",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: races.Elf,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Ranger,
		},
		BackgroundChoice: backgrounds.Outlander,
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 14,
			abilities.DEX: 16,
			abilities.CON: 14,
			abilities.INT: 12,
			abilities.WIS: 15,
			abilities.CHA: 10,
		},
	}

	// Mock the Get call
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: elfDraft}, nil)

	// Mock race data with racial skill proficiencies and traits
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(races.Elf)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:                  races.Elf,
				Name:                "Elf",
				Speed:               30,
				Size:                "Medium",
				Languages:           []languages.Language{languages.Common, languages.Elvish},
				SkillProficiencies:  []skills.Skill{skills.Perception},
				WeaponProficiencies: []string{"Longsword", "Shortbow"},
				Traits: []race.TraitData{
					{ID: "darkvision", Name: "Darkvision"},
					{ID: "keen-senses", Name: "Keen Senses"},
					{ID: "fey-ancestry", Name: "Fey Ancestry"},
					{ID: "trance", Name: "Trance"},
				},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(classes.Ranger)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:                  classes.Ranger,
				Name:                "Ranger",
				HitDice:             10,
				SavingThrows:        []abilities.Ability{abilities.STR, abilities.DEX},
				WeaponProficiencies: []string{"simple", "martial"},
				ArmorProficiencies:  []string{"light", "medium", "shields"},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(backgrounds.Outlander)).
		Return(&external.BackgroundData{
			ID:                 "outlander",
			Name:               "Outlander",
			SkillProficiencies: []string{"Athletics", "Survival"},
			Equipment:          []string{"Staff", "Hunting trap"},
		}, nil)

	// Mock character creation
	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Verify racial skill proficiencies are applied
			s.Equal(shared.Proficient, input.CharacterData.Skills[skills.Perception]) // From race
			s.Equal(shared.Proficient, input.CharacterData.Skills[skills.Athletics])  // From background
			s.Equal(shared.Proficient, input.CharacterData.Skills[skills.Survival])   // From background

			// Verify racial weapon proficiencies are added
			s.Contains(input.CharacterData.Proficiencies.Weapons, "Longsword")
			s.Contains(input.CharacterData.Proficiencies.Weapons, "Shortbow")
			s.Contains(input.CharacterData.Proficiencies.Weapons, "simple")  // From class
			s.Contains(input.CharacterData.Proficiencies.Weapons, "martial") // From class

			// Verify languages from race
			s.Contains(input.CharacterData.Languages, string(languages.Common))
			s.Contains(input.CharacterData.Languages, string(languages.Elvish))

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
	s.True(output.DraftDeleted)
}

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_HillDwarfWithHPBonus() {
	// Arrange
	draftID := "draft_dwarf_123"

	// Mock ID generation
	s.mockIDGen.EXPECT().Generate().Return("char-dwarf-123")

	dwarfDraft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Test Hill Dwarf Cleric",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID:    races.Dwarf,
			SubraceID: races.HillDwarf,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Cleric,
		},
		BackgroundChoice: backgrounds.Acolyte,
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 12,
			abilities.DEX: 10,
			abilities.CON: 16,
			abilities.INT: 13,
			abilities.WIS: 15,
			abilities.CHA: 14,
		},
	}

	// Mock the Get call
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: dwarfDraft}, nil)

	// Mock race data with tool proficiencies
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(races.Dwarf)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:                  races.Dwarf,
				Name:                "Dwarf",
				Speed:               25,
				Size:                "Medium",
				Languages:           []languages.Language{languages.Common, languages.Dwarvish},
				ToolProficiencies:   []string{"Smith's tools", "Brewer's supplies"},
				WeaponProficiencies: []string{"Battleaxe", "Handaxe", "Light hammer", "Warhammer"},
				Traits: []race.TraitData{
					{ID: "darkvision", Name: "Darkvision"},
					{ID: "dwarven-resilience", Name: "Dwarven Resilience"},
					{ID: "stonecunning", Name: "Stonecunning"},
				},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(classes.Cleric)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:                  classes.Cleric,
				Name:                "Cleric",
				HitDice:             8,
				SavingThrows:        []abilities.Ability{abilities.WIS, abilities.CHA},
				WeaponProficiencies: []string{"simple"},
				ArmorProficiencies:  []string{"light", "medium", "shields"},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(backgrounds.Acolyte)).
		Return(&external.BackgroundData{
			ID:                 "acolyte",
			Name:               "Acolyte",
			SkillProficiencies: []string{"Insight", "Religion"},
			Equipment:          []string{"Holy symbol", "Prayer book"},
		}, nil)

	// Mock character creation
	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Verify Hill Dwarf HP bonus (base 8 + CON mod 3 + Hill Dwarf bonus 1 = 12)
			expectedHP := 8 + 3 + 1 // Hit dice + CON modifier + Hill Dwarf bonus
			s.Equal(expectedHP, input.CharacterData.HitPoints)
			s.Equal(expectedHP, input.CharacterData.MaxHitPoints)

			// Verify racial tool proficiencies are added
			s.Contains(input.CharacterData.Proficiencies.Tools, "Smith's tools")
			s.Contains(input.CharacterData.Proficiencies.Tools, "Brewer's supplies")

			// Verify racial weapon proficiencies are added
			s.Contains(input.CharacterData.Proficiencies.Weapons, "Battleaxe")
			s.Contains(input.CharacterData.Proficiencies.Weapons, "Handaxe")
			s.Contains(input.CharacterData.Proficiencies.Weapons, "Light hammer")
			s.Contains(input.CharacterData.Proficiencies.Weapons, "Warhammer")
			s.Contains(input.CharacterData.Proficiencies.Weapons, "simple") // From class

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
	s.True(output.DraftDeleted)
}

func (s *FinalizeDraftOrchestratorTestSuite) TestFinalizeDraft_RacialSkillConflictWithBackground() {
	// Test case where race and background both provide the same skill proficiency
	draftID := "draft_conflict_123"

	// Mock ID generation
	s.mockIDGen.EXPECT().Generate().Return("char-conflict-123")

	draft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_123",
		Name:     "Test Elf Athlete",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: races.Elf,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Fighter,
		},
		BackgroundChoice: backgrounds.Soldier,
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 14,
			abilities.CON: 15,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category:       shared.ChoiceSkills,
				Source:         shared.SourceClass,
				ChoiceID:       "fighter_skills",
				SkillSelection: []skills.Skill{skills.Perception}, // Conflicts with racial Perception
			},
		},
	}

	// Mock the Get call
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock race data - Elf gets Perception
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(races.Elf)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:                 races.Elf,
				Name:               "Elf",
				Speed:              30,
				Size:               "Medium",
				Languages:          []languages.Language{languages.Common, languages.Elvish},
				SkillProficiencies: []skills.Skill{skills.Perception},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(classes.Fighter)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           classes.Fighter,
				Name:         "Fighter",
				HitDice:      10,
				SavingThrows: []abilities.Ability{abilities.STR, abilities.CON},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(backgrounds.Soldier)).
		Return(&external.BackgroundData{
			ID:                 "soldier",
			Name:               "Soldier",
			SkillProficiencies: []string{"Athletics", "Intimidation"},
			Equipment:          []string{"Uniform", "Javelin"},
		}, nil)

	// Mock character creation
	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Verify Perception is still proficient (should not be duplicated)
			s.Equal(shared.Proficient, input.CharacterData.Skills[skills.Perception])

			// Count how many times Perception appears - should only be once
			perceptionCount := 0
			for skill, level := range input.CharacterData.Skills {
				if skill == skills.Perception && level == shared.Proficient {
					perceptionCount++
				}
			}
			s.Equal(1, perceptionCount, "Perception should only be proficient once, not duplicated")

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
	s.True(output.DraftDeleted)
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
			RaceID: races.Human,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Barbarian,
		},
		BackgroundChoice: backgrounds.Soldier,
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 16,
			abilities.CON: 14,
			abilities.CHA: 10,
		},
	}

	// Mock draft retrieval
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(races.Human)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        races.Human,
				Name:      "Human",
				Speed:     30,
				Size:      "Medium",
				Languages: []languages.Language{languages.Common},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(classes.Barbarian)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           classes.Barbarian,
				Name:         "Barbarian",
				HitDice:      12,
				SavingThrows: []abilities.Ability{abilities.STR, abilities.CON},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(backgrounds.Soldier)).
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
			s.Contains(input.CharacterData.ClassResources, shared.ClassResourceRage)
			resource := input.CharacterData.ClassResources[shared.ClassResourceRage]
			s.Equal("Rage", resource.Name)
			s.Equal(2, resource.Max) // 2 rages at level 1
			s.Equal(2, resource.Current)
			s.Equal(shared.ResetTypeLongRest, resource.Resets)

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
			RaceID: races.Human,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Wizard,
		},
		BackgroundChoice: backgrounds.Soldier,
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 10,
			abilities.CON: 14,
			abilities.INT: 16,
		},
	}

	// Mock draft retrieval
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(races.Human)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        races.Human,
				Name:      "Human",
				Speed:     30,
				Size:      "Medium",
				Languages: []languages.Language{languages.Common},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(classes.Wizard)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           classes.Wizard,
				Name:         "Wizard",
				HitDice:      6,
				SavingThrows: []abilities.Ability{abilities.INT, abilities.WIS},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(backgrounds.Soldier)).
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
			RaceID: races.Human,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Bard,
		},
		BackgroundChoice: backgrounds.Soldier,
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 10,
			abilities.CON: 14,
			abilities.CHA: 16, // +3 modifier = 3 uses
		},
	}

	// Mock draft retrieval
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(races.Human)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        races.Human,
				Name:      "Human",
				Speed:     30,
				Size:      "Medium",
				Languages: []languages.Language{languages.Common},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(classes.Bard)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           classes.Bard,
				Name:         "Bard",
				HitDice:      8,
				SavingThrows: []abilities.Ability{abilities.DEX, abilities.CHA},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(backgrounds.Soldier)).
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
			s.Contains(input.CharacterData.ClassResources, shared.ClassResourceBardicInspiration)
			resource := input.CharacterData.ClassResources[shared.ClassResourceBardicInspiration]
			s.Equal("Bardic Inspiration", resource.Name)
			s.Equal(3, resource.Max) // CHA modifier (16 = +3)
			s.Equal(3, resource.Current)
			s.Equal(shared.ResetTypeLongRest, resource.Resets)

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
			RaceID: races.Human,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Warlock,
		},
		BackgroundChoice: backgrounds.Soldier,
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 10,
			abilities.CON: 14,
			abilities.CHA: 16,
		},
	}

	// Mock draft retrieval
	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock race data
	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(races.Human)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        races.Human,
				Name:      "Human",
				Speed:     30,
				Size:      "Medium",
				Languages: []languages.Language{languages.Common},
			},
		}, nil)

	// Mock class data
	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(classes.Warlock)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           classes.Warlock,
				Name:         "Warlock",
				HitDice:      8,
				SavingThrows: []abilities.Ability{abilities.WIS, abilities.CHA},
			},
		}, nil)

	// Mock background data
	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(backgrounds.Soldier)).
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
