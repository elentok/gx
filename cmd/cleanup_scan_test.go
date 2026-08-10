package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/testutil"
)

func writeCleanupScanTicket(t *testing.T, dir, epic, filename, id, status, ticketType string) {
	t.Helper()
	path := filepath.Join(dir, ".scratch", epic, "issues", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: \"" + id + "\"\nstatus: " + status + "\ntype: " + ticketType + "\n---\nBody.\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestExecute_CleanupScan_EpicsJSON(t *testing.T) {
	dir := testutil.TempRepo(t)

	// epic-done-merged-review-done: all tickets done, branch merged (no
	// extra commits so it's trivially an ancestor of main), has a done
	// code-review ticket.
	writeCleanupScanTicket(t, dir, "epic-done-merged-review-done", "01-work.md", "01", "done", "task")
	writeCleanupScanTicket(t, dir, "epic-done-merged-review-done", "02-review.md", "02", "done", "code-review")
	testutil.MustGitExported(t, dir, "branch", "epic-done-merged-review-done")

	// epic-done-unmerged-review-pending: all tickets done except the
	// code-review ticket; branch has an unmerged commit.
	writeCleanupScanTicket(t, dir, "epic-done-unmerged-review-pending", "01-work.md", "01", "done", "task")
	writeCleanupScanTicket(t, dir, "epic-done-unmerged-review-pending", "02-review.md", "02", "open", "code-review")
	// Stage and commit only unmerged.txt (not "git add ." / CommitAll) so the
	// untracked .scratch ticket fixtures never get swept into the commit —
	// committing them would delete them from disk on the checkout back to
	// main, since main's tree doesn't have them.
	testutil.MustGitExported(t, dir, "checkout", "-b", "epic-done-unmerged-review-pending")
	testutil.WriteFile(t, dir, "unmerged.txt", "wip")
	testutil.MustGitExported(t, dir, "add", "unmerged.txt")
	testutil.MustGitExported(t, dir, "commit", "-m", "unmerged work")
	testutil.MustGitExported(t, dir, "checkout", "main")

	// epic-open-no-review: has an open ticket and no code-review ticket at all.
	writeCleanupScanTicket(t, dir, "epic-open-no-review", "01-work.md", "01", "open", "task")

	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return dir, nil },
	}

	if err := execute([]string{"cleanup", "scan", "--json"}, d); err != nil {
		t.Fatalf("execute cleanup scan --json: %v", err)
	}

	var result CleanupScanResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal output: %v\noutput: %s", err, stdout.String())
	}

	byName := map[string]EpicScan{}
	for _, e := range result.Epics {
		byName[e.Name] = e
	}

	if got, want := byName["epic-done-merged-review-done"], (EpicScan{
		Name: "epic-done-merged-review-done", AllDone: true, MergedToMain: true,
		HasCodeReviewTicket: true, CodeReviewDone: true,
	}); got != want {
		t.Errorf("epic-done-merged-review-done = %+v, want %+v", got, want)
	}

	if got, want := byName["epic-done-unmerged-review-pending"], (EpicScan{
		Name: "epic-done-unmerged-review-pending", AllDone: false, MergedToMain: false,
		HasCodeReviewTicket: true, CodeReviewDone: false,
	}); got != want {
		t.Errorf("epic-done-unmerged-review-pending = %+v, want %+v", got, want)
	}

	if got, want := byName["epic-open-no-review"], (EpicScan{
		Name: "epic-open-no-review", AllDone: false, MergedToMain: false,
		HasCodeReviewTicket: false, CodeReviewDone: false,
	}); got != want {
		t.Errorf("epic-open-no-review = %+v, want %+v", got, want)
	}
}

func TestExecute_CleanupScan_HousekeepingReportsTrackedFilesAndMissingGitignore(t *testing.T) {
	dir := testutil.TempRepo(t)
	testutil.Mkdir(t, filepath.Join(dir, ".scratch", "stray-epic"))
	testutil.WriteFile(t, dir, ".scratch/stray-epic/leaked.txt", "oops, this got committed")
	testutil.CommitAll(t, dir, "accidentally commit .scratch")

	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return dir, nil },
	}

	if err := execute([]string{"cleanup", "scan", "--json"}, d); err != nil {
		t.Fatalf("execute cleanup scan --json: %v", err)
	}

	var result CleanupScanResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal output: %v\noutput: %s", err, stdout.String())
	}

	hk := result.Housekeeping
	if hk.Skipped {
		t.Fatalf("expected housekeeping not to be skipped for a plain repo")
	}
	if len(hk.TrackedFiles) != 1 || hk.TrackedFiles[0] != ".scratch/stray-epic/leaked.txt" {
		t.Errorf("TrackedFiles = %v, want [.scratch/stray-epic/leaked.txt]", hk.TrackedFiles)
	}
	if hk.GitignoreHasScratch {
		t.Errorf("GitignoreHasScratch = true, want false (no .gitignore present)")
	}
}

func TestExecute_CleanupScan_HousekeepingSkippedAtBareRootWithNoWorktree(t *testing.T) {
	dir := testutil.TempBareRepo(t)

	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return dir, nil },
	}

	if err := execute([]string{"cleanup", "scan", "--json"}, d); err != nil {
		t.Fatalf("execute cleanup scan --json: %v", err)
	}

	var result CleanupScanResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal output: %v\noutput: %s", err, stdout.String())
	}

	if !result.Housekeeping.Skipped {
		t.Errorf("expected housekeeping to be skipped when cwd is a bare repo root with no worktree")
	}
}

func TestExecute_CleanupScan_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()

	var stdout, stderr bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: &stderr,
		getwd:  func() (string, error) { return dir, nil },
	}

	err := execute([]string{"cleanup", "scan"}, d)
	if err == nil {
		t.Fatal("expected error when cwd is outside a git repo, got nil")
	}
}
