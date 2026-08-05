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
canvas: { width: 3, height: 2 }
rooms: []
start: [1, 1]
place:
  - { ref: "dnd5e:props:altar", at: [1, 0], facing: W }
walls:
  - { from: [1, 0], to: [1, 1], kind: door }
`

// TestCanvasProviderContract is the deliberate, minimal expected-red consumer
// test for rpg-toolkit#883. It names the whole API/provider seam rather than
// reproducing any geometry, validation, seating, or prior-state semantics in
// rpg-api:
//
//   - PriorState is toolkit-owned and opaque. PutDungeon obtains it from the
//     current registry entry and passes it verbatim before writeThrough/Put.
//     Its future region-cell occupancy is never inspected by this package.
//   - LoadWithPriorState validates candidate against that state before any API
//     disk or registry mutation; its named errors remain the response's field
//     error unchanged.
//   - BuildFloorPlan produces every wire-relevant canvas fact in provider
//     order. The API maps it only; it does not enumerate cells or edges.
//
// Once #883 supplies these symbols, the companion PutDungeon integration tests
// must prove validate_only nonmutation, failed-update atomicity, successful
// write/registry refresh/real reload, and room-chain regression using this
// same contract. They intentionally cannot be made honest until the provider
// exists.
func TestCanvasProviderContract(t *testing.T) {
	prior := dungeonspec.PriorState{}
	compiled, err := dungeonspec.LoadWithPriorState(
		[]byte(canvasYAML),
		dungeonspec.LoadConfig{PartyStartSeatCount: 4},
		prior,
	)
	require.NoError(t, err)

	plan, err := dungeonspec.BuildFloorPlan(context.Background(), dungeonspec.BuildFloorPlanInput{
		Compiled: compiled,
		Seed:     1,
	})
	require.NoError(t, err)
	require.Empty(t, plan.Rooms)
	require.Equal(t, 3, plan.Width)
	require.Equal(t, 2, plan.Height)
	require.Equal(t, []dungeonspec.FloorPlanCell{
		{Column: 0, Row: 0}, {Column: 0, Row: 1},
		{Column: 1, Row: 0}, {Column: 1, Row: 1},
		{Column: 2, Row: 0}, {Column: 2, Row: 1},
	}, plan.FloorCells)
	require.Equal(t, dungeonspec.FloorPlanCell{Column: 1, Row: 1}, plan.Entrance)
	require.Len(t, plan.Edges, 1)
}
