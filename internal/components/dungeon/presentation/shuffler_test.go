package presentation

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ShufflerTestSuite struct {
	suite.Suite
}

func TestShufflerTestSuite(t *testing.T) {
	suite.Run(t, new(ShufflerTestSuite))
}

func (s *ShufflerTestSuite) TestShuffleConnections_EmptySlice() {
	connections := []*RoomConnection{}
	result := ShuffleConnections(connections, 12345)
	s.Empty(result)
}

func (s *ShufflerTestSuite) TestShuffleConnections_SingleConnection() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", Type: ConnectionTypeDoor, IsMainPath: true},
	}

	result := ShuffleConnections(connections, 12345)
	s.Len(result, 1)
	s.Equal(connections[0], result[0])
}

func (s *ShufflerTestSuite) TestShuffleConnections_PreservesAllConnections() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", Type: ConnectionTypeDoor, IsMainPath: true, PhysicalHint: "north door"},
		{FromRoom: "room2", ToRoom: "room3", Type: ConnectionTypePassage, IsMainPath: true, PhysicalHint: "east passage"},
		{FromRoom: "room2", ToRoom: "room4", Type: ConnectionTypeStairs, IsMainPath: false, PhysicalHint: "south stairs"},
		{FromRoom: "room3", ToRoom: "room5", Type: ConnectionTypeDoor, IsMainPath: false, PhysicalHint: "west door"},
	}

	result := ShuffleConnections(connections, 12345)

	// Should have same length
	s.Len(result, len(connections))

	// Should contain all the same connections (different order)
	for _, original := range connections {
		found := false
		for _, shuffled := range result {
			if original.FromRoom == shuffled.FromRoom &&
				original.ToRoom == shuffled.ToRoom &&
				original.PhysicalHint == shuffled.PhysicalHint {
				found = true
				break
			}
		}
		s.True(found, "all original connections should be present after shuffle")
	}
}

func (s *ShufflerTestSuite) TestShuffleConnections_DifferentOrder() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", Type: ConnectionTypeDoor, IsMainPath: true, PhysicalHint: "hint1"},
		{FromRoom: "room2", ToRoom: "room3", Type: ConnectionTypePassage, IsMainPath: true, PhysicalHint: "hint2"},
		{FromRoom: "room2", ToRoom: "room4", Type: ConnectionTypeStairs, IsMainPath: false, PhysicalHint: "hint3"},
		{FromRoom: "room3", ToRoom: "room5", Type: ConnectionTypeDoor, IsMainPath: false, PhysicalHint: "hint4"},
		{FromRoom: "room4", ToRoom: "room6", Type: ConnectionTypeDoor, IsMainPath: true, PhysicalHint: "hint5"},
	}

	result := ShuffleConnections(connections, 12345)

	// Check that order is actually different (with high probability)
	orderChanged := false
	for i := range connections {
		if connections[i].PhysicalHint != result[i].PhysicalHint {
			orderChanged = true
			break
		}
	}

	s.True(orderChanged, "order should be different after shuffle")
}

func (s *ShufflerTestSuite) TestShuffleConnections_DeterministicWithSameSeed() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", PhysicalHint: "hint1"},
		{FromRoom: "room2", ToRoom: "room3", PhysicalHint: "hint2"},
		{FromRoom: "room2", ToRoom: "room4", PhysicalHint: "hint3"},
		{FromRoom: "room3", ToRoom: "room5", PhysicalHint: "hint4"},
	}

	result1 := ShuffleConnections(connections, 12345)
	result2 := ShuffleConnections(connections, 12345)

	// Same seed should produce same order
	s.Len(result1, len(result2))
	for i := range result1 {
		s.Equal(result1[i].PhysicalHint, result2[i].PhysicalHint,
			"same seed should produce same shuffle order")
	}
}

