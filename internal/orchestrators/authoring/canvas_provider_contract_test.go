package authoring_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"
)

const canvasYAML = `version: 1
key: canvas-provider-contract
name: Canvas Provider Contract
height: 1
canvas: { width: 4, height: 2 }
rooms: []
start: [1, 1]
place:
  - { ref: "dnd5e:props:altar", at: [1, 0], facing: W }
walls:
  - { from: [1, 0], to: [1, 1], kind: door }
`

const roomChainYAML = `version: 1
key: room-chain-provider-contract
name: Room Chain Provider Contract
height: 8
rooms:
  - id: entrance
    archetype: entrance
    width: 6
  - id: boss
    archetype: boss
    width: 8
    boss: { ref: "dnd5e:monsters:skeleton-captain", at: [4, 2] }
connectors:
  - { from: entrance, to: boss }
`

// TestCanvasProviderContract pins the released bounds-only provider projection.
// Each complete source compiles standalone: API does not forward prior compiled
// occupancy or treat an explicit deletion/shrink as invalid on its own.
//
// BuildFloorPlan returns toolkit-produced wire facts in canonical order. API
// maps them only; it does not enumerate floor cells, normalize edges, or derive
// a door identity. The separate room-chain assertion prevents canvas support
// from weakening the pre-existing provider projection.
func TestCanvasProviderContract(t *testing.T) {
	config := dungeonspec.LoadConfig{PartyStartSeatCount: 4}
	compiled, err := dungeonspec.LoadWithConfig([]byte(canvasYAML), config)
	require.NoError(t, err)
	plan, err := dungeonspec.BuildFloorPlan(context.Background(), dungeonspec.BuildFloorPlanInput{
		Compiled: compiled,
		Seed:     1,
	})
	require.NoError(t, err)
	require.Empty(t, plan.Rooms)
	require.Equal(t, 4, plan.Width)
	require.Equal(t, 2, plan.Height)
	require.Equal(t, []dungeonspec.FloorPlanCell{
		{Column: 0, Row: 0}, {Column: 0, Row: 1},
		{Column: 1, Row: 0}, {Column: 1, Row: 1},
		{Column: 2, Row: 0}, {Column: 2, Row: 1},
		{Column: 3, Row: 0}, {Column: 3, Row: 1},
	}, plan.FloorCells)
	require.Equal(t, dungeonspec.FloorPlanCell{Column: 1, Row: 1}, plan.Entrance)
	require.Len(t, plan.Edges, 1)
	require.Equal(t, dungeonspec.FloorPlanCell{Column: 1, Row: 0}, plan.Edges[0].From)
	require.Equal(t, dungeonspec.FloorPlanCell{Column: 1, Row: 1}, plan.Edges[0].To)
	require.Equal(t, dungeonspec.FloorPlanEdgeKindDoor, plan.Edges[0].Kind)
	require.Equal(t, "canvas-provider-contract-authored-door-1--2-1--1--1-0", plan.Edges[0].DoorID)
}

func TestBuildFloorPlan_RoomChainProjectionRegression(t *testing.T) {
	compiled, err := dungeonspec.LoadWithConfig(
		[]byte(roomChainYAML),
		dungeonspec.LoadConfig{PartyStartSeatCount: 4},
	)
	require.NoError(t, err)

	plan, err := dungeonspec.BuildFloorPlan(context.Background(), dungeonspec.BuildFloorPlanInput{
		Compiled: compiled,
		Seed:     1,
	})
	require.NoError(t, err)
	require.Len(t, plan.Rooms, 2)
	require.Equal(t, "entrance", plan.Rooms[0].ID)
	require.Equal(t, 0, plan.Rooms[0].StartColumn)
	require.Equal(t, "boss", plan.Rooms[1].ID)
	require.Equal(t, 7, plan.Rooms[1].StartColumn)
	require.Len(t, plan.Connectors, 1)
	require.Equal(t, "room-chain-provider-contract-door-entrance-boss", plan.Connectors[0].DoorID)
	require.Equal(t, dungeonspec.FloorPlanCell{Column: 0, Row: 4}, plan.Entrance)
	require.NotEmpty(t, plan.Edges)
}

func TestBuildFloorPlan_SemanticRegionsMapProviderFactsVerbatim(t *testing.T) {
	const source = `version: 1
key: semantic-region-projection
name: Semantic Region Projection
canvas: { width: 3, height: 2 }
rooms: []
regions:
  - id: outer
    cells: [[0,0], [0,1]]
  - id: inner
    cells: [[0,0]]
  - id: empty
    cells: []
`
	compiled, err := dungeonspec.LoadWithConfig([]byte(source), dungeonspec.LoadConfig{PartyStartSeatCount: 1})
	require.NoError(t, err)
	plan, err := dungeonspec.BuildFloorPlan(context.Background(), dungeonspec.BuildFloorPlanInput{Compiled: compiled, Seed: 1})
	require.NoError(t, err)
	require.Len(t, plan.Regions, 3)
	require.Equal(t, "outer", plan.Regions[0].ID)
	require.Nil(t, plan.Regions[0].ParentID)
	require.Equal(t, "inner", plan.Regions[1].ID)
	require.Equal(t, "outer", *plan.Regions[1].ParentID)
	require.Equal(t, []dungeonspec.FloorPlanCell{{Column: 0, Row: 0}}, plan.Regions[1].Cells)
	require.Nil(t, plan.Regions[2].ParentID)
}
