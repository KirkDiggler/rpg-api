package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Unpack decomposes a pack the actor already owns into its own contents
// (rpg-toolkit#1544). Hold's shape, one file over: no counterparty, no
// reach, no story beat -- the target is something the actor already holds,
// not something to reach for, so the response is ack-only (Saved/Delivery),
// no descriptor and no Seq. Pure proto <-> SDK translation, no rule lives
// here (design rule 8): which item names a pack, and how its contents
// resolve, are the SDK's own rules to enforce (ErrNotAPack,
// ErrBadPackContents, ErrNotInInventory), not this handler's.
func (h *Handler) Unpack(ctx context.Context, req *sessionpb.UnpackRequest) (*sessionpb.UnpackResponse, error) {
	if err := h.callerActingAs(ctx, req.GetActor()); err != nil {
		return nil, err
	}

	out, err := h.manager.Unpack(ctx, &sdk.UnpackInput{
		Session:  req.GetSession(),
		Actor:    req.GetActor(),
		ItemID:   req.GetItemId(),
		Quantity: int(req.GetQuantity()),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.UnpackResponse{
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}
