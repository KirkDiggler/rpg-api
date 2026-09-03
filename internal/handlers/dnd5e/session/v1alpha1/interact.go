package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Interact reaches for a placed world NPC (rpg-toolkit#1404) and reports
// what it is — identity, adjacency, and visibility are the SDK's own to
// confirm; this handler is pure proto <-> SDK translation, no rule lives
// here (design rule 8).
func (h *Handler) Interact(ctx context.Context, req *sessionpb.InteractRequest) (*sessionpb.InteractResponse, error) {
	if err := h.callerActingAs(ctx, req.GetActor()); err != nil {
		return nil, err
	}

	out, err := h.manager.Interact(ctx, &sdk.InteractInput{
		Session: req.GetSession(),
		Actor:   req.GetActor(),
		Target:  req.GetTarget(),
		Range:   int(req.GetRange()),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.InteractResponse{
		Descriptor_: worldNPCDescriptorToProto(out.Descriptor),
		Seq:         out.Seq,
		Saved:       saveReportToProto(out.Saved),
		Delivery:    deliveryReportToProto(out.Delivery),
	}, nil
}
