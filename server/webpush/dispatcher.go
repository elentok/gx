package webpush

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/elentok/gx/server/webauth"
)

const defaultMerge = time.Second

// Event is one thing worth a push. Path is a route path.
type Event struct {
	Title, Body, Path, Tag string
	Urgent                 bool
}

// Subs is the subscription store the dispatcher reads and prunes.
type Subs interface {
	PushTargets() []webauth.PushTarget
	PushSubscriptionOf(loginID string) (webauth.PushSubscription, bool)
	DeletePushSubscriptionIfEndpoint(loginID, endpoint string) error
}

// SendFunc is the Sender.Send signature, so tests can fake it.
type SendFunc func(ctx context.Context, sub Subscription, msg Message, opts Options) Outcome

// Dispatcher merges events per tag and fans each one out to every subscribed device.
type Dispatcher struct {
	Subs  Subs
	Send  SendFunc
	Badge func() int
	Merge time.Duration // zero means one second
	Log   *slog.Logger

	mu       sync.Mutex
	pending  map[string]*pendingPush
	failures map[string]failure
	closed   bool
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

type pendingPush struct {
	ev    Event
	timer *time.Timer
}

type failure struct {
	reason string
	at     time.Time
}

func (d *Dispatcher) init() {
	if d.ctx == nil {
		d.ctx, d.cancel = context.WithCancel(context.Background())
	}
}

// Push holds ev for the merge delay; a later event with the same tag replaces
// it without restarting the timer.
func (d *Dispatcher) Push(ev Event) { d.push(ev, false) }

// PushNow is Push for a caller that has already merged: it sends at once.
func (d *Dispatcher) PushNow(ev Event) { d.push(ev, true) }

func (d *Dispatcher) push(ev Event, now bool) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.init()
	if now {
		d.wg.Go(func() { d.fanOut(ev) })
		return
	}
	if p, ok := d.pending[ev.Tag]; ok {
		p.ev = ev
		return
	}
	if d.pending == nil {
		d.pending = map[string]*pendingPush{}
	}
	delay := d.Merge
	if delay == 0 {
		delay = defaultMerge
	}
	p := &pendingPush{ev: ev}
	d.pending[ev.Tag] = p
	d.wg.Add(1)
	p.timer = time.AfterFunc(delay, func() {
		defer d.wg.Done()
		d.mu.Lock()
		cur := d.pending[ev.Tag]
		delete(d.pending, ev.Tag)
		d.mu.Unlock()
		if cur == p {
			d.fanOut(p.ev)
		}
	})
}

// Close drops held events, cancels sends in flight and waits for them.
func (d *Dispatcher) Close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.closed = true
	d.init()
	for tag, p := range d.pending {
		if p.timer.Stop() {
			d.wg.Done()
		}
		delete(d.pending, tag)
	}
	d.cancel()
	d.mu.Unlock()
	d.wg.Wait()
}

func (d *Dispatcher) fanOut(ev Event) {
	targets := d.Subs.PushTargets()
	badge := d.Badge()
	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Go(func() {
			opts := Options{Urgency: "normal", TTL: time.Hour}
			if ev.Urgent {
				opts.Urgency = "high"
			}
			msg := Message{Title: ev.Title, Body: ev.Body, Path: ev.Path, Tag: ev.Tag, Badge: badge}
			out := d.Send(d.ctx, subscriptionOf(t.Sub), msg, opts)
			d.record(t.LoginID, t.Sub.Endpoint, out)
		})
	}
	wg.Wait()
}

func subscriptionOf(s webauth.PushSubscription) Subscription {
	return Subscription{Endpoint: s.Endpoint, P256dh: s.P256dh, Auth: s.Auth}
}

func (d *Dispatcher) record(loginID, endpoint string, out Outcome) {
	switch {
	case out.Gone:
		if err := d.Subs.DeletePushSubscriptionIfEndpoint(loginID, endpoint); err != nil {
			d.Log.Warn("push: could not delete a gone subscription", "login", loginID)
		}
	case out.Reason != "":
		d.Log.Warn("push failed", "login", loginID, "reason", out.Reason)
		d.mu.Lock()
		if d.failures == nil {
			d.failures = map[string]failure{}
		}
		d.failures[loginID] = failure{out.Reason, time.Now()}
		d.mu.Unlock()
	case out.Delivered:
		d.mu.Lock()
		delete(d.failures, loginID)
		d.mu.Unlock()
	}
}

// LastFailure is the login's most recent failed push, if no push has been delivered since.
func (d *Dispatcher) LastFailure(loginID string) (reason string, at time.Time, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f, ok := d.failures[loginID]
	return f.reason, f.at, ok
}

// TestPush sends one push to the login's own subscription: no merge, one attempt, no quiet check.
func (d *Dispatcher) TestPush(ctx context.Context, loginID string) Outcome {
	sub, ok := d.Subs.PushSubscriptionOf(loginID)
	if !ok {
		return Outcome{Reason: "This device has no push subscription."}
	}
	msg := Message{Title: "gx test push", Body: "Push notifications work on this device.", Path: "/", Tag: "test", Badge: d.Badge()}
	out := d.Send(ctx, subscriptionOf(sub), msg, Options{Urgency: "normal", Test: true})
	d.record(loginID, sub.Endpoint, out)
	return out
}
