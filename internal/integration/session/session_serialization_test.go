package session_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// Pause the first encounter save, after its Move has read/acted but before the
// persisted world changes. A second operation must not read that old world.
type pausedEncounterSave struct {
	key     string
	entered chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (h *pausedEncounterSave) DialHook(next goredis.DialHook) goredis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h *pausedEncounterSave) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func (h *pausedEncounterSave) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if cmd.Name() == "set" && len(cmd.Args()) > 1 && cmd.Args()[1] == h.key {
			h.once.Do(func() {
				close(h.entered)
				select {
				case <-h.resume:
				case <-ctx.Done():
				}
			})
		}
		return next(ctx, cmd)
	}
}

type SessionSerializationSuite struct{ suite.Suite }

func TestSessionSerializationSuite(t *testing.T) { suite.Run(t, new(SessionSerializationSuite)) }

func (s *SessionSerializationSuite) TestSameSessionReadsAndMovesWaitForCommittedState() {
	s.proveSerialization(nil)
}

func (s *SessionSerializationSuite) TestManagersSharingACoordinatorReadEachOthersCommittedState() {
	s.proveSerialization(sessionorch.NewInProcessSessionLocker())
}

func (s *SessionSerializationSuite) proveSerialization(locker sdk.SessionLocker) {
	h := newAcceptanceHarness(s.T())
	other := h.manager.Manager
	if locker != nil {
		cfg := sessionorch.Config{Redis: h.redis, Characters: h.charRepo, Dice: testDice{}, Locker: locker}
		first, err := sessionorch.New(cfg)
		s.Require().NoError(err)
		second, err := sessionorch.New(cfg)
		s.Require().NoError(err)
		h.manager = first
		other = second.Manager
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	s.Require().NoError(err)
	// A launch stores the run's world under the run's own id.
	const run = "serialized-run"
	h.launch(s.T(), run, buildThreeRoomTomb(s.T()), seatAt("alice", 1, 1))

	hook := &pausedEncounterSave{
		key: "session-enc:v1alpha1:" + run, entered: make(chan struct{}), resume: make(chan struct{}),
	}
	var resumeOnce sync.Once
	resume := func() { resumeOnce.Do(func() { close(hook.resume) }) }
	defer resume()
	h.redis.AddHook(hook)

	first := make(chan error, 1)
	go func() {
		_, moveErr := h.manager.Manager.Move(ctx, &sdk.MoveInput{
			Session: run, Member: "alice", Path: []spatial.Position{at(2, 1)},
		})
		first <- moveErr
	}()
	select {
	case <-hook.entered:
	case early := <-first:
		s.FailNow("move returned before its encounter save", "%v", early)
	case <-ctx.Done():
		s.FailNow("move never reached its encounter save")
	}

	waiting, stopWaiting := context.WithTimeout(ctx, 100*time.Millisecond)
	defer stopWaiting()
	_, err = other.Where(waiting, &sdk.WhereInput{Session: run, Member: "alice"})
	s.ErrorIs(err, context.DeadlineExceeded, "a read must wait, not report the pre-move position")

	second := make(chan error, 1)
	go func() {
		_, moveErr := other.Move(ctx, &sdk.MoveInput{
			Session: run, Member: "alice", Path: []spatial.Position{at(3, 1)},
		})
		second <- moveErr
	}()
	// With the old unguarded host this returns a broken-path refusal: (3,1)
	// is not adjacent to the OLD (1,1). Under exclusion it waits for (2,1).
	select {
	case early := <-second:
		s.Fail("second move ran before the first committed", "%v", early)
		second <- early
	case <-time.After(100 * time.Millisecond):
	}
	resume()
	for _, completed := range []<-chan error{first, second} {
		select {
		case moveErr := <-completed:
			s.Require().NoError(moveErr)
		case <-ctx.Done():
			s.FailNow("guard was not released")
		}
	}
	where, err := h.manager.Manager.Where(ctx, &sdk.WhereInput{Session: run, Member: "alice"})
	s.Require().NoError(err)
	s.Equal(at(3, 1), where.Position)
}
