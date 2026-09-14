// Package session owns the game server's single point of contact with the
// toolkit's rulebooks/dnd5e/session SDK: it constructs the one session.Manager
// (with every capability supplied explicitly -- toolkit law, never defaulted)
// and implements the repositories and event stream the SDK calls outward
// into (design doc rpg-project/ideas/session-api/design.md §3).
//
// This package holds no game rules and wraps no SDK verb in further
// Input/Output types: the SDK's own verbs (Manager.Join, Manager.Move, ...)
// already return exactly that shape, so handlers call the Manager directly.
// The only thing this package adds beyond construction is routing --
// translating storage and delivery, never deciding anything about the game.
package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/KirkDiggler/rpg-toolkit/dice"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// Config carries what New needs to build an Orchestrator. Redis and Characters
// are required; optional host settings have the defaults documented below.
// New supplies each SDK capability explicitly.
type Config struct {
	// StaleTargetPolicy is host configuration for known-creature casts.
	// Empty selects the API default, refuse; attempt permits paid misses.
	// The toolkit validates nonempty values and owns all casting rules.
	StaleTargetPolicy sdk.StaleTargetPolicy

	// Redis is the client backing the session and encounter stores.
	Redis redisclient.Client

	// Characters is rpg-api's existing character store, adapted to the SDK's
	// CharacterRepository contract by NewCharacterRepository.
	Characters characterrepo.Repository

	// TTL is the per-key expiration applied to session and encounter Redis
	// keys. Pass 0 to disable expiration.
	TTL time.Duration

	// Dice is the SDK's source of randomness. Optional: nil selects a
	// crypto-secure dice.CryptoRoller, the production default. This is this
	// PACKAGE's own ergonomics, not a relaxation of the toolkit's "supplied,
	// never defaulted" law -- what New hands to sdk.Config.Dice is always
	// explicit and never nil; the override exists so a deterministic roller
	// can be substituted in tests (a fixed source makes a reproducible
	// fight, exactly as session.Roller's own doc describes) without this
	// package's callers reaching past it into the SDK's construction.
	Dice sdk.Roller

	// PresentationIDs supplies opaque shared-die correlation tokens. Optional:
	// nil selects a UUID generator in production. Tests supply a sequential
	// generator so token identity is deterministic without deriving it from a
	// Story or recipient-local sequence.
	PresentationIDs sdk.PresentationIDGenerator

	// TurnDriver decides what an unplayed member does on its turn. Optional:
	// nil selects the production wiring, which is ONE sdk.Minded(nil) PER
	// SESSION handed over by turnDriverCache below. Dice's own doc explains
	// the shape and it applies verbatim here -- what New hands the SDK is
	// always explicit, and the override exists so a test can script a
	// monster's turn without reaching past this package into the SDK's
	// construction.
	//
	// A DRIVER SUPPLIED HERE SERVES EVERY SESSION, which is the one place the
	// override differs from production: it goes to sdk.Config.TurnDriver, the
	// every-session door, and no per-session cache is built (the SDK refuses
	// both doors wired at once). That is the right shape for the thing this
	// field exists for -- a scripted driver is the assertion, and a test that
	// wired one would not want a second session quietly getting a different
	// one.
	//
	// The reason a test needs to: the default driver attacks a standing
	// player and otherwise closes the distance, so it will never walk OUT of
	// a fighter's reach. A reaction window is exactly what a monster leaving
	// reach opens (rpg-project#316), which means the interrupt path has no
	// producer under the shipped driver and could not otherwise be proven
	// through this stack at all.
	TurnDriver sdk.TurnDriver
}

// turnDriverCache answers "what happens when the clock lands on a member with
// no player" -- toolkit#1162, ADR-0043 (rpg-toolkit encounter#1163) -- with
// ONE DRIVER PER SESSION, built the first time that session is seen.
//
// Without a driver at all, EndTurn parks the clock on a monster forever:
// nothing can act for it, since EndTurn requires Member to be the active
// member and the host binds Member to the authenticated human, who does not
// own the monster.
//
// sdk.Minded(nil) is the production driver as of rpg-toolkit#1725: each
// member is driven by the mind ITS OWN SHEET NAMES (rule A5), and a member
// whose sheet names none gets exactly sdk.Behavior()'s answer -- attack the
// closest standing player if one is in reach, otherwise close the distance,
// otherwise pass. Three monsters name a mind as of rpg-toolkit#1745 -- the
// skeleton a "retaliator", the thug a "berserker", the goblin a "coward" --
// and every other monster's turn is unchanged.
//
// Nil is the whole input, and there is nothing left to pass: how long a mind
// holds a grudge and how much room it keeps are the MIND's, named by the
// word on a monster's sheet and held by the rulebook's preset for that word.
// The Patience this package used to leave unset is gone from the SDK
// entirely, which is the same principle stated better -- a host naming a
// rules number is the smell CLAUDE.md opens with, and now there is no number
// here to leave alone. It wraps rulebooks/dnd5e/behavior entirely inside the
// toolkit; this package never imports encounter or behavior.
//
// # Why the cache is HERE and not in the SDK
//
// A session's lifetime is this package's: a Redis TTL, a run ending. The SDK's
// Manager is deliberately stateless per verb and gets no session-end signal,
// so a cache in there would have no owner for eviction. The SDK asks instead
// (sdk.Config.TurnDrivers, rpg-toolkit#1734, adoption rule A6) and this is
// what answers.
//
// # The lock is on the MAP, never on a driver
//
// sdk.Minded is stateful and its own doc says so -- it remembers which mind
// each member was given and parks the view it is answering on itself for the
// length of the call. What that value is not safe for is two goroutines
// inside ONE session's turns, and that is the boundary the session already
// had: two write verbs on one session race that session's own scope in the
// SDK regardless of the driver. So this lock guards the lookup and the build,
// and a driver handed out is a driver this type is done with. The mutex that
// used to wrap Act is gone with the process-wide driver it protected.
//
// # Nothing is evicted, and that is a decision
//
// A driver is a handful of small maps, and a process sees a bounded number of
// sessions between deploys, so the map's growth is bounded by the same thing
// the process is. What would pay for eviction is a server that outlives its
// sessions by enough that the dead ones dominate -- a long-lived process
// serving short one-shot runs, or a session count per process that stops
// being bounded by a deploy. At that point the eviction signal is the one
// this package already owns: the Redis key's expiry, or run ending.
type turnDriverCache struct {
	mu      sync.Mutex
	drivers map[string]sdk.TurnDriver
}

