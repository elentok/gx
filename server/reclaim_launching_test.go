package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

// launchSpy is a fake runner that snapshots runs.json as each agent starts.
type launchSpy struct {
	*runnerfake.Runner
	mu       sync.Mutex
	runsJSON string
	atStart  []map[string]any
}

func (l *launchSpy) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	l.mu.Lock()
	l.atStart = readRuns(nil, l.runsJSON)
	l.mu.Unlock()
	return l.Runner.Start(opts)
}

// readRuns decodes runs.json; t may be nil off the test goroutine, where a
// read failure just yields nothing.
func readRuns(t *testing.T, path string) []map[string]any {
	var runs []map[string]any
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &runs)
	}
	if err != nil && t != nil {
		t.Fatal(err)
	}
	return runs
}

// launchOne queues proj:epic-a/01 on spy and waits for the event want.
func launchOne(t *testing.T, spy *launchSpy, want string) *servertest.Harness {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.RunnerFor = func(_, _ string) agentrunner.Runner { return spy }
	})
	spy.mu.Lock()
	spy.runsJSON = filepath.Join(h.StateDir, "runs.json")
	spy.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for ev := range evs {
		if ev.Type == want {
			return h
		}
	}
	t.Fatalf("no %s event", want)
	return nil
}

func TestRunner_LaunchWritesALaunchingEntryBeforeTheAgentStartsAndFlipsIt(t *testing.T) {
	t.Parallel()
	spy := &launchSpy{Runner: runnerfake.NewRunner()}
	h := launchOne(t, spy, server.EventIterationStarted)
	t.Cleanup(func() { spy.SetState("epic-a-iter-01", agentrunner.StateDone, "") })
	spy.mu.Lock()
	atStart := spy.atStart
	spy.mu.Unlock()
	if len(atStart) != 1 || atStart[0]["state"] != "launching" || atStart[0]["base"] == "" || atStart[0]["runner"] != "fake" {
		t.Errorf("runs.json at start = %v, want one launching fake entry with its base", atStart)
	}
	after := readRuns(t, filepath.Join(h.StateDir, "runs.json"))
	if len(after) != 1 || after[0]["state"] != nil {
		t.Errorf("runs.json after launch = %v, want the entry no longer launching", after)
	}
}

func TestRunner_AFailedLaunchLeavesNoEntry(t *testing.T) {
	t.Parallel()
	spy := &launchSpy{Runner: runnerfake.NewRunner()}
	spy.SetStartErr("epic-a-iter-01", errors.New("boom"))
	h := launchOne(t, spy, server.EventIterationLaunchFailed)
	if runs := readRuns(t, filepath.Join(h.StateDir, "runs.json")); len(runs) != 0 {
		t.Errorf("runs.json = %v, want empty", runs)
	}
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Errorf("runs = %+v, want none", runs)
	}
}

