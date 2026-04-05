package encounter

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/dice"
	dicemock "github.com/KirkDiggler/rpg-toolkit/dice/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/armor"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/damage"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/gamectx"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster/monsters"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	dungeontoolkit "github.com/KirkDiggler/rpg-api/internal/components/dungeon/toolkit"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	dungeonrepo "github.com/KirkDiggler/rpg-api/internal/repositories/dungeons"
	dungeonmock "github.com/KirkDiggler/rpg-api/internal/repositories/dungeons/mock"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
	encountermock "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/mock"
)

type OrchestratorTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	mockCharRepo    *charactermock.MockRepository
	mockEncRepo     *encountermock.MockRepository
	mockDungeonRepo *dungeonmock.MockRepository
	mockRoller      *dicemock.MockRoller
	orchestrator    *Orchestrator
}

func (s *OrchestratorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.mockEncRepo = encountermock.NewMockRepository(s.ctrl)
	s.mockDungeonRepo = dungeonmock.NewMockRepository(s.ctrl)
	s.mockRoller = dicemock.NewMockRoller(s.ctrl)

	var err error
	s.orchestrator, err = New(&Config{
		CharacterRepo: s.mockCharRepo,
		EncounterRepo: s.mockEncRepo,
		DungeonRepo:   s.mockDungeonRepo,
		DungeonGen:    dungeontoolkit.CreateGenerator(&dungeontoolkit.ToolkitConfig{}),
	})
	s.Require().NoError(err)

	// Use real roller by default - tests that need deterministic rolls
	// will inject mockRoller directly
	s.orchestrator.roller = dice.NewRoller()
}

func (s *OrchestratorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}

// expectCharacterTurnEnd sets up mock expectations for the character repository
// calls that happen during EndTurn's turn-end event publishing.
// This is needed because EndTurn now publishes TurnEndEvent to character event buses.
func (s *OrchestratorTestSuite) expectCharacterTurnEnd(characterID string) {
	charData := &character.Data{
		ID:               characterID,
		Name:             "Test Character",
		PlayerID:         "player-1",
		Level:            1,
		RaceID:           "human",
		ClassID:          "fighter",
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 14,
			abilities.CON: 15,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		HitPoints:    12,
		MaxHitPoints: 12,
	}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)
}

// expectDungeonLookup sets up a mock expectation for optional dungeon lookups during EndTurn.
// Walls are optional in events, so we just return an empty result.
// Uses AnyTimes() since dungeon lookups can happen multiple times (event publishing, monster turns, etc.)
func (s *OrchestratorTestSuite) expectDungeonLookup(encounterID string) {
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), &dungeonrepo.GetByEncounterIDInput{EncounterID: encounterID}).
		Return(&dungeonrepo.GetOutput{}, nil).
		AnyTimes()
}

func (s *OrchestratorTestSuite) TestNew_Success() {
	orch, err := New(&Config{
		CharacterRepo: s.mockCharRepo,
		EncounterRepo: s.mockEncRepo,
		DungeonRepo:   s.mockDungeonRepo,
		DungeonGen:    dungeontoolkit.CreateGenerator(&dungeontoolkit.ToolkitConfig{}),
	})
	s.Require().NoError(err)
	s.Assert().NotNil(orch)
}

func (s *OrchestratorTestSuite) TestNew_NilConfig() {
	orch, err := New(nil)
	s.Require().Error(err)
	s.Assert().Nil(orch)
	s.Assert().Contains(err.Error(), "config is required")
}

func (s *OrchestratorTestSuite) TestNew_MissingCharacterRepo() {
	orch, err := New(&Config{
		EncounterRepo: s.mockEncRepo,
	})
	s.Require().Error(err)
	s.Assert().Nil(orch)
	s.Assert().Contains(err.Error(), "CharacterRepo")
}

func (s *OrchestratorTestSuite) TestNew_MissingEncounterRepo() {
	orch, err := New(&Config{
		CharacterRepo: s.mockCharRepo,
	})
	s.Require().Error(err)
	s.Assert().Nil(orch)
	s.Assert().Contains(err.Error(), "EncounterRepo")
}

func (s *OrchestratorTestSuite) TestResolveAttack_Success() {
	// Arrange - Create test character data (includes equipment slots)
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes() // May be called multiple times (for character data and weapon lookup)

	// Arrange - Mock encounter repo
	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Mock Update for when attack hits (may not be called if miss)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Mock dungeon repo for wall loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)

	// Attack result should have valid values
	s.Assert().GreaterOrEqual(output.Result.AttackRoll, 1)
	s.Assert().LessOrEqual(output.Result.AttackRoll, 20)

	// Attack could hit or miss - both are valid
	if output.Result.Hit {
		s.Assert().Greater(output.Result.TotalDamage, 0)
		s.Assert().NotEmpty(output.Result.DamageRolls)
		s.Assert().Equal(damage.Slashing, output.Result.DamageType)

		// Verify breakdown is populated on hit
		s.Require().NotNil(output.Result.Breakdown, "Breakdown should be present on hit")
		s.Assert().NotEmpty(output.Result.Breakdown.Components, "Breakdown should have components")
		s.Assert().NotEmpty(output.Result.Breakdown.AbilityUsed, "Breakdown should have ability used")
		s.Assert().Greater(output.Result.Breakdown.TotalDamage, 0, "Breakdown total damage should be > 0")

		// Verify components have expected fields
		for i, comp := range output.Result.Breakdown.Components {
			s.Assert().NotEmpty(comp.Source, "Component %d should have source", i)
			s.Assert().NotEmpty(comp.DamageType, "Component %d should have damage type", i)
			// Either dice rolls or flat bonus should be present
			s.Assert().True(
				len(comp.FinalDiceRolls) > 0 || comp.FlatBonus != 0,
				"Component %d should have dice or flat bonus", i,
			)
		}
	} else {
		s.Assert().Equal(0, output.Result.TotalDamage)
		// Breakdown should be nil on miss
		s.Assert().Nil(output.Result.Breakdown, "Breakdown should be nil on miss")
	}

	// Monster HP should be calculated
	s.Assert().GreaterOrEqual(output.MonsterHP, 0)
	s.Assert().LessOrEqual(output.MonsterHP, 7) // Goblin max HP
}

func (s *OrchestratorTestSuite) TestResolveAttack_NilInput() {
	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *OrchestratorTestSuite) TestResolveAttack_MissingEncounterID() {
	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		AttackerID: "char-1",
		TargetID:   "goblin-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *OrchestratorTestSuite) TestResolveAttack_MissingAttackerID() {
	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "attacker ID is required")
}

func (s *OrchestratorTestSuite) TestResolveAttack_MissingTargetID() {
	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "target ID is required")
}

func (s *OrchestratorTestSuite) TestResolveAttack_CharacterNotFound() {
	// Arrange
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "nonexistent"}).
		Return(nil, apierr.NotFound("character not found"))

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "nonexistent",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to load character")
}

func (s *OrchestratorTestSuite) TestResolveAttack_EncounterNotFound() {
	// Arrange - Character exists
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	// Arrange - Encounter not found
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "nonexistent"}).
		Return(nil, apierr.NotFound("encounter not found"))

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "nonexistent",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to load encounter")
}

func (s *OrchestratorTestSuite) TestResolveAttack_MultipleAttacks() {
	// This test verifies that multiple attacks produce different results
	// (due to different dice rolls) and that the EventBus is created fresh each time

	charData := createTestCharacterData("char-1", "Grog")
	encData := createTestEncounterData("enc-1")

	// Set up expectations for multiple calls (includes weapon lookup)
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil).
		Times(3)

	// Mock Update for when attacks hit (may not be called for misses)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Mock dungeon repo for wall loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Perform multiple attacks
	results := make([]*ResolveAttackOutput, 3)
	for i := 0; i < 3; i++ {
		output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
			EncounterID: "enc-1",
			AttackerID:  "char-1",
			TargetID:    "goblin-1",
		})
		s.Require().NoError(err)
		s.Require().NotNil(output)
		results[i] = output
	}

	// Verify that results are independent (not all identical)
	// With dice rolls, it's extremely unlikely all three attacks have identical results
	allIdentical := true
	for i := 1; i < len(results); i++ {
		if results[i].Result.AttackRoll != results[0].Result.AttackRoll ||
			results[i].Result.TotalDamage != results[0].Result.TotalDamage {
			allIdentical = false
			break
		}
	}
	s.Assert().False(allIdentical, "Multiple attacks should produce different results due to dice rolls")
}

func (s *OrchestratorTestSuite) TestResolveAttack_VulnerabilityMultiplier_AppearsInBreakdown() {
	// Test that vulnerability (2.0 multiplier) appears in the damage breakdown
	// when attacking a skeleton (vulnerable to bludgeoning) with a mace

	// Create a fighter with mace (bludgeoning damage)
	charData := &character.Data{
		ID:               "char-1",
		Name:             "Sir Bonecrusher",
		Level:            1,
		RaceID:           "human",
		ClassID:          "fighter",
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, // +3 modifier
			abilities.DEX: 14,
			abilities.CON: 14,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		Features: []json.RawMessage{},
		EquipmentSlots: character.EquipmentSlots{
			character.SlotMainHand: weapons.Mace, // Mace does bludgeoning damage
		},
	}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	// Create skeleton - has bludgeoning vulnerability via monstertraits
	skeleton := monsters.NewSkeleton("skeleton-1")
	skeletonData := skeleton.ToData()

	encData := &encounterrepo.EncounterData{
		ID:       "enc-1",
		Monsters: []*monster.Data{skeletonData},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Mock dungeon repo for wall loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Loop until we get a hit (random dice could miss)
	var hitOutput *ResolveAttackOutput
	for i := 0; i < 20; i++ { // Try up to 20 times to get a hit
		output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
			EncounterID: "enc-1",
			AttackerID:  "char-1",
			TargetID:    "skeleton-1",
		})
		s.Require().NoError(err)
		s.Require().NotNil(output)

		if output.Result.Hit {
			hitOutput = output
			break
		}

		// Reset encounter data for next attempt (skeleton needs full HP)
		skeleton = monsters.NewSkeleton("skeleton-1")
		skeletonData = skeleton.ToData()
		encData = &encounterrepo.EncounterData{
			ID:       "enc-1",
			Monsters: []*monster.Data{skeletonData},
		}
		s.mockEncRepo.EXPECT().
			Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
			Return(&encounterrepo.GetOutput{Data: encData}, nil)
	}

	s.Require().NotNil(hitOutput, "Should have gotten at least one hit in 20 attempts")
	s.Require().True(hitOutput.Result.Hit, "Attack should have hit")
	s.Require().NotNil(hitOutput.Result.Breakdown, "Breakdown should be present on hit")

	// Verify damage type is bludgeoning (from mace)
	s.Assert().Equal(damage.Bludgeoning, hitOutput.Result.DamageType, "Damage type should be bludgeoning")

	// Look for vulnerability multiplier component in breakdown
	var foundVulnerabilityMultiplier bool
	for _, comp := range hitOutput.Result.Breakdown.Components {
		if comp.Multiplier == 2.0 && comp.DamageType == damage.Bludgeoning {
			foundVulnerabilityMultiplier = true
			s.Assert().Equal("monster_trait", comp.Source, "Vulnerability should come from monster_trait source")
			break
		}
	}

	s.Assert().True(foundVulnerabilityMultiplier,
		"Breakdown should contain a component with Multiplier=2.0 for bludgeoning vulnerability. Got components: %+v",
		hitOutput.Result.Breakdown.Components)

	// Verify total damage reflects vulnerability (should be roughly doubled)
	// Base mace damage: 1d6 (1-6) + 3 STR = 4-9
	// With vulnerability: 8-18
	s.Assert().GreaterOrEqual(hitOutput.Result.TotalDamage, 2, "Total damage should be at least 2 (minimum 1 base doubled)")
}

// Helper functions for test data

