package cmd

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// scanGit runs git in dir with the commit dates pinned to when, so a test can
// age a branch's tip past worktreeActiveWindow.
func scanGit(t *testing.T, dir string, when time.Time, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	stamp := when.Format(time.RFC3339)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// scanCommitOnBranch creates branch from main with one commit made at when,
// then returns to main. It returns the new commit's hash.
func scanCommitOnBranch(t *testing.T, dir, branch string, when time.Time) string {
	t.Helper()
	scanGit(t, dir, when, "checkout", "-b", branch, "main")
	testutil.WriteFile(t, dir, strings.ReplaceAll(branch, "/", "_")+".txt", branch)
	scanGit(t, dir, when, "add", ".")
	scanGit(t, dir, when, "commit", "-m", "work on "+branch)
	hash := scanGit(t, dir, when, "rev-parse", "HEAD")
	scanGit(t, dir, when, "checkout", "main")
	return hash
}

func scanByBranch(t *testing.T, repo git.Repo, epics []tickets.Epic) map[string]WorktreeScan {
	t.Helper()
	scans, err := scanWorktrees(repo, epics)
	if err != nil {
		t.Fatalf("scanWorktrees: %v", err)
	}
	byBranch := map[string]WorktreeScan{}
	for _, s := range scans {
		byBranch[s.Branch] = s
	}
	return byBranch
}

func testEpic(name, ticketID, status string) tickets.Epic {
	return tickets.Epic{
		Name:    name,
		Tickets: []tickets.Ticket{{Identifier: ticketID, Status: status}},
	}
}

func TestScanWorktrees_CurrentPatternIterationBranch_MergedAndDone(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	old := time.Now().Add(-10 * 24 * time.Hour)
	hash := scanCommitOnBranch(t, dir, "ralph-loop/bugs-item-04", old)
	// Cherry-picked, so the epic branch holds a fresh commit that is not
	// reachable from the iteration branch.
	scanGit(t, dir, old, "checkout", "-b", "bugs", "main")
	scanGit(t, dir, old, "cherry-pick", hash)
	scanGit(t, dir, old, "checkout", "main")

	repo := git.Repo{Root: dir, Trunk: "main"}
	scan := scanByBranch(t, repo, []tickets.Epic{testEpic("bugs", "04", "done")})["ralph-loop/bugs-item-04"]

	if scan.Kind != "iteration" {
		t.Fatalf("Kind = %q, want iteration", scan.Kind)
	}
	if scan.Epic != "bugs" || scan.TicketID != "04" {
		t.Errorf("Epic/TicketID = %q/%q, want bugs/04", scan.Epic, scan.TicketID)
	}
	if !scan.TicketDone {
		t.Errorf("TicketDone = false, want true")
	}
	if !scan.Merged {
		t.Errorf("Merged = false, want true (cherry-picked onto bugs)")
	}
	if scan.Recommendation != "delete" {
		t.Errorf("Recommendation = %q, want delete", scan.Recommendation)
	}
}

func TestScanWorktrees_LegacyPatternIterationBranch_MergedAndDone(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	old := time.Now().Add(-10 * 24 * time.Hour)
	scanCommitOnBranch(t, dir, "ralph-loop/retrofit/iter-01", old)
	scanGit(t, dir, old, "branch", "retrofit", "ralph-loop/retrofit/iter-01")

	repo := git.Repo{Root: dir, Trunk: "main"}
	scan := scanByBranch(t, repo, []tickets.Epic{testEpic("retrofit", "01", "done")})["ralph-loop/retrofit/iter-01"]

	if scan.Kind != "iteration" {
		t.Fatalf("Kind = %q, want iteration", scan.Kind)
	}
	if scan.Epic != "retrofit" || scan.TicketID != "01" {
		t.Errorf("Epic/TicketID = %q/%q, want retrofit/01", scan.Epic, scan.TicketID)
	}
	if !scan.TicketDone {
		t.Errorf("TicketDone = false, want true")
	}
	if !scan.Merged {
		t.Errorf("Merged = false, want true (branch tip is an ancestor of retrofit)")
	}
	if scan.Recommendation != "delete" {
		t.Errorf("Recommendation = %q, want delete", scan.Recommendation)
	}
}

func TestScanWorktrees_FeatureBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	scanCommitOnBranch(t, dir, "cleanup", time.Now().Add(-10*24*time.Hour))

	repo := git.Repo{Root: dir, Trunk: "main"}
	scan := scanByBranch(t, repo, []tickets.Epic{testEpic("cleanup", "01", "open")})["cleanup"]

	if scan.Kind != "feature" {
		t.Fatalf("Kind = %q, want feature", scan.Kind)
	}
	if scan.Epic != "cleanup" {
		t.Errorf("Epic = %q, want cleanup", scan.Epic)
	}
}

func TestScanWorktrees_OtherBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	scanCommitOnBranch(t, dir, "wip", time.Now().Add(-10*24*time.Hour))

	repo := git.Repo{Root: dir, Trunk: "main"}
	scan := scanByBranch(t, repo, nil)["wip"]

	if scan.Kind != "other" {
		t.Fatalf("Kind = %q, want other", scan.Kind)
	}
}

func TestScanWorktrees_ActiveWorkNeverRecommended(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	// Merged into trunk and done, so only the recent tip suppresses "delete".
	scanCommitOnBranch(t, dir, "ralph-loop/bugs-item-05", time.Now())
	scanGit(t, dir, time.Now(), "merge", "--ff-only", "ralph-loop/bugs-item-05")

	repo := git.Repo{Root: dir, Trunk: "main"}
	scan := scanByBranch(t, repo, []tickets.Epic{testEpic("bugs", "05", "done")})["ralph-loop/bugs-item-05"]

	if !scan.Active {
		t.Fatalf("Active = false, want true (branch has a very recent commit)")
	}
	if !scan.TicketDone || !scan.Merged {
		t.Fatalf("TicketDone/Merged = %v/%v, want both true", scan.TicketDone, scan.Merged)
	}
	if scan.Recommendation != "" {
		t.Errorf("Recommendation = %q, want empty when Active is true", scan.Recommendation)
	}
}

func TestClassifyBranches_SkipsVanishedBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.TempRepo(t)
	scanCommitOnBranch(t, dir, "wip", time.Now().Add(-10*24*time.Hour))

	repo := git.Repo{Root: dir, Trunk: "main"}
	scans, err := classifyBranches(repo, []string{"wip", "gone"}, nil, nil)
	if err != nil {
		t.Fatalf("classifyBranches: %v", err)
	}
	if len(scans) != 1 || scans[0].Branch != "wip" {
		t.Errorf("scans = %+v, want only wip", scans)
	}
}
