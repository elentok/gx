package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

type graceFixture struct {
	h          *servertest.Harness
	path       string
	store      string
	evs        <-chan server.Event
	ctx        context.Context
	startTitle string
}

// Seam: the start banner title shows the configured grace duration.
func TestHardLimitGrace_StartTitleShowsGrace(t *testing.T) {
	for grace, want := range map[time.Duration]string{
		60 * time.Second:       "gx: agents stop in 60s",
		90 * time.Second:       "gx: agents stop in 90s",
		600 * time.Millisecond: "gx: agents stop in 600ms",
	} {
		if got := server.HardGraceTitle(grace); got != want {
			t.Errorf("title(%v) = %q, want %q", grace, got, want)
		}
	}
}

// startGraceServer runs epic-a's one ticket as a live agent with today's spend
// at $8 and a $20 hard limit, and leaves epic-b unqueued.
func startGraceServer(t *testing.T, grace time.Duration, opts ...func(*server.Config)) graceFixture {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "second", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	path := filepath.Join(t.TempDir(), "config.json")
	opts = append([]func(*server.Config){servertest.WithConfigFile(path), func(c *server.Config) {
		c.Runner = runnerfake.NewRunner()
		c.BudgetHardLimit = 20
		c.BudgetKillGrace = 10 * time.Millisecond
		c.BudgetPollInterval = time.Hour
		c.HardLimitGrace = grace
	}}, opts...)
	h := servertest.StartWithStore(t, store, opts...)
	h.Server.RecordSpend("spent", 8)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil {
		t.Fatal(err)
	}
	waitRuns(ctx, t, h, 1)
	// A launching run has no session yet, so the stop would skip it and report
	// zero agents stopped.
	for h.Server.LaunchingRuns() > 0 {
		if ctx.Err() != nil {
			t.Fatalf("runs = %+v, want none launching", h.Server.Runs())
		}
		time.Sleep(10 * time.Millisecond)
	}
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	return graceFixture{h: h, path: path, store: store, evs: evs, ctx: ctx, startTitle: server.HardGraceTitle(grace)}
}

// lowerBelowSpend reloads a $6 hard limit and checks the reports: the start
// banner plus one plain reload banner, not a second one for the grace period.
func (f graceFixture) lowerBelowSpend(t *testing.T) {
	t.Helper()
	servertest.WriteConfig(t, f.path, f.store, budgetLimits(0, 6))
	res := reload(t, f.h)
	if res.Refused || len(res.Warnings) != 0 || !slices.Contains(res.Changed, "budget.hard-limit") {
		t.Fatalf("reload = %+v, want budget.hard-limit changed with no warnings", res)
	}
	ev := servertest.WaitForEvent(f.ctx, t, f.evs, server.EventConfigReloaded, "")
	want := f.startTitle + ": New hard budget limit ($6.00) is below today's spend ($8.00). 1 agents will stop. " +
		"Raise the limit in config.json or run `gx budget increase` to cancel."
	if ev.Level != server.EventLevelWarning || ev.Message != want {
		t.Fatalf("start event = %+v, want warning %q", ev, want)
	}
	ev = servertest.WaitForEvent(f.ctx, t, f.evs, server.EventConfigReloaded, "")
	if ev.Level != server.EventLevelSuccess || !strings.HasPrefix(ev.Message, "gx config reloaded successfully") {
		t.Fatalf("reload event = %+v, want the success banner", ev)
	}
	if !f.hasBanner(f.startTitle) {
		t.Errorf("banners = %+v, want the start banner", f.h.Banners.Sent())
	}
}

