package authoringv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// PutDungeon compiles a dungeon file and, unless validate_only, stores it.
//
// Two kinds of failure, two transports (the proto's rule): a request that
// cannot name its target (empty key, bad charset, key/file mismatch) is a
// gRPC status; a file that does not compile is an OK response carrying
// `errors`, because the author needs the list, not a code.
func (h *Handler) PutDungeon(ctx context.Context, req *authoringpb.PutDungeonRequest) (*authoringpb.PutDungeonResponse, error) {
	if err := requireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}

	out, err := h.orch.PutDungeon(ctx, &authoringorch.PutDungeonInput{
		Key:          req.GetKey(),
		YAML:         []byte(req.GetYaml()),
		ValidateOnly: req.GetValidateOnly(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	resp := &authoringpb.PutDungeonResponse{}
	for _, fe := range out.Errors {
		resp.Errors = append(resp.Errors, &authoringpb.FieldError{Path: fe.Path, Message: fe.Message})
	}
	if len(out.Errors) == 0 && out.Atlas != nil {
		// The SAME conversion GetAtlas uses: one producer of the wire atlas.
		resp.Atlas = sessionhandler.AtlasToProto(out.Atlas)
	}

	return resp, nil
}
