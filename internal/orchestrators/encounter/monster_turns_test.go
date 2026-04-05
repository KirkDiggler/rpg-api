package encounter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-toolkit/core"
	dicemock "github.com/KirkDiggler/rpg-toolkit/dice/mock"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
)

type MonsterTurnsTestSuite struct {
	suite.Suite
	orchestrator *Orchestrator
}

func TestMonsterTurnsTestSuite(t *testing.T) {
	suite.Run(t, new(MonsterTurnsTestSuite))
}

func (s *MonsterTurnsTestSuite) SetupTest() {
	cfg := &Config{
		CharacterRepo: nil, // Will be mocked in tests that need it
		EncounterRepo: nil,
	}
	// Create orchestrator without validation for now
	s.orchestrator = &Orchestrator{
		charRepo: cfg.CharacterRepo,
		encRepo:  cfg.EncounterRepo,
	}
}

func (s *MonsterTurnsTestSuite) TestIsMonsterTurn() {
	tests := []struct {
		name     string
		enc      *encounterrepo.EncounterData
		expected bool
	}{
		{
			name:     "nil encounter",
			enc:      nil,
			expected: false,
		},
		{
			name: "nil initiative data",
			enc: &encounterrepo.EncounterData{
				InitiativeData: nil,
			},
			expected: false,
		},
		{
			name: "invalid current index - negative",
			enc: &encounterrepo.EncounterData{
				InitiativeData: &initiative.TrackerData{
					Current: -1,
					Order: []initiative.EntityData{
						{ID: "goblin-1", Type: "monster"},
					},
				},
			},
			expected: false,
		},
		{
			name: "invalid current index - too large",
			enc: &encounterrepo.EncounterData{
				InitiativeData: &initiative.TrackerData{
					Current: 5,
					Order: []initiative.EntityData{
						{ID: "goblin-1", Type: "monster"},
					},
				},
			},
			expected: false,
		},
		{
			name: "monster turn",
			enc: &encounterrepo.EncounterData{
				InitiativeData: &initiative.TrackerData{
					Current: 0,
					Order: []initiative.EntityData{
						{ID: "goblin-1", Type: "monster"},
						{ID: "char-1", Type: "character"},
					},
				},
			},
			expected: true,
		},
		{
			name: "character turn",
			enc: &encounterrepo.EncounterData{
				InitiativeData: &initiative.TrackerData{
					Current: 1,
					Order: []initiative.EntityData{
						{ID: "goblin-1", Type: "monster"},
						{ID: "char-1", Type: "character"},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := s.orchestrator.isMonsterTurn(tt.enc)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *MonsterTurnsTestSuite) TestFindMonsterData() {
	goblin1 := &monster.Data{
		ID:           "goblin-1",
		Name:         "Goblin Warrior",
		HitPoints:    7,
		MaxHitPoints: 7,
	}
	goblin2 := &monster.Data{
		ID:           "goblin-2",
		Name:         "Goblin Archer",
		HitPoints:    7,
		MaxHitPoints: 7,
	}

	tests := []struct {
		name     string
		enc      *encounterrepo.EncounterData
		id       string
		expected *monster.Data
	}{
		{
			name:     "nil encounter",
			enc:      nil,
			id:       "goblin-1",
			expected: nil,
		},
		{
			name: "empty monsters",
			enc: &encounterrepo.EncounterData{
				Monsters: []*monster.Data{},
			},
			id:       "goblin-1",
			expected: nil,
		},
		{
			name: "monster found - first",
			enc: &encounterrepo.EncounterData{
				Monsters: []*monster.Data{goblin1, goblin2},
			},
			id:       "goblin-1",
			expected: goblin1,
		},
		{
			name: "monster found - second",
			enc: &encounterrepo.EncounterData{
				Monsters: []*monster.Data{goblin1, goblin2},
			},
			id:       "goblin-2",
			expected: goblin2,
		},
		{
			name: "monster not found",
			enc: &encounterrepo.EncounterData{
				Monsters: []*monster.Data{goblin1, goblin2},
			},
			id:       "goblin-3",
			expected: nil,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := s.orchestrator.findMonsterData(tt.enc, tt.id)
			s.Equal(tt.expected, result)
		})
	}
}

func (s *MonsterTurnsTestSuite) TestExecuteMonsterTurns_NoMonsters() {
	// Setup encounter with character's turn
	enc := &encounterrepo.EncounterData{
		InitiativeData: &initiative.TrackerData{
			Current: 0,
			Round:   1,
			Order: []initiative.EntityData{
				{ID: "char-1", Type: "character"},
			},
		},
		Monsters: []*monster.Data{},
	}

	results, err := s.orchestrator.executeMonsterTurns(context.Background(), enc, []string{"char-1"})

	s.NoError(err)
	s.Empty(results, "should return empty results when it's a character's turn")
}

func (s *MonsterTurnsTestSuite) TestExecuteMonsterTurns_DeadMonsterSkipped() {
	// Setup encounter with dead monster
	deadGoblin := &monster.Data{
		ID:           "goblin-1",
		Name:         "Dead Goblin",
		HitPoints:    0, // Dead
		MaxHitPoints: 7,
	}

	enc := &encounterrepo.EncounterData{
		InitiativeData: &initiative.TrackerData{
			Current: 0,
			Round:   1,
			Order: []initiative.EntityData{
				{ID: "goblin-1", Type: "monster"},
				{ID: "char-1", Type: "character"},
			},
		},
		Monsters: []*monster.Data{deadGoblin},
	}

	results, err := s.orchestrator.executeMonsterTurns(context.Background(), enc, []string{"char-1"})

	s.NoError(err)
	s.Empty(results, "should skip dead monster")
	s.Equal(1, enc.InitiativeData.Current, "should advance to character's turn")
}

func (s *MonsterTurnsTestSuite) TestExecuteMonsterTurns_MonsterNotInData() {
	// Setup encounter where monster is in initiative but not in Monsters array
	enc := &encounterrepo.EncounterData{
		InitiativeData: &initiative.TrackerData{
			Current: 0,
			Round:   1,
			Order: []initiative.EntityData{
				{ID: "goblin-1", Type: "monster"},
				{ID: "char-1", Type: "character"},
			},
		},
		Monsters: []*monster.Data{}, // Empty - monster not found
	}

	results, err := s.orchestrator.executeMonsterTurns(context.Background(), enc, []string{"char-1"})

	s.NoError(err)
	s.Empty(results, "should skip missing monster")
	s.Equal(1, enc.InitiativeData.Current, "should advance to character's turn")
}

func (s *MonsterTurnsTestSuite) TestExecuteMonsterTurns_MultipleMonsters() {
	// Create a room with a goblin and character
	roomData := &spatial.RoomData{
		ID:       "test-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		Entities: map[string]spatial.EntityPlacement{
			"goblin-1": {
				EntityID:       "goblin-1",
				EntityType:     "monster",
				Position:       spatial.Position{X: 10, Y: 10},
				Size:           1,
				BlocksMovement: true,
			},
			"goblin-2": {
				EntityID:       "goblin-2",
				EntityType:     "monster",
				Position:       spatial.Position{X: 12, Y: 10},
				Size:           1,
				BlocksMovement: true,
			},
			"char-1": {
				EntityID:       "char-1",
				EntityType:     "character",
				Position:       spatial.Position{X: 5, Y: 10},
				Size:           1,
				BlocksMovement: true,
			},
		},
	}

	goblin1 := &monster.Data{
		ID:           "goblin-1",
		Name:         "Goblin Warrior",
		HitPoints:    7,
		MaxHitPoints: 7,
		ArmorClass:   15,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 8,
			abilities.DEX: 14,
			abilities.CON: 10,
			abilities.INT: 10,
			abilities.WIS: 8,
			abilities.CHA: 8,
		},
		Actions: []monster.ActionData{
			{
				Ref: core.Ref{
					Module: "dnd5e",
					Type:   "monster_actions",
					ID:     "scimitar",
				},
			},
		},
	}

	goblin2 := &monster.Data{
		ID:           "goblin-2",
		Name:         "Goblin Archer",
		HitPoints:    7,
		MaxHitPoints: 7,
		ArmorClass:   15,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 8,
			abilities.DEX: 14,
			abilities.CON: 10,
			abilities.INT: 10,
			abilities.WIS: 8,
			abilities.CHA: 8,
		},
		Actions: []monster.ActionData{
			{
				Ref: core.Ref{
					Module: "dnd5e",
					Type:   "monster_actions",
					ID:     "scimitar",
				},
			},
		},
	}

	enc := &encounterrepo.EncounterData{
		InitiativeData: &initiative.TrackerData{
			Current: 0,
			Round:   1,
			Order: []initiative.EntityData{
				{ID: "goblin-1", Type: "monster"},
				{ID: "goblin-2", Type: "monster"},
				{ID: "char-1", Type: "character"},
			},
		},
		RoomData: roomData,
		Monsters: []*monster.Data{goblin1, goblin2},
	}

	results, err := s.orchestrator.executeMonsterTurns(context.Background(), enc, []string{"char-1"})

	s.NoError(err)
	s.Len(results, 2, "should execute turns for both monsters")
	s.Equal("goblin-1", results[0].MonsterID)
	s.Equal("goblin-2", results[1].MonsterID)
	s.Equal(2, enc.InitiativeData.Current, "should advance to character's turn")
}

func (s *MonsterTurnsTestSuite) TestExecuteMonsterTurns_RoundWrap() {
	// Create a simple room
	roomData := &spatial.RoomData{
		ID:       "test-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		Entities: map[string]spatial.EntityPlacement{
			"goblin-1": {
				EntityID:       "goblin-1",
				EntityType:     "monster",
				Position:       spatial.Position{X: 10, Y: 10},
				Size:           1,
				BlocksMovement: true,
			},
			"char-1": {
				EntityID:       "char-1",
				EntityType:     "character",
				Position:       spatial.Position{X: 5, Y: 10},
				Size:           1,
				BlocksMovement: true,
			},
		},
	}

	goblin1 := &monster.Data{
		ID:           "goblin-1",
		Name:         "Goblin Warrior",
		HitPoints:    7,
		MaxHitPoints: 7,
		ArmorClass:   15,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 8,
			abilities.DEX: 14,
			abilities.CON: 10,
			abilities.INT: 10,
			abilities.WIS: 8,
			abilities.CHA: 8,
		},
		Actions: []monster.ActionData{
			{
				Ref: core.Ref{
					Module: "dnd5e",
					Type:   "monster_actions",
					ID:     "scimitar",
				},
			},
		},
	}

	enc := &encounterrepo.EncounterData{
		InitiativeData: &initiative.TrackerData{
			Current: 1, // Start at end of order
			Round:   1,
			Order: []initiative.EntityData{
				{ID: "char-1", Type: "character"},
				{ID: "goblin-1", Type: "monster"},
			},
		},
		RoomData: roomData,
		Monsters: []*monster.Data{goblin1},
	}

	results, err := s.orchestrator.executeMonsterTurns(context.Background(), enc, []string{"char-1"})

	s.NoError(err)
	s.Len(results, 1, "should execute goblin's turn")
	s.Equal("goblin-1", results[0].MonsterID)
	s.Equal(0, enc.InitiativeData.Current, "should wrap to start of order")
	s.Equal(2, enc.InitiativeData.Round, "should increment round")
}

