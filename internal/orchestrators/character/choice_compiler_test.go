package character_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	externalmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/engine"
	enginemock "github.com/KirkDiggler/rpg-api/internal/engine/mock"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
)

type ChoiceCompilerTestSuite struct {
	suite.Suite
	ctrl               *gomock.Controller
	ctx                context.Context
	orchestrator       *character.Orchestrator
	mockCharRepo       *characterrepomock.MockRepository
	mockDraftRepo      *draftrepomock.MockRepository
	mockEngine         *enginemock.MockEngine
	mockExternalClient *externalmock.MockClient
	mockDiceService    *dicemock.MockService
}

func TestChoiceCompilerSuite(t *testing.T) {
	suite.Run(t, new(ChoiceCompilerTestSuite))
}

func (s *ChoiceCompilerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.ctx = context.Background()

	s.mockCharRepo = characterrepomock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftrepomock.NewMockRepository(s.ctrl)
	s.mockEngine = enginemock.NewMockEngine(s.ctrl)
	s.mockExternalClient = externalmock.NewMockClient(s.ctrl)
	s.mockDiceService = dicemock.NewMockService(s.ctrl)

	config := &character.Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		Engine:             s.mockEngine,
		ExternalClient:     s.mockExternalClient,
		DiceService:        s.mockDiceService,
	}

	orchestrator, err := character.New(config)
	s.Require().NoError(err)
	s.orchestrator = orchestrator
}

