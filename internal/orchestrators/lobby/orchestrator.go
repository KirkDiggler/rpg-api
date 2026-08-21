// Package lobby is the LobbyService orchestrator: party assembly (join refs,
// membership, ready flags, lifecycle) plus the sole encounter-construction
// path (StartEncounter subsumes the deleted v1alpha2 CreateEncounter RPC).
//
// Boundary: NO rules logic lives here. The lobby is pure API orchestration —
// the toolkit has zero lobby concept and this package must keep it that way
// (rpg-project's lobby-surface.md design, "Toolkit owns nothing here").
// StartEncounter builds the session stack's world via internal/sessionworld
// and seats members through the rulebooks/dnd5e/session SDK (sdk.Manager) —
// data movement, authoring no game rules.
//
// The old encounter stack (github.com/KirkDiggler/rpg-toolkit/encounter,
// internal/orchestrators/encounter/v2) was removed in rpg-project#227: the
// session stack is StartEncounter's ONLY implementation now, not one branch
// of a coexistence choice.
package lobby

import (
	"errors"
	"time"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
)

// DefaultPartyCap is the normal product roster capacity. It was host policy
// shared with content/authoring's party-start configuration before both were
// deleted alongside the old encounter stack's dungeon-spec dialect
// (rpg-project#227); StartEncounter's session-stack path still passes it to
// sessionworld for the embedded reference-tomb's party seating.
const DefaultPartyCap = 4

// Config holds the dependencies for an Orchestrator.
type Config struct {
	// LobbyRepo persists lobby state. Required.
	LobbyRepo lobbyrepo.Repository

	// LobbyBroker fans out StreamLobby events. Required.
	LobbyBroker *Broker

	// CharacterRepo resolves character_id -> owning player + display name
	// (CreateLobby/JoinLobby). Required.
	CharacterRepo characterrepo.Repository

	// LobbyIDGenerator mints opaque lobby_id values. Required.
	LobbyIDGenerator idgen.Generator

	// JoinRefGenerator mints opaque join_ref values. Required.
	JoinRefGenerator idgen.Generator

	// EncounterIDGenerator mints the constructed session/encounter ID at
	// StartEncounter. Required.
	EncounterIDGenerator idgen.Generator

	// PartyCap is the max member count JoinLobby allows. Optional — defaults
	// to DefaultPartyCap (lobby-surface.md "Party cap").
	PartyCap int

	// Now supplies the current time. Optional — defaults to time.Now.
	// Reserved; no lobby field is timestamped today (TTL is repo-owned), but
	// kept for parity with other orchestrators' Config and any future need
	// (e.g. abandonment metrics).
	Now func() time.Time

	// SessionManager is the toolkit's rulebooks/dnd5e/session SDK entry
	// point StartEncounter builds onto, and GetMyActiveLobby/AbandonEncounter
	// query/close through (Manager.Status / Manager.End). Required.
	SessionManager *sdk.Manager
}

// Orchestrator is the lobby load -> mutate -> persist -> publish core. One
// method per LobbyService RPC (plus the internal SetConnected used by
// StreamLobby's subscribe/disconnect lifecycle).
type Orchestrator struct {
	lobbyRepo      lobbyrepo.Repository
	lobbyBroker    *Broker
	characterRepo  characterrepo.Repository
	lobbyIDGen     idgen.Generator
	joinRefGen     idgen.Generator
	encounterIDGen idgen.Generator
	partyCap       int
	now            func() time.Time
	locks          *keyedMutex

	// sessionManager is Config.SessionManager — StartEncounter builds onto
	// it, and GetMyActiveLobby/AbandonEncounter query/close through it.
	sessionManager *sdk.Manager
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
	if cfg.LobbyIDGenerator == nil {
		return nil, errors.New("lobby orchestrator: Config.LobbyIDGenerator is required")
	}
	if cfg.JoinRefGenerator == nil {
		return nil, errors.New("lobby orchestrator: Config.JoinRefGenerator is required")
	}
	if cfg.EncounterIDGenerator == nil {
		return nil, errors.New("lobby orchestrator: Config.EncounterIDGenerator is required")
	}
	if cfg.SessionManager == nil {
		return nil, errors.New("lobby orchestrator: Config.SessionManager is required")
	}
	if cfg.PartyCap < 0 {
		return nil, errors.New("lobby orchestrator: Config.PartyCap must not be negative")
	}
	partyCap := cfg.PartyCap
	if partyCap == 0 {
		partyCap = DefaultPartyCap
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &Orchestrator{
		lobbyRepo:      cfg.LobbyRepo,
		lobbyBroker:    cfg.LobbyBroker,
		characterRepo:  cfg.CharacterRepo,
		lobbyIDGen:     cfg.LobbyIDGenerator,
		joinRefGen:     cfg.JoinRefGenerator,
		encounterIDGen: cfg.EncounterIDGenerator,
		partyCap:       partyCap,
		now:            now,
		locks:          newKeyedMutex(),
		sessionManager: cfg.SessionManager,
	}, nil
}

// orderedMembers returns o's members in join order (MemberOrder), for
// deterministic snapshots and Join iteration.
func orderedMembers(data *lobbyrepo.Data) []*lobbyrepo.Member {
	out := make([]*lobbyrepo.Member, 0, len(data.MemberOrder))
	for _, pid := range data.MemberOrder {
		if m, ok := data.Members[pid]; ok {
			out = append(out, m)
		}
	}
	return out
}
