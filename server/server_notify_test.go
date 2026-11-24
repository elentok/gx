package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/sched"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/oneoff"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketlog"
)

// routeServer is a server whose Slack webhook is an httptest one and whose
// channels are set by ch.
func routeServer(t *testing.T, ch settings.NotifyChannels) *Server {
	t.Helper()
	hook := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(hook.Close)
	s, _, _ := pushServer(t, time.Now, func(c *Config) {
		c.Settings.NotifyChannels = ch
		c.Settings.Chat.SlackWebhookURL = hook.URL
	})
	t.Cleanup(s.chat.Close)
	return s
}

func outcomes(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var res wire.NotifyResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range res.Channels {
		got[c.Channel] = c.Outcome
	}
	if len(got) != 5 {
		t.Fatalf("channels = %v, want all five", got)
	}
	return got
}

func TestNotifyRoutes_OffChannelIsAResultNotAnHTTPError(t *testing.T) {
	t.Parallel()
	s := routeServer(t, settings.NotifyChannels{Slack: settings.NotifyChannel{Off: true}})
	rec := httptest.NewRecorder()

	rootRoutes{s}.notifySend(rec, httptest.NewRequest("POST", "/v1/notify/send", strings.NewReader(`{"message":"hi"}`)))

	got := outcomes(t, rec)
	if got["slack"] != notify.OutcomeOff {
		t.Errorf("slack = %q, want %q (all: %v)", got["slack"], notify.OutcomeOff, got)
	}
	if got["telegram"] != notify.OutcomeUnconfigured {
		t.Errorf("telegram = %q, want %q", got["telegram"], notify.OutcomeUnconfigured)
	}
}

func TestNotifyRoutes_SendAppliesTheKindListAndTestSkipsIt(t *testing.T) {
	t.Parallel()
	none := settings.NotifyChannel{Kinds: []string{}}
	s := routeServer(t, settings.NotifyChannels{Desktop: none, WebToast: none, Slack: none, Push: none})

	send := httptest.NewRecorder()
	rootRoutes{s}.notifySend(send, httptest.NewRequest("POST", "/v1/notify/send", strings.NewReader(`{"message":"hi"}`)))
	test := httptest.NewRecorder()
	rootRoutes{s}.notifyTest(test, httptest.NewRequest("POST", "/v1/notify/test", nil))

	gotSend := outcomes(t, send)
	for _, ch := range []string{"desktop", "web-toast", "slack", "push"} {
		if gotSend[ch] != notify.OutcomeUnchecked {
			t.Errorf("send: %s = %q, want %q", ch, gotSend[ch], notify.OutcomeUnchecked)
		}
	}
	gotTest := outcomes(t, test)
	for _, ch := range []string{"desktop", "web-toast", "slack", "push"} {
		if gotTest[ch] != notify.OutcomeSent {
			t.Errorf("test: %s = %q, want %q: a test skips the kind list", ch, gotTest[ch], notify.OutcomeSent)
		}
	}
}

func TestNotifyRoutes_SendNeedsAMessage(t *testing.T) {
	t.Parallel()
	s := routeServer(t, settings.NotifyChannels{})
	rec := httptest.NewRecorder()

	rootRoutes{s}.notifySend(rec, httptest.NewRequest("POST", "/v1/notify/send", strings.NewReader(`{}`)))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// chatServer is a server wired to a fake Slack webhook that records each body.
// It keeps the real 6s batch tick, so notices sent before s.chat.Close() share
// one batch; use chatServerFlushing to wait on the tick instead.
func chatServer(t *testing.T, soft, hard float64) (*Server, func(n int) []string) {
	t.Helper()
	return chatServerFlushing(t, soft, hard, 0)
}

// chatServerFlushing is chatServer with the batch tick set to flush (zero
// means the real one).
func chatServerFlushing(t *testing.T, soft, hard float64, flush time.Duration) (*Server, func(n int) []string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	arrived := make(chan struct{}, 1) // a wake-up for wait; one pending signal is enough
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		select {
		case arrived <- struct{}{}:
		default:
		}
	}))
	t.Cleanup(hook.Close)
	stateDir, err := os.MkdirTemp("", "gx") // short path: unix socket limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir(), Settings: Settings{BudgetSoftLimit: soft, BudgetHardLimit: hard,
		Chat: notify.ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: flush}},
		// Not the real ~/.claude.json, which could add an extra-usage notice.
		ExtraUsageCheck: func() subscription.State { return subscription.StateDisabled }})
	if err != nil {
		t.Fatal(err)
	}
	// wait returns what was sent once n messages arrived, plus a grace period
	// for an extra one to show. n < 0 returns at once: the caller has already
	// called s.chat.Close(), which sends what is queued and waits for it.
	wait := func(n int) []string {
		if n >= 0 {
			timeout := time.After(10 * time.Second)
		arrive:
			for {
				mu.Lock()
				got := len(bodies)
				mu.Unlock()
				if got >= n {
					break
				}
				select {
				case <-arrived:
				case <-timeout:
					break arrive
				}
			}
			time.Sleep(300 * time.Millisecond) // negative wait: anything extra would show up now
		}
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
	return s, wait
}

