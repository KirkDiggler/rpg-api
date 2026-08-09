// Package authoring is the AuthoringService orchestrator: PutDungeon,
// gated behind RPG_AUTHORING_ENABLED at cmd/server/server.go (this package
// itself has no opinion on the gate — it's the caller's job to decide
// whether to construct one at all, mirroring how every other orchestrator
// in this repo has no ambient env-reading of its own).
//
// Boundary: NO rules logic lives here beyond the toolkit calls the
// existing dungeonspec package already exposes (Load, Decode) — this
// package's own job is request-shape validation (key charset, key/YAML
// key: match), write-through targeting, and registry orchestration, the
// same "outside-in, orchestrators call the toolkit rather than
// reimplementing it" discipline internal/orchestrators/lobby already
// follows for StartEncounter.
package authoring

import (
	"errors"

	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
)

// Config holds the dependencies for an Orchestrator.
type Config struct {
	// Registry is the SAME shared *dungeonregistry.Registry pointer
	// cmd/server/server.go also passes into the lobby orchestrator's
	// Config — PutDungeon's registry swap must be visible to the very
	// next StartEncounter with no restart (plan.md's "Architecture
	// decision: the shared live registry"). A private registry
	// constructed here instead would compile, pass this package's own
	// tests, and still fail that requirement. Required.
	Registry *dungeonregistry.Registry

	// ContentDir is RPG_CONTENT_DIR's value, passed in explicitly rather
	// than read from os.Getenv inside this package (matching
	// lobbyorch.Config's existing style of explicit fields over ambient
	// env reads). Required — enabling the authoring gate without
	// RPG_CONTENT_DIR set is a construction-time failure (design.md's
	// "the gate requires RPG_CONTENT_DIR" decision), not a degraded mode:
	// write-through's on-disk durability guarantee has nowhere to write
	// without it.
	ContentDir string

	// Compiler is the protobuf-free provider seam. Nil selects the released
	// v0.3 toolkit adapter; Wave A finalization replaces that adapter with the
	// rpg-toolkit#897 provider without changing orchestration.
	Compiler Compiler

	// PartyStartSeatCount is the host's normal party capacity supplied to
	// dungeonspec.LoadWithConfig. Preview must compile the same reservation
	// StartEncounter will use; zero defaults to the normal four-seat product
	// configuration for standalone/test construction.
	PartyStartSeatCount int
}

const defaultPartyStartSeatCount = 4

// Orchestrator is the AuthoringService orchestrator core.
type Orchestrator struct {
	registry            *dungeonregistry.Registry
	contentDir          string
	partyStartSeatCount int
	compiler            Compiler
	updateLocks         *keyedMutex
	replaceSource       func(*replaceSourceInput) error
}

// New constructs an Orchestrator from cfg. Returns an error (never a nil
// Orchestrator) when a required dependency is missing — including a
// missing ContentDir, which is the "gate requires RPG_CONTENT_DIR, fails
// fast at construction" decision from design.md, enforced here exactly
// like lobbyorch.New's existing required-field checks.
func New(cfg *Config) (*Orchestrator, error) {
	if cfg == nil {
		return nil, errors.New("authoring orchestrator: Config is required")
	}
	if cfg.Registry == nil {
		return nil, errors.New("authoring orchestrator: Config.Registry is required")
	}
	if cfg.ContentDir == "" {
		return nil, errors.New("authoring orchestrator: Config.ContentDir is required")
	}
	if cfg.PartyStartSeatCount < 0 {
		return nil, errors.New("authoring orchestrator: Config.PartyStartSeatCount must not be negative")
	}
	partyStartSeatCount := cfg.PartyStartSeatCount
	if partyStartSeatCount == 0 {
		partyStartSeatCount = defaultPartyStartSeatCount
	}
	compiler := cfg.Compiler
	if compiler == nil {
		compiler = toolkitCompiler{}
	}
	return &Orchestrator{
		registry:            cfg.Registry,
		contentDir:          cfg.ContentDir,
		partyStartSeatCount: partyStartSeatCount,
		compiler:            compiler,
		updateLocks:         newKeyedMutex(),
		replaceSource:       replaceSourceDurably,
	}, nil
}
