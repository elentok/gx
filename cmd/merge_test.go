package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// mergeTestRepo creates a plain bare repo with a linked worktree for main
// (at mainDir) plus one linked worktree per branch/dir pair in worktrees
// (map of dir name -> branch name); each worktree's branch is created off
// main's tip. It returns the repo dir and main's worktree path.
func mergeTestRepo(t *testing.T, worktrees map[string]string) (repoDir, mainDir string) {
	t.Helper()
	repoDir = testutil.TempBareRepo(t)
	mainDir = filepath.Join(repoDir, "main")
	testutil.MustGitExported(t, repoDir, "worktree", "add", mainDir, "main")

	for dir, branch := range worktrees {
		wtDir := filepath.Join(repoDir, dir)
		testutil.MustGitExported(t, repoDir, "worktree", "add", "-b", branch, wtDir)
		testutil.MustGitExported(t, wtDir, "config", "user.email", "test@test.com")
		testutil.MustGitExported(t, wtDir, "config", "user.name", "Test")
	}
	return repoDir, mainDir
}

func commitFile(t *testing.T, dir, name, content string) {
	t.Helper()
	testutil.WriteFile(t, dir, name, content)
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "add "+name)
}

func decodeMergeResult(t *testing.T, stdout *bytes.Buffer) MergeResult {
	t.Helper()
	var result MergeResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, stdout.String())
	}
	return result
}

func TestRunMerge_CleanFastForward(t *testing.T) {
	t.Parallel()
	repoDir, mainDir := mergeTestRepo(t, map[string]string{"feature": "feature"})
	commitFile(t, filepath.Join(repoDir, "feature"), "feature.txt", "hi")

	var stdout bytes.Buffer
	if err := runMerge(mainDir, "feature", true, &stdout); err != nil {
		t.Fatalf("runMerge: %v", err)
	}

	result := decodeMergeResult(t, &stdout)
	if result.Status != "merged" {
		t.Errorf("Status = %q, want merged", result.Status)
	}

	head, err := git.HeadCommit(mainDir, "main")
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	if head.Subject != "add feature.txt" {
		t.Errorf("main tip subject = %q, want %q", head.Subject, "add feature.txt")
	}
}

func TestRunMerge_Diverged(t *testing.T) {
	t.Parallel()
	repoDir, mainDir := mergeTestRepo(t, map[string]string{"feature": "feature"})
	commitFile(t, filepath.Join(repoDir, "feature"), "feature.txt", "hi")
	commitFile(t, mainDir, "main-only.txt", "diverge")

	var stdout bytes.Buffer
	if err := runMerge(mainDir, "feature", true, &stdout); err != nil {
		t.Fatalf("runMerge: %v", err)
	}

	result := decodeMergeResult(t, &stdout)
	if result.Status != "needs_rebase" {
		t.Fatalf("Status = %q, want needs_rebase", result.Status)
	}
	if result.Branch != "feature" {
		t.Errorf("Branch = %q, want feature", result.Branch)
	}
	if result.Target != "main" {
		t.Errorf("Target = %q, want main", result.Target)
	}
	if result.WorktreePath != filepath.Join(repoDir, "feature") {
		t.Errorf("WorktreePath = %q, want %q", result.WorktreePath, filepath.Join(repoDir, "feature"))
	}
}

func TestRunMerge_ResolvesWorktreeDirNameToRealBranch(t *testing.T) {
	t.Parallel()
	// A nested branch name checked out under a shorter worktree dir name,
	// mirroring ralph-loop's iteration branches.
	repoDir, mainDir := mergeTestRepo(t, map[string]string{"item-01": "ralph-loop/epic-item-01"})
	commitFile(t, filepath.Join(repoDir, "item-01"), "work.txt", "hi")

	var stdout bytes.Buffer
	if err := runMerge(mainDir, "item-01", true, &stdout); err != nil {
		t.Fatalf("runMerge: %v", err)
	}

	result := decodeMergeResult(t, &stdout)
	if result.Status != "merged" {
		t.Fatalf("Status = %q, want merged", result.Status)
	}
}

func TestRunMerge_NoMatchingWorktreeTreatsArgAsLiteralBranch(t *testing.T) {
	t.Parallel()
	_, mainDir := mergeTestRepo(t, map[string]string{})
	// A branch that exists but has no linked worktree of its own.
	testutil.MustGitExported(t, mainDir, "branch", "literal-branch")

	var stdout bytes.Buffer
	if err := runMerge(mainDir, "literal-branch", true, &stdout); err != nil {
		t.Fatalf("runMerge: %v", err)
	}

	result := decodeMergeResult(t, &stdout)
	// literal-branch points at the same commit as main, so it's trivially a
	// fast-forward (a no-op merge); the point of this test is that
	// resolution didn't error out or mis-resolve for lack of a worktree.
	if result.Status != "merged" {
		t.Fatalf("Status = %q, want merged", result.Status)
	}
}