type recNotifier struct{ n atomic.Int32 }

func (r *recNotifier) Notify(context.Context, osnotify.Notification) error {
	r.n.Add(1)
	return nil
}

type recPlayer struct{ n atomic.Int32 }

func (p *recPlayer) Play(context.Context, config.AgentsSoundsConfig, sound.Name) { p.n.Add(1) }

func TestQuiet_RestPeriodSilencesBannerSoundAndChatTogether(t *testing.T) {
	t.Parallel()
	var posts atomic.Int32
	arrived := make(chan struct{}, 8)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		arrived <- struct{}{}
	}))
	t.Cleanup(hook.Close)
	stateDir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	mh := config.DefaultMentalHealthConfig()
	mh.Enabled = true
	mh.Bedtime.Enabled = true
	mh.Bedtime.Start, mh.Bedtime.End = "22:00", "06:00"
	var clockMu sync.Mutex
	now := time.Date(2026, 1, 5, 12, 0, 0, 0, time.Local)
	setNow := func(t time.Time) {
		clockMu.Lock()
		defer clockMu.Unlock()
		now = t
	}
	clock := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return now
	}
	notifier, player := &recNotifier{}, &recPlayer{}
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir(),
		Notifier: notifier, Player: player, RestClock: clock,
		Settings: Settings{MentalHealth: mh,
			Chat: notify.ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: 20 * time.Millisecond}},
		ExtraUsageCheck: func() subscription.State { return subscription.StateDisabled }})
	if err != nil {
		t.Fatal(err)
	}
	send := func() {
		_ = s.cfg.Notifier.Notify(t.Context(), osnotify.Notification{Title: "t"})
		s.cfg.Player.Play(t.Context(), config.AgentsSoundsConfig{}, sound.Done)
		s.chat.SessionAlert("p", nil, "name", "done", "body", true)
	}

	send() // outside a Rest period all three go out
	select {
	case <-arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("no chat post outside Rest")
	}
	if notifier.n.Load() != 1 || player.n.Load() != 1 || posts.Load() != 1 {
		t.Fatalf("outside Rest: banners=%d sounds=%d chat=%d, want 1 each", notifier.n.Load(), player.n.Load(), posts.Load())
	}

	setNow(time.Date(2026, 1, 5, 23, 0, 0, 0, time.Local))
	if _, err := s.rest.Tick(); err != nil {
		t.Fatal(err)
	}
	if !s.rest.Quiet() {
		t.Fatal("bedtime should be quiet")
	}
	send()
	s.chat.Close() // flushes what was queued
	if notifier.n.Load() != 1 || player.n.Load() != 1 || posts.Load() != 1 {
		t.Errorf("during Rest: banners=%d sounds=%d chat=%d, want still 1 each", notifier.n.Load(), player.n.Load(), posts.Load())
	}
}

