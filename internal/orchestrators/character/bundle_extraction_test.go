package character

import (
	"context"
	"testing"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	externalMock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	diceMock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenMock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	charrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charMock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftMock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type BundleExtractionTestSuite struct {
	suite.Suite
	ctrl               *gomock.Controller
	mockCharRepo       *charMock.MockRepository
	mockDraftRepo      *draftMock.MockRepository
	mockExternalClient *externalMock.MockClient
	mockDiceService    *diceMock.MockService
	mockIDGen          *idgenMock.MockGenerator
	mockDraftIDGen     *idgenMock.MockGenerator
	orchestrator       *Orchestrator
	ctx                context.Context
}

func TestBundleExtractionTestSuite(t *testing.T) {
	suite.Run(t, new(BundleExtractionTestSuite))
}

func (s *BundleExtractionTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charMock.NewMockRepository(s.ctrl)
	s.mockDraftRepo = draftMock.NewMockRepository(s.ctrl)
	s.mockExternalClient = externalMock.NewMockClient(s.ctrl)
	s.mockDiceService = diceMock.NewMockService(s.ctrl)
	s.mockIDGen = idgenMock.NewMockGenerator(s.ctrl)
	s.mockDraftIDGen = idgenMock.NewMockGenerator(s.ctrl)
	s.ctx = context.Background()

	orchestrator, err := New(&Config{
		CharacterRepo:      s.mockCharRepo,
		CharacterDraftRepo: s.mockDraftRepo,
		ExternalClient:     s.mockExternalClient,
		DiceService:        s.mockDiceService,
		IDGenerator:        s.mockIDGen,
		DraftIDGenerator:   s.mockDraftIDGen,
	})
	s.Require().NoError(err)
	s.orchestrator = orchestrator
}

func (s *BundleExtractionTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *BundleExtractionTestSuite) TestFinalizeDraft_ExtractsFighterMartialWeaponAndShieldBundle() {
	// Arrange
	draftID := "draft_123"
	characterID := "char_456"

	// Create a draft with fighter class and martial weapon + shield bundle selection
	draft := &toolkitchar.DraftData{
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
			abilities.STR: 15,
			abilities.DEX: 13,
			abilities.CON: 14,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category: shared.ChoiceEquipment,
				Source:   "class",
				// This simulates selecting a bundle that contains:
				// - longsword (martial weapon)
				// - shield
				// The bundle format is "bundle_1:0:longsword" and "bundle_1:1:shield"
				EquipmentSelection: []string{
					"bundle_1:0:longsword",
					"bundle_1:1:shield",
				},
			},
			{
				Category: shared.ChoiceEquipment,
				Source:   "class",
				// Another choice might be just a regular item (not in a bundle)
				EquipmentSelection: []string{
					"explorers-pack",
				},
			},
		},
	}

	// Mock the draft retrieval
	s.mockDraftRepo.EXPECT().
		Get(s.ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock external data retrieval
	s.setupExternalMocks(races.Human, classes.Fighter, backgrounds.Soldier)

	// Mock ID generation for the new character
	s.mockIDGen.EXPECT().
		Generate().
		Return(characterID)

	// Expected character data after bundle extraction
	expectedCharacter := &toolkitchar.Data{
		ID:            characterID,
		PlayerID:      draft.PlayerID,
		Name:          draft.Name,
		RaceID:        draft.RaceChoice.RaceID,
		ClassID:       draft.ClassChoice.ClassID,
		BackgroundID:  draft.BackgroundChoice,
		Level:         1,
		AbilityScores: draft.AbilityScoreChoice,
		Equipment: []string{
			"longsword",      // Extracted from bundle_1:0:longsword
			"shield",         // Extracted from bundle_1:1:shield
			"explorers-pack", // Regular item, not from bundle
		},
		HitPoints:    10, // Fighter base HP
		MaxHitPoints: 10,
	}

	// Mock character creation
	s.mockCharRepo.EXPECT().
		Create(s.ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Verify that the equipment was properly extracted from bundles
			s.Require().NotNil(input.CharacterData)
			s.Require().Len(input.CharacterData.Equipment, 3, "Should have 3 equipment items")

			// Check that bundle references were unpacked
			s.Assert().Contains(input.CharacterData.Equipment, "longsword", "Should have longsword extracted from bundle")
			s.Assert().Contains(input.CharacterData.Equipment, "shield", "Should have shield extracted from bundle")
			s.Assert().Contains(input.CharacterData.Equipment, "explorers-pack", "Should have explorer's pack")

			// Make sure no bundle references remain
			for _, item := range input.CharacterData.Equipment {
				s.Assert().NotContains(item, "bundle_", "Should not have any bundle references in final equipment")
			}

			return &charrepo.CreateOutput{
				CharacterData: expectedCharacter,
			}, nil
		})

	// Mock draft deletion
	s.mockDraftRepo.EXPECT().
		Delete(s.ctx, draftrepo.DeleteInput{ID: draftID}).
		Return(&draftrepo.DeleteOutput{}, nil)

	// Act
	result, err := s.orchestrator.FinalizeDraft(s.ctx, &FinalizeDraftInput{
		DraftID: draftID,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Assert().Equal(characterID, result.Character.ID)
	s.Assert().True(result.DraftDeleted)

	// Verify the equipment in the result
	s.Assert().Len(result.Character.Equipment, 3)
	s.Assert().Contains(result.Character.Equipment, "longsword")
	s.Assert().Contains(result.Character.Equipment, "shield")
	s.Assert().Contains(result.Character.Equipment, "explorers-pack")
}

func (s *BundleExtractionTestSuite) setupExternalMocks(raceID races.Race, classID classes.Class, backgroundID backgrounds.Background) {
	// Mock race data retrieval
	s.mockExternalClient.EXPECT().
		GetRaceData(s.ctx, string(raceID)).
		Return(&external.RaceDataOutput{
			RaceData: &race.Data{
				ID: raceID,
			},
		}, nil)

	// Mock class data retrieval
	s.mockExternalClient.EXPECT().
		GetClassData(s.ctx, string(classID)).
		Return(&external.ClassDataOutput{
			ClassData: &class.Data{
				ID:                classID,
				HitDice:           10,
				HitPointsPerLevel: 6,
			},
		}, nil)

	// Mock background data retrieval
	s.mockExternalClient.EXPECT().
		GetBackgroundData(s.ctx, string(backgroundID)).
		Return(&external.BackgroundData{
			ID: string(backgroundID),
		}, nil)
}

func (s *BundleExtractionTestSuite) TestFinalizeDraft_HandlesMultipleBundles() {
	// Arrange
	draftID := "draft_multi"
	characterID := "char_multi"

	draft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_multi",
		Name:     "Multi Bundle Fighter",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: races.Dwarf,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Fighter,
		},
		BackgroundChoice: backgrounds.Noble,
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 12,
			abilities.CON: 15,
			abilities.INT: 10,
			abilities.WIS: 13,
			abilities.CHA: 8,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category: shared.ChoiceEquipment,
				Source:   "class",
				EquipmentSelection: []string{
					// First bundle: martial weapon + shield
					"bundle_1:0:battleaxe",
					"bundle_1:1:shield",
					// Second bundle: two handaxes
					"bundle_2:0:handaxe",
					"bundle_2:1:handaxe",
					// Third bundle: dungeoneers pack items
					"bundle_3:0:crowbar",
					"bundle_3:1:hammer",
					"bundle_3:2:pitons",
				},
			},
		},
	}

	// Mock the draft retrieval
	s.mockDraftRepo.EXPECT().
		Get(s.ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock external data retrieval
	s.setupExternalMocks(races.Dwarf, classes.Fighter, backgrounds.Noble)

	// Mock ID generation
	s.mockIDGen.EXPECT().
		Generate().
		Return(characterID)

	// Mock character creation and verify bundle extraction
	s.mockCharRepo.EXPECT().
		Create(s.ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Verify all bundle items were extracted
			s.Require().Len(input.CharacterData.Equipment, 7, "Should have 7 equipment items from 3 bundles")

			expectedItems := []string{
				"battleaxe", "shield", // Bundle 1
				"handaxe", "handaxe", // Bundle 2 (two handaxes)
				"crowbar", "hammer", "pitons", // Bundle 3
			}

			for _, expectedItem := range expectedItems {
				s.Assert().Contains(input.CharacterData.Equipment, expectedItem,
					"Should have %s extracted from bundle", expectedItem)
			}

			// Ensure no bundle references remain
			for _, item := range input.CharacterData.Equipment {
				s.Assert().NotContains(item, "bundle_",
					"Item %s should not contain bundle reference", item)
			}

			return &charrepo.CreateOutput{
				CharacterData: &toolkitchar.Data{
					ID:           characterID,
					PlayerID:     draft.PlayerID,
					Name:         draft.Name,
					Equipment:    input.CharacterData.Equipment,
					HitPoints:    10,
					MaxHitPoints: 10,
				},
			}, nil
		})

	// Mock draft deletion
	s.mockDraftRepo.EXPECT().
		Delete(s.ctx, draftrepo.DeleteInput{ID: draftID}).
		Return(&draftrepo.DeleteOutput{}, nil)

	// Act
	result, err := s.orchestrator.FinalizeDraft(s.ctx, &FinalizeDraftInput{
		DraftID: draftID,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Assert().Len(result.Character.Equipment, 7)
}

func (s *BundleExtractionTestSuite) TestFinalizeDraft_MixedBundlesAndRegularItems() {
	// Arrange
	draftID := "draft_mixed"
	characterID := "char_mixed"

	draft := &toolkitchar.DraftData{
		ID:       draftID,
		PlayerID: "player_mixed",
		Name:     "Mixed Equipment Fighter",
		RaceChoice: toolkitchar.RaceChoice{
			RaceID: races.Elf,
		},
		ClassChoice: toolkitchar.ClassChoice{
			ClassID: classes.Fighter,
		},
		BackgroundChoice: backgrounds.Hermit,
		AbilityScoreChoice: shared.AbilityScores{
			abilities.STR: 14,
			abilities.DEX: 16,
			abilities.CON: 13,
			abilities.INT: 11,
			abilities.WIS: 14,
			abilities.CHA: 9,
		},
		Choices: []toolkitchar.ChoiceData{
			{
				Category: shared.ChoiceEquipment,
				Source:   "class",
				EquipmentSelection: []string{
					"leather-armor",         // Regular item
					"bundle_1:0:shortsword", // Bundle item
					"bundle_1:1:shield",     // Bundle item
					"longbow",               // Regular item
					"bundle_2:0:arrow",      // Bundle with multiple arrows
					"bundle_2:1:arrow",
					"bundle_2:2:arrow",
					"backpack", // Regular item
				},
			},
		},
	}

	// Mock the draft retrieval
	s.mockDraftRepo.EXPECT().
		Get(s.ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, nil)

	// Mock external data retrieval
	s.setupExternalMocks(races.Elf, classes.Fighter, backgrounds.Hermit)

	// Mock ID generation
	s.mockIDGen.EXPECT().
		Generate().
		Return(characterID)

	// Mock character creation
	s.mockCharRepo.EXPECT().
		Create(s.ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, input charrepo.CreateInput) (*charrepo.CreateOutput, error) {
			// Should have 8 items total: 3 regular + 5 from bundles
			s.Require().Len(input.CharacterData.Equipment, 8)

			// Check regular items are unchanged
			s.Assert().Contains(input.CharacterData.Equipment, "leather-armor")
			s.Assert().Contains(input.CharacterData.Equipment, "longbow")
			s.Assert().Contains(input.CharacterData.Equipment, "backpack")

			// Check bundle items are extracted
			s.Assert().Contains(input.CharacterData.Equipment, "shortsword")
			s.Assert().Contains(input.CharacterData.Equipment, "shield")

			// Count arrows (should be 3)
			arrowCount := 0
			for _, item := range input.CharacterData.Equipment {
				if item == "arrow" {
					arrowCount++
				}
			}
			s.Assert().Equal(3, arrowCount, "Should have 3 arrows from bundle_2")

			return &charrepo.CreateOutput{
				CharacterData: &toolkitchar.Data{
					ID:           characterID,
					Equipment:    input.CharacterData.Equipment,
					HitPoints:    10,
					MaxHitPoints: 10,
				},
			}, nil
		})

	// Mock draft deletion
	s.mockDraftRepo.EXPECT().
		Delete(s.ctx, draftrepo.DeleteInput{ID: draftID}).
		Return(&draftrepo.DeleteOutput{}, nil)

	// Act
	result, err := s.orchestrator.FinalizeDraft(s.ctx, &FinalizeDraftInput{
		DraftID: draftID,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(result)
}
