package sessionv1alpha2

import (
	"context"
	"log/slog"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
)

// maxRecordPage is the most events one GetRecord returns. A run's record is
// retained whole (session launches with unbounded retention), so a read is
// never unbounded: a limit of zero or less, or above this, means this.
const maxRecordPage = 500

// logRecordRestarted is the R25 measurement: how often a catch-up asks
// further back than the record retains.
const logRecordRestarted = "session record: catch-up restarted (older than retention)"

// GetRecord reads one page of the retained record after an id (R15). The
// caller must be seated as `member`. The page is at most maxRecordPage
// events; `more` says retained events after it did not fit, and is the
// client's only end signal.
func (h *Handler) GetRecord(ctx context.Context, req *sessionpb.GetRecordRequest) (*sessionpb.GetRecordResponse, error) {
	if err := h.gate.CallerMemberSeated(ctx, req.GetSession(), req.GetMember()); err != nil {
		return nil, err
	}
	page := int(req.GetLimit())
	if page <= 0 || page > maxRecordPage {
		page = maxRecordPage
	}
	// One past the page, to learn whether more remain without a second read.
	out, err := h.manager.Record(ctx, &sdk.RecordReadInput{
		Session: req.GetSession(), AfterID: req.GetAfterId(), Limit: page + 1,
	})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}
	if out.Restarted {
		slog.InfoContext(ctx, logRecordRestarted, "session", req.GetSession(), "after_id", req.GetAfterId())
	}
	read, more := out.Events, false
	if len(read) > page {
		read, more = read[:page], true
	}
	events := make([]*sessionpb.RecordEvent, 0, len(read))
	for _, evt := range read {
		converted, cerr := recordEventToProto(evt)
		if cerr != nil {
			slog.ErrorContext(ctx, "session record: fields not projectable, sent without them",
				"session", req.GetSession(), "id", evt.ID, "beat", evt.Beat, "error", cerr)
		}
		events = append(events, converted)
	}
	return &sessionpb.GetRecordResponse{Events: events, Restarted: out.Restarted, More: more}, nil
}
