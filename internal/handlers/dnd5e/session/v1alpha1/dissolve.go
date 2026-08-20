package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Dissolve ends the fight a member is in and returns everyone to free roam.
func (h *Handler) Dissolve(ctx context.Context, req *sessionpb.DissolveRequest) (*sessionpb.DissolveResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	cause, err := dissolveCauseFromProto(req.GetCause())
	if err != nil {
		return nil, statusError(err)
	}

	out, err := h.manager.Dissolve(ctx, &sdk.DissolveInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
		Cause:   cause,
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.DissolveResponse{
		Members:  out.Members,
		Cause:    dissolveKindToProto(out.Cause),
		Seq:      out.Seq,
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}
