package server

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/webpush"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/testutil/runnerfake"
)

type oneTarget struct{ webauth.PushTarget }

func (o oneTarget) PushTargets() []webauth.PushTarget { return []webauth.PushTarget{o.PushTarget} }
func (o oneTarget) PushSubscriptionOf(string) (webauth.PushSubscription, bool) {
	return o.Sub, true
}
func (o oneTarget) DeletePushSubscriptionIfEndpoint(string, string) error { return nil }

// pushServer builds a server whose push service is an httptest one and whose
// only device is subscribed to it. The web listener is off, so the real
// adapter has no gate; the test swaps in the one device.
func pushServer(t *testing.T, now func() time.Time) (s *Server, hits *atomic.Int32, got chan http.Header) {
	t.Helper()
	hits = new(atomic.Int32)
	got = make(chan http.Header, 8)
	svc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		got <- r.Header.Clone()
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(svc.Close)
	stateDir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	mh := config.DefaultMentalHealthConfig()
	mh.Enabled = true
	mh.Bedtime.Enabled = true
	mh.Bedtime.Start, mh.Bedtime.End = "22:00", "06:00"
	s, err = New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir(),
		RestClock: now, PushHTTPClient: svc.Client(), PushMergeDelay: 10 * time.Millisecond,
		Settings:        Settings{MentalHealth: mh, WebPushContact: "me@example.com"},
		ExtraUsageCheck: func() subscription.State { return subscription.StateDisabled }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.push.Close)
	ua, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	b64 := base64.RawURLEncoding
	s.push.Subs = oneTarget{webauth.PushTarget{LoginID: "a", Sub: webauth.PushSubscription{
		Endpoint: svc.URL + "/push/x", P256dh: b64.EncodeToString(ua.PublicKey().Bytes()), Auth: b64.EncodeToString(auth)}}}
	return s, hits, got
}

func TestPush_ServerSendsToTheServiceAndRestSendsNothing(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	clock := time.Date(2026, 1, 5, 12, 0, 0, 0, time.Local)
	now := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return clock
	}
	s, hits, got := pushServer(t, now)
	if s.push == nil || s.writer == nil || s.recovery == nil || s.sessions == nil {
		t.Fatal("push, writer, recovery and sessions must all exist")
	}

	s.push.Push(webpush.Event{Title: "t", Tag: "x", Urgent: true})
	select {
	case h := <-got:
		if h.Get("Urgency") != "high" || h.Get("Ttl") != "3600" {
			t.Fatalf("headers = %v", h)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the push service got nothing")
	}

	mu.Lock()
	clock = time.Date(2026, 1, 5, 23, 0, 0, 0, time.Local)
	mu.Unlock()
	if _, err := s.rest.Tick(); err != nil {
		t.Fatal(err)
	}
	if !s.rest.Quiet() {
		t.Fatal("bedtime should be quiet")
	}
	outcomes := make(chan webpush.Outcome, 1)
	send := s.push.Send
	s.push.Send = func(ctx context.Context, sub webpush.Subscription, msg webpush.Message, o webpush.Options) webpush.Outcome {
		out := send(ctx, sub, msg, o)
		outcomes <- out
		return out
	}
	s.push.Push(webpush.Event{Title: "t", Tag: "y"})
	select {
	case out := <-outcomes:
		if !out.Dropped {
			t.Fatalf("outcome = %+v, want dropped", out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the held push never reached the sender")
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1: a Rest period sends nothing", hits.Load())
	}
}

func TestPush_NoWebListenerMeansNoTargets(t *testing.T) {
	t.Parallel()
	s, _, _ := pushServer(t, time.Now)
	adapter := pushSubs{s}
	if got := adapter.PushTargets(); len(got) != 0 {
		t.Fatalf("targets = %v", got)
	}
	if _, ok := adapter.PushSubscriptionOf("a"); ok {
		t.Fatal("subscription without a gate")
	}
}
