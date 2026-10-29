package server_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// orphanMergeLock leaves the lock a merge that died mid-flight would: a dead
// owner, no marker.
func orphanMergeLock(t *testing.T, h *servertest.Harness, store string) string {
	t.Helper()
	// The lock root is the server's config's, not the process's.
	stateDir := filepath.Join(h.Env("XDG_STATE_HOME"), "gx")
	dir, err := ralphloop.MergeLockDirIn(stateDir, filepath.Join(store, "proj", "epic-a"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	owner, _ := json.Marshal(ralphloop.MergeLockOwner{PID: 2147483646, Time: time.Now().UTC(), Epic: "epic-a", Ticket: "01"})
	if err := os.WriteFile(filepath.Join(dir, "land.lock"), owner, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// restartAndSettle restarts the server and waits for the event the recovery
// publishes.
func restartAndSettle(t *testing.T, h *servertest.Harness, want string) {
	t.Helper()
	h.Restart(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	for ev := range evs {
		if ev.Type == want {
			return
		}
	}
	log, _ := os.ReadFile(server.LogPath(h.StateDir))
	t.Fatalf("never saw %s\n%s", want, log)
}

func startCrashedMerge(t *testing.T) (h *servertest.Harness, store, repo, lockDir string) {
	t.Helper()
	store, repo = t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h = servertest.StartWithStore(t, store)
	// Down, so the lock is only seen by the server that restarts onto it.
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	lockDir = orphanMergeLock(t, h, store)
	return h, store, repo, lockDir
}

func TestMergeRecovery_OrphanLockForAMergedTicketFinishesIt(t *testing.T) {
	t.Parallel()
	h, store, repo, lockDir := startCrashedMerge(t)
	wt, ticketPath := leaveIteration(t, h, store, repo, "herdr")
	_ = os.Remove(filepath.Join(h.StateDir, "runs.json"))
	// The merge got as far as the cherry-pick before the process died. -x keeps the
	// picked commit from hashing identically to its source within one second.
	feature := filepath.Join(filepath.Dir(wt), "epic-a")
	if out, err := exec.Command("git", "-C", feature, "cherry-pick", "-x", ralphloop.IterationBranch("epic-a", "01")).CombinedOutput(); err != nil {
		t.Fatalf("cherry-pick: %v\n%s", err, out)
	}

	restartAndSettle(t, h, server.EventTicketDone)

	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status != schema.StatusDone {
		t.Errorf("status=%q err=%v, want done", tk.Status, err)
	}
	if _, err := os.Stat(filepath.Join(lockDir, "land.lock")); !os.IsNotExist(err) {
		t.Errorf("merge lock still there: %v", err)
	}
}

func TestMergeRecovery_OrphanLockForAnUnmergedTicketRollsBackTheHalfPick(t *testing.T) {
	t.Parallel()
	h, store, repo, lockDir := startCrashedMerge(t)
	wt, ticketPath := leaveIteration(t, h, store, repo, "herdr")
	_ = os.Remove(filepath.Join(h.StateDir, "runs.json"))
	// A conflicting cherry-pick the crash left in progress.
	feature := filepath.Join(filepath.Dir(wt), "epic-a")
	testutil.WriteFile(t, feature, "agent.txt", "conflicting")
	testutil.CommitAll(t, feature, "conflict")
	_ = exec.Command("git", "-C", feature, "cherry-pick", ralphloop.IterationBranch("epic-a", "01")).Run()

	restartAndSettle(t, h, server.EventMergeRolledBack)

	if err := exec.Command("git", "-C", feature, "rev-parse", "-q", "--verify", "CHERRY_PICK_HEAD").Run(); err == nil {
		t.Error("cherry-pick still in progress")
	}
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status != schema.StatusClaimed {
		t.Errorf("status=%q err=%v, want claimed", tk.Status, err)
	}
	if _, err := os.Stat(filepath.Join(lockDir, "land.lock")); !os.IsNotExist(err) {
		t.Errorf("merge lock still there: %v", err)
	}
}

func TestMergeRecovery_OrphanLockWithNoEvidencePausesAmbiguousMerge(t *testing.T) {
	t.Parallel()
	h, store, _, lockDir := startCrashedMerge(t)
	ticketPath := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	if err := ralphloop.Claim(ticketPath); err != nil {
		t.Fatal(err)
	}

	restartAndSettle(t, h, server.EventTicketPaused)

	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	assertPaused(t, tk, schema.PauseClassRepair, schema.PauseAmbiguousMerge)
	if _, err := os.Stat(filepath.Join(lockDir, "land.lock")); !os.IsNotExist(err) {
		t.Errorf("merge lock still there: %v", err)
	}
}
