package sessionv1alpha2

import (
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
)

// convert_geometry.go is the atlas geometry a perception act carries, spelled
// on session v1alpha2 (rpg-project#630, event-stream R18). It is a second
// geometry converter beside v1alpha1's on purpose: v1alpha2 imports nothing
// from v1alpha1, so neither do its converters. Every function copies; none
// derives a cell, a blocking answer or a frame.

func positionToProto(p spatial.Position) *sessionpb.Position {
	return &sessionpb.Position{X: p.X, Y: p.Y}
}

func positionsToProto(ps []spatial.Position) []*sessionpb.Position {
	if len(ps) == 0 {
		return nil
	}
	out := make([]*sessionpb.Position, len(ps))
	for i, p := range ps {
		out[i] = positionToProto(p)
	}
	return out
}

func footprintPointToProto(p sdk.FootprintPoint) *sessionpb.FootprintPoint {
	return &sessionpb.FootprintPoint{X: p.X, Y: p.Y}
}

func axialPointToProto(p sdk.AxialPointF) *sessionpb.AxialPoint {
	return &sessionpb.AxialPoint{Q: p.Q, R: p.R}
}

func regionsToProto(rs []sdk.AtlasRegion) []*sessionpb.AtlasRegion {
	return mapSlice(rs, func(r sdk.AtlasRegion) *sessionpb.AtlasRegion {
		return &sessionpb.AtlasRegion{
			Id: r.ID, Name: r.Name, Cells: positionsToProto(r.Cells), Archetype: r.Archetype,
			Lighting: &sessionpb.Lighting{Intensity: r.Lighting.Intensity},
		}
	})
}

func propsToProto(ps []sdk.AtlasProp) []*sessionpb.AtlasProp {
	return mapSlice(ps, func(p sdk.AtlasProp) *sessionpb.AtlasProp {
		return &sessionpb.AtlasProp{
			Id: p.ID, Holdable: p.Holdable, Ref: p.Ref, At: positionToProto(p.At),
			BlocksMovement: p.BlocksMovement, BlocksLineOfSight: p.BlocksLineOfSight, Facing: p.Facing,
			OffsetX: p.Offset[0], OffsetY: p.Offset[1], OffsetZ: p.Offset[2],
		}
	})
}

func placedToProto(ps []sdk.AtlasPlacedProp) []*sessionpb.AtlasPlacedProp {
	return mapSlice(ps, func(p sdk.AtlasPlacedProp) *sessionpb.AtlasPlacedProp {
		return &sessionpb.AtlasPlacedProp{
			Id: p.ID,
			Placement: &sessionpb.FootprintPlacement{
				Width: p.Placement.Width, Depth: p.Placement.Depth,
				Origin: footprintPointToProto(p.Placement.Origin), Facing: p.Placement.Facing,
				LocalOffset: footprintPointToProto(p.Placement.LocalOffset),
			},
			BlocksMovement: p.BlocksMovement, BlocksLineOfSight: p.BlocksLineOfSight, Holdable: p.Holdable,
			Cells: positionsToProto(p.Cells),
		}
	})
}

func boundariesToProto(bs []sdk.AtlasBoundary) []*sessionpb.AtlasBoundary {
	return mapSlice(bs, func(b sdk.AtlasBoundary) *sessionpb.AtlasBoundary {
		return &sessionpb.AtlasBoundary{
			From: positionToProto(b.From), To: positionToProto(b.To),
			BlocksMovement: b.BlocksMovement, BlocksLineOfSight: b.BlocksLineOfSight, Height: b.Height,
		}
	})
}

func segmentsToProto(ss []sdk.AtlasSegment) []*sessionpb.AtlasSegment {
	return mapSlice(ss, func(s sdk.AtlasSegment) *sessionpb.AtlasSegment {
		return &sessionpb.AtlasSegment{From: axialPointToProto(s.From), To: axialPointToProto(s.To), Height: s.Height}
	})
}

func doorwaysToProto(ds []sdk.AtlasDoorway) []*sessionpb.AtlasDoorway {
	return mapSlice(ds, func(d sdk.AtlasDoorway) *sessionpb.AtlasDoorway {
		return &sessionpb.AtlasDoorway{Door: d.Door, From: positionToProto(d.From), To: positionToProto(d.To)}
	})
}

