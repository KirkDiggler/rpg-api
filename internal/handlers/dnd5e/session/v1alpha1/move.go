package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Move walks a member along a path of cells, one at a time.
func (h *Handler) Move(ctx context.Context, req *sessionpb.MoveRequest) (*sessionpb.MoveResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	path := make([]spatial.Position, len(req.GetPath()))
	for i, p := range req.GetPath() {
		path[i] = positionFromProto(p)
	}

	out, err := h.manager.Move(ctx, &sdk.MoveInput{
		Session:       req.GetSession(),
		Member:        req.GetMember(),
		DeclarationID: req.GetDeclarationId(),
		Path:          path,
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.MoveResponse{
		Steps:      stepsToProto(out.Steps),
		Discovered: discoveriesToProto(out.Discovered),
		Outcome:    outcomeToProto(out.Outcome),
		Formed:     formedToProto(out.Formed),
		Saved:      saveReportToProto(out.Saved),
		Delivery:   deliveryReportToProto(out.Delivery),
	}, nil
}