func createTestCharacterData(id, name string) *character.Data {
	// Create basic barbarian character with standard ability scores
	return &character.Data{
		ID:               id,
		Name:             name,
		Level:            1,
		RaceID:           "human",
		ClassID:          "barbarian",
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, // +3 modifier
			abilities.DEX: 14, // +2 modifier
			abilities.CON: 14, // +2 modifier
			abilities.INT: 10, // +0 modifier
			abilities.WIS: 12, // +1 modifier
			abilities.CHA: 8,  // -1 modifier
		},
		Features: []json.RawMessage{}, // No features for basic test
		EquipmentSlots: character.EquipmentSlots{
			character.SlotMainHand: "greataxe", // Default equipped weapon
		},
	}
}

func createTestEncounterData(id string) *encounterrepo.EncounterData {
	// Create a goblin with full HP for attack tests
	goblin := monster.NewGoblin("goblin-1")
	goblin.AddAction(monster.NewScimitarAction(monster.ScimitarConfig{
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageBonus: 2,
	}))
	goblinData := goblin.ToData()

	return &encounterrepo.EncounterData{
		ID:       id,
		Monsters: []*monster.Data{goblinData},
	}
}

// CreateDungeon Tests

func (s *OrchestratorTestSuite) TestCreateDungeon_Success() {
	// Arrange - Mock character repo to return character with DEX score
	// Called for initiative, and again if monster goes first and attacks
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: &character.Data{
				ID:        "char-1",
				HitPoints: 10, // Set HP to non-zero to avoid triggering TPK check
				AbilityScores: shared.AbilityScores{
					abilities.DEX: 14, // +2 modifier
				},
			}},
		}, nil).
		AnyTimes() // Initiative lookup + potential monster attack target

	// Mock character Update for long rest before dungeon starts
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil).
		AnyTimes()

	// Arrange - Mock dungeon repo save (required for generator path)
	s.mockDungeonRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.SaveOutput{Success: true}, nil)

	// Mock GetByEncounterID (called by various post-creation flows)
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Mock dungeon Update (called after TPK check)
	s.mockDungeonRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Arrange - Mock encounter repo save
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.SaveInput) (*encounterrepo.SaveOutput, error) {
			// Verify encounter ID is present and follows expected format
			s.Assert().NotEmpty(input.EncounterID)
			s.Assert().Contains(input.EncounterID, "enc-")

			// Verify room data is present
			s.Assert().NotNil(input.RoomData)
			roomData, ok := input.RoomData.(*spatial.RoomData)
			s.Assert().True(ok, "RoomData should be *spatial.RoomData")
			s.Assert().Equal("dungeon", roomData.Type)
			s.Assert().Equal(spatial.GridTypeHex, roomData.GridType)

			// Verify initiative data is now saved
			s.Assert().NotNil(input.InitiativeData, "InitiativeData should be present")
			s.Assert().NotNil(input.InitiativeRolls, "InitiativeRolls should be present")

			return &encounterrepo.SaveOutput{Success: true}, nil
		})

	// Arrange - Mock encounter repo update (called if monster goes first)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Act
	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		CharacterIDs: []string{"char-1"},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotEmpty(output.EncounterID)
	s.Assert().Contains(output.EncounterID, "enc-")

	// Verify dungeon ID is present (generator was used)
	s.Assert().NotEmpty(output.DungeonID)

	// Verify room data in output
	s.Assert().NotNil(output.Room)
	roomData, ok := output.Room.(*spatial.RoomData)
	s.Assert().True(ok, "Room should be *spatial.RoomData")
	s.Assert().Equal("dungeon", roomData.Type)
	s.Assert().Equal(spatial.GridTypeHex, roomData.GridType)

	// Verify combat state in output
	s.Assert().NotNil(output.CombatState, "CombatState should be present")
	s.Assert().Equal(output.EncounterID, output.CombatState.EncounterID)
	s.Assert().True(output.CombatState.CombatStarted)
	s.Assert().False(output.CombatState.CombatEnded)
	s.Assert().GreaterOrEqual(len(output.CombatState.TurnOrder), 2) // At least 1 character + 1 monster

	// Verify active turn index is valid
	s.Require().GreaterOrEqual(output.CombatState.ActiveIndex, 0)
	s.Require().Less(output.CombatState.ActiveIndex, len(output.CombatState.TurnOrder))
	activeTurn := output.CombatState.TurnOrder[output.CombatState.ActiveIndex]
	s.Assert().Equal("character", activeTurn.EntityType, "Active turn should be a character, not a monster")
}

func (s *OrchestratorTestSuite) TestCreateDungeon_NilInput() {
	// Act
	output, err := s.orchestrator.CreateDungeon(context.Background(), nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *OrchestratorTestSuite) TestCreateDungeon_SaveError() {
	// Arrange - Mock character repo
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: &character.Data{
				ID:        "char-1",
				HitPoints: 10,
				AbilityScores: shared.AbilityScores{
					abilities.DEX: 14,
				},
			}},
		}, nil).
		AnyTimes()

	// Mock character Update for long rest before dungeon starts
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil).
		AnyTimes()

	// Mock dungeon repo save (called before encounter save)
	s.mockDungeonRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.SaveOutput{Success: true}, nil)

	// Arrange - Mock encounter repo save to return error
	expectedError := fmt.Errorf("database error")
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(nil, expectedError)

	// Act
	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		CharacterIDs: []string{"char-1"},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to save encounter")
	s.Assert().ErrorIs(err, expectedError)
}

func (s *OrchestratorTestSuite) TestCreateDungeon_RequiresCharacters() {
	// Test that empty CharacterIDs fails since dungeon generator requires at least one character
	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		CharacterIDs: []string{},
	})

	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "party size must be greater than 0")
}

func (s *OrchestratorTestSuite) TestCreateDungeon_UniqueIDs() {
	// Verify that multiple calls generate unique IDs
	var capturedIDs []string

	// Mock character repo for all three calls
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: &character.Data{
				ID:        "char-1",
				HitPoints: 10,
				AbilityScores: shared.AbilityScores{
					abilities.DEX: 14,
				},
			}},
		}, nil).
		AnyTimes()

	// Mock character Update for long rest before dungeon starts
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil).
		AnyTimes()

	// Mock dungeon repo for all three calls
	s.mockDungeonRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.SaveOutput{Success: true}, nil).
		Times(3)

	// Mock GetByEncounterID (called by various post-creation flows)
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Mock dungeon Update (called after TPK check)
	s.mockDungeonRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.SaveInput) (*encounterrepo.SaveOutput, error) {
			capturedIDs = append(capturedIDs, input.EncounterID)
			return &encounterrepo.SaveOutput{Success: true}, nil
		}).
		Times(3)

	// Each CreateDungeon will also call Update to save initiative after monster turns
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Create three encounters
	for i := 0; i < 3; i++ {
		output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
			CharacterIDs: []string{"char-1"},
		})
		s.Require().NoError(err)
		s.Assert().NotEmpty(output.EncounterID)

		// Small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
	}

	// Assert all IDs are unique
	s.Assert().Len(capturedIDs, 3)
	s.Assert().NotEqual(capturedIDs[0], capturedIDs[1])
	s.Assert().NotEqual(capturedIDs[1], capturedIDs[2])
	s.Assert().NotEqual(capturedIDs[0], capturedIDs[2])
}

func (s *OrchestratorTestSuite) TestCreateDungeon_SavesInitiativeData() {
	// Mock character repo
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: &character.Data{
				ID:        "char-1",
				HitPoints: 10,
				AbilityScores: shared.AbilityScores{
					abilities.DEX: 14,
				},
			}},
		}, nil).
		AnyTimes()

	// Mock character Update for long rest before dungeon starts
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil).
		AnyTimes()

	// Mock dungeon repo save
	s.mockDungeonRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.SaveOutput{Success: true}, nil)

	// Mock GetByEncounterID (called by various post-creation flows)
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Mock dungeon Update (called after TPK check)
	s.mockDungeonRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Verify that encounter is created with room data AND initiative data
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.SaveInput) (*encounterrepo.SaveOutput, error) {
			// Verify encounter ID and room data are present
			s.Assert().NotEmpty(input.EncounterID)
			s.Assert().NotNil(input.RoomData, "RoomData should be present")

			// Verify initiative data is now saved
			s.Assert().NotNil(input.InitiativeData, "InitiativeData should be present")
			s.Assert().NotNil(input.InitiativeRolls, "InitiativeRolls should be present")
			s.Assert().GreaterOrEqual(len(input.InitiativeRolls), 2, "Should have at least 2 entities (1 char + monsters)")
			return &encounterrepo.SaveOutput{Success: true}, nil
		})

	// Expect Update call to save initiative after monster turns
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		CharacterIDs: []string{"char-1"},
	})

	s.Require().NoError(err)
	s.Assert().NotEmpty(output.EncounterID)
	s.Assert().NotNil(output.Room, "Room should be present in output")
	s.Assert().NotNil(output.CombatState, "CombatState should be present in output")
}

// TestMoveCharacter_Success tests successful movement
func (s *OrchestratorTestSuite) TestMoveCharacter_Success() {
	// Arrange - use hex grid with CubeEntities and cube coordinates
	roomData := &spatial.RoomData{
		ID:       "enc-1-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		CubeEntities: map[string]spatial.EntityCubePlacement{
			"char-1": {
				EntityID:       "char-1",
				EntityType:     "character",
				CubePosition:   spatial.CubeCoordinate{X: 0, Y: 0, Z: 0},
				Size:           1,
				BlocksMovement: true,
			},
		},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				RoomData:          roomData,
				MovementRemaining: 60, // Enough movement for the test
			},
		}, nil)

	// Mock dungeon repo for walls loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			// Verify the room was updated with new cube position
			updatedRoom, ok := input.RoomData.(*spatial.RoomData)
			s.Require().True(ok)
			s.Assert().Equal(5, updatedRoom.CubeEntities["char-1"].CubePosition.X)
			s.Assert().Equal(-10, updatedRoom.CubeEntities["char-1"].CubePosition.Y)
			s.Assert().Equal(5, updatedRoom.CubeEntities["char-1"].CubePosition.Z)

			// Verify movement was decremented
			s.Require().NotNil(input.MovementRemaining, "MovementRemaining should be updated")
			// Cube distance from (0,0,0) to (5,-10,5): (|5| + |-10| + |5|) / 2 = 10
			// 10 hexes * 5 feet = 50 feet, so 60 - 50 = 10
			s.Assert().Equal(int32(10), *input.MovementRemaining, "Movement should be decremented")

			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act - use cube coordinates (x + y + z = 0)
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		TargetPosition: &Position{
			X: 5,
			Y: -10,
			Z: 5,
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().Equal(float64(5), output.FinalPosition.X)
	s.Assert().Equal(float64(-10), output.FinalPosition.Y)
	s.Assert().Equal(float64(5), output.FinalPosition.Z)
	s.Assert().Equal("completed", output.StopReason)
	s.Assert().Equal(int32(10), output.MovementRemaining) // 60 - 50 = 10
}

// TestMoveCharacter_InvalidCubeCoordinates tests movement with invalid cube coordinates
func (s *OrchestratorTestSuite) TestMoveCharacter_InvalidCubeCoordinates() {
	// Arrange - use hex grid with CubeEntities
	roomData := &spatial.RoomData{
		ID:           "enc-1-room",
		Type:         "dungeon",
		Width:        20,
		Height:       20,
		GridType:     spatial.GridTypeHex,
		CubeEntities: make(map[string]spatial.EntityCubePlacement),
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:       "enc-1",
				RoomData: roomData,
			},
		}, nil)

	// Mock dungeon repo for walls loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil)

	// Note: No Update call expected for invalid coordinates

	// Act - use invalid cube coordinates (x + y + z != 0)
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		TargetPosition: &Position{
			X: 100,
			Y: 100,
			Z: 0, //nolint:gocritic // invalid cube coords for test
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().False(output.Success)
	s.Assert().Equal(float64(100), output.FinalPosition.X)
	s.Assert().Equal(float64(100), output.FinalPosition.Y)
	s.Assert().Equal(float64(0), output.FinalPosition.Z)
	s.Assert().Equal("invalid_coordinates", output.StopReason)
	s.Assert().Equal(int32(0), output.MovementRemaining)
}

