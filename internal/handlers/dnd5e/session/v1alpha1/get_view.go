package sessionv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// GetView asks what one member currently perceives. A cold client cannot yet
// learn its OWN position from this -- View skips self (design rule 11,
// toolkit#933, not a gate on this package); until that lands, reconnect
// leans on GetStory replay.
func (h *Handler) GetView(ctx context.Context, req *sessionpb.GetViewRequest) (*sessionpb.GetViewResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	viewInput := &sdk.ViewInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
	}
	sightings, err := h.manager.View(ctx, viewInput)
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}
	areas, err := h.manager.Areas(ctx, viewInput)
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}

	return &sessionpb.GetViewResponse{
		Sightings: sightingsToProto(sightings),
		Areas:     sightAreasToProto(areas),
	}, nil
}
