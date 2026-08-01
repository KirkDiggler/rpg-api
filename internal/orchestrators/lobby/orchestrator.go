// Package lobby is the LobbyService orchestrator: party assembly (join refs,
// membership, ready flags, lifecycle) plus the sole encounter-construction
// path (StartEncounter subsumes the deleted v1alpha2 CreateEncounter RPC).
//
// Boundary: NO rules logic lives here. The lobby is pure API orchestration —
// the toolkit has zero lobby concept and this package must keep it that way
// (rpg-project's lobby-surface.md design, "Toolkit owns nothing here").
// StartEncounter's toolkit construction (tkenc.New + AddPlayer per member)
// is data movement — building the toolkit's own encounter by reference,
// authoring no game rules — the same "no boundary violation" classification
// rpg-api#616 gave the single-caller version this replaces. Combat /
// movement resolvers stay injected as builders (CombatResolverBuilder /
// MovementResolverBuilder) so this package never imports the dnd5e
// rulebook, mirroring internal/orchestrators/encounter/v2's own discipline.
package lobby

import (
	"errors"
	"time"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"

	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
)

// CombatResolverBuilder builds a per-request combat resolver from the fresh
// encounter's data (always nil at StartEncounter time — a brand new
// encounter has no monsters yet). Injected by the handler so this package
// stays free of rulebook imports; mirrors
// internal/orchestrators/encounter/v2.CombatResolverBuilder.
type CombatResolverBuilder func(data *tkenc.Data) tkenc.CombatResolver

// MovementResolverBuilder builds a per-request movement resolver. Same
// rationale as CombatResolverBuilder.
type MovementResolverBuilder func(data *tkenc.Data) tkenc.MovementResolver

// CharacterResolverBuilder builds a per-request CharacterResolver bridging
// the toolkit's CharacterResolver to the character store (rpg-api#516). Same
// rationale as CombatResolverBuilder — the toolkit interface only carries a
// PlayerID, so a real implementation needs the loaded *tkenc.Data to map
// that PlayerID to a character; mirrors
// internal/orchestrators/encounter/v2.CharacterResolverBuilder.
type CharacterResolverBuilder func(data *tkenc.Data) tkenc.CharacterResolver

// defaultPartyCap is the Dungeon Night shape (lobby-surface.md "Party cap").
const defaultPartyCap = 4

