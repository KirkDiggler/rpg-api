package encounter

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

type PerceptionTestSuite struct {
	suite.Suite
}

func TestPerceptionTestSuite(t *testing.T) {
	suite.Run(t, new(PerceptionTestSuite))
}

func (s *PerceptionTestSuite) TestBuildPerception_WithCharactersAndMonsters() {
	// Create a simple room with entities
	roomData := &spatial.RoomData{
		ID:       "test-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeSquare,
		Entities: map[string]spatial.EntityPlacement{
			// Monster at center
			"monster-1": {
				EntityID:   "monster-1",
				EntityType: "monster",
				Position:   spatial.Position{X: 10, Y: 10},
			},
			// Character close to monster (adjacent)
			"char-1": {
				EntityID:   "char-1",
				EntityType: "character",
				Position:   spatial.Position{X: 10, Y: 11}, // 1 square away = 5 feet
			},
			// Character farther away
			"char-2": {
				EntityID:   "char-2",
				EntityType: "character",
				Position:   spatial.Position{X: 10, Y: 14}, // 4 squares away = 20 feet
			},
			// Another monster (ally)
			"monster-2": {
				EntityID:   "monster-2",
				EntityType: "monster",
				Position:   spatial.Position{X: 12, Y: 10}, // 2 squares away = 10 feet
			},
		},
	}

	characterIDs := []string{"char-1", "char-2"}
	monsters := []*monster.Data{
		{ID: "monster-1", Name: "Goblin 1"},
		{ID: "monster-2", Name: "Goblin 2"},
	}

	// Build perception for monster-1
	perception := buildPerception(roomData, "monster-1", characterIDs, monsters)

	// Verify perception data
	s.NotNil(perception)
	s.Equal(10, perception.MyPosition.X)
	s.Equal(10, perception.MyPosition.Y)

	// Should have 2 enemies (characters)
	s.Len(perception.Enemies, 2)

	// Enemies should be sorted by distance (closest first)
	s.Equal("char-1", perception.Enemies[0].Entity.GetID())
	s.Equal(5, perception.Enemies[0].Distance)
	s.True(perception.Enemies[0].Adjacent)

	s.Equal("char-2", perception.Enemies[1].Entity.GetID())
	s.Equal(20, perception.Enemies[1].Distance)
	s.False(perception.Enemies[1].Adjacent)

	// Should have 1 ally (other monster)
	s.Len(perception.Allies, 1)
	s.Equal("monster-2", perception.Allies[0].Entity.GetID())
	s.Equal(10, perception.Allies[0].Distance)
	s.False(perception.Allies[0].Adjacent)
}

func (s *PerceptionTestSuite) TestBuildPerception_NilRoomData() {
	perception := buildPerception(nil, "monster-1", []string{"char-1"}, nil)

	s.NotNil(perception)
	s.Len(perception.Enemies, 0)
	s.Len(perception.Allies, 0)
}

func (s *PerceptionTestSuite) TestBuildPerception_MonsterNotInRoom() {
	roomData := &spatial.RoomData{
		ID:       "test-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeSquare,
		Entities: map[string]spatial.EntityPlacement{},
	}

	perception := buildPerception(roomData, "missing-monster", []string{"char-1"}, nil)

	s.NotNil(perception)
	s.Len(perception.Enemies, 0)
	s.Len(perception.Allies, 0)
}

func (s *PerceptionTestSuite) TestBuildPerception_HexGrid() {
	// Test with hex grid (HexFlatTop=false means pointy-top, which is D&D 5e default)
	roomData := &spatial.RoomData{
		ID:         "test-room",
		Type:       "dungeon",
		Width:      20,
		Height:     20,
		GridType:   spatial.GridTypeHex,
		HexFlatTop: false, // pointy-top hex grid
		Entities: map[string]spatial.EntityPlacement{
			"monster-1": {
				EntityID:   "monster-1",
				EntityType: "monster",
				Position:   spatial.Position{X: 10, Y: 10},
			},
			"char-1": {
				EntityID:   "char-1",
				EntityType: "character",
				Position:   spatial.Position{X: 11, Y: 10}, // 1 hex away
			},
		},
	}

	characterIDs := []string{"char-1"}
	monsters := []*monster.Data{
		{ID: "monster-1", Name: "Goblin"},
	}

	perception := buildPerception(roomData, "monster-1", characterIDs, monsters)

	s.NotNil(perception)
	s.Len(perception.Enemies, 1)
	s.Equal("char-1", perception.Enemies[0].Entity.GetID())
	// Hex grid distance should be calculated correctly
	s.True(perception.Enemies[0].Distance > 0)
}

func (s *PerceptionTestSuite) TestEntityAdapter() {
	adapter := &entityAdapter{
		id:         "test-id",
		entityType: "character",
	}

	s.Equal("test-id", adapter.GetID())
	s.Equal("character", string(adapter.GetType()))
}
