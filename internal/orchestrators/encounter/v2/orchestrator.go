// Package encounter is the v1alpha2 encounter orchestrator: the single
// load → toolkit-verb → persist core for the v2 encounter vertical.
//
// It is the clean replacement for the handler-package Runner + the inline
// verb bodies (rpg-api#582). The orchestrator speaks entity / toolkit types
// only — it NEVER imports proto (pb.*). The thin v2 encounter handler converts
// proto ↔ these Input/Output types and maps the orchestrator's sentinel errors
// onto gRPC status codes; the orchestrator owns load, the toolkit verb call,
// and persistence.
//
// Boundary: NO rules logic lives here. The orchestrator routes by reference and
// orchestrates load/verb/save; all game mechanics live in the rpg-toolkit
// encounter SDK and the dnd5e rulebook (reached only through the combat /
// movement resolver adapters). If something here starts to feel rule-ish (a
// tier table, an AC computation, a "can this fire" decision), it belongs in the
// toolkit or a resolver adapter — surface it, do not inline it.
package encounter

import (
	"context"
	"errors"
	"time"

	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
)

// CombatResolver is the toolkit's combat resolver interface. The orchestrator
// wires an implementation onto every LoadFromData so attack-resolving verbs can
// dispatch through it. Aliased here so the orchestrator package references a
// single name regardless of which implementation is supplied.
type CombatResolver = tkenc.CombatResolver

// CharacterResolver is the toolkit's character resolver interface (ability
// modifier / tool-proficiency lookups for skill checks). Aliased for the same
// reason as CombatResolver.
type CharacterResolver = tkenc.CharacterResolver

// CombatResolverBuilder builds a per-request CombatResolver from the encounter
// data. The handler supplies this so the orchestrator stays free of rulebook
// imports: the concrete Dnd5eCombatResolver (which legitimately imports the
// dnd5e rulebook as a translation adapter) is constructed handler-side and
// handed in as a builder. When a fixed resolver override is present (tests),
// the builder returns it directly.
type CombatResolverBuilder func(data *tkenc.Data) CombatResolver

// MovementResolverBuilder builds a per-request movement resolver from the
// encounter data. Same rationale as CombatResolverBuilder — the concrete
// Dnd5eMovementResolver is a rulebook-importing adapter built handler-side.
//
// The return type is the toolkit MovementResolver interface so the orchestrator
// never imports the dnd5e rulebook movement adapter.
type MovementResolverBuilder func(data *tkenc.Data) tkenc.MovementResolver

// CharacterDataCascade attaches/persists the transient PlayerData.DataJSON for
// the #689 hydration cascade on combat-capable verbs. The orchestrator calls
// AttachCharacterData (before LoadFromData) and PersistCharacterData (after
// ToData, before Save). The concrete implementations marshal the dnd5e
// character blob, so they are supplied handler-side as funcs — the orchestrator
// stays free of the rulebooks/dnd5e/character import (rule-ish serialization is
// the handler/adapter's job, not the orchestrator's).
//
// Both funcs are no-ops when the handler has no character store wired (tests):
// the door verbs never set loadInput.WithCharacterData, so the cascade is never
// invoked on those paths.
type CharacterDataCascade struct {
	// Attach sets transient PlayerData.DataJSON on data from the character store,
	// before LoadFromData hydrates the held *character.Character per seat.
	Attach func(ctx context.Context, data *tkenc.Data) error

	// Persist flushes each player's post-verb DataJSON back to the character
	// store (the authoritative source) and clears it so it does not persist onto
	// the encounter snapshot.
	Persist func(ctx context.Context, data *tkenc.Data) error

	// SeedActorTurn seeds the active actor's turn-start action economy in the
	// character store when the encounter is turn-based and that actor has not yet
	// been seeded (rpg-api#598). The toolkit enforces the economy server-side and
	// seeds it only on a held character at SetMode/EndTurn; the v2 turn-based-entry
	// sites flip to turn-based before any character is held, so the FIRST actor is
	// never seeded and its TakeAction is rejected "not in combat". This runs the
	// toolkit's own StartTurn verb for that actor — the orchestrator invokes it by
	// reference (like EndTurn), authoring no economy values. Idempotent: a no-op
	// once the active actor is in combat. Supplied handler-side so the orchestrator
	// stays free of the rulebooks/dnd5e/character import; nil = no-op (tests
	// without a character store). Called in the combat-capable load path.
	SeedActorTurn func(ctx context.Context, data *tkenc.Data) error
}

