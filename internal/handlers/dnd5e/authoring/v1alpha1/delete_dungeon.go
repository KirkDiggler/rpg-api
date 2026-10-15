package authoringv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// DeleteDungeon removes one of the caller's encounters from their world.
// Another member's is PermissionDenied, a reference is PermissionDenied,
// an absent key is NotFound.
func (h *Handler) DeleteDungeon(
	ctx context.Context,
	req *authoringpb.DeleteDungeonRequest,
) (*authoringpb.DeleteDungeonResponse, error) {
	caller, err := callerOf(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, keyRequiredMessage)
	}

	if _, err := h.orch.DeleteDungeon(ctx, &authoringorch.DeleteDungeonInput{Caller: caller, Key: req.GetKey()}); err != nil {
		return nil, statusError(err)
	}

	return &authoringpb.DeleteDungeonResponse{}, nil
}
