package sessionv1alpha1

import (
	"context"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// CloseDoor forwards the owned member's intent to the SDK. State, reach,
// concealment probing and persistence are provider answers, not host checks.
func (h *Handler) CloseDoor(ctx context.Context, req *sessionpb.CloseDoorRequest) (*sessionpb.CloseDoorResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}
	out, err := h.manager.CloseDoor(ctx, &sdk.CloseDoorInput{
		Session: req.GetSession(), Member: req.GetMember(), Door: req.GetDoor(),
	})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}
	return &sessionpb.CloseDoorResponse{Door: doorToProto(out.Door)}, nil
}
