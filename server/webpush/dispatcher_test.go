package webpush

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server/webauth"
)

const waitFor = 5 * time.Second

type sent struct {
	sub  Subscription
	msg  Message
	opts Options
}

type fakeSubs struct {
	mu      sync.Mutex
	targets []webauth.PushTarget
	deleted []string
}

func (f *fakeSubs) PushTargets() []webauth.PushTarget { return f.targets }

func (f *fakeSubs) PushSubscriptionOf(id string) (webauth.PushSubscription, bool) {
	for _, t := range f.targets {
		if t.LoginID == id {
			return t.Sub, true
		}
	}
	return webauth.PushSubscription{}, false
}

func (f *fakeSubs) DeletePushSubscriptionIfEndpoint(id, endpoint string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, id+"|"+endpoint)
	return nil
}

func target(id string) webauth.PushTarget {
	return webauth.PushTarget{LoginID: id, Sub: webauth.PushSubscription{Endpoint: "https://push.example.test/" + id + "-secret"}}
}

type dispFixture struct {
	d    *Dispatcher
	subs *fakeSubs
	logs *lockedWriter
	out  chan sent
	// result decides the Outcome for a send; nil means delivered.
	result func(sent) Outcome
}

func newDisp(t *testing.T, ids ...string) *dispFixture {
	t.Helper()
	f := &dispFixture{subs: &fakeSubs{}, logs: &lockedWriter{}, out: make(chan sent, 32)}
	for _, id := range ids {
		f.subs.targets = append(f.subs.targets, target(id))
	}
	f.d = &Dispatcher{
		Subs: f.subs,
		Send: func(_ context.Context, sub Subscription, msg Message, opts Options) Outcome {
			s := sent{sub, msg, opts}
			f.out <- s
			if f.result != nil {
				return f.result(s)
			}
			return Outcome{Delivered: true}
		},
		Badge: func() int { return 7 },
		Merge: 20 * time.Millisecond,
		Log:   slog.New(slog.NewTextHandler(f.logs, nil)),
	}
	t.Cleanup(f.d.Close)
	return f
}

type lockedWriter struct {
	mu sync.Mutex
	sb strings.Builder
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sb.Write(p)
}

func (l *lockedWriter) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sb.String()
}

func (f *dispFixture) next(t *testing.T) sent {
	t.Helper()
	select {
	case s := <-f.out:
		return s
	case <-time.After(waitFor):
		t.Fatal("no push sent")
		return sent{}
	}
}

func (f *dispFixture) none(t *testing.T) {
	t.Helper()
	select {
	case s := <-f.out:
		t.Fatalf("unexpected push %+v", s.msg)
	default:
	}
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.After(waitFor)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !ok() {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		case <-tick.C:
		}
	}
}

func TestDispatcher_SameTagMergesToTheNewestText(t *testing.T) {
	f := newDisp(t, "a")
	f.d.Push(Event{Title: "old", Tag: "x"})
	f.d.Push(Event{Title: "new", Tag: "x"})
	if got := f.next(t).msg.Title; got != "new" {
		t.Fatalf("title = %q", got)
	}
	f.d.Close()
	f.none(t)
}

func TestDispatcher_DifferentTagsSendSeparately(t *testing.T) {
	f := newDisp(t, "a")
	f.d.Push(Event{Title: "one", Tag: "x"})
	f.d.Push(Event{Title: "two", Tag: "y"})
	got := map[string]bool{f.next(t).msg.Tag: true, f.next(t).msg.Tag: true}
	if !got["x"] || !got["y"] {
		t.Fatalf("tags = %v", got)
	}
}

func TestDispatcher_PayloadCarriesBadgeUrgencyAndTTL(t *testing.T) {
	f := newDisp(t, "a", "b")
	f.d.Push(Event{Title: "t", Path: "/p", Tag: "x", Urgent: true})
	f.d.Push(Event{Title: "t", Path: "/p", Tag: "y"})
	for range 4 {
		s := f.next(t)
		wantUrgency := map[string]string{"x": "high", "y": "normal"}[s.msg.Tag]
		if s.msg.Badge != 7 || s.opts.Urgency != wantUrgency || s.opts.TTL != time.Hour || s.msg.Path != "/p" {
			t.Fatalf("send = %+v", s)
		}
	}
}

func TestDispatcher_PushNowSkipsTheMerge(t *testing.T) {
	f := newDisp(t, "a")
	f.d.Merge = time.Hour
	f.d.PushNow(Event{Title: "now", Tag: "x"})
	if got := f.next(t).msg.Title; got != "now" {
		t.Fatalf("title = %q", got)
	}
}

