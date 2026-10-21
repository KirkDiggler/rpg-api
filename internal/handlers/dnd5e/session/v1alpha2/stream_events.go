package sessionv1alpha2

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
)

// logActRefused is the R22 measurement: an act that went out refused.
const logActRefused = "session story: act refused, sent with its id and no body"

// logStoryRestarted is the R25 measurement on the act stream.
const logStoryRestarted = "session story: catch-up restarted (older than retention)"

// StreamEvents delivers one member's acts after an id, then live (R12, R28).
//
// SUBSCRIBE FIRST, THEN REPLAY, THEN LIVE. The subscription is registered
// before the catch-up read, so an act folded between the two is queued rather
// than missed. A live act whose id is at or below the last REPLAYED id is one
// the read already returned, and is skipped; the comparison is against the
// replay's end, never the last id sent, because the acts of one same-id run
// share an id and every one of them must go out.
//
// A LAG ENDS THE STREAM, TOLD, BETWEEN TWO IDS. The broker enqueues a same-id
// run whole or not at all, and the replay is whole runs, so when the broker
// closes the subscription as lagged the handler drains what was queued and
// the last id it sent ends a whole run. It sends StoryLagged{last_id} with
// that id and ends with OK.
//
// A STALE RESUME IS TOLD TOO: StoryRestarted first, then a replay from the
// oldest retained beat (R25, R28).
func (h *StoryHandler) StreamEvents(req *sessionpb.StreamEventsRequest, stream sessionpb.StoryService_StreamEventsServer) error {
	ctx := stream.Context()
	session, member := req.GetSession(), req.GetMember()
	if err := h.gate.CallerMemberSeated(ctx, session, member); err != nil {
		return err
	}

	sub, err := h.broker.Subscribe(session, member)
	if err != nil {
		return status.Errorf(codes.Internal, "subscribe: %v", err)
	}
	defer func() { _ = sub.Close() }()

	replay, err := h.manager.Acts(ctx, &sdk.ActsInput{Session: session, Member: member, AfterID: req.GetAfterId()})
	if err != nil {
		return sdkerr.StatusError(err)
	}
	// last is the last id this stream delivered. Before anything is sent it is
	// the request's after_id, as on StreamRecord: a lag before the first send
	// then reports the request's own resume point, which is the right GetStory
	// catch-up.
	last := req.GetAfterId()
	if replay.Restarted {
		slog.InfoContext(ctx, logStoryRestarted, "session", session, "member", member, "after_id", req.GetAfterId(), "rpc", "StreamEvents")
		if err := stream.Send(&sessionpb.StreamEventsResponse{
			Item: &sessionpb.StreamEventsResponse_Restarted{Restarted: &sessionpb.StoryRestarted{}},
		}); err != nil {
			return err
		}
		last = "" // the replay starts at the oldest retained beat
	}

	send := func(a sdk.Act) error {
		if err := stream.Send(&sessionpb.StreamEventsResponse{
			Item: &sessionpb.StreamEventsResponse_Act{Act: convertLogged(ctx, a)},
		}); err != nil {
			return err
		}
		last = a.ID
		return nil
	}
	for _, a := range replay.Acts {
		if err := send(a); err != nil {
			return err
		}
	}
	replayed := last

	for {
		select {
		case <-ctx.Done():
			return nil
		case a, ok := <-sub.Acts():
			if !ok {
				if lagged, _ := sub.Lagged(); lagged {
					slog.WarnContext(ctx, "session story: subscriber lagged, stream ending",
						"session", session, "member", member, "last_id", last)
					return stream.Send(&sessionpb.StreamEventsResponse{
						Item: &sessionpb.StreamEventsResponse_Lagged{Lagged: &sessionpb.StoryLagged{LastId: last}},
					})
				}
				return nil
			}
			if a.ID <= replayed {
				continue // already sent by the catch-up read
			}
			if err := send(a); err != nil {
				return err
			}
		}
	}
}

// convertLogged spells one act, logging the reason when it goes out refused.
func convertLogged(ctx context.Context, a sdk.Act) *sessionpb.Act {
	out, err := actToProto(a)
	if err != nil {
		slog.ErrorContext(ctx, logActRefused,
			"session", a.Session, "recipient", a.Recipient, "id", a.ID, "act", a.Act, "family", a.Family, "error", err)
	}
	return out
}
