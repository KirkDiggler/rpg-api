package encounter

import "fmt"

// BudgetInput defines parameters for CR budget allocation
type BudgetInput struct {
	// TotalCR is the total CR budget to allocate
	TotalCR float64
	// RoomCount is the number of rooms in the dungeon
	RoomCount int
	// PartySize is the number of players
	PartySize int
	// TargetCR is the desired challenge rating
	TargetCR float64
}

// BudgetOutput contains the allocated CR budgets
type BudgetOutput struct {
	// BossRoomBudget is the CR allocated to the boss room (40% of total)
	BossRoomBudget float64
	// RoomBudgets contains CR for each regular room with progressive scaling
	RoomBudgets []float64
}

// MinimumRoomCR is the minimum CR budget per room to ensure at least one monster can spawn.
// Based on the cheapest monsters in theme pools (Skeleton at CR 0.25).
const MinimumRoomCR = 0.25

// AllocateBudget distributes CR across dungeon rooms with boss room priority.
// Ensures each room has at least MinimumRoomCR to spawn at least one monster.
func AllocateBudget(input *BudgetInput) (*BudgetOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	if input.RoomCount <= 0 {
		return nil, fmt.Errorf("RoomCount must be greater than 0")
	}

	if input.PartySize <= 0 {
		return nil, fmt.Errorf("PartySize must be greater than 0")
	}

	if input.TargetCR < 0 {
		return nil, fmt.Errorf("TargetCR cannot be negative")
	}

	// Calculate total CR based on D&D encounter math
	totalCR := input.TotalCR
	if totalCR == 0 {
		totalCR = float64(input.PartySize) * input.TargetCR
	}

	// Calculate number of regular rooms (total - 1 boss room)
	regularRoomCount := input.RoomCount - 1
	if regularRoomCount <= 0 {
		// Edge case: only boss room
		return &BudgetOutput{
			BossRoomBudget: totalCR,
			RoomBudgets:    []float64{},
		}, nil
	}

	// Ensure minimum total CR so every room can have at least one monster
	// Minimum = (regularRoomCount * MinimumRoomCR) + MinimumRoomCR for boss
	minimumTotalCR := float64(regularRoomCount+1) * MinimumRoomCR
	if totalCR < minimumTotalCR {
		totalCR = minimumTotalCR
	}

	// Allocate 40% to boss room (but at least MinimumRoomCR)
	bossRoomBudget := totalCR * 0.4
	if bossRoomBudget < MinimumRoomCR {
		bossRoomBudget = MinimumRoomCR
	}

	// Remaining budget distributed across regular rooms
	remainingBudget := totalCR - bossRoomBudget

	// Progressive scaling: later rooms are harder
	// Use weighted distribution where room weight = 1 + (index / roomCount)
	roomBudgets := make([]float64, regularRoomCount)
	totalWeight := 0.0
	weights := make([]float64, regularRoomCount)

	for i := 0; i < regularRoomCount; i++ {
		// Weight increases linearly from 1.0 to 2.0
		weight := 1.0 + (float64(i) / float64(regularRoomCount))
		weights[i] = weight
		totalWeight += weight
	}

	// Allocate budget proportional to weight, ensuring minimum per room
	for i := 0; i < regularRoomCount; i++ {
		budget := remainingBudget * (weights[i] / totalWeight)
		if budget < MinimumRoomCR {
			budget = MinimumRoomCR
		}
		roomBudgets[i] = budget
	}

	return &BudgetOutput{
		BossRoomBudget: bossRoomBudget,
		RoomBudgets:    roomBudgets,
	}, nil
}
