package sessionv1alpha1

import (
	"context"
	"errors"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

var errViewOutputRequired = errors.New("view returned no output")

// GetView returns one coherent observer view, including current/remembered
// mutable props and doors. It does not read live world placements separately.
func (h *Handler) GetView(ctx context.Context, req *sessionpb.GetViewRequest) (*sessionpb.GetViewResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}
	view, err := h.manager.View(ctx, &sdk.ViewInput{Session: req.GetSession(), Member: req.GetMember()})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}
	if view == nil {
		return nil, sdkerr.StatusError(errViewOutputRequired)
	}
	return viewToProto(view), nil
}

func viewToProto(in *sdk.ViewOutput) *sessionpb.GetViewResponse {
	out := &sessionpb.GetViewResponse{Sightings: sightingsToProto(in.Sightings), Areas: sightAreasToProto(in.Areas)}
	for _, s := range in.Props {
		p := &sessionpb.PropSighting{ObservedEmpty: s.ObservedEmpty, CurrentVia: append([]string(nil), s.CurrentVia...), Status: s.Status, At: s.At}
		if s.Prop != nil {
			shape := atlasPropToProto(*s.Prop)
			if s.ObservedEmpty {
				shape.At = nil
			}
			p.Shape = &sessionpb.PropSighting_Prop{Prop: shape}
		}
		if s.Placed != nil {
			shape := atlasPlacedPropToProto(*s.Placed)
			if s.ObservedEmpty {
				shape.Placement = nil
				shape.Cells = nil
			}
			p.Shape = &sessionpb.PropSighting_Placed{Placed: shape}
		}
		out.Props = append(out.Props, p)
	}
	for _, s := range in.Doors {
		out.Doors = append(out.Doors, &sessionpb.DoorSighting{
			Door: doorToProto(s.Door), CurrentVia: append([]string(nil), s.CurrentVia...), Status: s.Status, At: s.At,
		})
	}
	return out
}
