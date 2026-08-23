package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// ErrBrokerClosed means Subscribe was called after Close.
var ErrBrokerClosed = errors.New("session broker is closed")

// subscriberBuffer bounds how many events a slow subscriber can fall behind
// by before Publish starts dropping for it. Delivery is best-effort by the
// SDK's own EventStream contract (session.EventStream doc): the story log
// (Manager.Story) is the source of truth, and a client that notices a gap in
// Seq re-queries rather than relying on the stream never dropping.
const subscriberBuffer = 32

// ErrLagged is the sentinel a caller checks with errors.Is(err, ErrLagged)
// for a cheap "did anything drop" boolean. The concrete *ErrSubscriberLagged
// Publish actually returns wraps this and carries which (session, recipient,
// seq, kind) lagged for anyone using errors.As instead.
var ErrLagged = errors.New("session broker: subscriber lagged, event dropped")

// ErrSubscriberLagged reports that Publish dropped one event because its
// recipient's buffered channel was already full.
//
// RULED (Kirk, rpg-api#819, design rpg-project/ideas/stream-whole/design.md
// §5): drop, never silently. The stream is a fan-out and must never block
// the acting player's verb on a slow viewer -- the non-blocking send stays
// -- but a drop that nobody can see is unrecoverable, since StreamEvents
// deliberately carries no replay (rule 6: a client resyncs via GetStory on a
// seq gap). Returning this from Publish is what lets the SDK's own
// DeliveryReport.Failed become true instead of always being false.
type ErrSubscriberLagged struct {
	// Session and Recipient identify which (session, recipient) subscriber
	// lagged -- the same key Subscribe was called with.
	Session, Recipient string
	// Seq and Kind identify the dropped event itself.
	Seq  uint64
	Kind sdk.EventKind
	// Total is this subscriber's cumulative drop count, including this one --
	// Broker.Dropped's own value at the moment of this drop, not just a
	// count of drops within this one Publish call.
	Total uint64
}

// Error renders the drop as a single line carrying everything a human needs
// to correlate it against the per-recipient send trace on StreamEvents
// (stream_events.go) -- same four fields, same order.
func (e *ErrSubscriberLagged) Error() string {
	return fmt.Sprintf(
		"session broker: subscriber lagged, event dropped: session=%s recipient=%s seq=%d kind=%s (subscriber total dropped=%d)",
		e.Session, e.Recipient, e.Seq, e.Kind, e.Total,
	)
}

// Unwrap lets errors.Is(err, ErrLagged) match every drop against one
// sentinel while errors.As still recovers the concrete fields above.
func (e *ErrSubscriberLagged) Unwrap() error { return ErrLagged }

// subKey addresses one (session, recipient) pair -- the exact audience the
// SDK already projected each Event for (design rule 4: rpg-api must not
// filter or re-derive visibility).
type subKey struct {
	session   string
	recipient string
}

// Broker fans SDK events out to StreamEvents subscribers, keyed by
// (session, recipient).
//
// This is the EventStream the session.Manager publishes to (see EventStream
// below) and the thing StreamEvents subscribes against. Unlike the old
// encounter path's broker, there is no per-viewer projection HERE -- the SDK
// already produced one Event per recipient before Publish is ever called
// (session.Event doc: "An Event is per-recipient, not per-occurrence"). This
// broker only routes.
type Broker struct {
	mu      sync.Mutex
	nextID  uint64
	subs    map[subKey]map[uint64]chan sdk.Event
	dropped map[subKey]uint64
	closed  bool
}

// NewBroker returns an empty, ready-to-use Broker.
func NewBroker() *Broker {
	return &Broker{subs: make(map[subKey]map[uint64]chan sdk.Event), dropped: make(map[subKey]uint64)}
}

// Subscription is one live StreamEvents connection's event feed.
type Subscription struct {
	broker *Broker
	key    subKey
	id     uint64
	ch     chan sdk.Event
}

// Events returns the channel of events for this subscription. It is closed
// when Close is called or the broker shuts down.
func (s *Subscription) Events() <-chan sdk.Event { return s.ch }

// Close unregisters the subscription. Safe to call more than once.
func (s *Subscription) Close() error {
	s.broker.unsubscribe(s.key, s.id)
	return nil
}

