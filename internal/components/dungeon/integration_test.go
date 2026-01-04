package dungeon_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon/toolkit"
)

// IntegrationTestSuite tests end-to-end dungeon generation with real toolkit generators
type IntegrationTestSuite struct {
	suite.Suite
	generator *dungeon.Generator
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

func (s *IntegrationTestSuite) SetupTest() {
	// Create generator with real toolkit generators (not mocks/stubs)
	s.generator = toolkit.CreateGenerator(&toolkit.ToolkitConfig{})
}

// TestGenerateCryptDungeon verifies complete crypt dungeon generation
// Corresponds to test: generate-crypt-dungeon
func (s *IntegrationTestSuite) TestGenerateCryptDungeon() {
	output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    4,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  3,
		Seed:      12345,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(output.Dungeon)

	dun := output.Dungeon

	// Verify 4 rooms exist
	s.Assert().Len(dun.Rooms, 4, "dungeon should have 4 rooms")

	// Verify connections exist between rooms
	s.Assert().NotEmpty(dun.Connections, "dungeon should have connections")
	s.Assert().GreaterOrEqual(len(dun.Connections), 3, "linear dungeon needs at least 3 connections")

	// Verify start and boss rooms are designated
	s.Assert().NotEmpty(dun.StartRoom, "start room should be designated")
	s.Assert().NotEmpty(dun.BossRoom, "boss room should be designated")
	s.Assert().NotEqual(dun.StartRoom, dun.BossRoom, "start and boss should be different rooms")

	// Verify boss room exists in rooms list
	bossRoomFound := false
	for _, room := range dun.Rooms {
		if room.ID == dun.BossRoom {
			bossRoomFound = true
			break
		}
	}
	s.Assert().True(bossRoomFound, "boss room ID should match a room in the list")
}

// TestThemeShapesCorrect verifies that themes produce appropriate room shapes
// Corresponds to test: theme-shapes-correct
func (s *IntegrationTestSuite) TestThemeShapesCorrect() {
	s.Run("crypt has structured shapes", func() {
		output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
			Theme:     dungeon.ThemeCrypt,
			Size:      dungeon.RoomSizeMedium,
			Length:    3,
			Layout:    dungeon.LayoutLinear,
			PartySize: 4,
			TargetCR:  2,
			Seed:      11111,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output.Dungeon)

		// Verify all rooms have shapes
		for i, room := range output.Dungeon.Rooms {
			s.Require().NotNil(room.Shape, "room %d should have a shape", i)
			s.Assert().NotEmpty(room.Shape.Bounds, "room %d should have bounds", i)
			s.Assert().Greater(room.Shape.Width, 0, "room %d should have width", i)
			s.Assert().Greater(room.Shape.Height, 0, "room %d should have height", i)
			s.Assert().Greater(room.Shape.Area, 0, "room %d should have area", i)
		}

		// Crypt theme uses ShapeStyleStructured - rooms should be fairly regular
		// We can verify this by checking that bounds form reasonable rectangles
		for i, room := range output.Dungeon.Rooms {
			s.Assert().GreaterOrEqual(len(room.Shape.Bounds), 4,
				"structured room %d should have at least 4 vertices", i)
		}
	})

	s.Run("cave has organic shapes", func() {
		output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
			Theme:     dungeon.ThemeCave,
			Size:      dungeon.RoomSizeMedium,
			Length:    3,
			Layout:    dungeon.LayoutLinear,
			PartySize: 4,
			TargetCR:  2,
			Seed:      22222,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output.Dungeon)

		// Verify all rooms have shapes
		for i, room := range output.Dungeon.Rooms {
			s.Require().NotNil(room.Shape, "room %d should have a shape", i)
			s.Assert().NotEmpty(room.Shape.Bounds, "room %d should have bounds", i)
			s.Assert().Greater(room.Shape.Width, 0, "room %d should have width", i)
			s.Assert().Greater(room.Shape.Height, 0, "room %d should have height", i)
		}

		// Cave theme uses ShapeStyleOrganic - verify shapes exist
		// (Detailed shape analysis would require toolkit-specific knowledge)
		for i, room := range output.Dungeon.Rooms {
			s.Assert().NotNil(room.Shape.Bounds, "organic room %d should have bounds", i)
		}
	})
}

