package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Traverse moves a member through a named connection into the adjoining
// room. Transitional (design §0): the RPC exists while the SDK still
// exposes the verb, and retires when the SDK's own absolute-projection
// convergence retires it.
func (h *Handler) Traverse(ctx context.Context, req *sessionpb.TraverseRequest) (*sessionpb.TraverseResponse, error) {
	if _, err := authenticatedPlayerID(ctx); err != nil {
		return nil, err
	}

	out, err := h.manager.Traverse(ctx, &sdk.TraverseInput{
		Session:    req.GetSession(),
		Member:     req.GetMember(),
		Connection: req.GetConnection(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.TraverseResponse{
		FromRoom:   out.FromRoom,
		From:       positionToProto(out.From),
		ToRoom:     out.ToRoom,
		To:         positionToProto(out.To),
		Discovered: discoveriesToProto(out.Discovered),
		Seq:        out.Seq,
		Outcome:    outcomeToProto(out.Outcome),
		Formed:     formedToProto(out.Formed),
		Saved:      saveReportToProto(out.Saved),
		Delivery:   deliveryReportToProto(out.Delivery),
	}, nil
}
