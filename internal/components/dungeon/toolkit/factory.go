// Package toolkit provides integration between rpg-api dungeon generation and rpg-toolkit infrastructure.
// This package provides generators that can later be enhanced with toolkit components.
package toolkit

import (
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon/adapter"
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon/encounter"
)

// ToolkitConfig contains configuration for creating toolkit-based generators
type ToolkitConfig struct {
	// Placeholder for future toolkit configuration
}

// ToolkitGenerators contains all generators needed for dungeon generation
type ToolkitGenerators struct {
	Layout    dungeon.LayoutGenerator
	Shape     dungeon.ShapeGenerator
	Feature   dungeon.FeatureGenerator
	Encounter dungeon.EncounterGenerator
	Budget    dungeon.BudgetAllocator
}

// NewToolkitGenerators creates all generators
func NewToolkitGenerators(cfg *ToolkitConfig) *ToolkitGenerators {
	// Create generators
	layoutGen := NewToolkitLayoutGenerator(&ToolkitLayoutConfig{})
	shapeGen := NewToolkitShapeGenerator()
	featureGen := NewToolkitFeatureGenerator()

	// Create encounter generator (uses our encounter package)
	encounterGen := encounter.NewGenerator()

	// Wrap encounter generator with adapter
	encounterAdapter := adapter.NewEncounterAdapter(encounterGen)

	// Create budget allocator
	budgetAllocator := adapter.NewBudgetAllocator()

	return &ToolkitGenerators{
		Layout:    layoutGen,
		Shape:     shapeGen,
		Feature:   featureGen,
		Encounter: encounterAdapter,
		Budget:    budgetAllocator,
	}
}

// CreateGenerator creates a fully configured dungeon generator with toolkit integration
func CreateGenerator(cfg *ToolkitConfig) *dungeon.Generator {
	generators := NewToolkitGenerators(cfg)

	return dungeon.NewGenerator(&dungeon.GeneratorConfig{
		LayoutGen:       generators.Layout,
		ShapeGen:        generators.Shape,
		FeatureGen:      generators.Feature,
		EncounterGen:    generators.Encounter,
		BudgetAllocator: generators.Budget,
	})
}