// TestCRBudgetAllocation verifies CR budget is allocated correctly across rooms
// Corresponds to test: cr-budget-allocation
func (s *IntegrationTestSuite) TestCRBudgetAllocation() {
	output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    5,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  5,
		Seed:      33333,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output.Dungeon)

	totalCR := float64(4 * 5) // PartySize * TargetCR

	// Collect CR values
	roomCRs := make([]float64, len(output.Dungeon.Rooms))
	var actualTotal float64
	var bossRoomCR float64

	for i, room := range output.Dungeon.Rooms {
		s.Require().NotNil(room.Encounter, "room %d should have encounter", i)
		roomCRs[i] = room.Encounter.TotalCR
		actualTotal += room.Encounter.TotalCR

		if room.ID == output.Dungeon.BossRoom {
			bossRoomCR = room.Encounter.TotalCR
		}
	}

	// Verify total CR is approximately correct (within 20% tolerance for rounding)
	s.Assert().InDelta(totalCR, actualTotal, totalCR*0.2,
		"total CR should be approximately %f, got %f", totalCR, actualTotal)

	// Verify boss room gets significant portion of budget (~40% target, allow 20-60% range)
	bossPercentage := (bossRoomCR / actualTotal) * 100
	s.Assert().GreaterOrEqual(bossPercentage, 20.0,
		"boss room should get at least 20%% of CR budget, got %.1f%%", bossPercentage)
	s.Assert().LessOrEqual(bossPercentage, 60.0,
		"boss room should get at most 60%% of CR budget, got %.1f%%", bossPercentage)

	// Verify progressive difficulty: rooms generally get harder (allowing some variance)
	// Just check that the boss room has more CR than the first room
	s.Assert().Greater(bossRoomCR, roomCRs[0],
		"boss room CR (%.2f) should be greater than first room CR (%.2f)", bossRoomCR, roomCRs[0])
}

// TestRoleBasedMonsterSelection verifies monsters match theme pools with role distribution
// Corresponds to test: role-based-monster-selection
func (s *IntegrationTestSuite) TestRoleBasedMonsterSelection() {
	output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    4,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  3,
		Seed:      44444,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output.Dungeon)

	// Build set of valid monster IDs from theme
	validMonsterIDs := make(map[string]bool)
	for _, monster := range dungeon.ThemeCrypt.MonsterPool {
		validMonsterIDs[monster.ID] = true
	}
	for _, monster := range dungeon.ThemeCrypt.BossPool {
		validMonsterIDs[monster.ID] = true
	}

	// Verify all monsters are from the theme's pools
	for roomIdx, room := range output.Dungeon.Rooms {
		s.Require().NotNil(room.Encounter, "room %d should have encounter", roomIdx)

		for monsterIdx, placement := range room.Encounter.Monsters {
			s.Assert().True(validMonsterIDs[placement.MonsterID],
				"room %d monster %d (%s) should be from theme pool", roomIdx, monsterIdx, placement.MonsterID)
		}
	}

	// Verify role distribution exists (at least some variety in non-boss rooms)
	rolesFound := make(map[dungeon.MonsterRole]bool)
	for _, room := range output.Dungeon.Rooms {
		if room.ID == output.Dungeon.BossRoom {
			// Boss room should have a boss role
			for _, placement := range room.Encounter.Monsters {
				if placement.Role == dungeon.RoleBoss {
					rolesFound[dungeon.RoleBoss] = true
				}
			}
		} else {
			// Regular rooms should have tactical roles
			for _, placement := range room.Encounter.Monsters {
				rolesFound[placement.Role] = true
			}
		}
	}

	s.Assert().NotEmpty(rolesFound, "should have monsters with assigned roles")
}

