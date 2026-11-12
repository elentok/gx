package schedules_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/tickets"
)

func (r *rig) deleteSchedule(t *testing.T) {
	t.Helper()
	if err := schedrec.NewStore(r.h.TicketStore).Delete(r.key.Project, r.key.Slug); err != nil {
		t.Fatal(err)
	}
}

// writeRun writes a run of the Schedule carrying machine's Dedupe key.
func (r *rig) writeRun(t *testing.T, id, machine string, opts servertest.TicketOpts) {
	t.Helper()
	opts.Unique = "schedule:" + r.key.String() + "@" + machine
	opts.Type = "prompt"
	servertest.WriteTicketWith(t, r.h.TicketStore, "proj", "one-offs", id, "run"+id, opts)
}

// cancelled lists the ids of the Schedule's runs the sweep cancelled, live or archived.
func (r *rig) cancelled(t *testing.T) []string {
	t.Helper()
	live, archived, err := tickets.LoadOneOffs(filepath.Join(r.h.TicketStore, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, tk := range append(live.Tickets, archived.Tickets...) {
		if tk.Status == "cancelled" {
			ids = append(ids, tk.Unique)
		}
	}
	return ids
}

func (r *rig) statusOf(t *testing.T, id string) string {
	t.Helper()
	live, _, err := tickets.LoadOneOffs(filepath.Join(r.h.TicketStore, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range live.Tickets {
		if tk.DisplayNumber() == id {
			return tk.Status
		}
	}
	return ""
}

func (r *rig) hasState(h *servertest.Harness) bool {
	return slices.Contains(h.Server.Schedules().Keys(), r.key)
}

func TestSchedulerDelete_CancelsOnlyQueuedRunsAndDropsState(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	id := r.h.Server.Schedules().MachineID()
	r.writeRun(t, "90", id, servertest.TicketOpts{})
	r.writeRun(t, "91", id, servertest.TicketOpts{Status: "claimed", ClaimedAt: at(8, 30).Format(time.RFC3339)})
	r.writeRun(t, "92", id, servertest.TicketOpts{PauseReason: "question"})
	settle()
	if !r.hasState(r.h) {
		t.Fatal("no state before the delete")
	}

	r.deleteSchedule(t)

	servertest.WaitFor(t, 10*time.Second, "state dropped", func() bool { return !r.hasState(r.h) })
	if got := r.cancelled(t); len(got) != 1 {
		t.Errorf("cancelled = %v, want one run", got)
	}
	if s := r.statusOf(t, "91"); s != "claimed" {
		t.Errorf("started run status = %q", s)
	}
	if s := r.statusOf(t, "92"); s != "paused" {
		t.Errorf("paused run status = %q", s)
	}
	settle() // idempotent: a second check changes nothing
	if got := r.cancelled(t); len(got) != 1 {
		t.Errorf("cancelled after settle = %v", got)
	}
}

func TestSchedulerDelete_EachMachineCancelsOnlyItsOwnRun(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	other := servertest.StartWithStore(t, r.h.TicketStore, rigs.WithSchedulesClock(r.clock))
	r.enable(t)
	if err := other.Server.Schedules().Enable(r.key, r.clock.Now()); err != nil {
		t.Fatal(err)
	}
	a, b := r.h.Server.Schedules().MachineID(), other.Server.Schedules().MachineID()
	r.writeRun(t, "90", a, servertest.TicketOpts{})
	r.writeRun(t, "91", b, servertest.TicketOpts{})

	r.deleteSchedule(t)

	servertest.WaitFor(t, 10*time.Second, "both machines tidy up", func() bool {
		return !r.hasState(r.h) && !r.hasState(other) && len(r.cancelled(t)) == 2
	})
	got := r.cancelled(t)
	for _, id := range []string{a, b} {
		if want := "schedule:" + r.key.String() + "@" + id; !slices.Contains(got, want) {
			t.Errorf("run %q not cancelled, have %v", want, got)
		}
	}
}

func TestSchedulerDelete_SwitchOffLeavesAQueuedRunAlone(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	r.writeRun(t, "90", r.h.Server.Schedules().MachineID(), servertest.TicketOpts{})
	if err := r.h.Server.Schedules().Disable(r.key); err != nil {
		t.Fatal(err)
	}
	settle()
	if s := r.statusOf(t, "90"); s != "open" {
		t.Errorf("queued run status = %q, want open", s)
	}
	if !r.hasState(r.h) {
		t.Error("state of a live Schedule was dropped")
	}
}

func TestSchedulerDelete_FailedListCancelsNothing(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permissions do not apply to root")
	}
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	r.writeRun(t, "90", r.h.Server.Schedules().MachineID(), servertest.TicketOpts{})
	dir := filepath.Join(r.h.TicketStore, "proj", ".schedules")
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	settle()
	if s := r.statusOf(t, "90"); s != "open" {
		t.Errorf("queued run status = %q, want open", s)
	}
	if !r.hasState(r.h) {
		t.Error("state dropped after a failed list")
	}
}