func (s *ChoiceCompilerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ChoiceCompilerTestSuite) TestCompileChoices_CompleteCharacter() {
	// Test compiling choices for a complete character with all types of choices
	draft := &dnd5e.CharacterDraft{
		ID:           "draft-123",
		PlayerID:     "player-123",
		Name:         "Thorin",
		RaceID:       dnd5e.RaceDwarf,
		SubraceID:    dnd5e.SubraceMountainDwarf,
		ClassID:      dnd5e.ClassFighter,
		BackgroundID: dnd5e.BackgroundSoldier,
		AbilityScores: &dnd5e.AbilityScores{
			Strength:     15,
			Dexterity:    13,
			Constitution: 14,
			Intelligence: 8,
			Wisdom:       12,
			Charisma:     10,
		},
		ChoiceSelections: []dnd5e.ChoiceSelection{
			{
				ChoiceID:     "fighter-skills",
				ChoiceType:   dnd5e.ChoiceTypeSkill,
				Source:       dnd5e.ChoiceSourceClass,
				SelectedKeys: []string{dnd5e.SkillAthletics, dnd5e.SkillIntimidation},
			},
			{
				ChoiceID:     "additional-language",
				ChoiceType:   dnd5e.ChoiceTypeLanguage,
				Source:       dnd5e.ChoiceSourceBackground,
				SelectedKeys: []string{dnd5e.LanguageElvish},
			},
			{
				ChoiceID:     "fighting-style",
				ChoiceType:   dnd5e.ChoiceTypeFightingStyle,
				Source:       dnd5e.ChoiceSourceClass,
				SelectedKeys: []string{"defense"},
			},
			{
				ChoiceID:     "starting-equipment-1",
				ChoiceType:   dnd5e.ChoiceTypeEquipment,
				Source:       dnd5e.ChoiceSourceClass,
				SelectedKeys: []string{"chain-mail"},
			},
			{
				ChoiceID:     "starting-equipment-2",
				ChoiceType:   dnd5e.ChoiceTypeEquipment,
				Source:       dnd5e.ChoiceSourceClass,
				SelectedKeys: []string{"martial-weapon-longsword", "shield"},
			},
		},
	}

	hydratedDraft := &dnd5e.CharacterDraft{
		ID:               draft.ID,
		PlayerID:         draft.PlayerID,
		Name:             draft.Name,
		RaceID:           draft.RaceID,
		SubraceID:        draft.SubraceID,
		ClassID:          draft.ClassID,
		BackgroundID:     draft.BackgroundID,
		AbilityScores:    draft.AbilityScores,
		ChoiceSelections: draft.ChoiceSelections,
		Race: &dnd5e.RaceInfo{
			ID:             dnd5e.RaceDwarf,
			Name:           "Dwarf",
			AbilityBonuses: map[string]int32{"constitution": 2},
			Speed:          25,
			Languages:      []string{dnd5e.LanguageCommon, dnd5e.LanguageDwarvish},
			Proficiencies:  []string{}, // Dwarves don't get weapon proficiencies in base race
		},
		Subrace: &dnd5e.SubraceInfo{
			ID:             dnd5e.SubraceMountainDwarf,
			Name:           "Mountain Dwarf",
			AbilityBonuses: map[string]int32{"strength": 2},
			Proficiencies:  []string{"light armor", "medium armor"},
		},
		Class: &dnd5e.ClassInfo{
			ID:                       dnd5e.ClassFighter,
			Name:                     "Fighter",
			HitDie:                   "1d10",
			ArmorProficiencies:       []string{"all armor", "shields"},
			WeaponProficiencies:      []string{"simple weapons", "martial weapons"},
			ToolProficiencies:        []string{},
			SavingThrowProficiencies: []string{"strength", "constitution"},
			SkillChoicesCount:        2,
			AvailableSkills: []string{
				dnd5e.SkillAcrobatics, dnd5e.SkillAnimalHandling, dnd5e.SkillAthletics,
				dnd5e.SkillHistory, dnd5e.SkillInsight, dnd5e.SkillIntimidation,
				dnd5e.SkillPerception, dnd5e.SkillSurvival,
			},
			Choices: []dnd5e.Choice{
				{
					ID:          "starting-equipment-1",
					Description: "Choose your armor",
					Type:        dnd5e.ChoiceTypeEquipment,
					ChooseCount: 1,
					OptionSet: &dnd5e.ExplicitOptions{
						Options: []dnd5e.ChoiceOption{
							&dnd5e.ItemReference{ItemID: "chain-mail", Name: "Chain Mail"},
							&dnd5e.ItemReference{ItemID: "leather-armor", Name: "Leather Armor"},
						},
					},
				},
				{
					ID:          "starting-equipment-2",
					Description: "Choose your weapons",
					Type:        dnd5e.ChoiceTypeEquipment,
					ChooseCount: 1,
					OptionSet: &dnd5e.ExplicitOptions{
						Options: []dnd5e.ChoiceOption{
							&dnd5e.ItemBundle{
								Items: []dnd5e.BundleItem{
									{
										ItemType: &dnd5e.BundleItemConcreteItem{
											ConcreteItem: &dnd5e.CountedItemReference{
												ItemID:   "martial-weapon-longsword",
												Name:     "Longsword",
												Quantity: 1,
											},
										},
									},
									{
										ItemType: &dnd5e.BundleItemConcreteItem{
											ConcreteItem: &dnd5e.CountedItemReference{
												ItemID:   "shield",
												Name:     "Shield",
												Quantity: 1,
											},
										},
									},
								},
							},
							&dnd5e.CountedItemReference{ItemID: "martial-weapon-greatsword", Name: "Greatsword", Quantity: 1},
						},
					},
				},
			},
		},
		Background: &dnd5e.BackgroundInfo{
			ID:                 dnd5e.BackgroundSoldier,
			Name:               "Soldier",
			SkillProficiencies: []string{dnd5e.SkillAthletics, dnd5e.SkillIntimidation},
			ToolProficiencies:  []string{"gaming set", "vehicles (land)"},
			Languages:          []string{},
			StartingEquipment:  []string{"insignia of rank", "trophy", "deck of cards", "common clothes", "belt pouch"},
		},
	}

	s.Run("successful compilation with all choice types", func() {
		// Setup mocks for FinalizeDraft
		s.mockDraftRepo.EXPECT().
			Get(s.ctx, draftrepo.GetInput{ID: draft.ID}).
			Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(draft)}, nil)

		// Validate draft
		s.mockEngine.EXPECT().
			ValidateCharacterDraft(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterDraftOutput{
				IsComplete: true,
				IsValid:    true,
			}, nil)

		// Mock external client calls for hydration
		s.mockExternalClient.EXPECT().
			GetRaceData(s.ctx, dnd5e.RaceDwarf).
			Return(&external.RaceData{
				ID:             dnd5e.RaceDwarf,
				Name:           "Dwarf",
				Speed:          25,
				AbilityBonuses: map[string]int32{"constitution": 2},
				Languages:      []string{dnd5e.LanguageCommon, dnd5e.LanguageDwarvish},
				Subraces: []external.SubraceData{
					{
						ID:             dnd5e.SubraceMountainDwarf,
						Name:           "Mountain Dwarf",
						AbilityBonuses: map[string]int32{"strength": 2},
						Proficiencies:  []string{"light armor", "medium armor"},
					},
				},
			}, nil)

		s.mockExternalClient.EXPECT().
			GetClassData(s.ctx, dnd5e.ClassFighter).
			Return(&external.ClassData{
				ID:                  dnd5e.ClassFighter,
				Name:                "Fighter",
				HitDice:             "1d10",
				ArmorProficiencies:  []string{"all armor", "shields"},
				WeaponProficiencies: []string{"simple weapons", "martial weapons"},
				SavingThrows:        []string{"strength", "constitution"},
				SkillsCount:         2,
				AvailableSkills: []string{
					dnd5e.SkillAcrobatics, dnd5e.SkillAnimalHandling, dnd5e.SkillAthletics,
					dnd5e.SkillHistory, dnd5e.SkillInsight, dnd5e.SkillIntimidation,
					dnd5e.SkillPerception, dnd5e.SkillSurvival,
				},
				Choices: []dnd5e.Choice{
					hydratedDraft.Class.Choices[0],
					hydratedDraft.Class.Choices[1],
				},
			}, nil)

		s.mockExternalClient.EXPECT().
			GetBackgroundData(s.ctx, dnd5e.BackgroundSoldier).
			Return(&external.BackgroundData{
				ID:                 dnd5e.BackgroundSoldier,
				Name:               "Soldier",
				SkillProficiencies: []string{dnd5e.SkillAthletics, dnd5e.SkillIntimidation},
				Equipment:          []string{"insignia of rank", "trophy", "deck of cards", "common clothes", "belt pouch"},
			}, nil)

		// Calculate stats
		s.mockEngine.EXPECT().
			CalculateCharacterStats(s.ctx, gomock.Any()).
			DoAndReturn(func(
				_ context.Context,
				input *engine.CalculateCharacterStatsInput,
			) (*engine.CalculateCharacterStatsOutput, error) {
				// Verify ability scores have racial bonuses applied
				s.Equal(int32(17), input.Character.AbilityScores.Strength)     // 15 + 2 (mountain dwarf)
				s.Equal(int32(16), input.Character.AbilityScores.Constitution) // 14 + 2 (dwarf)

				// Verify proficiencies were compiled
				s.Contains(input.Character.SkillProficiencies, dnd5e.SkillAthletics)
				s.Contains(input.Character.SkillProficiencies, dnd5e.SkillIntimidation)
				s.Contains(input.Character.Languages, dnd5e.LanguageCommon)
				s.Contains(input.Character.Languages, dnd5e.LanguageDwarvish)
				s.Contains(input.Character.Languages, dnd5e.LanguageElvish)
				// Fighter class armor proficiencies
				s.Contains(input.Character.ArmorProficiencies, "all armor")
				s.Contains(input.Character.ArmorProficiencies, "shields")
				s.Contains(input.Character.WeaponProficiencies, "simple weapons")
				s.Contains(input.Character.WeaponProficiencies, "martial weapons")
				// Mountain Dwarf proficiencies (go to weapon proficiencies due to generic Proficiencies field)
				s.Contains(input.Character.WeaponProficiencies, "light armor")
				s.Contains(input.Character.WeaponProficiencies, "medium armor")
				// Background in this test doesn't provide tool proficiencies
				// since BackgroundData doesn't have that field
				s.Contains(input.Character.SavingThrows, "strength")
				s.Contains(input.Character.SavingThrows, "constitution")

				// Verify equipment was compiled
				s.Len(input.Character.Equipment, 8) // 5 from background + 3 from choices
				hasChainMail := false
				hasLongsword := false
				hasShield := false
				for _, eq := range input.Character.Equipment {
					if eq.ItemID == "chain-mail" {
						hasChainMail = true
					}
					if eq.ItemID == "martial-weapon-longsword" {
						hasLongsword = true
					}
					if eq.ItemID == "shield" {
						hasShield = true
					}
				}
				s.True(hasChainMail, "Should have chain mail from equipment choice")
				s.True(hasLongsword, "Should have longsword from bundle choice")
				s.True(hasShield, "Should have shield from bundle choice")

				return &engine.CalculateCharacterStatsOutput{
					MaxHP:            12,
					ArmorClass:       18, // Chain mail (16) + shield (2)
					Initiative:       1,
					Speed:            25,
					ProficiencyBonus: 2,
				}, nil
			})

		// Validate character
		s.mockEngine.EXPECT().
			ValidateCharacter(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterOutput{
				IsValid: true,
			}, nil)

		// Create character
		s.mockCharRepo.EXPECT().
			Create(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
				char := *input.Character
				char.ID = "char-123"
				return &characterrepo.CreateOutput{Character: &char}, nil
			})

		// Delete draft
		s.mockDraftRepo.EXPECT().
			Delete(s.ctx, draftrepo.DeleteInput{ID: draft.ID}).
			Return(&draftrepo.DeleteOutput{}, nil)

		// Execute
		output, err := s.orchestrator.FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draft.ID,
		})

		// Verify
		s.NoError(err)
		s.NotNil(output)
		s.NotNil(output.Character)
		s.True(output.DraftDeleted)
		s.Equal("Thorin", output.Character.Name)
		s.Equal(int32(17), output.Character.AbilityScores.Strength)
		s.Equal(int32(16), output.Character.AbilityScores.Constitution)
	})
}

