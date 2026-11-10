package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// ErrContinuationsDraining is Run's refusal once Drain has started: the host
// is shutting down and a continuation admitted now could be cut off by the
// process exit mid-save. Refused, the SDK gives the guard back and fails the
// verb with nothing saved, so the client hears an error rather than an answer
// whose rest never lands.
var ErrContinuationsDraining = errors.New("session continuations are draining for shutdown")

// InProcessContinuationRunner runs the part of a verb that follows its answer
// (sdk.ContinuationRunner): for a world-clock Move, the step that pays a
// round, the raise inside it, the remaining steps, the save and the delivery.
//
// THIS IS THE WRITE TIER OF rpg-project#879 R3: the verb's writes leave the
// answer's path. Run starts the continuation on its own goroutine under
// context.WithoutCancel(ctx), so the client's cancel or a finished response
// never ends the save, and returns at once so the handler can answer. Run
// cannot panic once it has started the goroutine: it only returns. A runner
// that panicked after spawning Continue would give the session guard back
// while the rest was still running.
//
// ORDER IS NOT KEPT HERE. The continuation holds the session's guard (the
// SDK hands it over still held, and Continue gives it back), so the next verb
// on that session waits on the locker until the world has finished. This
// runner only starts the work and counts it; it queues nothing.
//
// WHAT GOES WRONG AFTER THE ANSWER CANNOT BE TOLD TO THE CLIENT YET: the
// answer has already gone, and the wire has no signal for it (rpg-api#1198).
// So it is logged, with the session and the verb:
//   - A walk's rest commits at each pace boundary (rpg-project#879 R7): each
//     round's raise is saved and delivered before the walk's next step. A
//     failed Continue persists nothing after its last landed commit, and
//     nothing retries it. With no commit landed, nothing was saved, the
//     answered steps included, and the client must re-read the session.
//     With commits landed, the report beside the error names how many and
//     the newest record id they saved: the store and every client stop
//     there, and only the steps after it never happened. Logged as "saved
//     through its last pace commit", with the id in saved_through.
//   - A panic inside Continue is recovered on the goroutine, with its stack,
//     so one walk's panic never takes the server down. The SDK's own deferred
//     release has already given the guard back as the panic unwound, and
//     nothing was saved after its last pace commit, if any: the session is
//     as that commit left it (or as it was before the Move), and the client
//     must re-read.
//   - A report whose Saved.Failed or Delivery.Failed is not empty: part of the
//     rest was not stored, or did not reach the stream.
//
// Drain waits for the continuations in flight, for a graceful shutdown that
// must not drop a walk's save. Once Drain has started, Run refuses with
// ErrContinuationsDraining. The zero value is usable.
type InProcessContinuationRunner struct {
	mu       sync.Mutex
	draining bool
	inFlight int
	idle     chan struct{} // closed when inFlight reaches zero; nil while none ran
	logger   *slog.Logger
}

var _ sdk.ContinuationRunner = (*InProcessContinuationRunner)(nil)

// NewInProcessContinuationRunner constructs a process-local runner that logs
// to slog's default logger.
func NewInProcessContinuationRunner() *InProcessContinuationRunner {
	return &InProcessContinuationRunner{}
}

// Run starts in.Continue on its own goroutine with a context the request's
// cancel cannot end, and returns nil: the continuation will run. It refuses
// a nil input or one without Continue (which the SDK never sends) and, once
// Drain has started, every input.
func (r *InProcessContinuationRunner) Run(ctx context.Context, in *sdk.RunContinuationInput) error {
	if in == nil || in.Continue == nil {
		return sdk.ErrNilInput
	}
	if err := r.admit(); err != nil {
		return err
	}
	detached := context.WithoutCancel(ctx)
	go r.continueVerb(detached, in)
	return nil
}

// continueVerb runs one continuation to its end and reports what went wrong.
func (r *InProcessContinuationRunner) continueVerb(ctx context.Context, in *sdk.RunContinuationInput) {
	defer r.done()
	attrs := []any{slog.String("session", in.Session), slog.String("verb", string(in.Verb))}
	defer func() {
		if p := recover(); p != nil {
			r.log().ErrorContext(ctx,
				"session continuation panicked after its answer; nothing saved after its last pace commit, if any, the client must re-read",
				append(attrs, slog.String("panic", fmt.Sprint(p)), slog.String("stack", string(debug.Stack())))...)
		}
	}()
	report, err := in.Continue(ctx)
	if err != nil {
		if report != nil && report.Commits > 0 {
			r.log().ErrorContext(ctx, "session continuation failed after its answer; saved through its last pace commit, the steps after it never happened",
				append(attrs,
					slog.Int("commits_landed", report.Commits),
					slog.String("saved_through", report.SavedThrough),
					slog.Any("error", err))...)
			return
		}
		r.log().ErrorContext(ctx, "session continuation failed after its answer; nothing saved, the client must re-read",
			append(attrs, slog.Any("error", err))...)
		return
	}
	if report == nil {
		return
	}
	if len(report.Saved.Failed) > 0 || len(report.Delivery.Failed) > 0 {
		r.log().WarnContext(ctx, "session continuation ended degraded after its answer",
			append(attrs,
				slog.Any("saved_written", report.Saved.Written),
				slog.Any("saved_failed", report.Saved.Failed),
				slog.Any("delivery_failed", report.Delivery.Failed))...)
	}
}

// admit counts a continuation in flight, or refuses it while draining.
func (r *InProcessContinuationRunner) admit() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.draining {
		return ErrContinuationsDraining
	}
	if r.inFlight == 0 {
		r.idle = make(chan struct{})
	}
	r.inFlight++
	return nil
}

// done counts a continuation finished.
func (r *InProcessContinuationRunner) done() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inFlight--
	if r.inFlight == 0 {
		close(r.idle)
	}
}

// Drain refuses new continuations from now on and waits until every one
// already admitted has ended, or ctx is done. It returns ctx's error when the
// deadline wins; the continuations still running are not stopped.
func (r *InProcessContinuationRunner) Drain(ctx context.Context) error {
	r.mu.Lock()
	r.draining = true
	if r.inFlight == 0 {
		r.mu.Unlock()
		return nil
	}
	idle := r.idle
	r.mu.Unlock()
	select {
	case <-idle:
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
