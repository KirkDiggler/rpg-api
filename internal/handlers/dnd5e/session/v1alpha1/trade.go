package sessionv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Trade buys from or sells to a placed vendor (rpg-project#369/#370 buy
// wave, rpg-toolkit#1537 sell wave): Interact already answers what a
// vendor carries, this verb is what actually moves an item and its
// payment — one direction per call, dispatched by which side of the offer
// carries items (Receive populated is a buy, Give populated is a sell).
// Pure proto <-> SDK translation, no rule lives here (design rule 8): this
// handler forwards Give/Receive exactly as given in both directions: which
// shapes are legal, and the exact price required, are the SDK's own rules
// to enforce (ErrInvalidTradeOffer, ErrWrongPrice, ErrOutOfStock,
// ErrNotInInventory, ErrInsufficientFunds), not this handler's.
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

	descriptor, err := worldNPCDescriptorToProto(out.Descriptor)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &sessionpb.TradeResponse{
		Descriptor_: descriptor,
		Seq:         out.Seq,
		Saved:       saveReportToProto(out.Saved),
		Delivery:    deliveryReportToProto(out.Delivery),
	}, nil
}
