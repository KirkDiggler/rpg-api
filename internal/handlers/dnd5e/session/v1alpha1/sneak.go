package sessionv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// SetSneaking declares or clears sneaking on only the authenticated caller's
// exact seated character (rpg-project#606, R6). The mode is encounter state;
// the handler forwards it and never judges a roll.
func (h *Handler) SetSneaking(ctx context.Context, req *sessionpb.SetSneakingRequest) (*sessionpb.SetSneakingResponse, error) {
	gate, err := h.accessGate()
	if err != nil {
		return nil, err
	}
	if seatErr := gate.CallerMemberSeated(ctx, req.GetSession(), req.GetMember()); seatErr != nil {
		return nil, seatErr
	}
	if req.Sneaking == nil {
		return nil, status.Error(codes.InvalidArgument, "sneaking must be explicitly supplied")
	}
	out, err := h.manager.SetSneaking(ctx, &sdk.SetSneakingInput{Session: req.GetSession(), Member: req.GetMember(), Sneaking: req.GetSneaking()})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}
	return &sessionpb.SetSneakingResponse{Sneaking: out.Sneaking, Saved: saveReportToProto(out.Saved), Delivery: deliveryReportToProto(out.Delivery)}, nil
}
