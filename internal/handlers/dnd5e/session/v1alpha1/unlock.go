package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Unlock tries a locked door as `member` (rpg-project#268). The check rolls
// SERVER-SIDE — the SDK loads the sheet, rolls against the authored DC, and
// tells the composition only the verdict. The attempt is public down to the
// number (full data until v1.0): the response carries total and DC, and the
// beat every member hears carries the same facts. A failed attempt is an
// outcome, not an error — the door is unchanged and retryable.
func (h *Handler) Unlock(ctx context.Context, req *sessionpb.UnlockRequest) (*sessionpb.UnlockResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.Unlock(ctx, &sdk.UnlockInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
		Door:    req.GetDoor(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.UnlockResponse{
		Beaten: out.Beaten,
		Total:  int32(out.Total),
		Dc:     int32(out.DC),
		Door:   doorToProto(out.Door),
	}, nil
}
