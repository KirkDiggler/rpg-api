package authoring_test

import (
	"context"
	"strings"
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

const previousCanvasYAML = `version: 1
key: canvas-provider-contract
name: Previous Canvas Provider Contract
height: 1
canvas: { width: 4, height: 2 }
rooms: []
start: [0, 1]
place:
  - { ref: "dnd5e:props:pillar", at: [3, 0] }
walls:
  - { from: [2, 0], to: [2, 1], kind: solid }
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

// TestCanvasProviderContract is the deliberate, minimal expected-red consumer
// test for rpg-toolkit#883. PutDungeon retrieves the previous
// dungeonspec.CompiledDungeon from its registry and passes that value unchanged
// to LoadWithPrevious before writeThrough or registry mutation. CompiledDungeon
// is the sole opaque state carrier: toolkit owns all private placement, wall,
// start, and future region-cell state within or extracted from it. API neither
// exposes nor reconstructs any of those values.
//
// BuildFloorPlan returns toolkit-produced wire facts in canonical order. API
// maps them only; it does not enumerate floor cells, normalize edges, or derive
// a door identity. The separate room-chain assertion prevents canvas support
// from weakening the pre-existing provider projection.
func TestCanvasProviderContract(t *testing.T) {
	config := dungeonspec.LoadConfig{PartyStartSeatCount: 4}
	previous, err := dungeonspec.LoadWithConfig([]byte(previousCanvasYAML), config)
	require.NoError(t, err)

	shrunkYAML := strings.Replace(canvasYAML, "width: 4", "width: 3", 1)
	_, err = dungeonspec.LoadWithPrevious([]byte(shrunkYAML), config, previous)
	require.ErrorContains(t, err, "place[0]",
		"a previous compiled placement at [3,0] must reject a shrinking candidate before API mutation")

	compiled, err := dungeonspec.LoadWithPrevious([]byte(canvasYAML), config, previous)
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
