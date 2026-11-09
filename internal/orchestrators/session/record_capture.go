package session

import (
	"context"
	"crypto/rand"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

const recordCaptureCapacity = 64
const recordCaptureSchema = 1
const captureKindStatus = "capture"
const captureKindRecord = "record"
const captureStarted = "started"
const captureDrained = "drained"
const captureIncomplete = "incomplete"
const captureQueueFull = "queue_full"
const captureWriterFailed = "writer_failed"

var (
	// ErrRecordCaptureOverflow means a bounded capture stopped accepting batches.
	// Already accepted batches drain, followed by an incomplete marker if writable.
	ErrRecordCaptureOverflow = errors.New("session record capture: queue full, capture incomplete")
	// ErrRecordCaptureFailed wraps a writer/encoding failure. Gameplay is already
	// committed and is never rolled back because its diagnostic export failed.
	ErrRecordCaptureFailed = errors.New("session record capture: writer failed, capture incomplete")
	// ErrRecordCaptureClosed refuses publication after a clean Drain started.
	ErrRecordCaptureClosed = errors.New("session record capture: draining or closed")
)

// RecordCaptureConfig supplies one host-owned writer. Capacity counts commit
// batches, not individual events; zero selects64, negative is refused. CaptureID
// is an opaque process-capture identity, generated when absent. ReportError runs
// only on the worker, never under the gameplay operation's guard; nil uses slog.
// The capture does not close Writer. A host drains accepted continuations first,
// then this capture, before closing its output or exiting.
type RecordCaptureConfig struct {
	Writer      io.Writer
	Capacity    int
	CaptureID   string
	ReportError func(error)
}

// RecordCapture exports immutable copies of committed records as schema1 JSONL
// independently of browser subscribers. Publication does no writer I/O. Overflow
// is fail-stop for this capture, not silent sampling or a gameplay failure.
// A clean drain says only that accepted batches were written: initialization or
// history before this capture started is never represented as complete.
type RecordCapture struct {
	mu      sync.Mutex
	closed  bool
	failure error
	queue   chan []sdk.RecordEvent
	done    chan struct{}
	writer  io.Writer
	id      string
	report  func(error)
}

var _ sdk.RecordStream = (*RecordCapture)(nil)

// NewRecordCapture starts one export worker. Nil config/writer or negative
// capacity is refused. Writer failures are reported asynchronously and by Drain.
func NewRecordCapture(in *RecordCaptureConfig) (*RecordCapture, error) {
	if in == nil || in.Writer == nil {
		return nil, errors.New("record capture: writer is required")
	}
	if in.Capacity < 0 {
		return nil, errors.New("record capture: negative capacity")
	}
	capacity := in.Capacity
	if capacity == 0 {
		capacity = recordCaptureCapacity
	}
	id := in.CaptureID
	if id == "" {
		id = rand.Text()
	}
	out := &RecordCapture{queue: make(chan []sdk.RecordEvent, capacity), done: make(chan struct{}), writer: in.Writer, id: id, report: in.ReportError}
	go out.run()
	return out, nil
}

// PublishRecord admits a complete detached batch or returns an explicit capture
// failure. Cancellation cannot erase already committed evidence. Empty batches
// are a no-op. No sink, logger or error callback runs on this calling path.
func (c *RecordCapture) PublishRecord(_ context.Context, events []sdk.RecordEvent) error {
	if len(events) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return c.failure
	}
	if c.closed {
		return ErrRecordCaptureClosed
	}
	owned := make([]sdk.RecordEvent, len(events))
	for i, event := range events {
		owned[i] = event
		owned[i].Fields = append(json.RawMessage(nil), event.Fields...)
	}
	select {
	case c.queue <- owned:
		return nil
	default:
		c.failure = ErrRecordCaptureOverflow
		c.closed = true
		close(c.queue)
		return c.failure
	}
}

// Drain refuses new batches and waits for accepted ones, or returns ctx's
// error without canceling the worker. The writer's failure/overflow remains
// observable on subsequent Drain calls. The host owns blocking Writer lifetime.
func (c *RecordCapture) Drain(ctx context.Context) error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.queue)
	}
	c.mu.Unlock()
	select {
	case <-c.done:
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.failure
	case <-ctx.Done():
		return ctx.Err()
	}
}

