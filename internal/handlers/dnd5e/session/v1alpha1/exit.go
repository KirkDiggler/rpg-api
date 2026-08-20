package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Exit removes a member from a session's encounter.
func (h *Handler) Exit(ctx context.Context, req *sessionpb.ExitRequest) (*sessionpb.ExitResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.Exit(ctx, &sdk.ExitInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	carry := make([]*sessionpb.Sighting, len(out.Carry))
	for i, s := range out.Carry {
		carry[i] = sightingToProto(s)
	}

	return &sessionpb.ExitResponse{
		Outcome:  memberOutcomeToProto(out.Outcome),
		Carry:    carry,
		Seq:      out.Seq,
		Closed:   outcomeToProto(out.Closed),
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}
