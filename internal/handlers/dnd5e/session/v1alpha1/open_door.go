package sessionv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

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
		return nil, sdkerr.StatusErrorFor(err, req.GetMember())
	}
	if out.Refused != nil {
		return nil, doorRefusalError(out.Refused)
	}

	return &sessionpb.OpenDoorResponse{Door: doorToProto(out.Door)}, nil
}

// doorRefusalError is a door verb a post holds against the member
// (rpg-project#842, the guard holds the gate R2 and R7), answered on the wire
// exactly as the held door was before the bump spoke: FAILED_PRECONDITION
// whose message is the session's public reason ("<name> bars the door"). The
// session now answers rather than errs: the door is unchanged, the attempt was
// written as a `barred` act the holder heard, and his line arrives on the act
// stream. The door responses have no field for a refusal, and a refused verb
// must not cross as a success, so the status is the one way a client tells.
func doorRefusalError(r *sdk.DoorRefusal) error {
	return sdkerr.StatusError(&sdk.DoorHeldError{Reason: r.Reason})
}
