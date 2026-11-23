// Package webattach holds one holder attach per web client and session. The
// holder sees each browser tab as its own attached client, not as the prompt
// send path, which is what its input owner rules work on.
package webattach

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
)

// DefaultIdleTimeout closes an attach no request or stream has used for this long.
const DefaultIdleTimeout = 2 * time.Minute

// maxClientLen bounds the client id a browser picks for its tab.
const maxClientLen = 64

type Event = wire.SessionAttachEvent

const (
	EventExit    = wire.AttachEventExit
	EventOwner   = wire.AttachEventOwner
	EventBlocked = wire.AttachEventBlocked
)

// Attaches is the set of open web attaches.
type Attaches struct {
	sess *sessions.Manager
	idle time.Duration

	mu sync.Mutex
	m  map[key]*attach
}

type key struct{ session, client string }

// New returns an Attaches over sess. A zero idle is DefaultIdleTimeout.
func New(sess *sessions.Manager, idle time.Duration) *Attaches {
	if idle <= 0 {
		idle = DefaultIdleTimeout
	}
	return &Attaches{sess: sess, idle: idle, m: map[key]*attach{}}
}

// attach is one client's open holder attach. A reader goroutine reads the
// holder's messages, because an attach nobody reads would stall its holder
// writes.
type attach struct {
	a      *holder.HolderAttach
	events chan Event
	// done closes once the attach has ended and left the map.
	done  chan struct{}
	timer *time.Timer
	// streams counts open event streams. An open stream is a tab that is
	// watching, so the idle timer re-arms instead of closing the attach.
	streams atomic.Int32
}

func (at *attach) touch(idle time.Duration) { at.timer.Reset(idle) }

// watch marks an event stream open; the returned func marks it closed and
// starts a fresh idle period.
func (as *Attaches) watch(at *attach) func() {
	at.streams.Add(1)
	return func() {
		at.streams.Add(-1)
		at.touch(as.idle)
	}
}

// open returns the client's attach to the session, opening it on first use.
// A refusal comes back as a result with Refused set.
func (as *Attaches) open(id, client string) (*attach, wire.SessionResult, error) {
	if client == "" || len(client) > maxClientLen {
		return nil, sessions.Refusal(wire.ReasonSessionBadRequest, "bad client id"), nil
	}
	k := key{id, client}
	as.mu.Lock()
	defer as.mu.Unlock()
	if at, ok := as.m[k]; ok {
		at.touch(as.idle)
		return at, wire.SessionResult{}, nil
	}
	dir, meta, res, err := as.sess.SessionDir(id)
	if res.Refused || err != nil {
		return nil, res, err
	}
	if st := meta.Info().Status.State; !chat.AcceptsKeys(st) {
		return nil, sessions.Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s is %s", id, st)), nil
	}
	c, err := holder.DialHolder(dir)
	if err != nil {
		return nil, sessions.Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has no live holder", id)), nil
	}
	ha, err := c.Attach()
	if err != nil {
		return nil, sessions.Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s cannot be attached: %v", id, err)), nil
	}
	at := &attach{a: ha, events: make(chan Event, 16), done: make(chan struct{})}
	at.timer = time.AfterFunc(as.idle, func() {
		if at.streams.Load() > 0 {
			at.touch(as.idle)
			return
		}
		_ = ha.Close()
	})
	as.m[k] = at
	go as.read(k, at)
	return at, wire.SessionResult{}, nil
}

// read consumes the attach until it ends, then removes it.
func (as *Attaches) read(k key, at *attach) {
	defer func() {
		at.timer.Stop()
		_ = at.a.Close()
		as.mu.Lock()
		if as.m[k] == at {
			delete(as.m, k)
		}
		as.mu.Unlock()
		close(at.done)
	}()
	for {
		m, err := at.a.Next()
		if err != nil {
			return
		}
		switch m.Type {
		case holder.StreamExit:
			select {
			case at.events <- Event{Type: EventExit, Code: m.ExitCode}:
			default:
			}
			return
		case holder.StreamOwner:
			at.emit(Event{Type: EventOwner, Self: m.Owner.Self, GX: m.Owner.GX})
		case holder.StreamBlocked:
			at.emit(Event{Type: EventBlocked})
		}
	}
}

// emit queues ev, dropping the oldest queued event when a slow stream let the
// queue fill: the newest owner report is the one that matters.
func (at *attach) emit(ev Event) {
	for {
		select {
		case at.events <- ev:
			return
		default:
		}
		select {
		case <-at.events:
		default:
		}
	}
}

// TakeOver asks the holder to make the client the input owner.
func (as *Attaches) TakeOver(id, client string) (wire.SessionResult, error) {
	return as.send(id, client, func(a *holder.HolderAttach) error { return a.TakeOver() })
}

// Key types k into the session as the client.
func (as *Attaches) Key(id, client string, k screen.Key) (wire.SessionResult, error) {
	return as.send(id, client, func(a *holder.HolderAttach) error { return a.SendKey(k) })
}

// Paste pastes text into the session as the client.
func (as *Attaches) Paste(id, client, text string) (wire.SessionResult, error) {
	return as.send(id, client, func(a *holder.HolderAttach) error { return a.Paste(text) })
}

func (as *Attaches) send(id, client string, do func(*holder.HolderAttach) error) (wire.SessionResult, error) {
	at, res, err := as.open(id, client)
	if res.Refused || err != nil {
		return res, err
	}
	if err := do(at.a); err != nil {
		_ = at.a.Close()
		return sessions.Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s attach ended", id)), nil
	}
	return wire.SessionResult{}, nil
}

// Detach closes the client's attach, if it has one.
func (as *Attaches) Detach(id, client string) {
	as.mu.Lock()
	at := as.m[key{id, client}]
	as.mu.Unlock()
	if at != nil {
		_ = at.a.Close()
		<-at.done
	}
}

// Forget closes every attach to a session that is gone.
func (as *Attaches) Forget(id string) {
	as.mu.Lock()
	var gone []*attach
	for k, at := range as.m {
		if k.session == id {
			gone = append(gone, at)
		}
	}
	as.mu.Unlock()
	for _, at := range gone {
		_ = at.a.Close()
	}
}
