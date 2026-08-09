package authoring_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

const validYAML = `version: 1
key: handler-test
name: Handler Test Dungeon
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

func newTestHandler(t *testing.T) *authoringhandler.Handler {
	t.Helper()
	orch, err := authoringorch.New(&authoringorch.Config{
		Registry:   dungeonregistry.New(nil),
		ContentDir: t.TempDir(),
	})
	require.NoError(t, err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	require.NoError(t, err)
	return h
}

func TestPutDungeon_KeyCharsetViolation_InvalidArgumentNoBody(t *testing.T) {
	h := newTestHandler(t)
	resp, err := h.PutDungeon(context.Background(), &authoringv1alpha1.PutDungeonRequest{
		Key: "Not Valid!", Yaml: validYAML, ValidateOnly: true,
	})
	require.Nil(t, resp)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())
}

func TestPutDungeon_ContentFailure_OKStatusSuccessFalse(t *testing.T) {
	h := newTestHandler(t)
	resp, err := h.PutDungeon(context.Background(), &authoringv1alpha1.PutDungeonRequest{
		Key:          "broken",
		Yaml:         "version: 1\nkey: broken\nname: Broken\nheight: 8\nrooms:\n  - id: only-one\n    archetype: entrance\n    width: 6\n",
		ValidateOnly: true,
	})
	require.NoError(t, err, "content failure must be an OK-status response, not a gRPC error")
	require.NotNil(t, resp)
	require.False(t, resp.GetSuccess())
	require.Len(t, resp.GetFieldErrors(), 1)
	require.NotEmpty(t, resp.GetFieldErrors()[0].GetMessage())
	require.Nil(t, resp.GetFloorPlan())
}

// TestPutDungeon_AuthoredWallFieldErrorPassesThroughOKResponse keeps YAML
// topology feedback on the authoring response channel: walls are toolkit
// content, not an API InvalidArgument/geometry decision.
func TestPutDungeon_AuthoredWallFieldErrorPassesThroughOKResponse(t *testing.T) {
	h := newTestHandler(t)
	badOddQWallYAML := `version: 1
key: handler-authored-wall-error
name: Handler Authored Wall Error
height: 8
rooms:
  - id: entrance
    archetype: entrance
    width: 6
  - id: boss
    archetype: boss
    width: 8
    boss: { ref: "dnd5e:monsters:skeleton-captain", at: [4, 2] }
walls:
  - { from: [7, 1], to: [8, 0], kind: solid }
connectors:
  - { from: entrance, to: boss }
`
	resp, err := h.PutDungeon(context.Background(), &authoringv1alpha1.PutDungeonRequest{
		Key:          "handler-authored-wall-error",
		Yaml:         badOddQWallYAML,
		ValidateOnly: true,
	})
	require.NoError(t, err)
	require.False(t, resp.GetSuccess())
	require.Nil(t, resp.GetFloorPlan())
	require.Len(t, resp.GetFieldErrors(), 1)
	require.Contains(t, resp.GetFieldErrors()[0].GetMessage(), "walls[0]: endpoints must be adjacent pointy-top odd-q floor hexes")
}

func TestPutDungeon_ValidateOnlyCanvas_ProjectsCompleteBuilderWireShapeWithoutMutation(t *testing.T) {
	const key = "handler-canvas"
	dir := t.TempDir()
	registry := dungeonregistry.New(nil)
	orch, err := authoringorch.New(&authoringorch.Config{Registry: registry, ContentDir: dir, PartyStartSeatCount: 4})
	require.NoError(t, err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	require.NoError(t, err)

	resp, err := h.PutDungeon(context.Background(), &authoringv1alpha1.PutDungeonRequest{
		Key: key, ValidateOnly: true, Yaml: `version: 1
key: handler-canvas
name: Handler Canvas
height: 1
canvas: { width: 4, height: 2 }
rooms: []
start: [1, 1]
walls:
  - { from: [0, 0], to: [0, 1], kind: solid }
  - { from: [1, 0], to: [1, 1], kind: door }
