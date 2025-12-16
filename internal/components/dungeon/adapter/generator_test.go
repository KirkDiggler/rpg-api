package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon/encounter"
)

type GeneratorTestSuite struct {
	suite.Suite
	generator *dungeon.Generator
}

func (s *GeneratorTestSuite) SetupTest() {
	s.generator = dungeon.NewGenerator(&dungeon.GeneratorConfig{
		LayoutGen:       NewStubLayoutGenerator(),
		ShapeGen:        NewStubShapeGenerator(),
		FeatureGen:      NewStubFeatureGenerator(),
		EncounterGen:    NewEncounterAdapter(encounter.NewGenerator()),
		BudgetAllocator: NewBudgetAllocator(),
	})
}

func TestGeneratorTestSuite(t *testing.T) {
	suite.Run(t, new(GeneratorTestSuite))
}

func (s *GeneratorTestSuite) TestGenerate_Success() {
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    4,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  3,
		Seed:      12345,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.NoError(err)
	s.NotNil(output)
	s.NotNil(output.Dungeon)
	s.Equal(int64(12345), output.Seed)

	// Verify dungeon structure
	dung := output.Dungeon
	s.NotEmpty(dung.ID)
	s.Equal(dungeon.ThemeCrypt, dung.Theme)
	s.Len(dung.Rooms, 4)
	s.Len(dung.Connections, 3) // n-1 connections for linear layout
	s.NotEmpty(dung.StartRoom)
	s.NotEmpty(dung.BossRoom)
}

func (s *GeneratorTestSuite) TestGenerate_VerifyRoomStructure() {
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCave,
		Size:      dungeon.RoomSizeSmall,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 3,
		TargetCR:  2,
		Seed:      54321,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.NoError(err)

	// Verify each room has all required components
	for i, room := range output.Dungeon.Rooms {
		s.NotEmpty(room.ID, "Room %d missing ID", i)
		s.NotNil(room.Shape, "Room %d missing shape", i)
		s.NotNil(room.Features, "Room %d missing features", i)
		s.NotNil(room.Encounter, "Room %d missing encounter", i)

		// Verify shape has dimensions
		s.Greater(room.Shape.Width, 0, "Room %d shape has no width", i)
		s.Greater(room.Shape.Height, 0, "Room %d shape has no height", i)
		s.Greater(room.Shape.Area, 0, "Room %d shape has no area", i)

		// Verify spawn zones exist
		s.NotEmpty(room.SpawnZones, "Room %d missing spawn zones", i)
	}
}

func (s *GeneratorTestSuite) TestGenerate_VerifyConnections() {
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    4,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  3,
		Seed:      11111,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.NoError(err)

	// Build a map of room IDs
	roomIDs := make(map[string]bool)
	for _, room := range output.Dungeon.Rooms {
		roomIDs[room.ID] = true
	}

	// Verify all connections reference valid rooms
	for i, conn := range output.Dungeon.Connections {
		s.True(roomIDs[conn.FromRoom], "Connection %d FromRoom not found", i)
		s.True(roomIDs[conn.ToRoom], "Connection %d ToRoom not found", i)
		s.NotEqual(conn.FromRoom, conn.ToRoom, "Connection %d connects room to itself", i)
	}
}

func (s *GeneratorTestSuite) TestGenerate_VerifyStartAndBossRooms() {
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeBanditLair,
		Size:      dungeon.RoomSizeLarge,
		Length:    5,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  4,
		Seed:      22222,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.NoError(err)

	dung := output.Dungeon

	// Verify start and boss rooms exist
	s.NotEmpty(dung.StartRoom)
	s.NotEmpty(dung.BossRoom)

	// Verify they reference actual rooms
	var startFound, bossFound bool
	for _, room := range dung.Rooms {
		if room.ID == dung.StartRoom {
			startFound = true
		}
		if room.ID == dung.BossRoom {
			bossFound = true
		}
	}
	s.True(startFound, "Start room not found in rooms list")
	s.True(bossFound, "Boss room not found in rooms list")

	// Verify they're different
	s.NotEqual(dung.StartRoom, dung.BossRoom, "Start and boss room are the same")
}

func (s *GeneratorTestSuite) TestGenerate_VerifyCRBudgetAllocation() {
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    4,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  5,
		Seed:      33333,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.NoError(err)

	// Calculate total CR across all encounters
	totalCR := 0.0
	bossRoomCR := 0.0

	for _, room := range output.Dungeon.Rooms {
		totalCR += room.Encounter.TotalCR
		if room.ID == output.Dungeon.BossRoom {
			bossRoomCR = room.Encounter.TotalCR
		}
	}

	// Boss room should have substantial CR
	s.Greater(bossRoomCR, 0.0, "Boss room has no CR budget")

	// Total CR should be reasonable for the target
	expectedTotalCR := float64(input.PartySize * input.TargetCR)
	s.InDelta(expectedTotalCR, totalCR, expectedTotalCR*0.5, "Total CR significantly off from expected")
}

func (s *GeneratorTestSuite) TestGenerate_VerifyThemeRespected() {
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  3,
		Seed:      44444,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.NoError(err)

	// Verify theme is stored
	s.Equal(dungeon.ThemeCrypt, output.Dungeon.Theme)

	// Verify encounters use theme's monster pool
	cryptMonsterIDs := map[string]bool{
		"skeleton":        true,
		"zombie":          true,
		"skeleton-archer": true,
	}
	cryptBossIDs := map[string]bool{
		"skeleton-captain": true,
	}

	for _, room := range output.Dungeon.Rooms {
		for _, monster := range room.Encounter.Monsters {
			if room.ID == output.Dungeon.BossRoom {
				s.True(cryptBossIDs[monster.MonsterID] || cryptMonsterIDs[monster.MonsterID],
					"Boss room has invalid monster: %s", monster.MonsterID)
			} else {
				s.True(cryptMonsterIDs[monster.MonsterID],
					"Regular room has invalid monster: %s", monster.MonsterID)
			}
		}
	}
}