func (s *ChoiceCompilerTestSuite) TestCompileChoices_NoChoices() {
	// Test compiling choices when draft has no player choices
	draft := &dnd5e.CharacterDraft{
		ID:           "draft-456",
		PlayerID:     "player-456",
		Name:         "Gandalf",
		RaceID:       dnd5e.RaceHuman,
		ClassID:      dnd5e.ClassWizard,
		BackgroundID: dnd5e.BackgroundSage,
		AbilityScores: &dnd5e.AbilityScores{
			Strength:     8,
			Dexterity:    14,
			Constitution: 13,
			Intelligence: 16,
			Wisdom:       12,
			Charisma:     10,
		},
		ChoiceSelections: []dnd5e.ChoiceSelection{}, // No choices made
	}

	s.Run("automatic grants only", func() {
		s.mockDraftRepo.EXPECT().
			Get(s.ctx, draftrepo.GetInput{ID: draft.ID}).
			Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(draft)}, nil)

		s.mockEngine.EXPECT().
			ValidateCharacterDraft(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterDraftOutput{
				IsComplete: true,
				IsValid:    true,
			}, nil)

		// Mock external client calls
		s.mockExternalClient.EXPECT().
			GetRaceData(s.ctx, dnd5e.RaceHuman).
			Return(&external.RaceData{
				ID:    dnd5e.RaceHuman,
				Name:  "Human",
				Speed: 30,
				AbilityBonuses: map[string]int32{
					"strength":     1,
					"dexterity":    1,
					"constitution": 1,
					"intelligence": 1,
					"wisdom":       1,
					"charisma":     1,
				},
				Languages: []string{dnd5e.LanguageCommon},
			}, nil)

		s.mockExternalClient.EXPECT().
			GetClassData(s.ctx, dnd5e.ClassWizard).
			Return(&external.ClassData{
				ID:                  dnd5e.ClassWizard,
				Name:                "Wizard",
				HitDice:             "1d6",
				ArmorProficiencies:  []string{}, // No armor proficiencies
				WeaponProficiencies: []string{"daggers", "darts", "slings", "quarterstaffs", "light crossbows"},
				SavingThrows:        []string{"intelligence", "wisdom"},
				SkillsCount:         2,
			}, nil)

		s.mockExternalClient.EXPECT().
			GetBackgroundData(s.ctx, dnd5e.BackgroundSage).
			Return(&external.BackgroundData{
				ID:                 dnd5e.BackgroundSage,
				Name:               "Sage",
				SkillProficiencies: []string{dnd5e.SkillArcana, dnd5e.SkillHistory},
				Languages:          2, // Two additional languages
				Equipment: []string{
					"bottle of black ink", "quill", "small knife",
					"letter from colleague", "common clothes",
				},
			}, nil)

		s.mockEngine.EXPECT().
			CalculateCharacterStats(s.ctx, gomock.Any()).
			DoAndReturn(func(
				_ context.Context,
				input *engine.CalculateCharacterStatsInput,
			) (*engine.CalculateCharacterStatsOutput, error) {
				// Verify all ability scores got +1 from human
				s.Equal(int32(9), input.Character.AbilityScores.Strength)
				s.Equal(int32(15), input.Character.AbilityScores.Dexterity)
				s.Equal(int32(14), input.Character.AbilityScores.Constitution)
				s.Equal(int32(17), input.Character.AbilityScores.Intelligence)
				s.Equal(int32(13), input.Character.AbilityScores.Wisdom)
				s.Equal(int32(11), input.Character.AbilityScores.Charisma)

				// Verify automatic grants
				s.Contains(input.Character.SkillProficiencies, dnd5e.SkillArcana)
				s.Contains(input.Character.SkillProficiencies, dnd5e.SkillHistory)
				s.Contains(input.Character.Languages, dnd5e.LanguageCommon)
				s.Empty(input.Character.ArmorProficiencies)
				s.Contains(input.Character.WeaponProficiencies, "daggers")
				s.Contains(input.Character.SavingThrows, "intelligence")
				s.Contains(input.Character.SavingThrows, "wisdom")

				return &engine.CalculateCharacterStatsOutput{
					MaxHP:            5,  // 6 + (-1) CON mod
					ArmorClass:       12, // 10 + 2 DEX mod
					Initiative:       2,
					Speed:            30,
					ProficiencyBonus: 2,
				}, nil
			})

		s.mockEngine.EXPECT().
			ValidateCharacter(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterOutput{
				IsValid: true,
			}, nil)

		s.mockCharRepo.EXPECT().
			Create(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
				char := *input.Character
				char.ID = "char-456"
				return &characterrepo.CreateOutput{Character: &char}, nil
			})

		s.mockDraftRepo.EXPECT().
			Delete(s.ctx, draftrepo.DeleteInput{ID: draft.ID}).
			Return(&draftrepo.DeleteOutput{}, nil)

		// Execute
		output, err := s.orchestrator.FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draft.ID,
		})

		// Verify
		s.NoError(err)
		s.NotNil(output)
		s.NotNil(output.Character)
		s.Equal("Gandalf", output.Character.Name)
	})
}

