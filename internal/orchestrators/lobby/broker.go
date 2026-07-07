package lobby

import (
	"errors"
	"sync"
)

// ErrBrokerClosed means Subscribe was called after Close.
var ErrBrokerClosed = errors.New("lobby broker is closed")

// Broker fans out lobby Events to StreamLobby subscribers.
//
// Unlike the toolkit's encounter Broker, this carries no per-viewer
// projection — a lobby roster has no line-of-sight concept, so every
// subscriber for a given lobby ID receives an identical event stream. That
// keeps this broker deliberately simpler than tkenc.Broker: a map of
// lobbyID -> subscriber channels, guarded by one mutex.
type Broker struct {
	mu     sync.Mutex
	nextID uint64
	subs   map[string]map[uint64]chan *Event
	closed bool
}

// NewBroker returns an empty, ready-to-use Broker.
func NewBroker() *Broker {
	return &Broker{subs: make(map[string]map[uint64]chan *Event)}
}

// Subscription is one live StreamLobby connection's event feed.
type Subscription struct {
	broker  *Broker
	lobbyID string
	id      uint64
	ch      chan *Event
}

// Events returns the channel of events for this subscription. It is closed
// when Close is called or the broker shuts down.
func (s *Subscription) Events() <-chan *Event { return s.ch }

// Close unregisters the subscription. Safe to call more than once.
func (s *Subscription) Close() error {
	s.broker.unsubscribe(s.lobbyID, s.id)
	return nil
}

// subscriberBuffer bounds how many events a slow subscriber can fall behind
// by before Publish starts dropping for it. StreamLobby is snapshot-first
// (mirrors StreamEncounter), so a dropped event just means the next
// subscribe resyncs the roster — never a durable-log guarantee.
const subscriberBuffer = 16

// Subscribe registers a new subscription for lobbyID.
func (b *Broker) Subscribe(lobbyID string) (*Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrBrokerClosed
	}
	if b.subs[lobbyID] == nil {
		b.subs[lobbyID] = make(map[uint64]chan *Event)
	}
	b.nextID++
	id := b.nextID
	ch := make(chan *Event, subscriberBuffer)
	b.subs[lobbyID][id] = ch
	return &Subscription{broker: b, lobbyID: lobbyID, id: id, ch: ch}, nil
}

func (b *Broker) unsubscribe(lobbyID string, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs, ok := b.subs[lobbyID]
	if !ok {
		return
	}
	if ch, ok := subs[id]; ok {
		close(ch)
		delete(subs, id)
	}
	if len(subs) == 0 {
		delete(b.subs, lobbyID)
	}
}

// Publish fans evt out to every live subscriber of lobbyID. Non-blocking: a
// subscriber whose buffered channel is full drops the event rather than
// stalling the publisher (mirrors the encounter broker's fanout-only, no
// durable-log contract).
func (b *Broker) Publish(lobbyID string, evt *Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs[lobbyID] {
		select {
		case ch <- evt:
		default:
		}
	}
}

// Close shuts the broker down: every live subscription's channel is closed
// and no further Subscribe calls succeed. Call once at server shutdown
// (mirrors tkenc.Broker.Close in cmd/server/server.go).
func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, subs := range b.subs {
		for _, ch := range subs {
			close(ch)
		}
	}
	b.subs = make(map[string]map[uint64]chan *Event)
	return nil
}