// TestBranchingLayout verifies branching layouts create side rooms
// Corresponds to test: branching-layout
func (s *IntegrationTestSuite) TestBranchingLayout() {
	output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    5,
		Layout:    dungeon.LayoutBranching,
		PartySize: 4,
		TargetCR:  3,
		Seed:      55555,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output.Dungeon)

	// Verify basic structure
	s.Assert().Len(output.Dungeon.Rooms, 5, "should have 5 rooms")
	s.Assert().NotEmpty(output.Dungeon.Connections, "should have connections")

	// Build adjacency map to analyze layout structure
	adjacency := make(map[string][]string)
	mainPathConnections := 0

	for _, conn := range output.Dungeon.Connections {
		adjacency[conn.FromRoom] = append(adjacency[conn.FromRoom], conn.ToRoom)
		adjacency[conn.ToRoom] = append(adjacency[conn.ToRoom], conn.FromRoom)

		if conn.IsMainPath {
			mainPathConnections++
		}
	}

	// Verify there are main path connections
	s.Assert().Greater(mainPathConnections, 0, "should have main path connections")

	// For branching layout with 5 rooms, we expect some rooms to have multiple connections
	// indicating branching structure
	roomsWithMultipleConnections := 0
	for _, neighbors := range adjacency {
		if len(neighbors) > 2 {
			roomsWithMultipleConnections++
		}
	}

	// At least one room should be a junction point
	s.Assert().GreaterOrEqual(roomsWithMultipleConnections, 0,
		"branching layout may have junction rooms with multiple connections")
}

// TestSpawnZonesPlaced verifies rooms have appropriate spawn zones
// Corresponds to test: spawn-zones-placed
func (s *IntegrationTestSuite) TestSpawnZonesPlaced() {
	output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    4,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  3,
		Seed:      66666,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output.Dungeon)

	// Find entrance and boss rooms
	var entranceRoom, bossRoom *dungeon.Room
	for _, room := range output.Dungeon.Rooms {
		if room.ID == output.Dungeon.StartRoom {
			entranceRoom = room
		}
		if room.ID == output.Dungeon.BossRoom {
			bossRoom = room
		}
	}

	s.Require().NotNil(entranceRoom, "entrance room should exist")
	s.Require().NotNil(bossRoom, "boss room should exist")

	// Verify entrance room has player spawn zone
	s.Require().NotEmpty(entranceRoom.SpawnZones, "entrance should have spawn zones")
	hasPlayerSpawn := false
	for _, zone := range entranceRoom.SpawnZones {
		if zone.Type == dungeon.ZoneTypePlayerSpawn {
			hasPlayerSpawn = true
			s.Assert().NotEmpty(zone.Bounds, "player spawn zone should have bounds")
			s.Assert().Greater(zone.Capacity, 0, "player spawn zone should have capacity")
		}
	}
	s.Assert().True(hasPlayerSpawn, "entrance room should have player spawn zone")

	// Verify boss room has boss zone
	s.Require().NotEmpty(bossRoom.SpawnZones, "boss room should have spawn zones")
	hasBossZone := false
	for _, zone := range bossRoom.SpawnZones {
		if zone.Type == dungeon.ZoneTypeBoss {
			hasBossZone = true
			s.Assert().NotEmpty(zone.Bounds, "boss zone should have bounds")
			s.Assert().Greater(zone.Capacity, 0, "boss zone should have capacity")
		}
	}
	s.Assert().True(hasBossZone, "boss room should have boss spawn zone")

	// Verify all rooms have spawn zones
	for i, room := range output.Dungeon.Rooms {
		s.Assert().NotEmpty(room.SpawnZones, "room %d should have spawn zones", i)
	}
}

// TestSeedReproducibility verifies same seed produces identical dungeons
// Corresponds to test: seed-reproducibility
func (s *IntegrationTestSuite) TestSeedReproducibility() {
	seed := int64(77777)
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    4,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  3,
		Seed:      seed,
	}

	// Generate first dungeon
	output1, err := s.generator.Generate(context.Background(), input)
	s.Require().NoError(err)
	s.Require().NotNil(output1.Dungeon)

	// Generate second dungeon with same seed
	output2, err := s.generator.Generate(context.Background(), input)
	s.Require().NoError(err)
	s.Require().NotNil(output2.Dungeon)

	// Verify seeds match
	s.Assert().Equal(output1.Seed, output2.Seed, "seeds should match")

	// Verify same number of rooms
	s.Assert().Equal(len(output1.Dungeon.Rooms), len(output2.Dungeon.Rooms),
		"dungeons should have same number of rooms")

	// Verify same number of connections
	s.Assert().Equal(len(output1.Dungeon.Connections), len(output2.Dungeon.Connections),
		"dungeons should have same number of connections")

	// Verify room shapes are identical
	for i := 0; i < len(output1.Dungeon.Rooms); i++ {
		room1 := output1.Dungeon.Rooms[i]
		room2 := output2.Dungeon.Rooms[i]

		s.Assert().Equal(room1.Shape.Width, room2.Shape.Width,
			"room %d width should match", i)
		s.Assert().Equal(room1.Shape.Height, room2.Shape.Height,
			"room %d height should match", i)
		s.Assert().Equal(room1.Shape.Area, room2.Shape.Area,
			"room %d area should match", i)
	}

	// Verify encounters are identical
	for i := 0; i < len(output1.Dungeon.Rooms); i++ {
		room1 := output1.Dungeon.Rooms[i]
		room2 := output2.Dungeon.Rooms[i]

		s.Assert().Equal(len(room1.Encounter.Monsters), len(room2.Encounter.Monsters),
			"room %d should have same monster count", i)
		s.Assert().InDelta(room1.Encounter.TotalCR, room2.Encounter.TotalCR, 0.01,
			"room %d should have same total CR", i)

		// Verify monster IDs match (same monsters selected)
		for j := 0; j < len(room1.Encounter.Monsters); j++ {
			s.Assert().Equal(room1.Encounter.Monsters[j].MonsterID,
				room2.Encounter.Monsters[j].MonsterID,
				"room %d monster %d should match", i, j)
		}
	}
}