func (s *ChoiceCompilerTestSuite) TestCompileChoices_EquipmentBundles() {
	// Test equipment bundles and nested choices
	draft := &dnd5e.CharacterDraft{
		ID:           "draft-789",
		PlayerID:     "player-789",
		Name:         "Explorer",
		RaceID:       dnd5e.RaceElf,
		ClassID:      dnd5e.ClassRanger,
		BackgroundID: dnd5e.BackgroundOutlander,
		AbilityScores: &dnd5e.AbilityScores{
			Strength:     13,
			Dexterity:    16,
			Constitution: 14,
			Intelligence: 10,
			Wisdom:       15,
			Charisma:     8,
		},
		ChoiceSelections: []dnd5e.ChoiceSelection{
			{
				ChoiceID:     "explorer-pack",
				ChoiceType:   dnd5e.ChoiceTypeEquipment,
				Source:       dnd5e.ChoiceSourceClass,
				SelectedKeys: []string{"explorer-pack-bundle"},
			},
		},
	}

	s.Run("equipment bundle processing", func() {
		s.mockDraftRepo.EXPECT().
			Get(s.ctx, draftrepo.GetInput{ID: draft.ID}).
			Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(draft)}, nil)

		s.mockEngine.EXPECT().
			ValidateCharacterDraft(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterDraftOutput{
				IsComplete: true,
				IsValid:    true,
			}, nil)

		// Mock external client with equipment bundle choice
		s.mockExternalClient.EXPECT().
			GetRaceData(s.ctx, gomock.Any()).
			Return(&external.RaceData{
				ID:    dnd5e.RaceElf,
				Name:  "Elf",
				Speed: 30,
			}, nil)

		s.mockExternalClient.EXPECT().
			GetClassData(s.ctx, gomock.Any()).
			Return(&external.ClassData{
				ID:      dnd5e.ClassRanger,
				Name:    "Ranger",
				HitDice: "1d10",
				Choices: []dnd5e.Choice{
					{
						ID:          "explorer-pack",
						Description: "Choose your equipment pack",
						Type:        dnd5e.ChoiceTypeEquipment,
						ChooseCount: 1,
						OptionSet: &dnd5e.ExplicitOptions{
							Options: []dnd5e.ChoiceOption{
								&dnd5e.ItemBundle{
									Items: []dnd5e.BundleItem{
										{
											ItemType: &dnd5e.BundleItemConcreteItem{
												ConcreteItem: &dnd5e.CountedItemReference{
													ItemID:   "bedroll",
													Name:     "Bedroll",
													Quantity: 1,
												},
											},
										},
										{
											ItemType: &dnd5e.BundleItemConcreteItem{
												ConcreteItem: &dnd5e.CountedItemReference{
													ItemID:   "mess-kit",
													Name:     "Mess Kit",
													Quantity: 1,
												},
											},
										},
										{
											ItemType: &dnd5e.BundleItemConcreteItem{
												ConcreteItem: &dnd5e.CountedItemReference{
													ItemID:   "tinderbox",
													Name:     "Tinderbox",
													Quantity: 1,
												},
											},
										},
										{
											ItemType: &dnd5e.BundleItemConcreteItem{
												ConcreteItem: &dnd5e.CountedItemReference{
													ItemID:   "torch",
													Name:     "Torch",
													Quantity: 10,
												},
											},
										},
										{
											ItemType: &dnd5e.BundleItemConcreteItem{
												ConcreteItem: &dnd5e.CountedItemReference{
													ItemID:   "rations",
													Name:     "Rations (1 day)",
													Quantity: 10,
												},
											},
										},
										{
											ItemType: &dnd5e.BundleItemConcreteItem{
												ConcreteItem: &dnd5e.CountedItemReference{
													ItemID:   "waterskin",
													Name:     "Waterskin",
													Quantity: 1,
												},
											},
										},
										{
											ItemType: &dnd5e.BundleItemConcreteItem{
												ConcreteItem: &dnd5e.CountedItemReference{
													ItemID:   "rope-hempen",
													Name:     "Hempen Rope (50 feet)",
													Quantity: 1,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}, nil)

		s.mockExternalClient.EXPECT().
			GetBackgroundData(s.ctx, gomock.Any()).
			Return(&external.BackgroundData{
				ID:   dnd5e.BackgroundOutlander,
				Name: "Outlander",
			}, nil)

		s.mockEngine.EXPECT().
			CalculateCharacterStats(s.ctx, gomock.Any()).
			DoAndReturn(func(
				_ context.Context,
				input *engine.CalculateCharacterStatsInput,
			) (*engine.CalculateCharacterStatsOutput, error) {
				// Verify equipment bundle was unpacked
				expectedItems := map[string]int32{
					"bedroll":     1,
					"mess-kit":    1,
					"tinderbox":   1,
					"torch":       10,
					"rations":     10,
					"waterskin":   1,
					"rope-hempen": 1,
				}

				actualItems := make(map[string]int32)
				for _, eq := range input.Character.Equipment {
					actualItems[eq.ItemID] = eq.Quantity
				}

				for itemID, expectedQty := range expectedItems {
					actualQty, found := actualItems[itemID]
					s.True(found, "Should have %s from explorer pack bundle", itemID)
					s.Equal(expectedQty, actualQty, "Should have correct quantity of %s", itemID)
				}

				return &engine.CalculateCharacterStatsOutput{
					MaxHP: 12,
				}, nil
			})

		s.mockEngine.EXPECT().
			ValidateCharacter(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterOutput{
				IsValid: true,
			}, nil)

		s.mockCharRepo.EXPECT().
			Create(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
				char := *input.Character
				char.ID = "char-789"
				return &characterrepo.CreateOutput{Character: &char}, nil
			})

		s.mockDraftRepo.EXPECT().
			Delete(s.ctx, draftrepo.DeleteInput{ID: draft.ID}).
			Return(&draftrepo.DeleteOutput{}, nil)

		// Execute
		output, err := s.orchestrator.FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draft.ID,
		})

		// Verify
		s.NoError(err)
		s.NotNil(output)
		s.NotNil(output.Character)
	})
}

func (s *ChoiceCompilerTestSuite) TestCompileChoices_DuplicateProficiencies() {
	// Test that duplicate proficiencies are deduplicated
	draft := &dnd5e.CharacterDraft{
		ID:           "draft-dup",
		PlayerID:     "player-dup",
		Name:         "Duplicate Test",
		RaceID:       dnd5e.RaceElf,
		ClassID:      dnd5e.ClassRogue,
		BackgroundID: dnd5e.BackgroundCriminal,
		AbilityScores: &dnd5e.AbilityScores{
			Strength:     10,
			Dexterity:    16,
			Constitution: 14,
			Intelligence: 12,
			Wisdom:       13,
			Charisma:     8,
		},
		ChoiceSelections: []dnd5e.ChoiceSelection{
			{
				ChoiceID:   "rogue-skills",
				ChoiceType: dnd5e.ChoiceTypeSkill,
				Source:     dnd5e.ChoiceSourceClass,
				// Choosing skills that overlap with background
				SelectedKeys: []string{dnd5e.SkillStealth, dnd5e.SkillDeception, dnd5e.SkillAcrobatics, dnd5e.SkillPerception},
			},
		},
	}

	s.Run("deduplication of proficiencies", func() {
		s.mockDraftRepo.EXPECT().
			Get(s.ctx, draftrepo.GetInput{ID: draft.ID}).
			Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(draft)}, nil)

		s.mockEngine.EXPECT().
			ValidateCharacterDraft(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterDraftOutput{
				IsComplete: true,
				IsValid:    true,
			}, nil)

		// Mock external client
		s.mockExternalClient.EXPECT().
			GetRaceData(s.ctx, dnd5e.RaceElf).
			Return(&external.RaceData{
				ID:        dnd5e.RaceElf,
				Name:      "Elf",
				Languages: []string{dnd5e.LanguageCommon, dnd5e.LanguageElvish},
			}, nil)

		s.mockExternalClient.EXPECT().
			GetClassData(s.ctx, dnd5e.ClassRogue).
			Return(&external.ClassData{
				ID:                  dnd5e.ClassRogue,
				Name:                "Rogue",
				HitDice:             "1d8",
				SkillsCount:         4,
				WeaponProficiencies: []string{"simple weapons", "hand crossbows", "longswords", "rapiers", "shortswords"},
			}, nil)

		s.mockExternalClient.EXPECT().
			GetBackgroundData(s.ctx, dnd5e.BackgroundCriminal).
			Return(&external.BackgroundData{
				ID:   dnd5e.BackgroundCriminal,
				Name: "Criminal",
				// Criminal gives Deception and Stealth - overlaps with rogue choices
				SkillProficiencies: []string{dnd5e.SkillDeception, dnd5e.SkillStealth},
			}, nil)

		s.mockEngine.EXPECT().
			CalculateCharacterStats(s.ctx, gomock.Any()).
			DoAndReturn(func(
				_ context.Context,
				input *engine.CalculateCharacterStatsInput,
			) (*engine.CalculateCharacterStatsOutput, error) {
				// Verify no duplicate skills
				skillCount := make(map[string]int)
				for _, skill := range input.Character.SkillProficiencies {
					skillCount[skill]++
				}

				// Each skill should appear only once
				for skill, count := range skillCount {
					s.Equal(1, count, "Skill %s should appear only once", skill)
				}

				// Should have exactly 4 skills (deception and stealth not duplicated)
				s.Len(input.Character.SkillProficiencies, 4)
				s.Contains(input.Character.SkillProficiencies, dnd5e.SkillStealth)
				s.Contains(input.Character.SkillProficiencies, dnd5e.SkillDeception)
				s.Contains(input.Character.SkillProficiencies, dnd5e.SkillAcrobatics)
				s.Contains(input.Character.SkillProficiencies, dnd5e.SkillPerception)

				// Verify no duplicate weapon proficiencies
				weaponCount := make(map[string]int)
				for _, weapon := range input.Character.WeaponProficiencies {
					weaponCount[weapon]++
				}
				for weapon, count := range weaponCount {
					s.Equal(1, count, "Weapon proficiency %s should appear only once", weapon)
				}

				return &engine.CalculateCharacterStatsOutput{
					MaxHP: 10,
				}, nil
			})

		s.mockEngine.EXPECT().
			ValidateCharacter(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterOutput{
				IsValid: true,
			}, nil)

		s.mockCharRepo.EXPECT().
			Create(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
				char := *input.Character
				char.ID = "char-dup"
				return &characterrepo.CreateOutput{Character: &char}, nil
			})

		s.mockDraftRepo.EXPECT().
			Delete(s.ctx, draftrepo.DeleteInput{ID: draft.ID}).
			Return(&draftrepo.DeleteOutput{}, nil)

		// Execute
		output, err := s.orchestrator.FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draft.ID,
		})

		// Verify
		s.NoError(err)
		s.NotNil(output)
	})
}