// Config holds the dependencies for an Orchestrator.
type Config struct {
	// LobbyRepo persists lobby state. Required.
	LobbyRepo lobbyrepo.Repository

	// LobbyBroker fans out StreamLobby events. Required.
	LobbyBroker *Broker

	// CharacterRepo resolves character_id -> owning player + display name
	// (CreateLobby/JoinLobby) and seeds per-member HP at StartEncounter.
	// Required.
	CharacterRepo characterrepo.Repository

	// EncounterRepo is where StartEncounter persists the freshly constructed
	// encounter — the same v2 encounter repo the encounter service reads
	// from. Required.
	EncounterRepo encountersv2.Repository

	// EncounterBroker is the same v2 encounter broker the encounter service
	// subscribes StreamEncounter callers against — StartEncounter builds the
	// new encounter on this broker so it is live the instant clients switch
	// streams. Required.
	EncounterBroker *tkenc.Broker

	// BuildCharacterResolver builds the per-encounter CharacterResolver
	// bridging the toolkit's CharacterResolver to the character store, wired
	// onto every StartEncounter-constructed encounter. Required (the handler
	// supplies a builder that defaults to StubCharacterResolver, mirroring
	// the encounter handler's own default).
	BuildCharacterResolver CharacterResolverBuilder

	// BuildCombatResolver builds the per-encounter combat resolver at
	// StartEncounter. Required.
	BuildCombatResolver CombatResolverBuilder

	// BuildMovementResolver builds the per-encounter movement resolver at
	// StartEncounter. Required.
	BuildMovementResolver MovementResolverBuilder

	// LobbyIDGenerator mints opaque lobby_id values. Required.
	LobbyIDGenerator idgen.Generator

	// JoinRefGenerator mints opaque join_ref values. Required.
	JoinRefGenerator idgen.Generator

	// EncounterIDGenerator mints the constructed encounter's ID at
	// StartEncounter. Required.
	EncounterIDGenerator idgen.Generator

	// Registry is the shared, concurrency-safe live dungeon-spec registry
	// (internal/dungeonregistry) — the SAME pointer cmd/server/server.go
	// also passes into the authoring orchestrator's Config, so a
	// PutDungeon's registry swap is visible to the very next
	// StartEncounter with no restart (plan.md's "Architecture decision:
	// the shared live registry"). Required. Built once at startup via
	// LoadContentRegistry — New() itself never reads RPG_CONTENT_DIR;
	// that construction-time "operator/config mistake must fail loudly"
	// guarantee now lives at LoadContentRegistry's call site instead of
	// here (a behavior migration from this field's predecessor,
	// contentSpecs, which loadContentSpecs built inline).
	Registry *dungeonregistry.Registry

	// PartyCap is the max member count JoinLobby allows. Optional — defaults
	// to 4 (lobby-surface.md "Party cap").
	PartyCap int

	// Now supplies the current time. Optional — defaults to time.Now.
	// Reserved; no lobby field is timestamped today (TTL is repo-owned), but
	// kept for parity with the v2 encounter orchestrator's Config and any
	// future need (e.g. abandonment metrics).
	Now func() time.Time

	// DungeonKeyOverride is the RPG_DUNGEON_KEY dev-loop mechanism (Task
	// E2b): when non-empty, StartEncounter substitutes this key ONLY when
	// the caller supplied no DungeonKey at all — an explicit caller-
	// supplied key always wins (no real proto surface sets one yet, so
	// this is the only way to actually select a non-default key end to
	// end today). Optional — "" (unset RPG_DUNGEON_KEY, every real
	// deployment today) is a no-op, zero player-facing change.
	//
	// Validated here at construction, not deferred to first request: must
	// resolve via either Registry (and not be a DISABLED content key — a
	// load-error key set as the override fails loudly at boot too, same
	// posture as any other Config validation failure) or the legacy
	// dungeonSpecs map. An override naming nothing real is a
	// deploy-time misconfiguration, not something that should surface only
	// on the first StartEncounter call.
	DungeonKeyOverride string
}

// Orchestrator is the lobby load -> mutate -> persist -> publish core. One
// method per LobbyService RPC (plus the internal SetConnected used by
// StreamLobby's subscribe/disconnect lifecycle).
type Orchestrator struct {
	lobbyRepo              lobbyrepo.Repository
	lobbyBroker            *Broker
	characterRepo          characterrepo.Repository
	encounterRepo          encountersv2.Repository
	encounterBroker        *tkenc.Broker
	buildCharacterResolver CharacterResolverBuilder
	buildCombatResolver    CombatResolverBuilder
	buildMovementResolver  MovementResolverBuilder
	lobbyIDGen             idgen.Generator
	joinRefGen             idgen.Generator
	encounterIDGen         idgen.Generator
	partyCap               int
	now                    func() time.Time
	locks                  *keyedMutex

	// registry is Config.Registry — the shared live dungeon-spec registry.
	// StartEncounter resolves against it (resolveContentDungeonSpec), never
	// by calling content.AllSpecs/SpecByKey again per request.
	registry *dungeonregistry.Registry

	// dungeonKeyOverride is Config.DungeonKeyOverride, already validated
	// to resolve (Task E2b) — "" when RPG_DUNGEON_KEY was unset at
	// startup, in which case StartEncounter's effectiveKey substitution is
	// a no-op.
	dungeonKeyOverride DungeonKey
}

