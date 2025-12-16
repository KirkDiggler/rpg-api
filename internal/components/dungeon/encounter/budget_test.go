package encounter

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type BudgetTestSuite struct {
	suite.Suite
}

func TestBudgetTestSuite(t *testing.T) {
	suite.Run(t, new(BudgetTestSuite))
}

func (s *BudgetTestSuite) TestAllocateBudget_Success() {
	input := &BudgetInput{
		TotalCR:   12.0,
		RoomCount: 4,
		PartySize: 4,
		TargetCR:  3,
	}

	output, err := AllocateBudget(input)

	s.NoError(err)
	s.NotNil(output)

	// Boss room gets 40% (4.8)
	s.InDelta(4.8, output.BossRoomBudget, 0.01)

	// Remaining 60% (7.2) distributed across 3 rooms
	s.Equal(3, len(output.RoomBudgets))

	// Verify total equals input
	totalAllocated := output.BossRoomBudget
	for _, budget := range output.RoomBudgets {
		totalAllocated += budget
	}
	s.InDelta(input.TotalCR, totalAllocated, 0.01)
}

func (s *BudgetTestSuite) TestAllocateBudget_ProgressiveScaling() {
	input := &BudgetInput{
		TotalCR:   12.0,
		RoomCount: 5,
		PartySize: 4,
		TargetCR:  3,
	}

	output, err := AllocateBudget(input)

	s.NoError(err)
	s.NotNil(output)

	// Verify rooms get progressively harder (each budget > previous)
	s.Equal(4, len(output.RoomBudgets))
	for i := 1; i < len(output.RoomBudgets); i++ {
		s.Greater(output.RoomBudgets[i], output.RoomBudgets[i-1],
			"Room %d budget should be greater than room %d", i, i-1)
	}
}

func (s *BudgetTestSuite) TestAllocateBudget_CalculateTotalCR() {
	input := &BudgetInput{
		TotalCR:   0, // Let it calculate
		RoomCount: 4,
		PartySize: 4,
		TargetCR:  3,
	}

	output, err := AllocateBudget(input)

	s.NoError(err)
	s.NotNil(output)

	// Expected total: partySize * targetCR = 4 * 3 = 12
	expectedTotal := 12.0
	totalAllocated := output.BossRoomBudget
	for _, budget := range output.RoomBudgets {
		totalAllocated += budget
	}
	s.InDelta(expectedTotal, totalAllocated, 0.01)
}

func (s *BudgetTestSuite) TestAllocateBudget_SingleRoom() {
	input := &BudgetInput{
		TotalCR:   10.0,
		RoomCount: 1,
		PartySize: 4,
		TargetCR:  2.5,
	}

	output, err := AllocateBudget(input)

	s.NoError(err)
	s.NotNil(output)

	// Only boss room, gets all budget
	s.Equal(10.0, output.BossRoomBudget)
	s.Empty(output.RoomBudgets)
}

func (s *BudgetTestSuite) TestAllocateBudget_TwoRooms() {
	input := &BudgetInput{
		TotalCR:   10.0,
		RoomCount: 2,
		PartySize: 4,
		TargetCR:  2.5,
	}

	output, err := AllocateBudget(input)

	s.NoError(err)
	s.NotNil(output)

	// Boss room gets 40% (4.0)
	s.InDelta(4.0, output.BossRoomBudget, 0.01)

	// One regular room gets 60% (6.0)
	s.Equal(1, len(output.RoomBudgets))
	s.InDelta(6.0, output.RoomBudgets[0], 0.01)
}

func (s *BudgetTestSuite) TestAllocateBudget_NilInput() {
	output, err := AllocateBudget(nil)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "input is required")
}

func (s *BudgetTestSuite) TestAllocateBudget_ZeroRoomCount() {
	input := &BudgetInput{
		TotalCR:   10.0,
		RoomCount: 0,
		PartySize: 4,
		TargetCR:  2.5,
	}

	output, err := AllocateBudget(input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "RoomCount must be greater than 0")
}

func (s *BudgetTestSuite) TestAllocateBudget_NegativeRoomCount() {
	input := &BudgetInput{
		TotalCR:   10.0,
		RoomCount: -1,
		PartySize: 4,
		TargetCR:  2.5,
	}

	output, err := AllocateBudget(input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "RoomCount must be greater than 0")
}

func (s *BudgetTestSuite) TestAllocateBudget_ZeroPartySize() {
	input := &BudgetInput{
		TotalCR:   10.0,
		RoomCount: 4,
		PartySize: 0,
		TargetCR:  2.5,
	}

	output, err := AllocateBudget(input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "PartySize must be greater than 0")
}

func (s *BudgetTestSuite) TestAllocateBudget_NegativePartySize() {
	input := &BudgetInput{
		TotalCR:   10.0,
		RoomCount: 4,
		PartySize: -1,
		TargetCR:  2.5,
	}

	output, err := AllocateBudget(input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "PartySize must be greater than 0")
}

func (s *BudgetTestSuite) TestAllocateBudget_NegativeTargetCR() {
	input := &BudgetInput{
		TotalCR:   10.0,
		RoomCount: 4,
		PartySize: 4,
		TargetCR:  -1,
	}

	output, err := AllocateBudget(input)

	s.Error(err)
	s.Nil(output)
	s.Contains(err.Error(), "TargetCR cannot be negative")
}

func (s *BudgetTestSuite) TestAllocateBudget_LargeDungeon() {
	input := &BudgetInput{
		TotalCR:   30.0,
		RoomCount: 10,
		PartySize: 5,
		TargetCR:  6,
	}

	output, err := AllocateBudget(input)

	s.NoError(err)
	s.NotNil(output)

	// Verify boss room gets 40%
	s.InDelta(12.0, output.BossRoomBudget, 0.01)

	// Verify 9 regular rooms
	s.Equal(9, len(output.RoomBudgets))

	// Verify progressive scaling
	for i := 1; i < len(output.RoomBudgets); i++ {
		s.Greater(output.RoomBudgets[i], output.RoomBudgets[i-1])
	}

	// Verify total
	totalAllocated := output.BossRoomBudget
	for _, budget := range output.RoomBudgets {
		totalAllocated += budget
	}
	s.InDelta(input.TotalCR, totalAllocated, 0.01)
}
