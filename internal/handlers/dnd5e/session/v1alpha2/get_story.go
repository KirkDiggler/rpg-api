package sessionv1alpha2

import (
	"context"
	"log/slog"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
)

// maxStoryPage is the most acts one GetStory page asks for, before the page is
// extended to the end of its last same-id run. A limit of zero or less, or
// above this, means this.
const maxStoryPage = 500

// GetStory reads one page of a member's acts after an id (R12, R15, R22).
//
// THE CUT FOLLOWS THE FOLD. The SDK folds the member's whole story after the
// id (sdk.Manager.Acts has no limit, on purpose: cutting entries before the
// fold would close a segment early and make a page differ from live). This
// handler then takes the first `limit` acts and EXTENDS THE PAGE TO THE END OF
// ITS LAST SAME-ID RUN, so a page ends only between two ids and the next
// page, read after the last returned id, starts at the next run whole. `more`
// says acts remain.
//
// The cost is that each page re-folds the member's acts after its after_id:
// quadratic over one full catch-up, bounded by the run's size. The debug log
// line below measures it.
func (h *StoryHandler) GetStory(ctx context.Context, req *sessionpb.GetStoryRequest) (*sessionpb.GetStoryResponse, error) {
	if err := h.gate.CallerMemberSeated(ctx, req.GetSession(), req.GetMember()); err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > maxStoryPage {
		limit = maxStoryPage
	}
	out, err := h.manager.Acts(ctx, &sdk.ActsInput{
		Session: req.GetSession(), Member: req.GetMember(), AfterID: req.GetAfterId(),
	})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}
	if out.Restarted {
		slog.InfoContext(ctx, logStoryRestarted, "session", req.GetSession(), "member", req.GetMember(),
			"after_id", req.GetAfterId(), "rpc", "GetStory")
	}
	cut := pageEnd(out.Acts, limit)
	slog.DebugContext(ctx, "session story: page folded",
		"session", req.GetSession(), "member", req.GetMember(), "after_id", req.GetAfterId(),
		"folded", len(out.Acts), "returned", cut)

	acts := make([]*sessionpb.Act, 0, cut)
	for _, a := range out.Acts[:cut] {
		acts = append(acts, convertLogged(ctx, a))
	}
	return &sessionpb.GetStoryResponse{Acts: acts, Restarted: out.Restarted, More: cut < len(out.Acts)}, nil
}

// pageEnd is how many of acts one page returns: the first limit, extended to
// the end of the last same-id run they reach into.
func pageEnd(acts []sdk.Act, limit int) int {
	if len(acts) <= limit {
		return len(acts)
	}
	cut := limit
	for cut < len(acts) && acts[cut].ID == acts[cut-1].ID {
		cut++
	}
	return cut
}