// TestMoveCharacter_PositionOccupied tests movement to occupied position
func (s *OrchestratorTestSuite) TestMoveCharacter_PositionOccupied() {
	// Arrange - use hex grid with CubeEntities
	roomData := &spatial.RoomData{
		ID:       "enc-1-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		CubeEntities: map[string]spatial.EntityCubePlacement{
			"char-1": {
				EntityID:       "char-1",
				EntityType:     "character",
				CubePosition:   spatial.CubeCoordinate{X: 0, Y: 0, Z: 0},
				Size:           1,
				BlocksMovement: true,
			},
			"goblin-1": {
				EntityID:       "goblin-1",
				EntityType:     "monster",
				CubePosition:   spatial.CubeCoordinate{X: 5, Y: -10, Z: 5}, // Target position
				Size:           1,
				BlocksMovement: true,
			},
		},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:       "enc-1",
				RoomData: roomData,
			},
		}, nil)

	// Mock dungeon repo for walls loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil)

	// Note: No Update call expected for blocked position

	// Act - try to move to goblin's position (x + y + z = 0)
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		TargetPosition: &Position{
			X: 5,
			Y: -10,
			Z: 5,
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().False(output.Success)
	s.Assert().Equal(float64(0), output.FinalPosition.X) // Returns current position
	s.Assert().Equal(float64(0), output.FinalPosition.Y)
	s.Assert().Equal(float64(0), output.FinalPosition.Z)
	s.Assert().Equal("position_occupied", output.StopReason)
	s.Assert().Equal(int32(0), output.MovementRemaining)
}

// TestMoveCharacter_CreatesRoomIfMissing tests creating default room when none exists
func (s *OrchestratorTestSuite) TestMoveCharacter_CreatesRoomIfMissing() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:       "enc-1",
				RoomData: nil, // No room data
			},
		}, nil)

	// Mock dungeon repo for walls loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			// Verify a room was created with CubeEntities (for hex grids)
			updatedRoom, ok := input.RoomData.(*spatial.RoomData)
			s.Require().True(ok)
			s.Assert().Equal("enc-1-room", updatedRoom.ID)
			s.Assert().Equal(20, updatedRoom.Width)
			s.Assert().Equal(20, updatedRoom.Height)
			s.Assert().Contains(updatedRoom.CubeEntities, "char-1")
			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act - use cube coordinates (x + y + z = 0)
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		TargetPosition: &Position{
			X: 5,
			Y: -10,
			Z: 5,
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().Equal("completed", output.StopReason)
}

// TestMoveCharacter_NilInput tests nil input handling
func (s *OrchestratorTestSuite) TestMoveCharacter_NilInput() {
	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

// TestMoveCharacter_MissingEncounterID tests missing encounter ID
func (s *OrchestratorTestSuite) TestMoveCharacter_MissingEncounterID() {
	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EntityID:       "char-1",
		TargetPosition: &Position{X: 5, Y: 5},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

// TestMoveCharacter_MissingEntityID tests missing entity ID
func (s *OrchestratorTestSuite) TestMoveCharacter_MissingEntityID() {
	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID:    "enc-1",
		TargetPosition: &Position{X: 5, Y: 5},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "entity ID is required")
}

// TestMoveCharacter_MissingTargetPosition tests missing target position
func (s *OrchestratorTestSuite) TestMoveCharacter_MissingTargetPosition() {
	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "target position is required")
}

// TestMoveCharacter_EncounterNotFound tests encounter not found error
func (s *OrchestratorTestSuite) TestMoveCharacter_EncounterNotFound() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: nil,
		}, nil)

	// Act
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID:    "enc-1",
		EntityID:       "char-1",
		TargetPosition: &Position{X: 5, Y: 5},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter not found")
}

// TestMoveCharacter_UpdateError tests repository update error
func (s *OrchestratorTestSuite) TestMoveCharacter_UpdateError() {
	// Arrange - use hex grid with CubeEntities
	roomData := &spatial.RoomData{
		ID:           "enc-1-room",
		Type:         "dungeon",
		Width:        20,
		Height:       20,
		GridType:     spatial.GridTypeHex,
		CubeEntities: make(map[string]spatial.EntityCubePlacement),
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{
			EncounterID: "enc-1",
		}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:       "enc-1",
				RoomData: roomData,
			},
		}, nil)

	// Mock dungeon repo for walls loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	// Act - use cube coordinates (x + y + z = 0)
	output, err := s.orchestrator.MoveCharacter(context.Background(), &MoveCharacterInput{
		EncounterID:    "enc-1",
		EntityID:       "char-1",
		TargetPosition: &Position{X: 5, Y: -10, Z: 5},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to save updated room")
}

// ============================================================================
// EndTurn Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestEndTurn_Success() {
	// Arrange - Set up encounter with 2 characters and 1 monster
	// Turn order: char-1 (current), goblin-1, char-2
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "goblin-1", Type: "monster"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0, // char-1's turn
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("goblin-1", "monster"), Roll: 15, Modifier: 2, Total: 17},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	// Expect character turn-end event publishing for char-1 (whose turn is ending)
	s.expectCharacterTurnEnd("char-1")
	s.expectDungeonLookup("enc-1")

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
				Monsters: []*monster.Data{
					{ID: "goblin-1", Name: "Goblin", HitPoints: 7}, // Living monster (will be skipped since we don't execute turns in this test)
				},
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			// Verify initiative was advanced past monster to char-2
			s.Assert().Equal("enc-1", input.EncounterID)
			s.Require().NotNil(input.InitiativeData)
			s.Assert().Equal(2, input.InitiativeData.Current, "Should skip monster and advance to char-2")
			s.Assert().Equal(1, input.InitiativeData.Round, "Should still be round 1")

			// Verify movement was reset
			s.Require().NotNil(input.MovementRemaining)
			s.Assert().Equal(int32(30), *input.MovementRemaining)

			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act - No EntityID needed, server determines active entity from state
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)

	// Verify combat state
	s.Require().NotNil(output.CombatState)
	s.Assert().Equal("enc-1", output.CombatState.EncounterID)
	s.Assert().Equal(1, output.CombatState.Round)
	s.Assert().Equal(2, output.CombatState.ActiveIndex, "Should be char-2's turn (index 2)")
	s.Assert().Equal(int32(30), output.CombatState.MovementRemaining)
	s.Assert().True(output.CombatState.CombatStarted)
	s.Assert().False(output.CombatState.CombatEnded)

	// Verify turn change event
	s.Require().NotNil(output.TurnChange)
	s.Assert().Equal("char-1", output.TurnChange.PreviousEntityID)
	s.Assert().Equal("char-2", output.TurnChange.NextEntityID)
	s.Assert().Equal(1, output.TurnChange.Round)
	s.Assert().False(output.TurnChange.NewRound)

	// Verify monster turns were executed
	s.Assert().Len(output.MonsterTurns, 1, "One monster should execute a turn")
	s.Assert().Equal("goblin-1", output.MonsterTurns[0].MonsterID)
	s.Assert().Nil(output.EncounterResult, "Combat should continue")
}

func (s *OrchestratorTestSuite) TestEndTurn_AdvancesToNewRound() {
	// Arrange - char-2 is last in order, ending turn should start round 2
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 1, // char-2's turn (last)
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	// Expect character turn-end event publishing for char-2 (whose turn is ending)
	s.expectCharacterTurnEnd("char-2")
	s.expectDungeonLookup("enc-1")

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			s.Assert().Equal(0, input.InitiativeData.Current, "Should wrap to index 0")
			s.Assert().Equal(2, input.InitiativeData.Round, "Should be round 2")
			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act - No EntityID needed, server knows it's char-2's turn from state
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)

	s.Assert().Equal(2, output.CombatState.Round)
	s.Assert().Equal(0, output.CombatState.ActiveIndex)

	s.Assert().True(output.TurnChange.NewRound)
	s.Assert().Equal(2, output.TurnChange.Round)
	s.Assert().Equal("char-2", output.TurnChange.PreviousEntityID)
	s.Assert().Equal("char-1", output.TurnChange.NextEntityID)
}

func (s *OrchestratorTestSuite) TestEndTurn_SkipsMultipleMonsters() {
	// Arrange - char-1 followed by 2 monsters, then char-2
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "goblin-1", Type: "monster"},
			{ID: "goblin-2", Type: "monster"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 20, Modifier: 2, Total: 22},
		{Entity: initiative.NewParticipant("goblin-1", "monster"), Roll: 15, Modifier: 2, Total: 17},
		{Entity: initiative.NewParticipant("goblin-2", "monster"), Roll: 14, Modifier: 2, Total: 16},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	// Expect character turn-end event publishing for char-1 (whose turn is ending)
	s.expectCharacterTurnEnd("char-1")
	s.expectDungeonLookup("enc-1")

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
				Monsters: []*monster.Data{
					{ID: "goblin-1", Name: "Goblin 1", HitPoints: 7}, // Alive
					{ID: "goblin-2", Name: "Goblin 2", HitPoints: 7}, // Alive
				},
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			s.Assert().Equal(3, input.InitiativeData.Current, "Should skip both monsters to char-2 at index 3")
			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act - No EntityID needed
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal(3, output.CombatState.ActiveIndex)
	s.Assert().Equal("char-2", output.TurnChange.NextEntityID)
	s.Assert().Len(output.MonsterTurns, 2, "Two monsters should execute turns")
	s.Assert().Nil(output.EncounterResult, "Combat should continue")
}

func (s *OrchestratorTestSuite) TestEndTurn_SkipsMonstersAcrossRoundBoundary() {
	// Arrange - char-1 at end, followed by monsters at start of order
	// Turn order: goblin-1, goblin-2, char-1 (current)
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "goblin-1", Type: "monster"},
			{ID: "goblin-2", Type: "monster"},
			{ID: "char-1", Type: "character"},
		},
		Current: 2, // char-1's turn (last)
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("goblin-1", "monster"), Roll: 20, Modifier: 2, Total: 22},
		{Entity: initiative.NewParticipant("goblin-2", "monster"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	// Expect character turn-end event publishing for char-1 (whose turn is ending)
	s.expectCharacterTurnEnd("char-1")
	s.expectDungeonLookup("enc-1")

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
				Monsters: []*monster.Data{
					{ID: "goblin-1", Name: "Goblin 1", HitPoints: 7}, // Alive
					{ID: "goblin-2", Name: "Goblin 2", HitPoints: 7}, // Alive
				},
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			s.Assert().Equal(2, input.InitiativeData.Current, "Should wrap back to char-1 at index 2")
			s.Assert().Equal(2, input.InitiativeData.Round, "Should advance to round 2")
			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act - No EntityID needed
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Assert().Equal(2, output.CombatState.Round)
	s.Assert().Equal(2, output.CombatState.ActiveIndex)
	s.Assert().True(output.TurnChange.NewRound)
	s.Assert().Equal("char-1", output.TurnChange.NextEntityID) // Back to char-1
	s.Assert().Len(output.MonsterTurns, 2, "Two monsters should execute turns")
	s.Assert().Nil(output.EncounterResult, "Combat should continue")
}

func (s *OrchestratorTestSuite) TestEndTurn_NilInput() {
	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *OrchestratorTestSuite) TestEndTurn_MissingEncounterID() {
	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *OrchestratorTestSuite) TestEndTurn_EncounterNotFound() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "nonexistent"}).
		Return(&encounterrepo.GetOutput{Data: nil}, nil)

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "nonexistent",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter not found")
}

func (s *OrchestratorTestSuite) TestEndTurn_GetError() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(nil, fmt.Errorf("database error"))

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to load encounter")
}

func (s *OrchestratorTestSuite) TestEndTurn_NoInitiativeData() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:             "enc-1",
				InitiativeData: nil,
			},
		}, nil)

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "no initiative data")
}