func (s *ChoiceCompilerTestSuite) TestCompileChoices_CategoryReference() {
	// Test equipment choices with category references
	draft := &dnd5e.CharacterDraft{
		ID:           "draft-cat",
		PlayerID:     "player-cat",
		Name:         "Category Test",
		RaceID:       dnd5e.RaceHuman,
		ClassID:      dnd5e.ClassFighter,
		BackgroundID: dnd5e.BackgroundSoldier,
		AbilityScores: &dnd5e.AbilityScores{
			Strength:     16,
			Dexterity:    13,
			Constitution: 15,
			Intelligence: 10,
			Wisdom:       12,
			Charisma:     8,
		},
		ChoiceSelections: []dnd5e.ChoiceSelection{
			{
				ChoiceID:     "martial-weapon-choice",
				ChoiceType:   dnd5e.ChoiceTypeEquipment,
				Source:       dnd5e.ChoiceSourceClass,
				SelectedKeys: []string{"greataxe", "warhammer"},
			},
		},
	}

	s.Run("category reference equipment selection", func() {
		s.mockDraftRepo.EXPECT().
			Get(s.ctx, draftrepo.GetInput{ID: draft.ID}).
			Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(draft)}, nil)

		s.mockEngine.EXPECT().
			ValidateCharacterDraft(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterDraftOutput{
				IsComplete: true,
				IsValid:    true,
			}, nil)

		// Mock external client
		s.mockExternalClient.EXPECT().
			GetRaceData(s.ctx, gomock.Any()).
			Return(&external.RaceData{
				ID:   dnd5e.RaceHuman,
				Name: "Human",
			}, nil)

		s.mockExternalClient.EXPECT().
			GetClassData(s.ctx, gomock.Any()).
			Return(&external.ClassData{
				ID:      dnd5e.ClassFighter,
				Name:    "Fighter",
				HitDice: "1d10",
				Choices: []dnd5e.Choice{
					{
						ID:          "martial-weapon-choice",
						Description: "Choose two martial weapons",
						Type:        dnd5e.ChoiceTypeEquipment,
						ChooseCount: 2,
						OptionSet: &dnd5e.CategoryReference{
							CategoryID: "martial-weapons",
							ExcludeIDs: []string{}, // No exclusions
						},
					},
				},
			}, nil)

		s.mockExternalClient.EXPECT().
			GetBackgroundData(s.ctx, gomock.Any()).
			Return(&external.BackgroundData{
				ID:   dnd5e.BackgroundSoldier,
				Name: "Soldier",
			}, nil)

		s.mockEngine.EXPECT().
			CalculateCharacterStats(s.ctx, gomock.Any()).
			DoAndReturn(func(
				_ context.Context,
				input *engine.CalculateCharacterStatsInput,
			) (*engine.CalculateCharacterStatsOutput, error) {
				// Verify selected items from category were added
				hasGreataxe := false
				hasWarhammer := false
				for _, eq := range input.Character.Equipment {
					if eq.ItemID == "greataxe" {
						hasGreataxe = true
						s.Equal(int32(1), eq.Quantity)
					}
					if eq.ItemID == "warhammer" {
						hasWarhammer = true
						s.Equal(int32(1), eq.Quantity)
					}
				}
				s.True(hasGreataxe, "Should have greataxe from category selection")
				s.True(hasWarhammer, "Should have warhammer from category selection")

				return &engine.CalculateCharacterStatsOutput{
					MaxHP: 13,
				}, nil
			})

		s.mockEngine.EXPECT().
			ValidateCharacter(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterOutput{
				IsValid: true,
			}, nil)

		s.mockCharRepo.EXPECT().
			Create(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
				char := *input.Character
				char.ID = "char-cat"
				return &characterrepo.CreateOutput{Character: &char}, nil
			})

		s.mockDraftRepo.EXPECT().
			Delete(s.ctx, draftrepo.DeleteInput{ID: draft.ID}).
			Return(&draftrepo.DeleteOutput{}, nil)

		// Execute
		output, err := s.orchestrator.FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draft.ID,
		})

		// Verify
		s.NoError(err)
		s.NotNil(output)
	})
}

