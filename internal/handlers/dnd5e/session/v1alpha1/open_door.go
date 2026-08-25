package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// OpenDoor pushes a shut door open as `member` (rpg-project#268). A locked
// door refuses with FAILED_PRECONDITION naming the DC — the refusal is a
// fiction beat, not a defect (rpg-toolkit#1135). What the opened door
// revealed arrives where reveals always arrive: on the stream.
func (h *Handler) OpenDoor(ctx context.Context, req *sessionpb.OpenDoorRequest) (*sessionpb.OpenDoorResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.OpenDoor(ctx, &sdk.OpenDoorInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
		Door:    req.GetDoor(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.OpenDoorResponse{Door: doorToProto(out.Door)}, nil
}
