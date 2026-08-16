package sessionv1alpha1

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// StreamEvents subscribes the caller to everything addressed to one member
// and forwards it verbatim (design rule 4: rpg-api neither filters nor
// re-derives visibility -- the SDK already projected each event per
// audience before Publish ever ran). Unlike GetStory, this carries no replay
// obligation (design rule 6): a client resyncs via GetStory on a seq gap or
// on reconnect, not by expecting a snapshot here.
func (h *Handler) StreamEvents(req *sessionpb.StreamEventsRequest, stream sessionpb.SessionService_StreamEventsServer) error {
	ctx := stream.Context()

	playerID, err := authenticatedPlayerID(ctx)
	if err != nil {
		return err
	}
	member := req.GetMember()
	if member == "" {
		return status.Error(codes.InvalidArgument, "member is required")
	}
	if ownershipErr := h.verifyMemberOwnership(ctx, playerID, member); ownershipErr != nil {
		return ownershipErr
	}

	sub, err := h.broker.Subscribe(req.GetSession(), member)
	if err != nil {
		return status.Errorf(codes.Internal, "subscribe: %v", err)
	}
	defer func() { _ = sub.Close() }()

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-sub.Events():
			if !ok {
				return nil
			}
			if err := stream.Send(eventToProto(evt)); err != nil {
				return err
			}
		}
	}
}
