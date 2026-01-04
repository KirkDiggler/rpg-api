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
	bounds := make([]dungeon.Position, len(toolkitShape.Boundary))
	for i, point := range toolkitShape.Boundary {
		// Scale normalized coords to grid dimensions
		scaledX := int(point.X * float64(width-1))
		scaledY := int(point.Y * float64(height-1))

		// Convert to cube coordinates
		bounds[i] = offsetToCube(scaledX, scaledY)
	}

	// Calculate area based on shape type
	area := g.calculateArea(toolkitShape, width, height)

	return &dungeon.Shape{
		Bounds:   bounds,
		GridType: dungeon.GridTypeHex,
		Width:    width,
		Height:   height,
		Area:     area,
	}
}

// createRectangleShape creates a simple rectangular shape as fallback
func (g *ToolkitShapeGenerator) createRectangleShape(width, height int) *dungeon.Shape {
	bounds := []dungeon.Position{
		offsetToCube(0, 0),
		offsetToCube(width-1, 0),
		offsetToCube(width-1, height-1),
		offsetToCube(0, height-1),
	}

	return &dungeon.Shape{
		Bounds:   bounds,
		GridType: dungeon.GridTypeHex,
		Width:    width,
		Height:   height,
		Area:     width * height,
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
