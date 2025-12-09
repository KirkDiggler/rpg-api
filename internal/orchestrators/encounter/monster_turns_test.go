package encounter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

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
