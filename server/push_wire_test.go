package server

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/ralphloop/schedule"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/webpush"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
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
func pushServer(t *testing.T, now func() time.Time, tweak ...func(*Config)) (s *Server, hits *atomic.Int32, got chan http.Header) {
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
	cfg := Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir(),
		RestClock: now, PushHTTPClient: svc.Client(), PushMergeDelay: 10 * time.Millisecond,
		Settings:        Settings{MentalHealth: mh, WebPushContact: "me@example.com", NotifyChannels: allOn()},
		ExtraUsageCheck: func() subscription.State { return subscription.StateDisabled }}
	for _, f := range tweak {
		f(&cfg)
	}
	s, err = New(cfg)
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

// The Banner already merged for a second; the session sink must not hold the
// push again. An hour of merge delay makes any second hold a timeout.
func TestPush_SessionPushSkipsTheDispatcherHold(t *testing.T) {
	t.Parallel()
	s, _, got := pushServer(t, noonClock)
	s.push.Merge = time.Hour
	if s.sessions == nil {
		t.Fatal("sessions must exist")
	}
	bannerPush(s).SendPush("done", "body", "/sessions/x", "session:x", false)
	select {
	case <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("the session push was held by the dispatcher")
	}
}

func TestPush_NoWebListenerMeansNoTargets(t *testing.T) {
	t.Parallel()
	s, _, _ := pushServer(t, noonClock)
	adapter := pushSubs{s}
	if got := adapter.PushTargets(); len(got) != 0 {
		t.Fatalf("targets = %v", got)
	}
	if _, ok := adapter.PushSubscriptionOf("a"); ok {
		t.Fatal("subscription without a gate")
	}
}

