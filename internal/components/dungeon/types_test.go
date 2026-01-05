package dungeon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TypesTestSuite struct {
	suite.Suite
}

func TestTypesTestSuite(t *testing.T) {
	suite.Run(t, new(TypesTestSuite))
}

func (s *TypesTestSuite) TestGenerateInput_Instantiation() {
	input := &GenerateInput{
		Theme:     ThemeCrypt,
		Size:      RoomSizeMedium,
		Length:    4,
		Layout:    LayoutBranching,
		PartySize: 4,
		TargetCR:  3,
		Seed:      12345,
	}

	assert.NotNil(s.T(), input)
	assert.Equal(s.T(), ThemeCrypt, input.Theme)
	assert.Equal(s.T(), RoomSizeMedium, input.Size)
	assert.Equal(s.T(), 4, input.Length)
	assert.Equal(s.T(), LayoutBranching, input.Layout)
	assert.Equal(s.T(), 4, input.PartySize)
	assert.Equal(s.T(), 3, input.TargetCR)
	assert.Equal(s.T(), int64(12345), input.Seed)
}

func (s *TypesTestSuite) TestGenerateOutput_Instantiation() {
	output := &GenerateOutput{
		Dungeon: &Dungeon{
			ID:          "dungeon-1",
			StartRoom:   "room-1",
			BossRoom:    "room-4",
			Rooms:       []*Room{},
			Connections: []*RoomConnection{},
		},
		Seed: 12345,
	}

	assert.NotNil(s.T(), output)
	assert.NotNil(s.T(), output.Dungeon)
	assert.Equal(s.T(), "dungeon-1", output.Dungeon.ID)
	assert.Equal(s.T(), "room-1", output.Dungeon.StartRoom)
	assert.Equal(s.T(), "room-4", output.Dungeon.BossRoom)
	assert.Equal(s.T(), int64(12345), output.Seed)
}

func (s *TypesTestSuite) TestDungeon_Instantiation() {
	dungeon := &Dungeon{
		ID:        "dungeon-1",
		Theme:     ThemeCave,
		StartRoom: "room-1",
		BossRoom:  "room-4",
		Rooms: []*Room{
			{ID: "room-1"},
			{ID: "room-2"},
			{ID: "room-3"},
			{ID: "room-4"},
		},
		Connections: []*RoomConnection{
			{FromRoom: "room-1", ToRoom: "room-2"},
		},
	}

	assert.NotNil(s.T(), dungeon)
	assert.Equal(s.T(), "dungeon-1", dungeon.ID)
	assert.Equal(s.T(), ThemeCave, dungeon.Theme)
	assert.Equal(s.T(), 4, len(dungeon.Rooms))
	assert.Equal(s.T(), 1, len(dungeon.Connections))
}

func (s *TypesTestSuite) TestRoom_Instantiation() {
	room := &Room{
		ID: "room-1",
		Shape: &Shape{
			Bounds:   []Position{{X: 0, Y: 0, Z: 0}},
			GridType: GridTypeHex,
			Width:    20,
			Height:   15,
			Area:     300,
		},
		Features: &FeatureLayout{
			Obstacles: []Obstacle{},
			Terrain:   []TerrainPatch{},
		},
		SpawnZones: []*Zone{},
		Encounter: &Encounter{
			Monsters: []MonsterPlacement{},
			TotalCR:  2.5,
		},
	}

	assert.NotNil(s.T(), room)
	assert.Equal(s.T(), "room-1", room.ID)
	assert.NotNil(s.T(), room.Shape)
	assert.NotNil(s.T(), room.Features)
	assert.NotNil(s.T(), room.Encounter)
	assert.Equal(s.T(), 2.5, room.Encounter.TotalCR)
}