func (s *MonsterTurnsTestSuite) TestExecuteMonsterTurns_NilEncounter() {
	results, err := s.orchestrator.executeMonsterTurns(context.Background(), nil, []string{"char-1"})

	s.Error(err)
	s.Nil(results)
	s.Contains(err.Error(), "encounter data is required")
}

func (s *MonsterTurnsTestSuite) TestExecuteMonsterTurns_NilInitiative() {
	enc := &encounterrepo.EncounterData{
		InitiativeData: nil,
	}

	results, err := s.orchestrator.executeMonsterTurns(context.Background(), enc, []string{"char-1"})

	s.Error(err)
	s.Nil(results)
	s.Contains(err.Error(), "initiative data is required")
}

// =============================================================================
// RESOLVE MONSTER ATTACK TESTS
// =============================================================================

// ResolveMonsterAttackSuite tests resolveMonsterAttack with mocked dependencies
type ResolveMonsterAttackSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	mockRoller   *dicemock.MockRoller
	orchestrator *Orchestrator
}

func TestResolveMonsterAttackSuite(t *testing.T) {
	suite.Run(t, new(ResolveMonsterAttackSuite))
}

func (s *ResolveMonsterAttackSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.mockRoller = dicemock.NewMockRoller(s.ctrl)
	s.orchestrator = &Orchestrator{
		charRepo: s.mockCharRepo,
		roller:   s.mockRoller,
	}
}

