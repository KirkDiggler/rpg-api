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
	"time"

	"github.com/KirkDiggler/rpg-toolkit/dice"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// Config carries what New needs to build an Orchestrator. Every field is
// required: the SDK's own construction law (S8, "construction is total") is
// honored one level up here too, rather than letting a missing dependency
// surface later as a nil-pointer panic mid-verb.
type Config struct {
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

	broker := NewBroker()
	mgr, err := sdk.NewManager(&sdk.Config{
		Sessions:   NewSessionRepository(cfg.Redis, cfg.TTL),
		Encounters: NewEncounterRepository(cfg.Redis, cfg.TTL),
		Characters: NewCharacterRepository(cfg.Characters),
		Events:     broker,
		Dice:       roller,
	})
	if err != nil {
		return nil, fmt.Errorf("construct session manager: %w", err)
	}

	return &Orchestrator{Manager: mgr, Broker: broker}, nil
}
