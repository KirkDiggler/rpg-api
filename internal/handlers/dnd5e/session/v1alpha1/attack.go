package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Attack swings one member's weapon at another. v1 compiles character
// attackers only (design's own note); a monster attacker comes back as
// FAILED_PRECONDITION via the SDK's ErrNotACharacter.
func (h *Handler) Attack(ctx context.Context, req *sessionpb.AttackRequest) (*sessionpb.AttackResponse, error) {
	if err := h.callerActingAs(ctx, req.GetAttacker()); err != nil {
		return nil, err
	}

	out, err := h.manager.Attack(ctx, &sdk.AttackInput{
		Session:  req.GetSession(),
		Attacker: req.GetAttacker(),
		Target:   req.GetTarget(),
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
	}, nil
}