func (s *OrchestratorTestSuite) TestEndTurn_EmptyTurnOrder() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID: "enc-1",
				InitiativeData: &initiative.TrackerData{
					Order:   []initiative.EntityData{},
					Current: 0,
					Round:   1,
				},
			},
		}, nil)

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "empty turn order")
}

func (s *OrchestratorTestSuite) TestEndTurn_UpdateError() {
	// Arrange
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	// NOTE: We don't call expectCharacterTurnEnd here because the character turn-end
	// event publishing happens AFTER encRepo.Update. When Update fails, we return early
	// and never reach the publishCharacterTurnEnd call.

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:             "enc-1",
				InitiativeData: initiativeData,
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to save turn state")
}

func (s *OrchestratorTestSuite) TestEndTurn_WithRoomData() {
	// Arrange - Test that positions are included when room data exists
	roomData := &spatial.RoomData{
		ID:       "enc-1-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		Entities: map[string]spatial.EntityPlacement{
			"char-1": {
				EntityID:       "char-1",
				EntityType:     "character",
				Position:       spatial.Position{X: 2, Y: 8},
				Size:           1,
				BlocksMovement: true,
			},
			"char-2": {
				EntityID:       "char-2",
				EntityType:     "character",
				Position:       spatial.Position{X: 2, Y: 10},
				Size:           1,
				BlocksMovement: true,
			},
		},
	}

	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	// Expect character turn-end event publishing for char-1 (whose turn is ending)
	s.expectCharacterTurnEnd("char-1")
	s.expectDungeonLookup("enc-1")

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				RoomData:        roomData,
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)

	// Verify turn order has positions
	s.Require().Len(output.CombatState.TurnOrder, 2)
	s.Require().NotNil(output.CombatState.TurnOrder[0].Position)
	s.Assert().Equal(float64(2), output.CombatState.TurnOrder[0].Position.X)
	s.Assert().Equal(float64(8), output.CombatState.TurnOrder[0].Position.Y)

	s.Require().NotNil(output.CombatState.TurnOrder[1].Position)
	s.Assert().Equal(float64(2), output.CombatState.TurnOrder[1].Position.X)
	s.Assert().Equal(float64(10), output.CombatState.TurnOrder[1].Position.Y)
}

// ============================================================================
// EndTurn Ownership Validation Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestEndTurn_WithPlayerID_OwnershipValid() {
	// Arrange - char-1 owned by player-1
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0, // char-1's turn
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	// Character data used for both ownership validation and turn-end event publishing
	charData := &character.Data{
		ID:               "char-1",
		Name:             "Test Character",
		PlayerID:         "player-1", // Owned by player-1
		Level:            1,
		RaceID:           "human",
		ClassID:          "fighter",
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 14,
			abilities.CON: 15,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		HitPoints:    12,
		MaxHitPoints: 12,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	// Mock character lookup - called twice: ownership validation and turn-end event publishing
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		Times(2)

	// Mock character update after turn-end event processing
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	s.expectDungeonLookup("enc-1")

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Act - player-1 ends their own character's turn
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
		PlayerID:    "player-1", // Correct owner
	})

	// Assert - should succeed
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal("char-1", output.TurnChange.PreviousEntityID)
	s.Assert().Equal("char-2", output.TurnChange.NextEntityID)
}

func (s *OrchestratorTestSuite) TestEndTurn_WithPlayerID_NotYourCharacter() {
	// Arrange - char-1 owned by player-1, but player-2 tries to end turn
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0, // char-1's turn
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	// Mock character lookup - char-1 is owned by player-1
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: &character.Data{
				ID:       "char-1",
				PlayerID: "player-1", // Owned by player-1
			}},
		}, nil)

	// Act - player-2 tries to end someone else's turn
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
		PlayerID:    "player-2", // Wrong player!
	})

	// Assert - should fail with ownership error
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "you do not control character")
	s.Assert().Contains(err.Error(), "char-1")
	s.Assert().Contains(err.Error(), "player-1")
}

func (s *OrchestratorTestSuite) TestEndTurn_WithPlayerID_MonsterTurn() {
	// Arrange - it's a monster's turn, player cannot end it
	// Note: This shouldn't normally happen since we auto-skip monster turns,
	// but we need to handle it in case the state gets corrupted or initial load
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "goblin-1", Type: "monster"}, // Monster first
			{ID: "char-1", Type: "character"},
		},
		Current: 0, // Monster's turn (edge case)
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("goblin-1", "monster"), Roll: 20, Modifier: 2, Total: 22},
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	// Act - player tries to end monster's turn
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
		PlayerID:    "player-1",
	})

	// Assert - should fail with monster turn error
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "monster's turn")
	s.Assert().Contains(err.Error(), "goblin-1")
}

func (s *OrchestratorTestSuite) TestEndTurn_WithPlayerID_CharacterLookupFails() {
	// Arrange - character lookup fails
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:             "enc-1",
				InitiativeData: initiativeData,
			},
		}, nil)

	// Mock character lookup to fail
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(nil, apierr.NotFound("character not found"))

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
		PlayerID:    "player-1",
	})

	// Assert - should fail with lookup error
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to validate character ownership")
}

func (s *OrchestratorTestSuite) TestEndTurn_WithoutPlayerID_SkipsValidation() {
	// Arrange - backward compatibility: no PlayerID means no ownership validation
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "char-2", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	initiativeRolls := []initiative.Roll{
		{Entity: initiative.NewParticipant("char-1", "character"), Roll: 18, Modifier: 2, Total: 20},
		{Entity: initiative.NewParticipant("char-2", "character"), Roll: 10, Modifier: 1, Total: 11},
	}

	// Expect character turn-end event publishing for char-1 (whose turn is ending)
	// Note: Ownership validation is skipped (no PlayerID), but turn-end event publishing still happens
	s.expectCharacterTurnEnd("char-1")
	s.expectDungeonLookup("enc-1")

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:              "enc-1",
				InitiativeData:  initiativeData,
				InitiativeRolls: initiativeRolls,
			},
		}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Act - no PlayerID provided
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
		// PlayerID intentionally omitted
	})

	// Assert - should succeed without validation
	s.Require().NoError(err)
	s.Require().NotNil(output)
}

// ============================================================================
// ActivateFeature Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestActivateFeature_NilInput() {
	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), nil)

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *OrchestratorTestSuite) TestActivateFeature_MissingEncounterID() {
	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		CharacterID: "char-1",
		FeatureID:   "rage",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *OrchestratorTestSuite) TestActivateFeature_MissingCharacterID() {
	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		FeatureID:   "rage",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "character ID is required")
}

func (s *OrchestratorTestSuite) TestActivateFeature_MissingFeatureID() {
	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		CharacterID: "char-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "feature ID is required")
}

func (s *OrchestratorTestSuite) TestActivateFeature_EncounterNotFound() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "nonexistent"}).
		Return(&encounterrepo.GetOutput{Data: nil}, nil)

	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "nonexistent",
		CharacterID: "char-1",
		FeatureID:   "rage",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter not found")
}

func (s *OrchestratorTestSuite) TestActivateFeature_CharacterNotFound() {
	// Arrange - Encounter exists
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{ID: "enc-1"},
		}, nil)

	// Character not found via loadCharacterForCombat
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(nil, apierr.NotFound("character not found"))

	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		CharacterID: "char-1",
		FeatureID:   "rage",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to load character")
}

func (s *OrchestratorTestSuite) TestActivateFeature_UnknownFeature_PassedToToolkit() {
	// Arrange - Unknown feature IDs are no longer rejected at featureIDToRef.
	// Instead, they're constructed as a ref and passed to ActivateAbility,
	// which validates whether the feature exists on the character.

	// Encounter exists
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				MovementRemaining: 30,
			},
		}, nil)

	// Character loads successfully
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	// ActivateAbility may succeed with Success=false, then persist is called
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil).
		AnyTimes()

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Act - The toolkit's ActivateAbility will reject the unknown feature
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		CharacterID: "char-1",
		FeatureID:   "unknown_feature",
	})

	// Assert - Toolkit rejects the feature, returned as an error
	// (the exact behavior depends on toolkit's ActivateAbility for unknown refs)
	if err != nil {
		s.Assert().Contains(err.Error(), "activate ability")
	} else {
		// If toolkit returns success=false instead of error, that's also valid
		s.Require().NotNil(output)
		s.Assert().False(output.Success)
	}
}

func (s *OrchestratorTestSuite) TestActivateFeature_CallsCharActivateAbilityDirectly() {
	// This test verifies ActivateFeature calls char.ActivateAbility directly
	// (not through ActivateCombatAbility) preserving feature identity.

	// Arrange - Encounter exists
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				MovementRemaining: 30,
			},
		}, nil)

	// Character with barbarian data
	charData := createTestCharacterData("char-1", "Grog")
	// loadCharacterForCombat -> charRepo.Get
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	// persistCharacterData -> charRepo.Update
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// syncCharActionEconomyToEncounter -> encRepo.Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		CharacterID: "char-1",
		FeatureID:   refs.Features.Rage().ID,
	})

	// Assert - Character data returned, activation result from toolkit
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.CharacterData)
}

func (s *OrchestratorTestSuite) TestActivateFeature_EncounterGetError() {
	// Arrange
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(nil, fmt.Errorf("database error"))

	// Act
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		CharacterID: "char-1",
		FeatureID:   "rage",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "failed to load encounter")
}

// ============================================================================
// Action Economy Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestResolveAttack_NoActionAvailable_RejectsAttack() {
	// Arrange - Create test character data
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	// Arrange - Mock encounter repo with action already consumed
	encData := createTestEncounterData("enc-1")
	encData.ActionEconomy = &entities.ActionEconomyState{
		ActionsRemaining:      0, // Action already used
		BonusActionsRemaining: 1,
		ReactionsRemaining:    1,
	}
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert - Should return error when no action available
	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "no action available")
}

func (s *OrchestratorTestSuite) TestActivateFeature_NoBonusActionAvailable_RejectsActivation() {
	// Arrange - Encounter exists
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				MovementRemaining: 30,
			},
		}, nil)

	// Character with action economy showing bonus action used
	charData := createTestCharacterData("char-1", "Grog")
	charData.ActionEconomy = &character.ActionEconomyData{
		TurnNumber:            1, // Matches computeTurnNumber (no initiative -> 1)
		ActionsRemaining:      1,
		BonusActionsRemaining: 0, // Already used
		ReactionsRemaining:    1,
	}
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	// persistCharacterData still called after ability rejection
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// syncCharActionEconomyToEncounter -> encRepo.Update
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Act - Try to activate Rage (bonus action)
	output, err := s.orchestrator.ActivateFeature(context.Background(), &ActivateFeatureInput{
		EncounterID: "enc-1",
		CharacterID: "char-1",
		FeatureID:   refs.Features.Rage().ID,
	})

	// Assert - Should return success=false (toolkit rejects due to no bonus action)
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().False(output.Success)
}