func TestServerNotices_RestartSendsOneServerLevelMessage(t *testing.T) {
	t.Parallel()
	s, wait := chatServerFlushing(t, 0, 0, 100*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()

	got := wait(1)
	cancel()
	<-done
	if len(got) != 1 || !strings.Contains(got[0], "Server started") || !strings.Contains(got[0], "Build ") {
		t.Fatalf("sends = %v, want one 'server started'", got)
	}
}

func TestServerNotices_BudgetLimitLatchesOncePerDay(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 10, 20)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.budget.SeedNotes(now)

	s.budget.RecordSpendAt("a", 11, now)
	s.budget.NotifyAt(now)
	s.budget.RecordSpendAt("a", 12, now.Add(time.Minute))
	s.budget.NotifyAt(now.Add(time.Minute)) // still over the soft limit: no repeat
	s.budget.RecordSpendAt("a", 21, now.Add(2*time.Minute))
	s.budget.NotifyAt(now.Add(2 * time.Minute))

	s.chat.Close()
	got := wait(-1)
	if len(got) != 1 {
		t.Fatalf("sends = %d, want the soft and hard notices batched in 1: %v", len(got), got)
	}
	if !strings.Contains(got[0], "soft limit") || !strings.Contains(got[0], "hard limit") {
		t.Errorf("message lacks a limit notice: %s", got[0])
	}
}

func TestServerNotices_BudgetAlreadyOverAtStartIsNotAnnounced(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 10, 0)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.budget.RecordSpendAt("a", 11, now)
	s.budget.SeedNotes(now)
	s.budget.NotifyAt(now)

	s.chat.Close()
	if got := wait(-1); len(got) != 0 {
		t.Fatalf("sends = %v, want none", got)
	}
}

func TestServerNotices_BudgetLimitsCountSpendBeyondOverride(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 20, 50)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.budget.SeedNotes(now)

	s.budget.RecordSpendAt("a", 30, now)
	s.budget.NotifyAt(now)
	if _, err := s.budget.OverrideAt(now); err != nil {
		t.Fatal(err)
	}
	s.budget.RecordSpendAt("a", 45, now.Add(time.Minute))
	s.budget.NotifyAt(now.Add(time.Minute)) // 15 beyond the override: nothing
	s.budget.RecordSpendAt("a", 55, now.Add(2*time.Minute))
	s.budget.NotifyAt(now.Add(2 * time.Minute)) // 25 beyond: soft again, not hard
	s.budget.RecordSpendAt("a", 85, now.Add(3*time.Minute))
	s.budget.NotifyAt(now.Add(3 * time.Minute)) // 55 beyond: hard

	s.chat.Close()
	all := strings.Join(wait(-1), "\n")
	if n := strings.Count(all, "soft limit reached"); n != 2 {
		t.Errorf("soft notices = %d, want 2: %s", n, all)
	}
	if n := strings.Count(all, "hard limit reached"); n != 1 {
		t.Errorf("hard notices = %d, want 1: %s", n, all)
	}
	for _, want := range []string{"$25.00 of $20.00", `$55.00 of $50.00 today\nDispatch is held.`} {
		if !strings.Contains(all, want) {
			t.Errorf("missing post-override detail %q: %s", want, all)
		}
	}
}

func TestServerNotices_RestartAfterOverrideSeedsFromSpendBeyondIt(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 20, 0)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.budget.RecordSpendAt("a", 30, now)
	if _, err := s.budget.OverrideAt(now); err != nil {
		t.Fatal(err)
	}
	s.budget.RecordSpendAt("a", 45, now.Add(time.Minute))
	s.budget.SeedNotes(now.Add(time.Minute)) // restart: 15 beyond, not yet crossed
	s.budget.NotifyAt(now.Add(time.Minute))
	s.budget.RecordSpendAt("a", 55, now.Add(2*time.Minute))
	s.budget.NotifyAt(now.Add(2 * time.Minute))

	s.chat.Close()
	all := strings.Join(wait(-1), "\n")
	if n := strings.Count(all, "soft limit reached"); n != 1 {
		t.Errorf("soft notices = %d, want 1: %s", n, all)
	}
}

func TestServerNotices_IncreaseRearmsAgainstSpendBeyondOverride(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 20, 0)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.budget.SeedNotes(now)
	s.budget.RecordSpendAt("a", 30, now)
	if _, err := s.budget.OverrideAt(now); err != nil {
		t.Fatal(err)
	}
	s.budget.RecordSpendAt("a", 55, now.Add(time.Minute))
	s.budget.NotifyAt(now.Add(time.Minute)) // 25 beyond: soft
	soft := 30.0
	if _, err := s.budget.IncreaseAt(now.Add(time.Minute), wire.BudgetIncreaseRequest{Soft: &soft}); err != nil {
		t.Fatal(err)
	}
	s.budget.RecordSpendAt("a", 62, now.Add(2*time.Minute))
	s.budget.NotifyAt(now.Add(2 * time.Minute)) // 32 beyond the raised 30: soft again

	s.chat.Close()
	all := strings.Join(wait(-1), "\n")
	if n := strings.Count(all, "soft limit reached"); n != 2 {
		t.Errorf("soft notices = %d, want 2: %s", n, all)
	}
}

