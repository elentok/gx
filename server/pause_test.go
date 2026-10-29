package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		c.PollInterval = 50 * time.Millisecond
	})
	registerTurn(h)
	return h
}

func expectNoRun(t *testing.T, h *servertest.Harness) {
	t.Helper()
	time.Sleep(200 * time.Millisecond) // negative wait: a launch would come within a few 50ms polls
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Fatalf("runs = %+v, want none while the queue is held", runs)
	}
}

func expectRun(t *testing.T, h *servertest.Harness) {
	t.Helper()
	servertest.WaitFor(t, 10*time.Second, "a run to start", func() bool { return len(h.Server.Runs()) == 1 })
}

func TestPause_HoldsTheQueueUntilResumeAndSurvivesRestart(t *testing.T) {
	t.Parallel()
	h := startPauseHarness(t)
	ctx := context.Background()

	if res, err := h.Client.QueuePause(ctx); err != nil || res.Refused || res.Mode != server.QueueModePaused {
		t.Fatalf("pause: %+v, %v", res, err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	expectNoRun(t, h)

	h.Restart(t)
	registerTurn(h)
	expectNoRun(t, h)

	if res, err := h.Client.QueueResume(ctx); err != nil || res.Refused || res.Mode != server.QueueModeRunning {
		t.Fatalf("resume: %+v, %v", res, err)
	}
	expectRun(t, h)
}

func TestFlush_IsFinishAndPause(t *testing.T) {
	t.Parallel()
	h := startPauseHarness(t)
	ctx := context.Background()

	if res, err := h.Client.QueuePause(ctx); err != nil || res.Refused || res.Mode != server.QueueModePaused {
		t.Fatalf("flush: %+v, %v", res, err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	expectNoRun(t, h)

	h.Restart(t)
	registerTurn(h)
	expectNoRun(t, h)

	if res, err := h.Client.QueueResume(ctx); err != nil || res.Refused {
		t.Fatalf("resume: %+v, %v", res, err)
	}
	expectRun(t, h)
}

func TestSnapshot_ReportsQueueMode(t *testing.T) {
	t.Parallel()
	h := startPauseHarness(t)
	ctx := context.Background()
	steps := []struct {
		name string
		do   func(context.Context) (server.QueueResult, error)
		want server.QueueMode
	}{
		{"initial", nil, server.QueueModeRunning},
		{"pause", h.Client.QueuePause, server.QueueModePaused},
		{"resume", h.Client.QueueResume, server.QueueModeRunning},
		{"pause again", h.Client.QueuePause, server.QueueModePaused},
	}
	for _, st := range steps {
		if st.do != nil {
			if _, err := st.do(ctx); err != nil {
				t.Fatalf("%s: %v", st.name, err)
			}
		}
		snap, err := h.Client.Snapshot(ctx)
		if err != nil || snap.Mode != st.want {
			t.Fatalf("%s: mode = %q, %v; want %q", st.name, snap.Mode, err, st.want)
		}
	}
}

func TestQueuePaused(t *testing.T) {
	t.Parallel()
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