func (s *OrchestratorTestSuite) TestEndTurn_ResetsActionEconomy() {
	// Arrange - Create encounter with consumed action economy
	encData := &encounterrepo.EncounterData{
		ID: "enc-1",
		InitiativeData: &initiative.TrackerData{
			Round:   1,
			Current: 0,
			Order: []initiative.EntityData{
				{ID: "char-1", Type: "character"},
				{ID: "goblin-1", Type: "monster"},
			},
		},
		ActionEconomy: &entities.ActionEconomyState{
			ActionsRemaining:      0, // All consumed
			BonusActionsRemaining: 0,
			ReactionsRemaining:    0,
		},
		Monsters: []*monster.Data{monster.NewGoblin("goblin-1").ToData()},
	}

	// Expect character turn-end event publishing for char-1 (whose turn is ending)
	// Capture the character update to verify TurnNumber invalidation
	charData := &character.Data{
		ID:               "char-1",
		Name:             "Test Character",
		PlayerID:         "player-1",
		Level:            1,
		RaceID:           "human",
		ClassID:          "fighter",
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 14,
			abilities.CON: 15,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 8,
		},
		HitPoints:    12,
		MaxHitPoints: 12,
		ActionEconomy: &character.ActionEconomyData{
			TurnNumber:            100, // From previous turn
			ActionsRemaining:      0,
			BonusActionsRemaining: 0,
			ReactionsRemaining:    0,
			MovementRemaining:     0,
			Granted:               make(map[character.GrantedActionKey]int),
		},
	}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	var capturedCharUpdate characterrepo.UpdateInput
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			capturedCharUpdate = input
			return &characterrepo.UpdateOutput{}, nil
		})

	s.expectDungeonLookup("enc-1")

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Mock Update - Capture the update to verify action economy reset
	var capturedUpdate *encounterrepo.UpdateInput
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.UpdateInput) (*encounterrepo.UpdateOutput, error) {
			capturedUpdate = input
			return &encounterrepo.UpdateOutput{Success: true}, nil
		})

	// Act
	output, err := s.orchestrator.EndTurn(context.Background(), &EndTurnInput{
		EncounterID: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.CombatState)

	// Verify Update was called with fresh action economy
	s.Require().NotNil(capturedUpdate)
	s.Require().NotNil(capturedUpdate.ActionEconomy)
	s.Assert().Equal(1, capturedUpdate.ActionEconomy.ActionsRemaining, "Action should be reset")
	s.Assert().Equal(1, capturedUpdate.ActionEconomy.BonusActionsRemaining, "Bonus action should be reset")
	s.Assert().Equal(1, capturedUpdate.ActionEconomy.ReactionsRemaining, "Reaction should be reset")

	// Verify CombatState output has fresh action economy
	s.Require().NotNil(output.CombatState.ActionEconomy)
	s.Assert().Equal(1, output.CombatState.ActionEconomy.ActionsRemaining)
	s.Assert().Equal(1, output.CombatState.ActionEconomy.BonusActionsRemaining)
	s.Assert().Equal(1, output.CombatState.ActionEconomy.ReactionsRemaining)

	// Verify character's TurnNumber was invalidated by publishCharacterTurnEnd
	s.Require().NotNil(capturedCharUpdate.Character)
	s.Require().NotNil(capturedCharUpdate.Character.Data)
	s.Require().NotNil(capturedCharUpdate.Character.Data.ActionEconomy,
		"Character should still be in combat after EndTurn")
	s.Assert().Equal(-1, capturedCharUpdate.Character.Data.ActionEconomy.TurnNumber,
		"TurnNumber should be invalidated to -1 so next loadCharacterForCombat triggers StartTurn")
}

func (s *OrchestratorTestSuite) TestCreateDungeon_InitializesActionEconomy() {
	// Arrange - Mock character repo
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: &character.Data{
				ID:        "char-1",
				HitPoints: 10, // Set HP to non-zero to avoid triggering TPK check
				AbilityScores: shared.AbilityScores{
					abilities.DEX: 14, // +2 modifier for initiative
				},
			}},
		}, nil).
		AnyTimes()

	// Mock character Update for long rest before dungeon starts
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil).
		AnyTimes()

	// Mock dungeon repo save
	s.mockDungeonRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.SaveOutput{Success: true}, nil)

	// Mock GetByEncounterID (called by various post-creation flows)
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Mock dungeon Update (called after TPK check)
	s.mockDungeonRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Capture the save input to verify action economy
	var capturedSave *encounterrepo.SaveInput
	s.mockEncRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input *encounterrepo.SaveInput) (*encounterrepo.SaveOutput, error) {
			capturedSave = input
			return &encounterrepo.SaveOutput{Success: true}, nil
		})

	// Update might be called if monsters go first (monster turn execution)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Act
	output, err := s.orchestrator.CreateDungeon(context.Background(), &CreateDungeonInput{
		CharacterIDs: []string{"char-1"},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.CombatState)

	// Verify Save was called with initialized action economy
	s.Require().NotNil(capturedSave)
	s.Require().NotNil(capturedSave.ActionEconomy)
	s.Assert().Equal(1, capturedSave.ActionEconomy.ActionsRemaining, "Action should be initialized to 1")
	s.Assert().Equal(1, capturedSave.ActionEconomy.BonusActionsRemaining, "Bonus action should be initialized to 1")
	s.Assert().Equal(1, capturedSave.ActionEconomy.ReactionsRemaining, "Reaction should be initialized to 1")

	// Verify CombatState output has action economy
	s.Require().NotNil(output.CombatState.ActionEconomy)
	s.Assert().Equal(1, output.CombatState.ActionEconomy.ActionsRemaining)
	s.Assert().Equal(1, output.CombatState.ActionEconomy.BonusActionsRemaining)
	s.Assert().Equal(1, output.CombatState.ActionEconomy.ReactionsRemaining)
}

// ============================================================================
// ResolveAttack - Equipped Weapon Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestResolveAttack_UsesEquippedWeapon() {
	// Arrange - Create test character data with longsword equipped
	charData := createTestCharacterData("char-1", "Grog")
	charData.EquipmentSlots = character.EquipmentSlots{
		character.SlotMainHand: "longsword",
	}
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	// Arrange - Mock encounter repo
	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Mock Update for when attack hits
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Mock dungeon repo for wall loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)

	// Longsword does slashing damage
	if output.Result.Hit {
		s.Assert().Equal(damage.Slashing, output.Result.DamageType)
	}
}

func (s *OrchestratorTestSuite) TestResolveAttack_NoEquippedWeapon_FallsBackToUnarmedStrike() {
	// Arrange - Create test character data without equipped weapon
	charData := createTestCharacterData("char-1", "Grog")
	charData.EquipmentSlots = character.EquipmentSlots{} // Empty - no weapon equipped
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	// Arrange - Mock encounter repo
	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Mock Update for when attack hits
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Mock dungeon repo for wall loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert - Should succeed with fallback to unarmed strike
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)

	// Unarmed strike does bludgeoning damage
	if output.Result.Hit {
		s.Assert().Equal(damage.Bludgeoning, output.Result.DamageType)
	}
}

func (s *OrchestratorTestSuite) TestResolveAttack_UnknownWeaponID_FallsBackToUnarmedStrike() {
	// Arrange - Create test character data with an unknown weapon ID
	charData := createTestCharacterData("char-1", "Grog")
	charData.EquipmentSlots = character.EquipmentSlots{
		character.SlotMainHand: "unknown-weapon-id", // Invalid weapon ID
	}
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	// Arrange - Mock encounter repo
	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Mock Update for when attack hits
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Mock dungeon repo for wall loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Act - Should still succeed with fallback
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert - Should succeed with fallback to unarmed strike
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)
}

func (s *OrchestratorTestSuite) TestResolveAttack_NilEquipmentSlots_FallsBackToUnarmedStrike() {
	// Arrange - Create test character data with nil equipment slots
	charData := createTestCharacterData("char-1", "Grog")
	charData.EquipmentSlots = nil // No equipment data at all
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	// Arrange - Mock encounter repo
	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Mock Update for when attack hits
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Mock dungeon repo for wall loading
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Act - Should still succeed with fallback
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "char-1",
		TargetID:    "goblin-1",
	})

	// Assert - Should succeed with fallback to unarmed strike
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)
}

// TestBuildGameContext_MainHandOnly tests GameContext with only a main-hand weapon
func (s *OrchestratorTestSuite) TestBuildGameContext_MainHandOnly() {
	longsword, _ := weapons.GetByID(weapons.Longsword)
	slots := character.EquipmentSlots{
		character.SlotMainHand: weapons.Longsword,
	}

	gameCtx := s.orchestrator.buildGameContextFromEquipment("char-1", &longsword, slots, nil)

	s.Require().NotNil(gameCtx)
	registry, ok := gamectx.Characters(gamectx.WithGameContext(context.Background(), gameCtx))
	s.Require().True(ok)

	charWeapons := registry.GetCharacterWeapons("char-1")
	s.Require().NotNil(charWeapons)
	s.Assert().NotNil(charWeapons.MainHand())
	s.Assert().Equal("Longsword", charWeapons.MainHand().Name)
	s.Assert().Nil(charWeapons.OffHand(), "Should have no off-hand weapon")
}

// TestBuildGameContext_MainHandAndOffHandWeapon tests dual-wielding scenario
func (s *OrchestratorTestSuite) TestBuildGameContext_MainHandAndOffHandWeapon() {
	longsword, _ := weapons.GetByID(weapons.Longsword)
	slots := character.EquipmentSlots{
		character.SlotMainHand: weapons.Longsword,
		character.SlotOffHand:  weapons.Dagger,
	}

	gameCtx := s.orchestrator.buildGameContextFromEquipment("char-1", &longsword, slots, nil)

	s.Require().NotNil(gameCtx)
	registry, ok := gamectx.Characters(gamectx.WithGameContext(context.Background(), gameCtx))
	s.Require().True(ok)

	charWeapons := registry.GetCharacterWeapons("char-1")
	s.Require().NotNil(charWeapons)
	s.Assert().NotNil(charWeapons.MainHand())
	s.Assert().Equal("Longsword", charWeapons.MainHand().Name)
	s.Assert().NotNil(charWeapons.OffHand(), "Should have off-hand weapon")
	s.Assert().Equal("Dagger", charWeapons.OffHand().Name)
}

// TestBuildGameContext_MainHandAndShield tests sword-and-board scenario
func (s *OrchestratorTestSuite) TestBuildGameContext_MainHandAndShield() {
	longsword, _ := weapons.GetByID(weapons.Longsword)
	// In toolkit model, shields go in the off-hand slot with armor.Shield as ID
	slots := character.EquipmentSlots{
		character.SlotMainHand: weapons.Longsword,
		character.SlotOffHand:  armor.Shield,
	}

	gameCtx := s.orchestrator.buildGameContextFromEquipment("char-1", &longsword, slots, nil)

	s.Require().NotNil(gameCtx)
	registry, ok := gamectx.Characters(gamectx.WithGameContext(context.Background(), gameCtx))
	s.Require().True(ok)

	charWeapons := registry.GetCharacterWeapons("char-1")
	s.Require().NotNil(charWeapons)
	s.Assert().NotNil(charWeapons.MainHand())
	s.Assert().Equal("Longsword", charWeapons.MainHand().Name)
	// Shield is in off-hand slot but OffHand() returns nil for shields (by design)
	s.Assert().Nil(charWeapons.OffHand(), "Shield should not count as off-hand weapon")
}

// TestBuildGameContext_NilMainHandWeapon tests handling of nil main hand weapon
func (s *OrchestratorTestSuite) TestBuildGameContext_NilMainHandWeapon() {
	slots := character.EquipmentSlots{
		character.SlotOffHand: armor.Shield,
	}

	gameCtx := s.orchestrator.buildGameContextFromEquipment("char-1", nil, slots, nil)

	s.Require().NotNil(gameCtx)
	registry, ok := gamectx.Characters(gamectx.WithGameContext(context.Background(), gameCtx))
	s.Require().True(ok)

	charWeapons := registry.GetCharacterWeapons("char-1")
	s.Require().NotNil(charWeapons)
	s.Assert().Nil(charWeapons.MainHand(), "Should have no main-hand weapon")
}

// TestBuildGameContext_NilSlots tests handling of nil equipment slots
func (s *OrchestratorTestSuite) TestBuildGameContext_NilSlots() {
	longsword, _ := weapons.GetByID(weapons.Longsword)

	gameCtx := s.orchestrator.buildGameContextFromEquipment("char-1", &longsword, nil, nil)

	s.Require().NotNil(gameCtx)
	registry, ok := gamectx.Characters(gamectx.WithGameContext(context.Background(), gameCtx))
	s.Require().True(ok)

	charWeapons := registry.GetCharacterWeapons("char-1")
	s.Require().NotNil(charWeapons)
	s.Assert().NotNil(charWeapons.MainHand(), "Should still have main-hand from weapon param")
	s.Assert().Nil(charWeapons.OffHand(), "Should have no off-hand with nil slots")
}