`,
	})
	require.NoError(t, err)
	require.True(t, resp.GetSuccess())
	require.Empty(t, resp.GetFieldErrors())
	fp := resp.GetFloorPlan()
	require.NotNil(t, fp)
	require.Empty(t, fp.GetRooms())
	require.Empty(t, fp.GetConnectors())
	require.Equal(t, int32(4), fp.GetWidth())
	require.Equal(t, int32(2), fp.GetHeight())
	require.Equal(t, []*authoringv1alpha1.FloorPlanCell{
		{Column: 0, Row: 0}, {Column: 0, Row: 1}, {Column: 1, Row: 0}, {Column: 1, Row: 1},
		{Column: 2, Row: 0}, {Column: 2, Row: 1}, {Column: 3, Row: 0}, {Column: 3, Row: 1},
	}, fp.GetFloorCells(), "provider's canonical ascending floor cells must reach the wire unchanged")
	require.Equal(t, &authoringv1alpha1.FloorPlanCell{Column: 1, Row: 1}, fp.GetEntrance())
	require.Len(t, fp.GetEdges(), 24, "released provider's complete bounds envelope and authored edges must reach the wire")
	var authoredSolid, authoredDoor *authoringv1alpha1.FloorPlanEdge
	for _, edge := range fp.GetEdges() {
		if edge.GetFrom().GetColumn() == 0 && edge.GetFrom().GetRow() == 0 &&
			edge.GetTo().GetColumn() == 0 && edge.GetTo().GetRow() == 1 {
			authoredSolid = edge
		}
		if edge.GetDoorId() == "handler-canvas-authored-door-1--2-1--1--1-0" {
			authoredDoor = edge
		}
	}
	require.NotNil(t, authoredSolid)
	require.Equal(t, authoringv1alpha1.FloorPlanEdgeKind_FLOOR_PLAN_EDGE_KIND_SOLID, authoredSolid.GetKind())
	require.Nil(t, authoredSolid.DoorId, "solid edge must keep optional door_id absent")
	require.NotNil(t, authoredDoor)
	require.Equal(t, &authoringv1alpha1.FloorPlanCell{Column: 1, Row: 0}, authoredDoor.GetFrom())
	require.Equal(t, &authoringv1alpha1.FloorPlanCell{Column: 1, Row: 1}, authoredDoor.GetTo())
	require.NotNil(t, authoredDoor.DoorId, "door edge must keep optional door_id present")
	require.Empty(t, registry.Keys(), "validate_only must not mutate the live registry")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "validate_only must not write content")
}

func TestPutDungeon_Success_FloorPlanConvertedCorrectly(t *testing.T) {
	h := newTestHandler(t)
	resp, err := h.PutDungeon(context.Background(), &authoringv1alpha1.PutDungeonRequest{
		Key: "handler-test", Yaml: validYAML, ValidateOnly: true,
	})
	require.NoError(t, err)
	require.True(t, resp.GetSuccess())
	require.Empty(t, resp.GetFieldErrors())

	fp := resp.GetFloorPlan()
	require.NotNil(t, fp)
	require.Equal(t, int32(8), fp.GetHeight())
	require.Equal(t, int32(4), fp.GetDoorRow())
	require.Len(t, fp.GetRooms(), 2)
	require.Equal(t, "entrance", fp.GetRooms()[0].GetId())
	require.Equal(t, int32(0), fp.GetRooms()[0].GetStartColumn())
	require.Equal(t, "boss", fp.GetRooms()[1].GetId())
	require.Equal(t, int32(7), fp.GetRooms()[1].GetStartColumn())
	require.Len(t, fp.GetConnectors(), 1)
	require.Equal(t, "handler-test-door-entrance-boss", fp.GetConnectors()[0].GetDoorId())
	require.Equal(t, int32(6), fp.GetConnectors()[0].GetColumn())
	require.NotNil(t, fp.GetEntrance())
	require.Equal(t, int32(0), fp.GetEntrance().GetColumn())
	require.Equal(t, int32(4), fp.GetEntrance().GetRow())
	require.NotEmpty(t, fp.GetEdges())

	var solid, door *authoringv1alpha1.FloorPlanEdge
	for _, edge := range fp.GetEdges() {
		switch edge.GetKind() {
		case authoringv1alpha1.FloorPlanEdgeKind_FLOOR_PLAN_EDGE_KIND_SOLID:
			solid = edge
		case authoringv1alpha1.FloorPlanEdgeKind_FLOOR_PLAN_EDGE_KIND_DOOR:
			door = edge
		}
	}
	require.NotNil(t, solid)
	require.Nil(t, solid.DoorId, "solid edges must preserve the optional door_id's absence")
	require.NotNil(t, door)
	require.NotNil(t, door.DoorId, "door edges must map door_id as present")
	require.Equal(t, "handler-test-door-entrance-boss", door.GetDoorId())
}
