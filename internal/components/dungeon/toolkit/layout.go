package toolkit

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/google/uuid"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// Room type constants
const (
	roomTypeEntrance = "entrance"
	roomTypeRegular  = "regular"
	roomTypeBoss     = "boss"
)

// ToolkitLayoutConfig contains configuration for the toolkit layout generator
type ToolkitLayoutConfig struct {
	// Placeholder for future toolkit configuration
}

// ToolkitLayoutGenerator implements dungeon.LayoutGenerator using a simple graph-based approach
type ToolkitLayoutGenerator struct {
	random *rand.Rand
}

// NewToolkitLayoutGenerator creates a new layout generator
func NewToolkitLayoutGenerator(cfg *ToolkitLayoutConfig) *ToolkitLayoutGenerator {
	// #nosec G404 - Using math/rand for seeded procedural generation, not cryptographic purposes
	return &ToolkitLayoutGenerator{
		random: rand.New(rand.NewSource(0)), // Will be reseeded per generation
	}
}

// Generate creates a room layout using a simple graph-based approach
func (g *ToolkitLayoutGenerator) Generate(ctx context.Context, input *dungeon.LayoutInput) (*dungeon.LayoutOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Validate input
	if input.Length <= 0 {
		return nil, fmt.Errorf("length must be greater than 0")
	}

	// Reseed for deterministic generation
	if input.Seed != 0 {
		g.random.Seed(input.Seed)
	}

	// Generate layout based on type
	switch input.LayoutType {
	case dungeon.LayoutLinear:
		return g.generateLinearLayout(input.Length), nil
	case dungeon.LayoutBranching:
		return g.generateBranchingLayout(input.Length), nil
	case dungeon.LayoutHub:
		return g.generateHubLayout(input.Length), nil
	default:
		return g.generateLinearLayout(input.Length), nil
	}
}

// generateLinearLayout creates a straight sequence of connected rooms
func (g *ToolkitLayoutGenerator) generateLinearLayout(length int) *dungeon.LayoutOutput {
	// Create room slots
	roomSlots := make([]dungeon.RoomSlot, length)
	for i := 0; i < length; i++ {
		roomType := roomTypeRegular
		switch i {
		case 0:
			roomType = roomTypeEntrance
		case length - 1:
			roomType = roomTypeBoss
		}

		roomSlots[i] = dungeon.RoomSlot{
			ID:       uuid.New().String(),
			RoomType: roomType,
		}
	}

	// Create linear connections
	connections := make([]*dungeon.LayoutConnection, length-1)
	for i := 0; i < length-1; i++ {
		connections[i] = &dungeon.LayoutConnection{
			FromRoom:     i,
			ToRoom:       i + 1,
			Type:         dungeon.ConnectionTypeDoor,
			IsMainPath:   true,
			PhysicalHint: "forward",
		}
	}

	return &dungeon.LayoutOutput{
		RoomSlots:   roomSlots,
		Connections: connections,
		StartRoom:   0,
		BossRoom:    length - 1,
	}
}

// generateBranchingLayout creates a main path with side branches
func (g *ToolkitLayoutGenerator) generateBranchingLayout(length int) *dungeon.LayoutOutput {
	if length < 3 {
		return g.generateLinearLayout(length)
	}

	// Create room slots
	roomSlots := make([]dungeon.RoomSlot, length)
	for i := 0; i < length; i++ {
		roomType := roomTypeRegular
		switch i {
		case 0:
			roomType = roomTypeEntrance
		case length - 1:
			roomType = roomTypeBoss
		}

		roomSlots[i] = dungeon.RoomSlot{
			ID:       uuid.New().String(),
			RoomType: roomType,
		}
	}

	// Create main path (about 60% of rooms)
	mainPathLength := (length * 3) / 5
	if mainPathLength < 2 {
		mainPathLength = 2
	}
	if mainPathLength > length-1 {
		mainPathLength = length - 1
	}

	connections := make([]*dungeon.LayoutConnection, 0, length)

	// Main path connections
	for i := 0; i < mainPathLength; i++ {
		connections = append(connections, &dungeon.LayoutConnection{
			FromRoom:     i,
			ToRoom:       i + 1,
			Type:         dungeon.ConnectionTypeDoor,
			IsMainPath:   true,
			PhysicalHint: "forward",
		})
	}

	// Add side branches from main path
	branchStart := mainPathLength + 1
	for i := branchStart; i < length; i++ {
		// Connect to a random room on the main path (not entrance or last)
		mainPathRoom := 1
		if mainPathLength > 2 {
			mainPathRoom = 1 + g.random.Intn(mainPathLength-1)
		}

		connections = append(connections, &dungeon.LayoutConnection{
			FromRoom:     mainPathRoom,
			ToRoom:       i,
			Type:         dungeon.ConnectionTypeDoor,
			IsMainPath:   false,
			PhysicalHint: "side",
		})
	}

	return &dungeon.LayoutOutput{
		RoomSlots:   roomSlots,
		Connections: connections,
		StartRoom:   0,
		BossRoom:    mainPathLength,
	}
}

// generateHubLayout creates a central hub with rooms connected to it
func (g *ToolkitLayoutGenerator) generateHubLayout(length int) *dungeon.LayoutOutput {
	if length < 3 {
		return g.generateLinearLayout(length)
	}

	// Create room slots
	roomSlots := make([]dungeon.RoomSlot, length)
	roomSlots[0] = dungeon.RoomSlot{
		ID:       uuid.New().String(),
		RoomType: roomTypeEntrance,
	}

	// Hub is the second room
	roomSlots[1] = dungeon.RoomSlot{
		ID:       uuid.New().String(),
		RoomType: roomTypeRegular,
	}

	// Rest of the rooms connect to the hub
	for i := 2; i < length; i++ {
		roomType := roomTypeRegular
		if i == length-1 {
			roomType = roomTypeBoss
		}

		roomSlots[i] = dungeon.RoomSlot{
			ID:       uuid.New().String(),
			RoomType: roomType,
		}
	}

	// Create connections
	connections := make([]*dungeon.LayoutConnection, 0, length)

	// Connect entrance to hub
	connections = append(connections, &dungeon.LayoutConnection{
		FromRoom:     0,
		ToRoom:       1,
		Type:         dungeon.ConnectionTypeDoor,
		IsMainPath:   true,
		PhysicalHint: "to hub",
	})

	// Connect all other rooms to hub
	for i := 2; i < length; i++ {
		connections = append(connections, &dungeon.LayoutConnection{
			FromRoom:     1,
			ToRoom:       i,
			Type:         dungeon.ConnectionTypeDoor,
			IsMainPath:   i == length-1, // Only boss room is on main path
			PhysicalHint: "from hub",
		})
	}

	return &dungeon.LayoutOutput{
		RoomSlots:   roomSlots,
		Connections: connections,
		StartRoom:   0,
		BossRoom:    length - 1,
	}
}
