package toolkit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type ToolkitGeneratorsTestSuite struct {
	suite.Suite
}

func TestToolkitGeneratorsTestSuite(t *testing.T) {
	suite.Run(t, new(ToolkitGeneratorsTestSuite))
}

// TestLayoutGenerator tests the toolkit layout generator
func (s *ToolkitGeneratorsTestSuite) TestLayoutGenerator() {
	s.Run("generates linear layout", func() {
		gen := NewToolkitLayoutGenerator(&ToolkitLayoutConfig{})

		output, err := gen.Generate(context.Background(), &dungeon.LayoutInput{
			Length:     5,
			LayoutType: dungeon.LayoutLinear,
			Seed:       12345,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output)
		s.Assert().Len(output.RoomSlots, 5)
		s.Assert().Equal(0, output.StartRoom)
		s.Assert().Equal(4, output.BossRoom)
		s.Assert().Equal(dungeon.RoomTypeEntrance, output.RoomSlots[0].RoomType)
		s.Assert().Equal(dungeon.RoomTypeBoss, output.RoomSlots[4].RoomType)
	})

	s.Run("generates branching layout", func() {
		gen := NewToolkitLayoutGenerator(&ToolkitLayoutConfig{})

		output, err := gen.Generate(context.Background(), &dungeon.LayoutInput{
			Length:     8,
			LayoutType: dungeon.LayoutBranching,
			Seed:       54321,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output)
		s.Assert().Len(output.RoomSlots, 8)
		s.Assert().NotEmpty(output.Connections)
	})

	s.Run("validates input", func() {
		gen := NewToolkitLayoutGenerator(&ToolkitLayoutConfig{})

		_, err := gen.Generate(context.Background(), nil)
		s.Assert().Error(err)

		_, err = gen.Generate(context.Background(), &dungeon.LayoutInput{
			Length: 0,
		})
		s.Assert().Error(err)
	})
}

// TestShapeGenerator tests the toolkit shape generator
func (s *ToolkitGeneratorsTestSuite) TestShapeGenerator() {
	s.Run("generates small structured chamber shape", func() {
		gen := NewToolkitShapeGenerator()

		output, err := gen.Generate(context.Background(), &dungeon.ShapeInput{
			Size:     dungeon.RoomSizeSmall,
			Style:    dungeon.ShapeStyleStructured,
			RoomType: dungeon.RoomTypeChamber,
			Seed:     12345,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output)
		s.Require().NotNil(output.Shape)
		// Small chamber: base 12x12
		s.Assert().Equal(12, output.Shape.Width)
		s.Assert().Equal(12, output.Shape.Height)
		s.Assert().NotEmpty(output.Shape.Bounds)
		s.Assert().Equal(dungeon.GridTypeHex, output.Shape.GridType)

		// Verify perimeter walls are generated (4 walls for rectangular room)
		s.Assert().Len(output.PerimeterWalls, 4)
		for _, wall := range output.PerimeterWalls {
			s.Assert().Equal(dungeon.WallTypeIndestructible, wall.Type)
			s.Assert().True(wall.BlocksMovement)
			s.Assert().True(wall.BlocksLineOfSight)
		}
	})

	s.Run("generates medium organic chamber shape", func() {
		gen := NewToolkitShapeGenerator()

		output, err := gen.Generate(context.Background(), &dungeon.ShapeInput{
			Size:     dungeon.RoomSizeMedium,
			Style:    dungeon.ShapeStyleOrganic,
			RoomType: dungeon.RoomTypeChamber,
			Seed:     54321,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output)
		// Medium chamber: base 18x16
		s.Assert().Equal(18, output.Shape.Width)
		s.Assert().Equal(16, output.Shape.Height)
		s.Assert().NotEmpty(output.Shape.Bounds)

		// Organic shapes also have perimeter walls
		s.Assert().NotEmpty(output.PerimeterWalls)
	})

	s.Run("generates full-sized entrance room for combat", func() {
		gen := NewToolkitShapeGenerator()

		output, err := gen.Generate(context.Background(), &dungeon.ShapeInput{
			Size:     dungeon.RoomSizeMedium,
			Style:    dungeon.ShapeStyleStructured,
			RoomType: dungeon.RoomTypeEntrance,
			Seed:     12345,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output)
		// Entrance rooms are combat starting rooms - full base size with minimums 10x12
		// Medium base 18x16 -> 18x16 (meets minimums)
		s.Assert().Equal(18, output.Shape.Width)
		s.Assert().Equal(16, output.Shape.Height)
	})

	s.Run("generates larger boss room", func() {
		gen := NewToolkitShapeGenerator()

		output, err := gen.Generate(context.Background(), &dungeon.ShapeInput{
			Size:     dungeon.RoomSizeMedium,
			Style:    dungeon.ShapeStyleStructured,
			RoomType: dungeon.RoomTypeBoss,
			Seed:     12345,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output)
		// Boss rooms are larger: base + 5
		// Medium base 18x16 -> 23x21
		s.Assert().Equal(23, output.Shape.Width)
		s.Assert().Equal(21, output.Shape.Height)
	})

	s.Run("includes connection points for door placement", func() {
		gen := NewToolkitShapeGenerator()

		output, err := gen.Generate(context.Background(), &dungeon.ShapeInput{
			Size:     dungeon.RoomSizeMedium,
			Style:    dungeon.ShapeStyleStructured,
			RoomType: dungeon.RoomTypeChamber,
			Seed:     12345,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output.Shape)

		// Should have 4 connection points for rectangle (N, S, E, W)
		s.Assert().Len(output.Shape.ConnectionPoints, 4)

		// Check that each direction is represented
		directions := make(map[string]bool)
		for _, cp := range output.Shape.ConnectionPoints {
			directions[cp.Direction] = true
			s.Assert().NotEmpty(cp.Name)
			s.Assert().Equal("door", cp.Type)
		}
		s.Assert().True(directions["north"], "missing north connection")
		s.Assert().True(directions["south"], "missing south connection")
		s.Assert().True(directions["east"], "missing east connection")
		s.Assert().True(directions["west"], "missing west connection")
	})

	s.Run("validates input", func() {
		gen := NewToolkitShapeGenerator()

		_, err := gen.Generate(context.Background(), nil)
		s.Assert().Error(err)
	})
}

// TestFeatureGenerator tests the toolkit feature generator
func (s *ToolkitGeneratorsTestSuite) TestFeatureGenerator() {
	s.Run("generates features for entrance room", func() {
		gen := NewToolkitFeatureGenerator()

		shape := &dungeon.Shape{
			Width:  15,
			Height: 15,
			Area:   225,
		}

		output, err := gen.Generate(context.Background(), &dungeon.FeatureInput{
			Shape: shape,
			Rules: dungeon.FeatureRules{
				ObstacleChance: 1.0, // Always generate obstacles
				ObstacleTypes:  []dungeon.ObstacleType{dungeon.ObstacleTypePillar},
			},
			RoomType: "entrance",
			Seed:     12345,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output)
		s.Require().NotNil(output.Features)
		s.Assert().NotEmpty(output.Features.Obstacles)
		s.Assert().NotEmpty(output.Zones)

		// Check for player spawn zone
		hasPlayerSpawn := false
		for _, zone := range output.Zones {
			if zone.Type == dungeon.ZoneTypePlayerSpawn {
				hasPlayerSpawn = true
				break
			}
		}
		s.Assert().True(hasPlayerSpawn)
	})

	s.Run("generates features for boss room", func() {
		gen := NewToolkitFeatureGenerator()

		shape := &dungeon.Shape{
			Width:  20,
			Height: 20,
			Area:   400,
		}

		output, err := gen.Generate(context.Background(), &dungeon.FeatureInput{
			Shape:    shape,
			Rules:    dungeon.FeatureRules{},
			RoomType: "boss",
			Seed:     54321,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output)
		s.Assert().NotEmpty(output.Zones)

		// Check for boss zone
		hasBossZone := false
		for _, zone := range output.Zones {
			if zone.Type == dungeon.ZoneTypeBoss {
				hasBossZone = true
				break
			}
		}
		s.Assert().True(hasBossZone)
	})

	s.Run("validates input", func() {
		gen := NewToolkitFeatureGenerator()

		_, err := gen.Generate(context.Background(), nil)
		s.Assert().Error(err)

		_, err = gen.Generate(context.Background(), &dungeon.FeatureInput{
			Shape: nil,
		})
		s.Assert().Error(err)
	})
}

// TestFactory tests the factory functions
func (s *ToolkitGeneratorsTestSuite) TestFactory() {
	s.Run("creates all generators", func() {
		generators := NewToolkitGenerators(&ToolkitConfig{})

		s.Assert().NotNil(generators.Layout)
		s.Assert().NotNil(generators.Shape)
		s.Assert().NotNil(generators.Feature)
		s.Assert().NotNil(generators.Encounter)
		s.Assert().NotNil(generators.Budget)
	})

	s.Run("creates complete generator", func() {
		generator := CreateGenerator(&ToolkitConfig{})

		s.Assert().NotNil(generator)

		// Test end-to-end generation
		output, err := generator.Generate(context.Background(), &dungeon.GenerateInput{
			Theme:     dungeon.ThemeCrypt,
			Size:      dungeon.RoomSizeSmall,
			Length:    3,
			Layout:    dungeon.LayoutLinear,
			PartySize: 4,
			TargetCR:  2,
			Seed:      12345,
		})

		s.Require().NoError(err)
		s.Require().NotNil(output)
		s.Assert().NotNil(output.Dungeon)
		s.Assert().Len(output.Dungeon.Rooms, 3)
	})
}
