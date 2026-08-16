package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// GetView asks what one member currently perceives. A cold client cannot yet
// learn its OWN position from this -- View skips self (design rule 11,
// toolkit#933, not a gate on this package); until that lands, reconnect
// leans on GetStory replay.
func (h *Handler) GetView(ctx context.Context, req *sessionpb.GetViewRequest) (*sessionpb.GetViewResponse, error) {
	if _, err := authenticatedPlayerID(ctx); err != nil {
		return nil, err
	}

	sightings, err := h.manager.View(ctx, &sdk.ViewInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.GetViewResponse{Sightings: sightingsToProto(sightings)}, nil
}
