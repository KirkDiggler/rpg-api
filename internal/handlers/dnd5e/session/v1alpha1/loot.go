package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Loot moves what a downed body holds to the looter (rpg-project#368, design
// §4.2). Search's shape one verb over, and for search's reason: everything
// that keeps a secret secret lives at the rule half, and this file adds only
// what every verb at this seam adds.
//
// The response is ACK-ONLY -- the seam's standard saved and delivery reports
// and NOTHING about what moved. Loot is offered on every downed member
// (design P3), so a body carrying the run's only secret and a body carrying
// nothing have to answer with the same bytes: a found list, a count or a flag
// here would tell a party which corpse was worth looting, which is the one
// thing looting the wrong body must not teach. What the looter gained reaches
// them alone, on their own stream, as the DOOR_REVEALED beat a successful
// search already produces (design P4); everyone present hears LOOTED, which
// names looter and body and nothing else.
func (h *Handler) Loot(ctx context.Context, req *sessionpb.LootRequest) (*sessionpb.LootResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.Loot(ctx, &sdk.LootInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
		Target:  req.GetTarget(),
		Range:   int(req.GetRange()),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.LootResponse{
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}
