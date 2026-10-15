package authoringv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// newKeyRequiredMessage is the InvalidArgument refusal for an empty new_key.
const newKeyRequiredMessage = "new_key is required"

// RenameDungeon moves one of the caller's encounters to a new key, rewriting
// the file's root key line and nothing else.
func (h *Handler) RenameDungeon(
	ctx context.Context,
	req *authoringpb.RenameDungeonRequest,
) (*authoringpb.RenameDungeonResponse, error) {
	caller, err := callerOf(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, keyRequiredMessage)
	}
	if req.GetNewKey() == "" {
		return nil, status.Error(codes.InvalidArgument, newKeyRequiredMessage)
	}

	_, err = h.orch.RenameDungeon(ctx, &authoringorch.RenameDungeonInput{
		Caller: caller, Key: req.GetKey(), NewKey: req.GetNewKey(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &authoringpb.RenameDungeonResponse{}, nil
}