// markLaunching rewrites the staged handle as a launch the server died in.
func markLaunching(t *testing.T, h *servertest.Harness) {
	t.Helper()
	path := filepath.Join(h.StateDir, "runs.json")
	runs := readRuns(t, path)
	runs[0]["state"] = "launching"
	data, err := json.Marshal(runs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// restartLaunching leaves a launching handle with work as the agent's, then
// restarts. herdr sets up the herdr agent; fake is the headless runner, nil
// for a herdr run.
func restartLaunching(t *testing.T, fake *headlessFake, herdr func(*servertest.Harness), work func(*testing.T, string)) (*servertest.Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	var opts []func(*server.Config)
	if fake != nil {
		opts = append(opts, func(c *server.Config) {
			c.RunnerFor = func(_, _ string) agentrunner.Runner { return fake }
		})
	}
	h := servertest.StartWithStore(t, store, opts...)
	if herdr != nil {
		herdr(h)
	}
	_, ticketPath := leaveIterationWith(t, h, store, repo, func(p string) { work(t, p) })
	if fake != nil {
		asHeadlessRun(t, h)
	}
	markLaunching(t, h)
	h.Restart(t)
	return h, ticketPath
}

func TestReclaim_LaunchingWithNoAgentReopens(t *testing.T) {
	t.Parallel()
	runnerShapes(t, func(t *testing.T, headless bool) {
		var fake *headlessFake
		if headless {
			fake = &headlessFake{Runner: runnerfake.NewRunner(), verdict: nativerunner.VerdictDied}
		}
		_, ticketPath := restartLaunching(t, fake, nil, noWork)
		waitStatus(t, ticketPath, schema.StatusOpen)
	})
}

func TestReclaim_LaunchingIdleAgentIsStoppedThenReopens(t *testing.T) {
	t.Parallel()
	t.Run("herdr", func(t *testing.T) {
		var mu sync.Mutex
		closed := 0
		_, ticketPath := restartLaunching(t, nil, func(h *servertest.Harness) {
			registerLiveAgent(h)
			h.Herdr.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
				mu.Lock()
				closed++
				mu.Unlock()
				return map[string]any{}, herdrfake.Identities{}, nil
			})
		}, noWork)
		waitStatus(t, ticketPath, schema.StatusOpen)
		mu.Lock()
		defer mu.Unlock()
		if closed == 0 {
			t.Error("the idle agent's tab was never closed")
		}
	})
	t.Run("headless", func(t *testing.T) {
		fake := &headlessFake{Runner: runnerfake.NewRunner(), verdict: nativerunner.VerdictFinished}
		_, ticketPath := restartLaunching(t, fake, nil, noWork)
		waitStatus(t, ticketPath, schema.StatusOpen)
		fake.mu.Lock()
		defer fake.mu.Unlock()
		if !slices.Equal(fake.stopped, []string{"p1"}) {
			t.Errorf("stopped %v, want p1", fake.stopped)
		}
	})
}

func TestReclaim_LaunchingWorkingAgentIsALiveRun(t *testing.T) {
	t.Parallel()
	t.Run("herdr", func(t *testing.T) {
		var finished atomic.Bool
		h, ticketPath := restartLaunching(t, nil, func(h *servertest.Harness) {
			registerLiveAgent(h)
			h.Herdr.Register("agent", "get", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
				status := "working"
				if finished.Load() {
					status = "idle"
				}
				return map[string]any{"agent": map[string]any{"pane_id": "p1", "tab_id": "t1", "agent_status": status, "state_change_seq": 1}}, herdrfake.Identities{}, nil
			})
		}, commitAgentWork)
		waitReclaimedLive(t, h)
		finished.Store(true)
		waitStatus(t, ticketPath, schema.StatusDone)
	})
	t.Run("headless", func(t *testing.T) {
		fake := &headlessFake{Runner: runnerfake.NewRunner(), verdict: nativerunner.VerdictLive}
		fake.IDs = func(string) (string, string) { return "p1", "s1" }
		if _, err := fake.Start(agentrunner.StartOptions{Label: "epic-a-iter-01"}); err != nil {
			t.Fatal(err)
		}
		fake.SetState("epic-a-iter-01", agentrunner.StateWorking, "")
		h, ticketPath := restartLaunching(t, fake, nil, commitAgentWork)
		waitReclaimedLive(t, h)
		fake.SetState("epic-a-iter-01", agentrunner.StateDone, "")
		waitStatus(t, ticketPath, schema.StatusDone)
	})
}

// waitReclaimedLive waits for runs.json to hold the reclaimed run, no longer launching.
func waitReclaimedLive(t *testing.T, h *servertest.Harness) {
	t.Helper()
	path := filepath.Join(h.StateDir, "runs.json")
	if !servertest.Poll(10*time.Second, func() bool {
		runs := readRuns(t, path)
		return len(runs) == 1 && runs[0]["state"] == nil
	}) {
		t.Fatalf("runs.json = %v, want the reclaimed entry no longer launching", readRuns(t, path))
	}
}
