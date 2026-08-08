package cmd

import (
	"os"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// realDevRepoScan runs scanWorktrees against the actual repo this test
// itself is running inside (not a synthesized testutil.TempRepo). Per
// ticket 02's test-seams note, this repo's own dev history already has a
// real current-pattern iteration branch, a real legacy-pattern one, real
// feature/other branches, and (via the very branch this ticket's commits
// land on) a real very-recent-commit worktree — exactly the fixtures the
// ticket asks to assert against directly, instead of synthesizing new ones.
// If a named branch is missing (e.g. it was since cleaned up), the caller
// skips rather than failing, since this test's fixtures live outside the
// repo's own control.
func realDevRepoScan(t *testing.T) map[string]WorktreeScan {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	info, err := git.IdentifyDir(cwd)
	if err != nil {
		t.Skipf("not inside a git repo: %v", err)
	}

	epics, err := tickets.Load(info.Repo.ScratchRoot())
	if err != nil {
		t.Skipf("loading .scratch epics: %v", err)
	}

	scans, err := scanWorktrees(info.Repo, epics)
	if err != nil {
		t.Fatalf("scanWorktrees: %v", err)
	}

	byBranch := map[string]WorktreeScan{}
	for _, s := range scans {
		byBranch[s.Branch] = s
	}
	return byBranch
}

func requireBranch(t *testing.T, byBranch map[string]WorktreeScan, branch string) WorktreeScan {
	t.Helper()
	scan, ok := byBranch[branch]
	if !ok {
		t.Skipf("fixture branch %q not present in this repo's current state", branch)
	}
	return scan
}

func TestScanWorktrees_RealRepo_CurrentPatternIterationBranch_LandedAndDone(t *testing.T) {
	scan := requireBranch(t, realDevRepoScan(t), "ralph-loop/bugs-01-item-04")

	if scan.Kind != "iteration" {
		t.Fatalf("Kind = %q, want iteration", scan.Kind)
	}
	if scan.Epic != "bugs-01" || scan.TicketID != "04" {
		t.Errorf("Epic/TicketID = %q/%q, want bugs-01/04", scan.Epic, scan.TicketID)
	}
	if !scan.TicketDone {
		t.Errorf("TicketDone = false, want true (bugs-01 ticket 04 is done)")
	}
	if !scan.Landed {
		t.Errorf("Landed = false, want true (cherry-picked onto bugs-01)")
	}
}

func TestScanWorktrees_RealRepo_LegacyPatternIterationBranch_LandedAndDone(t *testing.T) {
	scan := requireBranch(t, realDevRepoScan(t), "ralph-loop/gx-tickets-set-retrofit/iter-01")

	if scan.Kind != "iteration" {
		t.Fatalf("Kind = %q, want iteration", scan.Kind)
	}
	if scan.Epic != "gx-tickets-set-retrofit" || scan.TicketID != "01" {
		t.Errorf("Epic/TicketID = %q/%q, want gx-tickets-set-retrofit/01", scan.Epic, scan.TicketID)
	}
	if !scan.TicketDone {
		t.Errorf("TicketDone = false, want true (gx-tickets-set-retrofit ticket 01 is done)")
	}
	if !scan.Landed {
		t.Errorf("Landed = false, want true (branch tip is an ancestor of gx-tickets-set-retrofit)")
	}
}

func TestScanWorktrees_RealRepo_FeatureBranch(t *testing.T) {
	byBranch := realDevRepoScan(t)
	scan := requireBranch(t, byBranch, "gx-cleanup")

	if scan.Kind != "feature" {
		t.Fatalf("Kind = %q, want feature", scan.Kind)
	}
	if scan.Epic != "gx-cleanup" {
		t.Errorf("Epic = %q, want gx-cleanup", scan.Epic)
	}
}

func TestScanWorktrees_RealRepo_OtherBranch(t *testing.T) {
	byBranch := realDevRepoScan(t)
	// "wip" matches no epic under .scratch and no ralph-loop/ iteration
	// naming - a real orphan/other branch from this repo's own history.
	scan := requireBranch(t, byBranch, "wip")

	if scan.Kind != "other" {
		t.Fatalf("Kind = %q, want other", scan.Kind)
	}
}

// TestScanWorktrees_RealRepo_ActiveWorkNeverRecommended exercises the
// active-work exclusion against this ticket's own branch: by the time this
// test runs, ticket 01's commit landed on it moments ago, so it's a real
// worktree with a genuinely recent commit - not a synthesized one.
func TestScanWorktrees_RealRepo_ActiveWorkNeverRecommended(t *testing.T) {
	byBranch := realDevRepoScan(t)
	scan := requireBranch(t, byBranch, "ralph-loop/gx-cleanup-item-02")

	if !scan.Active {
		t.Fatalf("Active = false, want true (branch has a very recent commit)")
	}
	if scan.Recommendation != "" {
		t.Errorf("Recommendation = %q, want empty when Active is true", scan.Recommendation)
	}
}
