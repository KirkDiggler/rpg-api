package toolkit

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/KirkDiggler/rpg-toolkit/tools/environments"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// ToolkitShapeGenerator implements dungeon.ShapeGenerator using toolkit shapes
type ToolkitShapeGenerator struct {
	random    *rand.Rand
	perimeter *PerimeterGenerator
	selector  *ShapeSelector
}

// NewToolkitShapeGenerator creates a new shape generator
func NewToolkitShapeGenerator() *ToolkitShapeGenerator {
	// #nosec G404 - Using math/rand for seeded procedural generation, not cryptographic purposes
	return &ToolkitShapeGenerator{
		random:    rand.New(rand.NewSource(0)), // Will be reseeded per generation
		perimeter: NewPerimeterGenerator(),
		selector:  NewShapeSelector(),
	}
}

// Generate creates a room shape based on the input parameters
func (g *ToolkitShapeGenerator) Generate(_ context.Context, input *dungeon.ShapeInput) (*dungeon.ShapeOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Reseed the random generator for deterministic results
	if input.Seed != 0 {
		g.random.Seed(input.Seed)
	}

	// Select appropriate shape from toolkit based on room type and style
	toolkitShape := g.selector.SelectShape(input.RoomType, input.Style)

	// Get dimensions appropriate for this room type and size
	width, height := g.selector.GetDimensions(input.RoomType, input.Size, toolkitShape)

	// Convert toolkit shape to dungeon shape with cube coordinates
	shape := g.convertToDungeonShape(toolkitShape, width, height)

	// Generate perimeter walls
	perimeterOutput := g.perimeter.Generate(&PerimeterInput{
		Shape:       shape,
		Connections: nil, // Connections are not known at shape generation time
	})

	return &dungeon.ShapeOutput{
		Shape:          shape,
		PerimeterWalls: perimeterOutput.Walls,
	}, nil
}

// convertToDungeonShape transforms a toolkit RoomShape to a dungeon.Shape with cube coordinates
func (g *ToolkitShapeGenerator) convertToDungeonShape(toolkitShape *environments.RoomShape, width, height int) *dungeon.Shape {
	if toolkitShape == nil {
		// Fallback to simple rectangle
		return g.createRectangleShape(width, height)
	}

	// Scale normalized boundary (0.0-1.0) to actual dimensions
	bounds := make([]dungeon.LocalPosition, len(toolkitShape.Boundary))
	for i, point := range toolkitShape.Boundary {
		// Scale normalized coords to grid dimensions
		scaledX := int(point.X * float64(width-1))
		scaledY := int(point.Y * float64(height-1))

		// Convert to cube coordinates
		bounds[i] = offsetToCube(scaledX, scaledY)
	}

	// Convert connection points from toolkit to dungeon coordinates
	connectionPoints := g.convertConnectionPoints(toolkitShape.Connections, width, height)

	// Calculate area based on shape type
	area := g.calculateArea(toolkitShape, width, height)

	return &dungeon.Shape{
		Bounds:           bounds,
		ConnectionPoints: connectionPoints,
		GridType:         dungeon.GridTypeHex,
		Width:            width,
		Height:           height,
		Area:             area,
	}
}

// convertConnectionPoints scales toolkit connection points to actual grid dimensions
func (g *ToolkitShapeGenerator) convertConnectionPoints(connections []environments.ConnectionPoint, width, height int) []dungeon.ConnectionPoint {
	result := make([]dungeon.ConnectionPoint, len(connections))
	for i, conn := range connections {
		// Scale normalized position to grid dimensions
		scaledX := int(conn.Position.X * float64(width-1))
		scaledY := int(conn.Position.Y * float64(height-1))

		result[i] = dungeon.ConnectionPoint{
			Name:      conn.Name,
			Position:  offsetToCube(scaledX, scaledY),
			Direction: conn.Direction,
			Type:      conn.Type,
		}
	}
	return result
}

// createRectangleShape creates a simple rectangular shape as fallback
func (g *ToolkitShapeGenerator) createRectangleShape(width, height int) *dungeon.Shape {
	bounds := []dungeon.LocalPosition{
		offsetToCube(0, 0),
		offsetToCube(width-1, 0),
		offsetToCube(width-1, height-1),
		offsetToCube(0, height-1),
	}

	// Default connection points at wall midpoints
	midX := (width - 1) / 2
	midY := (height - 1) / 2
	connectionPoints := []dungeon.ConnectionPoint{
		{Name: "south", Position: offsetToCube(midX, 0), Direction: "south", Type: "door"},
		{Name: "east", Position: offsetToCube(width-1, midY), Direction: "east", Type: "door"},
		{Name: "north", Position: offsetToCube(midX, height-1), Direction: "north", Type: "door"},
		{Name: "west", Position: offsetToCube(0, midY), Direction: "west", Type: "door"},
	}

	return &dungeon.Shape{
		Bounds:           bounds,
		ConnectionPoints: connectionPoints,
		GridType:         dungeon.GridTypeHex,
		Width:            width,
		Height:           height,
		Area:             width * height,
	}
}

// calculateArea estimates the area based on shape type
func (g *ToolkitShapeGenerator) calculateArea(shape *environments.RoomShape, width, height int) int {
	fullArea := width * height

	// Adjust for shape type
	switch shape.Type {
	case "basic":
		// Rectangle/square - full area
		return fullArea
	case "junction":
		// L-shape, T-shape - roughly 60-70% of bounding box
		return fullArea * 65 / 100
	case "hub":
		// Cross - roughly 50% of bounding box
		return fullArea * 50 / 100
	case "organic":
		// Oval, hexagon - roughly 75-80% of bounding box
		return fullArea * 78 / 100
	default:
		return fullArea
	}
}
