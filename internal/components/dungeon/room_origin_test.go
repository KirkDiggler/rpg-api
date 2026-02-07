package dungeon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type RoomOriginTestSuite struct {
	suite.Suite
}

func TestRoomOriginTestSuite(t *testing.T) {
	suite.Run(t, new(RoomOriginTestSuite))
}

// =============================================================================
// calculateNeighborOrigin Tests
// =============================================================================

func (s *RoomOriginTestSuite) TestCalculateNeighborOrigin_South() {
	current := &Room{
		ID:     "room-1",
		Shape:  &Shape{Width: 10, Height: 8},
		Origin: Position{X: 0, Y: 0, Z: 0},
	}
	neighbor := &Room{
		ID:    "room-2",
		Shape: &Shape{Width: 12, Height: 10},
	}

	result := calculateNeighborOrigin(current, neighbor, DirectionSouth)

	// South: Z increases by current height + 1
	s.Equal(0, result.X)
	s.Equal(9, result.Z)                  // 0 + 8 + 1
	s.Equal(-result.X-result.Z, result.Y) // y = -x - z
}

func (s *RoomOriginTestSuite) TestCalculateNeighborOrigin_North() {
	current := &Room{
		ID:     "room-1",
		Shape:  &Shape{Width: 10, Height: 8},
		Origin: Position{X: 0, Y: 0, Z: 0},
	}
	neighbor := &Room{
		ID:    "room-2",
		Shape: &Shape{Width: 12, Height: 10},
	}

	result := calculateNeighborOrigin(current, neighbor, DirectionNorth)

	// North: Z decreases by neighbor height + 1
	s.Equal(0, result.X)
	s.Equal(-11, result.Z)                // 0 - 10 - 1
	s.Equal(-result.X-result.Z, result.Y) // y = -x - z
}

func (s *RoomOriginTestSuite) TestCalculateNeighborOrigin_East() {
	current := &Room{
		ID:     "room-1",
		Shape:  &Shape{Width: 10, Height: 8},
		Origin: Position{X: 0, Y: 0, Z: 0},
	}
	neighbor := &Room{
		ID:    "room-2",
		Shape: &Shape{Width: 12, Height: 10},
	}

	result := calculateNeighborOrigin(current, neighbor, DirectionEast)

	// East: X increases by current width + 1
	s.Equal(11, result.X)                 // 0 + 10 + 1
	s.Equal(0, result.Z)                  // unchanged
	s.Equal(-result.X-result.Z, result.Y) // y = -x - z
}

func (s *RoomOriginTestSuite) TestCalculateNeighborOrigin_West() {
	current := &Room{
		ID:     "room-1",
		Shape:  &Shape{Width: 10, Height: 8},
		Origin: Position{X: 0, Y: 0, Z: 0},
	}
	neighbor := &Room{
		ID:    "room-2",
		Shape: &Shape{Width: 12, Height: 10},
	}

	result := calculateNeighborOrigin(current, neighbor, DirectionWest)

	// West: X decreases by neighbor width + 1
	s.Equal(-13, result.X)                // 0 - 12 - 1
	s.Equal(0, result.Z)                  // unchanged
	s.Equal(-result.X-result.Z, result.Y) // y = -x - z
}

func (s *RoomOriginTestSuite) TestCalculateNeighborOrigin_WithNonZeroOrigin() {
	current := &Room{
		ID:     "room-2",
		Shape:  &Shape{Width: 15, Height: 12},
		Origin: Position{X: 11, Y: -11, Z: 0},
	}
	neighbor := &Room{
		ID:    "room-3",
		Shape: &Shape{Width: 10, Height: 8},
	}

	result := calculateNeighborOrigin(current, neighbor, DirectionSouth)

	// South from non-zero origin: Z = 0 + 12 + 1 = 13
	s.Equal(11, result.X) // unchanged from current
	s.Equal(13, result.Z) // 0 + 12 + 1
	s.Equal(-result.X-result.Z, result.Y)
}