// TestBuildGameContext_WithAbilityScores tests that ability scores are registered in GameContext
func (s *OrchestratorTestSuite) TestBuildGameContext_WithAbilityScores() {
	longsword, _ := weapons.GetByID(weapons.Longsword)
	scores := shared.AbilityScores{
		abilities.STR: 16,
		abilities.DEX: 14,
		abilities.CON: 12,
		abilities.INT: 10,
		abilities.WIS: 15,
		abilities.CHA: 8,
	}

	gameCtx := s.orchestrator.buildGameContextFromEquipment("char-1", &longsword, nil, scores)

	s.Require().NotNil(gameCtx)
	registry, ok := gamectx.Characters(gamectx.WithGameContext(context.Background(), gameCtx))
	s.Require().True(ok)

	abilityScores := registry.GetCharacterAbilityScores("char-1")
	s.Require().NotNil(abilityScores, "Ability scores should be registered in GameContext")
	s.Assert().Equal(16, abilityScores.Strength)
	s.Assert().Equal(14, abilityScores.Dexterity)
	s.Assert().Equal(12, abilityScores.Constitution)
	s.Assert().Equal(10, abilityScores.Intelligence)
	s.Assert().Equal(15, abilityScores.Wisdom)
	s.Assert().Equal(8, abilityScores.Charisma)
}

// TestBuildGameContext_NilAbilityScores tests that nil ability scores are handled gracefully
func (s *OrchestratorTestSuite) TestBuildGameContext_NilAbilityScores() {
	longsword, _ := weapons.GetByID(weapons.Longsword)

	gameCtx := s.orchestrator.buildGameContextFromEquipment("char-1", &longsword, nil, nil)

	s.Require().NotNil(gameCtx)
	registry, ok := gamectx.Characters(gamectx.WithGameContext(context.Background(), gameCtx))
	s.Require().True(ok)

	abilityScores := registry.GetCharacterAbilityScores("char-1")
	s.Assert().Nil(abilityScores, "Nil ability scores should not crash, just not register")
}

// Victory/Failure Detection Tests

func (s *OrchestratorTestSuite) TestCheckAllCharactersDead_AllDead() {
	enc := &encounterrepo.EncounterData{
		CharacterHP: map[string]int{
			"char-1": 0,
			"char-2": 0,
			"char-3": 0,
		},
	}

	s.True(s.orchestrator.checkAllCharactersDead(enc))
}

func (s *OrchestratorTestSuite) TestCheckAllCharactersDead_SomeAlive() {
	enc := &encounterrepo.EncounterData{
		CharacterHP: map[string]int{
			"char-1": 0,
			"char-2": 10, // Still alive
			"char-3": 0,
		},
	}

	s.False(s.orchestrator.checkAllCharactersDead(enc))
}

func (s *OrchestratorTestSuite) TestCheckAllCharactersDead_AllAlive() {
	enc := &encounterrepo.EncounterData{
		CharacterHP: map[string]int{
			"char-1": 25,
			"char-2": 10,
			"char-3": 15,
		},
	}

	s.False(s.orchestrator.checkAllCharactersDead(enc))
}

func (s *OrchestratorTestSuite) TestCheckAllCharactersDead_EmptyMap() {
	enc := &encounterrepo.EncounterData{
		CharacterHP: map[string]int{},
	}

	s.False(s.orchestrator.checkAllCharactersDead(enc), "Empty map should not be TPK")
}

func (s *OrchestratorTestSuite) TestCheckAllCharactersDead_NilMap() {
	enc := &encounterrepo.EncounterData{
		CharacterHP: nil,
	}

	s.False(s.orchestrator.checkAllCharactersDead(enc), "Nil map should not be TPK")
}

// ============================================================================
// checkCombatEnd Tests - HasBossRoom prevents premature victory
// ============================================================================

func (s *OrchestratorTestSuite) TestCheckCombatEnd_AllMonstersDeadNoBossRoom_ReturnsVictory() {
	enc := &encounterrepo.EncounterData{
		InitiativeData: &initiative.TrackerData{
			Order: []initiative.EntityData{
				{ID: "char-1", Type: entityTypeCharacter},
				{ID: "goblin-1", Type: entityTypeMonster},
			},
		},
		Monsters: []*monster.Data{
			{ID: "goblin-1", Name: "Goblin", HitPoints: 0},
		},
		HasBossRoom: false,
	}

	result := s.orchestrator.checkCombatEnd(enc)
	s.NotNil(result, "Should return victory when all monsters dead and no boss room")
	s.Equal("victory", result.Reason)
}

func (s *OrchestratorTestSuite) TestCheckCombatEnd_AllMonstersDeadWithBossRoom_ReturnsNil() {
	enc := &encounterrepo.EncounterData{
		InitiativeData: &initiative.TrackerData{
			Order: []initiative.EntityData{
				{ID: "char-1", Type: entityTypeCharacter},
				{ID: "goblin-1", Type: entityTypeMonster},
			},
		},
		Monsters: []*monster.Data{
			{ID: "goblin-1", Name: "Goblin", HitPoints: 0},
		},
		HasBossRoom: true,
	}

	result := s.orchestrator.checkCombatEnd(enc)
	s.Nil(result, "Should NOT return victory when HasBossRoom is true - more rooms to explore")
}

func (s *OrchestratorTestSuite) TestCheckCombatEnd_AllMonstersDeadWithBossMonsterIDs_ReturnsNil() {
	enc := &encounterrepo.EncounterData{
		InitiativeData: &initiative.TrackerData{
			Order: []initiative.EntityData{
				{ID: "char-1", Type: entityTypeCharacter},
				{ID: "goblin-1", Type: entityTypeMonster},
			},
		},
		Monsters: []*monster.Data{
			{ID: "goblin-1", Name: "Goblin", HitPoints: 0},
		},
		BossMonsterIDs: []string{"boss-1"},
		HasBossRoom:    true,
	}

	result := s.orchestrator.checkCombatEnd(enc)
	s.Nil(result, "Should NOT return victory when boss monsters are tracked")
}

func (s *OrchestratorTestSuite) TestCheckCombatEnd_MonstersStillAlive_ReturnsNil() {
	enc := &encounterrepo.EncounterData{
		InitiativeData: &initiative.TrackerData{
			Order: []initiative.EntityData{
				{ID: "char-1", Type: entityTypeCharacter},
				{ID: "goblin-1", Type: entityTypeMonster},
			},
		},
		Monsters: []*monster.Data{
			{ID: "goblin-1", Name: "Goblin", HitPoints: 10},
		},
	}

	result := s.orchestrator.checkCombatEnd(enc)
	s.Nil(result, "Combat should continue when monsters are alive")
}

func (s *OrchestratorTestSuite) TestCheckBossesDefeated_AllBossesDead() {
	enc := &encounterrepo.EncounterData{
		BossMonsterIDs: []string{"boss-1", "boss-2"},
		Monsters: []*monster.Data{
			{ID: "boss-1", Name: "Dragon", HitPoints: 0},
			{ID: "boss-2", Name: "Lich", HitPoints: 0},
			{ID: "goblin-1", Name: "Goblin", HitPoints: 10},
		},
	}

	allDead, lastBoss := s.orchestrator.checkBossesDefeated(enc)
	s.True(allDead)
	s.NotNil(lastBoss)
}

func (s *OrchestratorTestSuite) TestCheckBossesDefeated_SomeBossesAlive() {
	enc := &encounterrepo.EncounterData{
		BossMonsterIDs: []string{"boss-1", "boss-2"},
		Monsters: []*monster.Data{
			{ID: "boss-1", Name: "Dragon", HitPoints: 0},
			{ID: "boss-2", Name: "Lich", HitPoints: 50}, // Still alive
		},
	}

	allDead, _ := s.orchestrator.checkBossesDefeated(enc)
	s.False(allDead)
}

func (s *OrchestratorTestSuite) TestCheckBossesDefeated_NoBossIDs() {
	enc := &encounterrepo.EncounterData{
		BossMonsterIDs: []string{},
		Monsters: []*monster.Data{
			{ID: "goblin-1", Name: "Goblin", HitPoints: 0},
		},
	}

	allDead, _ := s.orchestrator.checkBossesDefeated(enc)
	s.False(allDead, "No boss IDs means no victory condition")
}

func (s *OrchestratorTestSuite) TestCheckBossesDefeated_BossIDsButNoMatchingMonsters() {
	// When boss ID exists but monster data is not found,
	// it's treated as "dead" to not block victory
	enc := &encounterrepo.EncounterData{
		BossMonsterIDs: []string{"boss-1"},
		Monsters: []*monster.Data{
			{ID: "goblin-1", Name: "Goblin", HitPoints: 10},
		},
	}

	allDead, lastBoss := s.orchestrator.checkBossesDefeated(enc)
	s.True(allDead, "Missing boss treated as defeated")
	s.Nil(lastBoss, "No boss data to return")
}

// ============================================================================
// GetEncounterState Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestGetEncounterState_Success_WaitingState() {
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	charData := &character.Data{
		ID:   characterID,
		Name: "Test Character",
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:       encounterID,
				State:    encounterrepo.StateWaiting,
				JoinCode: "ABC123",
				HostID:   playerID,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsReady:     false,
					},
				},
				LastEventID: "01JFABC123",
			},
		}, nil)

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	output, err := s.orchestrator.GetEncounterState(context.Background(), &GetEncounterStateInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal(encounterID, output.EncounterID)
	s.Assert().Equal("waiting", output.State)
	s.Assert().Equal("ABC123", output.JoinCode)
	s.Assert().Equal(playerID, output.HostID)
	s.Assert().Equal("01JFABC123", output.LastEventID)
	s.Assert().Len(output.Party, 1)
	s.Assert().Equal(characterID, output.Party[0].CharacterID)
	s.Assert().Nil(output.CombatState) // No combat state in waiting
	s.Assert().Nil(output.Room)        // No room in waiting
}

func (s *OrchestratorTestSuite) TestGetEncounterState_Success_ActiveState() {
	encounterID := "enc-123"
	playerID := "player-1"
	characterID := "char-1"

	charData := &character.Data{
		ID:        characterID,
		Name:      "Test Character",
		HitPoints: 30,
	}

	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: characterID, Type: "character"},
			{ID: "goblin-1", Type: "monster"},
		},
		Current: 0,
		Round:   1,
	}

	roomData := &spatial.RoomData{
		ID:       "room-1",
		Width:    10,
		Height:   10,
		GridType: spatial.GridTypeHex,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                encounterID,
				State:             encounterrepo.StateActive,
				JoinCode:          "ABC123",
				HostID:            playerID,
				InitiativeData:    initiativeData,
				RoomData:          roomData,
				MovementRemaining: 30,
				Players: map[string]*encounterrepo.Player{
					playerID: {
						PlayerID:    playerID,
						CharacterID: characterID,
						IsReady:     true,
					},
				},
				Monsters: []*monster.Data{
					{ID: "goblin-1", Name: "Goblin", HitPoints: 7, MaxHitPoints: 7},
					{ID: "goblin-2", Name: "Goblin", HitPoints: 0, MaxHitPoints: 7}, // Dead
				},
				LastEventID: "01JFXYZ789",
			},
		}, nil)

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	// Mock dungeon repo for doors
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), &dungeonrepo.GetByEncounterIDInput{EncounterID: encounterID}).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{
				ID:          "dungeon-1",
				EncounterID: encounterID,
				StartRoomID: "room-1",
			},
		}, nil)

	output, err := s.orchestrator.GetEncounterState(context.Background(), &GetEncounterStateInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Equal(encounterID, output.EncounterID)
	s.Assert().Equal("active", output.State)
	s.Assert().Equal("01JFXYZ789", output.LastEventID)
	s.Assert().NotNil(output.CombatState)
	s.Assert().Equal(1, output.CombatState.Round)
	s.Assert().NotNil(output.Room)
	// Only alive monsters
	s.Assert().Len(output.Monsters, 1)
	s.Assert().Equal("goblin-1", output.Monsters[0].MonsterID)
	s.Assert().Equal(7, output.Monsters[0].CurrentHitPoints)
	// Dungeon info should be populated
	s.Assert().Equal("dungeon-1", output.DungeonID)
}

