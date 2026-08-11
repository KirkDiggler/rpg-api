package authoring

import (
	"testing"

	"github.com/stretchr/testify/require"

	authoringv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// TestToProtoFloorPlan_CanvasPreservesProviderProjection is deliberately a
// pure boundary test: Width, FloorCells, Entrance, and Edges are already
// toolkit-produced facts. The handler must preserve their order and values;
// it must not manufacture canvas geometry or normalize edges.
func TestToProtoFloorPlan_CanvasPreservesProviderProjection(t *testing.T) {
	plan := &authoringorch.FloorPlan{
		FloorSource: authoringorch.FloorSourceBounds,
		Rooms:       []authoringorch.FloorPlanRoom{},
		Width:       3,
		Height:      2,
		FloorCells: []authoringorch.FloorPlanCell{
			{Column: 0, Row: 0},
			{Column: 0, Row: 1},
			{Column: 1, Row: 0},
			{Column: 1, Row: 1},
			{Column: 2, Row: 0},
			{Column: 2, Row: 1},
		},
		Entrance: &authoringorch.FloorPlanCell{Column: 1, Row: 1},
		Edges: []authoringorch.FloorPlanEdge{{
			From:   authoringorch.FloorPlanCell{Column: 1, Row: 0},
			To:     authoringorch.FloorPlanCell{Column: 1, Row: 1},
			Kind:   authoringorch.FloorPlanEdgeKindDoor,
			DoorID: "canvas-provider-contract-authored-door-1--2-1--1--1-0",
		}},
	}

	got := toProtoFloorPlan(plan)
	require.Empty(t, got.GetRooms())
	require.Equal(t, int32(3), got.GetWidth())
	require.Equal(t, int32(2), got.GetHeight())
	require.Equal(t, int32(1), got.GetEntrance().GetColumn())
	require.Equal(t, int32(1), got.GetEntrance().GetRow())
	require.Len(t, got.GetFloorCells(), 6)
	for i, want := range plan.FloorCells {
		require.Equal(t, int32(want.Column), got.GetFloorCells()[i].GetColumn())
		require.Equal(t, int32(want.Row), got.GetFloorCells()[i].GetRow())
	}
	require.Len(t, got.GetEdges(), 1)
	require.Equal(t, int32(1), got.GetEdges()[0].GetFrom().GetColumn())
	require.Equal(t, int32(0), got.GetEdges()[0].GetFrom().GetRow())
	require.Equal(t, int32(1), got.GetEdges()[0].GetTo().GetColumn())
	require.Equal(t, int32(1), got.GetEdges()[0].GetTo().GetRow())
	require.Equal(t, authoringv1alpha1.FloorPlanEdgeKind_FLOOR_PLAN_EDGE_KIND_DOOR, got.GetEdges()[0].GetKind())
	require.Equal(t, "canvas-provider-contract-authored-door-1--2-1--1--1-0", got.GetEdges()[0].GetDoorId())
}

func TestToProtoFloorPlan_RegionsPreservesCellsAndParentPresence(t *testing.T) {
	parent := "outer"
	plan := &authoringorch.FloorPlan{FloorSource: authoringorch.FloorSourceBounds, Regions: []authoringorch.FloorPlanRegion{
		{ID: "outer", Cells: []authoringorch.FloorPlanCell{{Column: 0, Row: 0}}},
		{ID: "inner", Cells: []authoringorch.FloorPlanCell{{Column: 0, Row: 0}}, ParentID: &parent},
	}}

	got := toProtoFloorPlan(plan)
	require.Len(t, got.GetRegions(), 2)
	require.Nil(t, got.GetRegions()[0].ParentId)
	require.Equal(t, "inner", got.GetRegions()[1].GetId())
	require.Equal(t, "outer", got.GetRegions()[1].GetParentId())
	require.NotNil(t, got.GetRegions()[1].ParentId)
	require.Equal(t, int32(0), got.GetRegions()[1].GetCells()[0].GetColumn())
	require.Equal(t, int32(0), got.GetRegions()[1].GetCells()[0].GetRow())
}

func TestToProtoFloorPlan_WaveAResolvedRegionsAndAbsentEntrance(t *testing.T) {
	plan := &authoringorch.FloorPlan{
		FloorSource: authoringorch.FloorSourceRegions,
		FloorCells: []authoringorch.FloorPlanCell{
			{Column: 1, Row: 1}, {Column: 2, Row: 1},
		},
		Regions: []authoringorch.FloorPlanRegion{{
			ID: "draft", Cells: []authoringorch.FloorPlanCell{{Column: 1, Row: 1}, {Column: 2, Row: 1}},
		}},
		Edges: []authoringorch.FloorPlanEdge{{
			From: authoringorch.FloorPlanCell{Column: 2, Row: 2},
			To:   authoringorch.FloorPlanCell{Column: 2, Row: 1},
			Kind: authoringorch.FloorPlanEdgeKindSolid,
		}},
		// Entrance intentionally nil: [0,0] would be real data, not absence.
	}

	got := toProtoFloorPlan(plan)
	require.NotNil(t, got.FloorSource)
	require.Equal(t,
		authoringv1alpha1.FloorPlanFloorSource_FLOOR_PLAN_FLOOR_SOURCE_REGIONS,
		got.GetFloorSource())
	require.Nil(t, got.Entrance)
	require.Equal(t, int32(1), got.GetFloorCells()[0].GetColumn())
	require.Equal(t, int32(2), got.GetEdges()[0].GetFrom().GetColumn(), "pair orientation must be preserved")
	require.Equal(t, int32(2), got.GetEdges()[0].GetTo().GetColumn())
	require.Equal(t, "draft", got.GetRegions()[0].GetId())
}

func TestToProtoFloorPlan_PlacementOffsetPreservesPresenceAndProviderFields(t *testing.T) {
	east := uint32(0)
	plan := &authoringorch.FloorPlan{Placements: []authoringorch.FloorPlanPlacement{
		{
			Ref: "dnd5e:props:bookcase", At: authoringorch.FloorPlanCell{Column: 3, Row: 2},
			Facing: &east, BlocksMovement: true, BlocksLoS: false,
			SourcePath: "rooms[0].place[0]",
		},
		{
			Ref: "dnd5e:monsters:skeleton", At: authoringorch.FloorPlanCell{Column: -2, Row: 4},
			SourcePath: "rooms[1].place[2]", Offset: &authoringorch.PlacementOffset{},
		},
		{
			Ref: "dnd5e:monsters:skeleton-captain", At: authoringorch.FloorPlanCell{Column: 8, Row: 5},
			SourcePath: "rooms[1].boss", Offset: &authoringorch.PlacementOffset{X: -0.25, Y: 1.5, Z: 2.75},
		},
	}}

	got := toProtoFloorPlan(plan).GetPlacements()
	require.Len(t, got, 3)
	require.Equal(t, "dnd5e:props:bookcase", got[0].GetRef())
	require.Equal(t, int32(3), got[0].GetAt().GetColumn())
	require.NotNil(t, got[0].Facing, "explicit E=0 must remain present")
	require.Zero(t, got[0].GetFacing())
	require.True(t, got[0].GetBlocksMovement())
	require.False(t, got[0].GetBlocksLos())
	require.Equal(t, "rooms[0].place[0]", got[0].GetSourcePath())
	require.Nil(t, got[0].GetOffset(), "omission must remain absent")

	require.NotNil(t, got[1].GetOffset(), "explicit zero must remain present")
	require.Zero(t, got[1].GetOffset().GetX())
	require.Zero(t, got[1].GetOffset().GetY())
	require.Zero(t, got[1].GetOffset().GetZ())

	require.Equal(t, -0.25, got[2].GetOffset().GetX())
	require.Equal(t, 1.5, got[2].GetOffset().GetY())
	require.Equal(t, 2.75, got[2].GetOffset().GetZ())
}