func TestBadgeCount_ActionableSessionsPlusPausedTickets(t *testing.T) {
	t.Parallel()
	withReason := wire.SessionActionable{Reason: "needs you"}
	blocked := wire.SessionStatus{State: wire.SessionBlocked}
	for _, c := range []struct {
		name     string
		sessions []wire.SessionInfo
		tickets  []wire.TicketInfo
		want     int
	}{
		{"empty", nil, nil, 0},
		{"actionable reason", []wire.SessionInfo{{Actionable: withReason}}, nil, 1},
		{"blocked without reason", []wire.SessionInfo{{Status: blocked}}, nil, 1},
		{"idle session", []wire.SessionInfo{{}}, nil, 0},
		{"queue excluded", []wire.SessionInfo{{Queue: true, Actionable: withReason}}, nil, 0},
		{"design excluded", []wire.SessionInfo{{Design: true, Actionable: withReason}}, nil, 0},
		{"triage excluded", []wire.SessionInfo{{Flow: string(tickets.FlowTriage), Actionable: withReason}}, nil, 0},
		{"paused ticket", nil, []wire.TicketInfo{{Status: string(schema.StatusPaused)}}, 1},
		{"legacy paused", nil, []wire.TicketInfo{
			{Status: string(schema.StatusLegacyAnswer)}, {Status: string(schema.StatusLegacyRepair)},
		}, 2},
		{"map epic ticket excluded", nil, []wire.TicketInfo{{Status: string(schema.StatusPaused), EpicIsMap: true}}, 0},
		{"draft and error excluded", nil, []wire.TicketInfo{{Status: string(schema.StatusDraft)}, {Status: "error"}}, 0},
		{"sum", []wire.SessionInfo{{Actionable: withReason}, {Status: blocked}},
			[]wire.TicketInfo{{Status: string(schema.StatusPaused)}}, 3},
	} {
		if got := badgeCount(c.sessions, c.tickets); got != c.want {
			t.Errorf("%s: badgeCount = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestBadge_SnapshotEqualsLiveBadge(t *testing.T) {
	t.Parallel()
	stateDir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var snap wire.Snapshot
	rec := httptest.NewRecorder()
	rootRoutes{s}.snapshot(rec, httptest.NewRequest("GET", "/v1/snapshot", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Badge != liveBadge(s) {
		t.Fatalf("snapshot badge %d != liveBadge %d", snap.Badge, liveBadge(s))
	}
}

type sentPush struct {
	Title, Body, Path, Tag string
	Urgent                 bool
}

type pushLog struct {
	mu   sync.Mutex
	sent []sentPush
}

func (l *pushLog) SendPush(title, body, path, tag string, urgent bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = append(l.sent, sentPush{title, body, path, tag, urgent})
}

func (l *pushLog) all() []sentPush {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]sentPush(nil), l.sent...)
}

// recordedPushServer is a server whose pushes are recorded and whose Push
// channel is ch.
func recordedPushServer(t *testing.T, ch settings.NotifyChannel) (*Server, *pushLog) {
	t.Helper()
	log := &pushLog{}
	s, _, _ := pushServer(t, noonClock, func(c *Config) {
		c.SessionPush = log
		c.Settings.NotifyChannels = settings.NotifyChannels{Push: ch}
	})
	return s, log
}

func TestPushChannel_EveryNewKindSendsOneNormalPushWithTheChatHeadlineAndPath(t *testing.T) {
	t.Parallel()
	notice := func(kind string) func(*Server) {
		return func(s *Server) {
			s.chat.Notice(notify.ServerNotice{Kind: kind, Emoji: "ℹ️", Title: "Something happened", Detail: "details"})
		}
	}
	for _, tc := range []struct {
		name, kind, path string
		send             func(*Server)
	}{
		{"design ready", notifykind.DesignReady, "/d/proj:map", func(s *Server) { s.chat.DesignReady("proj", nil, "map", "impl", 2, "") }},
		{"chat question", notifykind.ChatQuestion, "/", func(s *Server) { s.chat.ChatQuestion("proj", nil, "fix", "which one?", "") }},
		{"one-off result", notifykind.OneOffResult, "/t/proj:epic/01", func(s *Server) {
			s.chat.Result("proj", nil, t.TempDir(), "epic", "", "01", "all done")
		}},
		{"conflict resolution started", notifykind.ConflictResolutionStarted, "/t/proj:epic/01", func(s *Server) {
			s.chat.FinishSink("proj", nil, "epic", "", "01", func() schedule.EpicCounts { return schedule.EpicCounts{} }).ConflictResolutionStarted("01")
		}},
		{"epic complete", notifykind.EpicComplete, "/e/proj:epic", func(s *Server) {
			s.chat.EpicComplete("proj", nil, "epic", schedule.EpicCounts{}, 60, 1, true)
		}},
		{"epic paused", notifykind.EpicPaused, "/e/proj:epic", func(s *Server) { s.chat.EpicPause("proj", nil, "epic", "Merge blocked", "dirty trunk") }},
		{"progress update", notifykind.ProgressUpdate, "/e/proj:epic", func(s *Server) { s.chat.Progress("proj", nil, "epic", notify.ProgressFacts{}) }},
		{"long ticket", notifykind.LongTicket, "/", notice("long-ticket")},
		{"budget and usage", notifykind.BudgetAndUsage, "/", notice("budget-soft-limit")},
		{"disk", notifykind.Disk, "/", notice("disk-warn")},
		{"project and trunk", notifykind.ProjectAndTrunk, "/", notice("trunk-found")},
		{"server started", notifykind.ServerStarted, "/", notice("server-started")},
		{"cli message", notifykind.CLIMessage, "/", func(s *Server) { s.chat.NotifyOutcomes("hello from the cli") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, log := recordedPushServer(t, settings.NotifyChannel{Enabled: true, Kinds: []string{tc.kind}})
			tc.send(s)
			got := log.all()
			if len(got) != 1 {
				t.Fatalf("pushes = %+v, want one", got)
			}
			if got[0].Title == "" || got[0].Path != tc.path || got[0].Urgent {
				t.Errorf("push = %+v, want a headline title, path %q and normal urgency", got[0], tc.path)
			}

			// The same event with the kind unchecked sends nothing.
			s, log = recordedPushServer(t, settings.NotifyChannel{Enabled: true, Kinds: []string{}})
			tc.send(s)
			if got := log.all(); len(got) != 0 {
				t.Errorf("unchecked kind pushed %+v", got)
			}
		})
	}
}

func TestPushChannel_ARouterPushIsOnePushWhateverTheProjectOverrideSays(t *testing.T) {
	t.Parallel()
	s, log := recordedPushServer(t, defaultOn())
	override := &notify.ServerChatConfig{}
	s.chat.EpicComplete("proj", override, "epic", schedule.EpicCounts{}, 60, 1, true)
	if got := log.all(); len(got) != 1 {
		t.Fatalf("pushes = %+v, want one", got)
	}
}

func TestPushChannel_SwitchOffSendsNothing(t *testing.T) {
	t.Parallel()
	s, log := recordedPushServer(t, settings.NotifyChannel{})
	s.chat.NotifyOutcomes("hi")
	if got := log.all(); len(got) != 0 {
		t.Errorf("pushes = %+v, want none", got)
	}
}

func TestPushChannel_TwoKindsOnOneTicketKeepTheirOwnTags(t *testing.T) {
	t.Parallel()
	s, log := recordedPushServer(t, defaultOn())
	s.chat.Result("proj", nil, t.TempDir(), "epic", "", "01", "done")
	s.chat.Pause("proj", nil, "epic", "", "01", "paused", "", "why", schedule.EpicCounts{})
	got := log.all()
	if len(got) != 2 || got[0].Tag == got[1].Tag || got[1].Tag != "ticket:proj:epic/01" {
		t.Fatalf("pushes = %+v, want two with different tags, the pause keeping ticket:<addr>", got)
	}
}

// pushWire is a server with the real dispatcher and the given devices.
func pushWire(t *testing.T, devices int, tweak ...func(*Config)) (*Server, *atomic.Int32, chan http.Header) {
	t.Helper()
	s, hits, got := pushServer(t, noonClock, tweak...)
	one := s.push.Subs.PushTargets()[0]
	var many []webauth.PushTarget
	for i := range devices {
		d := one
		d.LoginID = string(rune('a' + i))
		many = append(many, d)
	}
	s.push.Subs = targets(many)
	return s, hits, got
}

// switchAsked reports each time the dispatcher asks the server switch, which
// it does first thing when an event fires.
func switchAsked(s *Server) <-chan struct{} {
	asked := make(chan struct{}, 8)
	on := s.push.On
	s.push.On = func() bool {
		asked <- struct{}{}
		return on()
	}
	return asked
}

func awaitAsked(t *testing.T, asked <-chan struct{}) {
	t.Helper()
	select {
	case <-asked:
	case <-time.After(10 * time.Second):
		t.Fatal("the dispatcher never fired the event")
	}
}

type targets []webauth.PushTarget

func (o targets) PushTargets() []webauth.PushTarget { return o }
func (o targets) PushSubscriptionOf(string) (webauth.PushSubscription, bool) {
	return webauth.PushSubscription{}, false
}
func (o targets) DeletePushSubscriptionIfEndpoint(string, string) error { return nil }

func TestPushLayers_OneEventReachesEveryDevice(t *testing.T) {
	t.Parallel()
	s, hits, got := pushWire(t, 3)
	s.chat.NotifyOutcomes("hi")
	for range 3 {
		select {
		case <-got:
		case <-time.After(10 * time.Second):
			t.Fatalf("hits = %d, want 3 devices reached", hits.Load())
		}
	}
}

func TestPushLayers_NoSendWithTheSwitchOffNoEmailOrNoDevice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		devices int
		tweak   func(*Config)
	}{
		{"switch off", 1, func(c *Config) { c.Settings.NotifyChannels.Push.Enabled = false }},
		{"no email", 1, func(c *Config) { c.Settings.WebPushContact = "" }},
		{"no device", 0, func(*Config) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, hits, _ := pushWire(t, tc.devices, tc.tweak)
			asked := switchAsked(s)
			s.push.Push(webpush.Event{Title: "t", Tag: "x"})
			awaitAsked(t, asked)
			// Close waits for the event in flight, so a send would have landed by now.
			s.push.Close()
			if hits.Load() != 0 {
				t.Errorf("hits = %d, want none", hits.Load())
			}
		})
	}
}

