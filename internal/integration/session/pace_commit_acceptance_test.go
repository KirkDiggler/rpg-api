package session_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// THE CONTINUATION COMMITS AT EACH PACE BOUNDARY (rpg-project#879 R7, S13),
// through the production wiring over Redis: a walk whose rest pays one world
// round lands two fenced saves, the round's and the final one, and the
// walker's listener receives the round's tick in its own unit while the walk
// is still stepping. The store ends holding the whole walk.
func TestAcceptance_AWalkCommitsAtItsPaceBoundary(t *testing.T) {
	h := newAcceptanceHarness(t)
	sheets := &timedSheets{Repository: h.charRepo}
	orch, err := sessionorch.New(sessionorch.Config{
		Redis: h.redis, Characters: sheets, Dice: testDice{},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, orch.Drain(drain))
	})
	const run = "pace-commit-run"

	_, err = h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)
	_, err = orch.Manager.Launch(context.Background(), launchInput(run, buildThreeRoomTomb(t), []partySeat{seatAt("alice", 1, 1)}))
	require.NoError(t, err)

	sub, err := orch.ActBroker.Subscribe(run, "alice")
	require.NoError(t, err)
	type unit struct {
		at   time.Time
		tick bool
	}
	var (
		mu    sync.Mutex
		units []unit
	)
	heard := make(chan struct{})
	go func() {
		defer close(heard)
		for u := range sub.Units() {
			got := unit{at: time.Now()}
			for _, a := range u {
				if c, ok := a.Act.Body.(sdk.ClockEntry); ok && c.Mark == sdk.ClockTick {
					got.tick = true
				}
			}
			mu.Lock()
			units = append(units, got)
			mu.Unlock()
		}
	}()

	// Nine cells: a 30 ft walker pays a round every six, so the answer
	// stops before the sixth and the rest pays one round with cells after it.
	route := positionsOf(tombRoute()[:9])
	saves := &saveBytes{}
	h.redis.AddHook(saves)
	_, _ = saves.take()
	answer, err := orch.Manager.Move(context.Background(), &sdk.MoveInput{Session: run, Member: "alice", Path: route})
	require.NoError(t, err)
	require.True(t, answer.Continues)
	_, atAnswer := saves.take()
	require.Zero(t, atAnswer, "nothing is saved at the answer")

	drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, orch.Drain(drain), "the continuation finishes")
	_, landed := saves.take()
	require.NoError(t, sub.Close())
	<-heard

	rest, err := orch.Manager.Record(context.Background(), &sdk.RecordReadInput{Session: run, AfterID: answer.Through})
	require.NoError(t, err)
	ticks := 0
	for _, e := range rest.Events {
		if e.Beat == "tick" {
			ticks++
		}
	}
	require.Equal(t, 1, ticks, "control: the rest pays exactly one round")
	require.Equal(t, 2, landed, "one fenced save at the round and the final one")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, units, 2, "the listener hears each commit as its own unit")
	require.True(t, units[0].tick, "the first unit carries the round")
	require.False(t, units[1].tick, "the second carries only the remaining steps")
	last := sheets.last()
	require.False(t, last.IsZero(), "the walk read the walker's sheet after the boundary")
	require.True(t, units[0].at.Before(last),
		"the listener heard the round while the walk was still stepping")

	end := route[len(route)-1]
	where, err := orch.Manager.Where(context.Background(), &sdk.WhereInput{Session: run, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, spatial.Position{X: end.X, Y: end.Y}, where.Position, "the store holds the whole walk")
}

// timedSheets is the character store, stamping every read: a continued
// walk reads the walker's sheet before each step it takes.
type timedSheets struct {
	characterrepo.Repository
	mu    sync.Mutex
	reads []time.Time
}

func (s *timedSheets) Get(ctx context.Context, in characterrepo.GetInput) (*characterrepo.GetOutput, error) {
	s.mu.Lock()
	s.reads = append(s.reads, time.Now())
	s.mu.Unlock()
	return s.Repository.Get(ctx, in)
}

func (s *timedSheets) last() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reads) == 0 {
		return time.Time{}
	}
	return s.reads[len(s.reads)-1]
}
