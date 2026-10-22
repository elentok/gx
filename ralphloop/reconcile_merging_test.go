package ralphloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// deadPID is above every platform's max pid, so it is never Alive.
const deadPID = 99999999

func writeDeadMergeLock(t *testing.T, dir string, owner MergeLockOwner) {
	t.Helper()
	owner.PID = deadPID
	b, err := json.Marshal(owner)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, mergeLockFile), b, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// interruptedMergingDeps fakes a feature branch that moved from "pre" to
// "merged" while a dead gx held the merge lock for ticket 01, and an
// iteration branch whose commits have featureSubjects' subjects.
func interruptedMergingDeps(featureSubjects []string) (Deps, *[]string, *[]git.Trailer) {
	d, _, _ := fakeDeps()
	d.RevParse = func(dir, ref string) (string, error) {
		if ref == "HEAD" || ref == "epic" {
			return "merged", nil
		}
		return "iter-tip", nil
	}
	d.CommitSubjects = func(dir, fromExclusive, toRef string) ([]string, error) {
		if fromExclusive == "pre" {
			return featureSubjects, nil
		}
		return []string{"Add A"}, nil
	}
	var mu sync.Mutex
	picks := &[]string{}
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		mu.Lock()
		defer mu.Unlock()
		*picks = append(*picks, fromExclusive+".."+toInclusive)
		return nil
	}
	trailers := &[]git.Trailer{}
	d.AppendTrailers = func(dir string, ts ...git.Trailer) error {
		mu.Lock()
		defer mu.Unlock()
		*trailers = append(*trailers, ts...)
		return nil
	}
	return d, picks, trailers
}

func TestReconcile_InterruptedMerging_RecordsItInsteadOfRepicking(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	lockDir, err := mergeLockDir(scratchDir, "epic")
	if err != nil {
		t.Fatal(err)
	}
	writeDeadMergeLock(t, lockDir, MergeLockOwner{Time: time.Now().UTC(), Epic: "epic", Ticket: "01", PrePickHead: "pre"})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	d, picks, trailers := interruptedMergingDeps([]string{"Add A"})

	if _, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, noopEventSink{}), epics[0]); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if len(*picks) != 0 {
		t.Errorf("cherry-picks = %v, want none: the interrupted merging is already on the feature branch", *picks)
	}
	stamped := false
	for _, tr := range *trailers {
		stamped = stamped || tr.Key == ticketTrailerKey
	}
	if !stamped {
		t.Errorf("trailers = %v, want the ticket trailer stamped on the merged commit", *trailers)
	}
	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket 01 = %s, want done", raw)
	}
	events, _, err := ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if sha := latestCherryPickedSHA(events, "01"); sha != "merged" {
		t.Errorf("recorded cherry-picked SHA = %q, want %q", sha, "merged")
	}
	if owner, err := ReadMergeLock(lockDir); err != nil || owner != nil {
		t.Errorf("merge lock after reconcile = %+v, %v; want released", owner, err)
	}
}

func TestReconcile_DeadMergeLockWithUnrelatedCommits_ReleasedAndRepicked(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	lockDir, err := mergeLockDir(scratchDir, "epic")
	if err != nil {
		t.Fatal(err)
	}
	writeDeadMergeLock(t, lockDir, MergeLockOwner{Time: time.Now().UTC(), Epic: "epic", Ticket: "01", PrePickHead: "pre"})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	d, picks, _ := interruptedMergingDeps([]string{"Something a human committed"})

	if _, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, noopEventSink{}), epics[0]); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if len(*picks) != 1 {
		t.Errorf("cherry-picks = %v, want one: the feature branch's new commits aren't 01's", *picks)
	}
	events, _, err := ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	for _, ev := range events {
		if ev.Type == string(eventsc.CherryPicked) && ev.SHA == "merged" && ev.Reason != "" {
			t.Errorf("event %+v: want no interrupted-merging record for unrelated commits", ev)
		}
	}
	if owner, err := ReadMergeLock(lockDir); err != nil || owner != nil {
		t.Errorf("merge lock after reconcile = %+v, %v; want released", owner, err)
	}
}

func TestReconcile_OrphanedClaimMergingHoldsMergeLock(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	lockDir, err := mergeLockDir(scratchDir, "epic")
	if err != nil {
		t.Fatal(err)
	}
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	d, _, _ := fakeDeps()
	var owner *MergeLockOwner
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		owner, _ = ReadMergeLock(lockDir)
		return nil
	}

	if _, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, noopEventSink{}), epics[0]); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if owner == nil || owner.Ticket != "01" || owner.PrePickHead == "" {
		t.Errorf("merge lock during the pick = %+v, want held for 01 with its pre-pick head", owner)
	}
	if after, _ := ReadMergeLock(lockDir); after != nil {
		t.Errorf("merge lock after reconcile = %+v, want released", after)
	}
}