// ReactionResume carries the rulebook-touching pieces of the two-phase attack
// (TakeAction phase-1 pause + SubmitCheck take_reaction phase-2 resume) as
// injected funcs, so the orchestrator runs both halves without importing the
// dnd5e combat rulebook. The AttackContext marshal/decode (combat.AttackContext
// is the resolver's native opaque shape) and the take/skip -> ReactionModifier
// mapping (which carries the rule-ish Shield +5 AC magnitude) are supplied
// handler-side.
type ReactionResume struct {
	// MarshalAttackContext serializes the in-flight PhasedAttackContext returned
	// by TakeActionPhased (Rulebook = the resolver's native *combat.AttackContext)
	// into the opaque AttackContextJSON persisted on the pending reaction prompt.
	// The phase-1 counterpart to DecodeAttackContext; both live handler-side so
	// the orchestrator never type-asserts the rulebook payload. Required only on
	// the TakeAction reaction-pause path (when a player reaction pauses the
	// attack); checked lazily there, so non-phased callers need not wire it.
	MarshalAttackContext func(ctx *tkenc.PhasedAttackContext) ([]byte, error)

	// DecodeAttackContext unmarshals the persisted opaque AttackContextJSON back
	// into the toolkit's PhasedAttackContext (Rulebook = the resolver's native
	// *combat.AttackContext). Returns an error on a corrupt blob.
	DecodeAttackContext func(raw []byte) (*tkenc.PhasedAttackContext, error)

	// BuildReactionModifiers maps a pending reaction prompt + take/skip choice
	// to the ReactionModifier set for phase 2. When take is false (skip), it
	// returns no modifiers. The rule-ish per-condition modifier magnitudes
	// (Shield = +5 AC) live here, handler-side.
	BuildReactionModifiers func(prompt *tkenc.PendingReactionPrompt, take bool) []tkenc.ReactionModifier

	// IsOneShotReaction decides whether a taken reaction consumes its readiness
	// (so the reactor must re-ready before the next window) vs. stays ready
	// (free reactions like an opportunity attack). This is a rulebook decision —
	// which reactions are one-shot — so it lives handler-side. The orchestrator
	// only performs the readiness reset (a toolkit-flag bookkeeping call) when
	// this returns true, keeping it free of any per-condition ref knowledge.
	IsOneShotReaction func(conditionRef string) bool
}

// Config holds the dependencies for an Orchestrator.
type Config struct {
	// Broker is the encounter event broker. Required.
	Broker *tkenc.Broker

	// EncounterRepo persists encounter snapshots. Required.
	EncounterRepo encountersv2.Repository

	// Resolver bridges the toolkit's CharacterResolver to the character store
	// for skill-check totals. Required (the handler supplies a stub default).
	Resolver CharacterResolver

	// BuildCombatResolver builds the per-request combat resolver. Required.
	BuildCombatResolver CombatResolverBuilder

	// BuildMovementResolver builds the per-request movement resolver. Required.
	BuildMovementResolver MovementResolverBuilder

	// CharacterData attaches/persists the transient #689 cascade DataJSON on
	// combat-capable verbs. Optional — when the funcs are nil, combat-capable
	// verbs run without the cascade (the resolver stand-in path), which keeps
	// tests that don't wire a character store green. The door verbs (Interact)
	// never invoke it.
	CharacterData CharacterDataCascade

	// ReactionResume carries the rulebook-touching funcs for the SubmitCheck
	// take_reaction branch (AttackContext decode + modifier-building). Required
	// for SubmitReactionCheck; the skill-check and door verbs do not use it.
	ReactionResume ReactionResume

	// Now supplies the current time. Optional — defaults to time.Now.
	//
	// Not used by the door verbs (Interact); reserved for the projection /
	// snapshot read path carved in later steps of #582.
	Now func() time.Time
}

// Orchestrator is the v2 encounter load → verb → persist core. Construct via
// New. One method per encounter RPC; each does exactly one load + one
// toolkit-verb dispatch + one persist (the single-load guarantee that makes the
// #684 double-subscribe class structurally impossible).
type Orchestrator struct {
	broker  *tkenc.Broker
	encRepo encountersv2.Repository
	// now is wired now but unused by the door verbs (Interact); reserved for the
	// projection / snapshot read path carved in later steps of #582.
	resolver              CharacterResolver
	buildCombatResolver   CombatResolverBuilder
	buildMovementResolver MovementResolverBuilder
	// characterData carries the #689 hydration cascade (attach/persist of
	// transient PlayerData.DataJSON) for combat-capable verbs. The injected funcs
	// hold the character store handler-side, so the orchestrator needs no direct
	// character-repo handle of its own.
	characterData  CharacterDataCascade
	reactionResume ReactionResume
	now            func() time.Time
}

// New constructs an Orchestrator from cfg. Returns an error (never a nil
// Orchestrator) when a required dependency is missing.
func New(cfg *Config) (*Orchestrator, error) {
	if cfg == nil {
		return nil, errors.New("encounter orchestrator: Config is required")
	}
	if cfg.Broker == nil {
		return nil, errors.New("encounter orchestrator: Config.Broker is required")
	}
	if cfg.EncounterRepo == nil {
		return nil, errors.New("encounter orchestrator: Config.EncounterRepo is required")
	}
	if cfg.Resolver == nil {
		return nil, errors.New("encounter orchestrator: Config.Resolver is required")
	}
	if cfg.BuildCombatResolver == nil {
		return nil, errors.New("encounter orchestrator: Config.BuildCombatResolver is required")
	}
	if cfg.BuildMovementResolver == nil {
		return nil, errors.New("encounter orchestrator: Config.BuildMovementResolver is required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Orchestrator{
		broker:                cfg.Broker,
		encRepo:               cfg.EncounterRepo,
		resolver:              cfg.Resolver,
		buildCombatResolver:   cfg.BuildCombatResolver,
		buildMovementResolver: cfg.BuildMovementResolver,
		characterData:         cfg.CharacterData,
		reactionResume:        cfg.ReactionResume,
		now:                   now,
	}, nil
}
