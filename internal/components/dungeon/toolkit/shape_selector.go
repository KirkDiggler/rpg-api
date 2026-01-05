package toolkit

import (
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// Shape name constants matching toolkit shape names
const (
	shapeNameRectangle = "rectangle"
	shapeNameSquare    = "square"
	shapeNameLShape    = "l_shape"
	shapeNameOval      = "oval"
)

// ShapeSelector maps room types to appropriate shapes from the toolkit
type ShapeSelector struct {
	shapes map[string]*environments.RoomShape
}

// NewShapeSelector creates a selector with the default toolkit shapes
func NewShapeSelector() *ShapeSelector {
	return &ShapeSelector{
		shapes: environments.GetDefaultShapes(),
	}
}

// SelectShape returns the appropriate shape for a room type and style
func (s *ShapeSelector) SelectShape(roomType dungeon.RoomType, style dungeon.ShapeStyle) *environments.RoomShape {
	shapeName := s.getShapeNameForRoom(roomType, style)
	if shape, ok := s.shapes[shapeName]; ok {
		return shape
	}
	// Fallback to rectangle if shape not found
	return s.shapes[shapeNameRectangle]
}

// getShapeNameForRoom determines which shape to use based on room type and style
func (s *ShapeSelector) getShapeNameForRoom(roomType dungeon.RoomType, style dungeon.ShapeStyle) string {
	// Room type takes precedence for specific shapes
	switch roomType {
	case dungeon.RoomTypeEntrance:
		// Entrances are narrow corridors - use rectangle with corridor dimensions
		return shapeNameRectangle

	case dungeon.RoomTypeCorridor:
		// Corridors are long and narrow
		return shapeNameRectangle

	case dungeon.RoomTypeBoss:
		// Boss rooms are large and square for tactical combat
		return shapeNameSquare

	case dungeon.RoomTypeTreasure:
		// Treasure rooms are small and square
		return shapeNameSquare

	case dungeon.RoomTypeTrap:
		// Trap rooms can be L-shaped to hide dangers
		return shapeNameLShape

	case dungeon.RoomTypeChamber:
		// Chambers vary based on style
		return s.getShapeForStyle(style)

	default:
		return s.getShapeForStyle(style)
	}
}

// getShapeForStyle returns a shape appropriate for the style when room type doesn't dictate
func (s *ShapeSelector) getShapeForStyle(style dungeon.ShapeStyle) string {
	switch style {
	case dungeon.ShapeStyleStructured:
		return shapeNameRectangle
	case dungeon.ShapeStyleOrganic:
		return shapeNameOval
	case dungeon.ShapeStyleMixed:
		// For mixed, prefer rectangle as a safe default
		return shapeNameRectangle
	default:
		return shapeNameRectangle
	}
}

// GetDimensions returns appropriate dimensions based on room type, size, and GridHints
func (s *ShapeSelector) GetDimensions(roomType dungeon.RoomType, size dungeon.RoomSize, shape *environments.RoomShape) (width, height int) {
	// Start with base dimensions from size category
	baseWidth, baseHeight := getBaseDimensions(size)

	// Apply room type modifiers
	switch roomType {
	case dungeon.RoomTypeEntrance:
		// Entrance rooms are the combat starting room - need space for party and monsters
		// with separation between them (players near south door, monsters near north)
		width = baseWidth
		if width < 10 {
			width = 10
		}
		height = baseHeight
		if height < 12 {
			height = 12
		}

	case dungeon.RoomTypeCorridor:
		// Corridors are long and narrow
		width = baseWidth / 3
		if width < 4 {
			width = 4
		}
		height = baseHeight

	case dungeon.RoomTypeBoss:
		// Boss rooms are large
		width = baseWidth + 5
		height = baseHeight + 5

	default:
		width = baseWidth
		height = baseHeight
	}

	// Respect GridHints if available
	if shape != nil && shape.GridHints.MinSize.Width > 0 {
		minWidth := int(shape.GridHints.MinSize.Width)
		minHeight := int(shape.GridHints.MinSize.Height)
		if width < minWidth {
			width = minWidth
		}
		if height < minHeight {
			height = minHeight
		}
	}

	if shape != nil && shape.GridHints.MaxSize.Width > 0 {
		maxWidth := int(shape.GridHints.MaxSize.Width)
		maxHeight := int(shape.GridHints.MaxSize.Height)
		if width > maxWidth {
			width = maxWidth
		}
		if height > maxHeight {
			height = maxHeight
		}
	}

	return width, height
}

// getBaseDimensions returns base dimensions for a room size category
func getBaseDimensions(size dungeon.RoomSize) (width, height int) {
	switch size {
	case dungeon.RoomSizeSmall:
		return 12, 12
	case dungeon.RoomSizeMedium:
		return 18, 16
	case dungeon.RoomSizeLarge:
		return 28, 24
	default:
		return 18, 16
	}
}
