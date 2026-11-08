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

// actBuffer is how many commits a subscriber may fall behind by before it is
// closed as lagged. The buffer counts what a client waits for — one commit's
// acts for it, one delivery unit — never single acts: on the garrison keep one
// verb folds 165 to 320 acts for one player (every patrol step she sees is
// its own act), and a buffer of 256 ACTS closed the subscriber on the first
// verb (rpg-project#894, F15).
//
// 64 commits is a client that is truly behind: a verb is one or two commits
// (the answer, then its continuation), so it is thirty-odd verbs, or a
// monster round's turns several times over, unread. The ceiling per stalled
// client is therefore about 64 commits times a commit's acts for it (320 on
// the garrison, so some 20k acts), where it was 256 acts. Held units are the
// commit's own slices, shared by every subscriber of the recipient, so
// several stalled clients of one recipient hold one copy, not one each.
const actBuffer = 64

// ActBufferEnv overrides actBuffer for one process, in commits. It exists for
// the castle walk, which forces a lag with a small buffer; a value that is not
// a positive integer is ignored.
const ActBufferEnv = "SESSION_ACT_BUFFER"

// ErrActsLagged is what PublishActs returns when it closed at least one
// subscriber as lagged. errors.As recovers each *ActsLaggedError.
var ErrActsLagged = errors.New("session act broker: subscriber lagged, stream closed")

// ActsLaggedError names one subscriber the broker closed as lagged.
type ActsLaggedError struct {
	// Session and Recipient are the subscriber's key.
	Session   string
	Recipient string
	// LastID is the last id of the last unit queued for the subscriber
	// before it was closed; "" when nothing was.
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
// ONE COMMIT IS ONE DELIVERY UNIT. PublishActs is handed one commit's acts
// (sdk.ActStream); the broker groups them by session and recipient, in order,
// and enqueues each group as one unit ([ActUnit]) to that key's subscribers.
// The buffer bounds units, so a long commit never lags a client that keeps up.
//
// A SAME-ID RUN IS NEVER SPLIT (sdk.Act.ID). Acts that share an id are one
// record entry folded into several acts, adjacent within one commit, so a unit
// holds whole runs. A subscriber with no room for the next unit is closed as
// lagged with LastID the last id of the last unit it WAS handed, and removed.
// A lag therefore falls between two commits, so between two ids, and catching
// up after LastID loses nothing (R28).
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
// by buffer commits; buffer <= 0 takes the default.
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

// BrokeredAct is one act as the broker hands it over, with whether it ends its
// same-id run. The broker enqueues runs whole, so it knows the unit and marks
// it (Act.run_end): a client closes a live run on RunEnd, never on a timer.
type BrokeredAct struct {
	Act    sdk.Act
	RunEnd bool
}

// ActUnit is one commit's acts for one subscription's session and recipient,
// in record order: whole same-id runs, the last act of each marked RunEnd.
// Every subscriber of the key is handed the same slice; it is read-only.
type ActUnit []BrokeredAct

// ActSubscription is one live StreamEvents (v1alpha2) connection's feed.
type ActSubscription struct {
	broker *ActBroker
	key    actKey
	id     uint64
	ch     chan ActUnit

	// Guarded by broker.mu.
	lastID string
	lagged bool
	done   bool
}

// Units is the subscription's feed, one commit's acts per receive, closed when
// the subscription ends — by Close, by the broker's Close, or because it
// lagged ([Lagged]).
func (s *ActSubscription) Units() <-chan ActUnit { return s.ch }

// Lagged reports whether the broker closed this subscription because it fell
// behind, and the last id of the last unit queued for it before it did.
// Meaningful once Units is closed.
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
	sub := &ActSubscription{broker: b, key: key, id: b.nextID, ch: make(chan ActUnit, b.buffer)}
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

// commitUnits groups one commit's acts by session and recipient, in order of
// each key's first act, and marks the last act of every same-id run. The SDK
// hands a recipient's same-id acts over adjacent, so a run is never
// interleaved with another recipient's and every unit holds whole runs.
func commitUnits(acts []sdk.Act) ([]actKey, map[actKey]ActUnit) {
	var order []actKey
	units := make(map[actKey]ActUnit)
	for i, a := range acts {
		key := actKey{session: a.Session, recipient: a.Recipient}
		if _, seen := units[key]; !seen {
			order = append(order, key)
		}
		runEnd := i == len(acts)-1 ||
			acts[i+1].ID != a.ID ||
			acts[i+1].Session != a.Session ||
			acts[i+1].Recipient != a.Recipient
		units[key] = append(units[key], BrokeredAct{Act: a, RunEnd: runEnd})
	}
	return order, units
}

// PublishActs implements sdk.ActStream. The commit's acts for each session and
// recipient go as one unit to every live subscriber of that key. A subscriber
// with no room for the unit is closed as lagged rather than handed part of
// the commit; every other subscriber still gets every act. Lags are logged and
// returned joined, matching ErrActsLagged.
func (b *ActBroker) PublishActs(ctx context.Context, acts []sdk.Act) error {
	order, units := commitUnits(acts)
	b.mu.Lock()
	var lags []*ActsLaggedError
	for _, key := range order {
		unit := units[key]
		for _, sub := range b.subs[key] {
			select {
			case sub.ch <- unit:
				sub.lastID = unit[len(unit)-1].Act.ID
			default:
				sub.lagged = true
				lags = append(lags, &ActsLaggedError{Session: key.session, Recipient: key.recipient, LastID: sub.lastID})
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
