package session_test

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// A MOVE ANSWERS BEFORE THE WORLD RUNS (rpg-project#879 R2), through the
// production wiring: a free-roam walk longer than one pace answers with the
// steps before the one that pays a round, and the rest -- that step, the
// steps after, the save and the delivery -- runs on the orchestrator's
// continuation runner. Once the runner drains, the store holds the whole
// walk: a fresh orchestrator over the same Redis finds alice at the route's
// end, and the record carries every step the answer did not.
func TestAcceptance_AMoveAnswersBeforeTheWorldRuns(t *testing.T) {
	h := newAcceptanceHarnessAnswering(t, testDice{}, nil, false)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	const run = "continuation-run"

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	// The tomb with nobody in it: nothing forms a fight, so the whole route
	// is free roam on the world clock and a 30 ft walker pays a round every
	// six cells.
	h.launch(t, run, buildThreeRoomTomb(t), seatAt("alice", 1, 1))

	route := tombRoute()
	answer, err := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: route})
	require.NoError(t, err)

	require.True(t, answer.GetContinues(), "a route longer than a pace answers before the round is paid")
	require.Equal(t, sessionpb.MovementStatus_MOVEMENT_STATUS_CONTINUING, answer.GetStatus())
	answered := len(answer.GetSteps())
	require.Positive(t, answered, "the steps before the paying one are in the answer")
	require.Less(t, answered, len(route), "the rest of the walk is not")
	require.NotEmpty(t, answer.GetThrough(), "the answer names where the stream continues")
	require.Empty(t, answer.GetSaved().GetWritten(), "nothing is saved before the rest has run")
	require.Nil(t, answer.GetFormed())

	drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, h.manager.Drain(drain), "the continuation finishes")

	// A fresh orchestrator reads only what was stored.
	fresh, err := sessionorch.New(sessionorch.Config{
		Redis: h.redis, Characters: h.charRepo, Dice: testDice{}, AnswerWhole: true,
	})
	require.NoError(t, err)
	end := route[len(route)-1]
	where, err := fresh.Manager.Where(context.Background(), &sdk.WhereInput{Session: run, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, spatial.Position{X: float64(end.GetX()), Y: float64(end.GetY())}, where.Position,
		"the stored walk reached the route's end")

	rest, err := fresh.Manager.Record(context.Background(), &sdk.RecordReadInput{Session: run, AfterID: answer.GetThrough()})
	require.NoError(t, err)
	moved := 0
	for _, e := range rest.Events {
		if e.Beat == "moved" {
			moved++
		}
	}
	require.Equal(t, len(route)-answered, moved, "every step after the answer's is on the record after Through")
}

// A PANIC IN THE REST OF A WALK DOES NOT TAKE THE SERVER DOWN. Before
// rpg-project#879 the raise and the save ran on the request goroutine, where
// the recovery interceptor turned a panic into one caller's Internal error;
// now they run on the runner's goroutine, which recovers it itself. Through
// the real SDK: the continuation panics reading alice's sheet, the process
// lives, the SDK's deferred release gives the session guard back (the next
// verb on the session does not hang), and nothing was saved (alice stands
// where she launched).
func TestAcceptance_APanicInTheContinuationLeavesTheSessionUsable(t *testing.T) {
	logs := &syncBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	h := newAcceptanceHarness(t)
	sheets := &panickingSheets{Repository: h.charRepo}
	gate := &heldContinuations{inner: sessionorch.NewInProcessContinuationRunner()}
	orch, err := sessionorch.New(sessionorch.Config{
		Redis: h.redis, Characters: sheets, Dice: testDice{}, Continuations: gate,
	})
	require.NoError(t, err)
	const run = "panic-run"

	_, err = h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)
	_, err = orch.Manager.Launch(context.Background(), launchInput(run, buildThreeRoomTomb(t), []partySeat{seatAt("alice", 1, 1)}))
	require.NoError(t, err)

	answer, err := orch.Manager.Move(context.Background(), &sdk.MoveInput{
		Session: run, Member: "alice", Path: positionsOf(tombRoute()),
	})
	require.NoError(t, err)
	require.True(t, answer.Continues)

	// From here every sheet read panics, and the rest of the walk reads one.
	sheets.panicking.Store(true)
	gate.release(t)
	drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, orch.Drain(drain), "the panicked continuation is counted done")
	sheets.panicking.Store(false)

	require.Contains(t, logs.String(), "session continuation panicked after its answer")
	require.Contains(t, logs.String(), "session="+run)

	// The guard came back: the next verb on the session does not wait forever.
	next, cancelNext := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelNext()
	where, err := orch.Manager.Where(next, &sdk.WhereInput{Session: run, Member: "alice"})
	require.NoError(t, err, "the session's guard was given back")
	require.Equal(t, spatial.Position{X: 1, Y: 1}, where.Position, "nothing of the walk was saved")
}

func positionsOf(path []*sessionpb.Position) []spatial.Position {
	out := make([]spatial.Position, len(path))
	for i, p := range path {
		out[i] = spatial.Position{X: float64(p.GetX()), Y: float64(p.GetY())}
	}
	return out
}

// panickingSheets is the character store with a switch that makes every Get
// panic: a stand-in for any panic in the rest of a walk.
type panickingSheets struct {
	characterrepo.Repository
	panicking atomic.Bool
}

func (p *panickingSheets) Get(ctx context.Context, in characterrepo.GetInput) (*characterrepo.GetOutput, error) {
	if p.panicking.Load() {
		panic("sheet read panicked")
	}
	return p.Repository.Get(ctx, in)
}

// heldContinuations holds one continuation until the test releases it to the
// production runner, so the test can arrange the panic after the answer.
type heldContinuations struct {
	inner *sessionorch.InProcessContinuationRunner
	mu    sync.Mutex
	held  *sdk.RunContinuationInput
	ctx   context.Context
}

func (g *heldContinuations) Run(ctx context.Context, in *sdk.RunContinuationInput) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ctx, g.held = ctx, in
	return nil
}

func (g *heldContinuations) release(t *testing.T) {
	t.Helper()
	g.mu.Lock()
	ctx, in := g.ctx, g.held
	g.mu.Unlock()
	require.NotNil(t, in, "the walk handed off a continuation")
	require.NoError(t, g.inner.Run(ctx, in))
}

// Drain lets Orchestrator.Drain reach the production runner.
func (g *heldContinuations) Drain(ctx context.Context) error { return g.inner.Drain(ctx) }

// syncBuffer is a log sink safe to write from the runner's goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
