package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Turn asks what one member is waiting on. Asked of a member, never of the
// session (the SDK models several clocks running at once, so "whose turn is
// it?" has no session-wide answer -- see sdk.Manager.Turn).
func (h *Handler) Turn(ctx context.Context, req *sessionpb.TurnRequest) (*sessionpb.TurnResponse, error) {
	if _, err := authenticatedPlayerID(ctx); err != nil {
		return nil, err
	}

	out, err := h.manager.Turn(ctx, &sdk.TurnInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.TurnResponse{
		Clock:  clockKindToProto(out.Clock),
		Active: out.Active,
		Round:  int32(out.Round),
		Order:  out.Order,
	}, nil
}