func TestServerNotices_DayRolloverSendsTheSummary(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 0, 0)
	now := time.Date(2026, 10, 6, 23, 59, 0, 0, time.Local)
	s.budget.SeedNotes(now)
	s.budget.RecordSpendAt("a", 4, now)
	s.budget.NotifyAt(now) // same day: nothing

	s.budget.NotifyAt(now.Add(2 * time.Minute))
	s.chat.Close()
	got := wait(-1)
	if len(got) != 1 || !strings.Contains(got[0], "Daily summary 2026-10-06") || strings.Contains(got[0], "2026-10-06:") {
		t.Fatalf("sends = %v, want one daily summary for 2026-10-06", got)
	}
}

func TestNotifyPause_ShowsRealStatusKindAndReason(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 0, 0)
	addr := tickets.Address{Project: "p", Epic: "epic", ID: "01"}

	s.writer.NotifyPause(addr, "", events.BudgetKilled, "daily budget hard limit reached")
	s.writer.NotifyPause(tickets.Address{Project: "p", Epic: "epic", ID: "02"}, "", events.AmbiguousBase, "choose a base")

	s.chat.Close()
	all := strings.Join(wait(-1), "\n")
	for _, want := range []string{"🛑 Repair epic/01", "daily budget hard limit reached", "🙋 Answer epic/02", "choose a base", `\np\n\n---`} {
		if !strings.Contains(all, want) {
			t.Errorf("chat lacks %q: %s", want, all)
		}
	}
	// The recorder keeps the JSON body, so newlines are still escaped.
	if !strings.HasSuffix(all, `\np"}`) {
		t.Errorf("last line is not the project alone: %q", all)
	}
	for _, banned := range []string{"[gx]", "*p*", "needs repair", "needs answer"} {
		if strings.Contains(all, banned) {
			t.Errorf("chat has old-layout %q: %s", banned, all)
		}
	}
	if strings.Contains(all, "iteration ended without landing") {
		t.Errorf("hardcoded reason leaked: %s", all)
	}
}

