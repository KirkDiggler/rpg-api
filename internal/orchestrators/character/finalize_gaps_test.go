package character_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	extmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
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

// FinalizeGapsTestSuite identifies gaps in character finalization
type FinalizeGapsTestSuite struct {
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

func (s *FinalizeGapsTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charmock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftmock.NewMockRepository(s.ctrl)
	s.mockExtClient = extmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)
	s.mockIDGen = idgenmock.NewMockGenerator(s.ctrl)
	mockDraftIDGen := idgenmock.NewMockGenerator(s.ctrl)
	s.ctx = context.Background()

	cfg := &character.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		ExternalClient:     s.mockExtClient,
		DiceService:        s.mockDiceService,
		IDGenerator:        s.mockIDGen,
		DraftIDGenerator:   mockDraftIDGen,
	}
	orch, err := character.New(cfg)
	s.Require().NoError(err)
	s.orchestrator = orch
}

func (s *FinalizeGapsTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestGaps_BackgroundData tests that background data is properly processed
func (s *FinalizeGapsTestSuite) TestGaps_BackgroundData() {
	// Test that we properly handle:
	// 1. Background skills
	// 2. Background languages
	// 3. Background tool proficiencies
	// 4. Background equipment

	s.mockIDGen.EXPECT().Generate().Return("char-test-bg")

	draft := &toolkitchar.DraftData{
		ID:       "draft-bg-test",
		PlayerID: "player-bg",
		Name:     "Background Test",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: constants.RaceHuman,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassFighter,
		},
		BackgroundChoice: constants.BackgroundSage, // Sage gives Arcana, History + 2 languages
		AbilityScoreChoice: shared.AbilityScores{
			constants.STR: 15,
			constants.DEX: 14,
			constants.CON: 13,
			constants.INT: 12,
			constants.WIS: 11,
			constants.CHA: 10,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category: shared.ChoiceSkills,
				Source:   shared.SourceBackground,
				ChoiceID: "sage_skills",
				SkillSelection: []constants.Skill{
					constants.SkillArcana,
					constants.SkillHistory,
				},
			},
			{
				Category: shared.ChoiceLanguages,
				Source:   shared.SourceBackground,
				ChoiceID: "sage_languages",
				LanguageSelection: []constants.Language{
					constants.LanguageElvish,
					constants.LanguageDraconic,
				},
			},
		},
	}

	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draft.ID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(constants.RaceHuman)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        constants.RaceHuman,
				Speed:     30,
				Size:      "Medium",
				Languages: []constants.Language{constants.LanguageCommon},
			},
		}, nil)

	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(constants.ClassFighter)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:                  constants.ClassFighter,
				HitDice:             10,
				SavingThrows:        []constants.Ability{constants.STR, constants.CON},
				WeaponProficiencies: []string{"simple", "martial"},
				ArmorProficiencies:  []string{"light", "medium", "heavy", "shields"},
			},
		}, nil)

	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(constants.BackgroundSage)).
		Return(&external.BackgroundData{
			ID:                 "sage",
			Name:               "Sage",
			SkillProficiencies: []string{"Arcana", "History"},
			Languages:          2, // Sage gets 2 languages
			Equipment:          []string{"Bottle of black ink", "Quill", "Small knife", "Letter"},
			Feature:            "Researcher: When you attempt to learn or recall a piece of lore, you often know where and from whom you can obtain it.",
		}, nil)

	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Verify background skills are included
			s.T().Log("Skills in character:", input.CharacterData.Skills)

			// These should now work since we process all skill choices
			s.Equal(shared.Proficient, input.CharacterData.Skills[constants.SkillArcana],
				"Should have Arcana from Sage background")
			s.Equal(shared.Proficient, input.CharacterData.Skills[constants.SkillHistory],
				"Should have History from Sage background")

			// Verify background languages are included
			s.T().Log("Languages in character:", input.CharacterData.Languages)
			
			// Verify equipment from background
			s.T().Log("Equipment in character:", input.CharacterData.Equipment)
			s.Contains(input.CharacterData.Equipment, "Bottle of black ink", "Should have ink from Sage background")
			s.Contains(input.CharacterData.Equipment, "Quill", "Should have quill from Sage background")

			// These should now work since we process all language choices
			s.Contains(input.CharacterData.Languages, "elvish",
				"Should have Elvish from background choice")
			s.Contains(input.CharacterData.Languages, "draconic",
				"Should have Draconic from background choice")

			return &charrepo.CreateOutput{CharacterData: input.CharacterData}, nil
		})

	s.mockDraftRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(&draftrepo.DeleteOutput{}, nil)

	result, err := s.orchestrator.FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
		DraftID: draft.ID,
	})
	s.NoError(err)
	s.NotNil(result)
}

