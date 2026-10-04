package sessionv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// SetDiscoverySharing updates only the authenticated caller's exact seated character.
func (h *Handler) SetDiscoverySharing(ctx context.Context, req *sessionpb.SetDiscoverySharingRequest) (*sessionpb.SetDiscoverySharingResponse, error) {
	gate, err := h.accessGate()
	if err != nil {
		return nil, err
	}
	if seatErr := gate.CallerMemberSeated(ctx, req.GetSession(), req.GetMember()); seatErr != nil {
		return nil, seatErr
	}
	if req.Sharing == nil {
		return nil, status.Error(codes.InvalidArgument, "sharing must be explicitly supplied")
	}
	out, err := h.manager.SetDiscoverySharing(ctx, &sdk.SetDiscoverySharingInput{Session: req.GetSession(), Member: req.GetMember(), Sharing: req.GetSharing()})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}
	return &sessionpb.SetDiscoverySharingResponse{Sharing: out.Sharing, Saved: saveReportToProto(out.Saved), Delivery: deliveryReportToProto(out.Delivery)}, nil
}
