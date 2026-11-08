package session

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type ContinuationRunnerSuite struct {
	suite.Suite
	runner *InProcessContinuationRunner
	logs   *lockedBuffer
}

func TestContinuationRunnerSuite(t *testing.T) { suite.Run(t, new(ContinuationRunnerSuite)) }

func (s *ContinuationRunnerSuite) SetupTest() {
	s.logs = &lockedBuffer{}
	s.runner = NewInProcessContinuationRunner()
	s.runner.logger = slog.New(slog.NewTextHandler(s.logs, nil))
}

type requestKey struct{}

// The request's cancel never reaches the rest of the verb: the save has to
// land after the answer went, and the request's values still ride along.
func (s *ContinuationRunnerSuite) TestContinueOutlivesTheRequestsCancel() {
	request, endRequest := context.WithCancel(context.WithValue(context.Background(), requestKey{}, "req-1"))
	proceed := make(chan struct{})
	seen := make(chan context.Context, 1)
	err := s.runner.Run(request, &sdk.RunContinuationInput{
		Session: "sess-1", Verb: sdk.VerbMove,
		Continue: func(ctx context.Context) (*sdk.ContinuationReport, error) {
			<-proceed
			seen <- ctx
			return &sdk.ContinuationReport{}, nil
		},
	})
	s.Require().NoError(err)

	endRequest()
	close(proceed)
	ctx := <-seen
	s.NoError(ctx.Err(), "the continuation's context is not ended by the request's")
	s.Equal("req-1", ctx.Value(requestKey{}))
	s.NoError(s.runner.Drain(context.Background()))
}

// Run answers at once: the continuation runs beside the answer, not before it.
func (s *ContinuationRunnerSuite) TestRunReturnsBeforeTheContinuationEnds() {
	proceed := make(chan struct{})
	ran := make(chan struct{})
	s.Require().NoError(s.runner.Run(context.Background(), &sdk.RunContinuationInput{
		Session: "sess-1", Verb: sdk.VerbMove,
		Continue: func(context.Context) (*sdk.ContinuationReport, error) {
			<-proceed
			close(ran)
			return &sdk.ContinuationReport{}, nil
		},
	}))
	close(proceed)
	<-ran
	s.NoError(s.runner.Drain(context.Background()))
}

// Drain waits for a continuation still in flight, and gives up at its deadline
// without stopping it.
func (s *ContinuationRunnerSuite) TestDrainWaitsForInFlightAndReturnsOnDeadline() {
	proceed := make(chan struct{})
	var finished bool
	var mu sync.Mutex
	s.Require().NoError(s.runner.Run(context.Background(), &sdk.RunContinuationInput{
		Session: "sess-1", Verb: sdk.VerbMove,
		Continue: func(context.Context) (*sdk.ContinuationReport, error) {
			<-proceed
			mu.Lock()
			finished = true
			mu.Unlock()
			return &sdk.ContinuationReport{}, nil
		},
	}))

	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	s.ErrorIs(s.runner.Drain(short), context.DeadlineExceeded, "still in flight at the deadline")

	close(proceed)
	s.NoError(s.runner.Drain(context.Background()))
	mu.Lock()
	defer mu.Unlock()
	s.True(finished, "Drain returned only after the continuation ended")
}

// Nothing in flight: Drain returns at once.
func (s *ContinuationRunnerSuite) TestDrainWithNothingInFlight() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.NoError(s.runner.Drain(ctx))
	var zero InProcessContinuationRunner
	s.NoError(zero.Drain(ctx), "the zero value is usable")
}

// A failed rest cannot be told to the client; it is logged with the session
// and the verb, and nothing panics.
func (s *ContinuationRunnerSuite) TestFailedContinueIsLoggedNotPanicked() {
	s.Require().NoError(s.runner.Run(context.Background(), &sdk.RunContinuationInput{
		Session: "sess-9", Verb: sdk.VerbMove,
		Continue: func(context.Context) (*sdk.ContinuationReport, error) {
			return nil, errors.New("save refused")
		},
	}))
	s.Require().NoError(s.runner.Drain(context.Background()))
	out := s.logs.String()
	s.Contains(out, "session continuation failed after its answer")
	s.Contains(out, "session=sess-9")
	s.Contains(out, "verb=move")
	s.Contains(out, "save refused")
}

// The SDK never sends an input without Continue; a runner handed one refuses
// it, so the verb releases its own guard and saves nothing.
func (s *ContinuationRunnerSuite) TestRunRefusesAnInputWithoutContinue() {
	s.ErrorIs(s.runner.Run(context.Background(), nil), sdk.ErrNilInput)
	s.ErrorIs(s.runner.Run(context.Background(), &sdk.RunContinuationInput{Session: "sess-1"}), sdk.ErrNilInput)
	s.NoError(s.runner.Drain(context.Background()), "a refused input is not counted in flight")
}

// lockedBuffer is a log sink safe to write from the continuation's goroutine.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
