package authoring

// FloorPlanRoom is one compiled room's placement in the chain. Domain type
// (not a proto type) — the handler is the one conversion point at the
// handler/proto boundary (rpg-project's Boundary Rule), converting this
// into authoringv1alpha1.FloorPlanRoom. StartColumn is the room's absolute
// position in the compiled left-to-right chain — server-computed here (the
// ONE place in this arc allowed to do this arithmetic, since it's the
// producer of the compiled layout, not a consumer), never re-derived by a
// client from Width alone.
type FloorPlanRoom struct {
	ID          string
	Archetype   string
	Width       int
	StartColumn int
}

// FloorPlanConnector is one compiled connector's door + reserved gap
// column BETWEEN its two rooms — also server-computed, never derived
// client-side from room widths.
type FloorPlanConnector struct {
	DoorID     string
	Locked     bool
	FromRoomID string
	ToRoomID   string
	Column     int
}

// FloorPlanCell is an absolute [column, row] on the compiled grid.
type FloorPlanCell struct {
	Column int
	Row    int
}

// FloorPlanEdgeKind is the authoring-local meaning of a toolkit-generated
// physical edge. It deliberately is not the runtime encounter WallKind.
type FloorPlanEdgeKind string

const (
	FloorPlanEdgeKindSolid FloorPlanEdgeKind = "solid"
	FloorPlanEdgeKindDoor  FloorPlanEdgeKind = "door"
)

// FloorPlanEdge directly projects one toolkit canonical generated edge. The
// API does not sort, deduplicate, infer, or otherwise canonicalize this list.
type FloorPlanEdge struct {
	From   FloorPlanCell
	To     FloorPlanCell
	Kind   FloorPlanEdgeKind
	DoorID string
}

// FloorPlan is the compiled layout PutDungeon's success response carries.
// Entrance is the one value here a client genuinely cannot compute at all
// (a generator decision, SpaceData.Entrance) — distinct from
// StartColumn/Column, which a client could in principle (mis)compute from
// Width but shouldn't have to. See design.md's "Grid math is
// server-authoritative" principle and the FloorPlan.entrance proto
// comment (rpg-api-protos dnd5e/api/authoring/v1alpha1/service.proto) for
// why both classes get an explicit field.
type FloorPlan struct {
	Rooms      []FloorPlanRoom
	Connectors []FloorPlanConnector
	Height     int
	DoorRow    int
	Entrance   FloorPlanCell
	Edges      []FloorPlanEdge
}
