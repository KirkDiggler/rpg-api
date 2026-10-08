package sessionv1alpha1

import (
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// Permission and pose are the provider's answers. These mappings neither look
// up content nor derive visuals from collider dimensions or current world state.
func propPresentationToProto(p sdk.PropPresentation) *sessionpb.PropPresentation {
	out := &sessionpb.PropPresentation{
		Id: p.ID, Ref: p.Ref, Origin: &sessionpb.FootprintPoint{X: p.Origin.X, Y: p.Origin.Y},
		Elevation: p.Elevation, FacingDegrees: p.FacingDegrees, HeightScale: p.HeightScale,
		DoorId: p.DoorID, Label: p.Label,
	}
	if l := p.PointLight; l != nil {
		out.PointLight = &sessionpb.PropPointLight{
			Enabled: l.Enabled, Offset: &sessionpb.FootprintPoint{X: l.Offset.X, Y: l.Offset.Y},
			OffsetElevation: l.OffsetElevation, Color: l.Color, Intensity: l.Intensity, Range: l.Range,
		}
	}
	return out
}

func propPresentationsToProto(in []sdk.PropPresentation) []*sessionpb.PropPresentation {
	out := make([]*sessionpb.PropPresentation, 0, len(in))
	for _, p := range in {
		out = append(out, propPresentationToProto(p))
	}
	return out
}