func TestPushLayers_AnEventHeldWhileTheSwitchGoesOffIsDropped(t *testing.T) {
	t.Parallel()
	s, hits, _ := pushWire(t, 1)
	s.push.Merge = 200 * time.Millisecond
	asked := switchAsked(s)
	s.chat.NotifyOutcomes("held")
	cur := *s.settings.Load()
	cur.NotifyChannels.Push.Enabled = false
	s.settings.Store(&cur)
	awaitAsked(t, asked)
	s.push.Close()
	if hits.Load() != 0 {
		t.Errorf("hits = %d, want the held event dropped", hits.Load())
	}
}

func TestBadge_SnapshotHasNoBadgeWhileTheSwitchIsOffAndTheKindListDoesNotChangeTheCount(t *testing.T) {
	t.Parallel()
	s, _ := recordedPushServer(t, settings.NotifyChannel{Enabled: true, Kinds: []string{}})
	if !pushSwitchOn(s) {
		t.Fatal("an empty kind list turned the switch off")
	}
	snap := func(s *Server) wire.Snapshot {
		var out wire.Snapshot
		rec := httptest.NewRecorder()
		rootRoutes{s}.snapshot(rec, httptest.NewRequest(http.MethodGet, "/v1/snapshot", nil))
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got, want := snap(s).Badge, liveBadge(s); got != want {
		t.Errorf("badge %d != live %d with an empty kind list", got, want)
	}
	off, _ := recordedPushServer(t, settings.NotifyChannel{})
	if got := snap(off).Badge; got != 0 || liveBadge(off) != 0 {
		t.Errorf("badge = %d, live = %d, want 0 while the switch is off", got, liveBadge(off))
	}
}

// noonClock is the Rest clock for tests that expect notifications to send.
// The package pins time.Local to UTC+5, so a real clock lands inside the
// 22:00-06:00 bedtime for about eight hours of every day and mutes every channel.
func noonClock() time.Time { return time.Date(2026, 1, 5, 12, 0, 0, 0, time.Local) }