func (s *ChoiceCompilerTestSuite) TestCompileChoices_MissingChoiceDefinition() {
	// Test handling when choice definition is not found in hydrated data
	draft := &dnd5e.CharacterDraft{
		ID:           "draft-missing",
		PlayerID:     "player-missing",
		Name:         "Missing Choice Test",
		RaceID:       dnd5e.RaceHuman,
		ClassID:      dnd5e.ClassFighter,
		BackgroundID: dnd5e.BackgroundSoldier,
		AbilityScores: &dnd5e.AbilityScores{
			Strength:     15,
			Dexterity:    13,
			Constitution: 14,
			Intelligence: 10,
			Wisdom:       12,
			Charisma:     8,
		},
		ChoiceSelections: []dnd5e.ChoiceSelection{
			{
				ChoiceID:     "unknown-choice",
				ChoiceType:   dnd5e.ChoiceTypeEquipment,
				Source:       dnd5e.ChoiceSourceClass,
				SelectedKeys: []string{"mystery-item-1", "mystery-item-2"},
			},
		},
	}

	s.Run("missing choice definition fallback", func() {
		s.mockDraftRepo.EXPECT().
			Get(s.ctx, draftrepo.GetInput{ID: draft.ID}).
			Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(draft)}, nil)

		s.mockEngine.EXPECT().
			ValidateCharacterDraft(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterDraftOutput{
				IsComplete: true,
				IsValid:    true,
			}, nil)

		// Mock external client returns class without the unknown choice
		s.mockExternalClient.EXPECT().
			GetRaceData(s.ctx, gomock.Any()).
			Return(&external.RaceData{
				ID:   dnd5e.RaceHuman,
				Name: "Human",
			}, nil)

		s.mockExternalClient.EXPECT().
			GetClassData(s.ctx, gomock.Any()).
			Return(&external.ClassData{
				ID:      dnd5e.ClassFighter,
				Name:    "Fighter",
				HitDice: "1d10",
				Choices: []dnd5e.Choice{}, // No choices defined
			}, nil)

		s.mockExternalClient.EXPECT().
			GetBackgroundData(s.ctx, gomock.Any()).
			Return(&external.BackgroundData{
				ID:   dnd5e.BackgroundSoldier,
				Name: "Soldier",
			}, nil)

		s.mockEngine.EXPECT().
			CalculateCharacterStats(s.ctx, gomock.Any()).
			DoAndReturn(func(
				_ context.Context,
				input *engine.CalculateCharacterStatsInput,
			) (*engine.CalculateCharacterStatsOutput, error) {
				// Verify items were still added even without choice definition
				hasItem1 := false
				hasItem2 := false
				for _, eq := range input.Character.Equipment {
					if eq.ItemID == "mystery-item-1" {
						hasItem1 = true
						s.Equal("mystery-item-1", eq.Name) // Name defaults to ID
						s.Equal(int32(1), eq.Quantity)
					}
					if eq.ItemID == "mystery-item-2" {
						hasItem2 = true
						s.Equal("mystery-item-2", eq.Name)
						s.Equal(int32(1), eq.Quantity)
					}
				}
				s.True(hasItem1, "Should have mystery-item-1 even without choice definition")
				s.True(hasItem2, "Should have mystery-item-2 even without choice definition")

				return &engine.CalculateCharacterStatsOutput{
					MaxHP: 12,
				}, nil
			})

		s.mockEngine.EXPECT().
			ValidateCharacter(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterOutput{
				IsValid: true,
			}, nil)

		s.mockCharRepo.EXPECT().
			Create(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
				char := *input.Character
				char.ID = "char-missing"
				return &characterrepo.CreateOutput{Character: &char}, nil
			})

		s.mockDraftRepo.EXPECT().
			Delete(s.ctx, draftrepo.DeleteInput{ID: draft.ID}).
			Return(&draftrepo.DeleteOutput{}, nil)

		// Execute
		output, err := s.orchestrator.FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draft.ID,
		})

		// Verify
		s.NoError(err)
		s.NotNil(output)
	})
}

