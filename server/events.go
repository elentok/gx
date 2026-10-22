package server

import (
	"errors"
	"sync"
)

// EventTicketChanged is emitted once per ticket whose indexed state changed
// (added, edited or removed).
const EventTicketChanged = "ticket-changed"

const (
	defaultSubscriberBuffer = 64
	// Recent events kept so a client can snapshot, then subscribe from the
	// snapshot's seq without losing what happened in between. Not persisted:
	// events are ephemeral and a restart starts over.
	replayWindow = 256
)

// Event is one streamed event. Seq is server-wide and contiguous.
type Event struct {
	Seq     uint64 `json:"seq"`
	Type    string `json:"type"`
	Address string `json:"address,omitempty"`
	// Level (EventLevel*) and Message are set only by events that report an
	// outcome, such as config-reloaded.
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`
	// Kind (an events.Kind) tells root-paused causes apart.
	Kind string `json:"kind,omitempty"`
	// Client names the TUI an event is for, by its event-stream client id.
	Client string `json:"client,omitempty"`
}

const (
	EventLevelSuccess = "success"
	EventLevelWarning = "warning"
	EventLevelError   = "error"
)

// errSeqGone means the requested seq is outside the replay window: the client
// must re-snapshot.
var errSeqGone = errors.New("seq outside replay window")

// broker numbers events and fans them out. A subscriber whose buffer is full is
// dropped (its channel is closed) rather than ever blocking the publisher.
type broker struct {
	mu     sync.Mutex
	seq    uint64
	recent []Event
	subs   map[chan Event]struct{}
	buffer int
	closed bool
}

func newBroker(buffer int) *broker {
	if buffer <= 0 {
		buffer = defaultSubscriberBuffer
	}
	return &broker{subs: map[chan Event]struct{}{}, buffer: buffer}
}

func (b *broker) currentSeq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}

func (b *broker) publish(typ, address string) {
	b.publishEvent(Event{Type: typ, Address: address})
}

// publishEvent numbers ev (its Seq is overwritten) and fans it out.
func (b *broker) publishEvent(ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	ev.Seq = b.seq
	b.recent = append(b.recent, ev)
	if len(b.recent) > replayWindow {
		b.recent = b.recent[len(b.recent)-replayWindow:]
	}
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
			delete(b.subs, ch)
			close(ch)
		}
	}
}

// subscribe returns a channel carrying every event with seq > since. The
// channel is closed when the subscriber is dropped or the broker closes.
func (b *broker) subscribe(since uint64) (<-chan Event, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if since > b.seq || (since < b.seq && (len(b.recent) == 0 || b.recent[0].Seq > since+1)) {
		return nil, nil, errSeqGone
	}
	var replay []Event
	for _, ev := range b.recent {
		if ev.Seq > since {
			replay = append(replay, ev)
		}
	}
	ch := make(chan Event, len(replay)+b.buffer)
	for _, ev := range replay {
		ch <- ev
	}
	if b.closed {
		close(ch)
		return ch, func() {}, nil
	}
	b.subs[ch] = struct{}{}
	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
	return ch, cancel, nil
}

// close ends every stream so http shutdown isn't held open by subscribers.
func (b *broker) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}