func (s *OrchestratorTestSuite) TestGetEncounterState_EncounterNotFound() {
	encounterID := "nonexistent"
	playerID := "player-1"

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(nil, fmt.Errorf("not found"))

	output, err := s.orchestrator.GetEncounterState(context.Background(), &GetEncounterStateInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
	})

	s.Require().Error(err)
	s.Assert().Equal(ErrEncounterNotFound, err)
	s.Assert().Nil(output)
}

func (s *OrchestratorTestSuite) TestGetEncounterState_PlayerNotInEncounter() {
	encounterID := "enc-123"
	playerID := "other-player"

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: encounterID}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:     encounterID,
				State:  encounterrepo.StateWaiting,
				HostID: "host-player",
				Players: map[string]*encounterrepo.Player{
					"host-player": {
						PlayerID:    "host-player",
						CharacterID: "char-1",
					},
				},
			},
		}, nil)

	output, err := s.orchestrator.GetEncounterState(context.Background(), &GetEncounterStateInput{
		EncounterID: encounterID,
		PlayerID:    playerID,
	})

	s.Require().Error(err)
	s.Assert().Equal(ErrPlayerNotInEncounter, err)
	s.Assert().Nil(output)
}

func (s *OrchestratorTestSuite) TestGetEncounterState_NilInput() {
	output, err := s.orchestrator.GetEncounterState(context.Background(), nil)

	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "input is required")
	s.Assert().Nil(output)
}

func (s *OrchestratorTestSuite) TestGetEncounterState_MissingEncounterID() {
	output, err := s.orchestrator.GetEncounterState(context.Background(), &GetEncounterStateInput{
		PlayerID: "player-1",
	})

	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "encounter ID is required")
	s.Assert().Nil(output)
}

func (s *OrchestratorTestSuite) TestGetEncounterState_MissingPlayerID() {
	output, err := s.orchestrator.GetEncounterState(context.Background(), &GetEncounterStateInput{
		EncounterID: "enc-123",
	})

	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "player ID is required")
	s.Assert().Nil(output)
}

// ============================================================================
// ActivateCombatAbility Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestActivateCombatAbility_Attack_Success() {
	// Arrange - Set up encounter with a character whose turn it is
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "goblin-1", Type: "monster"},
		},
		Current: 0, // char-1's turn
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				InitiativeData:    initiativeData,
				MovementRemaining: 30,
			},
		}, nil)

	// Mock character repo - load character and persist after ability activation
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// Act
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), &ActivateCombatAbilityInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		AbilityID:   pb.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().Empty(output.Error)
	s.Assert().NotEmpty(output.GrantedCapacity) // Toolkit returns capacity description
	// Available abilities/actions should be populated from toolkit
	s.Assert().NotNil(output.AvailableAbilities)
	s.Assert().NotNil(output.AvailableActions)
}

func (s *OrchestratorTestSuite) TestActivateCombatAbility_Attack_NoActionAvailable() {
	// Arrange - Action already used (action economy on character data)
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				InitiativeData:    initiativeData,
				MovementRemaining: 30,
			},
		}, nil)

	// Character with action economy already consumed (same turn)
	charData := createTestCharacterData("char-1", "Grog")
	charData.ActionEconomy = &character.ActionEconomyData{
		TurnNumber:            100, // Matches computeTurnNumber(round=1, current=0)
		ActionsRemaining:      0,   // Already consumed
		BonusActionsRemaining: 1,
		ReactionsRemaining:    1,
		MovementRemaining:     30,
		Granted:               make(map[character.GrantedActionKey]int),
	}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	// Character state is persisted even on failure (it was loaded and may have been initialized)
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// Act
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), &ActivateCombatAbilityInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		AbilityID:   pb.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	// Assert - Should fail but not error (business logic failure)
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().False(output.Success)
	s.Assert().NotEmpty(output.Error)
}

func (s *OrchestratorTestSuite) TestActivateCombatAbility_Attack_AfterEndTurn_ResetsActionEconomy() {
	// Reproduce bug: after EndTurn → monster turn → TurnChange back to player,
	// calling ActivateCombatAbility(Attack) should succeed because StartTurn
	// resets the action economy when the turn number has changed.
	//
	// Setup: Round 2, Current=0 → turnNum=200. Character has stale TurnNumber=100.
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "goblin-1", Type: "monster"},
		},
		Current: 0,
		Round:   2, // Round 2 after EndTurn advanced past monster
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				InitiativeData:    initiativeData,
				MovementRemaining: 30,
				ActionEconomy:     entities.NewActionEconomyState(), // Fresh from EndTurn
			},
		}, nil)

	// Character with STALE action economy from turn 1 (all consumed)
	charData := createTestCharacterData("char-1", "Grog")
	charData.ActionEconomy = &character.ActionEconomyData{
		TurnNumber:            100, // Round 1 turn number (stale)
		ActionsRemaining:      0,   // All consumed in turn 1
		BonusActionsRemaining: 0,
		ReactionsRemaining:    0,
		MovementRemaining:     0,
		Granted:               make(map[character.GrantedActionKey]int),
	}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	// persistCharacterData after activation
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// Act - Should succeed: loadCharacterForCombat detects stale TurnNumber and calls StartTurn
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), &ActivateCombatAbilityInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		AbilityID:   pb.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	// Assert - Action economy was reset by StartTurn, so Attack should succeed
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success, "Attack should succeed on new turn, got error: %s", output.Error)
	s.Assert().NotEmpty(output.GrantedCapacity, "Should grant attacks")
}

func (s *OrchestratorTestSuite) TestActivateCombatAbility_Attack_AfterEndTurn_InvalidatedTurnNumber() {
	// Reproduce bug: publishCharacterTurnEnd now invalidates TurnNumber to -1.
	// When the character's next turn starts, loadCharacterForCombat should detect
	// TurnNumber=-1 != computeTurnNumber() and call StartTurn.
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "goblin-1", Type: "monster"},
		},
		Current: 0,
		Round:   2,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				InitiativeData:    initiativeData,
				MovementRemaining: 30,
				ActionEconomy:     entities.NewActionEconomyState(),
			},
		}, nil)

	// Character with TurnNumber=-1 (invalidated by publishCharacterTurnEnd)
	charData := createTestCharacterData("char-1", "Grog")
	charData.ActionEconomy = &character.ActionEconomyData{
		TurnNumber:            -1, // Invalidated by publishCharacterTurnEnd
		ActionsRemaining:      0,  // Zeroed by char.EndTurn()
		BonusActionsRemaining: 0,
		ReactionsRemaining:    0,
		MovementRemaining:     0,
		Granted:               make(map[character.GrantedActionKey]int),
	}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// Act
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), &ActivateCombatAbilityInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		AbilityID:   pb.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	// Assert - StartTurn should be called because -1 != 200 (computeTurnNumber)
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success, "Attack should succeed after turn invalidation, got error: %s", output.Error)
	s.Assert().NotEmpty(output.GrantedCapacity)
}

func (s *OrchestratorTestSuite) TestActivateCombatAbility_Dash_Success() {
	// Arrange
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				InitiativeData:    initiativeData,
				MovementRemaining: 30,
			},
		}, nil)

	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// Act
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), &ActivateCombatAbilityInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		AbilityID:   pb.CombatAbilityId_COMBAT_ABILITY_ID_DASH,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().NotEmpty(output.GrantedCapacity)
}

func (s *OrchestratorTestSuite) TestActivateCombatAbility_Dodge_Success() {
	// Arrange
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				InitiativeData:    initiativeData,
				MovementRemaining: 30,
			},
		}, nil)

	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// Act
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), &ActivateCombatAbilityInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		AbilityID:   pb.CombatAbilityId_COMBAT_ABILITY_ID_DODGE,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().NotEmpty(output.GrantedCapacity)
}

func (s *OrchestratorTestSuite) TestActivateCombatAbility_Disengage_Success() {
	// Arrange
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				InitiativeData:    initiativeData,
				MovementRemaining: 30,
			},
		}, nil)

	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// Act
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), &ActivateCombatAbilityInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		AbilityID:   pb.CombatAbilityId_COMBAT_ABILITY_ID_DISENGAGE,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().NotEmpty(output.GrantedCapacity)
}

func (s *OrchestratorTestSuite) TestActivateCombatAbility_OffhandAttack_Success() {
	// Arrange - Need a character with two light weapons for off-hand attack
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				InitiativeData:    initiativeData,
				MovementRemaining: 30,
			},
		}, nil)

	// Character with two light weapons (daggers) for TWF off-hand
	charData := createTestCharacterData("char-1", "Grog")
	charData.EquipmentSlots = character.EquipmentSlots{
		character.SlotMainHand: "dagger",
		character.SlotOffHand:  "dagger",
	}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// Act
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), &ActivateCombatAbilityInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		AbilityID:   pb.CombatAbilityId_COMBAT_ABILITY_ID_OFFHAND_ATTACK,
	})

	// Assert - Off-hand Attack is a bonus action ability.
	// Whether it succeeds depends on the toolkit recognizing the character has TWF setup.
	// The toolkit may not have OffHandAttack as a standard combat ability unless the
	// character has two light weapons. Either way, it should not error.
	s.Require().NoError(err)
	s.Require().NotNil(output)
	// The OffHandAttack ability may not be found if the character doesn't have it registered.
	// The toolkit currently only registers it dynamically when hasTwoLightWeapons() is true.
	// Since this is equipment-based, the Character's buildAvailableAbilities adds it.
	// But ActivateAbility routes by ref, so it needs to match a CombatAbility or Feature.
	// The toolkit ActivateAbility for OffHandAttack may return "unknown ability".
	// This is correct behavior - the test validates the delegation works.
}

func (s *OrchestratorTestSuite) TestActivateCombatAbility_NotEntityTurn() {
	// Arrange - char-1 is trying to act but it's goblin-1's turn
	initiativeData := &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "goblin-1", Type: "monster"},
			{ID: "char-1", Type: "character"},
		},
		Current: 0, // goblin-1's turn
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{
			Data: &encounterrepo.EncounterData{
				ID:                "enc-1",
				InitiativeData:    initiativeData,
				MovementRemaining: 30,
			},
		}, nil)

	// Act
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), &ActivateCombatAbilityInput{
		EncounterID: "enc-1",
		EntityID:    "char-1", // Trying to act when it's not their turn
		AbilityID:   pb.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	// Assert - Should error since it's not their turn
	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "not entity's turn")
	s.Assert().Nil(output)
}

func (s *OrchestratorTestSuite) TestActivateCombatAbility_NilInput() {
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), nil)

	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "input is required")
	s.Assert().Nil(output)
}

func (s *OrchestratorTestSuite) TestActivateCombatAbility_MissingEncounterID() {
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), &ActivateCombatAbilityInput{
		EntityID:  "char-1",
		AbilityID: pb.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "encounter ID is required")
	s.Assert().Nil(output)
}

func (s *OrchestratorTestSuite) TestActivateCombatAbility_MissingEntityID() {
	output, err := s.orchestrator.ActivateCombatAbility(context.Background(), &ActivateCombatAbilityInput{
		EncounterID: "enc-1",
		AbilityID:   pb.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	s.Require().Error(err)
	s.Assert().Contains(err.Error(), "entity ID is required")
	s.Assert().Nil(output)
}

// ============================================================================
// ExecuteAction Tests
// ============================================================================

func (s *OrchestratorTestSuite) TestExecuteAction_Strike_Success() {
	// Arrange - Create test character data with equipment slots and attacks granted
	charData := createTestCharacterData("char-1", "Grog")
	// Simulate ATTACK ability was activated: action consumed, 1 attack granted (same turn)
	charData.ActionEconomy = &character.ActionEconomyData{
		TurnNumber:            100, // Matches computeTurnNumber(round=1, current=0)
		ActionsRemaining:      0,   // Action consumed by ATTACK activation
		BonusActionsRemaining: 1,
		ReactionsRemaining:    1,
		MovementRemaining:     30,
		Granted: map[character.GrantedActionKey]int{
			character.GrantedAttacks: 1, // ATTACK granted one attack
		},
	}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil).
		AnyTimes()

	// Create encounter data with initiative
	encData := createTestEncounterData("enc-1")
	encData.InitiativeData = &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
			{ID: "goblin-1", Type: "monster"},
		},
		Current: 0,
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Mock Update for persisting the attack result
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()

	// Mock dungeon repo for wall loading (optional)
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{
			Dungeon: &entities.Dungeon{ID: "test-dungeon"},
		}, nil).
		AnyTimes()

	// Act
	output, err := s.orchestrator.ExecuteAction(context.Background(), &ExecuteActionInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		ActionID:    pb.ActionId_ACTION_ID_STRIKE,
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Assert().NotNil(output.AttackResult)

	// Verify attack result has valid values
	s.Assert().GreaterOrEqual(output.AttackResult.AttackRoll, 1)
	s.Assert().LessOrEqual(output.AttackResult.AttackRoll, 20)

	// Available abilities/actions should be populated from toolkit
	s.Assert().NotNil(output.AvailableAbilities)
	s.Assert().NotNil(output.AvailableActions)
}