func TestHardLimitGrace_ConfigStatusShowsCountdown(t *testing.T) {
	f := startGraceServer(t, time.Hour)
	f.lowerBelowSpend(t)
	st, err := f.h.Client.ConfigStatus(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if left := time.Until(st.HardStopAt); left <= 0 || left > time.Hour {
		t.Errorf("hard stop at = %v, want within the next hour", st.HardStopAt)
	}
}

// hasBanner waits briefly for a banner titled title.
func (f graceFixture) hasBanner(title string) bool {
	for range 50 {
		for _, b := range f.h.Banners.Sent() {
			if b.Title == title {
				return true
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// waitBanners waits briefly for n banners titled title, and returns the count.
func (f graceFixture) waitBanners(title string, n int) int {
	got := 0
	for range 50 {
		if got = f.countBanners(title); got >= n {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return got
}

func (f graceFixture) countBanners(title string) int {
	n := 0
	for _, b := range f.h.Banners.Sent() {
		if b.Title == title {
			n++
		}
	}
	return n
}

func (f graceFixture) parkKind(t *testing.T) string {
	t.Helper()
	tk, err := schema.ParseTicket(filepath.Join(f.store, "proj", "epic-a", "issues", "01-first.md"))
	if err != nil {
		t.Fatal(err)
	}
	return tk.PauseReason.LegacyKind()
}

// expectHeld checks the grace period's effect: nothing killed, no new start.
func (f graceFixture) expectHeld(t *testing.T) {
	t.Helper()
	if _, err := f.h.Client.QueueAdd(f.ctx, "proj:epic-b/01", "claude"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if runs := f.h.Server.Runs(); len(runs) != 1 {
		t.Fatalf("runs = %+v, want only the running agent during the grace period", runs)
	}
	st := f.h.Server.BudgetStatusAt(time.Now())
	if st.HardLatched || !st.BudgetPaused || f.h.Server.HardStopAt().IsZero() {
		t.Errorf("status = %+v, want paused, unlatched, with a stop time", st)
	}
}

// expectCancelled checks a raise ended the grace period without a kill.
func (f graceFixture) expectCancelled(t *testing.T) {
	t.Helper()
	const title = "gx: agent stop cancelled — hard budget limit raised."
	if !f.hasBanner(title) {
		t.Fatalf("banners = %+v, want the cancel banner", f.h.Banners.Sent())
	}
	waitRuns(f.ctx, t, f.h, 2) // starts resume
	if st := f.h.Server.BudgetStatusAt(time.Now()); st.BudgetPaused || !f.h.Server.HardStopAt().IsZero() {
		t.Errorf("status = %+v, want unpaused with no stop time", st)
	}
	if kind := f.parkKind(t); kind != "" {
		t.Errorf("park kind = %q, want the agent still running", kind)
	}
}

// Seam: when the grace period ends with spend still at the limit, the agents
// stop through the budget-kill path and the done banner reports how many.
func TestHardLimitGrace_ExpiryStopsAgents(t *testing.T) {
	f := startGraceServer(t, 600*time.Millisecond)
	f.lowerBelowSpend(t)
	if kind := f.parkKind(t); kind != "" {
		t.Fatalf("park kind = %q right after the reload, want no kill yet", kind)
	}
	f.expectHeld(t)

	const done = "gx: 1 agents stopped — hard budget limit reached."
	ev := servertest.WaitForEvent(f.ctx, t, f.evs, server.EventConfigReloaded, "")
	for strings.HasPrefix(ev.Message, "gx config reloaded") {
		ev = servertest.WaitForEvent(f.ctx, t, f.evs, server.EventConfigReloaded, "")
	}
	if ev.Level != server.EventLevelError || ev.Message != done {
		t.Fatalf("done event = %+v, want error %q", ev, done)
	}
	if !f.hasBanner(done) {
		t.Errorf("banners = %+v, want the done banner", f.h.Banners.Sent())
	}
	if kind := f.parkKind(t); kind != string(events.BudgetKilled) {
		t.Errorf("park kind = %q, want budget-killed", kind)
	}
	if !f.h.Server.BudgetStatusAt(time.Now()).HardLatched {
		t.Error("hard limit not latched after the grace period")
	}
}

// Seam: a reload raising the hard limit above spend cancels the countdown.
func TestHardLimitGrace_ReloadRaiseCancels(t *testing.T) {
	f := startGraceServer(t, time.Hour)
	f.lowerBelowSpend(t)
	f.expectHeld(t)

	servertest.WriteConfig(t, f.path, f.store, budgetLimits(0, 30))
	reload(t, f.h)
	f.expectCancelled(t)
}

// Seam: `gx budget increase` above spend cancels the countdown too.
func TestHardLimitGrace_BudgetIncreaseCancels(t *testing.T) {
	f := startGraceServer(t, time.Hour)
	f.lowerBelowSpend(t)
	f.expectHeld(t)

	hard := 30.0
	if res, err := f.h.Client.BudgetIncrease(f.ctx, server.BudgetIncreaseRequest{Hard: &hard}); err != nil || res.Refused {
		t.Fatalf("increase = %+v, %v", res, err)
	}
	f.expectCancelled(t)
}

// Seam: `gx budget override` cancels the countdown, so a later lowering reload
// starts a fresh one instead of inheriting the old deadline.
func TestHardLimitGrace_OverrideCancels(t *testing.T) {
	f := startGraceServer(t, time.Hour)
	f.lowerBelowSpend(t)
	stopAt := f.h.Server.HardStopAt()
	f.expectHeld(t)

	if res, err := f.h.Client.BudgetOverride(f.ctx); err != nil || res.Refused {
		t.Fatalf("override = %+v, %v", res, err)
	}
	f.expectCancelled(t)
	if f.h.Server.HardGraceRunning() {
		t.Fatal("countdown still running after the override")
	}

	// Spend since the override stays under the $6 limit, so only the reload
	// lowering it to $5 reaches it.
	f.h.Server.RecordSpend("more", 5.5)
	servertest.WriteConfig(t, f.path, f.store, budgetLimits(0, 5))
	reload(t, f.h)
	if n := f.waitBanners(f.startTitle, 2); n != 2 {
		t.Errorf("start banners = %d, want a second one for the fresh countdown", n)
	}
	if got := f.h.Server.HardStopAt(); !got.After(stopAt) {
		t.Errorf("stop time = %v, want a fresh deadline after %v", got, stopAt)
	}
}

// Seam: grace expiry racing frequent budget polls sends no hard-limit chat
// notice, and stops the agents once.
func TestHardLimitGrace_ExpiryDuringPollsSendsNoChatNotice(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	hook := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
	}))
	t.Cleanup(hook.Close)
	f := startGraceServer(t, 300*time.Millisecond, func(c *server.Config) {
		c.BudgetPollInterval = time.Millisecond
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: hook.URL,
			GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: 20 * time.Millisecond}
	})
	f.h.Server.SetCostOf(func(server.IterationInfo) (float64, bool) { return 0, false })
	f.lowerBelowSpend(t)

	const done = "gx: 1 agents stopped — hard budget limit reached."
	time.Sleep(300 * time.Millisecond) // the countdown ends
	if !f.hasBanner(done) {
		t.Fatalf("banners = %+v, want the done banner", f.h.Banners.Sent())
	}
	time.Sleep(300 * time.Millisecond) // a chat batch would flush by now
	if n := f.countBanners(done); n != 1 {
		t.Errorf("done banners = %d, want 1", n)
	}
	if kind := f.parkKind(t); kind != string(events.BudgetKilled) {
		t.Errorf("park kind = %q, want budget-killed", kind)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, b := range bodies {
		if strings.Contains(b, "hard limit") {
			t.Errorf("chat got a hard-limit notice: %s", b)
		}
	}
}

// Seam: a watcher reload still in flight at shutdown cannot leave a countdown
// running after Serve returns.
func TestHardLimitGrace_ShutdownStopsCountdownAfterWatcher(t *testing.T) {
	f := startGraceServer(t, time.Hour, func(c *server.Config) {
		c.WatchConfig = true
		c.ConfigWatchDebounce = 10 * time.Millisecond
	})
	unlock := f.h.Server.LockReload()
	servertest.WriteConfig(t, f.path, f.store, budgetLimits(0, 6))
	time.Sleep(200 * time.Millisecond) // the watcher's reload now waits on the lock
	stopped := make(chan error, 1)
	go func() { stopped <- f.h.Stop() }()
	time.Sleep(200 * time.Millisecond) // shutdown now waits on the watcher
	unlock()
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if f.h.Server.HardGraceRunning() {
		t.Error("countdown still running after shutdown")
	}
}

// Seam: a second lowering reload during the countdown neither restarts it nor
// announces it again.
func TestHardLimitGrace_SecondLoweringKeepsOneTimer(t *testing.T) {
	f := startGraceServer(t, time.Hour)
	f.lowerBelowSpend(t)
	stopAt := f.h.Server.HardStopAt()

	servertest.WriteConfig(t, f.path, f.store, budgetLimits(0, 5))
	reload(t, f.h)
	time.Sleep(100 * time.Millisecond)
	if n := f.countBanners(f.startTitle); n != 1 {
		t.Errorf("start banners = %d, want 1", n)
	}
	if got := f.h.Server.HardStopAt(); !got.Equal(stopAt) {
		t.Errorf("stop time = %v, want unchanged %v", got, stopAt)
	}
}

// Seam: spend crossing the hard limit in normal work still stops agents at once.
func TestHardLimitGrace_SpendCrossingStillLatchesAtOnce(t *testing.T) {
	f := startGraceServer(t, time.Hour)
	f.h.Server.RecordSpend("more", 15)
	if st := f.h.Server.BudgetStatusAt(time.Now()); !st.HardLatched || !f.h.Server.HardStopAt().IsZero() {
		t.Errorf("status = %+v, want latched at once with no countdown", st)
	}
}
