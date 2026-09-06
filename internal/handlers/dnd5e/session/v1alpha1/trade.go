package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Trade buys one item from a placed vendor (rpg-project#369/#370, wave 1 of
// rpg-toolkit#1275): Interact already answers what a vendor carries, this
// verb is what actually moves an item — decrementing the vendor's stock and
// adding it to the actor's inventory. Pure proto <-> SDK translation, no
// rule lives here (design rule 8): what shape an offer must have, and what
// it costs, are the SDK's own rules to enforce (ErrInvalidTradeOffer and the
// refusals beside it), never this handler's.
func (h *Handler) Trade(ctx context.Context, req *sessionpb.TradeRequest) (*sessionpb.TradeResponse, error) {
	if err := h.callerActingAs(ctx, req.GetActor()); err != nil {
		return nil, err
	}

	out, err := h.manager.Trade(ctx, &sdk.TradeInput{
		Session: req.GetSession(),
		Actor:   req.GetActor(),
		Target:  req.GetTarget(),
		Range:   int(req.GetRange()),
		Give:    tradeOfferFromProto(req.GetGive()),
		Receive: tradeOfferFromProto(req.GetReceive()),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.TradeResponse{
		Descriptor_: worldNPCDescriptorToProto(out.Descriptor),
		Seq:         out.Seq,
		Saved:       saveReportToProto(out.Saved),
		Delivery:    deliveryReportToProto(out.Delivery),
	}, nil
}
