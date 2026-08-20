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

	member := req.GetMember()
	if err := h.callerActingAs(ctx, member); err != nil {
		return err
	}

	// This is the ONE verb that never reaches the Manager, so an empty session
	// is refused nowhere else. Unvalidated it does not fail -- it subscribes
	// under the empty key and the call hangs forever, delivering nothing, which
	// is the worst way for a stream to be wrong. Every other verb inherits this
	// refusal from the SDK's ErrNoSessionID; this one has to say it itself.
	session := req.GetSession()
	if session == "" {
		return status.Error(codes.InvalidArgument, "session is required")
	}

	sub, err := h.broker.Subscribe(session, member)
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
