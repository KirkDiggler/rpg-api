package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// recordBuffer is how many commits a subscriber may fall behind by before it
// is closed as lagged. It counts commits, as actBuffer does, never single
// beats: on the garrison keep the record grows by hundreds of beats per verb
// (rpg-project#894, F15), and a buffer of 256 BEATS would close a live
// StreamRecord on the first verb. 64 is sized as actBuffer's is, and so is
// the ceiling: about 64 commits times a commit's beats per stalled client.
const recordBuffer = 64

// ErrRecordLagged is what PublishRecord returns when it closed at least one
// subscriber as lagged. errors.As recovers each *RecordLaggedError.
var ErrRecordLagged = errors.New("session record broker: subscriber lagged, stream closed")

// RecordLaggedError names one subscriber the broker closed as lagged.
type RecordLaggedError struct {
	// Session is the subscriber's session.
	Session string
	// LastID is the last record id of the last unit queued for the
	// subscriber before it was closed; "" when nothing was.
	LastID string
}

// Error renders the lag on one line.
func (e *RecordLaggedError) Error() string {
	return fmt.Sprintf("session record broker: subscriber lagged, stream closed: session=%s last_id=%s",
		e.Session, e.LastID)
}

// Unwrap lets errors.Is(err, ErrRecordLagged) match every lag.
func (e *RecordLaggedError) Unwrap() error { return ErrRecordLagged }

// RecordBroker fans the SDK's record beats out to StreamRecord subscribers,
// keyed by session — the record is per session, not per recipient
// (rpg-project#628, event-stream R13, R26).
//
// ONE COMMIT IS ONE DELIVERY UNIT. PublishRecord is handed one commit's
// beats (sdk.RecordStream); the broker groups them by session, in order, and
// enqueues each group as one unit ([RecordUnit]), so the buffer bounds
// commits, not beats.
//
// LOSS IS THE TRANSPORT'S CONTRACT (R28), and this broker keeps it honestly,
// which the v1 Broker does not (rpg-api#1122, left open for v1): a
// subscriber whose bounded buffer is full is NOT SKIPPED. It is marked
// lagged with the last id queued for it, its channel is closed, and it is
// removed. Its StreamRecord drains what was queued, then tells the client
// RecordLagged{last_id}; the client catches up with GetRecord after that id.
// Publishing never blocks the verb that produced the beats.
type RecordBroker struct {
	mu     sync.Mutex
	buffer int
	nextID uint64
	subs   map[string]map[uint64]*RecordSubscription
	closed bool
}

// NewRecordBroker returns an empty broker whose subscribers may each fall
// behind by buffer commits; buffer <= 0 takes the default.
func NewRecordBroker(buffer int) *RecordBroker {
	if buffer <= 0 {
		buffer = recordBuffer
	}
	return &RecordBroker{buffer: buffer, subs: make(map[string]map[uint64]*RecordSubscription)}
}

// RecordUnit is one commit's beats for one session, in id order. Every
// subscriber of the session is handed the same slice; it is read-only.
type RecordUnit []sdk.RecordEvent

// RecordSubscription is one live StreamRecord connection's feed.
type RecordSubscription struct {
	broker  *RecordBroker
	session string
	id      uint64
	ch      chan RecordUnit

	// Guarded by broker.mu.
	lastID string
	lagged bool
	done   bool
}

// Units is the subscription's feed, one commit's beats per receive, closed
// when the subscription ends — by Close, by the broker's Close, or because it
// lagged ([Lagged]).
func (s *RecordSubscription) Units() <-chan RecordUnit { return s.ch }

// Lagged reports whether the broker closed this subscription because it fell
// behind, and the last id queued for it before it did. Meaningful once
// Units is closed.
func (s *RecordSubscription) Lagged() (bool, string) {
	s.broker.mu.Lock()
	defer s.broker.mu.Unlock()
	return s.lagged, s.lastID
}

// Close ends the subscription. Safe to call more than once, and after the
// broker closed it.
func (s *RecordSubscription) Close() error {
	s.broker.mu.Lock()
	defer s.broker.mu.Unlock()
	s.broker.endLocked(s)
	return nil
}

// Subscribe registers a subscription to one session's record.
func (b *RecordBroker) Subscribe(session string) (*RecordSubscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrBrokerClosed
	}
	if b.subs[session] == nil {
		b.subs[session] = make(map[uint64]*RecordSubscription)
	}
	b.nextID++
	sub := &RecordSubscription{broker: b, session: session, id: b.nextID, ch: make(chan RecordUnit, b.buffer)}
	b.subs[session][sub.id] = sub
	return sub, nil
}

// endLocked closes and unregisters sub once. Caller holds b.mu.
func (b *RecordBroker) endLocked(sub *RecordSubscription) {
	if sub.done {
		return
	}
	sub.done = true
	close(sub.ch)
	if subs := b.subs[sub.session]; subs != nil {
		delete(subs, sub.id)
		if len(subs) == 0 {
			delete(b.subs, sub.session)
		}
	}
}

// PublishRecord implements sdk.RecordStream. The commit's beats for each
// session go as one unit to every live subscriber of that session. A
// subscriber with no room for the unit is closed as lagged rather than
// skipped or handed part of it; every other subscriber still gets every beat.
// Lags are logged and returned joined, matching ErrRecordLagged.
func (b *RecordBroker) PublishRecord(ctx context.Context, events []sdk.RecordEvent) error {
	var order []string
	units := make(map[string]RecordUnit)
	for _, evt := range events {
		if _, seen := units[evt.Session]; !seen {
			order = append(order, evt.Session)
		}
		units[evt.Session] = append(units[evt.Session], evt)
	}

	b.mu.Lock()
	var lags []*RecordLaggedError
	for _, session := range order {
		unit := units[session]
		for _, sub := range b.subs[session] {
			select {
			case sub.ch <- unit:
				sub.lastID = unit[len(unit)-1].ID
			default:
				sub.lagged = true
				lags = append(lags, &RecordLaggedError{Session: sub.session, LastID: sub.lastID})
				b.endLocked(sub)
			}
		}
	}
	b.mu.Unlock()

	if len(lags) == 0 {
		return nil
	}
	errs := make([]error, len(lags))
	for i, l := range lags {
		slog.WarnContext(ctx, "session record broker: subscriber lagged, stream closed",
			"session", l.Session, "last_id", l.LastID)
		errs[i] = l
	}
	return errors.Join(errs...)
}

// Close ends every subscription, unlagged, and refuses new ones. Call once at
// server shutdown.
func (b *RecordBroker) Close() error {
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
