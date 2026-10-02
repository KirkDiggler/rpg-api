package sessionv1alpha1

import (
	"log/slog"

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

	member, session := req.GetMember(), req.GetSession()
	gate, err := h.accessGate()
	if err != nil {
		return err
	}
	if seatErr := gate.CallerMemberSeated(ctx, session, member); seatErr != nil {
		return seatErr
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
			// A beat whose body this build cannot spell on the wire ENDS
			// THE STREAM rather than going out with the unspellable value
			// quietly replaced (rpg-project#458, answerWordToProto). A
			// stream that dropped it would leave a seq gap the client
			// cannot distinguish from a beat it was not entitled to see,
			// which is the one failure a per-recipient stream must never
			// produce silently.
			converted, err := eventToProto(evt)
			if err != nil {
				slog.ErrorContext(ctx, "session stream: event body not projectable, stream ending",
					"session", session, "recipient", member, "seq", evt.Seq, "kind", evt.Kind, "error", err)
				return status.Errorf(codes.Internal, "project event: %v", err)
			}
			if err := stream.Send(converted); err != nil {
				// The trace logged below only ever covers a SUCCESSFUL
				// Send -- logging beforehand would claim delivery for an
				// event this handler never actually got out (a
				// disconnected or full stream), which is worse than no
				// trace at all: a one-look diagnosis that lies (Copilot,
				// PR #821). This is that failure's own line instead.
				slog.DebugContext(ctx, "session stream: send failed, event not forwarded",
					"session", session, "recipient", member, "seq", evt.Seq, "kind", evt.Kind, "error", err)
				return err
			}
			// The per-recipient send trace: session, recipient, seq, body
			// kind, one line per event actually sent (after Send returns
			// successfully -- see above). rpg-api#819 "Defect 2" --
			// without this there is no way to tell "the forwarder never
			// sent it" from "the client dropped it": the next missing beat
			// is a one-look diagnosis against the client's own raw feed
			// (rpg-dnd5e-web#740) once both traces exist side by side.
			// Debug-level and unconditional (cheap: one log call per
			// already-sent event) -- on by default in local dev
			// (cmd/server/server.go raises the default slog level when
			// AUTH_DEV_MODE is set), silent elsewhere unless raised.
			slog.DebugContext(ctx, "session stream: forwarded event",
				"session", session, "recipient", member, "seq", evt.Seq, "kind", evt.Kind)
		}
	}
}
