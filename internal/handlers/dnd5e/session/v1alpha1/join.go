package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Join brings a player's character into a session's encounter.
func (h *Handler) Join(ctx context.Context, req *sessionpb.JoinRequest) (*sessionpb.JoinResponse, error) {
	if _, err := authenticatedPlayerID(ctx); err != nil {
		return nil, err
	}

	out, err := h.manager.Join(ctx, &sdk.JoinInput{
		Session:  req.GetSession(),
		Member:   req.GetMember(),
		Room:     req.GetRoom(),
		Position: positionFromProto(req.GetPosition()),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.JoinResponse{
		Member:     memberToProto(out.Member),
		Character:  characterStateToProto(out.Character),
		Discovered: discoveriesToProto(out.Discovered),
		Seq:        out.Seq,
		Outcome:    outcomeToProto(out.Outcome),
		Formed:     formedToProto(out.Formed),
		Saved:      saveReportToProto(out.Saved),
		Delivery:   deliveryReportToProto(out.Delivery),
	}, nil
}
