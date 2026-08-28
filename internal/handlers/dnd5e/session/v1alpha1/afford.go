package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Afford reports what one member can still declare this turn: can-or-cannot
// per verb, the same currency-naming text a refused Attack would use. Asked
// of a member, never of the session, for the same reason Turn is: the SDK
// models several clocks running at once, and on the world clock the question
// does not apply -- Declarations comes back empty, which IS the answer
// (ADR-0042).
func (h *Handler) Afford(ctx context.Context, req *sessionpb.AffordRequest) (*sessionpb.AffordResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.Afford(ctx, &sdk.AffordInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.AffordResponse{
		Clock:        clockKindToProto(out.Clock),
		Declarations: declarationsToProto(out.Declarations),
	}, nil
}
