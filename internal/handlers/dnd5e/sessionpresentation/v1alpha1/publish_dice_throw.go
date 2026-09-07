package sessionpresentationv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	presentationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	orchsessionpresentation "github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation"
)

func (h *Handler) PublishDiceThrow(ctx context.Context, req *presentationpb.PublishDiceThrowRequest) (*presentationpb.PublishDiceThrowResponse, error) {
	if err := h.access.CallerMemberSeated(ctx, req.GetSession(), req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.service.Publish(ctx, &orchsessionpresentation.PublishInput{
		Session: req.GetSession(),
		Member:  req.GetMember(),
		Draft:   draftFromProto(req.GetDraft()),
	})
	if err != nil {
		return nil, sessionPresentationRPCError(err)
	}
	if out == nil {
		return nil, status.Error(codes.Internal, errPublishOutputRequired)
	}

	return &presentationpb.PublishDiceThrowResponse{Plan: planToProto(out.Plan)}, nil
}
