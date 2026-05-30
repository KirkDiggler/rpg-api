package encounter

// runner.go — EncounterRunner: the single load→verb→persist core.
//
// Every action verb (TakeAction, ActivateFeature, EndTurn, Interact,
// SubmitCheck) eventually lives here. The runner owns:
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

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// RunnerConfig holds the dependencies for the EncounterRunner.
type RunnerConfig struct {
	Broker                 *tkenc.Broker
	Repo                   encountersv2.Repository
	Resolver               CharacterResolver            // optional; defaults to StubCharacterResolver
	CombatResolverConfig   *Dnd5eCombatResolverConfig   // optional
	MovementResolverConfig *Dnd5eMovementResolverConfig // optional
	Now                    func() time.Time             // optional; defaults to time.Now
}

// Runner is the encounter bus-execution core. Construct via NewRunner.
type Runner struct {
	broker                 *tkenc.Broker
	repo                   encountersv2.Repository
	resolver               CharacterResolver
	combatResolverConfig   Dnd5eCombatResolverConfig
	movementResolverConfig Dnd5eMovementResolverConfig
	now                    func() time.Time
}

// NewRunner constructs an EncounterRunner. Panics if required config fields
// are nil (programming error — caller must provide Broker + Repo).
func NewRunner(cfg RunnerConfig) *Runner {
	if cfg.Broker == nil {
		panic("RunnerConfig.Broker is required")
	}
	if cfg.Repo == nil {
		panic("RunnerConfig.Repo is required")
	}

	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	resolver := cfg.Resolver
	if resolver == nil {
		resolver = StubCharacterResolver{}
	}

	combatResolverConfig := Dnd5eCombatResolverConfig{}
	if cfg.CombatResolverConfig != nil {
		combatResolverConfig = *cfg.CombatResolverConfig
	}

	movementResolverConfig := Dnd5eMovementResolverConfig{}
	if cfg.MovementResolverConfig != nil {
		movementResolverConfig = *cfg.MovementResolverConfig
	}

	return &Runner{
		broker:                 cfg.Broker,
		repo:                   cfg.Repo,
		resolver:               resolver,
		combatResolverConfig:   combatResolverConfig,
		movementResolverConfig: movementResolverConfig,
		now:                    now,
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
// The verb must not return a non-nil error from grpc/status (it won't be
// rewrapped); any error is wrapped as codes.Internal if it is not already a
// gRPC status error.
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

// buildCombatResolver returns a per-request combat resolver, following the
// same selection logic as Handler.buildCombatResolver.
func (r *Runner) buildCombatResolver(data *tkenc.Data) CombatResolver {
	return NewDnd5eCombatResolverForData(r.combatResolverConfig, data)
}

// buildMovementResolver returns a per-request movement resolver.
func (r *Runner) buildMovementResolver(data *tkenc.Data) *Dnd5eMovementResolver {
	return NewDnd5eMovementResolverForData(r.movementResolverConfig, data)
}

