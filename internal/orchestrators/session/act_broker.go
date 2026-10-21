package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// actBuffer is how many acts a subscriber may fall behind by before it is
// closed as lagged. A monster round's burst is the case it is sized for, as
// recordBuffer is.
const actBuffer = 256

// ActBufferEnv overrides actBuffer for one process. It exists for the castle
// walk, which forces a lag with a small buffer; a value that is not a
// positive integer is ignored.
const ActBufferEnv = "SESSION_ACT_BUFFER"

// ErrActsLagged is what PublishActs returns when it closed at least one
// subscriber as lagged. errors.As recovers each *ActsLaggedError.
var ErrActsLagged = errors.New("session act broker: subscriber lagged, stream closed")

// ActsLaggedError names one subscriber the broker closed as lagged.
type ActsLaggedError struct {
	// Session and Recipient are the subscriber's key.
	Session   string
	Recipient string
	// LastID is the last id whose whole same-id run was queued for the
	// subscriber before it was closed; "" when nothing was.
	LastID string
}

// Error renders the lag on one line.
func (e *ActsLaggedError) Error() string {
	return fmt.Sprintf("session act broker: subscriber lagged, stream closed: session=%s recipient=%s last_id=%s",
		e.Session, e.Recipient, e.LastID)
}

// Unwrap lets errors.Is(err, ErrActsLagged) match every lag.
func (e *ActsLaggedError) Unwrap() error { return ErrActsLagged }

// actKey is a subscription's routing key: acts are folded per recipient, so
// they are routed per recipient (rpg-project#630).
type actKey struct {
	session   string
	recipient string
}

// ActBroker implements sdk.ActStream: it fans one commit's acts out to the
// StreamEvents (v1alpha2) subscribers of each act's session and recipient.
//
// A SAME-ID RUN IS ONE DELIVERY UNIT (sdk.Act.ID). Acts that share an id are
// one record entry folded into several acts — a rest and the concentrations it
// ended — and resuming after that id skips them all. So the broker enqueues a
// run whole or not at all: before enqueueing a run it checks that the
// subscriber's free buffer holds the whole of it. When it does not, nothing of
// the run is enqueued, the subscriber is closed as lagged with LastID the last
// id whose run WAS fully enqueued, and it is removed. A lag is therefore only
// ever between two ids, and catching up after LastID loses nothing (R28).
//
// Publishing never blocks the verb that produced the acts.
type ActBroker struct {
	mu     sync.Mutex
	buffer int
	nextID uint64
	subs   map[actKey]map[uint64]*ActSubscription
	closed bool
}

// NewActBroker returns an empty broker whose subscribers may each fall behind
// by buffer acts; buffer <= 0 takes the default.
func NewActBroker(buffer int) *ActBroker {
	if buffer <= 0 {
		buffer = actBuffer
	}
	return &ActBroker{buffer: buffer, subs: make(map[actKey]map[uint64]*ActSubscription)}
}

// ActBufferFromEnv reads ActBufferEnv: its value when it is a positive integer,
// 0 (the default) otherwise.
func ActBufferFromEnv() int {
	n, err := strconv.Atoi(os.Getenv(ActBufferEnv))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// ActSubscription is one live StreamEvents (v1alpha2) connection's feed.
type ActSubscription struct {
	broker *ActBroker
	key    actKey
	id     uint64
	ch     chan sdk.Act

	// Guarded by broker.mu.
	lastID string
	lagged bool
	done   bool
}

// Acts is the subscription's feed, closed when the subscription ends — by
// Close, by the broker's Close, or because it lagged ([Lagged]).
func (s *ActSubscription) Acts() <-chan sdk.Act { return s.ch }

// Lagged reports whether the broker closed this subscription because it fell
// behind, and the last id whose whole run was queued for it before it did.
// Meaningful once Acts is closed.
func (s *ActSubscription) Lagged() (bool, string) {
	s.broker.mu.Lock()
	defer s.broker.mu.Unlock()
	return s.lagged, s.lastID
}

// Close ends the subscription. Safe to call more than once, and after the
// broker closed it.
func (s *ActSubscription) Close() error {
	s.broker.mu.Lock()
	defer s.broker.mu.Unlock()
	s.broker.endLocked(s)
	return nil
}

// Subscribe registers a subscription to one recipient's acts in one session.
func (b *ActBroker) Subscribe(session, recipient string) (*ActSubscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrBrokerClosed
	}
	key := actKey{session: session, recipient: recipient}
	if b.subs[key] == nil {
		b.subs[key] = make(map[uint64]*ActSubscription)
	}
	b.nextID++
	sub := &ActSubscription{broker: b, key: key, id: b.nextID, ch: make(chan sdk.Act, b.buffer)}
	b.subs[key][sub.id] = sub
	return sub, nil
}

// endLocked closes and unregisters sub once. Caller holds b.mu.
func (b *ActBroker) endLocked(sub *ActSubscription) {
	if sub.done {
		return
	}
	sub.done = true
	close(sub.ch)
	if subs := b.subs[sub.key]; subs != nil {
		delete(subs, sub.id)
		if len(subs) == 0 {
			delete(b.subs, sub.key)
		}
	}
}

// sameIDRuns splits acts into maximal runs of consecutive acts that share a
// key and an id, in order. The SDK hands a recipient's same-id acts over
// adjacent, so a run is never interleaved with another recipient's.
func sameIDRuns(acts []sdk.Act) [][]sdk.Act {
	var runs [][]sdk.Act
	start := 0
	for i := 1; i <= len(acts); i++ {
		if i == len(acts) ||
			acts[i].ID != acts[start].ID ||
			acts[i].Session != acts[start].Session ||
			acts[i].Recipient != acts[start].Recipient {
			runs = append(runs, acts[start:i])
			start = i
		}
	}
	return runs
}

// PublishActs implements sdk.ActStream. Each same-id run goes whole to every
// live subscriber of its session and recipient, in order. A subscriber whose
// free buffer cannot hold a whole run is closed as lagged rather than handed
// part of it; every other subscriber still gets every act. Lags are logged and
// returned joined, matching ErrActsLagged.
func (b *ActBroker) PublishActs(ctx context.Context, acts []sdk.Act) error {
	b.mu.Lock()
	var lags []*ActsLaggedError
	for _, run := range sameIDRuns(acts) {
		key := actKey{session: run[0].Session, recipient: run[0].Recipient}
		for _, sub := range b.subs[key] {
			// Only this publisher, under b.mu, adds to sub.ch, so the free
			// room measured here can only grow before the sends below.
			if cap(sub.ch)-len(sub.ch) < len(run) {
				sub.lagged = true
				lags = append(lags, &ActsLaggedError{Session: key.session, Recipient: key.recipient, LastID: sub.lastID})
				b.endLocked(sub)
				continue
			}
			for _, a := range run {
				sub.ch <- a
			}
			sub.lastID = run[0].ID
		}
	}
	b.mu.Unlock()

	if len(lags) == 0 {
		return nil
	}
	errs := make([]error, len(lags))
	for i, l := range lags {
		slog.WarnContext(ctx, "session act broker: subscriber lagged, stream closed",
			"session", l.Session, "recipient", l.Recipient, "last_id", l.LastID)
		errs[i] = l
	}
	return errors.Join(errs...)
}

// Close ends every subscription, unlagged, and refuses new ones. Call once at
// server shutdown.
func (b *ActBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, subs := range b.subs {
		for _, sub := range subs {
			b.endLocked(sub)
		}
	}
	return nil
}
