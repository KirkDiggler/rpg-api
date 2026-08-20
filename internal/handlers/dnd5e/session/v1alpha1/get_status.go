package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// GetStatus reports whether a session's encounter is open. Encounter-wide and
// never per-member (design's own pin, mirrored in sdk.Status): a convenience
// field answering "the" active actor would create a privileged clock by
// implication -- ask Turn instead.
func (h *Handler) GetStatus(ctx context.Context, req *sessionpb.GetStatusRequest) (*sessionpb.GetStatusResponse, error) {
	if _, err := authenticatedPlayerID(ctx); err != nil {
		return nil, err
	}

	out, err := h.manager.Status(ctx, &sdk.StatusInput{Session: req.GetSession()})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.GetStatusResponse{
		Open:    out.Open,
		Outcome: outcomeToProto(out.Outcome),
	}, nil
}