func exitsToProto(es []sdk.AtlasExit) []*sessionpb.AtlasExit {
	return mapSlice(es, func(e sdk.AtlasExit) *sessionpb.AtlasExit {
		return &sessionpb.AtlasExit{Id: e.ID, At: positionToProto(e.At)}
	})
}

func openingsToProto(os []sdk.AtlasStructuralOpening) []*sessionpb.AtlasStructuralOpening {
	return mapSlice(os, func(o sdk.AtlasStructuralOpening) *sessionpb.AtlasStructuralOpening {
		return &sessionpb.AtlasStructuralOpening{Id: o.ID, Position: o.Position, Width: o.Width}
	})
}

func structuralWallsToProto(ws []sdk.AtlasStructuralWall) []*sessionpb.AtlasStructuralWall {
	return mapSlice(ws, func(w sdk.AtlasStructuralWall) *sessionpb.AtlasStructuralWall {
		return &sessionpb.AtlasStructuralWall{
			Id: w.ID, Ref: w.Ref, From: footprintPointToProto(w.From), To: footprintPointToProto(w.To),
			Height: w.Height, Thickness: w.Thickness, Elevation: w.Elevation,
			FootprintCells: positionsToProto(w.FootprintCells), Openings: openingsToProto(w.Openings),
		}
	})
}

func structuralDoorsToProto(ds []sdk.AtlasStructuralDoor) []*sessionpb.AtlasStructuralDoor {
	return mapSlice(ds, func(d sdk.AtlasStructuralDoor) *sessionpb.AtlasStructuralDoor {
		return &sessionpb.AtlasStructuralDoor{
			Id: d.ID, Ref: d.Ref, From: footprintPointToProto(d.From), To: footprintPointToProto(d.To),
			Height: d.Height, Thickness: d.Thickness, Elevation: d.Elevation,
		}
	})
}

func openingReplacementsToProto(rs []sdk.StructuralWallOpeningsReplacement) []*sessionpb.StructuralWallOpeningsReplacement {
	return mapSlice(rs, func(r sdk.StructuralWallOpeningsReplacement) *sessionpb.StructuralWallOpeningsReplacement {
		return &sessionpb.StructuralWallOpeningsReplacement{WallId: r.WallID, Openings: openingsToProto(r.Openings)}
	})
}

func propPresentationsToProto(ps []sdk.PropPresentation) []*sessionpb.PropPresentation {
	return mapSlice(ps, func(p sdk.PropPresentation) *sessionpb.PropPresentation {
		out := &sessionpb.PropPresentation{
			Id: p.ID, Ref: p.Ref, Origin: footprintPointToProto(p.Origin), Elevation: p.Elevation,
			FacingDegrees: p.FacingDegrees, HeightScale: p.HeightScale, DoorId: p.DoorID, Label: p.Label,
		}
		if l := p.PointLight; l != nil {
			out.PointLight = &sessionpb.PropPointLight{
				Enabled: l.Enabled, Offset: footprintPointToProto(l.Offset), OffsetElevation: l.OffsetElevation,
				Color: l.Color, Intensity: l.Intensity, Range: l.Range,
			}
		}
		return out
	})
}

func revealedDoorsToProto(ds []sdk.RevealedDoor) []*sessionpb.RevealedDoor {
	return mapSlice(ds, func(d sdk.RevealedDoor) *sessionpb.RevealedDoor {
		return &sessionpb.RevealedDoor{
			Door: d.Door, State: d.State, Doorways: doorwaysToProto(d.Doorways),
			Approaches: mapSlice(d.Approaches, func(a sdk.DoorApproach) *sessionpb.DoorApproach {
				return &sessionpb.DoorApproach{Ability: a.Ability, Tool: a.Tool, Dc: int32(a.DC)} //nolint:gosec // an authored DC fits int32
			}),
		}
	})
}

// mapSlice converts each element of in, nil for an empty slice so an empty
// list and an absent one read the same on the wire.
func mapSlice[T, U any](in []T, f func(T) U) []U {
	if len(in) == 0 {
		return nil
	}
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}