func (s *GeneratorTestSuite) TestGenerate_SeedReproducibility() {
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCave,
		Size:      dungeon.RoomSizeMedium,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  3,
		Seed:      99999,
	}

	// Generate twice with same seed
	output1, err1 := s.generator.Generate(context.Background(), input)
	output2, err2 := s.generator.Generate(context.Background(), input)

	s.NoError(err1)
	s.NoError(err2)

	// Verify same number of rooms
	s.Len(output2.Dungeon.Rooms, len(output1.Dungeon.Rooms))

	// Verify same shape dimensions
	for i := range output1.Dungeon.Rooms {
		s.Equal(output1.Dungeon.Rooms[i].Shape.Width,
			output2.Dungeon.Rooms[i].Shape.Width,
			"Room %d width differs", i)
		s.Equal(output1.Dungeon.Rooms[i].Shape.Height,
			output2.Dungeon.Rooms[i].Shape.Height,
			"Room %d height differs", i)
	}

	// Verify same encounter composition
	for i := range output1.Dungeon.Rooms {
		s.Equal(len(output1.Dungeon.Rooms[i].Encounter.Monsters),
			len(output2.Dungeon.Rooms[i].Encounter.Monsters),
			"Room %d monster count differs", i)

		s.InDelta(output1.Dungeon.Rooms[i].Encounter.TotalCR,
			output2.Dungeon.Rooms[i].Encounter.TotalCR,
			0.01,
			"Room %d total CR differs", i)
	}
}

func (s *GeneratorTestSuite) TestGenerate_AutoGeneratesSeed() {
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  3,
		Seed:      0, // Auto-generate
	}

	output1, err := s.generator.Generate(context.Background(), input)
	s.NoError(err)
	s.NotZero(output1.Seed, "Seed should be auto-generated")

	output2, err := s.generator.Generate(context.Background(), input)
	s.NoError(err)
	s.NotZero(output2.Seed, "Seed should be auto-generated")

	// Auto-generated seeds should differ
	s.NotEqual(output1.Seed, output2.Seed)
}

func (s *GeneratorTestSuite) TestGenerate_NilInput() {
	output, err := s.generator.Generate(context.Background(), nil)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "input is required")
}

func (s *GeneratorTestSuite) TestGenerate_InvalidLength() {
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    0,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  3,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "length must be greater than 0")
}

func (s *GeneratorTestSuite) TestGenerate_InvalidPartySize() {
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 0,
		TargetCR:  3,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "party size must be greater than 0")
}

func (s *GeneratorTestSuite) TestGenerate_InvalidTargetCR() {
	input := &dungeon.GenerateInput{
		Theme:     dungeon.ThemeCrypt,
		Size:      dungeon.RoomSizeMedium,
		Length:    3,
		Layout:    dungeon.LayoutLinear,
		PartySize: 4,
		TargetCR:  0,
	}

	output, err := s.generator.Generate(context.Background(), input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "target CR must be greater than 0")
}

func (s *GeneratorTestSuite) TestGenerate_DifferentSizes() {
	sizes := []dungeon.RoomSize{dungeon.RoomSizeSmall, dungeon.RoomSizeMedium, dungeon.RoomSizeLarge}

	for _, size := range sizes {
		input := &dungeon.GenerateInput{
			Theme:     dungeon.ThemeCrypt,
			Size:      size,
			Length:    2,
			Layout:    dungeon.LayoutLinear,
			PartySize: 4,
			TargetCR:  3,
			Seed:      12345,
		}

		output, err := s.generator.Generate(context.Background(), input)

		s.NoError(err, "Failed for size %v", size)
		s.NotNil(output)
		s.Len(output.Dungeon.Rooms, 2)

		// Verify rooms have appropriate dimensions based on size
		for _, room := range output.Dungeon.Rooms {
			switch size {
			case dungeon.RoomSizeSmall:
				s.LessOrEqual(room.Shape.Width, 15, "Small room too wide")
			case dungeon.RoomSizeMedium:
				s.GreaterOrEqual(room.Shape.Width, 15, "Medium room too small")
				s.LessOrEqual(room.Shape.Width, 25, "Medium room too large")
			case dungeon.RoomSizeLarge:
				s.GreaterOrEqual(room.Shape.Width, 25, "Large room too small")
			}
		}
	}
}

func (s *GeneratorTestSuite) TestGenerate_DifferentThemes() {
	themes := []dungeon.Theme{dungeon.ThemeCrypt, dungeon.ThemeCave, dungeon.ThemeBanditLair}

	for _, theme := range themes {
		input := &dungeon.GenerateInput{
			Theme:     theme,
			Size:      dungeon.RoomSizeMedium,
			Length:    3,
			Layout:    dungeon.LayoutLinear,
			PartySize: 4,
			TargetCR:  3,
			Seed:      12345,
		}

		output, err := s.generator.Generate(context.Background(), input)

		s.NoError(err, "Failed for theme %s", theme.Name)
		s.NotNil(output)
		s.Equal(theme, output.Dungeon.Theme)

		// Verify encounters have monsters
		for _, room := range output.Dungeon.Rooms {
			s.NotEmpty(room.Encounter.Monsters, "Room has no monsters for theme %s", theme.Name)
		}
	}
}
