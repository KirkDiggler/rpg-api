package authoring_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
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

// These regressions exercise the production API adapter against v0.53.0. They
// ensure Wave A region support does not alter the existing bounds and room-chain
// projections; API asserts provider facts without deriving any of them.
func TestPutDungeon_RealProviderBoundsProjectionRegression(t *testing.T) {
	orch, _, _ := waveAOrchestrator(t)
	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key: "canvas-provider-contract", YAML: canvasYAML, ValidateOnly: true,
	})
	require.NoError(t, err)
	require.True(t, out.Success)
	plan := out.FloorPlan
	require.Equal(t, authoring.FloorSourceBounds, plan.FloorSource)
	require.Empty(t, plan.Rooms)
	require.Equal(t, 4, plan.Width)
	require.Equal(t, 2, plan.Height)
	require.Equal(t, []authoring.FloorPlanCell{
		{Column: 0, Row: 0}, {Column: 0, Row: 1},
		{Column: 1, Row: 0}, {Column: 1, Row: 1},
		{Column: 2, Row: 0}, {Column: 2, Row: 1},
		{Column: 3, Row: 0}, {Column: 3, Row: 1},
	}, plan.FloorCells)
	require.Equal(t, &authoring.FloorPlanCell{Column: 1, Row: 1}, plan.Entrance)
	require.Len(t, plan.Edges, 23, "released provider includes the complete bounds envelope")
	require.Contains(t, plan.Edges, authoring.FloorPlanEdge{
		From:   authoring.FloorPlanCell{Column: 1, Row: 0},
		To:     authoring.FloorPlanCell{Column: 1, Row: 1},
		Kind:   authoring.FloorPlanEdgeKindDoor,
		DoorID: "canvas-provider-contract-authored-door-1--2-1--1--1-0",
	})
}

func TestPutDungeon_RealProviderRoomChainProjectionRegression(t *testing.T) {
	orch, _, _ := waveAOrchestrator(t)
	out, err := orch.PutDungeon(context.Background(), &authoring.PutDungeonInput{
		Key: "room-chain-provider-contract", YAML: roomChainYAML, ValidateOnly: true,
	})
	require.NoError(t, err)
	require.True(t, out.Success)
	plan := out.FloorPlan
	require.Len(t, plan.Rooms, 2)
	require.Equal(t, "entrance", plan.Rooms[0].ID)
	require.Equal(t, 0, plan.Rooms[0].StartColumn)
	require.Equal(t, "boss", plan.Rooms[1].ID)
	require.Equal(t, 7, plan.Rooms[1].StartColumn)
	require.Len(t, plan.Connectors, 1)
	require.Equal(t, "room-chain-provider-contract-door-entrance-boss", plan.Connectors[0].DoorID)
	require.Equal(t, &authoring.FloorPlanCell{Column: 0, Row: 4}, plan.Entrance)
	require.NotEmpty(t, plan.Edges)
	require.Nil(t, plan.FloorCells, "room chains retain their region-only legacy projection")
}