func (s *TypesTestSuite) TestShape_Instantiation() {
	shape := &Shape{
		Bounds: []Position{
			{X: 0, Y: 0, Z: 0},
			{X: 10, Y: -10, Z: 0},
			{X: 10, Y: -10, Z: 0},
			{X: 0, Y: 0, Z: 0},
		},
		GridType: GridTypeHex,
		Width:    20,
		Height:   15,
		Area:     300,
	}

	assert.NotNil(s.T(), shape)
	assert.Equal(s.T(), GridTypeHex, shape.GridType)
	assert.Equal(s.T(), 20, shape.Width)
	assert.Equal(s.T(), 15, shape.Height)
	assert.Equal(s.T(), 300, shape.Area)
	assert.Equal(s.T(), 4, len(shape.Bounds))
}

func (s *TypesTestSuite) TestFeatureLayout_Instantiation() {
	layout := &FeatureLayout{
		Obstacles: []Obstacle{
			{
				ID:                "obstacle-1",
				Type:              ObstacleTypePillar,
				Position:          Position{X: 5, Y: -5, Z: 0},
				BlocksMovement:    true,
				BlocksLineOfSight: true,
			},
		},
		Terrain: []TerrainPatch{
			{
				ID:           "terrain-1",
				Type:         TerrainTypeDifficult,
				Bounds:       []Position{{X: 0, Y: 0, Z: 0}},
				MovementCost: 2.0,
			},
		},
	}

	assert.NotNil(s.T(), layout)
	assert.Equal(s.T(), 1, len(layout.Obstacles))
	assert.Equal(s.T(), 1, len(layout.Terrain))
	assert.Equal(s.T(), ObstacleTypePillar, layout.Obstacles[0].Type)
	assert.True(s.T(), layout.Obstacles[0].BlocksMovement)
}

func (s *TypesTestSuite) TestZone_Instantiation() {
	zone := &Zone{
		ID:   "zone-1",
		Type: ZoneTypeMonsterSpawn,
		Bounds: []Position{
			{X: 0, Y: 0, Z: 0},
			{X: 5, Y: -5, Z: 0},
		},
		Capacity: 8,
	}

	assert.NotNil(s.T(), zone)
	assert.Equal(s.T(), "zone-1", zone.ID)
	assert.Equal(s.T(), ZoneTypeMonsterSpawn, zone.Type)
	assert.Equal(s.T(), 8, zone.Capacity)
	assert.Equal(s.T(), 2, len(zone.Bounds))
}

func (s *TypesTestSuite) TestEncounter_Instantiation() {
	encounter := &Encounter{
		Monsters: []MonsterPlacement{
			{
				ID:        "monster-1",
				MonsterID: "skeleton",
				Role:      RoleMelee,
				Position:  Position{X: 10, Y: -10, Z: 0},
				CR:        0.25,
			},
			{
				ID:        "monster-2",
				MonsterID: "skeleton-archer",
				Role:      RoleRanged,
				Position:  Position{X: 12, Y: -12, Z: 0},
				CR:        0.25,
			},
		},
		TotalCR: 0.5,
	}

	assert.NotNil(s.T(), encounter)
	assert.Equal(s.T(), 2, len(encounter.Monsters))
	assert.Equal(s.T(), 0.5, encounter.TotalCR)
	assert.Equal(s.T(), "skeleton", encounter.Monsters[0].MonsterID)
	assert.Equal(s.T(), RoleMelee, encounter.Monsters[0].Role)
}

func (s *TypesTestSuite) TestMonsterPlacement_Instantiation() {
	placement := &MonsterPlacement{
		ID:        "monster-1",
		MonsterID: "goblin",
		Role:      RoleMelee,
		Position:  Position{X: 5, Y: -5, Z: 0},
		CR:        0.25,
	}

	assert.NotNil(s.T(), placement)
	assert.Equal(s.T(), "monster-1", placement.ID)
	assert.Equal(s.T(), "goblin", placement.MonsterID)
	assert.Equal(s.T(), RoleMelee, placement.Role)
	assert.Equal(s.T(), 0.25, placement.CR)
}