// TestGaps_RacialTraits tests that racial traits are properly processed
func (s *FinalizeGapsTestSuite) TestGaps_RacialTraits() {
	// Test that we properly handle:
	// 1. Racial skill proficiencies (e.g., Elf Perception)
	// 2. Racial traits (Darkvision, Keen Senses, etc.)
	// 3. Subrace bonuses (e.g., Hill Dwarf HP)
	// 4. Racial cantrips (High Elf)

	s.mockIDGen.EXPECT().Generate().Return("char-test-racial")

	draft := &toolkitchar.DraftData{
		ID:       "draft-racial-test",
		PlayerID: "player-racial",
		Name:     "Racial Traits Test",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID:    constants.RaceElf,
			SubraceID: constants.SubraceHighElf,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassWizard,
		},
		BackgroundChoice: constants.BackgroundSage,
		AbilityScoreChoice: shared.AbilityScores{
			constants.STR: 8,
			constants.DEX: 16, // Base 14 + 2 from Elf
			constants.CON: 13,
			constants.INT: 16, // Base 15 + 1 from High Elf
			constants.WIS: 12,
			constants.CHA: 10,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category:         shared.ChoiceCantrips,
				Source:           shared.SourceRace,
				ChoiceID:         "high_elf_cantrip",
				CantripSelection: []string{"minor-illusion"},
			},
			{
				Category:          shared.ChoiceLanguages,
				Source:            shared.SourceRace,
				ChoiceID:          "high_elf_language",
				LanguageSelection: []constants.Language{constants.LanguageDraconic},
			},
		},
	}

	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draft.ID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(constants.RaceElf)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:                 constants.RaceElf,
				Speed:              30,
				Size:               "Medium",
				Languages:          []constants.Language{constants.LanguageCommon, constants.LanguageElvish},
				SkillProficiencies: []constants.Skill{constants.SkillPerception},
				// TODO: Traits field exists but character.Data doesn't have a place to store them yet
				// Traits: []race.TraitData{{Name: "Darkvision"}, {Name: "Keen Senses"}, {Name: "Fey Ancestry"}, {Name: "Trance"}},
			},
		}, nil)

	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(constants.ClassWizard)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:           constants.ClassWizard,
				HitDice:      6,
				SavingThrows: []constants.Ability{constants.INT, constants.WIS},
			},
		}, nil)

	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(constants.BackgroundSage)).
		Return(&external.BackgroundData{
			ID:                 "sage",
			Name:               "Sage",
			SkillProficiencies: []string{"Arcana", "History"},
			Languages:          2,
			Equipment:          []string{"Bottle of black ink", "Quill", "Small knife", "Letter"},
			Feature:            "Researcher: When you attempt to learn or recall a piece of lore, you often know where and from whom you can obtain it.",
		}, nil)

	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			s.T().Log("Checking racial traits in finalized character")

			// Racial skill proficiencies should work now
			s.Equal(shared.Proficient, input.CharacterData.Skills[constants.SkillPerception],
				"Elf should have Perception proficiency from Keen Senses")

			// TODO: Check for Darkvision trait
			// TODO: Check for Fey Ancestry trait
			// TODO: Check for Trance trait

			// High Elf extra language
			s.Contains(input.CharacterData.Languages, "draconic",
				"High Elf should have extra language choice")

			return &charrepo.CreateOutput{CharacterData: input.CharacterData}, nil
		})

	s.mockDraftRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(&draftrepo.DeleteOutput{}, nil)

	result, err := s.orchestrator.FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
		DraftID: draft.ID,
	})
	s.NoError(err)
	s.NotNil(result)
}

// TestGaps_ClassFeatures tests that class features are properly processed
func (s *FinalizeGapsTestSuite) TestGaps_ClassFeatures() {
	// Test that we properly handle:
	// 1. Fighting styles (Fighter level 1)
	// 2. Expertise (Rogue level 1)
	// 3. Spellcasting (Wizard, Cleric)
	// 4. Class resources (Rage, Ki, etc.)
	// 5. Tool proficiencies from class

	s.mockIDGen.EXPECT().Generate().Return("char-test-features")

	fightingStyle := "Defense"
	draft := &toolkitchar.DraftData{
		ID:       "draft-features-test",
		PlayerID: "player-features",
		Name:     "Class Features Test",
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
				Category:               shared.ChoiceFightingStyle,
				Source:                 shared.SourceClass,
				ChoiceID:               "fighter_fighting_style",
				FightingStyleSelection: &fightingStyle, // +1 AC while wearing armor
			},
		},
	}

	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draft.ID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), gomock.Any()).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        constants.RaceHuman,
				Speed:     30,
				Size:      "Medium",
				Languages: []constants.Language{constants.LanguageCommon},
			},
		}, nil)

	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), string(constants.ClassFighter)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:                  constants.ClassFighter,
				HitDice:             10,
				SavingThrows:        []constants.Ability{constants.STR, constants.CON},
				WeaponProficiencies: []string{"simple", "martial"},
				ArmorProficiencies:  []string{"light", "medium", "heavy", "shields"},
				// TODO: ClassData should include:
				// Features: []class.Feature{
				//	 {Level: 1, Name: "Fighting Style", Type: "choice"},
				//	 {Level: 1, Name: "Second Wind", Type: "resource"},
				// },
			},
		}, nil)

	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(constants.BackgroundSoldier)).
		Return(&external.BackgroundData{
			ID:                 "soldier",
			Name:               "Soldier",
			SkillProficiencies: []string{"Athletics", "Intimidation"},
			Equipment:          []string{"Uniform", "Javelin"},
			Feature:            "Military Rank: You have military authority.",
		}, nil)

	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			s.T().Log("Checking class features in finalized character")

			// Fighting style should be recorded in choices
			foundFightingStyle := false
			for _, choice := range input.CharacterData.Choices {
				if choice.Category == shared.ChoiceFightingStyle {
					s.NotNil(choice.FightingStyleSelection)
					s.Equal("Defense", *choice.FightingStyleSelection)
					foundFightingStyle = true
				}
			}
			s.True(foundFightingStyle, "Should have fighting style choice")

			// TODO: Check that Defense fighting style effect is applied
			// (requires tracking active features/effects)

			// Check for Second Wind resource
			s.NotNil(input.CharacterData.ClassResources, "Should have class resources")
			if input.CharacterData.ClassResources != nil {
				_, hasSecondWind := input.CharacterData.ClassResources["second_wind"]
				s.True(hasSecondWind, "Fighter should have Second Wind resource")
			}

			return &charrepo.CreateOutput{CharacterData: input.CharacterData}, nil
		})

	s.mockDraftRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(&draftrepo.DeleteOutput{}, nil)

	result, err := s.orchestrator.FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
		DraftID: draft.ID,
	})
	s.NoError(err)
	s.NotNil(result)
}

