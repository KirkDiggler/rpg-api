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
		Rooms:  []authoringorch.FloorPlanRoom{},
		Width:  3,
		Height: 2,
		FloorCells: []authoringorch.FloorPlanCell{
			{Column: 0, Row: 0},
			{Column: 0, Row: 1},
			{Column: 1, Row: 0},
			{Column: 1, Row: 1},
			{Column: 2, Row: 0},
			{Column: 2, Row: 1},
		},
		Entrance: authoringorch.FloorPlanCell{Column: 1, Row: 1},
		Edges: []authoringorch.FloorPlanEdge{{
			From:   authoringorch.FloorPlanCell{Column: 1, Row: 0},
			To:     authoringorch.FloorPlanCell{Column: 1, Row: 1},
			Kind:   authoringorch.FloorPlanEdgeKindDoor,
			DoorID: "canvas-door",
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
	require.Equal(t, "canvas-door", got.GetEdges()[0].GetDoorId())
}
