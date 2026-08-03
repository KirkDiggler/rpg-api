package authoring

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// PutDungeon handles the PutDungeon RPC — the one conversion point at the
// handler/proto boundary (rpg-project's Boundary Rule) for both the
// request and the error-transport split plan.md S1 decided:
//
//   - *authoringorch.MalformedRequestError (key charset / key-YAML
//     mismatch) -> gRPC InvalidArgument status, no response body. There is
//     nothing meaningful to populate for this case — see that error
//     type's doc for why a non-OK status dropping the body is fine here
//     specifically.
//   - any OTHER orchestrator error (e.g. a write-through failure) ->
//     Internal. This is a server-side failure, never author feedback.
//   - a non-nil *authoringorch.PutDungeonOutput is ALWAYS gRPC OK —
//     Success distinguishes "YAML content failed validate/compile"
//     (field_errors populated, floor_plan unset) from "compiled cleanly"
//     (floor_plan set, field_errors empty).
func (h *Handler) PutDungeon(
	ctx context.Context,
	req *authoringv1alpha1.PutDungeonRequest,
) (*authoringv1alpha1.PutDungeonResponse, error) {
	out, err := h.orch.PutDungeon(ctx, &authoringorch.PutDungeonInput{
		Key:          req.GetKey(),
		YAML:         req.GetYaml(),
		ValidateOnly: req.GetValidateOnly(),
	})
	if err != nil {
		var malformed *authoringorch.MalformedRequestError
		if errors.As(err, &malformed) {
			return nil, status.Error(codes.InvalidArgument, malformed.Message)
		}
		return nil, status.Errorf(codes.Internal, "authoring: %v", err)
	}

	if !out.Success {
		return &authoringv1alpha1.PutDungeonResponse{
			Success: false,
			FieldErrors: []*dnd5ev1alpha1.ValidationError{
				{Message: out.FieldError},
			},
		}, nil
	}

	return &authoringv1alpha1.PutDungeonResponse{
		Success:   true,
		FloorPlan: toProtoFloorPlan(out.FloorPlan),
	}, nil
}

// toProtoFloorPlan converts the orchestrator's domain FloorPlan into its
// wire shape — the one place this handler does so, per the Boundary Rule.
func toProtoFloorPlan(fp *authoringorch.FloorPlan) *authoringv1alpha1.FloorPlan {
	if fp == nil {
		return nil
	}

	rooms := make([]*authoringv1alpha1.FloorPlanRoom, len(fp.Rooms))
	for i, r := range fp.Rooms {
		rooms[i] = &authoringv1alpha1.FloorPlanRoom{
			Id:          r.ID,
			Archetype:   r.Archetype,
			Width:       int32(r.Width),
			StartColumn: int32(r.StartColumn),
		}
	}

	connectors := make([]*authoringv1alpha1.FloorPlanConnector, len(fp.Connectors))
	for i, c := range fp.Connectors {
		connectors[i] = &authoringv1alpha1.FloorPlanConnector{
			DoorId:     c.DoorID,
			Locked:     c.Locked,
			FromRoomId: c.FromRoomID,
			ToRoomId:   c.ToRoomID,
			Column:     int32(c.Column),
		}
	}

	edges := make([]*authoringv1alpha1.FloorPlanEdge, len(fp.Edges))
	for i, edge := range fp.Edges {
		edges[i] = toProtoFloorPlanEdge(edge)
	}

	return &authoringv1alpha1.FloorPlan{
		Rooms:      rooms,
		Connectors: connectors,
		Edges:      edges,
		Height:     int32(fp.Height),
		DoorRow:    int32(fp.DoorRow),
		Entrance: &authoringv1alpha1.FloorPlanCell{
			Column: int32(fp.Entrance.Column),
			Row:    int32(fp.Entrance.Row),
		},
	}
}

func toProtoFloorPlanEdge(edge authoringorch.FloorPlanEdge) *authoringv1alpha1.FloorPlanEdge {
	protoEdge := &authoringv1alpha1.FloorPlanEdge{
		From: &authoringv1alpha1.FloorPlanCell{Column: int32(edge.From.Column), Row: int32(edge.From.Row)},
		To:   &authoringv1alpha1.FloorPlanCell{Column: int32(edge.To.Column), Row: int32(edge.To.Row)},
	}
	switch edge.Kind {
	case authoringorch.FloorPlanEdgeKindSolid:
		protoEdge.Kind = authoringv1alpha1.FloorPlanEdgeKind_FLOOR_PLAN_EDGE_KIND_SOLID
	case authoringorch.FloorPlanEdgeKindDoor:
		protoEdge.Kind = authoringv1alpha1.FloorPlanEdgeKind_FLOOR_PLAN_EDGE_KIND_DOOR
	}
	if edge.DoorID != "" {
		protoEdge.DoorId = &edge.DoorID
	}
	return protoEdge
}
