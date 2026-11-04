package broker

import (
	"errors"
	"sync"

	"github.com/elentok/gx/server/wire"
)

const (
	defaultSubscriberBuffer = 64
	// Recent events kept so a client can snapshot, then subscribe from the
	// snapshot's seq without losing what happened in between. Not persisted:
	// events are ephemeral and a restart starts over.
	replayWindow = 256
)

// ErrSeqGone means the requested seq is outside the replay window: the client
// must re-snapshot.
var ErrSeqGone = errors.New("seq outside replay window")

// Broker numbers events and fans them out. A subscriber whose buffer is full is
// dropped (its channel is closed) rather than ever blocking the publisher.
type Broker struct {
	mu     sync.Mutex
	seq    uint64
	recent []wire.Event
	subs   map[chan wire.Event]struct{}
	buffer int
	closed bool
}

func New(buffer int) *Broker {
	if buffer <= 0 {
		buffer = defaultSubscriberBuffer
	}
	return &Broker{subs: map[chan wire.Event]struct{}{}, buffer: buffer}
}

func (b *Broker) CurrentSeq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}

func (b *Broker) Publish(typ, address string) {
	b.PublishEvent(wire.Event{Type: typ, Address: address})
}

// PublishEvent numbers ev (its Seq is overwritten) and fans it out.
func (b *Broker) PublishEvent(ev wire.Event) {
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

// Subscribe returns a channel carrying every event with seq > since. The
// channel is closed when the subscriber is dropped or the broker closes.
func (b *Broker) Subscribe(since uint64) (<-chan wire.Event, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if since > b.seq || (since < b.seq && (len(b.recent) == 0 || b.recent[0].Seq > since+1)) {
		return nil, nil, ErrSeqGone
	}
	var replay []wire.Event
	for _, ev := range b.recent {
		if ev.Seq > since {
			replay = append(replay, ev)
		}
	}
	ch := make(chan wire.Event, len(replay)+b.buffer)
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

// Close ends every stream so http shutdown isn't held open by subscribers.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}
