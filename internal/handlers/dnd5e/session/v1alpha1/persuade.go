package sessionv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Persuade talks one creature round (rpg-project#458, the front room goblin).
// Intimidate's twin, and DELIBERATELY ITS TWIN ALL THE WAY DOWN: the same
// three fields in, the same four fields out, the same refusals, the same
// silence about the verdict. A client that learned one shape must not have to
// learn a second.
//
// THE RESPONSE CARRIES NO beaten, total OR dc, for Intimidate's reason and
// under the same ruling (Kirk, rpg-api-protos#339, carried onto this verb by
// rpg-api-protos#340). The outcome reaches the actor as the `persuaded` beat
// on the stream, whose audience is every member whose sight reaches the
// actor's cell, THE ACTOR INCLUDED. A response repeating the numbers would be
// a second, differently shaped account of one throw. So PersuadeOutput.Beaten,
// .Total and .DC are read and dropped here exactly as Intimidate's are.
//
// WHAT IS LEFT IS THE CALLER'S ALONE: Paused and Roll are the offer window,
// asked before any beat exists, and Saved/Delivery are S6's partial-write
// report that every mutating verb at this seam carries.
//
// NOT EVEN THE ANSWER. A settled appeal may have made the goblin speak, teach
// the party a fact or bolt for the door -- the author's table, rolled by the
// world -- and none of that is here. It arrives as the `answered` beat, which
// is the whole point of a second beat: the creature's answer is the world's
// account, not this caller's receipt.
//
// A FAILED APPEAL IS AN OUTCOME, NOT AN ERROR, and this handler returns nil
// for it just as Intimidate does. The `persuade_failed` table still fires,
// which is where the goblin's bad directions come from.
func (h *Handler) Persuade(
	ctx context.Context, req *sessionpb.PersuadeRequest,
) (*sessionpb.PersuadeResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.Persuade(ctx, &sdk.PersuadeInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
		Target:  req.GetTarget(),
	})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}

	return &sessionpb.PersuadeResponse{
		Paused:   out.Paused,
		Roll:     persuadeRollToProto(out.Roll),
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}

// persuadeRollToProto mirrors PersuadeOutput.Roll's own presence law -- a
// *int meaningful only while Paused -- onto the wire's optional int32. Nil
// stays nil rather than crossing as a meaningless zero.
//
// A THIRD FUNCTION RATHER THAN A SHARED ONE, on intimidateRollToProto's own
// stated reasoning rather than by copying it thoughtlessly: the three are
// byte-identical today and each is named for the verb whose output law it
// keeps. The day one verb's roll stops being pointer-optional -- a verb that
// always poses, say, or one that poses twice -- a shared helper is the thing
// that would hide it behind a signature nobody re-read.
func persuadeRollToProto(roll *int) *int32 {
	if roll == nil {
		return nil
	}
	converted := int32(*roll)
	return &converted
}
