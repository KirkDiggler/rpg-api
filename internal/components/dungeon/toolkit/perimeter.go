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
	DoorPositions []dungeon.Position
}

// Generate creates perimeter walls around the room shape with door openings
func (g *PerimeterGenerator) Generate(input *PerimeterInput) *PerimeterOutput {
	if input == nil || input.Shape == nil || len(input.Shape.Bounds) < 3 {
		return &PerimeterOutput{}
	}

	var walls []dungeon.WallSegment
	var doorPositions []dungeon.Position
	wallID := 0

	bounds := input.Shape.Bounds
	numPoints := len(bounds)

	for i := 0; i < numPoints; i++ {
		start := bounds[i]
		end := bounds[(i+1)%numPoints]

		// Check if any connection should create a door on this segment
		doorConn := g.findConnectionOnSegment(start, end, input.Connections, i)

		if doorConn != nil {
			// Split wall segment for door
			doorPos := g.calculateDoorPosition(start, end)
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
	_, _ dungeon.Position,
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

func (g *PerimeterGenerator) calculateDoorPosition(start, end dungeon.Position) dungeon.Position {
	// Door is placed at midpoint of wall segment
	midX := (start.X + end.X) / 2
	midY := (start.Y + end.Y) / 2
	return dungeon.Position{
		X: midX,
		Y: midY,
		Z: -midX - midY, // Maintain cube coordinate constraint: x + y + z = 0
	}
}

func (g *PerimeterGenerator) createWallBeforeDoor(start, doorPos dungeon.Position, id int) dungeon.WallSegment {
	// Leave gap of 2 units for door
	doorGap := 1
	var adjustedEnd dungeon.Position
	if start.X == doorPos.X {
		// Vertical wall
		endX := doorPos.X
		endY := doorPos.Y - doorGap
		adjustedEnd = dungeon.Position{X: endX, Y: endY, Z: -endX - endY}
	} else {
		endX := doorPos.X - doorGap
		endY := doorPos.Y
		adjustedEnd = dungeon.Position{X: endX, Y: endY, Z: -endX - endY}
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

func (g *PerimeterGenerator) createWallAfterDoor(doorPos, end dungeon.Position, id int) dungeon.WallSegment {
	doorGap := 1
	var adjustedStart dungeon.Position
	if doorPos.X == end.X {
		// Vertical wall
		startX := doorPos.X
		startY := doorPos.Y + doorGap
		adjustedStart = dungeon.Position{X: startX, Y: startY, Z: -startX - startY}
	} else {
		startX := doorPos.X + doorGap
		startY := doorPos.Y
		adjustedStart = dungeon.Position{X: startX, Y: startY, Z: -startX - startY}
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