func (s *RoomOriginTestSuite) TestCalculateNeighborOrigin_NilShape() {
	current := &Room{
		ID:     "room-1",
		Shape:  nil,
		Origin: Position{X: 5, Y: -5, Z: 0},
	}
	neighbor := &Room{
		ID:    "room-2",
		Shape: &Shape{Width: 10, Height: 8},
	}

	result := calculateNeighborOrigin(current, neighbor, DirectionSouth)

	// Nil shape returns current origin unchanged
	s.Equal(current.Origin, result)
}

func (s *RoomOriginTestSuite) TestCalculateNeighborOrigin_Up() {
	current := &Room{
		ID:     "room-1",
		Shape:  &Shape{Width: 10, Height: 8},
		Origin: Position{X: 0, Y: 0, Z: 0},
	}
	neighbor := &Room{
		ID:    "room-2",
		Shape: &Shape{Width: 10, Height: 8},
	}

	result := calculateNeighborOrigin(current, neighbor, DirectionUp)

	// Up: Y increases by 1, XZ unchanged
	s.Equal(0, result.X)
	s.Equal(1, result.Y)
	s.Equal(0, result.Z)
}

func (s *RoomOriginTestSuite) TestCalculateNeighborOrigin_Down() {
	current := &Room{
		ID:     "room-1",
		Shape:  &Shape{Width: 10, Height: 8},
		Origin: Position{X: 0, Y: 0, Z: 0},
	}
	neighbor := &Room{
		ID:    "room-2",
		Shape: &Shape{Width: 10, Height: 8},
	}

	result := calculateNeighborOrigin(current, neighbor, DirectionDown)

	// Down: Y decreases by 1, XZ unchanged
	s.Equal(0, result.X)
	s.Equal(-1, result.Y)
	s.Equal(0, result.Z)
}

// =============================================================================
// calculateRoomOrigins Tests (BFS integration)
// =============================================================================

func (s *RoomOriginTestSuite) TestCalculateRoomOrigins_LinearDungeon() {
	// Create a 3-room linear dungeon: room-1 -> room-2 -> room-3
	rooms := []*Room{
		{ID: "room-1", Shape: &Shape{Width: 10, Height: 8}},
		{ID: "room-2", Shape: &Shape{Width: 12, Height: 10}},
		{ID: "room-3", Shape: &Shape{Width: 8, Height: 6}},
	}

	connections := []*RoomConnection{
		{FromRoom: "room-1", ToRoom: "room-2", Direction: DirectionSouth},
		{FromRoom: "room-2", ToRoom: "room-3", Direction: DirectionSouth},
	}

	gen := &Generator{}
	gen.calculateRoomOrigins(rooms, connections, "room-1")

	// Room 1: start at (0, 0, 0)
	s.Equal(Position{X: 0, Y: 0, Z: 0}, rooms[0].Origin)

	// Room 2: south of room 1 -> Z = 0 + 8 + 1 = 9
	s.Equal(0, rooms[1].Origin.X)
	s.Equal(9, rooms[1].Origin.Z)
	s.Equal(-rooms[1].Origin.X-rooms[1].Origin.Z, rooms[1].Origin.Y)

	// Room 3: south of room 2 -> Z = 9 + 10 + 1 = 20
	s.Equal(0, rooms[2].Origin.X)
	s.Equal(20, rooms[2].Origin.Z)
	s.Equal(-rooms[2].Origin.X-rooms[2].Origin.Z, rooms[2].Origin.Y)
}

