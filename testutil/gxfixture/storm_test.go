package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

func (r *running) epicTicketFile(t *testing.T, epic string) string {
	t.Helper()
	path := filepath.Join(r.StateDir, "..", "..", "data", "gx", "tickets", project, epic, "issues", "01-first.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (r *running) waitSleepers(t *testing.T, what string, ok func(n int) bool) {
	t.Helper()
	if !testutil.Poll(30*time.Second, 100*time.Millisecond, func() bool { return ok(r.Sleepers(t)) }) {
		t.Fatalf("%s; sleepers = %d", what, r.Sleepers(t))
	}
}

func TestProcessStormPausesTheTicketAndLeavesNoTree(t *testing.T) {
	baseline := (&running{}).Sleepers(t)
	r := start(t, "process-storm")
	if !testutil.Poll(60*time.Second, 200*time.Millisecond, func() bool {
		return strings.Contains(r.epicTicketFile(t, "storm-epic"), "pause_reason: process-storm")
	}) {
		t.Fatalf("ticket is not paused process-storm:\n%s", r.epicTicketFile(t, "storm-epic"))
	}
	r.waitSleepers(t, "the killed tree is not gone", func(n int) bool { return n <= baseline })
}

func TestSessionStormExitsTheSessionAndPausesNoTicket(t *testing.T) {
	baseline := (&running{}).Sleepers(t)
	r := start(t, "session-storm")
	if !testutil.Poll(60*time.Second, 200*time.Millisecond, func() bool {
		list := r.sessions(t)
		return len(list) == 1 && list[0].Status.State == wire.SessionExited
	}) {
		t.Fatalf("the storming session did not exit: %+v", r.sessions(t))
	}
	r.waitSleepers(t, "the killed tree is not gone", func(n int) bool { return n <= baseline })
	if body := r.epicTicketFile(t, "storm-session-epic"); strings.Contains(body, "pause_reason") || !strings.Contains(body, "status: open") {
		t.Errorf("the ticket changed:\n%s", body)
	}
}

func TestOutsideStormTreeIsLeftAlive(t *testing.T) {
	baseline := (&running{}).Sleepers(t)
	r := start(t, "outside-storm")
	if r.OutsideTreePID == 0 {
		t.Fatal("the fixture started no outside tree")
	}
	r.waitSleepers(t, "the outside tree did not come up", func(n int) bool { return n >= baseline+agentfake.StormDescendants })
	// Several watchdog intervals pass; the tree must survive them all.
	time.Sleep(2 * time.Second)
	if got := r.Sleepers(t); got < baseline+agentfake.StormDescendants {
		t.Errorf("sleepers = %d, want at least %d: the watchdog killed a tree gx did not launch", got, baseline+agentfake.StormDescendants)
	}
	if err := syscall.Kill(r.OutsideTreePID, 0); err != nil {
		t.Errorf("outside tree root: %v", err)
	}
}