func (s *ResolveMonsterAttackSuite) TestMonkUnarmoredDefenseAC() {
	// This test verifies that when a monster attacks a Monk with Unarmored Defense,
	// the API wires up a GameContext with the defender's ability scores so the
	// toolkit's AC chain correctly includes the WIS modifier.
	//
	// Monk stats: DEX 16 (+3), WIS 15 (+2)
	// Expected AC: 10 (base) + 3 (DEX) + 2 (WIS via Unarmored Defense) = 15
	// Bug AC: 10 (base) + 3 (DEX) = 13 (missing GameContext)

	monkCharData := &character.Data{
		ID:               "monk-defender",
		PlayerID:         "player-1",
		Name:             "Test Monk",
		Level:            1,
		ProficiencyBonus: 2,
		RaceID:           races.Human,
		ClassID:          classes.Monk,
		HitPoints:        10,
		MaxHitPoints:     10,
		ArmorClass:       13, // Base stored AC (10 + DEX), but effective should be 15
		AbilityScores: shared.AbilityScores{
			abilities.STR: 10,
			abilities.DEX: 16, // +3 modifier
			abilities.CON: 14,
			abilities.INT: 8,
			abilities.WIS: 15, // +2 modifier
			abilities.CHA: 10,
		},
		SavingThrows: map[abilities.Ability]shared.ProficiencyLevel{
			abilities.STR: shared.Proficient,
			abilities.DEX: shared.Proficient,
		},
		Conditions: []json.RawMessage{
			json.RawMessage(`{
				"ref": {"module": "dnd5e", "type": "conditions", "id": "unarmored_defense"},
				"type": "monk",
				"character_id": "monk-defender",
				"source": "dnd5e:classes:monk"
			}`),
		},
	}

	// Mock charRepo.Get to return the Monk character
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "monk-defender"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: monkCharData,
			},
		}, nil)

	// Mock charRepo.Update for persisting defender state after attack
	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		Return(&characterrepo.UpdateOutput{}, nil)

	// Mock roller: attack roll of 20 (natural 20, guaranteed hit so we can check AC)
	// ResolveAttack calls Roll for attack d20, then damage dice
	s.mockRoller.EXPECT().
		Roll(gomock.Any(), 20).
		Return(20, nil) // Natural 20

	// Damage rolls - allow any number of damage dice calls
	s.mockRoller.EXPECT().
		Roll(gomock.Any(), gomock.Any()).
		Return(4, nil).
		AnyTimes()

	s.mockRoller.EXPECT().
		RollN(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, count, size int) ([]int, error) {
			rolls := make([]int, count)
			for i := range rolls {
				rolls[i] = size / 2 // Use half the die size for predictable damage
			}
			return rolls, nil
		}).
		AnyTimes()

	// Create a goblin monster for the attacker
	goblinData := &monster.Data{
		ID:           "goblin-1",
		Name:         "Goblin Warrior",
		HitPoints:    7,
		MaxHitPoints: 7,
		ArmorClass:   15,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 8,
			abilities.DEX: 14,
			abilities.CON: 10,
			abilities.INT: 10,
			abilities.WIS: 8,
			abilities.CHA: 8,
		},
		ProficiencyBonus: 2,
	}

	bus := events.NewEventBus()
	goblin, err := monster.LoadFromData(context.Background(), goblinData, bus)
	s.Require().NoError(err)
	defer func() { _ = goblin.Cleanup(context.Background()) }()

	// Resolve the attack
	result, err := s.orchestrator.resolveMonsterAttack(
		context.Background(),
		goblin,
		goblinData,
		"monk-defender",
	)

	s.Require().NoError(err)
	s.Require().NotNil(result)

	// THE KEY ASSERTION: TargetAC should be 15 (10 + DEX 3 + WIS 2),
	// not 13 (10 + DEX 3 without GameContext)
	s.Equal(15, result.TargetAC,
		"Monk Unarmored Defense AC should be 15 (10 + DEX +3 + WIS +2), "+
			"not 13 (missing GameContext means WIS modifier is dropped)")

	// With nat 20, should always be a hit
	s.True(result.Hit, "Natural 20 should always hit")
	s.True(result.Critical, "Natural 20 should be a critical hit")
}
