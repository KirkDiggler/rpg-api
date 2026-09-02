package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// GetDoors reports every door THIS MEMBER KNOWS OF, with live state — the
// dynamic half of GetAtlas's doorways (rpg-project#268). The atlas says
// where a known door's edges are and never changes; this says what each one
// is doing now. A client fetches it once and keeps it fresh from DOOR events
// on the stream.
//
// Member is bound to the authenticated caller, never trusted from the
// request — the same law GetAtlas and GetWhere keep (rpg-api-protos#266): a
// concealed door the member has not had revealed is absent from this list,
// exactly as it is absent from their Atlas, so wiring a client-supplied
// member through unchecked would let one player learn what another has
// found.
func (h *Handler) GetDoors(ctx context.Context, req *sessionpb.GetDoorsRequest) (*sessionpb.GetDoorsResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.Doors(ctx, &sdk.DoorsInput{Session: req.GetSession(), Member: req.GetMember()})
	if err != nil {
		return nil, statusError(err)
	}

	doors := make([]*sessionpb.DoorInfo, 0, len(out.Doors))
	for _, d := range out.Doors {
		doors = append(doors, doorToProto(d))
	}
	return &sessionpb.GetDoorsResponse{Doors: doors}, nil
}
