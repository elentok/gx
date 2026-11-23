// Package webattach holds one holder attach per web client and session. The
// holder sees each browser tab as its own attached client, not as the prompt
// send path, which is what its input owner rules work on.
package webattach

import (
	"fmt"
	"sync"
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

// Event is one message of a client's event stream. Only the child's exit is
// reported for now.
type Event struct {
	Type string `json:"type"`
	Code int    `json:"code,omitempty"`
}

// EventExit is the Type of an event that reports the child's exit.
const EventExit = "exit"

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

// attach is one client's open holder attach. A reader goroutine drains the
// holder's messages, because an attach nobody reads would stall its holder
// writes.
type attach struct {
	a      *holder.HolderAttach
	events chan Event
	// done closes once the attach has ended and left the map.
	done  chan struct{}
	timer *time.Timer
}

func (at *attach) touch(idle time.Duration) { at.timer.Reset(idle) }

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
	at := &attach{a: ha, events: make(chan Event, 4), done: make(chan struct{})}
	at.timer = time.AfterFunc(as.idle, func() { _ = ha.Close() })
	as.m[k] = at
	go as.read(k, at)
	return at, wire.SessionResult{}, nil
}

// read drains the attach until it ends, then removes it.
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
		if m.Type == holder.StreamExit {
			select {
			case at.events <- Event{Type: EventExit, Code: m.ExitCode}:
			default:
			}
			return
		}
	}
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