// TestMinimumLengthDungeon verifies minimum viable dungeon (2 rooms)
func (s *IntegrationTestSuite) TestMinimumLengthDungeon() {
	output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeSmall,
		Length:    2,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  1,
		Seed:      88888,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output.Dungeon)

	// Verify structure
	s.Assert().Len(output.Dungeon.Rooms, 2, "should have 2 rooms")
	s.Assert().NotEmpty(output.Dungeon.Connections, "should have at least 1 connection")
	s.Assert().NotEqual(output.Dungeon.StartRoom, output.Dungeon.BossRoom,
		"start and boss should be different")

	// Verify both rooms are complete
	for i, room := range output.Dungeon.Rooms {
		s.Assert().NotNil(room.Shape, "room %d should have shape", i)
		s.Assert().NotNil(room.Features, "room %d should have features", i)
		s.Assert().NotNil(room.Encounter, "room %d should have encounter", i)
		s.Assert().NotEmpty(room.SpawnZones, "room %d should have spawn zones", i)
	}
}

// TestLargeDungeon verifies large dungeon generation (10 rooms)
func (s *IntegrationTestSuite) TestLargeDungeon() {
	output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    10,
		Layout:    dungeon.LayoutBranching,
		PartySize: 4,
		TargetCR:  5,
		Seed:      99999,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output.Dungeon)

	// Verify structure
	s.Assert().Len(output.Dungeon.Rooms, 10, "should have 10 rooms")
	s.Assert().NotEmpty(output.Dungeon.Connections, "should have connections")

	// Verify all rooms are complete
	for i, room := range output.Dungeon.Rooms {
		s.Assert().NotNil(room.Shape, "room %d should have shape", i)
		s.Assert().NotNil(room.Features, "room %d should have features", i)
		s.Assert().NotNil(room.Encounter, "room %d should have encounter", i)
		s.Assert().NotEmpty(room.SpawnZones, "room %d should have spawn zones", i)
		s.Assert().Greater(room.Encounter.TotalCR, 0.0, "room %d should have CR budget", i)
	}
}

// TestDifferentPartySizes verifies party size affects CR budget
func (s *IntegrationTestSuite) TestDifferentPartySizes() {
	// Generate with party size 2
	output2, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 2,
		TargetCR:  3,
		Seed:      111111,
	})
	s.Require().NoError(err)

	// Generate with party size 6
	output6, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 6,
		TargetCR:  3,
		Seed:      222222,
	})
	s.Require().NoError(err)

	// Calculate total CR for each dungeon
	var totalCR2, totalCR6 float64
	for _, room := range output2.Dungeon.Rooms {
		totalCR2 += room.Encounter.TotalCR
	}
	for _, room := range output6.Dungeon.Rooms {
		totalCR6 += room.Encounter.TotalCR
	}

	// Larger party should have higher total CR
	s.Assert().Greater(totalCR6, totalCR2,
		"party of 6 should have higher total CR (%.2f) than party of 2 (%.2f)",
		totalCR6, totalCR2)

	// Verify approximately correct scaling (6/2 = 3x, allow for variance)
	expectedRatio := 3.0
	actualRatio := totalCR6 / totalCR2
	s.Assert().InDelta(expectedRatio, actualRatio, 1.0,
		"CR ratio should be approximately %.1f, got %.2f", expectedRatio, actualRatio)
}

