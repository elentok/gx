package server_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/runnerfake"
)

const holderLabel = "epic-a-iter-01"

// spyWithHolder is a fake runner whose launch label is already held by a
// session in state.
func spyWithHolder(t *testing.T, state agentrunner.State) (*launchSpy, agentrunner.Session) {
	t.Helper()
	spy := &launchSpy{Runner: runnerfake.NewRunner()}
	holder, err := spy.Runner.Start(agentrunner.StartOptions{Label: holderLabel, Cwd: "/elsewhere"})
	if err != nil {
		t.Fatal(err)
	}
	spy.SetState(holderLabel, state, "")
	return spy, holder
}

func runLog(t *testing.T, h *servertest.Harness) []ralphloop.Event {
	t.Helper()
	logged, _, err := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	return logged
}

func ofType(logged []ralphloop.Event, typ events.Type) []ralphloop.Event {
	var out []ralphloop.Event
	for _, ev := range logged {
		if ev.Type == string(typ) {
			out = append(out, ev)
		}
	}
	return out
}

func TestLaunch_StaleHolderIsClosedAndTheLaunchRetried(t *testing.T) {
	for _, state := range []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone} {
		t.Run(string(state), func(t *testing.T) {
			spy, holder := spyWithHolder(t, state)
			h := launchOne(t, spy, server.EventIterationStarted)
			t.Cleanup(func() { spy.SetState(holderLabel, agentrunner.StateDone, "") })
			closed := ofType(runLog(t, h), events.StalePaneClosed)
			if len(closed) != 1 || closed[0].Pane != holder.ID || closed[0].Reason != string(state) || closed[0].Ticket != "01" {
				t.Errorf("stale-pane-closed events = %+v, want one for %s, %s", closed, holder.ID, state)
			}
			if runs := h.Server.Runs(); len(runs) != 1 || runs[0].Session.ID == holder.ID {
				t.Errorf("runs = %+v, want one on a fresh session", runs)
			}
		})
	}
}

// assertParkedNameTaken checks the launch logged launch-failed, then parked
// agent_name_taken with a reason containing want, and that R6 matches it.
func assertParkedNameTaken(t *testing.T, h *servertest.Harness, want ...string) {
	t.Helper()
	logged := runLog(t, h)
	failed := slices.IndexFunc(logged, func(ev ralphloop.Event) bool {
		return ev.Type == string(events.LaunchFailed) && ev.Kind == string(events.AgentNameTaken)
	})
	park := slices.IndexFunc(logged, func(ev ralphloop.Event) bool { return ev.Type == string(events.Paused) })
	if failed < 0 || park < failed || logged[park].Kind != string(events.AgentNameTaken) {
		t.Fatalf("run log = %+v, want launch-failed then a needs-repair agent_name_taken park", logged)
	}
	for _, w := range want {
		if !strings.Contains(logged[park].Reason, w) {
			t.Errorf("park reason %q lacks %q", logged[park].Reason, w)
		}
	}
	c := recovery.Default()
	for i := range c.Entries {
		c.Entries[i].Enabled = c.Entries[i].ID == "R6"
	}
	var seq []recovery.Event
	for _, ev := range logged[:park+1] {
		seq = append(seq, recovery.Event{Type: events.Type(ev.Type), Kind: events.Kind(ev.Kind), Reason: ev.Reason})
	}
	if e, ok := c.Match(seq); !ok || e.ID != "R6" {
		t.Errorf("Match = %q, %v; want R6", e.ID, ok)
	}
}

func TestLaunch_WorkingHolderParksNameTakenAndKeepsThePane(t *testing.T) {
	spy, holder := spyWithHolder(t, agentrunner.StateWorking)
	h := launchOne(t, spy, server.EventIterationLaunchFailed)
	assertParkedNameTaken(t, h, holder.ID, "working")
	if closed := ofType(runLog(t, h), events.StalePaneClosed); len(closed) != 0 {
		t.Errorf("stale-pane-closed = %+v, want none", closed)
	}
	if _, ok, _ := spy.Find(holderLabel); !ok {
		t.Error("the working holder was closed")
	}
}

func TestLaunch_SecondNameTakenAfterClosingParks(t *testing.T) {
	spy, _ := spyWithHolder(t, agentrunner.StateIdle)
	spy.SetStartErr(holderLabel, fmt.Errorf("%w: %s", agentrunner.ErrLabelTaken, holderLabel))
	h := launchOne(t, spy, server.EventIterationLaunchFailed)
	assertParkedNameTaken(t, h)
	if closed := ofType(runLog(t, h), events.StalePaneClosed); len(closed) != 1 {
		t.Errorf("stale-pane-closed = %+v, want exactly one: the retry happens once", closed)
	}
}
