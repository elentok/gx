package worktrees_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gx/git"
	"gx/testutil"
	teatest "gx/testutil/teatestv2"
	"gx/ui/worktrees"

	tea "charm.land/bubbletea/v2"
)

const (
	termWidth  = 120
	termHeight = 40
	loadWait   = 5 * time.Second
	actionWait = 3 * time.Second
)

func startTUI(t *testing.T, repoDir string) (git.Repo, *teatest.TestModel) {
	t.Helper()
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	m := worktrees.New(*repo, "")
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(termWidth, termHeight))
	return *repo, tm
}

func waitForText(t *testing.T, tm *teatest.TestModel, text string, timeout time.Duration) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte(text))
	}, teatest.WithDuration(timeout))
}

// waitForTexts waits until a single output frame contains ALL of the given
// strings. Use this instead of chained waitForText calls when the strings
// may all appear in the same render batch — a fast system can deliver them
// together and the subsequent waitForText would see an empty buffer.
func waitForTexts(t *testing.T, tm *teatest.TestModel, timeout time.Duration, texts ...string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		for _, text := range texts {
			if !bytes.Contains(bts, []byte(text)) {
				return false
			}
		}
		return true
	}, teatest.WithDuration(timeout))
}

func quit(t *testing.T, tm *teatest.TestModel) {
	t.Helper()
	tm.Send(keyRune('q'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func keyCtrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

func keySpecial(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// ── delete ────────────────────────────────────────────────────────────────────

func TestDeleteConfirmationAppearsAndCancels(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	_, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	// Enter delete mode
	tm.Send(keyRune('d'))
	waitForText(t, tm, "Delete", actionWait)

	// Cancel with esc — should return to normal without crashing
	tm.Send(keySpecial(tea.KeyEsc))

	quit(t, tm)
}

func TestDeleteWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	repo, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	// Delete the selected (first) worktree
	tm.Send(keyRune('d'))
	waitForText(t, tm, "Delete", actionWait)
	tm.Send(keyRune('y'))

	// Wait until git actually has only 1 worktree left
	teatest.WaitFor(t, tm.Output(), func(_ []byte) bool {
		wts, err := git.ListWorktrees(repo)
		return err == nil && len(wts) == 1
	}, teatest.WithDuration(loadWait))

	quit(t, tm)
}

func TestDeleteWorktree_ShowsToastAfterDeletion(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	_, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	tm.Send(keyRune('d'))
	waitForText(t, tm, "Delete", actionWait)
	tm.Send(keyRune('y'))

	// The toast proves spinnerActive was cleared — if the spinner stays stuck
	// the model never re-renders status messages and this will time out.
	waitForText(t, tm, "deleted worktree feature-a", loadWait)

	quit(t, tm)
}

func TestDeleteCancelWithN(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	tm.Send(keyRune('d'))
	waitForText(t, tm, "Delete", actionWait)
	tm.Send(keyRune('n'))

	// Worktree should still exist
	wts, err := git.ListWorktrees(repo)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 1 {
		t.Errorf("expected 1 worktree after cancel, got %d", len(wts))
	}

	quit(t, tm)
}

// ── clone ─────────────────────────────────────────────────────────────────────

func TestCloneInputAppearsAndCancels(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	_, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	tm.Send(keyRune('c'))
	waitForText(t, tm, "Clone", actionWait)

	tm.Send(keySpecial(tea.KeyEsc))

	quit(t, tm)
}

func TestCloneWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	_, tm := startTUI(t, repoDir)

	// Add an untracked file to the source worktree before starting the TUI
	wtDir := filepath.Join(repoDir, "feature-a")
	testutil.WriteFile(t, wtDir, "untracked.txt", "hello from untracked")

	waitForText(t, tm, "feature-a", loadWait)

	// Open clone input (pre-filled with "feature-a")
	tm.Send(keyRune('c'))
	waitForText(t, tm, "Clone", actionWait)

	// Clear pre-filled value and type new name
	tm.Send(keyCtrl('u'))
	tm.Type("feature-copy")
	tm.Send(keySpecial(tea.KeyEnter))

	// Wait until the untracked file appears in the clone. Waiting for the file
	// (rather than just the worktree in git's list) avoids a race where git
	// reports the worktree as existing before cmdClone has finished copying files.
	clonedFile := filepath.Join(repoDir, "feature-copy", "untracked.txt")
	teatest.WaitFor(t, tm.Output(), func(_ []byte) bool {
		_, err := os.ReadFile(clonedFile)
		return err == nil
	}, teatest.WithDuration(loadWait))

	data, err := os.ReadFile(clonedFile)
	if err != nil {
		t.Fatalf("untracked.txt missing in clone: %v", err)
	}
	if string(data) != "hello from untracked" {
		t.Errorf("untracked.txt content = %q, want %q", string(data), "hello from untracked")
	}

	quit(t, tm)
}

// ── new ───────────────────────────────────────────────────────────────────────

func TestNewInputAppearsAndCancels(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	_, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	tm.Send(keyRune('n'))
	waitForText(t, tm, "New worktree", actionWait)

	tm.Send(keySpecial(tea.KeyEsc))
	quit(t, tm)
}

func TestNewWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	tm.Send(keyRune('n'))
	waitForText(t, tm, "New worktree", actionWait)
	tm.Type("feature-new")
	tm.Send(keySpecial(tea.KeyEnter))

	teatest.WaitFor(t, tm.Output(), func(_ []byte) bool {
		wts, err := git.ListWorktrees(repo)
		if err != nil {
			return false
		}
		for _, wt := range wts {
			if wt.Name == "feature-new" && wt.Branch == "feature-new" {
				return true
			}
		}
		return false
	}, teatest.WithDuration(loadWait))

	quit(t, tm)
}

// ── yank / paste ──────────────────────────────────────────────────────────────

func TestYankModalAppearsAndCancels(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	_, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	tm.Send(keyRune('y'))
	waitForText(t, tm, "Yank files from", actionWait)

	tm.Send(keySpecial(tea.KeyEsc))

	quit(t, tm)
}

func TestYankAndPaste(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")

	// Add an untracked file to feature-a before starting the TUI
	wtDir := filepath.Join(repoDir, "feature-a")
	testutil.WriteFile(t, wtDir, "shared.txt", "hello from feature-a")

	_, tm := startTUI(t, repoDir)
	waitForText(t, tm, "feature-a", loadWait)

	// Yank from feature-a (cursor is on row 0)
	tm.Send(keyRune('y'))
	waitForText(t, tm, "Yank files from", actionWait)
	// Confirm with all items checked
	tm.Send(keySpecial(tea.KeyEnter))

	// Clipboard indicator should appear
	waitForText(t, tm, "feature-a", actionWait)

	// Navigate to feature-b and paste
	tm.Send(keyRune('j'))
	tm.Send(keyRune('p'))

	// Wait until the pasted file appears in feature-b
	teatest.WaitFor(t, tm.Output(), func(_ []byte) bool {
		data, err := os.ReadFile(filepath.Join(repoDir, "feature-b", "shared.txt"))
		return err == nil && string(data) == "hello from feature-a"
	}, teatest.WithDuration(loadWait))

	quit(t, tm)
}

// ── push ──────────────────────────────────────────────────────────────────────

func TestPushWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	wtDir := filepath.Join(repoDir, "feature-a")

	// Push the branch to origin so a remote tracking ref exists, then add a commit.
	testutil.PushBranchWithUpstream(t, wtDir, "origin", "feature-a")
	testutil.WriteFile(t, wtDir, "extra.txt", "more content")
	testutil.CommitAll(t, wtDir, "second commit")

	_, tm := startTUI(t, repoDir)
	waitForText(t, tm, "feature-a", loadWait)

	tm.Send(keyRune('P'))
	waitForText(t, tm, "Push feature-a?", actionWait)
	tm.Send(keyRune('y'))

	waitForText(t, tm, "push complete", loadWait)

	quit(t, tm)
}

