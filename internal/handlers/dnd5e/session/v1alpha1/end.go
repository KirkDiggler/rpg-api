package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// End closes a session's encounter through a declared external ending.
func (h *Handler) End(ctx context.Context, req *sessionpb.EndRequest) (*sessionpb.EndResponse, error) {
	if _, err := authenticatedPlayerID(ctx); err != nil {
		return nil, err
	}

	out, err := h.manager.End(ctx, &sdk.EndInput{
		Session: req.GetSession(),
		Ending:  req.GetEnding(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.EndResponse{
		Outcome:  outcomeToProto(&out.Outcome),
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}
