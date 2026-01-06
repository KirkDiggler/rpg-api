package toolkit

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type ShapeSelectorTestSuite struct {
	suite.Suite
	selector *ShapeSelector
}

func (s *ShapeSelectorTestSuite) SetupTest() {
	s.selector = NewShapeSelector()
}

func TestShapeSelectorSuite(t *testing.T) {
	suite.Run(t, new(ShapeSelectorTestSuite))
}

func (s *ShapeSelectorTestSuite) TestSelectShape_EntranceRoom() {
	shape := s.selector.SelectShape(dungeon.RoomTypeEntrance, dungeon.ShapeStyleStructured)

	s.Require().NotNil(shape)
	s.Equal("rectangle", shape.Name, "entrance rooms should use rectangle shape")
}

func (s *ShapeSelectorTestSuite) TestSelectShape_BossRoom() {
	shape := s.selector.SelectShape(dungeon.RoomTypeBoss, dungeon.ShapeStyleStructured)

	s.Require().NotNil(shape)
	s.Equal("square", shape.Name, "boss rooms should use square shape for tactical combat")
}

func (s *ShapeSelectorTestSuite) TestSelectShape_TrapRoom() {
	shape := s.selector.SelectShape(dungeon.RoomTypeTrap, dungeon.ShapeStyleStructured)

	s.Require().NotNil(shape)
	s.Equal("l_shape", shape.Name, "trap rooms should use l_shape to hide dangers")
}

func (s *ShapeSelectorTestSuite) TestSelectShape_ChamberWithOrganicStyle() {
	shape := s.selector.SelectShape(dungeon.RoomTypeChamber, dungeon.ShapeStyleOrganic)

	s.Require().NotNil(shape)
	s.Equal("oval", shape.Name, "organic style chambers should use oval shape")
}

func (s *ShapeSelectorTestSuite) TestSelectShape_DefaultFallback() {
	// Unknown room type should fall back based on style
	shape := s.selector.SelectShape(dungeon.RoomType("unknown"), dungeon.ShapeStyleStructured)

	s.Require().NotNil(shape)
	s.Equal("rectangle", shape.Name, "unknown room types with structured style should use rectangle")
}

func (s *ShapeSelectorTestSuite) TestGetDimensions_EntranceIsCombatSized() {
	shape := s.selector.SelectShape(dungeon.RoomTypeEntrance, dungeon.ShapeStyleStructured)
	entranceWidth, entranceHeight := s.selector.GetDimensions(dungeon.RoomTypeEntrance, dungeon.RoomSizeMedium, shape)

	chamberShape := s.selector.SelectShape(dungeon.RoomTypeChamber, dungeon.ShapeStyleStructured)
	chamberWidth, chamberHeight := s.selector.GetDimensions(dungeon.RoomTypeChamber, dungeon.RoomSizeMedium, chamberShape)

	// Entrance rooms are combat starting rooms - same size as chambers with minimums 10x12
	s.Equal(entranceWidth, chamberWidth, "entrance rooms should be same width as chambers for combat")
	s.Equal(entranceHeight, chamberHeight, "entrance rooms should be same height as chambers for combat")
	s.GreaterOrEqual(entranceWidth, 10, "entrance rooms should have minimum width for combat")
	s.GreaterOrEqual(entranceHeight, 12, "entrance rooms should have minimum height for combat")
}

func (s *ShapeSelectorTestSuite) TestGetDimensions_BossIsLarger() {
	shape := s.selector.SelectShape(dungeon.RoomTypeBoss, dungeon.ShapeStyleStructured)
	bossWidth, bossHeight := s.selector.GetDimensions(dungeon.RoomTypeBoss, dungeon.RoomSizeMedium, shape)

	chamberShape := s.selector.SelectShape(dungeon.RoomTypeChamber, dungeon.ShapeStyleStructured)
	chamberWidth, chamberHeight := s.selector.GetDimensions(dungeon.RoomTypeChamber, dungeon.RoomSizeMedium, chamberShape)

	s.Greater(bossWidth, chamberWidth, "boss rooms should be wider than chambers")
	s.Greater(bossHeight, chamberHeight, "boss rooms should be taller than chambers")
}

func (s *ShapeSelectorTestSuite) TestGetDimensions_RespectsGridHints() {
	shape := s.selector.SelectShape(dungeon.RoomTypeChamber, dungeon.ShapeStyleStructured)

	// Small size with GridHints should respect minimum
	width, height := s.selector.GetDimensions(dungeon.RoomTypeChamber, dungeon.RoomSizeSmall, shape)

	// GridHints.MinSize is 4x4 for rectangle, but our base dimensions are larger
	s.GreaterOrEqual(width, 4, "width should respect GridHints minimum")
	s.GreaterOrEqual(height, 4, "height should respect GridHints minimum")
}

func (s *ShapeSelectorTestSuite) TestGetDimensions_SizeCategories() {
	shape := s.selector.SelectShape(dungeon.RoomTypeChamber, dungeon.ShapeStyleStructured)

	smallW, smallH := s.selector.GetDimensions(dungeon.RoomTypeChamber, dungeon.RoomSizeSmall, shape)
	medW, medH := s.selector.GetDimensions(dungeon.RoomTypeChamber, dungeon.RoomSizeMedium, shape)
	largeW, largeH := s.selector.GetDimensions(dungeon.RoomTypeChamber, dungeon.RoomSizeLarge, shape)

	s.Less(smallW, medW, "small rooms should be smaller than medium")
	s.Less(medW, largeW, "medium rooms should be smaller than large")
	s.Less(smallH, medH, "small room height should be less than medium")
	s.Less(medH, largeH, "medium room height should be less than large")
}
