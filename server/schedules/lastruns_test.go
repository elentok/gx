package schedules_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/schedules"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire/ops"
)

func ts(h, m, s int) string { return at(h, m).Add(time.Duration(s) * time.Second).Format(time.RFC3339) }

func (r *rig) seed(t *testing.T, epic, id, machine string, opts servertest.TicketOpts) {
	t.Helper()
	opts.Unique = "schedule:" + r.key.String() + "@" + machine
	opts.Type = "prompt"
	servertest.WriteTicketWith(t, r.h.TicketStore, "proj", epic, id, "run"+id, opts)
}

func (r *rig) lastRuns(t *testing.T) []ops.ScheduleRun {
	t.Helper()
	got, err := schedules.LastRuns(r.h.TicketStore, r.h.Server.Schedules(), r.key, 2)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestLastRuns_StatusTable(t *testing.T) {
	t.Parallel()
	m := "m1"
	sched := ts(9, 0, 0)
	cases := []struct {
		id, epic string
		opts     servertest.TicketOpts
		want     ops.ScheduleRunStatus
	}{
		{"01", "one-offs", servertest.TicketOpts{ScheduledFor: sched}, ops.ScheduleRunQueued},
		{"02", "one-offs", servertest.TicketOpts{ScheduledFor: sched, ClaimedAt: ts(9, 0, 5)}, ops.ScheduleRunQueued},
		{"03", "one-offs", servertest.TicketOpts{Status: "claimed", ScheduledFor: sched, ClaimedAt: ts(9, 0, 5)}, ops.ScheduleRunRunning},
		{"04", "one-offs-archived", servertest.TicketOpts{Status: "done", ScheduledFor: sched, CreatedAt: ts(9, 0, 60)}, ops.ScheduleRunFinished},
		{"05", "one-offs-archived", servertest.TicketOpts{Status: "done", ScheduledFor: sched, CreatedAt: ts(9, 1, 1)}, ops.ScheduleRunRanLate},
		{"06", "one-offs", servertest.TicketOpts{Status: "claimed", ScheduledFor: sched, CreatedAt: ts(9, 30, 0)}, ops.ScheduleRunRunning},
		{"07", "one-offs", servertest.TicketOpts{Status: "done"}, ops.ScheduleRunFinished},
		{"08", "one-offs", servertest.TicketOpts{PauseReason: "question"}, ops.ScheduleRunPaused},
		{"09", "one-offs", servertest.TicketOpts{Status: "needs-answer"}, ops.ScheduleRunPaused}, // oldword:ok legacy status input
		{"10", "one-offs", servertest.TicketOpts{Status: "needs-repair"}, ops.ScheduleRunPaused}, // oldword:ok legacy status input
		{"11", "one-offs-archived", servertest.TicketOpts{Status: "cancelled", CancelledAt: ts(9, 5, 0)}, ops.ScheduleRunCancelled},
	}
	// Ten rows is the cap, so the eleven cases go through two rigs.
	for _, group := range [][]int{{0, 5}, {5, len(cases)}} {
		r := start(t, 2, "09:00")
		part := cases[group[0]:group[1]]
		for _, c := range part {
			r.seed(t, c.epic, c.id, m, c.opts)
		}
		servertest.WriteTicketWith(t, r.h.TicketStore, "proj", "one-offs", "99", "foreign", servertest.TicketOpts{Unique: "schedule:proj/" + r.key.Slug + "-b@x"})
		got := map[string]ops.ScheduleRunStatus{}
		for _, row := range r.lastRuns(t) {
			got[row.Address] = row.Status
		}
		if len(got) != len(part) {
			t.Errorf("got %d rows, want %d: %v", len(got), len(part), got)
		}
		for _, c := range part {
			if st := got["proj:"+c.epic+"/"+c.id]; st != c.want {
				t.Errorf("ticket %s: status %q, want %q", c.id, st, c.want)
			}
		}
	}
}

func TestLastRuns_OnlyNewestTenAndRunNowAndSkipShapes(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	for i := 1; i <= 9; i++ {
		r.seed(t, "one-offs", fmt.Sprintf("%02d", i), "m1", servertest.TicketOpts{
			Status: "done", ScheduledFor: ts(9+i, 0, 0), CreatedAt: ts(9+i, 0, 0)})
	}
	r.seed(t, "one-offs", "20", "m1", servertest.TicketOpts{Status: "done", CreatedAt: ts(23, 0, 0)}) // Run now
	if err := r.h.Server.Schedules().AppendSkip(r.key, schedules.Skip{SetTime: at(10, 30), Reason: schedules.ReasonLate, LoggedAt: at(8, 0)}); err != nil {
		t.Fatal(err)
	}
	got := r.lastRuns(t)
	if len(got) != 10 {
		t.Fatalf("got %d rows, want 10", len(got))
	}
	if got[0].ScheduledFor != "" || got[0].Address != "proj:one-offs/20" {
		t.Errorf("newest row = %+v, want the Run now", got[0])
	}
	last := got[9]
	if last.Status != ops.ScheduleRunSkipped || last.Address != "" || last.Reason != schedules.ReasonLate {
		t.Errorf("oldest kept row = %+v, want the skip", last)
	}
	if last.ReasonText != "Past the 2 h Late limit." {
		t.Errorf("ReasonText = %q", last.ReasonText)
	}
}

func TestLastRuns_TwoMachinesShareOneOffsButNotSkips(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	other := servertest.StartWithStore(t, r.h.TicketStore, rigs.WithSchedulesClock(r.clock))
	a, b := r.h.Server.Schedules().MachineID(), other.Server.Schedules().MachineID()
	r.seed(t, "one-offs", "01", a, servertest.TicketOpts{ScheduledFor: ts(9, 0, 0)})
	r.seed(t, "one-offs", "02", b, servertest.TicketOpts{ScheduledFor: ts(10, 0, 0)})
	if err := other.Server.Schedules().AppendSkip(r.key, schedules.Skip{SetTime: at(11, 0), Reason: schedules.ReasonLate, LoggedAt: at(11, 0)}); err != nil {
		t.Fatal(err)
	}
	if got := r.lastRuns(t); len(got) != 2 {
		t.Errorf("machine a sees %d rows, want the 2 One-offs only: %+v", len(got), got)
	}
	got, err := schedules.LastRuns(r.h.TicketStore, other.Server.Schedules(), r.key, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Status != ops.ScheduleRunSkipped {
		t.Errorf("machine b rows = %+v, want its skip first then both One-offs", got)
	}
}

func TestLastRuns_ResultIsFirstNonEmptyLine(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.seed(t, "one-offs", "01", "m1", servertest.TicketOpts{Status: "done", ScheduledFor: ts(9, 0, 0)})
	r.seed(t, "one-offs", "02", "m1", servertest.TicketOpts{Status: "done", ScheduledFor: ts(8, 0, 0)})
	path := filepath.Join(r.h.TicketStore, "proj", "one-offs", "issues", "01-run01.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n## Result\n\n  All good  \nsecond line\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	got := r.lastRuns(t)
	if got[0].Result != "All good" || got[1].Result != "" {
		t.Errorf("results = %q, %q; want %q, empty", got[0].Result, got[1].Result, "All good")
	}
}
