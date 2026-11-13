package schedules_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server/schedules"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

func (r *rig) runNow(t *testing.T) ops.ScheduleRunNowResult {
	t.Helper()
	return runNowOf(t, r.h, r.key)
}

func runNowOf(t *testing.T, h *servertest.Harness, k schedules.Key) ops.ScheduleRunNowResult {
	t.Helper()
	res, err := h.Client.Schedules.RunNow(context.Background(), ops.ScheduleRunNowRequest{Project: k.Project, Slug: k.Slug})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSchedulesRunNow_QueuesAOneOffWithTheKeyAndNoSetTime(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00") // switch never turned on here
	res := r.runNow(t)
	if res.Refused || res.Address == "" || res.Status == "" {
		t.Fatalf("run-now = %+v", res)
	}
	runs := r.runs(t)
	if len(runs) != 1 {
		t.Fatalf("one-offs = %d, want 1", len(runs))
	}
	got := runs[0]
	if want := "schedule:" + r.key.String() + "@" + r.h.Server.Schedules().MachineID(); got.Unique != want {
		t.Errorf("Unique = %q, want %q", got.Unique, want)
	}
	if !got.ScheduledFor.IsZero() {
		t.Errorf("ScheduledFor = %v, want empty", got.ScheduledFor)
	}
	if want := schedules.Prompt("Nightly", "check the logs"); !strings.HasPrefix(strings.TrimSpace(got.Body), "check the logs") || !strings.Contains(got.Body, `"Nightly"`) || !strings.Contains(want, "gx notify") {
		t.Errorf("body = %q, want the prompt then the note naming the Schedule", got.Body)
	}
	if _, ok := r.h.Server.Schedules().Baseline(r.key); ok {
		t.Error("run-now set a baseline")
	}
	if r.h.Server.Schedules().Enabled(r.key) {
		t.Error("run-now turned the switch on")
	}
}

func TestSchedulesRunNow_ReplacesAQueuedRun(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	first := r.runNow(t)
	second := r.runNow(t)
	if first.Refused || second.Refused || first.Address == second.Address {
		t.Fatalf("first = %+v second = %+v", first, second)
	}
	servertest.WaitFor(t, 10*time.Second, "the first run cancelled and the second queued", func() bool {
		runs := r.runs(t)
		return len(runs) == 1 && runs[0].Status == "open"
	})
	if n := len(r.h.Server.Schedules().Skips(r.key)); n != 0 {
		t.Errorf("skips = %d, want 0", n)
	}
}

func TestSchedulesRunNow_RefusalsStartNothingAndLogNoSkip(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		opts   servertest.TicketOpts
		reason string
		attn   bool
	}{
		{"started", servertest.TicketOpts{Status: "claimed"}, schedules.ReasonPreviousRunStarted, false},
		{"paused", servertest.TicketOpts{Status: "paused", PauseReason: "question"}, schedules.ReasonPreviousRunNeedsAttention, true},
		{"legacy-repair-status", servertest.TicketOpts{Status: "needs-repair"}, schedules.ReasonPreviousRunNeedsAttention, true}, // oldword:ok legacy status input
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := start(t, 2, "09:00")
			r.writePrevious(t, tc.opts)
			res := r.runNow(t)
			if !res.Refused || res.Reason != tc.reason || res.Message == "" {
				t.Fatalf("run-now = %+v", res)
			}
			if tc.attn != (res.AttentionAddress == prevAddr) {
				t.Errorf("attention address = %q", res.AttentionAddress)
			}
			if n := len(r.runs(t)); n != 1 {
				t.Errorf("one-offs = %d, want only the previous run", n)
			}
			if n := len(r.h.Server.Schedules().Skips(r.key)); n != 0 {
				t.Errorf("skips = %d, want 0", n)
			}
		})
	}
}

func TestSchedulesRunNow_UnregisteredProjectPassesTheSubmitRefusalThrough(t *testing.T) {
	t.Parallel()
	m := startMissing(t)
	res := m.runNow(t)
	if !res.Refused || res.Reason != wire.ReasonUnknownProj {
		t.Fatalf("run-now = %+v", res)
	}
	if n := len(m.h.Server.Schedules().Skips(m.key)); n != 0 {
		t.Errorf("skips = %d, want 0", n)
	}
}

func TestSchedulesRunNow_UnregisteredProjectKeepsTheQueuedPreviousRun(t *testing.T) {
	t.Parallel()
	m := startMissing(t)
	m.writePrevious(t, servertest.TicketOpts{Status: "open"})
	// Writing a ticket recreates project.json; the project must be gone again.
	if err := os.Remove(filepath.Join(m.h.TicketStore, "proj", "project.json")); err != nil {
		t.Fatal(err)
	}
	res := m.runNow(t)
	if !res.Refused || res.Reason != wire.ReasonUnknownProject {
		t.Fatalf("run-now = %+v", res)
	}
	runs := m.runs(t)
	if len(runs) != 1 || runs[0].Status != "open" {
		t.Errorf("one-offs = %+v, want the previous run still open", runs)
	}
	if n := len(m.h.Server.Schedules().Skips(m.key)); n != 0 {
		t.Errorf("skips = %d, want 0", n)
	}
}

func TestSchedulesRunNow_UnknownScheduleIsRefused(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	res := runNowOf(t, r.h, schedules.Key{Project: "proj", Slug: "nope"})
	if !res.Refused || res.Reason != ops.ScheduleReasonUnknown {
		t.Errorf("run-now = %+v", res)
	}
}

func TestSchedulesRunNow_FullOneOffCapStillQueuesAndSucceeds(t *testing.T) {
	t.Parallel()
	fake := runnerfake.NewRunner()
	h, _ := seedCapRig(t, 3, fake)
	for _, slug := range []string{"a", "b"} {
		if res := runNowOf(t, h, schedules.Key{Project: "proj", Slug: slug}); res.Refused {
			t.Fatalf("run-now %s = %+v", slug, res)
		}
	}
	rigs.WaitForAgentKinds(t, fake, "01", "02")
	res := runNowOf(t, h, schedules.Key{Project: "proj", Slug: "c"})
	if res.Refused || res.Address == "" {
		t.Fatalf("run-now with the cap full = %+v", res)
	}
}

func TestSchedulesRunNow_PublishesSchedulesChanged(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	events := rigs.EventStream(t, r.h)
	r.runNow(t)
	for {
		select {
		case ev := <-events:
			if ev.Type == ops.EventSchedulesChanged && ev.Address == "proj:nightly" {
				return
			}
		case <-time.After(10 * time.Second):
			t.Fatal("no schedules-changed event")
		}
	}
}

func TestSchedulesRunNow_TheScheduleTierPinsTheRunsFirstLaunch(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	store := schedrec.NewStore(r.h.TicketStore)
	sc, err := store.Get(r.key.Project, r.key.Slug)
	if err != nil {
		t.Fatal(err)
	}
	sc.Tier = "strongest"
	if _, err := store.Update(r.key.Project, r.key.Slug, sc); err != nil {
		t.Fatal(err)
	}
	if res := r.runNow(t); res.Refused {
		t.Fatalf("run-now = %+v", res)
	}
	runs := r.runs(t)
	if len(runs) != 1 {
		t.Fatalf("one-offs = %d, want 1", len(runs))
	}
	tk, err := schema.ParseTicket(runs[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := tk.TierOverride; got != "strongest" {
		t.Errorf("TierOverride = %q, want strongest", got)
	}
}
