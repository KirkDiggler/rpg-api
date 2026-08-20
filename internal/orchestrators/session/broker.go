package session

import (
	"context"
	"errors"
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
	mu     sync.Mutex
	nextID uint64
	subs   map[subKey]map[uint64]chan sdk.Event
	closed bool
}

// NewBroker returns an empty, ready-to-use Broker.
func NewBroker() *Broker {
	return &Broker{subs: make(map[subKey]map[uint64]chan sdk.Event)}
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
	}
}

// Publish implements sdk.EventStream. It fans each event out to the live
// subscribers of its (Session, Recipient) pair. Non-blocking: a subscriber
// whose buffered channel is full drops the event rather than stalling the
// publisher -- required by the SDK's EventStream contract ("implementations
// should therefore not block indefinitely") since a stalled Publish would
// stall the verb that produced the events.
func (b *Broker) Publish(_ context.Context, events []sdk.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, evt := range events {
		key := subKey{session: evt.Session, recipient: evt.Recipient}
		for _, ch := range b.subs[key] {
			select {
			case ch <- evt:
			default:
			}
		}
	}
	return nil
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
	return nil
}
