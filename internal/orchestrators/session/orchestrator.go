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
	"time"

	"github.com/KirkDiggler/rpg-toolkit/dice"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/repositories/characterconfig"
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
	//
	// IT MUST BE THE REDIS CHARACTER REPOSITORY ON Config.Redis. The fenced
	// save writes a verb's seated sheets in the same Lua script as its world
	// (redis_fenced.go, rpg-project#879 S12a), under characterrepo.Key, so the
	// sheets the session writes land in Config.Redis whatever this field
	// holds. A different store would read sheets the session never wrote.
	Characters characterrepo.Repository

	// TTL is the per-key expiration applied to session and encounter Redis
	// keys. Pass 0 to disable expiration.
	TTL time.Duration

	// CharacterConfig is the character configuration store the SDK reads and
	// Preferences writes (rpg-project#753 R2). Optional: nil selects the
	// Redis store on Redis.
	CharacterConfig characterconfig.Repository

	// Locker coordinates full SDK operations. Nil selects process-local
	// exclusion keyed by session, and separately by character. All
	// handlers/lobby calls share it. Multiple Orchestrators/processes must
	// inject one shared coordinator; independent local lockers are insufficient.
	Locker sdk.SessionLocker

	// Continuations runs the part of a verb that follows its answer
	// (rpg-project#879 R2): a world-clock Move answers with the steps before
	// the one that pays a round and hands the rest to this runner with the
	// session's guard held. Optional: nil selects an
	// InProcessContinuationRunner, which Orchestrator.Drain waits on. A test
	// injects its own to observe or script the hand-off.
	Continuations sdk.ContinuationRunner

	// AnswerWhole hands the SDK no runner at all, so every verb runs whole
	// before it answers and the answer carries the verb's Saved and Delivery,
	// exactly as before rpg-project#879. New refuses it together with a
	// non-nil Continuations, which it would otherwise ignore. It is
	// for tests that read the store, or a report, right after a verb
	// returns; production leaves it false.
	AnswerWhole bool

	// RecordCapture is an optional host-owned diagnostic sink, independent of
	// browser subscriptions. Nil keeps broker-only delivery. The host drains
	// its capture after accepted continuations; New does not own its writer.
	RecordCapture sdk.RecordStream

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

// Orchestrator owns the toolkit session.Manager and the two brokers its
// streams subscribe against. All are exported for handlers to use directly:
// Manager for every verb, RecordBroker for StreamRecord's (v1alpha2,
// rpg-project#628), ActBroker for v1alpha2 StreamEvents' (rpg-project#630).
// The v1alpha1 event Broker went with the v1 stream (rpg-project#631).
type Orchestrator struct {
	Manager      *sdk.Manager
	RecordBroker *RecordBroker
	ActBroker    *ActBroker
	// Preferences writes character configuration under the character guard
	// of the same locker Manager uses.
	Preferences *Preferences

	// continuations is what Manager hands the rest of a verb to; nil when
	// Config.AnswerWhole is set.
	continuations sdk.ContinuationRunner
}

// continuationDrainer is a runner that can wait for its continuations in
// flight, as InProcessContinuationRunner does.
type continuationDrainer interface {
	Drain(ctx context.Context) error
}

// Drain waits for the continuations in flight -- verbs that answered before
// their save -- to finish, or for ctx to end, whichever comes first. A host
// calls it on shutdown, after the gRPC server stopped taking verbs, so no
// answered walk loses its save. It returns nil at once when the runner keeps
// nothing in flight (Config.AnswerWhole, or an injected runner without a
// Drain of its own), and ctx's error when the deadline wins.
func (o *Orchestrator) Drain(ctx context.Context) error {
	d, ok := o.continuations.(continuationDrainer)
	if !ok {
		return nil
	}
	return d.Drain(ctx)
}

// New constructs an Orchestrator, wiring every session.Config capability
// explicitly: Redis-backed SessionRepository, EncounterRepository and
// SeatRepository, the
// CharacterRepository adapter, session.DiscardEvents as the EventStream
// (rpg-project#631), and a crypto-secure dice.CryptoRoller as the SDK's Roller -- the host supplies
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

	// SESSIONS LOCK BY SESSION (rpg-project#753 R2): a verb holds its own
	// session's guard and nothing wider, so a slow verb stalls its own table
	// only. No session verb writes a record shared across sessions.
	locker := cfg.Locker
	if locker == nil {
		locker = NewInProcessSessionLocker()
	}
	// THE VERB ANSWERS BEFORE THE WORLD RUNS (rpg-project#879 R2): the
	// rest of a world-clock Move runs on the runner, under the session
	// guard it inherits. AnswerWhole leaves the SDK's own inline path.
	if cfg.AnswerWhole && cfg.Continuations != nil {
		return nil, errors.New("session orchestrator: Config.AnswerWhole and Config.Continuations are exclusive")
	}
	var continuations sdk.ContinuationRunner
	if !cfg.AnswerWhole {
		continuations = cfg.Continuations
		if continuations == nil {
			continuations = NewInProcessContinuationRunner()
		}
	}
	configs := cfg.CharacterConfig
	if configs == nil {
		var err error
		configs, err = characterconfig.NewRedis(&characterconfig.RedisConfig{Client: cfg.Redis})
		if err != nil {
			return nil, fmt.Errorf("character config: %w", err)
		}
	}

	recordBroker := NewRecordBroker(0)
	var recordStream sdk.RecordStream = recordBroker
	if cfg.RecordCapture != nil {
		recordStream = &recordPublisher{broker: recordBroker, capture: cfg.RecordCapture}
	}
	actBroker := NewActBroker(ActBufferFromEnv())
	mgr, err := sdk.NewManager(&sdk.Config{
		Locker:            locker,
		Continuations:     continuations,
		CharacterConfig:   &characterConfigReader{repo: configs},
		StaleTargetPolicy: policy,
		PresentationIDs:   presentationIDs,
		Sessions:          NewSessionRepository(cfg.Redis, cfg.TTL),
		Encounters:        NewEncounterRepository(cfg.Redis, cfg.TTL),
		Characters:        NewCharacterRepository(cfg.Characters),
		Seats:             NewSeatRepository(cfg.Redis),
		Record:            recordStream,
		Acts:              actBroker,
		Dice:              roller,
		TurnDriver:        driverForSDK,
	})
	if err != nil {
		return nil, fmt.Errorf("construct session manager: %w", err)
	}

	return &Orchestrator{
		Manager: mgr, RecordBroker: recordBroker, ActBroker: actBroker,
		Preferences:   &Preferences{locker: locker, repo: configs},
		continuations: continuations,
	}, nil
}
