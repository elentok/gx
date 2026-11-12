package schedules_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server/schedules"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/tickets"
)

var tomorrow = time.Now().AddDate(0, 0, 1)

// at is the wall-clock time hh:mm tomorrow, local.
func at(hh, mm int) time.Time {
	y, m, d := tomorrow.Date()
	return time.Date(y, m, d, hh, mm, 0, 0, time.Local)
}

type rig struct {
	h     *servertest.Harness
	clock *rigs.SchedulesClock
	key   schedules.Key
}

// start makes a server whose clock reads 08:00 and a Schedule "Nightly" with
// the given set times and a switch that is on from 08:00.
func start(t *testing.T, late float64, times ...string) *rig {
	t.Helper()
	clock := rigs.NewSchedulesClock(at(8, 0))
	h, _ := rigs.StartSchedulesHarness(t, clock)
	s, err := schedrec.NewStore(h.TicketStore).Create("proj", schedrec.Schedule{
		Name: "Nightly", Prompt: "check the logs", Days: allDays(), Times: times, LateLimitHours: late,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &rig{h: h, clock: clock, key: schedules.Key{Project: s.Project, Slug: s.Slug}}
}

func allDays() []time.Weekday {
	return []time.Weekday{0, 1, 2, 3, 4, 5, 6}
}

func (r *rig) enable(t *testing.T) {
	t.Helper()
	if err := r.h.Server.Schedules().Enable(r.key, r.clock.Now()); err != nil {
		t.Fatal(err)
	}
}

// restartAt stops the server, moves the clock while it is down and restarts it.
func (r *rig) restartAt(t *testing.T, now time.Time) {
	t.Helper()
	if err := r.h.Stop(); err != nil {
		t.Fatal(err)
	}
	r.clock.Set(now)
	r.h.Restart(t)
}

func (r *rig) runs(t *testing.T) []tickets.Ticket {
	t.Helper()
	epics, err := tickets.Load(filepath.Join(r.h.TicketStore, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range epics {
		if e.Name == "one-offs" {
			return e.Tickets
		}
	}
	return nil
}

func (r *rig) waitRuns(t *testing.T, n int) []tickets.Ticket {
	t.Helper()
	var got []tickets.Ticket
	servertest.WaitFor(t, 10*time.Second, "scheduled runs", func() bool {
		got = r.runs(t)
		return len(got) == n
	})
	return got
}

// settle lets the scheduler check many times, for a claim that nothing happens.
func settle() { time.Sleep(10 * rigs.SchedulesInterval) }

func TestScheduler_DueScheduleStartsOneRun(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	r.clock.Set(at(9, 0).Add(30 * time.Second))

	r.waitRuns(t, 1)
	settle()
	run := r.waitRuns(t, 1)[0]
	if want := "schedule:" + r.key.String() + "@" + r.h.Server.Schedules().MachineID(); run.Unique != want {
		t.Errorf("unique = %q, want %q", run.Unique, want)
	}
	if !run.ScheduledFor.Equal(at(9, 0)) {
		t.Errorf("scheduled_for = %v, want %v", run.ScheduledFor, at(9, 0))
	}
	if !strings.HasPrefix(strings.TrimSpace(run.Body), "check the logs") || !strings.Contains(run.Body, `"Nightly"`) || !strings.Contains(run.Body, "gx notify") {
		t.Errorf("body = %q, want the prompt then the note naming the Schedule", run.Body)
	}
}

func TestScheduler_RestartWithinLateLimitStartsOneRun(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	r.restartAt(t, at(9, 30))

	got := r.waitRuns(t, 1)
	if !got[0].ScheduledFor.Equal(at(9, 0)) {
		t.Errorf("scheduled_for = %v, want %v", got[0].ScheduledFor, at(9, 0))
	}
}

func TestScheduler_ThreeMissedTimesStartOneRunForTheLatest(t *testing.T) {
	t.Parallel()
	r := start(t, 24, "09:00", "10:00", "11:00")
	r.enable(t)
	r.restartAt(t, at(11, 30))

	r.waitRuns(t, 1)
	settle()
	got := r.waitRuns(t, 1)
	if !got[0].ScheduledFor.Equal(at(11, 0)) {
		t.Errorf("scheduled_for = %v, want %v", got[0].ScheduledFor, at(11, 0))
	}
	if skips := r.h.Server.Schedules().Skips(r.key); len(skips) != 0 {
		t.Errorf("skips = %+v, want none", skips)
	}
}

func TestScheduler_SwitchOffThenOnStartsNoRun(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.clock.Set(at(9, 30))
	settle()
	r.enable(t)
	r.clock.Set(at(9, 40))
	settle()

	r.waitRuns(t, 0)
}

func TestScheduler_RestartPastLateLimitLogsOneLateSkip(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	r.restartAt(t, at(12, 0))

	servertest.WaitFor(t, 10*time.Second, "the late skip", func() bool {
		return len(r.h.Server.Schedules().Skips(r.key)) == 1
	})
	settle()
	skips := r.h.Server.Schedules().Skips(r.key)
	if len(skips) != 1 || skips[0].Reason != schedules.ReasonLate || !skips[0].SetTime.Equal(at(9, 0)) {
		t.Errorf("skips = %+v, want one late skip for 09:00", skips)
	}
	r.waitRuns(t, 0)
}

func TestScheduler_QueueWaitNeverTurnsARunIntoASkip(t *testing.T) {
	t.Parallel()
	r := start(t, 2, "09:00")
	r.enable(t)
	r.clock.Set(at(9, 0).Add(time.Minute))
	r.waitRuns(t, 1)

	r.clock.Set(at(20, 0))
	settle()
	if skips := r.h.Server.Schedules().Skips(r.key); len(skips) != 0 {
		t.Errorf("skips = %+v, want none", skips)
	}
	r.waitRuns(t, 1)
}
