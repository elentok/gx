package schedules_test

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/schedules"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil/runnerfake"
)

// edit saves new set times to the rig's Schedule through a store stamped by clock.
func (r *rig) edit(t *testing.T, clock *rigs.SchedulesClock, times ...string) {
	t.Helper()
	st := schedrec.NewStore(r.h.TicketStore).WithClock(clock.Now)
	s, err := st.Get(r.key.Project, r.key.Slug)
	if err != nil {
		t.Fatal(err)
	}
	s.Times = times
	if _, err := st.Update(r.key.Project, r.key.Slug, s); err != nil {
		t.Fatal(err)
	}
}

func newYork(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// A time that passed while the Schedule had its old times never runs after an edit.
func TestSchedulerRules_EditNeverRunsATimeBeforeTheSave(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	if err := r.h.Stop(); err != nil {
		t.Fatal(err)
	}
	r.clock.Set(at(9, 30))
	r.edit(t, r.clock, "09:00", "20:00")
	r.h.Restart(t)

	settle()
	r.waitRuns(t, 0)
	r.clock.Set(at(20, 0).Add(30 * time.Second))
	got := r.waitRuns(t, 1)
	if !got[0].ScheduledFor.Equal(at(20, 0)) {
		t.Errorf("scheduled_for = %v, want the new 20:00", got[0].ScheduledFor)
	}
}

// A second machine that slept through the edit moves its baseline to the save too.
func TestSchedulerRules_EditReachesAMachineThatSleptThroughIt(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	asleepClock := rigs.NewSchedulesClock(at(8, 0))
	asleep := servertest.StartWithStore(t, r.h.TicketStore, rigs.WithSchedulesClock(asleepClock))
	if err := asleep.Server.Schedules().Enable(r.key, asleepClock.Now()); err != nil {
		t.Fatal(err)
	}
	settle()
	if err := asleep.Stop(); err != nil {
		t.Fatal(err)
	}
	asleepClock.Set(at(9, 30))
	r.edit(t, asleepClock, "09:00", "20:00")
	asleep.Restart(t)

	settle()
	r.waitRuns(t, 0)
}

func TestSchedulerRules_TwoMachinesEachRunWithTheirOwnKey(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	other := servertest.StartWithStore(t, r.h.TicketStore, rigs.WithSchedulesClock(r.clock))
	r.enable(t)
	if err := other.Server.Schedules().Enable(r.key, r.clock.Now()); err != nil {
		t.Fatal(err)
	}
	a, b := r.h.Server.Schedules().MachineID(), other.Server.Schedules().MachineID()
	if a == b {
		t.Fatalf("both machines have id %q", a)
	}
	settle()
	r.clock.Set(at(9, 0).Add(30 * time.Second))

	got := r.waitRuns(t, 2)
	settle()
	got = r.waitRuns(t, 2)
	have := map[string]bool{}
	for _, run := range got {
		have[run.Unique] = true
	}
	for _, id := range []string{a, b} {
		if want := "schedule:" + r.key.String() + "@" + id; !have[want] {
			t.Errorf("no run with unique %q, have %v", want, have)
		}
	}
}

func TestSchedulerRules_SpringForwardGapStartsAtTheFirstValidMoment(t *testing.T) {
	t.Parallel()
	ny := newYork(t)
	clock := rigs.NewSchedulesClock(time.Date(2027, 3, 14, 1, 0, 0, 0, ny))
	r := startOn(t, clock, 2, "02:30")
	r.enable(t)

	clock.Set(time.Date(2027, 3, 14, 1, 59, 0, 0, ny))
	settle()
	r.waitRuns(t, 0)
	clock.Set(time.Date(2027, 3, 14, 3, 0, 30, 0, ny))
	got := r.waitRuns(t, 1)
	if want := time.Date(2027, 3, 14, 3, 0, 0, 0, ny); !got[0].ScheduledFor.Equal(want) {
		t.Errorf("scheduled_for = %v, want %v", got[0].ScheduledFor, want)
	}
}

func TestSchedulerRules_FallBackRepeatStartsOneRun(t *testing.T) {
	t.Parallel()
	ny := newYork(t)
	clock := rigs.NewSchedulesClock(time.Date(2027, 11, 7, 0, 30, 0, 0, ny))
	r := startOn(t, clock, 3, "01:30")
	r.enable(t)

	first := time.Date(2027, 11, 7, 1, 30, 30, 0, ny)
	clock.Set(first)
	r.waitRuns(t, 1)
	clock.Set(first.Add(time.Hour))
	settle()
	r.waitRuns(t, 1)
}

// seedCapRig builds a store with a root ticket and n due Schedules, starts the
// cap rig over it and moves the clock past 09:00. The Schedules use claude.
func seedCapRig(t *testing.T, n int, fake *runnerfake.Runner, opts ...func(*server.Config)) (*servertest.Harness, *rigs.SchedulesClock) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	clock := rigs.NewSchedulesClock(at(8, 0))
	all := append([]func(*server.Config){servertest.WithRunner(fake), rigs.WithSchedulesClock(clock)}, opts...)
	h := rigs.StartOneOffsHarness(t, store, all...)
	st := schedrec.NewStore(store).WithClock(clock.Now)
	for i := range n {
		s, err := st.Create("proj", schedrec.Schedule{
			Name: string(rune('A' + i)), Prompt: "check", Days: allDays(), Times: []string{"09:00"}, LateLimitHours: 2, Harness: "claude",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := h.Server.Schedules().Enable(schedules.Key{Project: s.Project, Slug: s.Slug}, clock.Now()); err != nil {
			t.Fatal(err)
		}
	}
	settle()
	return h, clock
}

// A Scheduled run counts against the one-off cap, not the per-root cap.
func TestSchedulerRules_ScheduledRunsUseTheOneOffCapNotThePerRootCap(t *testing.T) {
	t.Parallel()
	fake := runnerfake.NewRunner()
	h, clock := seedCapRig(t, 2, fake)
	h.Server.Registry().PutRun("proj:epic-a", runs.Run{Address: "proj:epic-a/90", Runner: "herdr"})

	clock.Set(at(9, 0).Add(30 * time.Second))
	rigs.WaitForAgentKinds(t, fake, "01", "02")
}

// With the usage gate closed a Scheduled run stays queued until the gate opens.
func TestSchedulerRules_UsageGateKeepsTheRunQueuedUntilItOpens(t *testing.T) {
	t.Parallel()
	fake := runnerfake.NewRunner()
	rec := nativerunner.UsageRecord{SeenAt: time.Now(), FiveHour: &nativerunner.UsageWindow{UsedPercentage: 99, ResetsAt: time.Now().Add(time.Hour).Unix()}}
	h, clock := seedCapRig(t, 1, fake, rigs.WithUsageSample(t, rec))

	clock.Set(at(9, 0).Add(30 * time.Second))
	servertest.WaitFor(t, 10*time.Second, "the run is queued", func() bool {
		q, err := h.Client.Queue.Items(context.Background())
		return err == nil && len(q) > 0
	})
	settle()
	if k := fake.Kind("one-offs-iter-01"); k != "" {
		t.Fatalf("run started as %q while the gate was closed", k)
	}
	if res, err := h.Client.Admin.UsageOverride(context.Background(), ""); err != nil || res.Refused {
		t.Fatalf("override: %+v, %v", res, err)
	}
	rigs.WaitForAgentKinds(t, fake, "01")
}
