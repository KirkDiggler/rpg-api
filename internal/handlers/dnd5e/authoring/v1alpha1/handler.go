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
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
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

// keyRequiredMessage is the InvalidArgument refusal for an empty key.
const keyRequiredMessage = "key is required"

// noPlayerMessage is the Unauthenticated refusal every RPC here answers
// without a signed-in player.
const noPlayerMessage = "no player id in context"

// requireAuthenticated is the gate the catalog RPCs run (ListScenarios,
// ListWeapons): their answer is a property of the binary, so they need a
// signed-in player and no world.
func requireAuthenticated(ctx context.Context) error {
	if auth.GetPlayerID(ctx) == "" {
		return status.Error(codes.Unauthenticated, noPlayerMessage)
	}

	return nil
}

// callerOf is the gate every dungeon RPC runs: the signed-in player and the
// world the auth boundary installed (worldcontext), never a request field
// (design R21). No player is Unauthenticated; no world is FailedPrecondition.
func callerOf(ctx context.Context) (dungeons.Caller, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return dungeons.Caller{}, status.Error(codes.Unauthenticated, noPlayerMessage)
	}
	world, ok := worldcontext.Get(ctx)
	if !ok || world.WorldID == "" {
		return dungeons.Caller{}, status.Error(codes.FailedPrecondition, worldcontext.RequiredMessage)
	}

	return dungeons.Caller{WorldID: world.WorldID, PlayerID: playerID}, nil
}

// statusError maps the registry's sentinels (passed through the
// orchestrator) onto gRPC codes:
//
//   - dungeons.ErrInvalidKey / ErrKeyMismatch → InvalidArgument (the
//     request could not name its target; the proto's own rule)
//   - dungeons.ErrNotFound → NotFound
//   - dungeons.ErrReferenceKey / ErrReferenceReadOnly / ErrOwnedByOther →
//     PermissionDenied, the sentinel's own sentence as the message: the
//     author reads it at the key field, the preview included (S7-Q2)
//   - dungeons.ErrKeyExists → AlreadyExists, its sentence
//   - dungeons.ErrAuthoringDisabled → FailedPrecondition (unreachable when
//     the service is only registered with the gate on; kept so a wiring
//     mistake is a clear refusal rather than a 500)
//   - dungeons.ErrNoCaller → FailedPrecondition (the handler checks the
//     caller first; kept so a wiring mistake is a clear refusal)
//   - unclassified → Internal
func statusError(err error) error {
	switch {
	case errors.Is(err, dungeons.ErrReferenceKey):
		return status.Error(codes.PermissionDenied, dungeons.ErrReferenceKey.Error())
	case errors.Is(err, dungeons.ErrReferenceReadOnly):
		return status.Error(codes.PermissionDenied, dungeons.ErrReferenceReadOnly.Error())
	case errors.Is(err, dungeons.ErrOwnedByOther):
		return status.Error(codes.PermissionDenied, dungeons.ErrOwnedByOther.Error())
	case errors.Is(err, dungeons.ErrKeyExists):
		return status.Error(codes.AlreadyExists, dungeons.ErrKeyExists.Error())
	case errors.Is(err, dungeons.ErrNoCaller):
		return status.Error(codes.FailedPrecondition, worldcontext.RequiredMessage)
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