func (s *TypesTestSuite) TestRoomConnection_Instantiation() {
	connection := &RoomConnection{
		FromRoom:     "room-1",
		ToRoom:       "room-2",
		Type:         ConnectionTypeDoor,
		IsMainPath:   true,
		PhysicalHint: "north door",
	}

	assert.NotNil(s.T(), connection)
	assert.Equal(s.T(), "room-1", connection.FromRoom)
	assert.Equal(s.T(), "room-2", connection.ToRoom)
	assert.Equal(s.T(), ConnectionTypeDoor, connection.Type)
	assert.True(s.T(), connection.IsMainPath)
	assert.Equal(s.T(), "north door", connection.PhysicalHint)
}

func (s *TypesTestSuite) TestRoomSize_Values() {
	assert.Equal(s.T(), RoomSize(0), RoomSizeSmall)
	assert.Equal(s.T(), RoomSize(1), RoomSizeMedium)
	assert.Equal(s.T(), RoomSize(2), RoomSizeLarge)
}

func (s *TypesTestSuite) TestLayoutType_Values() {
	assert.Equal(s.T(), LayoutType(0), LayoutLinear)
	assert.Equal(s.T(), LayoutType(1), LayoutBranching)
	assert.Equal(s.T(), LayoutType(2), LayoutHub)
}

func (s *TypesTestSuite) TestShapeStyle_Values() {
	assert.Equal(s.T(), ShapeStyle(0), ShapeStyleStructured)
	assert.Equal(s.T(), ShapeStyle(1), ShapeStyleOrganic)
	assert.Equal(s.T(), ShapeStyle(2), ShapeStyleMixed)
}

func (s *TypesTestSuite) TestZoneType_Values() {
	assert.Equal(s.T(), ZoneType(0), ZoneTypePlayerSpawn)
	assert.Equal(s.T(), ZoneType(1), ZoneTypeMonsterSpawn)
	assert.Equal(s.T(), ZoneType(2), ZoneTypeBoss)
	assert.Equal(s.T(), ZoneType(3), ZoneTypeEntrance)
	assert.Equal(s.T(), ZoneType(4), ZoneTypeExit)
}

func (s *TypesTestSuite) TestMonsterRole_Values() {
	assert.Equal(s.T(), MonsterRole(0), RoleMelee)
	assert.Equal(s.T(), MonsterRole(1), RoleRanged)
	assert.Equal(s.T(), MonsterRole(2), RoleSupport)
	assert.Equal(s.T(), MonsterRole(3), RoleBoss)
}

func (s *TypesTestSuite) TestConnectionType_Values() {
	assert.Equal(s.T(), ConnectionType(0), ConnectionTypeDoor)
	assert.Equal(s.T(), ConnectionType(1), ConnectionTypeStairs)
	assert.Equal(s.T(), ConnectionType(2), ConnectionTypePassage)
}

func (s *TypesTestSuite) TestGridType_Values() {
	assert.Equal(s.T(), GridType(0), GridTypeHex)
	assert.Equal(s.T(), GridType(1), GridTypeSquare)
}

func (s *TypesTestSuite) TestObstacleType_Values() {
	assert.Equal(s.T(), ObstacleType(0), ObstacleTypePillar)
	assert.Equal(s.T(), ObstacleType(1), ObstacleTypeSarcophagus)
	assert.Equal(s.T(), ObstacleType(2), ObstacleTypeAltar)
	assert.Equal(s.T(), ObstacleType(3), ObstacleTypeBoulder)
	assert.Equal(s.T(), ObstacleType(4), ObstacleTypeStalagmite)
	assert.Equal(s.T(), ObstacleType(5), ObstacleTypePool)
	assert.Equal(s.T(), ObstacleType(6), ObstacleTypeCrate)
	assert.Equal(s.T(), ObstacleType(7), ObstacleTypeBarrel)
}

func (s *TypesTestSuite) TestTerrainType_Values() {
	assert.Equal(s.T(), TerrainType(0), TerrainTypeDifficult)
	assert.Equal(s.T(), TerrainType(1), TerrainTypeHazardous)
	assert.Equal(s.T(), TerrainType(2), TerrainTypeWater)
	assert.Equal(s.T(), TerrainType(3), TerrainTypeLava)
	assert.Equal(s.T(), TerrainType(4), TerrainTypeIce)
}