func (s *ShufflerTestSuite) TestShuffleConnections_DifferentWithDifferentSeeds() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", PhysicalHint: "hint1"},
		{FromRoom: "room2", ToRoom: "room3", PhysicalHint: "hint2"},
		{FromRoom: "room2", ToRoom: "room4", PhysicalHint: "hint3"},
		{FromRoom: "room3", ToRoom: "room5", PhysicalHint: "hint4"},
		{FromRoom: "room4", ToRoom: "room6", PhysicalHint: "hint5"},
	}

	result1 := ShuffleConnections(connections, 12345)
	result2 := ShuffleConnections(connections, 54321)

	// Different seeds should produce different orders
	orderDifferent := false
	for i := range result1 {
		if result1[i].PhysicalHint != result2[i].PhysicalHint {
			orderDifferent = true
			break
		}
	}

	s.True(orderDifferent, "different seeds should produce different orders")
}

func (s *ShufflerTestSuite) TestShuffleConnections_DoesNotMutateOriginal() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", PhysicalHint: "hint1"},
		{FromRoom: "room2", ToRoom: "room3", PhysicalHint: "hint2"},
		{FromRoom: "room2", ToRoom: "room4", PhysicalHint: "hint3"},
	}

	// Keep a copy of original order
	originalOrder := make([]string, len(connections))
	for i, conn := range connections {
		originalOrder[i] = conn.PhysicalHint
	}

	_ = ShuffleConnections(connections, 12345)

	// Verify original slice wasn't mutated
	for i, conn := range connections {
		s.Equal(originalOrder[i], conn.PhysicalHint,
			"original slice should not be mutated")
	}
}

func (s *ShufflerTestSuite) TestShuffleConnections_MainPathNotFirst() {
	// Create connections where first few are main path
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", IsMainPath: true, PhysicalHint: "main1"},
		{FromRoom: "room2", ToRoom: "room3", IsMainPath: true, PhysicalHint: "main2"},
		{FromRoom: "room3", ToRoom: "room4", IsMainPath: true, PhysicalHint: "main3"},
		{FromRoom: "room2", ToRoom: "room5", IsMainPath: false, PhysicalHint: "side1"},
		{FromRoom: "room3", ToRoom: "room6", IsMainPath: false, PhysicalHint: "side2"},
	}

	// Run multiple shuffles with different seeds
	mainPathFirstCount := 0
	iterations := 20

	for i := 0; i < iterations; i++ {
		result := ShuffleConnections(connections, int64(i))
		if result[0].IsMainPath {
			mainPathFirstCount++
		}
	}

	// Main path should not ALWAYS be first (should be roughly 60% given 3/5 are main path)
	// But definitely not 100% of the time
	s.Less(mainPathFirstCount, iterations,
		"main path should not always be first after shuffle")

	// And should appear first sometimes (given 3/5 are main path)
	s.Greater(mainPathFirstCount, 0,
		"main path should sometimes be first (probabilistically)")
}

func (s *ShufflerTestSuite) TestShuffleConnections_TwoConnections() {
	connections := []*RoomConnection{
		{FromRoom: "room1", ToRoom: "room2", PhysicalHint: "first"},
		{FromRoom: "room2", ToRoom: "room3", PhysicalHint: "second"},
	}

	result := ShuffleConnections(connections, 12345)

	// Should have both connections
	s.Len(result, 2)

	// Should contain both hints
	hints := make(map[string]bool)
	for _, conn := range result {
		hints[conn.PhysicalHint] = true
	}

	s.True(hints["first"])
	s.True(hints["second"])
}

func (s *ShufflerTestSuite) TestShuffleConnections_ManyConnections() {
	// Create 10 connections
	connections := make([]*RoomConnection, 10)
	for i := range connections {
		connections[i] = &RoomConnection{
			FromRoom:     "room1",
			ToRoom:       "room2",
			IsMainPath:   i < 5, // First 5 are main path
			PhysicalHint: string(rune('A' + i)),
		}
	}

	result := ShuffleConnections(connections, 12345)

	// Should have all connections
	s.Len(result, 10)

	// Verify all hints are present
	originalHints := make(map[string]bool)
	resultHints := make(map[string]bool)

	for i := range connections {
		originalHints[connections[i].PhysicalHint] = true
		resultHints[result[i].PhysicalHint] = true
	}

	s.Equal(originalHints, resultHints, "should have same hints after shuffle")
}
