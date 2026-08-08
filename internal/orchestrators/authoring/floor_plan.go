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

	plan := &FloorPlan{
		Rooms:      make([]FloorPlanRoom, len(providerPlan.Rooms)),
		Connectors: make([]FloorPlanConnector, len(providerPlan.Connectors)),
		Width:      providerPlan.Width,
		Height:     providerPlan.Height,
		DoorRow:    providerPlan.DoorRow,
		FloorCells: make([]FloorPlanCell, len(providerPlan.FloorCells)),
		Entrance:   floorPlanCellFromProvider(providerPlan.Entrance),
		Edges:      make([]FloorPlanEdge, len(providerPlan.Edges)),
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
	return plan, nil
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
