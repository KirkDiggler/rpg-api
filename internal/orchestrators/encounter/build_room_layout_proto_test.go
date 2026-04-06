package encounter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

func TestBuildRoomLayoutProto_NilRoom(t *testing.T) {
	result := buildRoomLayoutProto(nil, nil)
	assert.Nil(t, result)
}

func TestBuildRoomLayoutProto_NilShape(t *testing.T) {
	room := &dungeon.Room{ID: "room-1", Shape: nil}
	result := buildRoomLayoutProto(room, nil)
	assert.Nil(t, result)
}

func TestBuildRoomLayoutProto_WallsShiftedByOrigin(t *testing.T) {
	// Room 2 has walls in room-local coordinates. With origin (5, -5, 0), the
	// absolute positions must be offset by that origin.
	room := &dungeon.Room{
		ID: "room-2",
		Shape: &dungeon.Shape{
			GridType: dungeon.GridTypeHex,
			Width:    5,
			Height:   5,
		},
		Walls: []dungeon.WallSegment{
			{
				ID:                "wall-1",
				Start:             dungeon.Position{X: 1, Y: -1, Z: 0},
				End:               dungeon.Position{X: 2, Y: -2, Z: 0},
				Type:              dungeon.WallTypeIndestructible,
				BlocksMovement:    true,
				BlocksLineOfSight: true,
			},
		},
	}

	// origin (5, -5, 0): X=5, Z=0, Y=-5 (derived from cube constraint -5-0).
	origin := &dungeon.Position{X: 5, Y: -5, Z: 0}

	result := buildRoomLayoutProto(room, origin)

	require.NotNil(t, result)
	require.Len(t, result.Walls, 1)

	wall := result.Walls[0]

	// Start: local (1, -1, 0) + origin (5, 0) => absolute X=6, Z=0, Y=-6-0=-6
	assert.Equal(t, 6.0, wall.Start.X, "Start.X should be local+origin.X")
	assert.Equal(t, 0.0, wall.Start.Z, "Start.Z should be local+origin.Z")
	assert.Equal(t, -6.0, wall.Start.Y, "Start.Y must satisfy cube constraint: -x-z")

	// End: local (2, -2, 0) + origin (5, 0) => absolute X=7, Z=0, Y=-7-0=-7
	assert.Equal(t, 7.0, wall.End.X, "End.X should be local+origin.X")
	assert.Equal(t, 0.0, wall.End.Z, "End.Z should be local+origin.Z")
	assert.Equal(t, -7.0, wall.End.Y, "End.Y must satisfy cube constraint: -x-z")

	assert.Equal(t, "stone", wall.Material)
	assert.True(t, wall.BlocksMovement)
	assert.True(t, wall.BlocksLineOfSight)
}

func TestBuildRoomLayoutProto_WallsNilOriginPassThrough(t *testing.T) {
	// When origin is nil the wall positions come through unchanged, but Y is still
	// recomputed from the cube constraint.
	room := &dungeon.Room{
		ID: "room-1",
		Shape: &dungeon.Shape{
			GridType: dungeon.GridTypeHex,
			Width:    3,
			Height:   3,
		},
		Walls: []dungeon.WallSegment{
			{
				ID:    "wall-1",
				Start: dungeon.Position{X: 2, Y: -3, Z: 1},
				End:   dungeon.Position{X: 3, Y: -4, Z: 1},
				Type:  dungeon.WallTypeDestructible,
			},
		},
	}

	result := buildRoomLayoutProto(room, nil)

	require.NotNil(t, result)
	require.Len(t, result.Walls, 1)

	wall := result.Walls[0]

	// No origin: X and Z pass through, Y recomputed from cube constraint.
	assert.Equal(t, 2.0, wall.Start.X)
	assert.Equal(t, 1.0, wall.Start.Z)
	assert.Equal(t, -3.0, wall.Start.Y, "Start.Y must satisfy cube constraint: -2-1=-3")

	assert.Equal(t, 3.0, wall.End.X)
	assert.Equal(t, 1.0, wall.End.Z)
	assert.Equal(t, -4.0, wall.End.Y, "End.Y must satisfy cube constraint: -3-1=-4")

	assert.Equal(t, "wood", wall.Material, "destructible walls use wood material")
}

func TestBuildRoomLayoutProto_OriginSetInProto(t *testing.T) {
	room := &dungeon.Room{
		ID: "room-3",
		Shape: &dungeon.Shape{
			GridType: dungeon.GridTypeHex,
			Width:    4,
			Height:   4,
		},
	}
	origin := &dungeon.Position{X: 5, Y: -5, Z: 0}

	result := buildRoomLayoutProto(room, origin)

	require.NotNil(t, result)
	require.NotNil(t, result.Origin)
	assert.Equal(t, 5.0, result.Origin.X)
	assert.Equal(t, -5.0, result.Origin.Y)
	assert.Equal(t, 0.0, result.Origin.Z)
}

func TestBuildRoomLayoutProto_MultipleWallsAllShifted(t *testing.T) {
	// Verify that every wall in a multi-wall room gets the offset applied.
	room := &dungeon.Room{
		ID: "room-4",
		Shape: &dungeon.Shape{
			GridType: dungeon.GridTypeHex,
			Width:    6,
			Height:   6,
		},
		Walls: []dungeon.WallSegment{
			{ID: "w1", Start: dungeon.Position{X: 0, Y: 0, Z: 0}, End: dungeon.Position{X: 1, Y: -1, Z: 0}},
			{ID: "w2", Start: dungeon.Position{X: 1, Y: -1, Z: 0}, End: dungeon.Position{X: 2, Y: -1, Z: -1}},
		},
	}

	origin := &dungeon.Position{X: 3, Y: -3, Z: 0}

	result := buildRoomLayoutProto(room, origin)

	require.NotNil(t, result)
	require.Len(t, result.Walls, 2)

	// Wall 0: start (0,0,0) + origin(3,0) => (3,0) => Y=-3-0=-3
	assert.Equal(t, 3.0, result.Walls[0].Start.X)
	assert.Equal(t, 0.0, result.Walls[0].Start.Z)
	assert.Equal(t, -3.0, result.Walls[0].Start.Y)

	// Wall 1: end (2,-1,-1) + origin(3,0) => (5,-1) => Y=-5-(-1)=-4
	assert.Equal(t, 5.0, result.Walls[1].End.X)
	assert.Equal(t, -1.0, result.Walls[1].End.Z)
	assert.Equal(t, -4.0, result.Walls[1].End.Y)
}