func TestPushRejectedShowsForcePushPrompt(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	wtDir := filepath.Join(repoDir, "feature-a")

	// Push to origin then amend the local commit to diverge from remote.
	testutil.PushBranchWithUpstream(t, wtDir, "origin", "feature-a")
	testutil.AmendLastCommit(t, wtDir)

	_, tm := startTUI(t, repoDir)
	waitForText(t, tm, "feature-a", loadWait)

	tm.Send(keyRune('P'))
	waitForText(t, tm, "Push feature-a?", actionWait)
	tm.Send(keyRune('y'))

	// The model should detect divergence before push and show a menu modal.
	waitForTexts(t, tm, loadWait,
		"has diverged from the remote branch",
		"Rebase",
		"Push --force",
		"Abort",
	)

	// Abort with Esc.
	tm.Send(keySpecial(tea.KeyEsc))
	quit(t, tm)
}

func TestPushRejectedForcePushConfirmed(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	wtDir := filepath.Join(repoDir, "feature-a")

	testutil.PushBranchWithUpstream(t, wtDir, "origin", "feature-a")
	testutil.AmendLastCommit(t, wtDir)

	_, tm := startTUI(t, repoDir)
	waitForText(t, tm, "feature-a", loadWait)

	// Trigger push (will be rejected).
	tm.Send(keyRune('P'))
	waitForText(t, tm, "Push feature-a?", actionWait)
	tm.Send(keyRune('y'))
	waitForText(t, tm, "has diverged from the remote branch", loadWait)

	// Choose force push (default is rebase, so move down once).
	tm.Send(keyRune('j'))
	tm.Send(keySpecial(tea.KeyEnter))

	waitForText(t, tm, "force push complete", loadWait)

	quit(t, tm)
}