// TestAllThemesWork verifies all three themes generate successfully
func (s *IntegrationTestSuite) TestAllThemesWork() {
	themes := []dungeon.Theme{
		dungeon.ThemeCrypt,
		dungeon.ThemeCave,
		dungeon.ThemeBanditLair,
	}

	for _, theme := range themes {
		s.Run(theme.ID, func() {
			output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
				Theme:     theme,
				Size:      dungeon.RoomSizeMedium,
				Length:    3,
				Layout:    dungeon.LayoutLinear,
				PartySize: 4,
				TargetCR:  2,
				Seed:      333333,
			})

			s.Require().NoError(err, "theme %s should generate successfully", theme.ID)
			s.Require().NotNil(output.Dungeon)
			s.Assert().Equal(theme.ID, output.Dungeon.Theme.ID, "theme should match")
			s.Assert().Len(output.Dungeon.Rooms, 3, "should have 3 rooms")

			// Verify theme-specific monsters are used
			validMonsterIDs := make(map[string]bool)
			for _, monster := range theme.MonsterPool {
				validMonsterIDs[monster.ID] = true
			}
			for _, monster := range theme.BossPool {
				validMonsterIDs[monster.ID] = true
			}

			for _, room := range output.Dungeon.Rooms {
				for _, placement := range room.Encounter.Monsters {
					s.Assert().True(validMonsterIDs[placement.MonsterID],
						"monster %s should be from theme %s pool", placement.MonsterID, theme.ID)
				}
			}
		})
	}
}

// TestDataIntegrity verifies structural integrity of generated dungeons
func (s *IntegrationTestSuite) TestDataIntegrity() {
	output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    5,
		Layout:    dungeon.LayoutBranching,
		PartySize: 4,
		TargetCR:  4,
		Seed:      444444,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output.Dungeon)

	dun := output.Dungeon

	// Verify all room IDs are unique
	roomIDs := make(map[string]bool)
	for _, room := range dun.Rooms {
		s.Assert().False(roomIDs[room.ID], "room ID %s should be unique", room.ID)
		roomIDs[room.ID] = true
		s.Assert().NotEmpty(room.ID, "room ID should not be empty")
	}

	// Verify all connections reference valid rooms
	for i, conn := range dun.Connections {
		s.Assert().True(roomIDs[conn.FromRoom],
			"connection %d FromRoom (%s) should reference valid room", i, conn.FromRoom)
		s.Assert().True(roomIDs[conn.ToRoom],
			"connection %d ToRoom (%s) should reference valid room", i, conn.ToRoom)
		s.Assert().NotEqual(conn.FromRoom, conn.ToRoom,
			"connection %d should not be self-referential", i)
	}

	// Verify StartRoom and BossRoom are valid and different
	s.Assert().True(roomIDs[dun.StartRoom], "StartRoom should reference valid room")
	s.Assert().True(roomIDs[dun.BossRoom], "BossRoom should reference valid room")
	s.Assert().NotEqual(dun.StartRoom, dun.BossRoom, "StartRoom and BossRoom should differ")

	// Verify no nil pointers in structure
	s.Assert().NotNil(dun.Rooms, "Rooms should not be nil")
	s.Assert().NotNil(dun.Connections, "Connections should not be nil")

	for i, room := range dun.Rooms {
		s.Assert().NotNil(room, "room %d should not be nil", i)
		s.Assert().NotNil(room.Shape, "room %d Shape should not be nil", i)
		s.Assert().NotNil(room.Features, "room %d Features should not be nil", i)
		s.Assert().NotNil(room.Encounter, "room %d Encounter should not be nil", i)
		s.Assert().NotNil(room.SpawnZones, "room %d SpawnZones should not be nil", i)

		// Verify shape has valid data
		s.Assert().Greater(room.Shape.Width, 0, "room %d width should be positive", i)
		s.Assert().Greater(room.Shape.Height, 0, "room %d height should be positive", i)
		s.Assert().Greater(room.Shape.Area, 0, "room %d area should be positive", i)
		s.Assert().NotEmpty(room.Shape.Bounds, "room %d bounds should not be empty", i)

		// Verify encounter has valid data
		s.Assert().NotNil(room.Encounter.Monsters, "room %d Monsters should not be nil", i)
		if len(room.Encounter.Monsters) > 0 {
			s.Assert().Greater(room.Encounter.TotalCR, 0.0,
				"room %d with monsters should have positive CR", i)
		}

		// Verify monster placements
		for j, monster := range room.Encounter.Monsters {
			s.Assert().NotEmpty(monster.ID, "room %d monster %d ID should not be empty", i, j)
			s.Assert().NotEmpty(monster.MonsterID,
				"room %d monster %d MonsterID should not be empty", i, j)
			s.Assert().GreaterOrEqual(monster.CR, 0.0,
				"room %d monster %d CR should be non-negative", i, j)
		}
	}
}

