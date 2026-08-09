package authoring

import (
	"context"
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"
)

// FloorPlanRoom is one provider-compiled room placement.
type FloorPlanRoom struct {
	ID          string
	Archetype   string
	Width       int
	StartColumn int
}

// FloorPlanConnector is one provider-compiled room connector.
type FloorPlanConnector struct {
	DoorID     string
	Locked     bool
	FromRoomID string
	ToRoomID   string
	Column     int
}

// FloorPlanRegion is one provider-authored semantic scope. ParentID is nil
// when the provider resolves the scope directly under the implicit root.
type FloorPlanRegion struct {
	ID       string
	Cells    []FloorPlanCell
	ParentID *string
}

// FloorPlanCell is an absolute provider-produced grid cell.
type FloorPlanCell struct {
	Column int
	Row    int
}

// FloorPlanEdgeKind is the provider-produced meaning of a physical edge.
type FloorPlanEdgeKind string

const (
	FloorPlanEdgeKindSolid FloorPlanEdgeKind = "solid"
	FloorPlanEdgeKindDoor  FloorPlanEdgeKind = "door"
)

// FloorPlanEdge directly projects one toolkit canonical edge.
type FloorPlanEdge struct {
	From   FloorPlanCell
	To     FloorPlanCell
	Kind   FloorPlanEdgeKind
	DoorID string
}

// PlacementOffset is an optional mechanically inert world-axis translation.
// Presence distinguishes omission from an explicitly authored zero vector.
type PlacementOffset struct {
	X float64
	Y float64
	Z float64
}

// FloorPlanPlacement is one provider-compiled authored placement. At is
// absolute and every other field is copied from provider truth verbatim.
type FloorPlanPlacement struct {
	Ref            string
	At             FloorPlanCell
	Facing         *uint32
	BlocksMovement bool
	BlocksLoS      bool
	SourcePath     string
	Offset         *PlacementOffset
}

// FloorPlan is PutDungeon's provider-produced layout response.
type FloorPlan struct {
	Rooms      []FloorPlanRoom
	Regions    []FloorPlanRegion
	Connectors []FloorPlanConnector
	Width      int
	Height     int
	DoorRow    int
	FloorCells []FloorPlanCell
	Entrance   FloorPlanCell
	Edges      []FloorPlanEdge
	Placements []FloorPlanPlacement
}

// buildFloorPlan maps the complete toolkit projection field-for-field. Geometry,
// ordering, and canonicalization all remain in dungeonspec.BuildFloorPlan.
func buildFloorPlan(ctx context.Context, compiled dungeonspec.CompiledDungeon, seed int64) (*FloorPlan, error) {
	providerPlan, err := dungeonspec.BuildFloorPlan(ctx, dungeonspec.BuildFloorPlanInput{
		Compiled: compiled,
		Seed:     seed,
	})
	if err != nil {
		return nil, fmt.Errorf("build floor plan: %w", err)
	}
	if providerPlan.Entrance == nil {
		return nil, fmt.Errorf("build floor plan: provider returned nil entrance")
	}

	plan := &FloorPlan{
		Rooms:      make([]FloorPlanRoom, len(providerPlan.Rooms)),
		Connectors: make([]FloorPlanConnector, len(providerPlan.Connectors)),
		Width:      providerPlan.Width,
		Height:     providerPlan.Height,
		DoorRow:    providerPlan.DoorRow,
		FloorCells: make([]FloorPlanCell, len(providerPlan.FloorCells)),
		Entrance:   floorPlanCellFromProvider(*providerPlan.Entrance),
		Edges:      make([]FloorPlanEdge, len(providerPlan.Edges)),
		Placements: make([]FloorPlanPlacement, len(providerPlan.Placements)),
	}
	for index, room := range providerPlan.Rooms {
		plan.Rooms[index] = FloorPlanRoom{ID: room.ID, Archetype: room.Archetype, Width: room.Width, StartColumn: room.StartColumn}
	}
	if providerPlan.Regions != nil {
		plan.Regions = make([]FloorPlanRegion, len(providerPlan.Regions))
		for index, region := range providerPlan.Regions {
			plan.Regions[index] = FloorPlanRegion{
				ID:       region.ID,
				Cells:    make([]FloorPlanCell, len(region.Cells)),
				ParentID: cloneOptionalString(region.ParentID),
			}
			for cellIndex, cell := range region.Cells {
				plan.Regions[index].Cells[cellIndex] = floorPlanCellFromProvider(cell)
			}
		}
	}
	for index, connector := range providerPlan.Connectors {
		plan.Connectors[index] = FloorPlanConnector{
			DoorID: connector.DoorID, Locked: connector.Locked,
			FromRoomID: connector.FromRoomID, ToRoomID: connector.ToRoomID, Column: connector.Column,
		}
	}
	for index, cell := range providerPlan.FloorCells {
		plan.FloorCells[index] = floorPlanCellFromProvider(cell)
	}
	for index, edge := range providerPlan.Edges {
		plan.Edges[index] = FloorPlanEdge{
			From: floorPlanCellFromProvider(edge.From), To: floorPlanCellFromProvider(edge.To),
			Kind: FloorPlanEdgeKind(edge.Kind), DoorID: edge.DoorID,
		}
	}
	for index, placement := range providerPlan.Placements {
		plan.Placements[index] = FloorPlanPlacement{
			Ref: placement.Ref, At: floorPlanCellFromProvider(placement.At),
			Facing:         cloneOptionalUint32(placement.Facing),
			BlocksMovement: placement.BlocksMovement, BlocksLoS: placement.BlocksLoS,
			SourcePath: placement.SourcePath, Offset: placementOffsetFromProvider(placement.Offset),
		}
	}
	return plan, nil
}

func placementOffsetFromProvider(offset *dungeonspec.PlacementOffset) *PlacementOffset {
	if offset == nil {
		return nil
	}
	return &PlacementOffset{X: offset[0], Y: offset[1], Z: offset[2]}
}

func floorPlanCellFromProvider(cell dungeonspec.FloorPlanCell) FloorPlanCell {
	return FloorPlanCell{Column: cell.Column, Row: cell.Row}
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneOptionalUint32(value *uint32) *uint32 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