// Subscribe registers a new subscription for (session, recipient).
func (b *Broker) Subscribe(session, recipient string) (*Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrBrokerClosed
	}
	key := subKey{session: session, recipient: recipient}
	if b.subs[key] == nil {
		b.subs[key] = make(map[uint64]chan sdk.Event)
	}
	b.nextID++
	id := b.nextID
	ch := make(chan sdk.Event, subscriberBuffer)
	b.subs[key][id] = ch
	return &Subscription{broker: b, key: key, id: id, ch: ch}, nil
}

func (b *Broker) unsubscribe(key subKey, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs, ok := b.subs[key]
	if !ok {
		return
	}
	if ch, ok := subs[id]; ok {
		close(ch)
		delete(subs, id)
	}
	if len(subs) == 0 {
		delete(b.subs, key)
		// The (session, recipient) key has no live subscriber left at all --
		// drop its accumulated count with it. Otherwise this map only ever
		// grows: a long-lived broker would retain one entry per (session,
		// recipient) that EVER lagged, for the life of the process, long
		// after the session and the connection it belonged to are gone
		// (Copilot, PR #821). A fresh reconnect under the same key already
		// starts with an empty channel; its drop count starts at zero too.
		delete(b.dropped, key)
	}
}

// Dropped returns how many events Publish has ever dropped for one
// (session, recipient) subscriber, cumulative since the broker was
// constructed. Zero for a recipient that has never lagged.
func (b *Broker) Dropped(session, recipient string) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped[subKey{session: session, recipient: recipient}]
}

// Publish implements sdk.EventStream. It fans each event out to the live
// subscribers of its (Session, Recipient) pair.
//
// Non-blocking: a subscriber whose buffered channel is full is skipped
// rather than stalling the publisher -- required by the SDK's EventStream
// contract ("implementations should therefore not block indefinitely")
// since a stalled Publish would stall the verb that produced the events,
// and RULED (Kirk, rpg-api#819): the stream is a fan-out and must never
// block the acting player's verb on a lagging viewer. Every OTHER
// subscriber in the batch -- same recipient's other connections, and every
// other recipient entirely -- still gets its events; one lagging viewer
// never starves the rest.
//
// The drop itself is never silent, though. Each one is counted per
// (session, recipient), logged at warn with enough to correlate against the
// StreamEvents send trace (session, recipient, seq, kind), and reported
// back through the returned error so the SDK's own DeliveryReport.Failed
// reflects reality (events.go: "a failure here is reported ... because the
// story log remains the source of truth"). Multiple drops in one batch join
// into a single error via errors.Join; errors.Is(err, ErrLagged) matches
// the whole thing, and errors.As recovers each *ErrSubscriberLagged in turn.
func (b *Broker) Publish(ctx context.Context, events []sdk.Event) error {
	b.mu.Lock()
	var drops []*ErrSubscriberLagged
	for _, evt := range events {
		key := subKey{session: evt.Session, recipient: evt.Recipient}
		for _, ch := range b.subs[key] {
			select {
			case ch <- evt:
			default:
				b.dropped[key]++
				drops = append(drops, &ErrSubscriberLagged{
					Session: evt.Session, Recipient: evt.Recipient,
					Seq: evt.Seq, Kind: evt.Kind, Total: b.dropped[key],
				})
			}
		}
	}
	b.mu.Unlock()

	if len(drops) == 0 {
		return nil
	}

	errs := make([]error, len(drops))
	for i, d := range drops {
		slog.WarnContext(ctx, "session broker: subscriber lagged, event dropped",
			"session", d.Session, "recipient", d.Recipient, "seq", d.Seq, "kind", d.Kind, "dropped_total", d.Total)
		errs[i] = d
	}
	return errors.Join(errs...)
}

// Close shuts the broker down: every live subscription's channel is closed
// and no further Subscribe calls succeed. Call once at server shutdown.
func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, subs := range b.subs {
		for _, ch := range subs {
			close(ch)
		}
	}
	b.subs = make(map[subKey]map[uint64]chan sdk.Event)
	b.dropped = make(map[subKey]uint64)
	return nil
}