// newTurnDriverCache returns an empty cache. It builds no driver: the first
// verb about a session is what mints that session's.
func newTurnDriverCache() *turnDriverCache {
	return &turnDriverCache{drivers: map[string]sdk.TurnDriver{}}
}

// compile-time proof the cache satisfies what it is handed to.
var _ sdk.TurnDriverSource = (*turnDriverCache)(nil)

// DriverFor returns sessionID's own driver, building one on first sight.
//
// A failure here is a WIRING fault, not a game outcome, so it is returned and
// the verb that asked fails -- a monster silently falling back to the basic
// driver would look like a design choice rather than a broken pin. The SDK
// never falls back either (sdk.Config.TurnDrivers).
func (c *turnDriverCache) DriverFor(_ context.Context, sessionID string) (sdk.TurnDriver, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if driver, built := c.drivers[sessionID]; built {
		return driver, nil
	}

	driver, err := sdk.Minded(nil)
	if err != nil {
		return nil, fmt.Errorf("construct turn driver for session %q: %w", sessionID, err)
	}

	c.drivers[sessionID] = driver
	return driver, nil
}

const presentationIDPrefix = "presentation"

func newDefaultPresentationIDs() sdk.PresentationIDGenerator {
	return idgen.NewUUID(presentationIDPrefix)
}

// Orchestrator owns the toolkit session.Manager and the Broker StreamEvents
// subscribes against. Both are exported for handlers to use directly:
// Manager for every verb, Broker for StreamEvents' subscription.
type Orchestrator struct {
	Manager *sdk.Manager
	Broker  *Broker
}

// New constructs an Orchestrator, wiring every session.Config capability
// explicitly: Redis-backed SessionRepository and EncounterRepository, the
// CharacterRepository adapter, this package's Broker as the EventStream, and
// a crypto-secure dice.CryptoRoller as the SDK's Roller -- the host supplies
// entropy only, never turn order (session.Roller doc).
func New(cfg Config) (*Orchestrator, error) {
	if cfg.Redis == nil {
		return nil, errors.New("session orchestrator: Config.Redis is required")
	}
	if cfg.Characters == nil {
		return nil, errors.New("session orchestrator: Config.Characters is required")
	}

	roller := cfg.Dice
	if roller == nil {
		roller = &dice.CryptoRoller{}
	}

	presentationIDs := cfg.PresentationIDs
	if presentationIDs == nil {
		presentationIDs = newDefaultPresentationIDs()
	}

	// EXACTLY ONE OF THE TWO DOORS, which the SDK enforces at construction.
	// Production wires the per-session source; a test that scripted a monster's
	// turn wired one driver, and that driver keeps serving every session,
	// because a scripted driver is what the test is asserting about.
	drivers := newTurnDriverCache()
	var (
		sourceForSDK sdk.TurnDriverSource = drivers
		driverForSDK sdk.TurnDriver
	)
	if cfg.TurnDriver != nil {
		sourceForSDK, driverForSDK = nil, cfg.TurnDriver
	}

	policy := cfg.StaleTargetPolicy
	if policy == "" {
		policy = sdk.StaleTargetRefuse
	}

	broker := NewBroker()
	mgr, err := sdk.NewManager(&sdk.Config{
		StaleTargetPolicy: policy,
		PresentationIDs:   presentationIDs,
		Sessions:          NewSessionRepository(cfg.Redis, cfg.TTL),
		Encounters:        NewEncounterRepository(cfg.Redis, cfg.TTL),
		Characters:        NewCharacterRepository(cfg.Characters),
		Events:            broker,
		Dice:              roller,
		TurnDriver:        driverForSDK,
		TurnDrivers:       sourceForSDK,
	})
	if err != nil {
		return nil, fmt.Errorf("construct session manager: %w", err)
	}

	return &Orchestrator{Manager: mgr, Broker: broker}, nil
}
