// Package authoring is the AuthoringService v1alpha1 handler: thin proto
// <-> input translation over internal/orchestrators/authoring, mirroring
// the lobby v1alpha1 handler's shape (internal/handlers/dnd5e/lobby/v1alpha1)
// — Chapter-1 layering, not handler-package orchestration.
package authoring

import (
	"errors"

	authoringv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// HandlerConfig configures a Handler.
type HandlerConfig struct {
	// Orchestrator is the authoring orchestrator this handler delegates
	// to. Required.
	Orchestrator *authoringorch.Orchestrator
}

// Handler implements dnd5e.api.authoring.v1alpha1.AuthoringServiceServer.
// cmd/server/server.go only ever registers this with the gRPC server when
// RPG_AUTHORING_ENABLED is set — see plan.md S1's gate decision. This
// package itself has no gate logic of its own; it's simply never wired in
// when the gate is off, which is what makes PutDungeon Unimplemented in
// that case (grpc's own behavior for an unregistered service, not a
// custom check here).
type Handler struct {
	authoringv1alpha1.UnimplementedAuthoringServiceServer
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