func (s *RoomOriginTestSuite) TestCalculateRoomOrigins_BranchingDungeon() {
	// Create a branching dungeon:
	// room-1 (start) -> room-2 (south) -> room-3 (south)
	//                 -> room-4 (east)
	rooms := []*Room{
		{ID: "room-1", Shape: &Shape{Width: 10, Height: 8}},
		{ID: "room-2", Shape: &Shape{Width: 12, Height: 10}},
		{ID: "room-3", Shape: &Shape{Width: 8, Height: 6}},
		{ID: "room-4", Shape: &Shape{Width: 10, Height: 10}},
	}

	connections := []*RoomConnection{
		{FromRoom: "room-1", ToRoom: "room-2", Direction: DirectionSouth},
		{FromRoom: "room-2", ToRoom: "room-3", Direction: DirectionSouth},
		{FromRoom: "room-1", ToRoom: "room-4", Direction: DirectionEast},
	}

	gen := &Generator{}
	gen.calculateRoomOrigins(rooms, connections, "room-1")

	// Room 1: start at origin
	s.Equal(Position{X: 0, Y: 0, Z: 0}, rooms[0].Origin)

	// Room 4: east of room 1 -> X = 0 + 10 + 1 = 11
	s.Equal(11, rooms[3].Origin.X)
	s.Equal(0, rooms[3].Origin.Z)
	s.Equal(-rooms[3].Origin.X-rooms[3].Origin.Z, rooms[3].Origin.Y)
}

func (s *RoomOriginTestSuite) TestCalculateRoomOrigins_EmptyConnections() {
	rooms := []*Room{
		{ID: "room-1", Shape: &Shape{Width: 10, Height: 8}},
	}

	gen := &Generator{}
	gen.calculateRoomOrigins(rooms, nil, "room-1")

	// Single room still gets origin (0,0,0)
	s.Equal(Position{X: 0, Y: 0, Z: 0}, rooms[0].Origin)
}

func (s *RoomOriginTestSuite) TestCalculateRoomOrigins_SkipsEmptyFromRoom() {
	// Connections with empty FromRoom (entrance from outside) are skipped
	rooms := []*Room{
		{ID: "room-1", Shape: &Shape{Width: 10, Height: 8}},
		{ID: "room-2", Shape: &Shape{Width: 12, Height: 10}},
	}

	connections := []*RoomConnection{
		{FromRoom: "", ToRoom: "room-1", Direction: DirectionSouth}, // entrance
		{FromRoom: "room-1", ToRoom: "room-2", Direction: DirectionSouth},
	}

	gen := &Generator{}
	gen.calculateRoomOrigins(rooms, connections, "room-1")

	// Room 1: start at origin
	s.Equal(Position{X: 0, Y: 0, Z: 0}, rooms[0].Origin)

	// Room 2: south of room 1
	s.Equal(9, rooms[1].Origin.Z) // 0 + 8 + 1
}

func (s *RoomOriginTestSuite) TestCalculateRoomOrigins_OppositeDirections() {
	// Verify that going south then looking from the neighbor's perspective (north)
	// gives consistent results
	rooms := []*Room{
		{ID: "room-1", Shape: &Shape{Width: 10, Height: 8}},
		{ID: "room-2", Shape: &Shape{Width: 10, Height: 8}},
	}

	connections := []*RoomConnection{
		{FromRoom: "room-1", ToRoom: "room-2", Direction: DirectionSouth},
	}

	gen := &Generator{}
	gen.calculateRoomOrigins(rooms, connections, "room-1")

	// Room 2 should be south of room 1
	assert.Greater(s.T(), rooms[1].Origin.Z, rooms[0].Origin.Z,
		"room to the south should have higher Z")
}

func (s *RoomOriginTestSuite) TestCalculateRoomOrigins_EastWestConsistency() {
	rooms := []*Room{
		{ID: "room-1", Shape: &Shape{Width: 10, Height: 8}},
		{ID: "room-2", Shape: &Shape{Width: 12, Height: 10}},
	}

	connections := []*RoomConnection{
		{FromRoom: "room-1", ToRoom: "room-2", Direction: DirectionEast},
	}

	gen := &Generator{}
	gen.calculateRoomOrigins(rooms, connections, "room-1")

	// Room 2 should be east of room 1
	assert.Greater(s.T(), rooms[1].Origin.X, rooms[0].Origin.X,
		"room to the east should have higher X")
}