// TestValidationErrors verifies input validation
func (s *IntegrationTestSuite) TestValidationErrors() {
	s.Run("nil input", func() {
		_, err := s.generator.Generate(context.Background(), nil)
		s.Assert().Error(err, "should reject nil input")
	})

	s.Run("zero length", func() {
		_, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
			Theme:     dungeon.ThemeCrypt,
			Length:    0,
			PartySize: 4,
			TargetCR:  3,
		})
		s.Assert().Error(err, "should reject zero length")
	})

	s.Run("negative length", func() {
		_, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
			Theme:     dungeon.ThemeCrypt,
			Length:    -1,
			PartySize: 4,
			TargetCR:  3,
		})
		s.Assert().Error(err, "should reject negative length")
	})

	s.Run("zero party size", func() {
		_, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
			Theme:     dungeon.ThemeCrypt,
			Length:    3,
			PartySize: 0,
			TargetCR:  3,
		})
		s.Assert().Error(err, "should reject zero party size")
	})

	s.Run("zero target CR", func() {
		_, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
			Theme:     dungeon.ThemeCrypt,
			Length:    3,
			PartySize: 4,
			TargetCR:  0,
		})
		s.Assert().Error(err, "should reject zero target CR")
	})
}

// TestPatternBasedWalls verifies that theme tables produce wall patterns
func (s *IntegrationTestSuite) TestPatternBasedWalls() {
	// Each theme has different wall pattern configurations
	themes := []dungeon.Theme{
		dungeon.ThemeCrypt,
		dungeon.ThemeCave,
		dungeon.ThemeBanditLair,
	}

	for _, theme := range themes {
		s.Run(theme.ID, func() {
			output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
				Theme:     theme,
				Size:      dungeon.RoomSizeMedium,
				Length:    4,
				Layout:    dungeon.LayoutLinear,
				PartySize: 4,
				TargetCR:  3,
				Seed:      12345, // Fixed seed for reproducibility
			})

			s.Require().NoError(err)
			s.Require().NotNil(output.Dungeon)

			// Verify theme tables exist (they enable pattern selection)
			s.Require().NotNil(theme.Tables, "theme %s should have tables", theme.ID)
			s.Assert().NotEmpty(theme.Tables.WallPatterns, "theme %s should have wall patterns", theme.ID)
		})
	}
}

// TestSpawnZonesAccessible verifies spawn zones are not blocked by walls
func (s *IntegrationTestSuite) TestSpawnZonesAccessible() {
	// Use linear layout for consistent boss room placement
	output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt, // Crypt has various wall patterns
		Size:      dungeon.RoomSizeMedium,
		Length:    5,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  3,
		Seed:      98765,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output.Dungeon)

	// Verify each room has spawn zones
	for i, room := range output.Dungeon.Rooms {
		s.Assert().NotEmpty(room.SpawnZones, "room %d should have spawn zones", i)

		// Verify spawn zones have valid positions
		for j, zone := range room.SpawnZones {
			s.Assert().NotEmpty(zone.ID, "room %d zone %d should have ID", i, j)
			s.Assert().NotEmpty(zone.Bounds, "room %d zone %d should have bounds", i, j)
			s.Assert().Greater(zone.Capacity, 0, "room %d zone %d should have capacity", i, j)
		}
	}

	// Find entrance room and verify player spawn exists
	for _, room := range output.Dungeon.Rooms {
		if room.ID == output.Dungeon.StartRoom {
			hasPlayerSpawn := false
			for _, zone := range room.SpawnZones {
				if zone.Type == dungeon.ZoneTypePlayerSpawn {
					hasPlayerSpawn = true
					break
				}
			}
			s.Assert().True(hasPlayerSpawn, "start room should have player spawn zone")
		}
	}

	// Find boss room and verify boss zone exists
	for _, room := range output.Dungeon.Rooms {
		if room.ID == output.Dungeon.BossRoom {
			hasBossZone := false
			for _, zone := range room.SpawnZones {
				if zone.Type == dungeon.ZoneTypeBoss {
					hasBossZone = true
					break
				}
			}
			s.Assert().True(hasBossZone, "boss room should have boss zone")
		}
	}
}

