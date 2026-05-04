package toolkit

import (
	"fmt"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// PerimeterGenerator creates room boundary walls with door openings
type PerimeterGenerator struct{}

// NewPerimeterGenerator creates a new perimeter generator
func NewPerimeterGenerator() *PerimeterGenerator {
	return &PerimeterGenerator{}
}

// PerimeterInput contains parameters for perimeter generation
type PerimeterInput struct {
	Shape       *dungeon.Shape
	Connections []*dungeon.RoomConnection
}

// PerimeterOutput contains the generated perimeter walls
type PerimeterOutput struct {
	Walls         []dungeon.WallSegment
	DoorPositions []dungeon.LocalPosition
}

// Generate creates perimeter walls around the room shape with door openings
func (g *PerimeterGenerator) Generate(input *PerimeterInput) *PerimeterOutput {
	if input == nil || input.Shape == nil || len(input.Shape.Bounds) < 3 {
		return &PerimeterOutput{}
	}

	var walls []dungeon.WallSegment
	var doorPositions []dungeon.LocalPosition
	wallID := 0

	// Direction mapping for rectangular rooms: segment 0=south, 1=east, 2=north, 3=west
	// Convention: "south" = row=0/Z=0 edge, "north" = row=height/Z=height edge.
	// This matches createRectangleShape connection points and toolkit shapes.
	directions := []string{"south", "east", "north", "west"}

	bounds := input.Shape.Bounds
	numPoints := len(bounds)

	for i := 0; i < numPoints; i++ {
		start := bounds[i]
		end := bounds[(i+1)%numPoints]

		// Get direction for this segment
		direction := ""
		if i < len(directions) {
			direction = directions[i]
		}

		// Check if any connection should create a door on this segment
		doorConn := g.findConnectionOnSegment(input.Connections, i)

		if doorConn != nil {
			// Split wall segment for door using ConnectionPoints
			doorPos := g.calculateDoorPosition(input.Shape, direction, start, end)
			doorPositions = append(doorPositions, doorPos)

			// Wall before door
			beforeDoor := g.createWallBeforeDoor(start, doorPos, wallID)
			wallID++
			walls = append(walls, beforeDoor)

			// Wall after door
			afterDoor := g.createWallAfterDoor(doorPos, end, wallID)
			wallID++
			walls = append(walls, afterDoor)
		} else {
			// Solid wall segment
			wall := dungeon.WallSegment{
				ID:                fmt.Sprintf("perimeter_%d", wallID),
				Start:             start,
				End:               end,
				Type:              dungeon.WallTypeIndestructible,
				BlocksMovement:    true,
				BlocksLineOfSight: true,
			}
			wallID++
			walls = append(walls, wall)
		}
	}

	return &PerimeterOutput{
		Walls:         walls,
		DoorPositions: doorPositions,
	}
}

func (g *PerimeterGenerator) findConnectionOnSegment(
	connections []*dungeon.RoomConnection,
	segmentIndex int,
) *dungeon.RoomConnection {
	// Map segment index to direction (assuming rectangular room: 0=south, 1=east, 2=north, 3=west)
	directions := []string{"south", "east", "north", "west"}
	if segmentIndex >= len(directions) {
		return nil
	}

	segmentDirection := directions[segmentIndex]

	for _, conn := range connections {
		if conn.PhysicalHint == segmentDirection {
			return conn
		}
	}
	return nil
}

// getDoorPositionFromConnectionPoints finds the door position using shape's ConnectionPoints
func (g *PerimeterGenerator) getDoorPositionFromConnectionPoints(shape *dungeon.Shape, direction string) *dungeon.LocalPosition {
	if shape == nil {
		return nil
	}

	for _, cp := range shape.ConnectionPoints {
		if cp.Direction == direction {
			return &cp.Position
		}
	}
	return nil
}

func (g *PerimeterGenerator) calculateDoorPosition(shape *dungeon.Shape, direction string, start, end dungeon.LocalPosition) dungeon.LocalPosition {
	// First try to use ConnectionPoints from the shape
	if pos := g.getDoorPositionFromConnectionPoints(shape, direction); pos != nil {
		return *pos
	}

	// Fallback: door is placed at midpoint of wall segment.
	// NOTE: this midpoint math reads existing cube X/Y as if they were 2D offset
	// X/Y. That's a 2D-vestige carried over from the pre-cube perimeter logic;
	// see issue #471 / design.md. Phase 2 keeps the math intact for behavioral
	// equivalence; Phase 3 (and #402's perimeter cleanup) will revisit.
	midX := (start.X + end.X) / 2
	midY := (start.Y + end.Y) / 2
	return dungeon.LocalPosition{
		X: midX,
		Y: midY,
		Z: -midX - midY, // Maintain cube coordinate constraint: x + y + z = 0
	}
}

func (g *PerimeterGenerator) createWallBeforeDoor(start, doorPos dungeon.LocalPosition, id int) dungeon.WallSegment {
	// Leave gap of 2 units for door
	doorGap := 1
	var adjustedEnd dungeon.LocalPosition
	if start.X == doorPos.X {
		// Vertical wall
		endX := doorPos.X
		endY := doorPos.Y - doorGap
		adjustedEnd = dungeon.LocalPosition{X: endX, Y: endY, Z: -endX - endY}
	} else {
		endX := doorPos.X - doorGap
		endY := doorPos.Y
		adjustedEnd = dungeon.LocalPosition{X: endX, Y: endY, Z: -endX - endY}
	}

	return dungeon.WallSegment{
		ID:                fmt.Sprintf("perimeter_%d", id),
		Start:             start,
		End:               adjustedEnd,
		Type:              dungeon.WallTypeIndestructible,
		BlocksMovement:    true,
		BlocksLineOfSight: true,
	}
}

func (g *PerimeterGenerator) createWallAfterDoor(doorPos, end dungeon.LocalPosition, id int) dungeon.WallSegment {
	doorGap := 1
	var adjustedStart dungeon.LocalPosition
	if doorPos.X == end.X {
		// Vertical wall
		startX := doorPos.X
		startY := doorPos.Y + doorGap
		adjustedStart = dungeon.LocalPosition{X: startX, Y: startY, Z: -startX - startY}
	} else {
		startX := doorPos.X + doorGap
		startY := doorPos.Y
		adjustedStart = dungeon.LocalPosition{X: startX, Y: startY, Z: -startX - startY}
	}

	return dungeon.WallSegment{
		ID:                fmt.Sprintf("perimeter_%d", id),
		Start:             adjustedStart,
		End:               end,
		Type:              dungeon.WallTypeIndestructible,
		BlocksMovement:    true,
		BlocksLineOfSight: true,
	}
}

// UpdatePerimeter implements dungeon.PerimeterUpdater
// Regenerates perimeter walls with door openings based on room connections
func (g *PerimeterGenerator) UpdatePerimeter(input *dungeon.UpdatePerimeterInput) *dungeon.UpdatePerimeterOutput {
	if input == nil || input.Shape == nil {
		return &dungeon.UpdatePerimeterOutput{}
	}

	// Generate perimeter with connections
	output := g.Generate(&PerimeterInput{
		Shape:       input.Shape,
		Connections: input.Connections,
	})

	return &dungeon.UpdatePerimeterOutput{
		Walls:         output.Walls,
		DoorPositions: output.DoorPositions,
	}
}
