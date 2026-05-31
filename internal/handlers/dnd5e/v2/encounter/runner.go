package encounter

// runner.go — EncounterRunner: the single load→verb→persist core.
//
// Every action verb (TakeAction, ActivateFeature, EndTurn, Interact,
// SubmitCheck) can eventually live here. The runner owns:
//  1. Load once  — encRepo.Get + tkenc.LoadFromData.
//  2. Auth check — player-membership + entity-ownership.
//  3. Verb call  — caller-supplied closure; the verb mutates the aggregate
//     and publishes to the broker.
//  4. Persist    — enc.ToData() written back to the repo.
//
// The runner is NOT responsible for event forwarding: the long-lived
// StreamEncounter subscription already drains the broker and runs every
// event through translate.go. The runner never harvests or forwards events
// per-call.
//
// The single-load guarantee: because every call to Run does exactly ONE
// repo.Get + LoadFromData + repo.Save, there is no opportunity for the
// "modifier ID already exists" double-subscribe class. Scattered loaders
// (lazy loadCharacterWithBus, applyReactionConditions, charCache) created
// that class; the runner makes it structurally impossible.

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// Runner is the encounter bus-execution core. Construct via newRunner.
type Runner struct {
	broker                 *tkenc.Broker
	repo                   encountersv2.Repository
	resolver               CharacterResolver
	// combatResolver is the fixed resolver override (non-nil in tests that wire
	// HandlerConfig.CombatResolver e.g. StandInCombatResolver). Mirrors the same
	// override field on Handler so verbs executed through the runner respect it.
	combatResolver         CombatResolver
	combatResolverConfig   Dnd5eCombatResolverConfig
	movementResolverConfig Dnd5eMovementResolverConfig
}

// runnerConfig holds the dependencies for the Runner. Internal to this
// package — callers use HandlerConfig; New builds the runner from it.
type runnerConfig struct {
	Broker                 *tkenc.Broker
	Repo                   encountersv2.Repository
	Resolver               CharacterResolver
	// CombatResolver is the fixed override (mirrors HandlerConfig.CombatResolver).
	// When non-nil, buildCombatResolver returns it directly instead of constructing
	// a fresh Dnd5eCombatResolver. Ensures test fixtures that wire StandInCombatResolver
	// are honored by runner-based verbs exactly as they are by the inline handler verbs.
	CombatResolver         CombatResolver
	CombatResolverConfig   Dnd5eCombatResolverConfig
	MovementResolverConfig Dnd5eMovementResolverConfig
}

// newRunner constructs a Runner from the resolved config. Panics on nil
// Broker or Repo — these are programming errors caught at startup.
func newRunner(cfg runnerConfig) *Runner {
	if cfg.Broker == nil {
		panic("runnerConfig.Broker is required")
	}
	if cfg.Repo == nil {
		panic("runnerConfig.Repo is required")
	}
	return &Runner{
		broker:                 cfg.Broker,
		repo:                   cfg.Repo,
		resolver:               cfg.Resolver,
		combatResolver:         cfg.CombatResolver,
		combatResolverConfig:   cfg.CombatResolverConfig,
		movementResolverConfig: cfg.MovementResolverConfig,
	}
}

// EncounterRunnerInput carries the per-request context for Run.
type EncounterRunnerInput struct {
	// EncounterID identifies the encounter to load.
	EncounterID string

	// PlayerID is the authenticated player making the request.
	PlayerID encountercore.PlayerID

	// EntityID is the entity the player claims to control. When non-empty,
	// Run verifies that the encounter data maps PlayerID → EntityID.
	// Pass empty to skip the ownership check (e.g. read-path operations).
	EntityID string
}

// Run is the load→verb→persist execution core.
//
// It:
//  1. Gets the encounter data from the repo (NotFound if missing).
//  2. Verifies the player is a member of the encounter (PermissionDenied if not).
//  3. When EntityID is non-empty, verifies the player owns it (PermissionDenied if not).
//  4. Calls LoadFromData to rehydrate the encounter onto the broker bus.
//  5. Calls verb(enc, data) — the verb mutates aggregates and publishes to the broker.
//  6. Saves enc.ToData() back to the repo.
//
// This is the single load path. Each RPC that calls Run gets exactly one
// LoadFromData per request, so no entity is ever subscribed to the bus twice.
func (r *Runner) Run(
	ctx context.Context,
	in *EncounterRunnerInput,
	verb func(enc *tkenc.Encounter, data *tkenc.Data) error,
) error {
	data, err := r.repo.Get(ctx, in.EncounterID)
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return status.Error(codes.NotFound, "encounter not found")
		}
		return status.Errorf(codes.Internal, "load encounter %q: %v", in.EncounterID, err)
	}

	// Player-membership check. Read from data before LoadFromData to avoid
	// paying rehydration cost on the auth-fail path.
	pd, ok := data.Players[in.PlayerID]
	if !ok {
		return status.Error(codes.PermissionDenied, "player is not in this encounter")
	}

	// Entity-ownership check: only when the caller specifies an EntityID.
	if in.EntityID != "" && string(pd.EntityID) != in.EntityID {
		return status.Error(codes.PermissionDenied, "entity_id does not match player's controlled entity")
	}

	enc, err := tkenc.LoadFromData(data, r.broker,
		tkenc.WithCharacterResolver(r.resolver),
		tkenc.WithCombatResolver(r.buildCombatResolver(data)),
		tkenc.WithMovementResolver(r.buildMovementResolver(data)))
	if err != nil {
		return status.Errorf(codes.Internal, "load encounter from data %q: %v", in.EncounterID, err)
	}

	if verbErr := verb(enc, data); verbErr != nil {
		// Re-surface gRPC status errors unchanged; wrap anything else as Internal.
		if _, ok := status.FromError(verbErr); ok {
			return verbErr
		}
		return status.Errorf(codes.Internal, "encounter verb: %v", verbErr)
	}

	if err := r.repo.Save(ctx, enc.ToData()); err != nil {
		return status.Errorf(codes.Internal, "save encounter %q: %v", in.EncounterID, err)
	}

	return nil
}

// buildCombatResolver returns a per-request combat resolver.
// Mirrors Handler.buildCombatResolver: if a fixed combatResolver was wired
// (e.g. StandInCombatResolver in tests), it is returned directly; otherwise
// a fresh Dnd5eCombatResolver is constructed from the config.
func (r *Runner) buildCombatResolver(data *tkenc.Data) CombatResolver {
	if r.combatResolver != nil {
		return r.combatResolver
	}
	return NewDnd5eCombatResolverForData(r.combatResolverConfig, data)
}

// buildMovementResolver returns a per-request movement resolver.
func (r *Runner) buildMovementResolver(data *tkenc.Data) *Dnd5eMovementResolver {
	return NewDnd5eMovementResolverForData(r.movementResolverConfig, data)
}