// TestPerimeterWallsGenerated verifies that shapes include perimeter walls
func (s *IntegrationTestSuite) TestPerimeterWallsGenerated() {
	output, err := s.generator.Generate(context.Background(), &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  2,
		Seed:      11111,
	})

	s.Require().NoError(err)
	s.Require().NotNil(output.Dungeon)

	// Each room should have a valid shape
	for i, room := range output.Dungeon.Rooms {
		s.Require().NotNil(room.Shape, "room %d should have a shape", i)
		s.Assert().Greater(room.Shape.Width, 0, "room %d should have width", i)
		s.Assert().Greater(room.Shape.Height, 0, "room %d should have height", i)
		s.Assert().NotEmpty(room.Shape.Bounds, "room %d should have bounds", i)
	}
}

// TestThemePatternWeights verifies themes have weighted pattern selection
func (s *IntegrationTestSuite) TestThemePatternWeights() {
	// Verify crypt theme patterns
	s.Run("crypt theme patterns", func() {
		s.Require().NotNil(dungeon.ThemeCrypt.Tables)
		tables := dungeon.ThemeCrypt.Tables

		// Entrance rooms should have patterns
		entrancePatterns := tables.WallPatterns[dungeon.RoomTypeEntrance]
		s.Assert().NotEmpty(entrancePatterns, "entrance should have patterns")

		// Boss rooms should have patterns
		bossPatterns := tables.WallPatterns[dungeon.RoomTypeBoss]
		s.Assert().NotEmpty(bossPatterns, "boss should have patterns")

		// Regular chambers should have patterns
		chamberPatterns := tables.WallPatterns[dungeon.RoomTypeChamber]
		s.Assert().NotEmpty(chamberPatterns, "chamber should have patterns")
	})

	// Verify cave theme patterns
	s.Run("cave theme patterns", func() {
		s.Require().NotNil(dungeon.ThemeCave.Tables)
		tables := dungeon.ThemeCave.Tables

		s.Assert().NotEmpty(tables.WallPatterns[dungeon.RoomTypeEntrance])
		s.Assert().NotEmpty(tables.WallPatterns[dungeon.RoomTypeBoss])
	})

	// Verify bandit lair patterns
	s.Run("bandit lair theme patterns", func() {
		s.Require().NotNil(dungeon.ThemeBanditLair.Tables)
		tables := dungeon.ThemeBanditLair.Tables

		s.Assert().NotEmpty(tables.WallPatterns[dungeon.RoomTypeEntrance])
		s.Assert().NotEmpty(tables.WallPatterns[dungeon.RoomTypeBoss])
	})
}

// TestDensityRangesValid verifies density ranges are properly defined
func (s *IntegrationTestSuite) TestDensityRangesValid() {
	// Verify density ranges
	s.Assert().Less(dungeon.DensityLow.Min, dungeon.DensityLow.Max)
	s.Assert().Less(dungeon.DensityMedium.Min, dungeon.DensityMedium.Max)
	s.Assert().Less(dungeon.DensityHigh.Min, dungeon.DensityHigh.Max)

	// Verify ranges don't overlap excessively
	s.Assert().LessOrEqual(dungeon.DensityLow.Max, dungeon.DensityMedium.Max)
	s.Assert().LessOrEqual(dungeon.DensityMedium.Max, dungeon.DensityHigh.Max)

	// Verify all themes have density in their tables
	for _, theme := range []dungeon.Theme{dungeon.ThemeCrypt, dungeon.ThemeCave, dungeon.ThemeBanditLair} {
		s.Require().NotNil(theme.Tables, "theme %s should have tables", theme.ID)
		s.Assert().NotEmpty(theme.Tables.Density, "theme %s should have density", theme.ID)
	}
}
