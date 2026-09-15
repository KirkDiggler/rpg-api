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
// A SWING CAN STOP PART-WAY THROUGH (rpg-project#398, rpg-api#985). When the
// attacker holds something spendable on their own d20 -- a Bardic
// Inspiration die today -- the machine poses a window after the roll and
// before the outcome is read, and out.Paused is true. Roll and Total are
// then the only two numbers that are answers; Against, Hit, Critical and
// Damage all cross the wire as their zero value, because the toolkit's own
// AttackOutput leaves them unset in this case -- nothing here has to
// special-case them, mirroring the field is enough. Answering the window
// with the existing, already-generic React verb finishes the attack and
// writes the beat this response did not.
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
		Roll:        int32(out.Roll),
		Total:       int32(out.Total),
		Against:     int32(out.Against),
		Hit:         out.Hit,
		Critical:    out.Critical,
		Damage:      int32(out.Damage),
		Paused:      out.Paused,
		Seq:         out.Seq,
		Saved:       saveReportToProto(out.Saved),
		Delivery:    deliveryReportToProto(out.Delivery),
		Attack:      attackRefToProto(out.Attack),
		Calculation: rollCalculationToProto(out.Calculation),
		// The opaque token this swing was minted with. The same value reaches
		// every other member on the Struck/Missed beat, and it is the only
		// thing the attacker and a witness can both name this roll by: seq is
		// per recipient, so no number here means anything in another's stream.
		PresentationId: out.PresentationID,
	}, nil
}