// An event held for the merge delay is dropped when the switch goes off before
// it fires; one event reaches every device while the switch is on.
func TestDispatcher_SwitchIsCheckedAfterTheMergeHold(t *testing.T) {
	f := newDisp(t, "a", "b", "c")
	var off atomic.Bool
	asked := make(chan struct{}, 8)
	f.d.On = func() bool {
		asked <- struct{}{}
		return !off.Load()
	}
	await := func() {
		t.Helper()
		select {
		case <-asked:
		case <-time.After(waitFor):
			t.Fatal("the dispatcher never asked the switch")
		}
	}

	f.d.Push(Event{Title: "on", Tag: "x"})
	got := map[string]bool{}
	for range 3 {
		got[f.next(t).sub.Endpoint] = true
	}
	await()
	if len(got) != 3 {
		t.Fatalf("one event reached %d devices, want 3", len(got))
	}

	// The switch goes off while the event is held; it fires, asks, and is dropped.
	f.d.Merge = 200 * time.Millisecond
	f.d.Push(Event{Title: "held", Tag: "y"})
	off.Store(true)
	await()
	f.none(t)
	f.d.PushNow(Event{Title: "now", Tag: "z"})
	await()
	f.none(t)
}

func TestDispatcher_ABlockedDeviceDoesNotHoldBackAnother(t *testing.T) {
	f := newDisp(t, "slow", "fast")
	release := make(chan struct{})
	done := make(chan string, 2)
	f.d.Send = func(_ context.Context, sub Subscription, _ Message, _ Options) Outcome {
		if strings.Contains(sub.Endpoint, "slow") {
			<-release
		}
		done <- sub.Endpoint
		return Outcome{Delivered: true}
	}
	f.d.Push(Event{Tag: "x"})
	select {
	case got := <-done:
		if !strings.Contains(got, "fast") {
			t.Fatalf("first to finish = %q", got)
		}
	case <-time.After(waitFor):
		t.Fatal("fast device never sent")
	}
	close(release)
}

func TestDispatcher_GoneDeletesTheSubscription(t *testing.T) {
	f := newDisp(t, "a")
	f.result = func(sent) Outcome { return Outcome{Gone: true} }
	f.d.Push(Event{Tag: "x"})
	f.next(t)
	waitUntil(t, "the delete", func() bool {
		f.subs.mu.Lock()
		defer f.subs.mu.Unlock()
		return len(f.subs.deleted) == 1
	})
	if f.subs.deleted[0] != "a|https://push.example.test/a-secret" {
		t.Fatalf("deleted = %v", f.subs.deleted)
	}
}

func TestDispatcher_FailureLogsOnceSetsLastFailureAndSuccessClearsIt(t *testing.T) {
	f := newDisp(t, "a")
	f.result = func(sent) Outcome { return Outcome{Reason: "service down"} }
	f.d.Push(Event{Tag: "x"})
	f.next(t)
	waitUntil(t, "the failure", func() bool { _, _, ok := f.d.LastFailure("a"); return ok })
	reason, at, _ := f.d.LastFailure("a")
	if reason != "service down" || at.IsZero() {
		t.Fatalf("failure = %q at %v", reason, at)
	}
	log := f.logs.String()
	if strings.Count(log, "\n") != 1 || strings.Contains(log, "secret") || strings.Contains(log, "push.example") {
		t.Fatalf("log = %q", log)
	}
	f.result = func(sent) Outcome { return Outcome{Delivered: true} }
	f.d.Push(Event{Tag: "y"})
	f.next(t)
	waitUntil(t, "the failure to clear", func() bool { _, _, ok := f.d.LastFailure("a"); return !ok })
}

func TestDispatcher_RestDropsARetryAndSendsNothingLater(t *testing.T) {
	f := newDisp(t, "a")
	f.result = func(sent) Outcome { return Outcome{Dropped: true} }
	f.d.Push(Event{Tag: "x"})
	f.next(t)
	f.d.Close()
	f.none(t)
	if _, _, ok := f.d.LastFailure("a"); ok {
		t.Fatal("a dropped push set Last push failed")
	}
	if strings.Contains(f.logs.String(), "failed") {
		t.Fatalf("log = %q", f.logs.String())
	}
}

func TestPush_TestPushMakesOneAttemptWithoutAQuietCheck(t *testing.T) {
	f := newDisp(t, "a", "b")
	out := f.d.TestPush(context.Background(), "b")
	s := f.next(t)
	if !out.Delivered || !s.opts.Test || s.msg.Tag != "test" || s.msg.Badge != 7 || !strings.Contains(s.sub.Endpoint, "/b-") {
		t.Fatalf("out %+v send %+v", out, s)
	}
	f.none(t)
	f.result = func(sent) Outcome { return Outcome{Reason: "nope"} }
	if out := f.d.TestPush(context.Background(), "a"); out.Reason != "nope" {
		t.Fatalf("out = %+v", out)
	}
	if reason, _, ok := f.d.LastFailure("a"); !ok || reason != "nope" {
		t.Fatalf("last failure = %q, %v", reason, ok)
	}
}

func TestDispatcher_NilIsANoOp(t *testing.T) {
	var d *Dispatcher
	d.Push(Event{Tag: "x"})
	d.Close()
}
