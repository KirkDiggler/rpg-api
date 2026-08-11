package encounter_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// MoveEntity concurrency test (rpg-api#787). This is the headline proof for
// the fix: the per-encounter keyedMutex (encounterLocks) closes the
// unguarded read-modify-write race that let a fast concurrent free-roam move
// silently erase a slower one's already-applied move — confirmed live
// (issue #787's linked repro): the mover's own client believed it had moved
// to its destination; the server's persisted state disagreed, permanently,
// until a resnapshot.

const (
	mcEncID     = "enc-move-concurrent"
	mcPlayerAmy = "amy-mc"
	mcEntityAmy = "char-amy-mc"
	mcPlayerBob = "bob-mc"
	mcEntityBob = "char-bob-mc"
)

// delayedGetRepo wraps a Repository and sleeps briefly after each Get
// returns from the underlying store, widening the window a mutating verb's
// load phase opens between reading the snapshot and persisting the result.
// This deterministically reproduces the #787 lost-update race without a
// hard mutual barrier: two concurrent callers whose Get calls land within
// the delay window are guaranteed to both read the same pre-mutation
// snapshot before either has a chance to Save.
//
// This cannot deadlock the FIXED (locked) code: encounterLocks.Lock is taken
// as the first line of every mutating verb, before load() ever reaches Get.
// A second caller blocked at Lock() has not called Get at all yet — its
// (delayed) Get only runs after the first caller's entire
// load->verb->persist span, delay included, has already completed and
// released the lock. The delay is a one-way tax on the now-serialized case,
// not a mutual wait between the two callers' Get calls — so nothing can
// deadlock either way, unlike a "wait for both Gets" barrier would (which
// would hang forever once the lock prevents the second Get from ever being
// reached concurrently).
type delayedGetRepo struct {
	encountersv2.Repository
	delay time.Duration
}

func (r *delayedGetRepo) Get(ctx context.Context, id string) (*tkenc.Data, error) {
	data, err := r.Repository.Get(ctx, id)
	time.Sleep(r.delay)
	return data, err
}

type MoveEntityConcurrencySuite struct {
	suite.Suite

	ctx      context.Context
	broker   *tkenc.Broker
	baseRepo encountersv2.Repository // undelayed — used for seeding/final assertions
	repo     encountersv2.Repository // delayedGetRepo wrapping baseRepo; wired into the orchestrator
	orch     *encounterorch.Orchestrator
}

func (s *MoveEntityConcurrencySuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.baseRepo = encountersv2.NewInMemory()
	s.repo = &delayedGetRepo{Repository: s.baseRepo, delay: 20 * time.Millisecond}

	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:                 s.broker,
		EncounterRepo:          s.repo,
		BuildCharacterResolver: constCharacterResolver(stubCharacterResolver{}),
		BuildCombatResolver: func(_ *tkenc.Data) encounterorch.CombatResolver {
			return nil
		},
		// nil movement resolver: the toolkit Move takes the legacy single-jump
		// branch (no per-step chain, no OAs) — isolates the load -> Move ->
		// persist race from rulebook combat math, matching MoveEntitySuite's
		// own choice in move_entity_test.go.
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)
	s.orch = orch
}

// seedTwoMovers persists a free-roam encounter with two players far enough
// apart (distance 10 hexes, well outside either's sight range) that their
// paths never share a hex and neither sights the other — no occupancy
// interaction or combat-entry side effect to confound the race assertion.
func (s *MoveEntityConcurrencySuite) seedTwoMovers(encID string) {
	enc := tkenc.New(s.ctx, core.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   mcPlayerAmy,
		EntityID:   mcEntityAmy,
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 4,
		HP:         14,
		MaxHP:      14,
		AC:         14,
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   mcPlayerBob,
		EntityID:   mcEntityBob,
		Position:   core.Hex{Q: 10, R: 0, S: -10},
		SightRange: 4,
		HP:         14,
		MaxHP:      14,
		AC:         14,
	}))
	s.Require().NoError(s.baseRepo.Save(s.ctx, enc.ToData()))
}

// TestMoveEntity_ConcurrentDifferentPlayers_BothMovesSurvive is the rpg-api#787
// headline proof: two players moving concurrently in the SAME free-roam
// encounter must both persist, not just whichever caller's Save lands last.
//
// Without encounterLocks, MoveEntity is load -> mutate a private in-memory
// copy -> save the whole snapshot: both goroutines' delayedGetRepo.Get calls
// return within the artificial delay window, well before either has reached
// Save, so both read the SAME pre-move snapshot. Whichever Save commits
// second overwrites the first mover's already-applied position change with
// its own stale copy of it — one of the two position assertions below fails,
// reproducing the live desync from the issue.
//
// With encounterLocks (the fix), the second caller blocks at Lock() before
// ever reaching Get, so its load necessarily observes the first caller's
// already-persisted move — both survive.
func (s *MoveEntityConcurrencySuite) TestMoveEntity_ConcurrentDifferentPlayers_BothMovesSurvive() {
	s.seedTwoMovers(mcEncID)

	amyDest := core.Hex{Q: 1, R: -1, S: 0}
	bobDest := core.Hex{Q: 11, R: -1, S: -10}

	var wg sync.WaitGroup
	var errAmy, errBob error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errAmy = s.orch.MoveEntity(s.ctx, &encounterorch.MoveEntityInput{
			EncounterID: mcEncID,
			PlayerID:    mcPlayerAmy,
			EntityID:    core.EntityID(mcEntityAmy),
			Path: []core.Hex{
				{Q: 0, R: 0, S: 0},
				amyDest,
			},
		})
	}()
	go func() {
		defer wg.Done()
		_, errBob = s.orch.MoveEntity(s.ctx, &encounterorch.MoveEntityInput{
			EncounterID: mcEncID,
			PlayerID:    mcPlayerBob,
			EntityID:    core.EntityID(mcEntityBob),
			Path: []core.Hex{
				{Q: 10, R: 0, S: -10},
				bobDest,
			},
		})
	}()
	wg.Wait()

	s.Require().NoError(errAmy, "amy's concurrent move must not error")
	s.Require().NoError(errBob, "bob's concurrent move must not error")

	loaded, err := s.baseRepo.Get(s.ctx, mcEncID)
	s.Require().NoError(err)
	s.Equal(amyDest, loaded.Players[mcPlayerAmy].View.Position,
		"amy's move must survive a concurrent move by a different player")
	s.Equal(bobDest, loaded.Players[mcPlayerBob].View.Position,
		"bob's move must survive a concurrent move by a different player")
}

func TestMoveEntityConcurrencySuite(t *testing.T) {
	suite.Run(t, new(MoveEntityConcurrencySuite))
}
