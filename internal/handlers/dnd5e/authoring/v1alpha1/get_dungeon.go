package authoringv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// GetDungeon returns the stored file for a key, verbatim, resolved for the
// caller: their world's record, then the shipped reference.
//
// An empty key is InvalidArgument once the caller is admitted: the web's
// authoring gate probes with one and relies on a non-Unimplemented answer.
func (h *Handler) GetDungeon(ctx context.Context, req *authoringpb.GetDungeonRequest) (*authoringpb.GetDungeonResponse, error) {
	caller, err := callerOf(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, keyRequiredMessage)
	}

	out, err := h.orch.GetDungeon(ctx, &authoringorch.GetDungeonInput{Caller: caller, Key: req.GetKey()})
	if err != nil {
		return nil, statusError(err)
	}

	return &authoringpb.GetDungeonResponse{Yaml: string(out.YAML)}, nil
}
