package schedules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/notify"
	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/schedules"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

type missingRig struct {
	*rig
	hook *rigs.SlackHook
	gate string
}

// startMissing makes a Schedule under a store directory that has no
// project.json, with the Slack hook as the chat endpoint.
func startMissing(t *testing.T) *missingRig {
	t.Helper()
	hook := rigs.NewSlackHook(t)
	gate := filepath.Join(t.TempDir(), "gate.json")
	clock := rigs.NewSchedulesClock(at(8, 0))
	h, _ := rigs.StartOneOffHarness(t, rigs.WithSchedulesClock(clock), func(c *server.Config) {
		c.Chat = notify.ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: gate, FlushInterval: servertest.FlushInterval}
	})
	s, err := schedrec.NewStore(h.TicketStore).WithClock(clock.Now).Create("proj", schedrec.Schedule{
		Name: "Weather check", Prompt: "look outside", Days: allDays(), Times: []string{"09:00"}, LateLimitHours: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Unregistering the project leaves its folder, and the Schedule in it.
	if err := os.Remove(filepath.Join(h.TicketStore, "proj", "project.json")); err != nil {
		t.Fatal(err)
	}
	r := &rig{h: h, clock: clock, key: schedules.Key{Project: s.Project, Slug: s.Slug}}
	r.enable(t)
	return &missingRig{rig: r, hook: hook, gate: gate}
}

// messages counts posts that carry the skip message.
func (m *missingRig) messages(from int) int {
	n := 0
	for _, p := range m.hook.PostsFrom(from) {
		n += strings.Count(p, "was not found")
	}
	return n
}

func TestSchedulerMissingProject_SkipsLogsOnceAndSendsOneMessage(t *testing.T) {
	t.Parallel()
	m := startMissing(t)
	from := m.hook.WaitStarted(t)
	m.clock.Set(at(9, 0).Add(30 * time.Second))

	skips := m.waitSkips(t, 1)
	if skips[0].Reason != schedules.ReasonProjectNotFound || !skips[0].SetTime.Equal(at(9, 0)) {
		t.Errorf("skip = %+v, want project-not-found at 09:00", skips[0])
	}
	servertest.WaitFor(t, 10*time.Second, "the error message in chat", func() bool { return m.messages(from) > 0 })
	settle()
	if n := m.messages(from); n != 1 {
		t.Errorf("messages = %d, want 1", n)
	}
	if got := m.hook.PostsFrom(from); !strings.Contains(strings.Join(got, "\n"), `Weather check: skipped the run for 09:00, project \"proj\" was not found.`) {
		t.Errorf("posts = %q, want the schedule name, set time and project", got)
	}
	if n := len(m.h.Server.Schedules().Skips(m.key)); n != 1 {
		t.Errorf("skips = %d, want 1", n)
	}
	if n := len(m.runs(t)); n != 0 {
		t.Errorf("runs = %d, want none", n)
	}
}

func TestSchedulerMissingProject_RestartSendsNothingAgain(t *testing.T) {
	t.Parallel()
	m := startMissing(t)
	from := m.hook.WaitStarted(t)
	m.clock.Set(at(9, 0).Add(30 * time.Second))
	m.waitSkips(t, 1)
	servertest.WaitFor(t, 10*time.Second, "the error message in chat", func() bool { return m.messages(from) > 0 })

	// Put the baseline back behind the set time: only the skip log is left to stop a repeat.
	if err := m.h.Stop(); err != nil {
		t.Fatal(err)
	}
	st, err := schedules.Open(m.h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetBaseline(m.key, at(8, 0)); err != nil {
		t.Fatal(err)
	}
	m.h.Restart(t)
	// The start notice of the second run marks the point after which a repeat would show.
	servertest.WaitFor(t, 15*time.Second, "the second start notice", func() bool {
		return strings.Count(strings.Join(m.hook.PostsFrom(0), "\n"), "Server started") >= 2
	})
	settle()

	if n := m.messages(0); n != 1 {
		t.Errorf("messages after restart = %d, want 1", n)
	}
	if n := len(m.h.Server.Schedules().Skips(m.key)); n != 1 {
		t.Errorf("skips after restart = %d, want 1", n)
	}
}

func TestSchedulerMissingProject_MutedTransportStillLogsTheSkip(t *testing.T) {
	t.Parallel()
	m := startMissing(t)
	from := m.hook.WaitStarted(t)
	err := notify.UpdateNotificationState(m.gate, func(s *notify.NotificationState) {
		if s.Transports == nil {
			s.Transports = map[string]notify.TransportState{}
		}
		s.Transports["slack"] = notify.TransportState{Muted: true, Reason: "manual-disable", TrippedAt: time.Now()}
	})
	if err != nil {
		t.Fatal(err)
	}
	m.clock.Set(at(9, 0).Add(30 * time.Second))

	m.waitSkips(t, 1)
	settle()
	if n := m.messages(from); n != 0 {
		t.Errorf("messages = %d, want 0 while muted", n)
	}
}
