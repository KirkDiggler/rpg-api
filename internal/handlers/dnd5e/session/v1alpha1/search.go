package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Search sweeps the region `member` stands in for concealed structure
// (rpg-project#350 slice 1). The response is deliberately ACK-ONLY --
// SearchOutput carries no outcome, because an outcome answers "was there
// anything to find here", which is the secret a search keeps. A success
// reaches the searcher, and only the searcher, on the stream: a DOOR_REVEALED
// or REGION_REVEALED beat addressed to them alone (see convert.go's
// setEventBody). A world with nothing hidden and a failed roll both resolve
// through this same silence -- the answer never leaks the question.
func (h *Handler) Search(ctx context.Context, req *sessionpb.SearchRequest) (*sessionpb.SearchResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.Search(ctx, &sdk.SearchInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
		Region:  req.GetRegion(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.SearchResponse{
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}
