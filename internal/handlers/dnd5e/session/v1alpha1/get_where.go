package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// GetWhere reports the cell one member stands on -- the caller's own
// placement. Deliberately singular, no roster read: a batched positions-of-
// everyone response would hand a client cells it has never perceived
// (design rule 4), which is the one thing perception exists to prevent.
func (h *Handler) GetWhere(ctx context.Context, req *sessionpb.GetWhereRequest) (*sessionpb.GetWhereResponse, error) {
	if _, err := authenticatedPlayerID(ctx); err != nil {
		return nil, err
	}

	out, err := h.manager.Where(ctx, &sdk.WhereInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return whereToProto(out), nil
}
