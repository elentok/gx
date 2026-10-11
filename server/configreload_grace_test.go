package server_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

const graceStartTitle = "gx: agents stop in 60s"

type graceFixture struct {
	h     *servertest.Harness
	path  string
	store string
	evs   <-chan server.Event
	ctx   context.Context
}

// startGraceServer runs epic-a's one ticket as a live agent with today's spend
// at $8 and a $20 hard limit, and leaves epic-b unqueued.
func startGraceServer(t *testing.T, grace time.Duration) graceFixture {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "second", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) {
		c.Runner = runnerfake.NewRunner()
		c.BudgetHardLimit = 20
		c.BudgetKillGrace = 10 * time.Millisecond
		c.BudgetPollInterval = time.Hour
		c.HardLimitGrace = grace
	})
	h.Server.RecordSpend("spent", 8)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil {
		t.Fatal(err)
	}
	waitRuns(ctx, t, h, 1)
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	return graceFixture{h: h, path: path, store: store, evs: evs, ctx: ctx}
}

// lowerBelowSpend reloads a $6 hard limit and checks the start report.
func (f graceFixture) lowerBelowSpend(t *testing.T) {
	t.Helper()
	servertest.WriteConfig(t, f.path, f.store, budgetLimits(0, 6))
	reload(t, f.h)
	ev := servertest.WaitForEvent(f.ctx, t, f.evs, server.EventConfigReloaded, "")
	want := graceStartTitle + ": New hard budget limit ($6.00) is below today's spend ($8.00). 1 agents will stop. " +
		"Raise the limit in config.json or run `gx budget increase` to cancel."
	if ev.Level != server.EventLevelWarning || ev.Message != want {
		t.Fatalf("start event = %+v, want warning %q", ev, want)
	}
	if !f.hasBanner(graceStartTitle) {
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
	return string(tk.ParkKind)
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
	if st.HardLatched || !st.BudgetPaused || st.HardStopAt.IsZero() {
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
	if st := f.h.Server.BudgetStatusAt(time.Now()); st.BudgetPaused || !st.HardStopAt.IsZero() {
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

// Seam: a second lowering reload during the countdown neither restarts it nor
// announces it again.
func TestHardLimitGrace_SecondLoweringKeepsOneTimer(t *testing.T) {
	f := startGraceServer(t, time.Hour)
	f.lowerBelowSpend(t)
	stopAt := f.h.Server.BudgetStatusAt(time.Now()).HardStopAt

	servertest.WriteConfig(t, f.path, f.store, budgetLimits(0, 5))
	reload(t, f.h)
	time.Sleep(100 * time.Millisecond)
	if n := f.countBanners(graceStartTitle); n != 1 {
		t.Errorf("start banners = %d, want 1", n)
	}
	if got := f.h.Server.BudgetStatusAt(time.Now()).HardStopAt; !got.Equal(stopAt) {
		t.Errorf("stop time = %v, want unchanged %v", got, stopAt)
	}
}

// Seam: spend crossing the hard limit in normal work still stops agents at once.
func TestHardLimitGrace_SpendCrossingStillLatchesAtOnce(t *testing.T) {
	f := startGraceServer(t, time.Hour)
	f.h.Server.RecordSpend("more", 15)
	if st := f.h.Server.BudgetStatusAt(time.Now()); !st.HardLatched || !st.HardStopAt.IsZero() {
		t.Errorf("status = %+v, want latched at once with no countdown", st)
	}
}
