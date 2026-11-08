package session

import (
	"context"
	"log/slog"
	"sync"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// InProcessContinuationRunner runs the part of a verb that follows its answer
// (sdk.ContinuationRunner): for a world-clock Move, the step that pays a
// round, the raise inside it, the remaining steps, the save and the delivery.
//
// THIS IS THE WRITE TIER OF rpg-project#879 R3: the verb's writes leave the
// answer's path. Run starts the continuation on its own goroutine under
// context.WithoutCancel(ctx), so the client's cancel or a finished response
// never ends the save, and returns at once so the handler can answer.
//
// ORDER IS NOT KEPT HERE. The continuation holds the session's guard (the
// SDK hands it over still held, and Continue gives it back), so the next verb
// on that session waits on the locker until the world has finished. This
// runner only starts the work and counts it; it queues nothing.
//
// A FAILED CONTINUE CANNOT BE TOLD TO THE CLIENT: the answer has already
// gone. A failed continuation persists nothing, the answered steps included,
// so it is logged with the session, the verb and the error, and the client
// learns the stored state by reading it again.
//
// Run cannot panic once it has started the goroutine: it only returns. A
// runner that panicked after spawning Continue would give the session guard
// back while the rest was still running.
//
// Drain waits for the continuations in flight, for a graceful shutdown that
// must not drop a walk's save. The zero value is usable.
type InProcessContinuationRunner struct {
	inFlight sync.WaitGroup
	logger   *slog.Logger
}

var _ sdk.ContinuationRunner = (*InProcessContinuationRunner)(nil)

// NewInProcessContinuationRunner constructs a process-local runner that logs
// failed continuations to slog's default logger.
func NewInProcessContinuationRunner() *InProcessContinuationRunner {
	return &InProcessContinuationRunner{}
}

// Run starts in.Continue on its own goroutine with a context the request's
// cancel cannot end, and returns nil: the continuation will run. It refuses
// only a nil input or one without Continue, which the SDK never sends.
func (r *InProcessContinuationRunner) Run(ctx context.Context, in *sdk.RunContinuationInput) error {
	if in == nil || in.Continue == nil {
		return sdk.ErrNilInput
	}
	detached := context.WithoutCancel(ctx)
	r.inFlight.Add(1)
	go func() {
		defer r.inFlight.Done()
		if _, err := in.Continue(detached); err != nil {
			r.log().ErrorContext(detached, "session continuation failed after its answer",
				slog.String("session", in.Session),
				slog.String("verb", string(in.Verb)),
				slog.Any("error", err))
		}
	}()
	return nil
}

// Drain waits until every continuation started so far has ended, or ctx is
// done. It returns ctx's error when the deadline wins; the continuations
// still running are not stopped.
func (r *InProcessContinuationRunner) Drain(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		r.inFlight.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *InProcessContinuationRunner) log() *slog.Logger {
	if r.logger != nil {
		return r.logger
	}
	return slog.Default()
}
