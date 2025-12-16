// Package adapter provides adapters between dungeon and encounter packages to avoid import cycles.
package adapter

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon/encounter"
)

// encounterAdapter adapts encounter.EncounterGenerator to dungeon.EncounterGenerator
type encounterAdapter struct {
	gen encounter.EncounterGenerator
}

// NewEncounterAdapter creates an adapter for encounter generator
func NewEncounterAdapter(gen encounter.EncounterGenerator) dungeon.EncounterGenerator {
	return &encounterAdapter{gen: gen}
}

// Generate adapts the encounter generator interface
func (a *encounterAdapter) Generate(ctx context.Context, input *dungeon.EncounterInput) (*dungeon.EncounterOutput, error) {
	output, err := a.gen.Generate(ctx, &encounter.GenerateInput{
		MonsterPool: input.MonsterPool,
		BossPool:    input.BossPool,
		CRBudget:    input.CRBudget,
		IsBossRoom:  input.IsBossRoom,
		Seed:        input.Seed,
	})
	if err != nil {
		return nil, err
	}

	return &dungeon.EncounterOutput{
		Encounter: output.Encounter,
	}, nil
}

// budgetAllocator implements BudgetAllocator using encounter package
type budgetAllocator struct{}

// NewBudgetAllocator creates a new budget allocator
func NewBudgetAllocator() dungeon.BudgetAllocator {
	return &budgetAllocator{}
}

// AllocateBudget allocates CR budget across rooms
func (b *budgetAllocator) AllocateBudget(input *dungeon.BudgetInput) *dungeon.BudgetOutput {
	// Calculate total CR
	totalCR := float64(input.PartySize * input.TargetCR)

	output, err := encounter.AllocateBudget(&encounter.BudgetInput{
		TotalCR:   totalCR,
		RoomCount: input.TotalRooms,
		PartySize: input.PartySize,
		TargetCR:  float64(input.TargetCR),
	})
	if err != nil {
		// For now, return empty budgets if allocation fails
		// In a real implementation, we might want to handle this differently
		budgets := make([]float64, input.TotalRooms)
		evenSplit := totalCR / float64(input.TotalRooms)
		for i := range budgets {
			budgets[i] = evenSplit
		}
		return &dungeon.BudgetOutput{
			RoomBudgets: budgets,
		}
	}

	// The encounter package returns BossRoomBudget separately from RoomBudgets
	// We need to combine them into a single array where the last room is the boss
	allBudgets := make([]float64, input.TotalRooms)

	// Copy regular room budgets
	copy(allBudgets, output.RoomBudgets)

	// Add boss room budget as the last element
	allBudgets[input.TotalRooms-1] = output.BossRoomBudget

	return &dungeon.BudgetOutput{
		RoomBudgets: allBudgets,
	}
}