// ── pull ──────────────────────────────────────────────────────────────────────

func TestPullMainRefreshesBaseStatus(t *testing.T) {
	// Regression: after pulling main the base-status column for feature branches
	// must update. Before the fix, pullResultMsg only refreshed base statuses
	// when the selected branch matched MainBranch — verify the happy path.
	repoDir := testutil.TempBareRepoWithMainWorktreeAhead(t, "feature-a")
	_, tm := startTUI(t, repoDir)

	// Wait for the table and base-status to appear in the same frame.
	// (Using waitForTexts avoids a race where both strings arrive in one render
	// batch and a chained waitForText would see an empty buffer on the second call.)
	waitForTexts(t, tm, loadWait, "main", "✓") // feature-a is rebased on old main

	// Pull main (cursor is on main).
	tm.Send(keyRune('p'))
	waitForText(t, tm, "pull complete", loadWait)

	// After pulling, main advances; feature-a is now behind main → ✗.
	waitForText(t, tm, "✗", loadWait)

	quit(t, tm)
}

func TestStashPullMainRefreshesBaseStatus(t *testing.T) {
	// Regression: the stash-pull path (dirty worktree → stash → pull → pop)
	// was not refreshing base statuses after completing. stashPopResultMsg only
	// ran cmdLoadBaseStatus for "rebase" ops, not "pull". Verify the fix.
	repoDir := testutil.TempBareRepoWithMainWorktreeAhead(t, "feature-a")
	mainWtDir := filepath.Join(repoDir, "main")

	// Make the main worktree dirty (modify a tracked file) so the stash-pull
	// code path is taken. git stash does not stash untracked-only changes.
	testutil.WriteFile(t, mainWtDir, "README.md", "modified")

	_, tm := startTUI(t, repoDir)
	waitForTexts(t, tm, loadWait, "main", "✓") // feature-a rebased on old main

	// Pull main — dirty worktree triggers the stash prompt.
	tm.Send(keyRune('p'))
	waitForText(t, tm, "Stash", actionWait)
	tm.Send(keyRune('y'))

	waitForText(t, tm, "pull complete (stash restored)", loadWait)

	tm.Send(keyRune('o'))
	tm.Send(keyRune('o'))
	waitForTexts(t, tm, actionWait, "$ git stash", "$ git pull", "$ git stash pop")
	tm.Send(keySpecial(tea.KeyEsc))

	// After stash-pull + stash-pop, main advanced; feature-a is behind → ✗.
	waitForText(t, tm, "✗", loadWait)

	quit(t, tm)
}

