package sessionv1alpha2

import (
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
)

// StreamRecord delivers a session's record after an id, then live (R12,
// R15, R28).
//
// SUBSCRIBE FIRST, THEN REPLAY, THEN LIVE. The subscription is registered
// before the catch-up read, so a beat written between the two is queued
// rather than missed; a live beat at or below the last id already sent is
// the same beat the read returned, and is skipped. Each beat goes out once,
// in id order.
//
// A LAG ENDS THE STREAM, TOLD. When the broker closes the subscription as
// lagged, the handler sends what was queued, then RecordLagged{last_id} with
// the last id it sent, and ends with OK.
//
// A STALE RESUME IS TOLD TOO. A resume point older than retention opens the
// stream with RecordRestarted, then replays from the oldest retained beat,
// so the subscriber replaces what it holds instead of appending across a
// hole (R25, R28).
func (h *Handler) StreamRecord(req *sessionpb.StreamRecordRequest, stream sessionpb.RecordService_StreamRecordServer) error {
	ctx := stream.Context()
	session, member := req.GetSession(), req.GetMember()
	if err := h.gate.CallerMemberSeated(ctx, session, member); err != nil {
		return err
	}

	sub, err := h.broker.Subscribe(session)
	if err != nil {
		return status.Errorf(codes.Internal, "subscribe: %v", err)
	}
	defer func() { _ = sub.Close() }()

	replay, err := h.manager.Record(ctx, &sdk.RecordReadInput{Session: session, AfterID: req.GetAfterId()})
	if err != nil {
		return sdkerr.StatusError(err)
	}
	last := req.GetAfterId()
	if replay.Restarted {
		slog.InfoContext(ctx, logRecordRestarted, "session", session, "after_id", req.GetAfterId(), "rpc", "StreamRecord")
		if err := stream.Send(&sessionpb.StreamRecordResponse{
			Item: &sessionpb.StreamRecordResponse_Restarted{Restarted: &sessionpb.RecordRestarted{}},
		}); err != nil {
			return err
		}
		last = "" // the replay starts at the oldest retained beat
	}

	send := func(evt sdk.RecordEvent) error {
		converted, cerr := recordEventToProto(evt)
		if cerr != nil {
			slog.ErrorContext(ctx, "session record: fields not projectable, sent without them",
				"session", session, "id", evt.ID, "beat", evt.Beat, "error", cerr)
		}
		if err := stream.Send(&sessionpb.StreamRecordResponse{
			Item: &sessionpb.StreamRecordResponse_Event{Event: converted},
		}); err != nil {
			return err
		}
		last = evt.ID
		return nil
	}
	for _, evt := range replay.Events {
		if err := send(evt); err != nil {
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-sub.Events():
			if !ok {
				if lagged, _ := sub.Lagged(); lagged {
					slog.WarnContext(ctx, "session record: subscriber lagged, stream ending",
						"session", session, "member", member, "last_id", last)
					return sendLagged(stream, last)
				}
				return nil
			}
			if evt.ID <= last {
				continue // already sent by the catch-up read
			}
			if err := send(evt); err != nil {
				return err
			}
		}
	}
}

// sendLagged sends the stream's last message.
func sendLagged(stream sessionpb.RecordService_StreamRecordServer, lastID string) error {
	return stream.Send(&sessionpb.StreamRecordResponse{
		Item: &sessionpb.StreamRecordResponse_Lagged{Lagged: &sessionpb.RecordLagged{LastId: lastID}},
	})
}
