package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Activate uses a combat ability or feature the member already carries --
// Dodge, Dash, Disengage, Help, Hide, Rage, Second Wind.
//
// The selector says WHICH one: Afford compiles one offer per thing the member
// carries, so there is no ability ref on the request and this handler does not
// choose. A caller naming the ability itself would be deciding something
// Afford already decided.
//
// The response is an acknowledgement (rpg-project#300). It carries no
// refreshed declaration list -- a second declaration surface beside Afford
// would be two reads answering "what can I do", free to disagree -- and no
// success flag, because a refusal is a status code. statusError owns that
// translation: ErrCannotActivate and ErrStaleDeclaration are
// FAILED_PRECONDITION, ErrBadActivation is INVALID_ARGUMENT.
func (h *Handler) Activate(
	ctx context.Context, req *sessionpb.ActivateRequest,
) (*sessionpb.ActivateResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.Activate(ctx, &sdk.ActivateInput{
		Session:       req.GetSession(),
		Member:        req.GetMember(),
		DeclarationID: req.GetDeclarationId(),
		Target:        req.GetTarget(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.ActivateResponse{
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}
