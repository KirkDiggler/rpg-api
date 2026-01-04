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
	// Create a room with cube entities (cube coordinates: x + y + z = 0)
	roomData := &spatial.RoomData{
		ID:       "test-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
		CubeEntities: map[string]spatial.EntityCubePlacement{
			// Monster at center
			"monster-1": {
				EntityID:   "monster-1",
				EntityType: "monster",
				CubePosition: spatial.CubeCoordinate{
					X: 10,
					Y: -20, // y = -x - z
					Z: 10,
				},
			},
			// Character close to monster (adjacent - 1 hex away)
			"char-1": {
				EntityID:   "char-1",
				EntityType: "character",
				CubePosition: spatial.CubeCoordinate{
					X: 11,
					Y: -21, // y = -x - z
					Z: 10,
				},
			},
			// Character farther away (4 hexes)
			"char-2": {
				EntityID:   "char-2",
				EntityType: "character",
				CubePosition: spatial.CubeCoordinate{
					X: 10,
					Y: -24, // y = -x - z
					Z: 14,
				},
			},
			// Another monster (ally - 2 hexes away)
			"monster-2": {
				EntityID:   "monster-2",
				EntityType: "monster",
				CubePosition: spatial.CubeCoordinate{
					X: 12,
					Y: -22, // y = -x - z
					Z: 10,
				},
			},
		},
	}

	characterIDs := []string{"char-1", "char-2"}
	monsters := []*monster.Data{
		{ID: "monster-1", Name: "Goblin 1"},
		{ID: "monster-2", Name: "Goblin 2"},
	}

	// Build perception for monster-1 (no walls in this test)
	perception := buildPerception(roomData, "monster-1", characterIDs, monsters, nil)

	// Verify perception data
	s.NotNil(perception)
	s.Equal(10, perception.MyPosition.X)
	s.Equal(-20, perception.MyPosition.Y)
	s.Equal(10, perception.MyPosition.Z)

	// Should have 2 enemies (characters)
	s.Len(perception.Enemies, 2)

	// Enemies should be sorted by distance (closest first)
	s.Equal("char-1", perception.Enemies[0].Entity.GetID())
	s.Equal(1, perception.Enemies[0].Distance) // 1 hex away
	s.True(perception.Enemies[0].Adjacent)

	s.Equal("char-2", perception.Enemies[1].Entity.GetID())
	s.Equal(4, perception.Enemies[1].Distance) // 4 hexes away
	s.False(perception.Enemies[1].Adjacent)

	// Should have 1 ally (other monster)
	s.Len(perception.Allies, 1)
	s.Equal("monster-2", perception.Allies[0].Entity.GetID())
	s.Equal(2, perception.Allies[0].Distance) // 2 hexes away
	s.False(perception.Allies[0].Adjacent)
}

func (s *PerceptionTestSuite) TestBuildPerception_NilRoomData() {
	perception := buildPerception(nil, "monster-1", []string{"char-1"}, nil, nil)

	s.NotNil(perception)
	s.Len(perception.Enemies, 0)
	s.Len(perception.Allies, 0)
}

func (s *PerceptionTestSuite) TestBuildPerception_MonsterNotInRoom() {
	roomData := &spatial.RoomData{
		ID:           "test-room",
		Type:         "dungeon",
		Width:        20,
		Height:       20,
		GridType:     spatial.GridTypeHex,
		CubeEntities: map[string]spatial.EntityCubePlacement{},
	}

	perception := buildPerception(roomData, "missing-monster", []string{"char-1"}, nil, nil)

	s.NotNil(perception)
	s.Len(perception.Enemies, 0)
	s.Len(perception.Allies, 0)
}

func (s *PerceptionTestSuite) TestBuildPerception_HexGrid() {
	// Test with hex grid using CubeEntities (cube coordinates: x + y + z = 0)
	roomData := &spatial.RoomData{
		ID:         "test-room",
		Type:       "dungeon",
		Width:      20,
		Height:     20,
		GridType:   spatial.GridTypeHex,
		HexFlatTop: false, // pointy-top hex grid
		CubeEntities: map[string]spatial.EntityCubePlacement{
			"monster-1": {
				EntityID:   "monster-1",
				EntityType: "monster",
				CubePosition: spatial.CubeCoordinate{
					X: 10,
					Y: -20, // y = -x - z
					Z: 10,
				},
			},
			"char-1": {
				EntityID:   "char-1",
				EntityType: "character",
				CubePosition: spatial.CubeCoordinate{
					X: 11,
					Y: -21, // y = -x - z
					Z: 10,
				}, // 1 hex away (dx=1, dy=1, dz=0 -> distance = 1 hex)
			},
		},
	}

	characterIDs := []string{"char-1"}
	monsters := []*monster.Data{
		{ID: "monster-1", Name: "Goblin"},
	}

	perception := buildPerception(roomData, "monster-1", characterIDs, monsters, nil)

	s.NotNil(perception)
	s.Len(perception.Enemies, 1)
	s.Equal("char-1", perception.Enemies[0].Entity.GetID())
	// Hex grid distance: (|dx| + |dy| + |dz|) / 2 = (1 + 1 + 0) / 2 = 1 hex
	s.Equal(1, perception.Enemies[0].Distance)
	s.True(perception.Enemies[0].Adjacent) // 1 hex is adjacent
}

func (s *PerceptionTestSuite) TestEntityAdapter() {
	adapter := &entityAdapter{
		id:         "test-id",
		entityType: "character",
	}

	s.Equal("test-id", adapter.GetID())
	s.Equal("character", string(adapter.GetType()))
}
