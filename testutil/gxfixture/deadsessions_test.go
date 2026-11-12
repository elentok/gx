package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func (r *running) sessions(t *testing.T) []wire.SessionInfo {
	t.Helper()
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func (r *running) wantAllCrashed(t *testing.T, n int) []wire.SessionInfo {
	t.Helper()
	var list []wire.SessionInfo
	var holders, agents int
	// The server marks the dead sessions and reaps their processes after it
	// starts, so a one-shot read can see the state half way.
	settled := testutil.Poll(15*time.Second, 100*time.Millisecond, func() bool {
		list = r.sessions(t)
		if len(list) != n {
			return false
		}
		for _, s := range list {
			if s.Status.State != wire.SessionExited || s.Exit != wire.SessionExitCrashed {
				return false
			}
		}
		holders, agents = r.ProcessCounts(t)
		return holders == 0 && agents == 0
	})
	if !settled {
		t.Fatalf("sessions never settled to %d exited/crashed with no processes: holders = %d, agents = %d, list = %+v", n, holders, agents, list)
	}
	return list
}

// restartAfterKill stops the server, kills every session holder and agent, then
// starts a server again, and returns once it answers. The new server is stopped
// at cleanup; the fixture stops the holders it started.
func (r *running) restartAfterKill(t *testing.T) {
	t.Helper()
	var ids []string
	for _, s := range r.sessions(t) {
		ids = append(ids, s.ID)
	}
	r.Gx(t, "server", "stop")
	r.KillSessionProcesses(t, ids)
	cmd := exec.Command(fixturetest.GxBin, "server")
	// The fixture's server finds the fake claude through <root>/bin, which Env lacks.
	cmd.Env = append(slices.Clone(r.Env), "PATH="+filepath.Join(r.Root(), "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Reaped as soon as it exits: `server stop` waits on the pid, which a
	// zombie keeps alive until the 45 s stop timeout.
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("server log:\n%s", r.Gx(t, "server", "logs"))
		}
		stop := exec.Command(fixturetest.GxBin, "server", "stop")
		stop.Env = r.Env
		_ = stop.Run()
		<-exited
	})
	waitFor(t, "the restarted server to answer", func() bool {
		probe := exec.Command(fixturetest.GxBin, "server", "status", "--json")
		probe.Env = r.Env
		out, err := probe.Output()
		var st struct {
			Running bool `json:"running"`
		}
		return err == nil && json.Unmarshal(out, &st) == nil && st.Running
	})
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestRestartSessions_OptInOffResumesNothing restarts over dead sessions with the opt-in off: nothing
// is launched, and the sessions stay Exited.
func TestRestartSessions_OptInOffResumesNothing(t *testing.T) {
	r := start(t, "restart-sessions")
	r.restartAfterKill(t)
	// The server has answered, so its start path has run; give a wrongly
	// resumed session time to show up.
	time.Sleep(2 * time.Second)
	if holders, agents := r.ProcessCounts(t); holders != 0 || agents != 0 {
		t.Errorf("holders = %d, agents = %d, want none with the opt-in off", holders, agents)
	}
}

func TestRestartSessions_OptInResumesEachSessionOnce(t *testing.T) {
	for name, key := range map[string]string{
		"restore-sessions-on-start": "agents.restore-sessions-on-start=true",
		"old resume-on-start alias": "agents.resume-on-start=true",
	} {
		t.Run(name, func(t *testing.T) {
			r := startWith(t, "restart-sessions", nil, "--config", key)
			wantHolders, wantAgents := r.ProcessCounts(t)
			if wantHolders != spec.RestartSessionCount {
				t.Fatalf("holders = %d before the restart, want %d", wantHolders, spec.RestartSessionCount)
			}
			r.restartAfterKill(t)
			waitFor(t, "every session to be resumed once", func() bool {
				holders, agents := r.ProcessCounts(t)
				return holders == wantHolders && agents == wantAgents
			})
			time.Sleep(time.Second)
			if holders, agents := r.ProcessCounts(t); holders != wantHolders || agents != wantAgents {
				t.Errorf("holders/agents = %d/%d, want %d/%d (a session resumed twice?)", holders, agents, wantHolders, wantAgents)
			}
		})
	}
}

func TestDeadMany(t *testing.T) {
	r := start(t, "dead-many")
	r.wantAllCrashed(t, spec.DeadSessionCount)
}

func TestDeadQuestion(t *testing.T) {
	r := start(t, "dead-question")
	s := r.wantAllCrashed(t, 1)[0]
	if !s.Unread.Unread {
		t.Error("the dead session lost its Unread")
	}
}

func TestDeadDesign(t *testing.T) {
	r := start(t, "dead-design")
	if s := r.wantAllCrashed(t, 1)[0]; !s.Design {
		t.Errorf("session %s is not a design session", s.ID)
	}
}
