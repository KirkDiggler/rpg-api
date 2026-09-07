package sessionpresentationv1alpha1

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	presentationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	orchsessionpresentation "github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation"
)

func (h *Handler) StreamDiceThrows(req *presentationpb.StreamDiceThrowsRequest, stream presentationpb.SessionPresentationService_StreamDiceThrowsServer) error {
	ctx := stream.Context()
	if err := h.access.CallerMemberSeated(ctx, req.GetSession(), req.GetMember()); err != nil {
		return err
	}

	sub, err := h.service.Subscribe(ctx, &orchsessionpresentation.SubscribeInput{Session: req.GetSession(), Member: req.GetMember()})
	if err != nil {
		return sessionPresentationRPCError(err)
	}
	if sub == nil {
		return status.Error(codes.Internal, errSubscriptionRequired)
	}
	defer func() { _ = sub.Close() }()

	for {
		select {
		case <-ctx.Done():
			return nil
		case plan, ok := <-sub.Plans():
			if !ok {
				return nil
			}
			if err := stream.Send(planToProto(plan)); err != nil {
				return err
			}
		}
	}
}
