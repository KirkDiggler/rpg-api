package authoringv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// GetDungeon returns the stored file for a key, verbatim.
func (h *Handler) GetDungeon(ctx context.Context, req *authoringpb.GetDungeonRequest) (*authoringpb.GetDungeonResponse, error) {
	if err := requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}

	out, err := h.orch.GetDungeon(ctx, &authoringorch.GetDungeonInput{Key: req.GetKey()})
	if err != nil {
		return nil, statusError(err)
	}

	return &authoringpb.GetDungeonResponse{Yaml: string(out.YAML)}, nil
}
