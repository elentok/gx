package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func startPauseHarness(t *testing.T) *servertest.Harness {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.PollInterval = 50 * time.Millisecond
	})
	registerLaunch(h)
	return h
}

func expectNoRun(t *testing.T, h *servertest.Harness) {
	t.Helper()
	time.Sleep(500 * time.Millisecond)
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Fatalf("runs = %+v, want none while the queue is held", runs)
	}
}

func expectRun(t *testing.T, h *servertest.Harness) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(h.Server.Runs()) == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no run started")
}

func TestPause_HoldsTheQueueUntilResumeAndSurvivesRestart(t *testing.T) {
	h := startPauseHarness(t)
	ctx := context.Background()

	if res, err := h.Client.QueuePause(ctx); err != nil || res.Refused || res.Mode != server.ModePaused {
		t.Fatalf("pause: %+v, %v", res, err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	expectNoRun(t, h)

	h.Restart(t)
	registerLaunch(h)
	expectNoRun(t, h)

	if res, err := h.Client.QueueResume(ctx); err != nil || res.Refused || res.Mode != server.ModeRunning {
		t.Fatalf("resume: %+v, %v", res, err)
	}
	expectRun(t, h)
}

func TestDrain_StartsNothingNewAndEndsOnResume(t *testing.T) {
	h := startPauseHarness(t)
	ctx := context.Background()

	if res, err := h.Client.QueueDrain(ctx); err != nil || res.Refused || res.Mode != server.ModeDraining {
		t.Fatalf("drain: %+v, %v", res, err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	expectNoRun(t, h)

	if res, err := h.Client.QueueResume(ctx); err != nil || res.Refused {
		t.Fatalf("resume: %+v, %v", res, err)
	}
	expectRun(t, h)
}

func TestPause_RefusedWhileSchedulerIsInProcess(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir())
	res, err := h.Client.QueuePause(context.Background())
	if err != nil || !res.Refused || res.Reason != server.ReasonSchedulerNotSelected {
		t.Fatalf("pause = %+v, %v", res, err)
	}
}

func TestQueuePaused(t *testing.T) {
	dir := t.TempDir()
	if paused, err := server.QueuePaused(dir); err != nil || paused {
		t.Fatalf("no file: paused = %v, err = %v; want false, nil", paused, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "queue-mode.json"), []byte(`{"paused":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if paused, err := server.QueuePaused(dir); err != nil || !paused {
		t.Fatalf("paused file: paused = %v, err = %v; want true, nil", paused, err)
	}
}
