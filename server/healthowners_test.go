package server_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/syshealth"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// pidRunner is a fake runner that knows the pids of every session.
type pidRunner struct {
	*runnerfake.Runner
	pids []agentrunner.OwnerPID
	err  error
}

func (r *pidRunner) OwnerPIDs(agentrunner.Session) ([]agentrunner.OwnerPID, error) {
	return r.pids, r.err
}

func child(pid, ppid int, comm string, ticks uint64) syshealth.Process {
	p := proc(pid, comm, ticks, 1)
	p.PPID = ppid
	return p
}

func startOwnedRun(t *testing.T, r *pidRunner) *servertest.Harness {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithRunner(r), func(c *server.Config) {
		c.HealthPollInterval = 5 * time.Millisecond
		c.ProcessSecondReadDelay = 5 * time.Millisecond
		c.ProcessShareWindow = 50 * time.Millisecond
		c.ProcessCutoff = 10 * time.Second
	})
	h.Processes.Set(child(100, 1, "claude", 10), child(101, 100, "go test", 400))
	t.Cleanup(h.Processes.Release)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil {
		t.Fatal(err)
	}
	testutil.WaitFor(t, 10*time.Second, "the run to start", func() bool {
		live, _ := r.List("epic-a")
		return len(live) > 0
	})
	return h
}

func settledCauses(t *testing.T, h *servertest.Harness) server.HealthCauses {
	t.Helper()
	var causes server.HealthCauses
	testutil.WaitFor(t, 10*time.Second, "the cause lists to settle", func() bool {
		rep := health(t, h, true)
		causes = rep.Causes
		return rep.Causes.CPU.Marker == ""
	})
	return causes
}

// Seam 1: a tracked run's agent tree shows as its ticket address with the
// busiest process (stories 37, 38).
func TestHealthOwners_RunShowsAddressWithBusiestProcess(t *testing.T) {
	t.Parallel()
	h := startOwnedRun(t, &pidRunner{Runner: runnerfake.NewRunner(), pids: []agentrunner.OwnerPID{{PID: 100, Start: 1000}}})
	rows := settledCauses(t, h).CPU.Rows
	if len(rows) != 1 {
		t.Fatalf("cpu rows = %+v, want one agent row", rows)
	}
	if r := rows[0]; r.Kind != "agent" || r.Label != "proj:epic-a/01" || r.Ticket != "proj:epic-a/01" || r.Busiest != "go test" {
		t.Errorf("row = %+v, want agent proj:epic-a/01 busiest go test", r)
	}
}

// Seam 1: a run whose owner call fails is skipped and the lists still come back.
func TestHealthOwners_FailedOwnerCallStillReturnsLists(t *testing.T) {
	t.Parallel()
	h := startOwnedRun(t, &pidRunner{Runner: runnerfake.NewRunner(), err: errors.New("boom")})
	got := labels(settledCauses(t, h).CPU)
	if !sameLabels(got, "go test", "claude") {
		t.Errorf("cpu list = %v, want the processes under their own names", got)
	}
}