func TestNotifyPause_SelfReportedShowsQuestionSummary(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 0, 0)
	path := filepath.Join(t.TempDir(), "01-x.md")
	if err := os.WriteFile(path, []byte("---\nid: \"01\"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary := "Should the cache be per-project?"
	if _, err := ticketlog.AppendLog(path, ticketlog.LogEvent{
		Type: ticketlog.EventPauseQuestion, By: "agent", Summary: summary, Handoff: "h",
	}); err != nil {
		t.Fatal(err)
	}

	s.writer.NotifyPause(tickets.Address{Project: "p", Epic: "epic", ID: "01"}, path, events.SelfReported, "agent reported asking via iteration_status")

	s.chat.Close()
	all := strings.Join(wait(-1), "\n")
	if !strings.Contains(all, summary) {
		t.Errorf("chat lacks the question summary: %s", all)
	}
	if strings.Contains(all, "left no question") || strings.Contains(all, "agent reported asking") {
		t.Errorf("generic reason leaked: %s", all)
	}
}

func TestNotifyResult_OnlyWhenTheSubmitAskedForIt(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 0, 0)
	dir := filepath.Join(s.cfg.TicketStore, "scratch", "e", "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Dir(dir))
	write := func(name string, notify bool) string {
		return writeResultTicket(t, dir, name, notify, "the answer is 42")
	}
	addr := tickets.Address{Project: "scratch", Epic: "e", ID: "01"}

	s.oneOffs.NotifyResult(addr, write("01-quiet.md", false))
	s.chat.Close()
	if got := wait(-1); len(got) != 0 {
		t.Fatalf("a ticket without --notify sent %v", got)
	}

	s, wait = chatServer(t, 0, 0)
	s.oneOffs.NotifyResult(addr, write("01-loud.md", true))
	s.chat.Close()
	got := wait(1)
	if len(got) != 1 || !strings.Contains(got[0], "✅ Done e/01 loud") || !strings.Contains(got[0], "the answer is 42") {
		t.Fatalf("sends = %v, want the Done headline and the Result", got)
	}
}

func TestNotifyResult_EmptyAndLongResults(t *testing.T) {
	t.Parallel()
	send := func(result string) string {
		s, wait := chatServer(t, 0, 0)
		dir := filepath.Join(s.cfg.TicketStore, "scratch", "e", "issues")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.EnsureEpicTicketMD(t, filepath.Dir(dir))
		path := writeResultTicket(t, dir, "01-loud.md", true, result)
		s.oneOffs.NotifyResult(tickets.Address{Project: "scratch", Epic: "e", ID: "01"}, path)
		s.chat.Close()
		got := wait(1)
		if len(got) != 1 {
			t.Fatalf("sends = %v, want one", got)
		}
		return got[0]
	}

	if got := send(""); !strings.Contains(got, "No result recorded.") {
		t.Errorf("empty Result sent %q, want 'No result recorded.'", got)
	}
	got := send(strings.Repeat("y", 1600))
	if !strings.Contains(got, strings.Repeat("y", 1500)+"…") || strings.Contains(got, strings.Repeat("y", 1501)) {
		t.Errorf("long Result not cut at 1500 with an ellipsis: %q", got)
	}
}

func writeResultTicket(t *testing.T, dir, name string, notify bool, result string) string {
	t.Helper()
	tk := schema.Ticket{ID: "01", Status: schema.StatusDone, Type: schema.TypePrompt, Notify: notify}
	out, err := schema.MarshalTicket(tk, "\n# loud\n\nprompt\n\n## Result\n\n"+result+"\n")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// A merged epic ticket sends no message of its own; the next Progress message
// reports it once, and a one-off never counts.
func TestNotifyDone_EpicTicketFeedsTheNextProgressOneOffStaysOut(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 0, 0)
	proj := filepath.Join(s.cfg.TicketStore, "p")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "project.json"), []byte(`{"repo":"/x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(epic, name string, tk schema.Ticket) string {
		dir := filepath.Join(proj, epic, "issues")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.EnsureEpicTicketMD(t, filepath.Dir(dir))
		out, err := schema.MarshalTicket(tk, "\nbody\n")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	merged := write("epic", "01-landed-ticket.md", schema.Ticket{ID: "01", Status: schema.StatusDone, Type: schema.TypeImplement, ElapsedTime: 332, ActualCost: 1.5})
	write("epic", "02-next.md", schema.Ticket{ID: "02", Status: schema.StatusOpen, Type: schema.TypeImplement})
	oneOff := write("cron", oneoff.OneOffFile, schema.Ticket{ID: "01", Status: schema.StatusDone, Type: schema.TypePrompt, ElapsedTime: 99})
	if !s.registry.PutIfAbsent(runs.TrackedRun{Run: runs.Run{Address: "p:epic/02"}}) {
		t.Fatal("run not registered")
	}

	t0 := time.Now()
	s.progress.Tick(t0) // the epic's timer starts
	s.launcher.NotifyDone(tickets.Address{Project: "p", Epic: "cron", ID: "01"}, oneOff)
	s.launcher.NotifyDone(tickets.Address{Project: "p", Epic: "epic", ID: "01"}, merged)
	s.progress.Tick(t0.Add(sched.DefaultProgressInterval))
	s.progress.Tick(t0.Add(2 * sched.DefaultProgressInterval))
	s.chat.Close()
	all := strings.Join(wait(-1), "\n")
	if n := strings.Count(all, "Finished since the last update"); n != 1 {
		t.Fatalf("finished lines = %d, want 1 in:\n%s", n, all)
	}
	for _, want := range []string{"Finished since the last update: 1 ticket · 5m · $1.50"} {
		if !strings.Contains(all, want) {
			t.Errorf("progress lacks %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "Landed ticket") || strings.Contains(all, "1m39s") {
		t.Errorf("a finished message or the one-off leaked:\n%s", all)
	}
}

// crossEpicFixture is project "p" with epic "epic" whose two done tickets are
// blocked_by ref, a qualified ticket address in a second epic of the project
// whose only ticket, other/01, is done.
func crossEpicFixture(t *testing.T, s *Server, ref string) {
	t.Helper()
	proj := filepath.Join(s.cfg.TicketStore, "p")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "project.json"), []byte(`{"repo":"/x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(epic, name string, tk schema.Ticket) {
		dir := filepath.Join(proj, epic, "issues")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.EnsureEpicTicketMD(t, filepath.Dir(dir))
		out, err := schema.MarshalTicket(tk, "\nbody\n")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	blocked := []schema.TicketID{schema.TicketID(ref)}
	write("epic", "01-a.md", schema.Ticket{ID: "01", Status: schema.StatusDone, Type: schema.TypeImplement, BlockedBy: blocked})
	write("epic", "02-b.md", schema.Ticket{ID: "02", Status: schema.StatusDone, Type: schema.TypeImplement, BlockedBy: blocked})
	write("other", "01-c.md", schema.Ticket{ID: "01", Status: schema.StatusDone, Type: schema.TypeImplement})
}

func TestCompleteRootIfDone_ResolvedCrossEpicBlockersCompleteTheEpic(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 0, 0)
	crossEpicFixture(t, s, "other/01")

	s.merger.CompleteRootIfDone(queue.RootRef{Project: "p", Epic: "epic"}, ralphloop.OneIteration{Epic: "epic"})
	s.chat.Close()
	got := wait(1)
	if len(got) != 1 || !strings.Contains(got[0], "Complete epic") || !strings.Contains(got[0], "2 done · 2 total") {
		t.Fatalf("sends = %v, want one epic-complete message with 2 done · 2 total", got)
	}
}

func TestNotifyDone_CrossEpicBlockersDoNotUndercountDoneTickets(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 0, 0)
	crossEpicFixture(t, s, "other/01")
	if !s.registry.PutIfAbsent(runs.TrackedRun{Run: runs.Run{Address: "p:epic/03"}}) {
		t.Fatal("run not registered")
	}

	t0 := time.Now()
	s.progress.Tick(t0)
	s.launcher.NotifyDone(tickets.Address{Project: "p", Epic: "epic", ID: "02"}, "")
	s.progress.Tick(t0.Add(sched.DefaultProgressInterval))
	s.chat.Close()
	got := strings.Join(wait(-1), "\n")
	if !strings.Contains(got, "Finished since the last update: 1 ticket") || !strings.Contains(got, "2 done") {
		t.Fatalf("sends = %v, want one finished ticket and 2 done", got)
	}
}

