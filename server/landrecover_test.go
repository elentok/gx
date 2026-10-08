package server_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// orphanLandLock leaves the lock a land that died mid-flight would: a dead
// owner, no marker.
func orphanLandLock(t *testing.T, store string) string {
	t.Helper()
	dir, err := ralphloop.LandLockDir(filepath.Join(store, "proj", "epic-a"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	owner, _ := json.Marshal(ralphloop.LandLockOwner{PID: 2147483646, Time: time.Now().UTC(), Epic: "epic-a", Ticket: "01"})
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

func startCrashedLand(t *testing.T) (h *servertest.Harness, store, repo, lockDir string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store, repo = t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h = servertest.StartWithStore(t, store)
	// Down, so the lock is only seen by the server that restarts onto it.
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	lockDir = orphanLandLock(t, store)
	return h, store, repo, lockDir
}

func TestLandRecovery_OrphanLockForALandedTicketFinishesIt(t *testing.T) {
	h, store, repo, lockDir := startCrashedLand(t)
	wt, ticketPath := leaveIteration(t, h, store, repo)
	_ = os.Remove(filepath.Join(h.StateDir, "runs.json"))
	// The land got as far as the cherry-pick before the process died. -x keeps the
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
		t.Errorf("land lock still there: %v", err)
	}
}

func TestLandRecovery_OrphanLockForAnUnlandedTicketRollsBackTheHalfPick(t *testing.T) {
	h, store, repo, lockDir := startCrashedLand(t)
	wt, ticketPath := leaveIteration(t, h, store, repo)
	_ = os.Remove(filepath.Join(h.StateDir, "runs.json"))
	// A conflicting cherry-pick the crash left in progress.
	feature := filepath.Join(filepath.Dir(wt), "epic-a")
	testutil.WriteFile(t, feature, "agent.txt", "conflicting")
	testutil.CommitAll(t, feature, "conflict")
	_ = exec.Command("git", "-C", feature, "cherry-pick", ralphloop.IterationBranch("epic-a", "01")).Run()

	restartAndSettle(t, h, server.EventLandRolledBack)

	if err := exec.Command("git", "-C", feature, "rev-parse", "-q", "--verify", "CHERRY_PICK_HEAD").Run(); err == nil {
		t.Error("cherry-pick still in progress")
	}
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status != schema.StatusClaimed {
		t.Errorf("status=%q err=%v, want claimed", tk.Status, err)
	}
	if _, err := os.Stat(filepath.Join(lockDir, "land.lock")); !os.IsNotExist(err) {
		t.Errorf("land lock still there: %v", err)
	}
}

func TestLandRecovery_OrphanLockWithNoEvidenceParksAmbiguousLand(t *testing.T) {
	h, store, _, lockDir := startCrashedLand(t)
	ticketPath := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	if err := ralphloop.Claim(ticketPath); err != nil {
		t.Fatal(err)
	}

	restartAndSettle(t, h, server.EventTicketParked)

	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusNeedsRepair || tk.ParkKind != schema.ParkKind(events.AmbiguousLand) {
		t.Errorf("status=%q park_kind=%q, want needs-repair/ambiguous-land", tk.Status, tk.ParkKind)
	}
	if _, err := os.Stat(filepath.Join(lockDir, "land.lock")); !os.IsNotExist(err) {
		t.Errorf("land lock still there: %v", err)
	}
}
