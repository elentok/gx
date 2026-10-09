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

func writeDeadLandLock(t *testing.T, dir string, owner LandLockOwner) {
	t.Helper()
	owner.PID = deadPID
	b, err := json.Marshal(owner)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, landLockFile), b, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// interruptedLandingDeps fakes a feature branch that moved from "pre" to
// "landed" while a dead gx held the land lock for ticket 01, and an
// iteration branch whose commits have featureSubjects' subjects.
func interruptedLandingDeps(featureSubjects []string) (Deps, *[]string, *[]git.Trailer) {
	d, _, _ := fakeDeps()
	d.RevParse = func(dir, ref string) (string, error) {
		if ref == "HEAD" || ref == "epic" {
			return "landed", nil
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

func TestReconcile_InterruptedLanding_RecordsItInsteadOfRepicking(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	lockDir, err := landLockDir(scratchDir, "epic")
	if err != nil {
		t.Fatal(err)
	}
	writeDeadLandLock(t, lockDir, LandLockOwner{Time: time.Now().UTC(), Epic: "epic", Ticket: "01", PrePickHead: "pre"})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	d, picks, trailers := interruptedLandingDeps([]string{"Add A"})

	if _, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, noopEventSink{}), epics[0]); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if len(*picks) != 0 {
		t.Errorf("cherry-picks = %v, want none: the interrupted landing is already on the feature branch", *picks)
	}
	stamped := false
	for _, tr := range *trailers {
		stamped = stamped || tr.Key == ticketTrailerKey
	}
	if !stamped {
		t.Errorf("trailers = %v, want the ticket trailer stamped on the landed commit", *trailers)
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
	if sha := latestCherryPickedSHA(events, "01"); sha != "landed" {
		t.Errorf("recorded cherry-picked SHA = %q, want %q", sha, "landed")
	}
	if owner, err := ReadLandLock(lockDir); err != nil || owner != nil {
		t.Errorf("land lock after reconcile = %+v, %v; want released", owner, err)
	}
}

func TestReconcile_DeadLandLockWithUnrelatedCommits_ReleasedAndRepicked(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	lockDir, err := landLockDir(scratchDir, "epic")
	if err != nil {
		t.Fatal(err)
	}
	writeDeadLandLock(t, lockDir, LandLockOwner{Time: time.Now().UTC(), Epic: "epic", Ticket: "01", PrePickHead: "pre"})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	d, picks, _ := interruptedLandingDeps([]string{"Something a human committed"})

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
		if ev.Type == string(eventsc.CherryPicked) && ev.SHA == "landed" && ev.Reason != "" {
			t.Errorf("event %+v: want no interrupted-landing record for unrelated commits", ev)
		}
	}
	if owner, err := ReadLandLock(lockDir); err != nil || owner != nil {
		t.Errorf("land lock after reconcile = %+v, %v; want released", owner, err)
	}
}

func TestReconcile_OrphanedClaimLandingHoldsLandLock(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	lockDir, err := landLockDir(scratchDir, "epic")
	if err != nil {
		t.Fatal(err)
	}
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	d, _, _ := fakeDeps()
	var owner *LandLockOwner
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		owner, _ = ReadLandLock(lockDir)
		return nil
	}

	if _, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, noopEventSink{}), epics[0]); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if owner == nil || owner.Ticket != "01" || owner.PrePickHead == "" {
		t.Errorf("land lock during the pick = %+v, want held for 01 with its pre-pick head", owner)
	}
	if after, _ := ReadLandLock(lockDir); after != nil {
		t.Errorf("land lock after reconcile = %+v, want released", after)
	}
}