type captureStatusLine struct {
	Schema          int       `json:"schema"`
	Kind            string    `json:"kind"`
	Capture         string    `json:"capture"`
	LoggedAt        time.Time `json:"logged_at"`
	Status          string    `json:"status"`
	HistoryComplete bool      `json:"history_complete"`
	Reason          string    `json:"reason,omitempty"`
}

type captureRecordLine struct {
	Schema   int             `json:"schema"`
	Kind     string          `json:"kind"`
	Capture  string          `json:"capture"`
	LoggedAt time.Time       `json:"logged_at"`
	Session  string          `json:"session"`
	ID       string          `json:"id"`
	At       uint64          `json:"at"`
	Act      string          `json:"act"`
	Beat     string          `json:"beat"`
	Fields   json.RawMessage `json:"fields"`
}

type captureWriteInput struct{ Value any }

func (c *RecordCapture) write(in *captureWriteInput) error {
	// JSONv2 preserves raw numeric/string payloads without interpreting them or
	// introducing HTML escaping; the raw record remains the owner's evidence.
	raw, err := jsonv2.Marshal(in.Value)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	n, err := c.writer.Write(raw)
	if err == nil && n != len(raw) {
		err = io.ErrShortWrite
	}
	return err
}

type captureStopInput struct{ Error error }

func (c *RecordCapture) stop(in *captureStopInput) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failure = errors.Join(c.failure, in.Error)
	if !c.closed {
		c.closed = true
		close(c.queue)
	}
}

func (c *RecordCapture) run() {
	defer func() {
		if p := recover(); p != nil {
			c.stop(&captureStopInput{Error: fmt.Errorf("%w: writer panic: %v", ErrRecordCaptureFailed, p)})
		}
		// stop closes the queue before a failing worker returns. Release any
		// accepted-but-unwritten payloads; their loss is explicit, not hidden.
		for range c.queue {
		}
		c.reportFailure()
		close(c.done)
	}()
	if err := c.write(&captureWriteInput{Value: captureStatusLine{Schema: recordCaptureSchema, Kind: captureKindStatus,
		Capture: c.id, LoggedAt: time.Now().UTC(), Status: captureStarted}}); err != nil {
		c.stop(&captureStopInput{Error: fmt.Errorf("%w: %w", ErrRecordCaptureFailed, err)})
		return
	}
	for batch := range c.queue {
		at := time.Now().UTC()
		for _, event := range batch {
			if err := c.write(&captureWriteInput{Value: captureRecordLine{Schema: recordCaptureSchema, Kind: captureKindRecord,
				Capture: c.id, LoggedAt: at, Session: event.Session, ID: event.ID, At: event.At,
				Act: event.Act, Beat: event.Beat, Fields: event.Fields}}); err != nil {
				c.stop(&captureStopInput{Error: fmt.Errorf("%w: %w", ErrRecordCaptureFailed, err)})
				return
			}
		}
	}
	c.mu.Lock()
	failure := c.failure
	c.mu.Unlock()
	status, reason := captureDrained, ""
	if failure != nil {
		status, reason = captureIncomplete, captureWriterFailed
		if errors.Is(failure, ErrRecordCaptureOverflow) {
			reason = captureQueueFull
		}
	}
	if err := c.write(&captureWriteInput{Value: captureStatusLine{Schema: recordCaptureSchema, Kind: captureKindStatus,
		Capture: c.id, LoggedAt: time.Now().UTC(), Status: status, Reason: reason}}); err != nil {
		c.stop(&captureStopInput{Error: fmt.Errorf("%w: %w", ErrRecordCaptureFailed, err)})
	}
}

func (c *RecordCapture) reportFailure() {
	c.mu.Lock()
	err := c.failure
	c.mu.Unlock()
	if err == nil {
		return
	}
	// An observer's callback must not bring down the server either.
	defer func() {
		if p := recover(); p != nil {
			slog.Error("session record capture: error reporter panicked", "capture", c.id, "panic", p)
		}
	}()
	if c.report != nil {
		c.report(err)
		return
	}
	slog.Error("session record capture incomplete", "capture", c.id, "error", err)
}
