package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// GetStory reads the beats a member has witnessed, from FromSeq onward
// inclusive. The resync source of truth (design rule 6): a client that
// notices a gap in a stream event's seq re-queries here.
func (h *Handler) GetStory(ctx context.Context, req *sessionpb.GetStoryRequest) (*sessionpb.GetStoryResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	entries, err := h.manager.Story(ctx, &sdk.StoryInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
		FromSeq: req.GetFromSeq(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.GetStoryResponse{Entries: storyEntriesToProto(entries)}, nil
}
