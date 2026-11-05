package sched_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/check"
	"github.com/elentok/gx/server/servertest"
)

func writeWaiting(t *testing.T, h *servertest.Harness, w check.Waiting) string {
	t.Helper()
	dir := filepath.Join(h.StateDir, "check-waiting")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(w)
	path := filepath.Join(dir, strconv.Itoa(w.PID)+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitingLabels(t *testing.T, h *servertest.Harness) (queue, detail string) {
	t.Helper()
	rows := servertest.QueueRows(t, h)
	if len(rows) != 1 {
		t.Fatalf("queue rows = %+v, want one", rows)
	}
	d, err := h.Client.Reads.Detail(context.Background(), servertest.QueueRowTicket)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != "claimed" || d.PauseReason != "" {
		t.Errorf("status = %q, pause = %q; a label must not change them", d.Status, d.PauseReason)
	}
	return rows[0].WaitingLabel, d.WaitingLabel
}

func workingQueueRow(t *testing.T) (*servertest.Harness, string) {
	t.Helper()
	h, fake := servertest.StartQueueRows(t, "pty")
	fake.SetState(servertest.ResumeLabel, agentrunner.StateWorking, "")
	servertest.WaitFor(t, 10*time.Second, "a working row", func() bool {
		rows := servertest.QueueRows(t, h)
		return len(rows) == 1 && rows[0].Status.State == wire.SessionWorking
	})
	return h, servertest.QueueRows(t, h)[0].Dir
}

// Seam 6: the label follows a live waiting state of the claimed ticket's worktree.
func TestWaitingLabel_ShowsOnQueueRowAndDetailAndClears(t *testing.T) {
	t.Parallel()
	h, cwd := workingQueueRow(t)

	if q, d := waitingLabels(t, h); q != "" || d != "" {
		t.Fatalf("labels without a waiting state = %q, %q", q, d)
	}

	path := writeWaiting(t, h, check.Waiting{PID: os.Getpid(), HeldBy: "RUN-A", Root: cwd})
	want := "waiting for the check lock, held by RUN-A"
	if q, d := waitingLabels(t, h); q != want || d != want {
		t.Errorf("labels = %q, %q; want %q", q, d, want)
	}

	os.Remove(path)
	if q, d := waitingLabels(t, h); q != "" || d != "" {
		t.Errorf("labels after the lock was taken = %q, %q; want none", q, d)
	}
}

func TestWaitingLabel_DeadWaiterShowsNothing(t *testing.T) {
	t.Parallel()
	h, cwd := workingQueueRow(t)

	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	writeWaiting(t, h, check.Waiting{PID: cmd.Process.Pid, HeldBy: "RUN-A", Root: cwd})
	if q, d := waitingLabels(t, h); q != "" || d != "" {
		t.Errorf("labels for a dead waiter = %q, %q; want none", q, d)
	}
}

func TestWaitingLabel_WorktreeWithNoClaimedTicketShowsNothing(t *testing.T) {
	t.Parallel()
	h, _ := workingQueueRow(t)

	writeWaiting(t, h, check.Waiting{PID: os.Getpid(), HeldBy: "RUN-A", Root: t.TempDir()})
	if q, d := waitingLabels(t, h); q != "" || d != "" {
		t.Errorf("labels for another worktree = %q, %q; want none", q, d)
	}
}