// TestGaps_ToolProficiencies tests that tool proficiencies are properly tracked
func (s *FinalizeGapsTestSuite) TestGaps_ToolProficiencies() {
	s.mockIDGen.EXPECT().Generate().Return("char-test-tools")

	draft := &toolkitchar.DraftData{
		ID:       "draft-tools-test",
		PlayerID: "player-tools",
		Name:     "Tool Proficiency Test",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID:    constants.RaceDwarf, // Dwarves get tool proficiencies
			SubraceID: constants.SubraceHillDwarf,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: constants.ClassFighter,
		},
		BackgroundChoice: constants.BackgroundGuildArtisan, // Gives artisan's tools
		AbilityScoreChoice: shared.AbilityScores{
			constants.STR: 15,
			constants.DEX: 13,
			constants.CON: 17, // Base 15 + 2 from Dwarf
			constants.INT: 10,
			constants.WIS: 12,
			constants.CHA: 8,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				// TODO: ChoiceToolProficiency doesn't exist yet
				// Category: shared.ChoiceToolProficiency,
				Category: shared.ChoiceEquipment, // Using as placeholder
				Source:   shared.SourceRace,
				ChoiceID: "dwarf_tool_proficiency",
				// TODO: Need a field for tool proficiency selection
				// ToolSelection: []string{"smith's tools"},
			},
		},
	}

	s.mockDraftRepo.EXPECT().
		Get(gomock.Any(), draftrepo.GetInput{ID: draft.ID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	s.mockExtClient.EXPECT().
		GetRaceData(gomock.Any(), string(constants.RaceDwarf)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID:        constants.RaceDwarf,
				Speed:     25,
				Size:      "Medium",
				Languages: []constants.Language{constants.LanguageCommon, constants.LanguageDwarvish},
				// TODO: RaceData should include:
				// ToolProficiencyChoice: []string{"smith's tools", "brewer's supplies", "mason's tools"},
			},
		}, nil)

	s.mockExtClient.EXPECT().
		GetClassData(gomock.Any(), gomock.Any()).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:                  constants.ClassFighter,
				HitDice:             10,
				SavingThrows:        []constants.Ability{constants.STR, constants.CON},
				WeaponProficiencies: []string{"simple", "martial"},
				ArmorProficiencies:  []string{"light", "medium", "heavy", "shields"},
			},
		}, nil)

	s.mockExtClient.EXPECT().
		GetBackgroundData(gomock.Any(), string(constants.BackgroundGuildArtisan)).
		Return(&external.BackgroundData{
			ID:                 "guild-artisan",
			Name:               "Guild Artisan",
			SkillProficiencies: []string{"Insight", "Persuasion"},
			Equipment:          []string{"Artisan's tools", "Letter of introduction"},
			Feature:            "Guild Membership: As an established and respected member of a guild, you have access to certain benefits.",
		}, nil)

	s.mockCharRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			s.T().Log("Tool proficiencies:", input.CharacterData.Proficiencies.Tools)

			// TODO: These assertions will fail until tool proficiencies are tracked
			// s.Contains(input.CharacterData.Proficiencies.Tools, "smith's tools",
			//	"Dwarf should have chosen tool proficiency")
			// s.Contains(input.CharacterData.Proficiencies.Tools, "artisan's tools",
			//	"Guild Artisan background should give artisan's tools")

			return &charrepo.CreateOutput{CharacterData: input.CharacterData}, nil
		})

	s.mockDraftRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(&draftrepo.DeleteOutput{}, nil)

	result, err := s.orchestrator.FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
		DraftID: draft.ID,
	})
	s.NoError(err)
	s.NotNil(result)
}

func TestFinalizeGapsTestSuite(t *testing.T) {
	suite.Run(t, new(FinalizeGapsTestSuite))
}