// New constructs an Orchestrator from cfg. Returns an error (never a nil
// Orchestrator) when a required dependency is missing.
func New(cfg *Config) (*Orchestrator, error) {
	if cfg == nil {
		return nil, errors.New("lobby orchestrator: Config is required")
	}
	if cfg.LobbyRepo == nil {
		return nil, errors.New("lobby orchestrator: Config.LobbyRepo is required")
	}
	if cfg.LobbyBroker == nil {
		return nil, errors.New("lobby orchestrator: Config.LobbyBroker is required")
	}
	if cfg.CharacterRepo == nil {
		return nil, errors.New("lobby orchestrator: Config.CharacterRepo is required")
	}
	if cfg.EncounterRepo == nil {
		return nil, errors.New("lobby orchestrator: Config.EncounterRepo is required")
	}
	if cfg.EncounterBroker == nil {
		return nil, errors.New("lobby orchestrator: Config.EncounterBroker is required")
	}
	if cfg.BuildCharacterResolver == nil {
		return nil, errors.New("lobby orchestrator: Config.BuildCharacterResolver is required")
	}
	if cfg.BuildCombatResolver == nil {
		return nil, errors.New("lobby orchestrator: Config.BuildCombatResolver is required")
	}
	if cfg.BuildMovementResolver == nil {
		return nil, errors.New("lobby orchestrator: Config.BuildMovementResolver is required")
	}
	if cfg.LobbyIDGenerator == nil {
		return nil, errors.New("lobby orchestrator: Config.LobbyIDGenerator is required")
	}
	if cfg.JoinRefGenerator == nil {
		return nil, errors.New("lobby orchestrator: Config.JoinRefGenerator is required")
	}
	if cfg.EncounterIDGenerator == nil {
		return nil, errors.New("lobby orchestrator: Config.EncounterIDGenerator is required")
	}
	if cfg.Registry == nil {
		return nil, errors.New("lobby orchestrator: Config.Registry is required")
	}
	if cfg.PartyCap < 0 {
		return nil, errors.New("lobby orchestrator: Config.PartyCap must not be negative")
	}
	partyCap := cfg.PartyCap
	if partyCap == 0 {
		partyCap = defaultPartyCap
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	// RPG_DUNGEON_KEY override (Task E2b): validated NOW, against
	// cfg.Registry (built by the caller — see LoadContentRegistry — before
	// New() ever runs), so a misconfigured override fails construction
	// loudly rather than deferring to StartEncounter's first real call.
	if err := validateDungeonKeyOverride(cfg.DungeonKeyOverride, cfg.Registry); err != nil {
		return nil, err
	}

	return &Orchestrator{
		lobbyRepo:              cfg.LobbyRepo,
		lobbyBroker:            cfg.LobbyBroker,
		characterRepo:          cfg.CharacterRepo,
		encounterRepo:          cfg.EncounterRepo,
		encounterBroker:        cfg.EncounterBroker,
		buildCharacterResolver: cfg.BuildCharacterResolver,
		buildCombatResolver:    cfg.BuildCombatResolver,
		buildMovementResolver:  cfg.BuildMovementResolver,
		lobbyIDGen:             cfg.LobbyIDGenerator,
		joinRefGen:             cfg.JoinRefGenerator,
		encounterIDGen:         cfg.EncounterIDGenerator,
		partyCap:               partyCap,
		now:                    now,
		locks:                  newKeyedMutex(),
		registry:               cfg.Registry,
		dungeonKeyOverride:     DungeonKey(cfg.DungeonKeyOverride),
	}, nil
}

// orderedMembers returns o's members in join order (MemberOrder), for
// deterministic snapshots and AddPlayer iteration.
func orderedMembers(data *lobbyrepo.Data) []*lobbyrepo.Member {
	out := make([]*lobbyrepo.Member, 0, len(data.MemberOrder))
	for _, pid := range data.MemberOrder {
		if m, ok := data.Members[pid]; ok {
			out = append(out, m)
		}
	}
	return out
}