func TestRunMerge_TextOutput(t *testing.T) {
	t.Parallel()
	repoDir, mainDir := mergeTestRepo(t, map[string]string{"feature": "feature"})
	commitFile(t, filepath.Join(repoDir, "feature"), "feature.txt", "hi")

	var stdout bytes.Buffer
	if err := runMerge(mainDir, "feature", false, &stdout); err != nil {
		t.Fatalf("runMerge: %v", err)
	}
	if got, want := stdout.String(), "merged onto main\n"; got != want {
		t.Errorf("text output = %q, want %q", got, want)
	}
}

func revParse(t *testing.T, dir, ref string) string {
	t.Helper()
	sha, err := git.RevParse(dir, ref)
	if err != nil {
		t.Fatalf("rev-parse %s: %v", ref, err)
	}
	return sha
}

// registerWithTrunk isolates the ticket store and registers repoDir as a
// project whose project.json sets trunk. Not parallel-safe (sets env).
func registerWithTrunk(t *testing.T, repoDir, trunk string) {
	t.Helper()
	projectDir := addProject(t, isolateTicketStore(t), "p", repoDir)
	testutil.WriteFile(t, projectDir, "project.json",
		`{"name":"p","repo":"`+repoDir+`","trunk":"`+trunk+`"}`)
}

func TestRunMerge_LandsOnConfiguredTrunk(t *testing.T) {
	repoDir, mainDir := mergeTestRepo(t, map[string]string{"feature": "feature"})
	testutil.MustGitExported(t, mainDir, "branch", "develop")
	registerWithTrunk(t, repoDir, "develop")
	commitFile(t, filepath.Join(repoDir, "feature"), "feature.txt", "hi")
	mainBefore := revParse(t, mainDir, "main")

	var stdout bytes.Buffer
	if err := runMerge(mainDir, "feature", true, &stdout); err != nil {
		t.Fatalf("runMerge: %v", err)
	}

	if got, want := decodeMergeResult(t, &stdout), (MergeResult{Status: "merged", Target: "develop"}); got != want {
		t.Errorf("result = %+v, want %+v", got, want)
	}
	if got, want := revParse(t, mainDir, "develop"),
		revParse(t, mainDir, "feature"); got != want {
		t.Errorf("develop = %s, want feature tip %s", got, want)
	}
	if got := revParse(t, mainDir, "main"); got != mainBefore {
		t.Errorf("main moved from %s to %s", mainBefore, got)
	}
}

func TestRunMerge_TextOutputNamesConfiguredTrunk(t *testing.T) {
	repoDir, mainDir := mergeTestRepo(t, map[string]string{"feature": "feature"})
	testutil.MustGitExported(t, mainDir, "branch", "develop")
	registerWithTrunk(t, repoDir, "develop")
	commitFile(t, filepath.Join(repoDir, "feature"), "feature.txt", "hi")
	commitFile(t, mainDir, "main-only.txt", "x")
	testutil.MustGitExported(t, mainDir, "branch", "-f", "develop", "main")

	var stdout bytes.Buffer
	if err := runMerge(mainDir, "feature", false, &stdout); err != nil {
		t.Fatalf("runMerge: %v", err)
	}
	if got := stdout.String(); !strings.HasPrefix(got, "needs rebase: feature onto develop\n") {
		t.Errorf("text output = %q, want needs rebase onto develop", got)
	}
}

func TestRunMerge_UnregisteredRepoUsesDetectedDefault(t *testing.T) {
	isolateTicketStore(t)
	repoDir, mainDir := mergeTestRepo(t, map[string]string{"feature": "feature"})
	commitFile(t, filepath.Join(repoDir, "feature"), "feature.txt", "hi")

	var stdout bytes.Buffer
	if err := runMerge(mainDir, "feature", true, &stdout); err != nil {
		t.Fatalf("runMerge: %v", err)
	}
	if got, want := decodeMergeResult(t, &stdout), (MergeResult{Status: "merged", Target: "main"}); got != want {
		t.Errorf("result = %+v, want %+v", got, want)
	}
}

func TestRunMerge_MissingTrunkFailsWithSharedError(t *testing.T) {
	repoDir, mainDir := mergeTestRepo(t, map[string]string{"feature": "feature"})
	registerWithTrunk(t, repoDir, "develop")
	commitFile(t, filepath.Join(repoDir, "feature"), "feature.txt", "hi")
	mainBefore := revParse(t, mainDir, "main")

	var stdout bytes.Buffer
	err := runMerge(mainDir, "feature", true, &stdout)
	var missing *tickets.ErrTrunkMissing
	if !errors.As(err, &missing) || missing.Trunk != "develop" {
		t.Fatalf("err = %v, want ErrTrunkMissing for develop", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
	if got := revParse(t, mainDir, "main"); got != mainBefore {
		t.Errorf("main moved from %s to %s", mainBefore, got)
	}
}
