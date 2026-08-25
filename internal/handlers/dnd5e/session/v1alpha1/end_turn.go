package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// EndTurn ends a member's turn and hands the fight to whoever is next.
func (h *Handler) EndTurn(ctx context.Context, req *sessionpb.EndTurnRequest) (*sessionpb.EndTurnResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.EndTurn(ctx, &sdk.EndTurnInput{
		Session:       req.GetSession(),
		Member:        req.GetMember(),
		DeclarationID: req.GetDeclarationId(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.EndTurnResponse{
		Next:         out.Next,
		RoundWrapped: out.RoundWrapped,
		Seq:          out.Seq,
		Saved:        saveReportToProto(out.Saved),
		Delivery:     deliveryReportToProto(out.Delivery),
	}, nil
}
