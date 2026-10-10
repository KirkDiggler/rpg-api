package sessionv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// React answers an open reaction window: take the reaction, or let the mover
// pass (rpg-project#316 rung 3).
//
// THE ONE VERB OFFERED OFF THE CALLER'S OWN TURN. A monster's step stopped
// part-way through its turn to ask this member whether they swing, and the
// fight is frozen on the answer -- every other verb is refused with
// ErrWindowOpen until it arrives. The gate is unchanged all the same: the
// window is answered by its audience and by nobody else, so callerActingAs
// runs first here exactly as it does on Move and Activate.
//
// The selector says WHICH window. A member can hold more than one at a time
// (ruling R3: one step asks every player reactor at once), so the id is the
// only thing that distinguishes them and this handler chooses nothing.
//
// The response is an acknowledgement, on Activate's template: no refreshed
// declaration list -- a second declaration surface beside Afford would be two
// reads free to disagree -- and no account of the resumed turn, which reaches
// every client as beats on the stream. A refusal is a status code, and
// statusError owns that translation: ErrNoWindow and ErrNotOffered are
// FAILED_PRECONDITION, ErrNotAudience is PERMISSION_DENIED.
func (h *Handler) React(ctx context.Context, req *sessionpb.ReactRequest) (*sessionpb.ReactResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	answer, err := reactAnswerFromProto(req.GetChoice(), req.GetOption())
	if err != nil {
		return nil, err
	}

	out, err := h.manager.React(ctx, &sdk.ReactInput{
		Session:       req.GetSession(),
		Member:        req.GetMember(),
		DeclarationID: req.GetDeclarationId(),
		Answer:        answer,
	})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}

	return &sessionpb.ReactResponse{
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}

// reactAnswerFromProto builds the SDK's Answer from the wire's choice and
// option: STRIKE takes the offer (the option may be empty for an option-less
// offer), HOLD declines.
//
// HOLD WITH AN OPTION IS INVALID_ARGUMENT. The wire doc says declining leaves
// the option empty, and an option on a decline names a choice nobody can
// refuse, so the request is malformed rather than the window stale.
//
// REACT_CHOICE_UNSPECIFIED IS INVALID_ARGUMENT, not a quiet default, and the
// wire's own ReactRequest doc states it: "the client forgot to say" and "the
// player chose to hold" must not be the same bytes, because one of them
// declines a swing on the player's behalf. A future value this build does not
// recognize lands here too.
func reactAnswerFromProto(c sessionpb.ReactChoice, option string) (sdk.Answer, error) {
	switch c {
	case sessionpb.ReactChoice_REACT_CHOICE_STRIKE:
		return sdk.Take(option), nil
	case sessionpb.ReactChoice_REACT_CHOICE_HOLD:
		if option != "" {
			return sdk.Answer{}, status.Error(codes.InvalidArgument, "react: declining names no option")
		}
		return sdk.Decline(), nil
	default:
		return sdk.Answer{}, status.Errorf(codes.InvalidArgument, "react: unrecognized choice %v", c)
	}
}
