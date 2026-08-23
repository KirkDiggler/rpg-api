// Package authoringv1alpha1 is the AuthoringService v1alpha1 handler: thin
// proto <-> input translation over internal/orchestrators/authoring
// (rpg-api#806, rpg-project#256).
//
// Registered by cmd/server only when RPG_AUTHORING_ENABLED=1. With the gate
// off the whole service is absent and a caller sees Unimplemented — the
// proto's documented way for a client to tell "authoring is off" from
// "server unreachable".
package authoringv1alpha1

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// HandlerConfig configures an authoring Handler.
type HandlerConfig struct {
	// Orchestrator is the authoring orchestrator this handler delegates to.
	// Required.
	Orchestrator *authoringorch.Orchestrator
}

// Handler implements dnd5e.api.authoring.v1alpha1.AuthoringServiceServer.
type Handler struct {
	authoringpb.UnimplementedAuthoringServiceServer

	orch *authoringorch.Orchestrator
}

// New constructs a Handler. Returns an error (never a nil Handler) on a
// missing required dependency.
func New(cfg *HandlerConfig) (*Handler, error) {
	if cfg == nil {
		return nil, errors.New("authoring handler: HandlerConfig is required")
	}
	if cfg.Orchestrator == nil {
		return nil, errors.New("authoring handler: HandlerConfig.Orchestrator is required")
	}

	return &Handler{orch: cfg.Orchestrator}, nil
}

// requireAuthenticated is the one gate every RPC here runs: authoring is a
// signed-in verb even though no per-player ownership exists on a dungeon yet
// (rpg-api#803 tracks verb authorization generally), so the caller's identity
// is checked and not yet used.
func requireAuthenticated(ctx context.Context) error {
	if auth.GetPlayerID(ctx) == "" {
		return status.Error(codes.Unauthenticated, "no player id in context")
	}

	return nil
}

// statusError maps the registry's sentinels (passed through the
// orchestrator) onto gRPC codes:
//
//   - dungeons.ErrInvalidKey / ErrKeyMismatch → InvalidArgument (the
//     request could not name its target; the proto's own rule)
//   - dungeons.ErrNotFound → NotFound
//   - dungeons.ErrAuthoringDisabled → FailedPrecondition (unreachable when
//     the service is only registered with the gate on; kept so a wiring
//     mistake is a clear refusal rather than a 500)
//   - unclassified → Internal
func statusError(err error) error {
	switch {
	case errors.Is(err, dungeons.ErrInvalidKey):
		return status.Error(codes.InvalidArgument, "key must match [a-z0-9-]+")
	case errors.Is(err, dungeons.ErrKeyMismatch):
		return status.Error(codes.InvalidArgument, "key does not match the file's own key line")
	case errors.Is(err, dungeons.ErrNotFound):
		return status.Error(codes.NotFound, "dungeon not found")
	case errors.Is(err, dungeons.ErrAuthoringDisabled):
		return status.Error(codes.FailedPrecondition, "authoring is disabled")
	}

	return status.Errorf(codes.Internal, "authoring: %v", err)
}
