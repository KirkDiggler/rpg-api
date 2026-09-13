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
	// nil selects sdk.Minded(nil), the production driver (see
	// newDefaultTurnDriver below). Dice's own doc explains the shape and it
	// applies verbatim here -- what New hands to sdk.Config.TurnDriver is
	// always explicit and never nil, and the override exists so a test can
	// script a monster's turn without reaching past this package into the
	// SDK's construction.
	//
	// The reason a test needs to: the default driver attacks a standing
	// player and otherwise closes the distance, so it will never walk OUT of
	// a fighter's reach. A reaction window is exactly what a monster leaving
	// reach opens (rpg-project#316), which means the interrupt path has no
	// producer under the shipped driver and could not otherwise be proven
	// through this stack at all.
	TurnDriver sdk.TurnDriver
}

// newDefaultTurnDriver answers "what happens when the clock lands on a member
// with no player" -- toolkit#1162, ADR-0043 (rpg-toolkit encounter#1163).
// Without one, EndTurn parks the clock on a monster forever: nothing can act
// for it, since EndTurn requires Member to be the active member and the host
// binds Member to the authenticated human, who does not own the monster.
//
// sdk.Minded(nil) is the production driver as of rpg-toolkit#1725: each
// member is driven by the mind ITS OWN SHEET NAMES (rule A5), and a member
// whose sheet names none gets exactly sdk.Behavior()'s answer -- attack the
// closest standing player if one is in reach, otherwise close the distance,
// otherwise pass. Only the skeleton names a mind today ("retaliator": it
// turns on whoever attacked it while the deed is fresh, and otherwise goes
// for the closest), so every other monster's turn is unchanged. Passing nil
// takes the rulebook's own Patience, the feel number the first walk tunes;
// this package never names it, because a host naming a rules number is the
// smell CLAUDE.md opens with. It wraps rulebooks/dnd5e/behavior entirely
// inside the toolkit; this package never imports encounter or behavior.
//
// A failure here is a WIRING fault, not a game outcome, so it is returned
// and New refuses to build -- a monster silently falling back to the basic
// driver would look like a design choice rather than a broken pin.
func newDefaultTurnDriver() (sdk.TurnDriver, error) {
	driver, err := sdk.Minded(nil)
	if err != nil {
		return nil, fmt.Errorf("construct default turn driver: %w", err)
	}

	return &serializedTurnDriver{driver: driver}, nil
}

// serializedTurnDriver serializes Act against one stateful driver.
//
// WHY THIS EXISTS, and it is not a nicety: sdk.Minded is STATEFUL and its own
// doc says so -- "one driver serves one encounter, one turn at a time; not
// safe for concurrent use". It remembers which mind each member was given,
// and it parks the view it is answering from on itself for the length of the
// call. This package has nowhere to hang one driver per encounter: the SDK
// takes TurnDriver once, on session.Config, and the Manager built from it
// serves EVERY session in the process. So the one driver is shared, and two
// sessions taking a monster's turn at the same moment would race its maps --
// a Go runtime panic, not a wrong answer.
//
// What sharing still costs after the lock, stated rather than waved away:
// the mind a member was assigned persists across sessions, keyed by the
// member id, and member ids are AUTHORED PER DUNGEON (sessionworld's
// Monster.MemberID) rather than minted per run. Two runs of the same dungeon
// therefore hand the same member the same mind, which is the answer its
// sheet names both times. It stops being harmless the day one id can mean
// two different sheets; the fix then is a driver per encounter, which needs
// a seam the SDK does not have yet (rpg-toolkit#1725 follow-up).
type serializedTurnDriver struct {
	mu     sync.Mutex
	driver sdk.TurnDriver
}

// compile-time proof the adapter satisfies what it is handed to.
var _ sdk.TurnDriver = (*serializedTurnDriver)(nil)

// Act takes one member's turn, one at a time.
func (d *serializedTurnDriver) Act(view sdk.MonsterView) (sdk.TurnIntent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.driver.Act(view)
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

	driver := cfg.TurnDriver
	if driver == nil {
		var err error
		if driver, err = newDefaultTurnDriver(); err != nil {
			return nil, fmt.Errorf("session orchestrator: %w", err)
		}
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
		TurnDriver:        driver,
	})
	if err != nil {
		return nil, fmt.Errorf("construct session manager: %w", err)
	}

	return &Orchestrator{Manager: mgr, Broker: broker}, nil
}
