package sessionv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Unlock tries a locked door as `member` (rpg-project#268). The check rolls
// SERVER-SIDE — the SDK loads the sheet, rolls against the authored DC, and
// tells the composition only the verdict. The attempt is public down to the
// number (full data until v1.0): the response carries total and DC, and the
// beat every member hears carries the same facts. A failed attempt is an
// outcome, not an error — the door is unchanged and retryable.
//
// THE ATTEMPT CAN STOP PART-WAY THROUGH (rpg-project#398, rpg-api#985), the
// same shape Attack's own pause carries: when the member holds something
// spendable on their own check -- Guidance today -- the machine poses a
// window after the roll and before the verdict is read, and out.Paused is
// true. Total and Roll are then the only answers; Beaten, DC and Door all
// cross the wire as their zero value, because the toolkit's own
// UnlockOutput leaves them unset in this case -- nothing here has to
// special-case them. Answering the window with the existing, already-generic
// React verb finishes the attempt and writes the beat this response did not.
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
		return nil, sdkerr.StatusError(err)
	}

	return &sessionpb.UnlockResponse{
		Beaten: out.Beaten,
		Total:  int32(out.Total),
		Dc:     int32(out.DC),
		Door:   doorToProto(out.Door),
		Paused: out.Paused,
		Roll:   unlockRollToProto(out.Roll),
	}, nil
}

// unlockRollToProto mirrors UnlockOutput.Roll's own presence law: a *int,
// meaningful only while Paused, onto the wire's optional int32 -- nil stays
// nil rather than crossing as a meaningless zero.
func unlockRollToProto(roll *int) *int32 {
	if roll == nil {
		return nil
	}
	converted := int32(*roll)
	return &converted
}
