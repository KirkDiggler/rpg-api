package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Hold picks up one holdable prop, named by its placement id
// (rpg-project#368, design §4.3 and R10). Loot's shape, one file over.
//
// HOLD, NEVER TAKE. A holding is run-scoped state about a pair of hands and
// writes nothing to a character sheet; Take is reserved for the act that
// lands a thing in inventory. No verb, no beat and no field in this package
// says "took" for a thing that is only held.
//
// The response is ack-only for Loot's reason. The world change everyone needs
// -- the prop off the map, the holder carrying it -- arrives as the HELD beat
// to everyone present, and GetAtlas omits held props for everyone, so a
// client that patches its cached map from the beat lands where a refetch
// would have put it.
//
// # The probe law survives this translation, structurally
//
// For a prop standing in space the member is not shown, the composition
// answers EVERY refusal -- no such prop, not holdable, already held, out of
// range -- with a bare sdk.ErrNoProp, so all four reach statusError as one
// sentinel and leave it as one code carrying one sentence. Nothing here
// re-derives a reason from the request, which is what would break it: this
// handler cannot tell the four cases apart either, and that is the point.
// See errors.go's NOT_FOUND bucket, and hold_test.go, which pins the four
// byte-for-byte.
func (h *Handler) Hold(ctx context.Context, req *sessionpb.HoldRequest) (*sessionpb.HoldResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.Hold(ctx, &sdk.HoldInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
		Target:  req.GetTarget(),
		Range:   int(req.GetRange()),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.HoldResponse{
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}
