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

	// TurnDriver decides what a member with no player does when it is given
	// time -- its turn in a fight, or a round of the world clock. Optional:
	// nil selects the production wiring, sdk.Driver(). Dice's own doc explains
	// the shape and it applies verbatim here -- what New hands the SDK is
	// always explicit, and the override exists so a test can script a
	// monster's turn without reaching past this package into the SDK's
	// construction.
	//
	// EITHER WAY IT IS ONE DRIVER FOR EVERY SESSION, which is the difference
	// from the pair this replaced (rpg-project#465). sdk.Minded(nil) was
	// stateful -- it remembered which Go preset each member's SHEET named --
	// so it needed one instance per session and a cache here to hand them out.
	// A creature's policy is now the table an author wrote for it, held with
	// the member in the world rather than in the driver, so sdk.Driver() holds
	// nothing between turns and one value serves every session this Manager
	// serves. The cache went with the state that justified it.
	//
	// The reason a test needs the override: the table drives a creature from
	// what its author wrote, so a test that wants a specific turn out of a
	// specific monster scripts the driver rather than authoring a dungeon.
	TurnDriver sdk.TurnDriver
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

	// ONE DRIVER, THE EVERY-SESSION DOOR (rpg-project#465). sdk.Driver() is
	// the creature's own authored table, rolled through the session's shared
	// dice, and it holds nothing between turns -- so there is nothing for a
	// per-session cache to own and sdk.Config.TurnDrivers stays unwired. A
	// test's override lands in the same field, because a scripted driver is
	// what that test is asserting about.
	driverForSDK := cfg.TurnDriver
	if driverForSDK == nil {
		driverForSDK = sdk.Driver()
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
	})
	if err != nil {
		return nil, fmt.Errorf("construct session manager: %w", err)
	}

	return &Orchestrator{Manager: mgr, Broker: broker}, nil
}
