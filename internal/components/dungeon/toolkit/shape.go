package toolkit

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// ToolkitShapeGenerator implements dungeon.ShapeGenerator using simple geometric shapes
type ToolkitShapeGenerator struct {
	random    *rand.Rand
	perimeter *PerimeterGenerator
}

// NewToolkitShapeGenerator creates a new shape generator
func NewToolkitShapeGenerator() *ToolkitShapeGenerator {
	// #nosec G404 - Using math/rand for seeded procedural generation, not cryptographic purposes
	return &ToolkitShapeGenerator{
		random:    rand.New(rand.NewSource(0)), // Will be reseeded per generation
		perimeter: NewPerimeterGenerator(),
	}
}

// Generate creates a room shape based on the input parameters
func (g *ToolkitShapeGenerator) Generate(ctx context.Context, input *dungeon.ShapeInput) (*dungeon.ShapeOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Reseed the random generator for deterministic results
	if input.Seed != 0 {
		g.random.Seed(input.Seed)
	}

	// Determine room dimensions based on size
	width, height := calculateDimensions(input.Size, input.Style, g.random)

	// Generate shape based on style
	shape := generateShapeForStyle(width, height, input.Style, g.random)

	// Generate perimeter walls (without doors - those are added later based on connections)
	perimeterOutput := g.perimeter.Generate(&PerimeterInput{
		Shape:       shape,
		Connections: nil, // Connections are not known at shape generation time
	})

	return &dungeon.ShapeOutput{
		Shape:          shape,
		PerimeterWalls: perimeterOutput.Walls,
	}, nil
}

// calculateDimensions returns width and height based on room size
func calculateDimensions(size dungeon.RoomSize, style dungeon.ShapeStyle, rng *rand.Rand) (int, int) {
	var minW, maxW, minH, maxH int

	// Base dimensions on size category
	switch size {
	case dungeon.RoomSizeSmall:
		minW, maxW = 10, 15
		minH, maxH = 10, 15
	case dungeon.RoomSizeMedium:
		minW, maxW = 15, 25
		minH, maxH = 15, 20
	case dungeon.RoomSizeLarge:
		minW, maxW = 25, 40
		minH, maxH = 20, 30
	default:
		minW, maxW = 15, 25
		minH, maxH = 15, 20
	}

	// Add variation based on style
	if style == dungeon.ShapeStyleOrganic {
		// Organic shapes have more variation
		minW -= 2
		maxW += 3
		minH -= 2
		maxH += 3
	}

	// Generate random dimensions within bounds
	width := minW
	if maxW > minW {
		width += rng.Intn(maxW - minW + 1)
	}

	height := minH
	if maxH > minH {
		height += rng.Intn(maxH - minH + 1)
	}

	return width, height
}

// generateShapeForStyle creates a shape with bounds appropriate to the style
func generateShapeForStyle(width, height int, style dungeon.ShapeStyle, rng *rand.Rand) *dungeon.Shape {
	switch style {
	case dungeon.ShapeStyleStructured:
		return generateStructuredShape(width, height, rng)
	case dungeon.ShapeStyleOrganic:
		return generateOrganicShape(width, height, rng)
	case dungeon.ShapeStyleMixed:
		// Randomly choose between structured and organic
		if rng.Float64() < 0.5 {
			return generateStructuredShape(width, height, rng)
		}
		return generateOrganicShape(width, height, rng)
	default:
		return generateStructuredShape(width, height, rng)
	}
}

// generateStructuredShape creates a rectangular room with clean edges
func generateStructuredShape(width, height int, _ *rand.Rand) *dungeon.Shape {
	// Simple rectangle using cube coordinates
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

// generateOrganicShape creates an irregular cave-like room
func generateOrganicShape(width, height int, rng *rand.Rand) *dungeon.Shape {
	// For now, create a rectangle with some irregular edges
	// In a full implementation, this would create truly organic shapes
	// with varying edge positions

	variation := 2 // How much edges can vary

	// Use cube coordinates for bounds
	bounds := []dungeon.Position{
		offsetToCube(rng.Intn(variation), 0),
		offsetToCube(width-1-rng.Intn(variation), rng.Intn(variation)),
		offsetToCube(width-1, height-1-rng.Intn(variation)),
		offsetToCube(rng.Intn(variation), height-1),
	}

	// Calculate approximate area (simplified)
	area := width * height

	return &dungeon.Shape{
		Bounds:   bounds,
		GridType: dungeon.GridTypeHex,
		Width:    width,
		Height:   height,
		Area:     area,
	}
}