// ── rename ────────────────────────────────────────────────────────────────────

func TestRenameWorktree_DotBareRepo(t *testing.T) {
	// Regression test: in a .bare-style repo the new worktree path was built
	// using repo.Root (.bare/) instead of repo.LinkedWorktreeDir() (outer/),
	// causing the rename target to land inside .bare/<new-name>.
	outerDir := testutil.TempDotBareRepoWithWorktrees(t, "feature-a")
	repo, tm := startTUI(t, outerDir)

	waitForText(t, tm, "feature-a", loadWait)

	tm.Send(keyRune('r'))
	waitForText(t, tm, "Rename", actionWait)

	tm.Send(keyCtrl('u'))
	tm.Type("feature-renamed")
	tm.Send(keySpecial(tea.KeyEnter))

	teatest.WaitFor(t, tm.Output(), func(_ []byte) bool {
		wts, err := git.ListWorktrees(repo)
		if err != nil {
			return false
		}
		for _, wt := range wts {
			if filepath.Base(wt.Path) == "feature-renamed" {
				// Must be directly under outerDir, not under .bare/
				if wt.Path == filepath.Join(outerDir, "feature-renamed") {
					return true
				}
			}
		}
		return false
	}, teatest.WithDuration(loadWait))

	quit(t, tm)
}

func TestRenameInputAppearsAndCancels(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	_, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	tm.Send(keyRune('r'))
	waitForText(t, tm, "Rename", actionWait)

	// Cancel with esc
	tm.Send(keySpecial(tea.KeyEsc))

	quit(t, tm)
}

func TestRenameWorktree(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a")
	repo, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	// Open rename input (pre-filled with "feature-a")
	tm.Send(keyRune('r'))
	waitForText(t, tm, "Rename", actionWait)

	// Clear the pre-filled value with ctrl+u then type new name
	tm.Send(keyCtrl('u'))
	tm.Type("feature-renamed")
	tm.Send(keySpecial(tea.KeyEnter))

	// Wait until git reports the renamed worktree
	teatest.WaitFor(t, tm.Output(), func(_ []byte) bool {
		wts, err := git.ListWorktrees(repo)
		if err != nil {
			return false
		}
		for _, wt := range wts {
			if wt.Name == "feature-renamed" {
				return true
			}
		}
		return false
	}, teatest.WithDuration(loadWait))

	quit(t, tm)
}

// ── search ────────────────────────────────────────────────────────────────────

func TestSearchModeAppearsAndCancels(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b")
	_, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	tm.Send(keyRune('/'))
	waitForText(t, tm, "Search:", actionWait)

	tm.Send(keySpecial(tea.KeyEsc))

	quit(t, tm)
}

func TestSearchHighlightsAndJumps(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "fix-b", "feature-c")
	_, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	// Enter search and type "fix"
	tm.Send(keyRune('/'))
	waitForText(t, tm, "Search:", actionWait)
	tm.Type("fix")

	// Should show "1/1" (one match)
	waitForText(t, tm, "1/1", actionWait)

	// Exit with enter
	tm.Send(keySpecial(tea.KeyEnter))

	quit(t, tm)
}

func TestSearchCyclesMatches(t *testing.T) {
	repoDir := testutil.TempBareRepoWithWorktrees(t, "feature-a", "feature-b", "fix-c")
	_, tm := startTUI(t, repoDir)

	waitForText(t, tm, "feature-a", loadWait)

	// Enter search and type "feature" — two matches
	tm.Send(keyRune('/'))
	waitForText(t, tm, "Search:", actionWait)
	tm.Type("feature")
	waitForText(t, tm, "1/2", actionWait)

	// ctrl+n → second match
	tm.Send(keyCtrl('n'))
	waitForText(t, tm, "2/2", actionWait)

	// ctrl+p → back to first match
	tm.Send(keyCtrl('p'))
	waitForText(t, tm, "1/2", actionWait)

	tm.Send(keySpecial(tea.KeyEsc))

	quit(t, tm)
}
