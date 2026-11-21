package sessionv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Wait lets a seated player outside a fight pass one round on the world clock
// (rpg-project#1079): the act that sits where End Turn sits in a fight.
//
// The api decides nothing here. It binds the member to the authenticated
// caller, hands the member to the session, maps the session's refusal to a
// status, and returns the seam's two reports. Whether the character may wait
// (not in a fight, not downed) and what the round costs are the engine's.
func (h *Handler) Wait(ctx context.Context, req *sessionpb.WaitRequest) (*sessionpb.WaitResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.Wait(ctx, &sdk.WaitInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
	})
	if err != nil {
		return nil, sdkerr.StatusErrorFor(err, req.GetMember())
	}

	return &sessionpb.WaitResponse{
		Saved:    saveReportToProto(ctx, req.GetSession(), out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}
