package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Attack swings one member's weapon at another. v1 compiles character
// attackers only (design's own note); a monster attacker comes back as
// FAILED_PRECONDITION via the SDK's ErrNotACharacter.
//
// A SWING CAN STOP PART-WAY THROUGH (rpg-project#398). When the attacker
// holds something spendable on their own d20 -- a Bardic Inspiration die
// today -- the machine poses a window after the roll and before the outcome
// is read, and out.Paused is true. Roll and Total are then the only two
// numbers that are answers; Against, Hit, Critical and Damage are all zero
// because nothing has landed and the AC has deliberately not been shown.
//
// THE PAUSE ITSELF HAS NO WIRE FIELD ON THIS RESPONSE. AttackResponse
// carries no `paused` bool -- see rpg-api-protos service.proto -- so this
// handler does not invent one, and a client that only read this message
// could not tell a pause from a miss that dealt nothing. Two things say it
// instead, and both are shipped: the ROLL_WINDOW_OPENED beat reaches the
// attacker's own stream with the roll, the total and the offer's label, and
// their next Afford carries the REACT row while every other member sees
// ShortfallWindowOpen. Adding the field is a proto change, not something
// this converter can paper over.
func (h *Handler) Attack(ctx context.Context, req *sessionpb.AttackRequest) (*sessionpb.AttackResponse, error) {
	if err := h.callerActingAs(ctx, req.GetAttacker()); err != nil {
		return nil, err
	}

	out, err := h.manager.Attack(ctx, &sdk.AttackInput{
		Session:       req.GetSession(),
		Attacker:      req.GetAttacker(),
		Target:        req.GetTarget(),
		DeclarationID: req.GetDeclarationId(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.AttackResponse{
		Roll:     int32(out.Roll),
		Total:    int32(out.Total),
		Against:  int32(out.Against),
		Hit:      out.Hit,
		Critical: out.Critical,
		Damage:   int32(out.Damage),
		Seq:      out.Seq,
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
		Attack:   attackRefToProto(out.Attack),
		// The opaque token this swing was minted with. The same value reaches
		// every other member on the Struck/Missed beat, and it is the only
		// thing the attacker and a witness can both name this roll by: seq is
		// per recipient, so no number here means anything in another's stream.
		PresentationId: out.PresentationID,
	}, nil
}
