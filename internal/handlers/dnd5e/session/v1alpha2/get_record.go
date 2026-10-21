package sessionv1alpha2

import (
	"context"
	"log/slog"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
)

// logRecordRestarted is the R25 measurement: how often a catch-up asks
// further back than the record retains.
const logRecordRestarted = "session record: catch-up restarted (older than retention)"

// GetRecord reads the retained record after an id (R15). The caller must be
// seated as `member`.
func (h *Handler) GetRecord(ctx context.Context, req *sessionpb.GetRecordRequest) (*sessionpb.GetRecordResponse, error) {
	if err := h.gate.CallerMemberSeated(ctx, req.GetSession(), req.GetMember()); err != nil {
		return nil, err
	}
	out, err := h.manager.Record(ctx, &sdk.RecordReadInput{
		Session: req.GetSession(), AfterID: req.GetAfterId(), Limit: int(req.GetLimit()),
	})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}
	if out.Restarted {
		slog.InfoContext(ctx, logRecordRestarted, "session", req.GetSession(), "after_id", req.GetAfterId())
	}
	events := make([]*sessionpb.RecordEvent, 0, len(out.Events))
	for _, evt := range out.Events {
		converted, cerr := recordEventToProto(evt)
		if cerr != nil {
			slog.ErrorContext(ctx, "session record: fields not projectable, sent without them",
				"session", req.GetSession(), "id", evt.ID, "beat", evt.Beat, "error", cerr)
		}
		events = append(events, converted)
	}
	return &sessionpb.GetRecordResponse{Events: events, Restarted: out.Restarted}, nil
}