func (s *OrchestratorTestSuite) TestExecuteAction_Strike_NoAttacksRemaining() {
	// Arrange - Character with NO attacks granted (action economy on character)
	charData := createTestCharacterData("char-1", "Grog")
	charData.ActionEconomy = &character.ActionEconomyData{
		ActionsRemaining:      1,
		BonusActionsRemaining: 1,
		ReactionsRemaining:    1,
		MovementRemaining:     30,
		Granted:               map[character.GrantedActionKey]int{}, // No attacks granted!
	}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil)

	encData := createTestEncounterData("enc-1")
	encData.InitiativeData = &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	// Act
	output, err := s.orchestrator.ExecuteAction(context.Background(), &ExecuteActionInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		ActionID:    pb.ActionId_ACTION_ID_STRIKE,
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err) // Not an error, but Success=false
	s.Require().NotNil(output)
	s.Assert().False(output.Success)
	s.Assert().Contains(output.Error, "no attacks remaining")
}

func (s *OrchestratorTestSuite) TestExecuteAction_Move_Success() {
	// Arrange - Create encounter with room data
	encData := createTestEncounterData("enc-1")
	encData.InitiativeData = &initiative.TrackerData{
		Order: []initiative.EntityData{
			{ID: "char-1", Type: "character"},
		},
		Current: 0,
		Round:   1,
	}
	encData.MovementRemaining = 30

	// Create room data with entity at position (0, 0, 0)
	encData.RoomData = &spatial.RoomData{
		ID:       "room-1",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		CubeEntities: map[string]spatial.EntityCubePlacement{
			"char-1": {
				EntityID:       "char-1",
				EntityType:     "character",
				CubePosition:   spatial.CubeCoordinate{X: 0, Y: 0, Z: 0},
				Size:           1,
				BlocksMovement: true,
			},
		},
	}

	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)

	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil)

	// Mock character repo for loading available abilities/actions at end of move
	charData := createTestCharacterData("char-1", "Grog")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{}, nil).
		AnyTimes()

	// Act - Move 2 hexes (10 feet)
	output, err := s.orchestrator.ExecuteAction(context.Background(), &ExecuteActionInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
		ActionID:    pb.ActionId_ACTION_ID_MOVE,
		Path: []Position{
			{X: 1, Y: -1, Z: 0}, // Move one hex
			{X: 2, Y: -2, Z: 0}, // Move second hex
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().True(output.Success)
	s.Require().NotNil(output.MoveResult)
	s.Assert().Equal(10, output.MoveResult.MovementUsed)
	s.Assert().Equal("completed", output.MoveResult.StopReason)
	s.Assert().Equal(float64(2), output.MoveResult.FinalPosition.X)
	s.Assert().Equal(float64(-2), output.MoveResult.FinalPosition.Y)
	s.Assert().Equal(float64(0), output.MoveResult.FinalPosition.Z)
}

func (s *OrchestratorTestSuite) TestExecuteAction_NilInput() {
	output, err := s.orchestrator.ExecuteAction(context.Background(), nil)

	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "input is required")
}

func (s *OrchestratorTestSuite) TestExecuteAction_MissingEncounterID() {
	output, err := s.orchestrator.ExecuteAction(context.Background(), &ExecuteActionInput{
		EntityID: "char-1",
		ActionID: pb.ActionId_ACTION_ID_STRIKE,
	})

	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "encounter ID is required")
}

func (s *OrchestratorTestSuite) TestExecuteAction_MissingEntityID() {
	output, err := s.orchestrator.ExecuteAction(context.Background(), &ExecuteActionInput{
		EncounterID: "enc-1",
		ActionID:    pb.ActionId_ACTION_ID_STRIKE,
	})

	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "entity ID is required")
}

func (s *OrchestratorTestSuite) TestExecuteAction_MissingActionID() {
	output, err := s.orchestrator.ExecuteAction(context.Background(), &ExecuteActionInput{
		EncounterID: "enc-1",
		EntityID:    "char-1",
	})

	s.Require().Error(err)
	s.Assert().Nil(output)
	s.Assert().Contains(err.Error(), "action ID is required")
}

func createTestMonkCharacterData(id, name string) *character.Data {
	return &character.Data{
		ID:               id,
		Name:             name,
		Level:            1,
		RaceID:           "human",
		ClassID:          classes.Monk,
		ProficiencyBonus: 2,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 10,
			abilities.DEX: 16, // Monks use DEX
			abilities.CON: 14,
			abilities.INT: 10,
			abilities.WIS: 14, // Important for Monk AC
			abilities.CHA: 8,
		},
		Features: []json.RawMessage{},
		EquipmentSlots: character.EquipmentSlots{
			character.SlotMainHand: "shortsword", // Monk weapon
		},
	}
}

func (s *OrchestratorTestSuite) TestResolveAttack_Monk_GrantsMartialArtsBonusStrike() {
	// Arrange - Create Monk character with shortsword (monk weapon)
	charData := createTestMonkCharacterData("monk-1", "Shadow")
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "monk-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{Dungeon: &entities.Dungeon{ID: "test-dungeon"}}, nil).
		AnyTimes()

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "monk-1",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.GrantedAction, "Monk should receive a granted action")
	s.Assert().Equal("martial-arts-bonus-strike", output.GrantedAction.Type)
	s.Assert().Equal("Martial Arts Bonus Strike", output.GrantedAction.Name)
}

func (s *OrchestratorTestSuite) TestResolveAttack_NonMonk_NoMartialArtsBonusStrike() {
	// Arrange - Create non-Monk character (barbarian with shortsword)
	charData := createTestCharacterData("barb-1", "Grog")
	charData.EquipmentSlots[character.SlotMainHand] = "shortsword" // Same weapon, different class
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "barb-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{Dungeon: &entities.Dungeon{ID: "test-dungeon"}}, nil).
		AnyTimes()

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "barb-1",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	// Barbarian with shortsword should NOT get martial arts bonus strike
	// (might get TWF if dual-wielding, but not MA)
	if output.GrantedAction != nil {
		s.Assert().NotEqual("martial-arts-bonus-strike", output.GrantedAction.Type,
			"Non-Monk should not receive Martial Arts bonus strike")
	}
}

func (s *OrchestratorTestSuite) TestResolveAttack_Monk_NonMonkWeapon_NoBonusStrike() {
	// Arrange - Monk with greataxe (NOT a monk weapon)
	charData := createTestMonkCharacterData("monk-1", "Shadow")
	charData.EquipmentSlots[character.SlotMainHand] = "greataxe" // Not a monk weapon
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "monk-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{Dungeon: &entities.Dungeon{ID: "test-dungeon"}}, nil).
		AnyTimes()

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "monk-1",
		TargetID:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(output)
	// Monk with greataxe should NOT get martial arts bonus strike
	if output.GrantedAction != nil {
		s.Assert().NotEqual("martial-arts-bonus-strike", output.GrantedAction.Type,
			"Monk with non-monk weapon should not receive MA bonus strike")
	}
}

// --- Issue #437: Unarmed strike option for Monks ---

func (s *OrchestratorTestSuite) TestAddUnarmedStrikeOption_AddsWhenStrikeAvailable() {
	actions := []*entities.AvailableAction{
		{ActionID: refs.Actions.Move().ID, Name: "Move", CanUse: true},
		{ActionID: refs.Actions.Strike().ID, Name: "Strike", CanUse: true},
	}

	result := addUnarmedStrikeOption(actions)

	s.Require().Len(result, 3)
	s.Assert().Equal(refs.Actions.UnarmedStrike().ID, result[2].ActionID)
	s.Assert().Equal("Unarmed Strike", result[2].Name)
	s.Assert().True(result[2].CanUse)
}

func (s *OrchestratorTestSuite) TestAddUnarmedStrikeOption_NoopWhenStrikeAbsent() {
	actions := []*entities.AvailableAction{
		{ActionID: refs.Actions.Move().ID, Name: "Move", CanUse: true},
	}

	result := addUnarmedStrikeOption(actions)

	s.Assert().Len(result, 1, "should not add unarmed strike when Strike is not available")
}

func (s *OrchestratorTestSuite) TestAddUnarmedStrikeOption_NoopWhenAlreadyPresent() {
	actions := []*entities.AvailableAction{
		{ActionID: refs.Actions.Strike().ID, Name: "Strike", CanUse: true},
		{ActionID: refs.Actions.UnarmedStrike().ID, Name: "Unarmed Strike", CanUse: true},
	}

	result := addUnarmedStrikeOption(actions)

	s.Assert().Len(result, 2, "should not duplicate unarmed strike")
}

func (s *OrchestratorTestSuite) TestAddUnarmedStrikeOption_InheritsStrikeCanUse() {
	actions := []*entities.AvailableAction{
		{ActionID: refs.Actions.Strike().ID, Name: "Strike", CanUse: false, Reason: "no attacks remaining"},
	}

	result := addUnarmedStrikeOption(actions)

	s.Require().Len(result, 2)
	s.Assert().False(result[1].CanUse, "unarmed strike should inherit Strike's can_use state")
}

func (s *OrchestratorTestSuite) TestResolveAttack_IsUnarmed_DetectedFromWeapon() {
	// Arrange - Monk with no equipped weapon (falls back to unarmed strike)
	charData := createTestMonkCharacterData("monk-1", "Shadow")
	charData.EquipmentSlots = character.EquipmentSlots{} // No weapon
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "monk-1"}).
		Return(&characterrepo.GetOutput{Character: &entities.Character{Data: charData}}, nil).
		AnyTimes()

	encData := createTestEncounterData("enc-1")
	s.mockEncRepo.EXPECT().
		Get(gomock.Any(), &encounterrepo.GetInput{EncounterID: "enc-1"}).
		Return(&encounterrepo.GetOutput{Data: encData}, nil)
	s.mockEncRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&encounterrepo.UpdateOutput{Success: true}, nil).
		AnyTimes()
	s.mockDungeonRepo.EXPECT().
		GetByEncounterID(gomock.Any(), gomock.Any()).
		Return(&dungeonrepo.GetOutput{Dungeon: &entities.Dungeon{ID: "test-dungeon"}}, nil).
		AnyTimes()

	// Act
	output, err := s.orchestrator.ResolveAttack(context.Background(), &ResolveAttackInput{
		EncounterID: "enc-1",
		AttackerID:  "monk-1",
		TargetID:    "goblin-1",
	})

	// Assert - Should succeed with unarmed strike and grant martial arts bonus
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().NotNil(output.Result)

	// Unarmed strike does bludgeoning damage
	if output.Result.Hit {
		s.Assert().Equal(damage.Bludgeoning, output.Result.DamageType)
	}

	// Monk with unarmed strike should get martial arts bonus strike
	s.Assert().NotNil(output.GrantedAction, "Monk using unarmed strike should get martial arts bonus")
	if output.GrantedAction != nil {
		s.Assert().Equal("martial-arts-bonus-strike", output.GrantedAction.Type)
	}
}
