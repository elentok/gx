package main

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// claudeSessions is the --session-id of every live fake claude of this fixture.
// A fresh claim or a repair agent started after a restart has a new one.
func (r *running) claudeSessions(t *testing.T) []string {
	t.Helper()
	var ids []string
	for _, line := range testutil.PsLines(t, r.Env) {
		if !strings.HasPrefix(line, "claude ") || !strings.Contains(line, r.Root()) {
			continue
		}
		fields := strings.Fields(line)
		for i, f := range fields[:len(fields)-1] {
			if f == "--session-id" {
				ids = append(ids, fields[i+1])
			}
		}
	}
	return ids
}

// killServerAndAgent ends the claimed run's agent together with the server,
// then starts the server again. It returns the dead agent's claude sessions.
func (r *running) killServerAndAgent(t *testing.T) []string {
	t.Helper()
	r.WaitStatus(t, spec.DeadRunTicket, "claimed")
	var meta nativerunner.Meta
	if !testutil.Poll(30*time.Second, 100*time.Millisecond, func() bool {
		var ok bool
		meta, ok = readAnyMeta(nativerunner.AgentsRoot(r.StateDir, project), "*")
		return ok
	}) {
		t.Fatal("the claimed run has no live agent")
	}
	dead := r.claudeSessions(t)
	if len(dead) == 0 {
		t.Fatal("the process list does not show the running agent")
	}
	ctl := filepath.Join(r.MigrateDir, "server")
	if out, err := exec.Command(ctl, "kill").CombinedOutput(); err != nil {
		t.Fatalf("server kill: %v\n%s", err, out)
	}
	for _, pid := range []int{meta.HolderPID, meta.PID} {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	if out, err := exec.Command(ctl, "start").CombinedOutput(); err != nil {
		t.Fatalf("server start: %v\n%s", err, out)
	}
	if !testutil.Poll(30*time.Second, 100*time.Millisecond, func() bool {
		cmd := exec.Command(fixturetest.GxBin, "server", "status", "--json")
		cmd.Env = r.Env
		return cmd.Run() == nil
	}) {
		t.Fatal("the server did not come back")
	}
	return dead
}

// wantNoneOf fails when a live claude still runs one of the dead sessions.
func (r *running) wantNoneOf(t *testing.T, dead []string) {
	t.Helper()
	for _, id := range r.claudeSessions(t) {
		if slices.Contains(dead, id) {
			t.Errorf("claude session %s of the dead run is running again", id)
		}
	}
}

func TestDeadRunEmptyReopens(t *testing.T) {
	r := start(t, "dead-run-empty")
	dead := r.killServerAndAgent(t)
	// The epic is still queued, so the reopened ticket is claimed again at once;
	// the reopen shows in its history.
	if !testutil.Poll(60*time.Second, time.Second, func() bool {
		return strings.Contains(r.Gx(t, "server", "tickets", "history", spec.DeadRunTicket), "claim-reopened")
	}) {
		t.Fatal("the restart did not reopen the ticket")
	}
	r.wantNoneOf(t, dead)
}

func TestDeadRunWithWorkPauses(t *testing.T) {
	r := start(t, "dead-run-work")
	dead := r.killServerAndAgent(t)
	// A repair pause shows in the snapshot under its legacy label.
	r.WaitStatus(t, spec.DeadRunTicket, "needs-repair")
	if body := r.epicTicketFile(t, "dead-run-epic"); !strings.Contains(body, "pause_reason: iteration-error") {
		t.Errorf("ticket is not paused iteration-error:\n%s", body)
	}
	r.wantNoneOf(t, dead)
}
