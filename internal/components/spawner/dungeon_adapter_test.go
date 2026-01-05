package spawner

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type DungeonAdapterTestSuite struct {
	suite.Suite
	spawner *DefaultSpawner
}

func TestDungeonAdapterTestSuite(t *testing.T) {
	suite.Run(t, new(DungeonAdapterTestSuite))
}

func (s *DungeonAdapterTestSuite) SetupTest() {
	s.spawner = NewSpawner()
}

func (s *DungeonAdapterTestSuite) TestSpawnInRoom_NilInput() {
	output, err := SpawnInRoom(s.spawner, nil)

	s.Require().NoError(err)
	s.Assert().NotNil(output)
	s.Assert().Empty(output.Placements)
}

func (s *DungeonAdapterTestSuite) TestSpawnInRoom_NilRoom() {
	output, err := SpawnInRoom(s.spawner, &DungeonSpawnInput{
		Room: nil,
	})

	s.Require().NoError(err)
	s.Assert().NotNil(output)
	s.Assert().Empty(output.Placements)
}

func (s *DungeonAdapterTestSuite) TestSpawnInRoom_SpawnsMonsters() {
	room := &dungeon.Room{
		ID: "test-room",
		Shape: &dungeon.Shape{
			Width:  10,
			Height: 10,
		},
		SpawnZones: []*dungeon.Zone{
			{
				ID:   "monster-zone",
				Type: dungeon.ZoneTypeMonsterSpawn,
				Bounds: []dungeon.Position{
					{X: 5, Y: -10, Z: 5},
					{X: 6, Y: -12, Z: 6},
				},
				Capacity: 2,
			},
		},
	}

	output, err := SpawnInRoom(s.spawner, &DungeonSpawnInput{
		Room:            room,
		EntitiesToSpawn: CreateMonsterSpawnEntities([]string{"monster-1", "monster-2"}),
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 2)
	s.Assert().Empty(output.Errors)
}

func (s *DungeonAdapterTestSuite) TestSpawnInRoom_AvoidsOccupiedPositions() {
	occupiedPos := dungeon.Position{X: 5, Y: -10, Z: 5}
	freePos := dungeon.Position{X: 6, Y: -12, Z: 6}

	room := &dungeon.Room{
		ID: "test-room",
		Shape: &dungeon.Shape{
			Width:  10,
			Height: 10,
		},
		SpawnZones: []*dungeon.Zone{
			{
				ID:   "monster-zone",
				Type: dungeon.ZoneTypeMonsterSpawn,
				Bounds: []dungeon.Position{
					occupiedPos,
					freePos,
				},
				Capacity: 2,
			},
		},
	}

	output, err := SpawnInRoom(s.spawner, &DungeonSpawnInput{
		Room: room,
		OccupiedPositions: []dungeon.Position{
			occupiedPos,
		},
		EntitiesToSpawn: CreateMonsterSpawnEntities([]string{"monster-1"}),
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 1)

	// Should have spawned at the free position
	s.Assert().Equal(freePos.X, output.Placements[0].Position.X)
	s.Assert().Equal(freePos.Y, output.Placements[0].Position.Y)
	s.Assert().Equal(freePos.Z, output.Placements[0].Position.Z)
}

func (s *DungeonAdapterTestSuite) TestSpawnInRoom_SpawnsPlayers() {
	room := &dungeon.Room{
		ID: "test-room",
		Shape: &dungeon.Shape{
			Width:  10,
			Height: 10,
		},
		SpawnZones: []*dungeon.Zone{
			{
				ID:   "player-zone",
				Type: dungeon.ZoneTypePlayerSpawn,
				Bounds: []dungeon.Position{
					{X: 2, Y: -4, Z: 2},
					{X: 3, Y: -6, Z: 3},
					{X: 4, Y: -8, Z: 4},
					{X: 5, Y: -10, Z: 5},
				},
				Capacity: 4,
			},
		},
	}

	output, err := SpawnInRoom(s.spawner, &DungeonSpawnInput{
		Room:            room,
		EntitiesToSpawn: CreatePlayerSpawnEntities([]string{"player-1", "player-2"}),
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 2)
	s.Assert().Empty(output.Errors)
}

func (s *DungeonAdapterTestSuite) TestSpawnInRoom_HandlesWalls() {
	wallPos := dungeon.Position{X: 5, Y: -10, Z: 5}
	freePos := dungeon.Position{X: 6, Y: -12, Z: 6}

	room := &dungeon.Room{
		ID: "test-room",
		Shape: &dungeon.Shape{
			Width:  10,
			Height: 10,
		},
		SpawnZones: []*dungeon.Zone{
			{
				ID:   "monster-zone",
				Type: dungeon.ZoneTypeMonsterSpawn,
				Bounds: []dungeon.Position{
					wallPos,
					freePos,
				},
				Capacity: 2,
			},
		},
		Walls: []dungeon.WallSegment{
			{
				ID:             "wall-1",
				Start:          wallPos,
				End:            wallPos,
				BlocksMovement: true,
			},
		},
	}

	output, err := SpawnInRoom(s.spawner, &DungeonSpawnInput{
		Room:            room,
		EntitiesToSpawn: CreateMonsterSpawnEntities([]string{"monster-1"}),
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Assert().Len(output.Placements, 1)

	// Should have spawned at the free position, not the wall
	s.Assert().Equal(freePos.X, output.Placements[0].Position.X)
	s.Assert().Equal(freePos.Y, output.Placements[0].Position.Y)
	s.Assert().Equal(freePos.Z, output.Placements[0].Position.Z)
}

func (s *DungeonAdapterTestSuite) TestCreateMonsterSpawnEntities() {
	entities := CreateMonsterSpawnEntities([]string{"m1", "m2", "m3"})

	s.Assert().Len(entities, 3)
	for i, e := range entities {
		s.Assert().Equal(EntityTypeMonster, e.Type)
		s.Assert().Equal(ZoneTypeMonsterSpawn, e.PreferredZone)
		s.Assert().Equal(1, e.Size)
		s.Assert().Equal([]string{"m1", "m2", "m3"}[i], e.ID)
	}
}

func (s *DungeonAdapterTestSuite) TestCreatePlayerSpawnEntities() {
	entities := CreatePlayerSpawnEntities([]string{"p1", "p2"})

	s.Assert().Len(entities, 2)
	for i, e := range entities {
		s.Assert().Equal(EntityTypePlayer, e.Type)
		s.Assert().Equal(ZoneTypePlayerSpawn, e.PreferredZone)
		s.Assert().Equal(1, e.Size)
		s.Assert().Equal([]string{"p1", "p2"}[i], e.ID)
	}
}