// A ref that names no ticket stays an error, which is never terminal.
func TestCompleteRootIfDone_UnresolvedCrossEpicBlockerKeepsTheEpicOpen(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 0, 0)
	crossEpicFixture(t, s, "other/99")

	s.merger.CompleteRootIfDone(queue.RootRef{Project: "p", Epic: "epic"}, ralphloop.OneIteration{Epic: "epic"})
	s.chat.Close()
	if got := wait(-1); len(got) != 0 {
		t.Fatalf("sends = %v, want none", got)
	}
}

// With no GateStatePath configured, the gate state lands under the server's
// state dir, not the real one.
func TestServerChat_EmptyGateStatePathUsesTheStateDir(t *testing.T) {
	t.Parallel()
	hook := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(hook.Close)
	stateDir, err := os.MkdirTemp("", "gx") // short path: unix socket limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir(),
		Settings:        Settings{Chat: notify.ServerChatConfig{SlackWebhookURL: hook.URL}},
		ExtraUsageCheck: func() subscription.State { return subscription.StateDisabled }})
	if err != nil {
		t.Fatal(err)
	}

	s.writer.NotifyPause(tickets.Address{Project: "p", Epic: "epic", ID: "01"}, "", events.BudgetKilled, "why")
	s.chat.Close()

	if _, err := os.Stat(notify.NotificationStatePath(stateDir)); err != nil {
		t.Errorf("gate state not under the server's state dir: %v", err)
	}
}

// The read path (index scan, then projectEpics) resolves a cross-epic blocker:
// a done ticket stays done instead of failing closed to error.
func TestProjectEpics_CrossEpicBlockerReadsAsDone(t *testing.T) {
	t.Parallel()
	s, _ := chatServer(t, 0, 0)
	crossEpicFixture(t, s, "other/01")
	if err := s.idx.Refresh(); err != nil {
		t.Fatal(err)
	}

	epics, err := s.projects.Epics("p")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range epics {
		for _, tk := range e.Tickets {
			if got := e.RenderedStatus(tk); got != tickets.StatusDone {
				t.Errorf("%s/%s = %v, want done", e.Name, tk.Identifier, got)
			}
		}
	}
}