func (s *ChoiceCompilerTestSuite) TestCompileChoices_ErrorHandling() {
	draft := &dnd5e.CharacterDraft{
		ID:           "draft-error",
		PlayerID:     "player-error",
		Name:         "Error Test",
		RaceID:       dnd5e.RaceHuman,
		ClassID:      dnd5e.ClassFighter,
		BackgroundID: dnd5e.BackgroundSoldier,
		AbilityScores: &dnd5e.AbilityScores{
			Strength:     15,
			Dexterity:    13,
			Constitution: 14,
			Intelligence: 10,
			Wisdom:       12,
			Charisma:     8,
		},
	}

	s.Run("hydration error during finalization", func() {
		s.mockDraftRepo.EXPECT().
			Get(s.ctx, draftrepo.GetInput{ID: draft.ID}).
			Return(&draftrepo.GetOutput{Draft: dnd5e.FromCharacterDraft(draft)}, nil)

		s.mockEngine.EXPECT().
			ValidateCharacterDraft(s.ctx, gomock.Any()).
			Return(&engine.ValidateCharacterDraftOutput{
				IsComplete: true,
				IsValid:    true,
			}, nil)

		// Mock external client error
		s.mockExternalClient.EXPECT().
			GetRaceData(s.ctx, gomock.Any()).
			Return(nil, errors.New("external API error"))

		// Execute
		output, err := s.orchestrator.FinalizeDraft(s.ctx, &character.FinalizeDraftInput{
			DraftID: draft.ID,
		})

		// Verify
		s.Error(err)
		s.Contains(err.Error(), "failed to hydrate draft")
		s.Nil(output)
	})
}
