package sessionv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Intimidate threatens one creature that can perceive `member`
// (rpg-project#454, the first of the shenanigans). Unlock's shape with a
// mind where the door is: the SDK loads the sheet, rolls the member's best
// listed approach against the target's authored or derived DC, and tells the
// composition the verdict and nothing else.
//
// THE RESPONSE CARRIES NO beaten, total OR dc, and that is the whole reason
// this handler does not look like Unlock's. The outcome reaches the actor the
// way it reaches everyone else -- as the `intimidated` beat on the stream,
// whose audience is every member whose sight reaches the actor's cell, THE
// ACTOR INCLUDED. A response repeating the numbers would be a second,
// differently shaped account of one throw, and two accounts of one roll is
// how a client learns to disagree with the table about what was rolled
// (ruled by Kirk on rpg-api-protos#339; UnlockResponse's beaten/total/dc are
// the older shape, not a precedent). So IntimidateOutput.Beaten, .Total and
// .DC are deliberately read and dropped here.
//
// WHAT IS LEFT IS THE CALLER'S ALONE. Paused and Roll are the offer window --
// the one moment of this verb nobody else at the table is entitled to, asked
// before any beat exists -- and Saved/Delivery are S6's partial-write report,
// which every mutating verb at this seam carries. While Paused is true the
// verdict does not exist yet, so nothing else could be said anyway.
//
// NOTHING ABOUT THE CONSEQUENCE, either here or on the beat. A beaten threat
// lands an intimidate deed on the witnesses and stops; what the deed is worth
// is the target's own mind's to decide, and it reaches clients as that
// creature's next turn. A "fleeing" field here would make the outcome this
// verb's instead of the mind's, which is the first cut the design broke and
// recorded so it is not proposed again.
func (h *Handler) Intimidate(
	ctx context.Context, req *sessionpb.IntimidateRequest,
) (*sessionpb.IntimidateResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.Intimidate(ctx, &sdk.IntimidateInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
		Target:  req.GetTarget(),
	})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}

	return &sessionpb.IntimidateResponse{
		Paused:   out.Paused,
		Roll:     intimidateRollToProto(out.Roll),
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}

// intimidateRollToProto mirrors IntimidateOutput.Roll's own presence law --
// a *int meaningful only while Paused -- onto the wire's optional int32. Nil
// stays nil rather than crossing as a meaningless zero, exactly as
// unlockRollToProto's does for the same field on the same machine.
//
// A SECOND FUNCTION RATHER THAN A SHARED ONE, for now. The two are
// byte-identical and each is named for the verb whose output law it keeps;
// collapsing them would put one doc comment in front of two presence rules
// that are only accidentally the same, and the day one verb's roll stops
// being pointer-optional the shared helper is the thing that hides it.
func intimidateRollToProto(roll *int) *int32 {
	if roll == nil {
		return nil
	}
	converted := int32(*roll)
	return &converted
}
