// Package lobby is the LobbyService v1alpha1 handler: thin proto <-> input
// translation over internal/orchestrators/lobby, mirroring the v2 encounter
// handler's shape (internal/handlers/dnd5e/v2/encounter) — Chapter-1
// layering from day one, not the handler-package orchestration #616
// complains about.
package lobby

import (
	"errors"
	"time"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	encounterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
)

// HandlerConfig configures a lobby Handler.
type HandlerConfig struct {
	// Orchestrator is the lobby orchestrator this handler delegates to.
	// Required.
	Orchestrator *lobbyorch.Orchestrator

	// Broker fans out StreamLobby events — the SAME *lobbyorch.Broker passed
	// into Orchestrator's Config (one broker instance shared by both).
	// Required.
	Broker *lobbyorch.Broker

	// Now supplies the current time. Optional — defaults to time.Now.
	Now func() time.Time
}

// Handler implements dnd5e.api.lobby.v1alpha1.LobbyServiceServer.
type Handler struct {
	lobbyv1alpha1.UnimplementedLobbyServiceServer
	orch   *lobbyorch.Orchestrator
	broker *lobbyorch.Broker
	now    func() time.Time
}

// New constructs a Handler. Returns an error (never a nil Handler) on a
// missing required dependency.
func New(cfg *HandlerConfig) (*Handler, error) {
	if cfg == nil {
		return nil, errors.New("lobby handler: HandlerConfig is required")
	}
	if cfg.Orchestrator == nil {
		return nil, errors.New("lobby handler: HandlerConfig.Orchestrator is required")
	}
	if cfg.Broker == nil {
		return nil, errors.New("lobby handler: HandlerConfig.Broker is required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Handler{orch: cfg.Orchestrator, broker: cfg.Broker, now: now}, nil
}

// BuildCombatResolver adapts encounterhandlerv2's Dnd5eCombatResolver
// construction into the lobbyorch.CombatResolverBuilder shape, so
// StartEncounter's freshly constructed encounter gets the SAME production
// combat resolver the encounter service itself uses — never a duplicate
// implementation. Exported so cmd/server/server.go and the integration
// harness can wire it without reaching into this package's internals.
func BuildCombatResolver(cfg encounterhandlerv2.Dnd5eCombatResolverConfig) lobbyorch.CombatResolverBuilder {
	return func(data *tkenc.Data) tkenc.CombatResolver {
		return encounterhandlerv2.NewDnd5eCombatResolverForData(cfg, data)
	}
}

// BuildMovementResolver is BuildCombatResolver's movement-resolver
// counterpart.
func BuildMovementResolver(cfg encounterhandlerv2.Dnd5eMovementResolverConfig) lobbyorch.MovementResolverBuilder {
	return func(data *tkenc.Data) tkenc.MovementResolver {
		return encounterhandlerv2.NewDnd5eMovementResolverForData(cfg, data)
	}
}
