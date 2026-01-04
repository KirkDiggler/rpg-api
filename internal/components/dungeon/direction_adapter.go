package dungeon

import (
	"strings"
)

// DirectionAdapter translates toolkit layout hints into cardinal directions
type DirectionAdapter struct{}

// NewDirectionAdapter creates a new direction adapter
func NewDirectionAdapter() *DirectionAdapter {
	return &DirectionAdapter{}
}

// DirectionAdapterInput contains the data needed to derive directions
type DirectionAdapterInput struct {
	Connections []*RoomConnection
	Layout      LayoutType
	RoomOrder   []string // Room IDs in traversal order (first room is entrance)
}

// DirectionAdapterOutput contains connections with directions assigned
type DirectionAdapterOutput struct {
	Connections []*RoomConnection
}

// AssignDirections derives cardinal directions from layout hints and room topology
func (a *DirectionAdapter) AssignDirections(input *DirectionAdapterInput) *DirectionAdapterOutput {
	if input == nil || len(input.Connections) == 0 {
		return &DirectionAdapterOutput{Connections: input.Connections}
	}

	// Build room index map for ordering
	roomIndex := make(map[string]int)
	for i, roomID := range input.RoomOrder {
		roomIndex[roomID] = i
	}

	// Process each connection
	for _, conn := range input.Connections {
		conn.Direction = a.deriveDirection(conn, input.Layout, roomIndex)
	}

	return &DirectionAdapterOutput{Connections: input.Connections}
}

// deriveDirection determines the cardinal direction for a connection
func (a *DirectionAdapter) deriveDirection(
	conn *RoomConnection,
	layout LayoutType,
	roomIndex map[string]int,
) Direction {
	// First, check if PhysicalHint already contains a direction
	if dir := a.parseDirectionFromHint(conn.PhysicalHint); dir != "" {
		return dir
	}

	// Derive direction based on layout type and room order
	fromIdx, fromOk := roomIndex[conn.FromRoom]
	toIdx, toOk := roomIndex[conn.ToRoom]

	if !fromOk || !toOk {
		// Can't determine order, use default
		return DirectionSouth
	}

	switch layout {
	case LayoutLinear:
		return a.linearDirection(fromIdx, toIdx, conn.IsMainPath)
	case LayoutBranching:
		return a.branchingDirection(fromIdx, toIdx, conn.IsMainPath)
	case LayoutHub:
		return a.hubDirection(fromIdx, toIdx)
	default:
		return a.linearDirection(fromIdx, toIdx, conn.IsMainPath)
	}
}

// parseDirectionFromHint extracts direction if hint contains one
func (a *DirectionAdapter) parseDirectionFromHint(hint string) Direction {
	h := strings.ToLower(hint)

	switch {
	case strings.Contains(h, "north"):
		return DirectionNorth
	case strings.Contains(h, "south"):
		return DirectionSouth
	case strings.Contains(h, "east"):
		return DirectionEast
	case strings.Contains(h, "west"):
		return DirectionWest
	case strings.Contains(h, "up"):
		return DirectionUp
	case strings.Contains(h, "down"):
		return DirectionDown
	default:
		return ""
	}
}

// linearDirection assigns directions for linear layouts
// Rooms progress from north to south (entrance at top, boss at bottom)
func (a *DirectionAdapter) linearDirection(fromIdx, toIdx int, _ bool) Direction {
	if toIdx > fromIdx {
		// Moving forward in the dungeon (deeper)
		return DirectionSouth
	}
	// Moving backward (toward entrance)
	return DirectionNorth
}

// branchingDirection assigns directions for branching layouts
func (a *DirectionAdapter) branchingDirection(fromIdx, toIdx int, isMainPath bool) Direction {
	if isMainPath {
		// Main path goes south
		if toIdx > fromIdx {
			return DirectionSouth
		}
		return DirectionNorth
	}
	// Side branches go east/west alternating
	if fromIdx%2 == 0 {
		return DirectionEast
	}
	return DirectionWest
}

// hubDirection assigns directions for hub layouts (rooms radiate from center)
func (a *DirectionAdapter) hubDirection(fromIdx, toIdx int) Direction {
	// Hub room is typically index 0, spokes radiate outward
	if fromIdx == 0 {
		// From hub to spoke - distribute around compass
		directions := []Direction{
			DirectionNorth,
			DirectionEast,
			DirectionSouth,
			DirectionWest,
		}
		return directions[toIdx%len(directions)]
	}
	// From spoke to hub - opposite of hub-to-spoke
	directions := []Direction{
		DirectionSouth,
		DirectionWest,
		DirectionNorth,
		DirectionEast,
	}
	return directions[fromIdx%len(directions)]
}
